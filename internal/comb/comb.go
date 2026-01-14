// Package comb provides the shared memory store (Comb) for sessions.
package comb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Comb provides session-scoped shared memory storage.
type Comb interface {
	// Get retrieves a value by key for a session.
	Get(ctx context.Context, sessionID, key string) (string, error)
	
	// Set stores a value by key for a session with optional TTL.
	Set(ctx context.Context, sessionID, key, value string, ttl time.Duration) error
	
	// Delete removes a key for a session.
	Delete(ctx context.Context, sessionID, key string) error
	
	// Exists checks if a key exists for a session.
	Exists(ctx context.Context, sessionID, key string) (bool, error)
	
	// Incr increments a numeric value by key for a session.
	Incr(ctx context.Context, sessionID, key string) (int64, error)
	
	// Keys returns all keys matching a pattern for a session.
	Keys(ctx context.Context, sessionID, pattern string) ([]string, error)
	
	// Clear clears all keys for a session.
	Clear(ctx context.Context, sessionID string) error
	
	// Close closes the Comb store.
	Close() error
}

// Store implements Comb using an in-memory map with session isolation.
type Store struct {
	logger    *zap.Logger
	data      map[string]map[string]*entry // sessionID -> key -> entry
	mu        sync.RWMutex
	stopCh    chan struct{}
	wg        sync.WaitGroup
	memoryLimits map[string]int64 // sessionID -> max memory in bytes
	memoryUsage  map[string]int64 // sessionID -> current memory in bytes
}

// entry represents a stored value with metadata.
type entry struct {
	value     string
	expiresAt *time.Time
	mu        sync.RWMutex
}

// NewStore creates a new Comb store.
func NewStore(logger *zap.Logger) *Store {
	s := &Store{
		logger:       logger,
		data:         make(map[string]map[string]*entry),
		stopCh:       make(chan struct{}),
		memoryLimits: make(map[string]int64),
		memoryUsage:  make(map[string]int64),
	}

	// Start expiration goroutine
	s.wg.Add(1)
	go s.runExpiration()

	return s
}

// sessionKey constructs a session-scoped key.
func (s *Store) sessionKey(sessionID, key string) string {
	return fmt.Sprintf("%s:%s", sessionID, key)
}

