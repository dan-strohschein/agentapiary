package queen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewLeaderElector_NoEndpoints(t *testing.T) {
	logger := zap.NewNop()
	elector := NewLeaderElector(nil, "", 0, logger)
	
	// Should return noOpLeaderElector which always considers itself leader
	assert.True(t, elector.IsLeader())
	
	err := elector.Start(context.Background())
	assert.NoError(t, err)
	
	err = elector.Stop(context.Background())
	assert.NoError(t, err)
}

func TestNewLeaderElector_WithEndpoints(t *testing.T) {
	logger := zap.NewNop()
	endpoints := []string{"http://etcd1:2379", "http://etcd2:2379"}
	elector := NewLeaderElector(endpoints, "/apiary/leader", 30, logger)
	
	// Stub implementation should not be leader initially
	assert.False(t, elector.IsLeader())
	
	// Start should fail (stub implementation)
	err := elector.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestNewLeaderElector_DefaultKeyAndTTL(t *testing.T) {
	logger := zap.NewNop()
	endpoints := []string{"http://etcd1:2379"}
	
	// Test with empty key and zero TTL - should use defaults
	elector := NewLeaderElector(endpoints, "", 0, logger)
	
	// Should create leaderElector (not noOp)
	assert.False(t, elector.IsLeader())
}

func TestLeaderElector_IsLeader(t *testing.T) {
	logger := zap.NewNop()
	endpoints := []string{"http://etcd1:2379"}
	elector := NewLeaderElector(endpoints, "/test/leader", 30, logger)
	
	// Initially not leader
	assert.False(t, elector.IsLeader())
}

func TestLeaderElector_OnLeaderElected(t *testing.T) {
	logger := zap.NewNop()
	endpoints := []string{"http://etcd1:2379"}
	elector := NewLeaderElector(endpoints, "/test/leader", 30, logger)
	
	// Set callback (should not panic)
	called := false
	elector.OnLeaderElected(func(ctx context.Context) error {
		called = true
		return nil
	})
	
	// Callback not called yet (stub implementation)
	assert.False(t, called)
}

func TestLeaderElector_OnLeaderLost(t *testing.T) {
	logger := zap.NewNop()
	endpoints := []string{"http://etcd1:2379"}
	elector := NewLeaderElector(endpoints, "/test/leader", 30, logger)
	
	// Set callback (should not panic)
	called := false
	elector.OnLeaderLost(func(ctx context.Context) error {
		called = true
		return nil
	})
	
	// Callback not called yet (stub implementation)
	assert.False(t, called)
}

func TestNoOpLeaderElector(t *testing.T) {
	logger := zap.NewNop()
	elector := NewLeaderElector(nil, "", 0, logger)
	
	require.IsType(t, &noOpLeaderElector{}, elector)
	
	noOp := elector.(*noOpLeaderElector)
	
	// Should always be leader
	assert.True(t, noOp.IsLeader())
	
	// Start/Stop should succeed
	err := noOp.Start(context.Background())
	assert.NoError(t, err)
	
	err = noOp.Stop(context.Background())
	assert.NoError(t, err)
	
	// Callbacks should not panic (no-ops)
	noOp.OnLeaderElected(func(ctx context.Context) error { return nil })
	noOp.OnLeaderLost(func(ctx context.Context) error { return nil })
}
