// Package guardrails provides safety guardrails for agent execution.
package guardrails

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	// Requests per minute
	requestsPerMinute int

	// Burst size (max tokens in bucket)
	burstSize int

	// Current tokens available
	tokens int

	// Last refill time
	lastRefill time.Time

	// Refill interval (how often to add tokens)
	refillInterval time.Duration
	// Tokens per refill
	tokensPerRefill int

	mu sync.Mutex
}

// NewRateLimiter creates a new rate limiter.
// requestsPerMinute: maximum requests per minute
// burstSize: maximum burst size (0 = use requestsPerMinute)
func NewRateLimiter(requestsPerMinute int, burstSize int) *RateLimiter {
	if requestsPerMinute <= 0 {
		// No rate limit
		return nil
	}

	if burstSize <= 0 {
		// Default burst size is equal to requests per minute
		burstSize = requestsPerMinute
	}

	// Refill every second with tokensPerSecond tokens
	tokensPerSecond := requestsPerMinute / 60
	if tokensPerSecond < 1 {
		tokensPerSecond = 1
	}

	return &RateLimiter{
		requestsPerMinute: requestsPerMinute,
		burstSize:         burstSize,
		tokens:            burstSize, // Start with full bucket
		lastRefill:        time.Now(),
		refillInterval:    time.Second,
		tokensPerRefill:   tokensPerSecond,
	}
}

// Allow checks if a request is allowed.
// Returns true if allowed, false if rate limited.
func (r *RateLimiter) Allow(ctx context.Context) bool {
	if r == nil {
		// No rate limit configured
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Refill tokens based on elapsed time
	elapsed := now.Sub(r.lastRefill)
	if elapsed > 0 {
		refills := int(elapsed / r.refillInterval)
		if refills > 0 {
			newTokens := refills * r.tokensPerRefill
			r.tokens += newTokens
			if r.tokens > r.burstSize {
				r.tokens = r.burstSize
			}
			r.lastRefill = now
		}
	}

	// Check if we have tokens available
	if r.tokens > 0 {
		r.tokens--
		return true
	}

	return false
}

// Wait waits until a request is allowed (with context cancellation support).
// Returns error if context is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r == nil {
		// No rate limit configured
		return nil
	}

	// Try to allow immediately
	if r.Allow(ctx) {
		return nil
	}

	// Calculate wait time until next token is available
	waitTime := r.refillInterval

	// Wait with context cancellation
	ticker := time.NewTicker(waitTime)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if r.Allow(ctx) {
				return nil
			}
		}
	}
}

// RateLimitExceededError indicates that a rate limit has been exceeded.
type RateLimitExceededError struct {
	RequestsPerMinute int
}

func (e *RateLimitExceededError) Error() string {
	return fmt.Sprintf("rate limit exceeded: %d requests per minute", e.RequestsPerMinute)
}

// RequestsPerMinute returns the configured requests per minute limit.
func (r *RateLimiter) RequestsPerMinute() int {
	if r == nil {
		return 0
	}
	return r.requestsPerMinute
}
