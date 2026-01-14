// Package badger provides a BadgerDB implementation of the ResourceStore interface.
package badger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agentapiary/apiary/internal/secrets"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/dgraph-io/badger/v4"
)

// Store implements ResourceStore using BadgerDB.
type Store struct {
	db               *badger.DB
	mu               sync.RWMutex
	watchers         map[string]map[chan apiary.ResourceEvent]struct{}
	watcherMu        sync.RWMutex
	stopCh           chan struct{}
	stopWg           sync.WaitGroup
	maxVersions      int // Maximum number of versions to keep per resource
	pruneInterval    time.Duration
	secretPassphrase string // Passphrase for encrypting/decrypting secrets
}

// NewStore creates a new BadgerDB store.
func NewStore(dataDir string) (*Store, error) {
	opts := badger.DefaultOptions(filepath.Join(dataDir, "badger"))
	opts.Logger = nil // Disable Badger's default logging

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	// Get encryption passphrase from environment variable
	// Default to a warning if not set (for development)
	passphrase := os.Getenv("APIARY_SECRET_PASSPHRASE")
	if passphrase == "" {
		// In production, this should be set. For now, use a default for development.
		// TODO: Make this required in production
		passphrase = "default-dev-passphrase-change-in-production"
	}

	s := &Store{
		db:               db,
		watchers:         make(map[string]map[chan apiary.ResourceEvent]struct{}),
		stopCh:           make(chan struct{}),
		maxVersions:      10,            // Keep last 10 versions by default
		pruneInterval:    1 * time.Hour, // Prune every hour
		secretPassphrase: passphrase,
	}

	// Start background goroutine for TTL and cleanup
	s.stopWg.Add(1)
	go s.runBackgroundTasks()

	return s, nil
}

// runBackgroundTasks runs background maintenance tasks.
func (s *Store) runBackgroundTasks() {
	defer s.stopWg.Done()

	gcTicker := time.NewTicker(5 * time.Minute)
	defer gcTicker.Stop()

	pruneTicker := time.NewTicker(s.pruneInterval)
	defer pruneTicker.Stop()

	for {
		select {
		case <-gcTicker.C:
			// Run garbage collection
			for {
				err := s.db.RunValueLogGC(0.5)
				if err == badger.ErrNoRewrite {
					break
				}
			}
		case <-pruneTicker.C:
			// Prune old versions
			s.pruneOldVersions()
		case <-s.stopCh:
			return
		}
	}
}

// key constructs a storage key from kind, name, and namespace.
func (s *Store) key(kind, name, namespace string) []byte {
	if namespace == "" {
		return []byte(fmt.Sprintf("/%s/%s", kind, name))
	}
	return []byte(fmt.Sprintf("/%s/%s/%s", namespace, kind, name))
}

// versionKey constructs a versioned storage key.
func (s *Store) versionKey(kind, name, namespace string, version int) []byte {
	base := s.key(kind, name, namespace)
	return []byte(fmt.Sprintf("%s:v%d", string(base), version))
}

// Create stores a new resource.
func (s *Store) Create(ctx context.Context, resource apiary.Resource) error {
	// Generate UID if not set
	ensureUID(resource)

	key := s.key(resource.GetKind(), resource.GetName(), resource.GetNamespace())

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if resource already exists
		_, err := txn.Get(key)
		if err == nil {
			return apiary.ErrAlreadyExists
		}
		if err != badger.ErrKeyNotFound {
			return fmt.Errorf("failed to check existence: %w", err)
		}

		// Encrypt Secret data before storing
		if err := s.encryptSecretData(resource); err != nil {
			return fmt.Errorf("failed to encrypt secret data: %w", err)
		}

		// Serialize resource
		data, err := json.Marshal(resource)
		if err != nil {
			return fmt.Errorf("failed to marshal resource: %w", err)
		}

		// Store resource
		entry := badger.NewEntry(key, data)
		if err := txn.SetEntry(entry); err != nil {
			return fmt.Errorf("failed to set entry: %w", err)
		}

		// Store version 1
		versionKey := s.versionKey(resource.GetKind(), resource.GetName(), resource.GetNamespace(), 1)
		versionData := &versionEntry{
			Version:   1,
			Timestamp: time.Now(),
			Data:      data,
		}
		versionBytes, err := json.Marshal(versionData)
		if err != nil {
			return fmt.Errorf("failed to marshal version: %w", err)
		}
		if err := txn.SetEntry(badger.NewEntry(versionKey, versionBytes)); err != nil {
			return fmt.Errorf("failed to set version: %w", err)
		}

		// Notify watchers
		s.notifyWatchers(resource.GetKind(), resource.GetNamespace(), apiary.EventTypeCreated, resource)

		return nil
	})
}

