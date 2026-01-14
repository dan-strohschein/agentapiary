// Package guardrails provides safety guardrails for agent execution.
package guardrails

import (
	"sync"
	"time"
)

// TimeoutTracker tracks request timeouts and enforces response time limits.
type TimeoutTracker struct {
	timeoutSeconds int

	// Active requests being tracked
	activeRequests map[string]time.Time // messageID -> request start time

	mu sync.RWMutex
}

// NewTimeoutTracker creates a new timeout tracker.
func NewTimeoutTracker(timeoutSeconds int) *TimeoutTracker {
	if timeoutSeconds <= 0 {
		return nil // No timeout configured
	}

	return &TimeoutTracker{
		timeoutSeconds: timeoutSeconds,
		activeRequests: make(map[string]time.Time),
	}
}

// StartRequest starts tracking a request.
func (t *TimeoutTracker) StartRequest(messageID string) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.activeRequests[messageID] = time.Now()
}

// CompleteRequest completes tracking for a request.
func (t *TimeoutTracker) CompleteRequest(messageID string) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.activeRequests, messageID)
}

// CheckTimeouts checks for timed-out requests and returns any message IDs that have timed out.
func (t *TimeoutTracker) CheckTimeouts() []string {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	timeoutDuration := time.Duration(t.timeoutSeconds) * time.Second
	timedOut := make([]string, 0)

	for msgID, startTime := range t.activeRequests {
		if now.Sub(startTime) > timeoutDuration {
			timedOut = append(timedOut, msgID)
			delete(t.activeRequests, msgID)
		}
	}

	return timedOut
}

// GetActiveRequestCount returns the number of currently active requests.
func (t *TimeoutTracker) GetActiveRequestCount() int {
	if t == nil {
		return 0
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.activeRequests)
}

// Clear clears all active requests (useful for cleanup).
func (t *TimeoutTracker) Clear() {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.activeRequests = make(map[string]time.Time)
}
