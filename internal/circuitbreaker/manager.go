// Package circuitbreaker implements circuit breaker pattern for tracking and isolating failing Drones.
package circuitbreaker

import (
	"context"
	"sync"
)

// Manager manages circuit breakers for multiple Drones.
type Manager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	config   Config
}

// NewManager creates a new circuit breaker manager.
func NewManager(cfg Config) *Manager {
	return &Manager{
		breakers: make(map[string]*CircuitBreaker),
		config:   cfg,
	}
}

// GetOrCreate gets an existing circuit breaker for a Drone, or creates a new one.
func (m *Manager) GetOrCreate(droneID string) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cb, exists := m.breakers[droneID]; exists {
		return cb
	}

	cb := NewCircuitBreaker(droneID, m.config)
	m.breakers[droneID] = cb
	return cb
}

// Get returns the circuit breaker for a Drone, or nil if it doesn't exist.
func (m *Manager) Get(droneID string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.breakers[droneID]
}

// RecordFailure records a failure for a Drone.
func (m *Manager) RecordFailure(ctx context.Context, droneID string) {
	cb := m.GetOrCreate(droneID)
	cb.RecordFailure(ctx)
}

// RecordSuccess records a success for a Drone.
func (m *Manager) RecordSuccess(ctx context.Context, droneID string) {
	cb := m.GetOrCreate(droneID)
	cb.RecordSuccess(ctx)
}

// AllowRequest checks if a request should be allowed for a Drone.
func (m *Manager) AllowRequest(ctx context.Context, droneID string, requestID string) (bool, string) {
	cb := m.GetOrCreate(droneID)
	return cb.AllowRequest(ctx, requestID)
}

// Remove removes the circuit breaker for a Drone.
func (m *Manager) Remove(droneID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.breakers, droneID)
}

// IsOpen returns true if the circuit breaker for a Drone is open.
func (m *Manager) IsOpen(droneID string) bool {
	cb := m.Get(droneID)
	if cb == nil {
		return false
	}
	return cb.IsOpen()
}

// IsClosed returns true if the circuit breaker for a Drone is closed.
func (m *Manager) IsClosed(droneID string) bool {
	cb := m.Get(droneID)
	if cb == nil {
		return true // If no breaker exists, consider it closed (allow requests)
	}
	return cb.IsClosed()
}
