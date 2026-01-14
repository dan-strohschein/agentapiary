package comb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Conversation(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Append messages
	msg1 := ConversationMessage{
		Type:    "user",
		Content: "Hello",
	}
	err := store.AppendConversation(ctx, sessionID, msg1)
	require.NoError(t, err)

	msg2 := ConversationMessage{
		Type:    "assistant",
		Content: "Hi there!",
	}
	err = store.AppendConversation(ctx, sessionID, msg2)
	require.NoError(t, err)

	// Get conversation
	messages, err := store.GetConversation(ctx, sessionID, 0)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, "Hello", messages[0].Content)
	assert.Equal(t, "Hi there!", messages[1].Content)

	// Get with limit
	messages, err = store.GetConversation(ctx, sessionID, 1)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, "Hi there!", messages[0].Content)
}

func TestStore_Artifacts(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Store artifact
	data := []byte("artifact data")
	err := store.StoreArtifact(ctx, sessionID, "artifact1", data)
	require.NoError(t, err)

	// Get artifact
	retrieved, err := store.GetArtifact(ctx, sessionID, "artifact1")
	require.NoError(t, err)
	assert.Equal(t, data, retrieved)

	// List artifacts
	artifacts, err := store.ListArtifacts(ctx, sessionID)
	require.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "artifact1", artifacts[0].Name)
	assert.Equal(t, int64(len(data)), artifacts[0].Size)
}

func TestStore_StateMachine(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set initial state
	err := store.SetState(ctx, sessionID, "initial")
	require.NoError(t, err)

	// Get state
	state, err := store.GetState(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "initial", state)

	// Transition state
	err = store.TransitionState(ctx, sessionID, "initial", "processing")
	require.NoError(t, err)

	state, err = store.GetState(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "processing", state)

	// Get state history
	history, err := store.GetStateHistory(ctx, sessionID)
	require.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, "initial", history[0].From)
	assert.Equal(t, "processing", history[0].To)
}

func TestStore_StateTransition_Validation(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set state
	err := store.SetState(ctx, sessionID, "state1")
	require.NoError(t, err)

	// Try invalid transition
	err = store.TransitionState(ctx, sessionID, "wrong-state", "state2")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid transition")
}

func TestStore_Scratchpad(t *testing.T) {
	store, cleanup := setupTestComb(t)
	defer cleanup()

	ctx := context.Background()
	sessionID := "test-session"

	// Set scratchpad values
	err := store.SetScratchpad(ctx, sessionID, "temp1", "value1")
	require.NoError(t, err)

	err = store.SetScratchpad(ctx, sessionID, "temp2", "value2")
	require.NoError(t, err)

	// Get scratchpad value
	val, err := store.GetScratchpad(ctx, sessionID, "temp1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val)

	// Clear scratchpad
	err = store.ClearScratchpad(ctx, sessionID)
	require.NoError(t, err)

	// Values should be gone
	_, err = store.GetScratchpad(ctx, sessionID, "temp1")
	assert.Error(t, err)
}
