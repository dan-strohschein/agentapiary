package comb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestComb(t *testing.T) (*Store, func()) {
	logger := zap.NewNop()
	store := NewStore(logger)

	cleanup := func() {
		store.Close()
	}

	return store, cleanup
}

func TestStore_GetSet(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set a value
	err := store.Set(ctx, sessionID, "key1", "value1", 0)
	require.NoError(t, err)

	// Get the value
	value, err := store.Get(ctx, sessionID, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", value)

	// Get non-existent key
	_, err = store.Get(ctx, sessionID, "nonexistent")
	assert.Error(t, err)
}

func TestStore_Delete(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set a value
	err := store.Set(ctx, sessionID, "key1", "value1", 0)
	require.NoError(t, err)

	// Delete the key
	err = store.Delete(ctx, sessionID, "key1")
	require.NoError(t, err)

	// Get should fail
	_, err = store.Get(ctx, sessionID, "key1")
	assert.Error(t, err)
}

func TestStore_Exists(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Key doesn't exist
	exists, err := store.Exists(ctx, sessionID, "key1")
	require.NoError(t, err)
	assert.False(t, exists)

	// Set a value
	err = store.Set(ctx, sessionID, "key1", "value1", 0)
	require.NoError(t, err)

	// Key exists
	exists, err = store.Exists(ctx, sessionID, "key1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestStore_Incr(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Increment non-existent key (starts at 0)
	value, err := store.Incr(ctx, sessionID, "counter")
	require.NoError(t, err)
	assert.Equal(t, int64(1), value)

	// Increment again
	value, err = store.Incr(ctx, sessionID, "counter")
	require.NoError(t, err)
	assert.Equal(t, int64(2), value)

	// Verify value
	val, err := store.Get(ctx, sessionID, "counter")
	require.NoError(t, err)
	assert.Equal(t, "2", val)
}

func TestStore_Keys(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set multiple keys
	store.Set(ctx, sessionID, "key1", "value1", 0)
	store.Set(ctx, sessionID, "key2", "value2", 0)
	store.Set(ctx, sessionID, "key3", "value3", 0)

	// Get all keys
	keys, err := store.Keys(ctx, sessionID, "*")
	require.NoError(t, err)
	assert.Len(t, keys, 3)
	assert.Contains(t, keys, "key1")
	assert.Contains(t, keys, "key2")
	assert.Contains(t, keys, "key3")
}

func TestStore_TTL(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set a value with short TTL
	err := store.Set(ctx, sessionID, "key1", "value1", 100*time.Millisecond)
	require.NoError(t, err)

	// Get should work immediately
	value, err := store.Get(ctx, sessionID, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", value)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Get should fail (expired)
	_, err = store.Get(ctx, sessionID, "key1")
	assert.Error(t, err)
}

func TestStore_SessionIsolation(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	session1 := "session-1"
	session2 := "session-2"

	// Set same key in different sessions
	store.Set(ctx, session1, "key1", "value1", 0)
	store.Set(ctx, session2, "key1", "value2", 0)

	// Get should return different values
	val1, err := store.Get(ctx, session1, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val1)

	val2, err := store.Get(ctx, session2, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value2", val2)
}

func TestStore_MemoryLimit(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set memory limit (1 MB)
	store.SetMemoryLimit(sessionID, 1)

	// Set a value that exceeds limit
	largeValue := make([]byte, 2*1024*1024) // 2 MB
	err := store.Set(ctx, sessionID, "large", string(largeValue), 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "memory limit exceeded")
}

func TestStore_Clear(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set multiple keys
	store.Set(ctx, sessionID, "key1", "value1", 0)
	store.Set(ctx, sessionID, "key2", "value2", 0)

	// Clear session
	err := store.Clear(ctx, sessionID)
	require.NoError(t, err)

	// Keys should be gone
	keys, err := store.Keys(ctx, sessionID, "*")
	require.NoError(t, err)
	assert.Len(t, keys, 0)
}

func BenchmarkStore_Set(b *testing.B) {
	store, cleanup := setupTestComb(&testing.T{})
	defer cleanup()

	ctx := context.Background()
	sessionID := "bench-session"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.Set(ctx, sessionID, fmt.Sprintf("key-%d", i), "value", 0)
	}
}

func BenchmarkStore_Get(b *testing.B) {
	store, cleanup := setupTestComb(&testing.T{})
	defer cleanup()

	ctx := context.Background()
	sessionID := "bench-session"

	store.Set(ctx, sessionID, "key", "value", 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(ctx, sessionID, "key")
	}
}
