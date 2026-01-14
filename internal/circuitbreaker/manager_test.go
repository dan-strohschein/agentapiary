package circuitbreaker

import (
	"context"
	"testing"
)

func TestManager_GetOrCreate(t *testing.T) {
	m := NewManager(Config{})

	droneID := "test-drone"
	cb1 := m.GetOrCreate(droneID)
	cb2 := m.GetOrCreate(droneID)

	if cb1 != cb2 {
		t.Error("Expected GetOrCreate to return the same circuit breaker instance")
	}
}

func TestManager_Get(t *testing.T) {
	m := NewManager(Config{})

	droneID := "test-drone"

	// Get non-existent breaker should return nil
	if m.Get(droneID) != nil {
		t.Error("Expected Get to return nil for non-existent breaker")
	}

	// Create breaker
	m.GetOrCreate(droneID)

	// Get should return the breaker
	if m.Get(droneID) == nil {
		t.Error("Expected Get to return breaker after creation")
	}
}

func TestManager_RecordFailureAndSuccess(t *testing.T) {
	m := NewManager(Config{
		FailureThreshold: 2,
	})

	ctx := context.Background()
	droneID := "test-drone"

	// Record failures
	m.RecordFailure(ctx, droneID)
	m.RecordFailure(ctx, droneID)

	// Circuit should be open
	if !m.IsOpen(droneID) {
		t.Error("Expected circuit to be open after threshold failures")
	}

	// Record success
	m.RecordSuccess(ctx, droneID)

	// Circuit should still be open (needs half-open transition)
	if !m.IsOpen(droneID) {
		t.Error("Expected circuit to remain open until half-open transition")
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager(Config{})

	droneID := "test-drone"
	m.GetOrCreate(droneID)

	if m.Get(droneID) == nil {
		t.Fatal("Expected breaker to exist before removal")
	}

	m.Remove(droneID)

	if m.Get(droneID) != nil {
		t.Error("Expected breaker to be removed")
	}
}

func TestManager_IsClosed(t *testing.T) {
	m := NewManager(Config{})

	droneID := "test-drone"

	// Non-existent breaker should be considered closed
	if !m.IsClosed(droneID) {
		t.Error("Expected non-existent breaker to be considered closed")
	}

	// Created breaker should be closed
	m.GetOrCreate(droneID)
	if !m.IsClosed(droneID) {
		t.Error("Expected new breaker to be closed")
	}
}