// Get retrieves a value by key for a session.
func (s *Store) Get(ctx context.Context, sessionID, key string) (string, error) {
	// Check context cancellation
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Hold lock for the entire operation to prevent race conditions
	s.mu.RLock()
	sessionData, exists := s.data[sessionID]
	if !exists {
		s.mu.RUnlock()
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	// Get entry reference while holding lock
	ent, exists := sessionData[key]
	if !exists {
		s.mu.RUnlock()
		return "", fmt.Errorf("key not found: %s", key)
	}

	// Lock entry while still holding store lock (fine for read locks)
	ent.mu.RLock()
	
	// Check expiration while holding both locks
	now := time.Now()
	expired := ent.expiresAt != nil && now.After(*ent.expiresAt)
	
	var value string
	if !expired {
		value = ent.value
	}
	
	// Release entry lock first
	ent.mu.RUnlock()
	
	// Release store lock
	s.mu.RUnlock()

	// If expired, delete it (need write lock)
	if expired {
		s.mu.Lock()
		// Re-check session and key exist (could have been deleted)
		if sessionData, exists := s.data[sessionID]; exists {
			if ent, keyExists := sessionData[key]; keyExists {
				// Update memory usage
				s.memoryUsage[sessionID] -= int64(len(ent.value))
				// Delete expired key
				delete(sessionData, key)
			}
		}
		s.mu.Unlock()
		return "", fmt.Errorf("key expired: %s", key)
	}

	return value, nil
}

// Set stores a value by key for a session with optional TTL.
func (s *Store) Set(ctx context.Context, sessionID, key, value string, ttl time.Duration) error {
	// Check context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check context cancellation again after acquiring lock (in case it was cancelled during wait)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Initialize session data if needed
	if s.data[sessionID] == nil {
		s.data[sessionID] = make(map[string]*entry)
		s.memoryUsage[sessionID] = 0
	}

	sessionData := s.data[sessionID]

	// Check memory limit and update atomically
	valueSize := int64(len(value))
	if limit, hasLimit := s.memoryLimits[sessionID]; hasLimit {
		currentUsage := s.memoryUsage[sessionID]
		// Subtract old value size if key exists
		oldSize := int64(0)
		if oldEntry, exists := sessionData[key]; exists {
			oldSize = int64(len(oldEntry.value))
		}
		newUsage := currentUsage - oldSize + valueSize
		if newUsage > limit {
			return fmt.Errorf("memory limit exceeded for session %s: %d bytes would be used, %d bytes limit", sessionID, newUsage, limit)
		}
		// Update memory usage atomically with the check
		s.memoryUsage[sessionID] = newUsage
	} else {
		// Update memory usage even if no limit (for tracking)
		oldSize := int64(0)
		if oldEntry, exists := sessionData[key]; exists {
			oldSize = int64(len(oldEntry.value))
		}
		s.memoryUsage[sessionID] = s.memoryUsage[sessionID] - oldSize + valueSize
	}

	// Create entry
	ent := &entry{
		value: value,
	}

	// Set expiration if TTL provided
	if ttl > 0 {
		expiresAt := time.Now().Add(ttl)
		ent.expiresAt = &expiresAt
	}

	// Store entry
	sessionData[key] = ent

	return nil
}

// Delete removes a key for a session.
func (s *Store) Delete(ctx context.Context, sessionID, key string) error {
	// Check context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check context cancellation again after acquiring lock
	if ctx.Err() != nil {
		return ctx.Err()
	}

	sessionData, exists := s.data[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	ent, exists := sessionData[key]
	if !exists {
		return nil // Key doesn't exist, no error
	}

	// Update memory usage
	s.memoryUsage[sessionID] -= int64(len(ent.value))

	// Delete key
	delete(sessionData, key)

	return nil
}

// Exists checks if a key exists for a session.
func (s *Store) Exists(ctx context.Context, sessionID, key string) (bool, error) {
	// Check context cancellation
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	// Hold lock for the entire operation to prevent race conditions
	s.mu.RLock()
	sessionData, exists := s.data[sessionID]
	if !exists {
		s.mu.RUnlock()
		return false, nil
	}

	// Get entry reference while holding lock
	ent, exists := sessionData[key]
	if !exists {
		s.mu.RUnlock()
		return false, nil
	}

	// Lock entry while still holding store lock
	ent.mu.RLock()
	
	// Check expiration while holding both locks
	now := time.Now()
	expired := ent.expiresAt != nil && now.After(*ent.expiresAt)
	
	// Release entry lock first
	ent.mu.RUnlock()
	
	// Release store lock
	s.mu.RUnlock()

	if expired {
		// Delete expired key (need write lock)
		s.mu.Lock()
		// Re-check session and key exist (could have been deleted)
		if sessionData, exists := s.data[sessionID]; exists {
			if ent, keyExists := sessionData[key]; keyExists {
				// Update memory usage
				s.memoryUsage[sessionID] -= int64(len(ent.value))
				// Delete expired key
				delete(sessionData, key)
			}
		}
		s.mu.Unlock()
		return false, nil
	}

	return true, nil
}

// Incr increments a numeric value by key for a session.
func (s *Store) Incr(ctx context.Context, sessionID, key string) (int64, error) {
	// Check context cancellation
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check context cancellation again after acquiring lock
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	// Initialize session data if needed
	if s.data[sessionID] == nil {
		s.data[sessionID] = make(map[string]*entry)
		s.memoryUsage[sessionID] = 0
	}

	sessionData := s.data[sessionID]

	// Get or create entry
	ent, exists := sessionData[key]
	if !exists {
		ent = &entry{value: "0"}
		sessionData[key] = ent
	}

	ent.mu.Lock()
	defer ent.mu.Unlock()

	// Parse current value
	var current int64
	if _, err := fmt.Sscanf(ent.value, "%d", &current); err != nil {
		return 0, fmt.Errorf("value is not a number: %s", ent.value)
	}

	// Increment
	current++
	ent.value = fmt.Sprintf("%d", current)

	return current, nil
}

// Keys returns all keys matching a pattern for a session.
func (s *Store) Keys(ctx context.Context, sessionID, pattern string) ([]string, error) {
	// Check context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	s.mu.RLock()
	sessionData, exists := s.data[sessionID]
	s.mu.RUnlock()

	if !exists {
		return []string{}, nil
	}

	var keys []string
	now := time.Now()

	s.mu.RLock()
	for key, ent := range sessionData {
		// Check context cancellation periodically
		if ctx.Err() != nil {
			s.mu.RUnlock()
			return nil, ctx.Err()
		}

		// Check expiration
		ent.mu.RLock()
		expired := ent.expiresAt != nil && now.After(*ent.expiresAt)
		ent.mu.RUnlock()

		if expired {
			continue
		}

		// Simple pattern matching (supports * wildcard)
		if matchesPattern(key, pattern) {
			keys = append(keys, key)
		}
	}
	s.mu.RUnlock()

	return keys, nil
}

// matchesPattern checks if a key matches a pattern (simple * wildcard support).
func matchesPattern(key, pattern string) bool {
	if pattern == "*" {
		return true
	}
	// Simple implementation - can be enhanced with proper glob matching
	return key == pattern
}

// Clear clears all keys for a session.
func (s *Store) Clear(ctx context.Context, sessionID string) error {
	// Check context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if sessionData, exists := s.data[sessionID]; exists {
		// Clear memory usage
		s.memoryUsage[sessionID] = 0
		// Clear data
		for k := range sessionData {
			// Check context cancellation periodically
			if ctx.Err() != nil {
				return ctx.Err()
			}
			delete(sessionData, k)
		}
	}

	return nil
}

// SetMemoryLimit sets the memory limit for a session.
func (s *Store) SetMemoryLimit(sessionID string, limitMB int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memoryLimits[sessionID] = int64(limitMB) * 1024 * 1024
}

// GetMemoryUsage returns the current memory usage for a session.
func (s *Store) GetMemoryUsage(sessionID string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memoryUsage[sessionID]
}

// runExpiration periodically removes expired keys.
func (s *Store) runExpiration() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.expireKeys()
		case <-s.stopCh:
			return
		}
	}
}

