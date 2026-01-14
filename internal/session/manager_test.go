package session

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestManager(t *testing.T) (*Manager, apiary.ResourceStore, func()) {
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	logger := zap.NewNop()

	manager := NewManager(Config{
		Store:  store,
		Logger: logger,
	})

	ctx := context.Background()
	err = manager.Start(ctx)
	require.NoError(t, err)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Stop(ctx)
		store.Close()
	}

	return manager, store, cleanup
}

func TestManager_CreateSession(t *testing.T) {
	manager, _, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	config := SessionConfig{
		Namespace:          "default",
		TimeoutMinutes:     30,
		MaxDurationMinutes: 120,
		PersistOnTerminate: false,
		MaxMemoryMB:        256,
	}

	session, err := manager.CreateSession(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.NotEmpty(t, session.GetUID())
	assert.Equal(t, apiary.SessionPhaseCreated, session.Status.Phase)
	assert.Equal(t, "default", session.GetNamespace())
}

func TestManager_GetSession(t *testing.T) {
	manager, _, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	config := SessionConfig{
		Namespace: "default",
	}

	session, err := manager.CreateSession(ctx, config)
	require.NoError(t, err)

	// Get the session
	retrieved, err := manager.GetSession(ctx, session.GetUID())
	require.NoError(t, err)
	assert.Equal(t, session.GetUID(), retrieved.GetUID())
	assert.Equal(t, session.GetNamespace(), retrieved.GetNamespace())
}

func TestManager_UpdateActivity(t *testing.T) {
	manager, _, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	config := SessionConfig{
		Namespace: "default",
	}

	session, err := manager.CreateSession(ctx, config)
	require.NoError(t, err)

	// Update activity
	err = manager.UpdateActivity(ctx, session.GetUID())
	require.NoError(t, err)

	// Verify phase changed to active
	retrieved, err := manager.GetSession(ctx, session.GetUID())
	require.NoError(t, err)
	assert.Equal(t, apiary.SessionPhaseActive, retrieved.Status.Phase)
}

func TestManager_TerminateSession(t *testing.T) {
	manager, store, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	config := SessionConfig{
		Namespace: "default",
	}

	session, err := manager.CreateSession(ctx, config)
	require.NoError(t, err)

	// Terminate session
	err = manager.TerminateSession(ctx, session.GetUID())
	require.NoError(t, err)

	// Verify session is terminated - check in store directly since it's removed from cache
	resource, err := store.Get(ctx, "Session", session.GetName(), session.GetNamespace())
	require.NoError(t, err)
	retrieved, ok := resource.(*apiary.Session)
	require.True(t, ok)
	assert.Equal(t, apiary.SessionPhaseTerminated, retrieved.Status.Phase)
}

func TestManager_Timeout(t *testing.T) {
	manager, store, cleanup := setupTestManager(t)
	defer cleanup()

	ctx := context.Background()

	config := SessionConfig{
		Namespace:      "default",
		TimeoutMinutes: 1, // 1 minute timeout for testing
	}

	session, err := manager.CreateSession(ctx, config)
	require.NoError(t, err)

	// Wait for timeout check (runs every 30 seconds)
	// For testing, we'll manually trigger timeout by setting last activity far in past
	manager.mu.Lock()
	if state, exists := manager.sessions[session.GetUID()]; exists {
		state.LastActivity = time.Now().Add(-2 * time.Minute)
	}
	manager.mu.Unlock()

	// Trigger timeout check
	manager.checkTimeouts(ctx)

	// Verify session is terminated - check in store directly
	resource, err := store.Get(ctx, "Session", session.GetName(), session.GetNamespace())
	require.NoError(t, err)
	retrieved, ok := resource.(*apiary.Session)
	require.True(t, ok)
	assert.Equal(t, apiary.SessionPhaseTerminated, retrieved.Status.Phase)
}
