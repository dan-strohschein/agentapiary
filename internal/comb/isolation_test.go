package comb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStore_NamespaceIsolation(t *testing.T) {
	store := NewStore(zap.NewNop())
	defer store.Close()

	ctx := context.Background()

	// Create sessions in different namespaces with same sessionID
	namespace1 := "cell1"
	namespace2 := "cell2"
	sessionID := "sess-12345"

	namespacedSessionID1 := namespace1 + ":" + sessionID
	namespacedSessionID2 := namespace2 + ":" + sessionID

	// Set data in namespace1
	err := store.Set(ctx, namespacedSessionID1, "key1", "value1", 0)
	require.NoError(t, err)

	// Set data in namespace2 with same key
	err = store.Set(ctx, namespacedSessionID2, "key1", "value2", 0)
	require.NoError(t, err)

	// Verify isolation: namespace1 should have value1
	value1, err := store.Get(ctx, namespacedSessionID1, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", value1)

	// Verify isolation: namespace2 should have value2
	value2, err := store.Get(ctx, namespacedSessionID2, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value2", value2)

	// Verify that namespace1 cannot access namespace2's data
	keys1, err := store.Keys(ctx, namespacedSessionID1, "*")
	require.NoError(t, err)
	assert.Contains(t, keys1, "key1")
	assert.Len(t, keys1, 1) // Should only have key1

	keys2, err := store.Keys(ctx, namespacedSessionID2, "*")
	require.NoError(t, err)
	assert.Contains(t, keys2, "key1")
	assert.Len(t, keys2, 1) // Should only have key1
}

func TestStore_NamespaceIsolation_Clear(t *testing.T) {
	store := NewStore(zap.NewNop())
	defer store.Close()

	ctx := context.Background()

	namespace1 := "cell1"
	namespace2 := "cell2"
	sessionID := "sess-12345"

	namespacedSessionID1 := namespace1 + ":" + sessionID
	namespacedSessionID2 := namespace2 + ":" + sessionID

	// Set data in both namespaces
	err := store.Set(ctx, namespacedSessionID1, "key1", "value1", 0)
	require.NoError(t, err)
	err = store.Set(ctx, namespacedSessionID2, "key1", "value2", 0)
	require.NoError(t, err)

	// Clear namespace1
	err = store.Clear(ctx, namespacedSessionID1)
	require.NoError(t, err)

	// Verify namespace1 is cleared
	_, err = store.Get(ctx, namespacedSessionID1, "key1")
	assert.Error(t, err)

	// Verify namespace2 still has its data
	value2, err := store.Get(ctx, namespacedSessionID2, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value2", value2)
}