// expireKeys removes expired keys.
func (s *Store) expireKeys() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Collect expired keys first to avoid holding entry locks while modifying map
	type expiredKey struct {
		sessionID string
		key       string
		valueSize int64
	}
	var expiredKeys []expiredKey

	for sessionID, sessionData := range s.data {
		for key, ent := range sessionData {
			ent.mu.RLock()
			expired := ent.expiresAt != nil && now.After(*ent.expiresAt)
			var valueSize int64
			if expired {
				// Read value size while holding entry lock
				valueSize = int64(len(ent.value))
			}
			ent.mu.RUnlock()

			if expired {
				expiredKeys = append(expiredKeys, expiredKey{
					sessionID: sessionID,
					key:       key,
					valueSize: valueSize,
				})
			}
		}
	}

	// Now delete expired keys and update memory usage
	for _, ek := range expiredKeys {
		if sessionData, exists := s.data[ek.sessionID]; exists {
			if _, keyExists := sessionData[ek.key]; keyExists {
				// Update memory usage
				s.memoryUsage[ek.sessionID] -= ek.valueSize
				// Delete expired key
				delete(sessionData, ek.key)
			}
		}
	}
}

// Close closes the Comb store.
func (s *Store) Close() error {
	close(s.stopCh)
	s.wg.Wait()
	return nil
}
