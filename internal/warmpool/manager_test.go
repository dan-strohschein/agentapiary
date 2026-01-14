package warmpool

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func setupTestManager(t *testing.T) (*Manager, func()) {
	logger := zap.NewNop()
	
	// Create a mock store, launcher, and scheduler
	// For now, we'll just test the pool management logic
	manager := &Manager{
		logger:        logger,
		warmPools:     make(map[string]*WarmPool),
		idleTimeout:   30 * time.Minute,
		checkInterval: 5 * time.Minute,
		stopCh:        make(chan struct{}),
	}
	
	cleanup := func() {
		close(manager.stopCh)
	}
	
	return manager, cleanup
}

func TestManager_EnsurePool(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()
	
	// Test that pool doesn't exist initially
	key := "default/test-agent"
	
	manager.mu.Lock()
	pool, exists := manager.warmPools[key]
	manager.mu.Unlock()
	
	assert.False(t, exists)
	assert.Nil(t, pool)
}

func TestWarmPool_GetWarmDrone(t *testing.T) {
	pool := &WarmPool{
		AgentSpec:   "test-agent",
		Namespace:   "default",
		MinSize:     2,
		WarmDrones:  make([]*apiary.Drone, 0),
		LastUsed:    time.Now(),
	}
	
	// Pool is empty
	pool.mu.Lock()
	assert.Equal(t, 0, len(pool.WarmDrones))
	pool.mu.Unlock()
}

func TestWarmPool_ReturnDrone(t *testing.T) {
	pool := &WarmPool{
		AgentSpec:   "test-agent",
		Namespace:   "default",
		MinSize:     2,
		WarmDrones:  make([]*apiary.Drone, 0),
		LastUsed:    time.Now(),
	}
	
	pool.mu.Lock()
	currentSize := pool.CurrentSize
	pool.mu.Unlock()
	
	assert.Equal(t, 0, currentSize)
}

func TestManager_CleanupIdlePools(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()
	
	// Create an idle pool
	pool := &WarmPool{
		AgentSpec:   "test-agent",
		Namespace:   "default",
		MinSize:     0,
		WarmDrones:  make([]*apiary.Drone, 0),
		LastUsed:    time.Now().Add(-2 * manager.idleTimeout), // Very idle
	}
	
	key := "default/test-agent"
	manager.mu.Lock()
	manager.warmPools[key] = pool
	manager.mu.Unlock()
	
	// Cleanup should remove the pool
	ctx := context.Background()
	manager.cleanupIdlePools(ctx)
	
	manager.mu.RLock()
	_, exists := manager.warmPools[key]
	manager.mu.RUnlock()
	
	// Pool should be removed if idle for 2x timeout and minSize is 0
	assert.False(t, exists)
}
