// Package guardrails provides safety guardrails for agent execution.
package guardrails

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// TokenBudgetTracker tracks token usage and enforces budgets.
type TokenBudgetTracker struct {
	// Per-request budget (0 = no limit)
	perRequestBudget int64

	// Per-session budget (0 = no limit)
	perSessionBudget int64

	// Per-minute budget (0 = no limit)
	perMinuteBudget int64

	// Current usage
	sessionTokens     int64
	minuteTokens      int64
	minuteWindow      time.Time // Start of current minute window
	lastRequestTokens int64

	mu sync.RWMutex
}

// NewTokenBudgetTracker creates a new token budget tracker.
func NewTokenBudgetTracker(perRequest, perSession, perMinute int64) *TokenBudgetTracker {
	return &TokenBudgetTracker{
		perRequestBudget: perRequest,
		perSessionBudget: perSession,
		perMinuteBudget:  perMinute,
		minuteWindow:     time.Now().Truncate(time.Minute),
	}
}

// RecordUsage records token usage for a request.
// Returns an error if the budget is exceeded.
func (t *TokenBudgetTracker) RecordUsage(ctx context.Context, tokens int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check per-request budget
	if t.perRequestBudget > 0 && tokens > t.perRequestBudget {
		return &BudgetExceededError{
			BudgetType: "per-request",
			Limit:      t.perRequestBudget,
			Used:       tokens,
		}
	}

	now := time.Now()

	// Reset minute window if needed
	currentMinute := now.Truncate(time.Minute)
	if currentMinute.After(t.minuteWindow) {
		t.minuteTokens = 0
		t.minuteWindow = currentMinute
	}

	// Check per-minute budget
	if t.perMinuteBudget > 0 {
		newMinuteTokens := t.minuteTokens + tokens
		if newMinuteTokens > t.perMinuteBudget {
			return &BudgetExceededError{
				BudgetType: "per-minute",
				Limit:      t.perMinuteBudget,
				Used:       newMinuteTokens,
			}
		}
		t.minuteTokens = newMinuteTokens
	}

	// Check per-session budget
	if t.perSessionBudget > 0 {
		newSessionTokens := t.sessionTokens + tokens
		if newSessionTokens > t.perSessionBudget {
			return &BudgetExceededError{
				BudgetType: "per-session",
				Limit:      t.perSessionBudget,
				Used:       newSessionTokens,
			}
		}
		t.sessionTokens = newSessionTokens
	}

	t.lastRequestTokens = tokens
	return nil
}

// GetUsage returns current token usage statistics.
func (t *TokenBudgetTracker) GetUsage() TokenUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TokenUsage{
		SessionTokens:     t.sessionTokens,
		MinuteTokens:      t.minuteTokens,
		LastRequestTokens: t.lastRequestTokens,
		MinuteWindowStart: t.minuteWindow,
	}
}

// ResetSession resets session-level tracking (but not minute tracking).
func (t *TokenBudgetTracker) ResetSession() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.sessionTokens = 0
}

// TokenUsage represents current token usage statistics.
type TokenUsage struct {
	SessionTokens     int64
	MinuteTokens      int64
	LastRequestTokens int64
	MinuteWindowStart time.Time
}

// BudgetExceededError indicates that a token budget has been exceeded.
type BudgetExceededError struct {
	BudgetType string
	Limit      int64
	Used       int64
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("token budget exceeded: %s limit %d, used %d", e.BudgetType, e.Limit, e.Used)
}

// ExtractTokenCount extracts token count from message metadata.
// Token count can be provided in metadata with key "token-count" or "tokens-used".
func ExtractTokenCount(msgMetadata map[string]string) int64 {
	if msgMetadata == nil {
		return 0
	}

	// Try "token-count" first
	if countStr, ok := msgMetadata["token-count"]; ok {
		if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
			return count
		}
	}

	// Try "tokens-used" as fallback
	if countStr, ok := msgMetadata["tokens-used"]; ok {
		if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
			return count
		}
	}

	return 0
}
