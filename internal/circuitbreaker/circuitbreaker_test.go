package circuitbreaker

import (
	"context"
	"testing"
	"time"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := NewCircuitBreaker("test-drone", Config{
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
	})

	// Initial state should be closed
	if !cb.IsClosed() {
		t.Error("Expected circuit breaker to start in closed state")
	}

	// Record failures up to threshold
	ctx := context.Background()
	cb.RecordFailure(ctx)
	cb.RecordFailure(ctx)
	cb.RecordFailure(ctx)

	// Should be open after threshold
	if !cb.IsOpen() {
		t.Error("Expected circuit breaker to be open after threshold failures")
	}

	// Should transition to half-open after reset timeout
	time.Sleep(150 * time.Millisecond)
	allowed, _ := cb.AllowRequest(ctx, "req1")
	if !allowed {
		t.Error("Expected request to be allowed in half-open state")
	}
	if !cb.IsHalfOpen() {
		t.Error("Expected circuit breaker to transition to half-open state")
	}

	// Success should close the circuit
	cb.RecordSuccess(ctx)
	if !cb.IsClosed() {
		t.Error("Expected circuit breaker to close on success")
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker("test-drone", Config{
		FailureThreshold: 2,
		ResetTimeout:     50 * time.Millisecond,
	})

	ctx := context.Background()

	// Open the circuit
	cb.RecordFailure(ctx)
	cb.RecordFailure(ctx)
	if !cb.IsOpen() {
		t.Fatal("Expected circuit breaker to be open")
	}

	// Wait for reset timeout
	time.Sleep(100 * time.Millisecond)

	// Allow request (should transition to half-open)
	allowed, reqID := cb.AllowRequest(ctx, "req1")
	if !allowed || reqID == "" {
		t.Fatal("Expected request to be allowed in half-open state")
	}

	// Failure in half-open should reopen circuit
	cb.RecordFailure(ctx)
	if !cb.IsOpen() {
		t.Error("Expected circuit breaker to reopen on failure in half-open state")
	}
}

func TestCircuitBreaker_RequestFiltering(t *testing.T) {
	cb := NewCircuitBreaker("test-drone", Config{
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
	})

	ctx := context.Background()

	// Open the circuit
	cb.RecordFailure(ctx)
	cb.RecordFailure(ctx)

	// Requests should be rejected in open state (before timeout)
	allowed, _ := cb.AllowRequest(ctx, "req1")
	if allowed {
		t.Error("Expected request to be rejected in open state")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// First request should be allowed (half-open)
	allowed1, reqID1 := cb.AllowRequest(ctx, "req1")
	if !allowed1 {
		t.Error("Expected first request to be allowed in half-open state")
	}

	// Other requests should be rejected in half-open state
	allowed2, _ := cb.AllowRequest(ctx, "req2")
	if allowed2 {
		t.Error("Expected second request to be rejected in half-open state")
	}

	// The tracked request should still be allowed
	allowed3, reqID3 := cb.AllowRequest(ctx, reqID1)
	if !allowed3 || reqID3 != reqID1 {
		t.Error("Expected tracked request to still be allowed")
	}
}

func TestCircuitBreaker_FailureCountReset(t *testing.T) {
	cb := NewCircuitBreaker("test-drone", Config{
		FailureThreshold: 5,
		ResetTimeout:     100 * time.Millisecond,
	})

	ctx := context.Background()

	// Record some failures
	cb.RecordFailure(ctx)
	cb.RecordFailure(ctx)

	if cb.GetFailureCount() != 2 {
		t.Errorf("Expected failure count to be 2, got %d", cb.GetFailureCount())
	}

	// Record success should reset count
	cb.RecordSuccess(ctx)
	if cb.GetFailureCount() != 0 {
		t.Errorf("Expected failure count to be 0 after success, got %d", cb.GetFailureCount())
	}
}

func TestCircuitBreaker_MaxFailures(t *testing.T) {
	cb := NewCircuitBreaker("test-drone", Config{
		FailureThreshold: 3,
		MaxFailures:      5,
		ResetTimeout:     100 * time.Millisecond,
	})

	ctx := context.Background()

	// Record more failures than max
	for i := 0; i < 10; i++ {
		cb.RecordFailure(ctx)
	}

	// Failure count should be capped at max
	if cb.GetFailureCount() > cb.maxFailures {
		t.Errorf("Expected failure count to be capped at %d, got %d", cb.maxFailures, cb.GetFailureCount())
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	cb := NewCircuitBreaker("test-drone", Config{})

	if cb.failureThreshold != 5 {
		t.Errorf("Expected default failure threshold to be 5, got %d", cb.failureThreshold)
	}

	if cb.resetTimeout != 30*time.Second {
		t.Errorf("Expected default reset timeout to be 30s, got %v", cb.resetTimeout)
	}

	if cb.maxFailures != 10 {
		t.Errorf("Expected default max failures to be 10, got %d", cb.maxFailures)
	}
}
