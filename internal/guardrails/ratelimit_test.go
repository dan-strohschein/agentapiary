package guardrails

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter_ValidConfig(t *testing.T) {
	limiter := NewRateLimiter(60, 0)
	if limiter == nil {
		t.Fatal("Expected rate limiter to be created")
	}
	if limiter.RequestsPerMinute() != 60 {
		t.Errorf("Expected requests per minute to be 60, got %d", limiter.RequestsPerMinute())
	}
}

func TestNewRateLimiter_ZeroLimit(t *testing.T) {
	limiter := NewRateLimiter(0, 0)
	if limiter != nil {
		t.Error("Expected rate limiter to be nil for zero limit")
	}
}

func TestNewRateLimiter_DefaultBurst(t *testing.T) {
	limiter := NewRateLimiter(60, 0)
	if limiter == nil {
		t.Fatal("Expected rate limiter to be created")
	}
	// Burst should default to requests per minute
	// We can't directly test this, but we can test that it allows at least that many requests
	ctx := context.Background()
	allowed := 0
	for i := 0; i < 60; i++ {
		if limiter.Allow(ctx) {
			allowed++
		}
	}
	if allowed < 60 {
		t.Errorf("Expected at least 60 requests to be allowed, got %d", allowed)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	limiter := NewRateLimiter(60, 60) // 60 requests per minute, burst of 60
	ctx := context.Background()

	// First 60 requests should be allowed (burst)
	allowed := 0
	for i := 0; i < 60; i++ {
		if limiter.Allow(ctx) {
			allowed++
		}
	}
	if allowed != 60 {
		t.Errorf("Expected 60 requests to be allowed, got %d", allowed)
	}

	// Next request should be denied (no tokens left)
	if limiter.Allow(ctx) {
		t.Error("Expected request to be denied after burst")
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	limiter := NewRateLimiter(60, 10) // 60 requests per minute, burst of 10
	ctx := context.Background()

	// Exhaust tokens
	for i := 0; i < 10; i++ {
		limiter.Allow(ctx)
	}

	// Should be denied
	if limiter.Allow(ctx) {
		t.Error("Expected request to be denied after exhausting tokens")
	}

	// Wait for refill (1 second)
	time.Sleep(1100 * time.Millisecond)

	// Should allow at least 1 more request (1 token per second refill)
	if !limiter.Allow(ctx) {
		t.Error("Expected request to be allowed after refill")
	}
}

func TestRateLimiter_NilLimiter(t *testing.T) {
	var limiter *RateLimiter = nil
	ctx := context.Background()

	// Nil limiter should always allow
	if !limiter.Allow(ctx) {
		t.Error("Expected nil limiter to allow requests")
	}

	err := limiter.Wait(ctx)
	if err != nil {
		t.Errorf("Expected nil limiter wait to return nil, got %v", err)
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	limiter := NewRateLimiter(60, 1) // 60 requests per minute, burst of 1
	ctx := context.Background()

	// Use first token
	if !limiter.Allow(ctx) {
		t.Fatal("Expected first request to be allowed")
	}

	// Next request should be denied
	if limiter.Allow(ctx) {
		t.Error("Expected second request to be denied")
	}

	// Wait should succeed after refill
	// With 60 requests per minute, tokens refill at 1 per second
	// So we need to wait at least 1 second for a token to be available
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := limiter.Wait(waitCtx)
	if err != nil {
		t.Errorf("Expected wait to succeed, got error: %v", err)
		return
	}

	// Wait() already consumed the token, so we should be able to use it
	// The Wait() method calls Allow() internally, so it already consumed the token
	// This test is checking that Wait() succeeds and allows the request
}

func TestRateLimiter_Wait_ContextCancellation(t *testing.T) {
	limiter := NewRateLimiter(60, 1) // 60 requests per minute, burst of 1
	ctx := context.Background()

	// Use first token
	limiter.Allow(ctx)

	// Wait with cancelled context
	waitCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := limiter.Wait(waitCtx)
	if err == nil {
		t.Error("Expected wait to return error on context cancellation")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestRateLimitExceededError(t *testing.T) {
	err := &RateLimitExceededError{
		RequestsPerMinute: 60,
	}

	errorMsg := err.Error()
	if errorMsg == "" {
		t.Error("Expected error message to be non-empty")
	}
	if errorMsg != "rate limit exceeded: 60 requests per minute" {
		t.Errorf("Expected specific error message, got: %s", errorMsg)
	}
}

func TestRateLimiter_RequestsPerMinute(t *testing.T) {
	limiter := NewRateLimiter(120, 0)
	if limiter.RequestsPerMinute() != 120 {
		t.Errorf("Expected requests per minute to be 120, got %d", limiter.RequestsPerMinute())
	}

	nilLimiter := (*RateLimiter)(nil)
	if nilLimiter.RequestsPerMinute() != 0 {
		t.Errorf("Expected nil limiter to return 0, got %d", nilLimiter.RequestsPerMinute())
	}
}