// Get retrieves a resource.
func (s *Store) Get(ctx context.Context, kind, name, namespace string) (apiary.Resource, error) {
	key := s.key(kind, name, namespace)

	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err == badger.ErrKeyNotFound {
			return apiary.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get key: %w", err)
		}

		data, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		return nil, err
	}

	// Deserialize based on kind
	resource, err := s.unmarshalResource(kind, data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource: %w", err)
	}

	// Decrypt Secret data after retrieving
	if err := s.decryptSecretData(resource); err != nil {
		return nil, fmt.Errorf("failed to decrypt secret data: %w", err)
	}

	return resource, nil
}

// Update updates an existing resource.
func (s *Store) Update(ctx context.Context, resource apiary.Resource) error {
	key := s.key(resource.GetKind(), resource.GetName(), resource.GetNamespace())

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if resource exists
		_, err := txn.Get(key)
		if err == badger.ErrKeyNotFound {
			return apiary.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get existing resource: %w", err)
		}

		// Find next version number efficiently by scanning version keys
		// Use iterator to find the highest version number
		baseKey := s.key(resource.GetKind(), resource.GetName(), resource.GetNamespace())
		versionPrefix := []byte(fmt.Sprintf("%s:v", string(baseKey)))

		opts := badger.DefaultIteratorOptions
		opts.Prefix = versionPrefix
		it := txn.NewIterator(opts)
		defer it.Close()

		maxVersion := 0
		for it.Rewind(); it.Valid(); it.Next() {
			// Extract version number from key (format: /kind/name:vN)
			keyStr := string(it.Item().Key())
			// Find the version number after ":v"
			if idx := strings.LastIndex(keyStr, ":v"); idx != -1 {
				var version int
				if _, err := fmt.Sscanf(keyStr[idx+2:], "%d", &version); err == nil {
					if version > maxVersion {
						maxVersion = version
					}
				}
			}
		}

		version := maxVersion + 1

		// Encrypt Secret data before storing
		if err := s.encryptSecretData(resource); err != nil {
			return fmt.Errorf("failed to encrypt secret data: %w", err)
		}

		// Serialize resource
		data, err := json.Marshal(resource)
		if err != nil {
			return fmt.Errorf("failed to marshal resource: %w", err)
		}

		// Update resource
		entry := badger.NewEntry(key, data)
		if err := txn.SetEntry(entry); err != nil {
			return fmt.Errorf("failed to set entry: %w", err)
		}

		// Store new version
		versionKey := s.versionKey(resource.GetKind(), resource.GetName(), resource.GetNamespace(), version)
		versionData := &versionEntry{
			Version:   version,
			Timestamp: time.Now(),
			Data:      data,
		}
		versionBytes, err := json.Marshal(versionData)
		if err != nil {
			return fmt.Errorf("failed to marshal version: %w", err)
		}
		if err := txn.SetEntry(badger.NewEntry(versionKey, versionBytes)); err != nil {
			return fmt.Errorf("failed to set version: %w", err)
		}

		// Notify watchers
		s.notifyWatchers(resource.GetKind(), resource.GetNamespace(), apiary.EventTypeUpdated, resource)

		return nil
	})
}

