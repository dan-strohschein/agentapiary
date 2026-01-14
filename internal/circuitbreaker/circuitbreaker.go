// Package circuitbreaker implements circuit breaker pattern for tracking and isolating failing Drones.
package circuitbreaker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	// StateClosed indicates the circuit is closed (normal operation).
	StateClosed State = iota
	// StateOpen indicates the circuit is open (failing, requests rejected).
	StateOpen
	// StateHalfOpen indicates the circuit is half-open (testing recovery).
	StateHalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker tracks failures and manages circuit state for a Drone.
type CircuitBreaker struct {
	droneID string

	// Configuration
	failureThreshold int           // Number of failures before opening circuit
	resetTimeout     time.Duration // Time to wait before attempting half-open
	maxFailures      int           // Maximum failures to track

	// State
	mu            sync.RWMutex
	state         State
	failureCount  int
	lastFailure   time.Time
	halfOpenStart time.Time
	halfOpenRequestID string // Track the single request allowed in half-open state
}

// Config holds circuit breaker configuration.
type Config struct {
	FailureThreshold int           // Number of failures before opening circuit (default: 5)
	ResetTimeout     time.Duration // Time to wait before attempting half-open (default: 30s)
	MaxFailures      int           // Maximum failures to track (default: 10)
}

// NewCircuitBreaker creates a new circuit breaker for a Drone.
func NewCircuitBreaker(droneID string, cfg Config) *CircuitBreaker {
	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = 5
	}
	resetTimeout := cfg.ResetTimeout
	if resetTimeout <= 0 {
		resetTimeout = 30 * time.Second
	}
	maxFailures := cfg.MaxFailures
	if maxFailures <= 0 {
		maxFailures = 10
	}

	return &CircuitBreaker{
		droneID:          droneID,
		failureThreshold: threshold,
		resetTimeout:     resetTimeout,
		maxFailures:      maxFailures,
		state:            StateClosed,
	}
}

// RecordFailure records a failure for the Drone.
func (cb *CircuitBreaker) RecordFailure(ctx context.Context) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	if cb.failureCount > cb.maxFailures {
		// Reset counter if it exceeds max (prevent unbounded growth)
		cb.failureCount = cb.maxFailures
	}
	cb.lastFailure = time.Now()

	// Transition to open if threshold exceeded
	if cb.state == StateClosed && cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
	} else if cb.state == StateHalfOpen {
		// If we're in half-open and get a failure, reopen the circuit
		cb.state = StateOpen
		cb.halfOpenRequestID = ""
	}
}

// RecordSuccess records a success for the Drone.
func (cb *CircuitBreaker) RecordSuccess(ctx context.Context) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		// Success in half-open state closes the circuit
		cb.state = StateClosed
		cb.failureCount = 0
		cb.halfOpenRequestID = ""
	} else if cb.state == StateClosed {
		// Reset failure count on success in closed state
		cb.failureCount = 0
	}
}

// AllowRequest checks if a request should be allowed.
// Returns true if allowed, false if rejected, and an optional request ID for tracking in half-open state.
func (cb *CircuitBreaker) AllowRequest(ctx context.Context, requestID string) (bool, string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		// Always allow requests in closed state
		return true, ""

	case StateOpen:
		// Check if reset timeout has passed
		if now.Sub(cb.lastFailure) >= cb.resetTimeout {
			// Transition to half-open
			cb.state = StateHalfOpen
			cb.halfOpenStart = now
			cb.halfOpenRequestID = requestID
			return true, requestID
		}
		// Reject requests in open state
		return false, ""

	case StateHalfOpen:
		// Only allow the tracked request in half-open state
		if cb.halfOpenRequestID == "" {
			// No request tracked yet, allow this one
			cb.halfOpenRequestID = requestID
			return true, requestID
		}
		if cb.halfOpenRequestID == requestID {
			// This is the tracked request
			return true, requestID
		}
		// Reject other requests
		return false, ""

	default:
		return false, ""
	}
}

// GetState returns the current circuit breaker state.
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailureCount returns the current failure count.
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failureCount
}

// IsOpen returns true if the circuit is open.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateOpen
}

// IsClosed returns true if the circuit is closed.
func (cb *CircuitBreaker) IsClosed() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateClosed
}

// IsHalfOpen returns true if the circuit is half-open.
func (cb *CircuitBreaker) IsHalfOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateHalfOpen
}

// Error represents a circuit breaker error.
type Error struct {
	DroneID string
	State   State
}

func (e *Error) Error() string {
	return fmt.Sprintf("circuit breaker open for drone %s (state: %s)", e.DroneID, e.State.String())
}
