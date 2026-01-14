// Package session provides session lifecycle management for Apiary.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Manager manages session lifecycles.
type Manager struct {
	store    apiary.ResourceStore
	logger   *zap.Logger
	sessions map[string]*SessionState
	mu       sync.RWMutex
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// SessionState tracks the state of a session.
type SessionState struct {
	Session     *apiary.Session
	LastActivity time.Time
	mu          sync.RWMutex
}

// Config holds session manager configuration.
type Config struct {
	Store  apiary.ResourceStore
	Logger *zap.Logger
}

// NewManager creates a new session manager.
func NewManager(cfg Config) *Manager {
	return &Manager{
		store:    cfg.Store,
		logger:   cfg.Logger,
		sessions: make(map[string]*SessionState),
		stopCh:   make(chan struct{}),
	}
}

// Start starts the session manager and begins background tasks.
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Session manager starting")

	// Start timeout monitoring
	m.wg.Add(1)
	go m.monitorTimeouts(ctx)

	m.logger.Info("Session manager started")
	return nil
}

// Stop gracefully stops the session manager.
func (m *Manager) Stop(ctx context.Context) error {
	m.logger.Info("Session manager stopping")
	close(m.stopCh)
	
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("Session manager stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CreateSession creates a new session.
func (m *Manager) CreateSession(ctx context.Context, config SessionConfig) (*apiary.Session, error) {
	sessionID := generateSessionID()
	now := time.Now()

	// Set annotations with Hive name if provided
	annotations := make(map[string]string)
	if config.HiveName != "" {
		annotations["apiary.io/hive"] = config.HiveName
	}

	session := &apiary.Session{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Session",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:        sessionID,
			Namespace:   config.Namespace,
			UID:         sessionID,
			CreatedAt:   now,
			UpdatedAt:   now,
			Annotations: annotations,
		},
		Status: apiary.SessionStatus{
			Phase:        apiary.SessionPhaseCreated,
			LastActivity: now,
		},
		Spec: apiary.SessionConfig{
			TimeoutMinutes:     config.TimeoutMinutes,
			MaxDurationMinutes: config.MaxDurationMinutes,
			PersistOnTerminate:  config.PersistOnTerminate,
			PersistPath:         config.PersistPath,
			MaxMemoryMB:         config.MaxMemoryMB,
		},
	}

	// Store session
	if err := m.store.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	// Track session state
	m.mu.Lock()
	m.sessions[sessionID] = &SessionState{
		Session:      session,
		LastActivity: now,
	}
	m.mu.Unlock()

	m.logger.Info("Created session",
		zap.String("id", sessionID),
		zap.String("namespace", config.Namespace),
	)

	return session, nil
}

// GetSession retrieves a session.
func (m *Manager) GetSession(ctx context.Context, id string) (*apiary.Session, error) {
	m.mu.RLock()
	state, exists := m.sessions[id]
	m.mu.RUnlock()

	if exists {
		state.mu.RLock()
		session := state.Session
		state.mu.RUnlock()
		return session, nil
	}

	// Try to load from store - search all namespaces since we don't know which one
	allSessions, err := m.store.List(ctx, "Session", "", nil)
	if err != nil {
		return nil, err
	}

	// Find session by UID or name
	for _, s := range allSessions {
		if s.GetUID() == id || s.GetName() == id {
			session, ok := s.(*apiary.Session)
			if ok {
				// Cache it
				m.mu.Lock()
				m.sessions[id] = &SessionState{
					Session:      session,
					LastActivity: session.Status.LastActivity,
				}
				m.mu.Unlock()
				return session, nil
			}
		}
	}

	return nil, apiary.ErrNotFound
}

// UpdateActivity updates the last activity time for a session.
func (m *Manager) UpdateActivity(ctx context.Context, id string) error {
	m.mu.Lock()
	state, exists := m.sessions[id]
	m.mu.Unlock()

	if !exists {
		// Load from store
		session, err := m.GetSession(ctx, id)
		if err != nil {
			return err
		}
		m.mu.Lock()
		state = &SessionState{
			Session:      session,
			LastActivity: time.Now(),
		}
		m.sessions[id] = state
		m.mu.Unlock()
	}

	now := time.Now()
	state.mu.Lock()
	state.LastActivity = now
	state.Session.Status.LastActivity = now
	state.Session.Status.Phase = apiary.SessionPhaseActive
	state.mu.Unlock()

	// Update in store
	if err := m.store.Update(ctx, state.Session); err != nil {
		m.logger.Warn("Failed to update session activity in store",
			zap.String("id", id),
			zap.Error(err),
		)
	}

	return nil
}

// TerminateSession terminates a session.
func (m *Manager) TerminateSession(ctx context.Context, id string) error {
	// Get session first to ensure we have it
	session, err := m.GetSession(ctx, id)
	if err != nil {
		return err
	}

	// Update phase
	session.Status.Phase = apiary.SessionPhaseTerminated

	m.mu.Lock()
	if state, exists := m.sessions[id]; exists {
		state.mu.Lock()
		state.Session.Status.Phase = apiary.SessionPhaseTerminated
		state.mu.Unlock()
	}
	m.mu.Unlock()

	// Persist if configured
	if session.Spec.PersistOnTerminate && session.Spec.PersistPath != "" {
		if err := m.persistSession(ctx, session); err != nil {
			m.logger.Warn("Failed to persist session",
				zap.String("id", id),
				zap.Error(err),
			)
		}
	}

	// Update in store (use session's namespace)
	if err := m.store.Update(ctx, session); err != nil {
		m.logger.Warn("Failed to update session in store",
			zap.String("id", id),
			zap.String("namespace", session.GetNamespace()),
			zap.Error(err),
		)
	}

	// Remove from cache
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()

	m.logger.Info("Terminated session", zap.String("id", id))
	return nil
}

// monitorTimeouts monitors sessions for timeout and max duration violations.
func (m *Manager) monitorTimeouts(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkTimeouts(ctx)
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// checkTimeouts checks all sessions for timeout violations.
func (m *Manager) checkTimeouts(ctx context.Context) {
	now := time.Now()

	m.mu.RLock()
	sessions := make([]*SessionState, 0, len(m.sessions))
	for _, state := range m.sessions {
		sessions = append(sessions, state)
	}
	m.mu.RUnlock()

	for _, state := range sessions {
		state.mu.RLock()
		session := state.Session
		lastActivity := state.LastActivity
		state.mu.RUnlock()

		// Check idle timeout
		if session.Spec.TimeoutMinutes > 0 {
			idleDuration := now.Sub(lastActivity)
			if idleDuration > time.Duration(session.Spec.TimeoutMinutes)*time.Minute {
				m.logger.Info("Session timed out (idle)",
					zap.String("id", session.GetUID()),
					zap.Duration("idle", idleDuration),
				)
				_ = m.TerminateSession(ctx, session.GetUID())
				continue
			}
		}

		// Check max duration
		if session.Spec.MaxDurationMinutes > 0 {
			sessionDuration := now.Sub(session.GetCreatedAt())
			if sessionDuration > time.Duration(session.Spec.MaxDurationMinutes)*time.Minute {
				m.logger.Info("Session timed out (max duration)",
					zap.String("id", session.GetUID()),
					zap.Duration("duration", sessionDuration),
				)
				_ = m.TerminateSession(ctx, session.GetUID())
				continue
			}
		}

		// Update phase to idle if inactive
		if session.Status.Phase == apiary.SessionPhaseActive {
			idleDuration := now.Sub(lastActivity)
			if idleDuration > 5*time.Minute {
				state.mu.Lock()
				state.Session.Status.Phase = apiary.SessionPhaseIdle
				state.mu.Unlock()
			}
		}
	}
}

// persistSession persists a session to disk.
func (m *Manager) persistSession(ctx context.Context, session *apiary.Session) error {
	// TODO: Implement persistence to disk
	// For now, just log that persistence would happen
	m.logger.Info("Session persistence not yet implemented",
		zap.String("id", session.GetUID()),
		zap.String("path", session.Spec.PersistPath),
	)
	return nil
}

// generateSessionID generates a unique session ID.
func generateSessionID() string {
	return fmt.Sprintf("sess-%s", uuid.New().String()[:8])
}

// SessionConfig holds configuration for creating a session.
type SessionConfig struct {
	Namespace          string
	HiveName           string // Optional: Hive name if session belongs to a Hive
	TimeoutMinutes     int
	MaxDurationMinutes int
	PersistOnTerminate bool
	PersistPath        string
	MaxMemoryMB        int
}