// Delete removes a resource.
func (s *Store) Delete(ctx context.Context, kind, name, namespace string) error {
	key := s.key(kind, name, namespace)

	return s.db.Update(func(txn *badger.Txn) error {
		// Get resource before deleting (for watcher notification)
		item, err := txn.Get(key)
		if err == badger.ErrKeyNotFound {
			return apiary.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get resource: %w", err)
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("failed to copy value: %w", err)
		}

		resource, err := s.unmarshalResource(kind, data)
		if err != nil {
			// If we can't unmarshal, still delete but skip notification
			resource = nil
		}

		// Delete resource
		if err := txn.Delete(key); err != nil {
			return fmt.Errorf("failed to delete key: %w", err)
		}

		// Notify watchers
		if resource != nil {
			s.notifyWatchers(kind, namespace, apiary.EventTypeDeleted, resource)
		}

		return nil
	})
}

// List returns all resources of the given kind in the namespace.
func (s *Store) List(ctx context.Context, kind, namespace string, selector apiary.Labels) ([]apiary.Resource, error) {
	var resources []apiary.Resource
	prefix := s.listPrefix(kind, namespace)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()

			// Skip version keys (they contain ":v" followed by a number)
			keyStr := string(key)
			if strings.Contains(keyStr, ":v") {
				continue
			}

			data, err := item.ValueCopy(nil)
			if err != nil {
				continue
			}

			resource, err := s.unmarshalResource(kind, data)
			if err != nil {
				continue
			}

			// Apply selector filter
			if selector != nil && !resource.GetLabels().Match(selector) {
				continue
			}

			resources = append(resources, resource)
		}

		return nil
	})

	return resources, err
}

// listPrefix returns the prefix for listing resources.
func (s *Store) listPrefix(kind, namespace string) []byte {
	if namespace == "" {
		return []byte(fmt.Sprintf("/%s/", kind))
	}
	return []byte(fmt.Sprintf("/%s/%s/", namespace, kind))
}

// Watch returns a channel that emits events when resources change.
func (s *Store) Watch(ctx context.Context, kind, namespace string) (<-chan apiary.ResourceEvent, error) {
	ch := make(chan apiary.ResourceEvent, 100)
	watchKey := fmt.Sprintf("%s:%s", kind, namespace)

	s.watcherMu.Lock()
	if s.watchers[watchKey] == nil {
		s.watchers[watchKey] = make(map[chan apiary.ResourceEvent]struct{})
	}
	s.watchers[watchKey][ch] = struct{}{}
	s.watcherMu.Unlock()

	// Cleanup when context is done
	go func() {
		<-ctx.Done()
		s.watcherMu.Lock()
		// Check if channel still exists (may have been closed by Store.Close())
		if watcherMap, exists := s.watchers[watchKey]; exists {
			if _, chExists := watcherMap[ch]; chExists {
				delete(watcherMap, ch)
				if len(watcherMap) == 0 {
					delete(s.watchers, watchKey)
				}
				// Close channel only if we successfully removed it
				// Use select to avoid panic if already closed
				select {
				case <-ch:
					// Channel already closed
				default:
					close(ch)
				}
			}
		}
		s.watcherMu.Unlock()
	}()

	return ch, nil
}

// notifyWatchers notifies all watchers of a resource change.
func (s *Store) notifyWatchers(kind, namespace string, eventType apiary.EventType, resource apiary.Resource) {
	watchKey := fmt.Sprintf("%s:%s", kind, namespace)

	s.watcherMu.RLock()
	watchers := s.watchers[watchKey]
	if watchers == nil {
		s.watcherMu.RUnlock()
		return
	}

	// Copy watchers to avoid holding lock while sending
	watcherList := make([]chan apiary.ResourceEvent, 0, len(watchers))
	for ch := range watchers {
		watcherList = append(watcherList, ch)
	}
	s.watcherMu.RUnlock()

	event := apiary.ResourceEvent{
		Type:     eventType,
		Resource: resource,
	}

	// Send to all watchers (non-blocking)
	// If channel is full, we close the channel to signal the watcher
	// that events are being dropped, allowing them to handle it appropriately
	for _, ch := range watcherList {
		select {
		case ch <- event:
			// Successfully sent
		default:
			// Channel full - watcher is falling behind
			// Remove from watchers map first to prevent future sends
			s.watcherMu.Lock()
			if watcherMap, exists := s.watchers[watchKey]; exists {
				delete(watcherMap, ch)
				if len(watcherMap) == 0 {
					delete(s.watchers, watchKey)
				}
			}
			s.watcherMu.Unlock()

			// Close channel to signal the watcher
			// Use recover to safely handle case where channel is already closed
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Channel already closed, which is fine
						// This can happen if the watcher closed it or another goroutine did
					}
				}()
				close(ch)
			}()
		}
	}
}

// unmarshalResource deserializes a resource based on its kind.
func (s *Store) unmarshalResource(kind string, data []byte) (apiary.Resource, error) {
	switch kind {
	case "AgentSpec":
		var spec apiary.AgentSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, err
		}
		return &spec, nil
	case "Drone":
		var drone apiary.Drone
		if err := json.Unmarshal(data, &drone); err != nil {
			return nil, err
		}
		return &drone, nil
	case "Hive":
		var hive apiary.Hive
		if err := json.Unmarshal(data, &hive); err != nil {
			return nil, err
		}
		return &hive, nil
	case "Cell":
		var cell apiary.Cell
		if err := json.Unmarshal(data, &cell); err != nil {
			return nil, err
		}
		return &cell, nil
	case "Session":
		var session apiary.Session
		if err := json.Unmarshal(data, &session); err != nil {
			return nil, err
		}
		return &session, nil
	case "Secret":
		var secret apiary.Secret
		if err := json.Unmarshal(data, &secret); err != nil {
			return nil, err
		}
		return &secret, nil
	default:
		return nil, fmt.Errorf("unknown resource kind: %s", kind)
	}
}

// encryptSecretData encrypts Secret.Data values before storing.
func (s *Store) encryptSecretData(resource apiary.Resource) error {
	secret, ok := resource.(*apiary.Secret)
	if !ok {
		return nil // Not a Secret, nothing to encrypt
	}

	if secret.Data == nil {
		return nil // No data to encrypt
	}

	// Encrypt each value in the Data map
	encryptedData := make(map[string][]byte, len(secret.Data))
	for key, value := range secret.Data {
		encrypted, err := secrets.Encrypt(value, s.secretPassphrase)
		if err != nil {
			return fmt.Errorf("failed to encrypt secret key %s: %w", key, err)
		}
		// Store encrypted data as base64-encoded string in []byte
		encryptedData[key] = encrypted
	}

	secret.Data = encryptedData
	return nil
}

// decryptSecretData decrypts Secret.Data values after retrieving.
func (s *Store) decryptSecretData(resource apiary.Resource) error {
	secret, ok := resource.(*apiary.Secret)
	if !ok {
		return nil // Not a Secret, nothing to decrypt
	}

	if secret.Data == nil {
		return nil // No data to decrypt
	}

	// Decrypt each value in the Data map
	decryptedData := make(map[string][]byte, len(secret.Data))
	for key, encryptedValue := range secret.Data {
		decrypted, err := secrets.Decrypt(encryptedValue, s.secretPassphrase)
		if err != nil {
			return fmt.Errorf("failed to decrypt secret key %s: %w", key, err)
		}
		decryptedData[key] = decrypted
	}

	secret.Data = decryptedData
	return nil
}

// Close closes the store and releases all resources.
func (s *Store) Close() error {
	// Signal background goroutine to stop
	close(s.stopCh)

	// Wait for background goroutine to finish
	s.stopWg.Wait()

	// Close all watcher channels
	s.watcherMu.Lock()
	for watchKey, watcherMap := range s.watchers {
		for ch := range watcherMap {
			// Use select to avoid panic if channel already closed
			select {
			case <-ch:
				// Channel already closed
			default:
				close(ch)
			}
		}
		delete(s.watchers, watchKey)
	}
	s.watcherMu.Unlock()

	return s.db.Close()
}

// versionEntry represents a versioned resource entry.
type versionEntry struct {
	Version   int       `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"`
}

// GetVersion retrieves a specific version of a resource.
func (s *Store) GetVersion(ctx context.Context, kind, name, namespace string, version int) (apiary.Resource, error) {
	versionKey := s.versionKey(kind, name, namespace, version)

	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(versionKey)
		if err == badger.ErrKeyNotFound {
			return apiary.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get version: %w", err)
		}

		data, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		return nil, err
	}

	var versionEntry versionEntry
	if err := json.Unmarshal(data, &versionEntry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal version entry: %w", err)
	}

	resource, err := s.unmarshalResource(kind, versionEntry.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource: %w", err)
	}

	// Decrypt Secret data after retrieving
	if err := s.decryptSecretData(resource); err != nil {
		return nil, fmt.Errorf("failed to decrypt secret data: %w", err)
	}

	return resource, nil
}

// ListVersions returns all versions of a resource.
func (s *Store) ListVersions(ctx context.Context, kind, name, namespace string) ([]Version, error) {
	baseKey := s.key(kind, name, namespace)
	prefix := []byte(fmt.Sprintf("%s:v", string(baseKey)))

	var versions []Version
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			data, err := item.ValueCopy(nil)
			if err != nil {
				continue
			}

			var versionEntry versionEntry
			if err := json.Unmarshal(data, &versionEntry); err != nil {
				continue
			}

			versions = append(versions, Version{
				Version:   versionEntry.Version,
				Timestamp: versionEntry.Timestamp,
			})
		}

		return nil
	})

	return versions, err
}

// Version represents a resource version.
type Version struct {
	Version   int
	Timestamp time.Time
}

// pruneOldVersions removes old versions beyond maxVersions.
func (s *Store) pruneOldVersions() {
	if s.maxVersions <= 0 {
		return // No pruning if maxVersions is 0 or negative
	}

	err := s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("/")
		it := txn.NewIterator(opts)
		defer it.Close()

		// Track versions per resource
		resourceVersions := make(map[string][]int)

		// First pass: collect all version keys
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())

			// Check if this is a version key (contains :v followed by number)
			if strings.Contains(key, ":v") {
				// Extract resource key and version
				parts := strings.Split(key, ":v")
				if len(parts) == 2 {
					resourceKey := parts[0]
					var version int
					if _, err := fmt.Sscanf(parts[1], "%d", &version); err == nil {
						resourceVersions[resourceKey] = append(resourceVersions[resourceKey], version)
					}
				}
			}
		}

		// Second pass: delete old versions
		for resourceKey, versions := range resourceVersions {
			if len(versions) <= s.maxVersions {
				continue // Not enough versions to prune
			}

			// Sort versions (ascending order) to ensure we delete the oldest
			// Simple bubble sort for small arrays (versions per resource)
			for i := 0; i < len(versions)-1; i++ {
				for j := 0; j < len(versions)-i-1; j++ {
					if versions[j] > versions[j+1] {
						versions[j], versions[j+1] = versions[j+1], versions[j]
					}
				}
			}

			// Keep the highest N versions, delete the oldest ones
			versionsToDelete := len(versions) - s.maxVersions
			for i := 0; i < versionsToDelete; i++ {
				versionKey := []byte(fmt.Sprintf("%s:v%d", resourceKey, versions[i]))
				if err := txn.Delete(versionKey); err != nil {
					// Continue on error
					continue
				}
			}
		}

		return nil
	})

	if err != nil {
		// Log error but don't fail - pruning is best effort
		return
	}
}

// SetMaxVersions sets the maximum number of versions to keep per resource.
func (s *Store) SetMaxVersions(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxVersions = max
}
