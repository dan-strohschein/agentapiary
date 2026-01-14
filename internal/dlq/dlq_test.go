package dlq

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewManager(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	require.NotNil(t, mgr)
	assert.NotNil(t, mgr.queues)
	assert.NotNil(t, mgr.logger)
}

func TestManager_AddMessage(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	msg := &nectar.Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}

	err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Verify message was added
	messages, err := mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0].ID, "msg-1") // DLQ ID includes timestamp
	assert.Equal(t, "msg-1", messages[0].OriginalMsg.ID) // Original message ID
	assert.Equal(t, "test failure", messages[0].FailureReason)
	assert.Equal(t, 3, messages[0].Attempts)
}

func TestManager_AddMessage_MultipleMessages(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	// Add multiple messages
	for i := 0; i < 5; i++ {
		msg := &nectar.Message{
			ID:        "msg-" + string(rune('0'+i)),
			Type:      "test",
			Payload:   []byte("test payload"),
			Timestamp: time.Now(),
		}
		err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", i+1)
		require.NoError(t, err)
	}

	// Verify all messages were added
	messages, err := mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Len(t, messages, 5)
}

func TestManager_AddMessage_DifferentHives(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"

	// Add messages to different hives
	msg1 := &nectar.Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := mgr.AddMessage(ctx, namespace, "hive-1", msg1, "failure 1", 1)
	require.NoError(t, err)

	msg2 := &nectar.Message{
		ID:        "msg-2",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err = mgr.AddMessage(ctx, namespace, "hive-2", msg2, "failure 2", 2)
	require.NoError(t, err)

	// Verify messages are isolated per hive
	messages1, err := mgr.ListMessages(ctx, namespace, "hive-1")
	require.NoError(t, err)
	assert.Len(t, messages1, 1)
	assert.Equal(t, "msg-1", messages1[0].OriginalMsg.ID)

	messages2, err := mgr.ListMessages(ctx, namespace, "hive-2")
	require.NoError(t, err)
	assert.Len(t, messages2, 1)
	assert.Equal(t, "msg-2", messages2[0].OriginalMsg.ID)
}

func TestManager_ListMessages_EmptyQueue(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	messages, err := mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestManager_GetMessage(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	msg := &nectar.Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}

	err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Get the message - first list to get the DLQ message ID
	messages, err := mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	dlqMsgID := messages[0].ID

	// Get the message using the DLQ message ID
	dlqMsg, err := mgr.GetMessage(ctx, namespace, hiveName, dlqMsgID)
	require.NoError(t, err)
	assert.NotNil(t, dlqMsg)
	assert.Equal(t, dlqMsgID, dlqMsg.ID)
	assert.Equal(t, "msg-1", dlqMsg.OriginalMsg.ID)
	assert.Equal(t, "test failure", dlqMsg.FailureReason)
	assert.Equal(t, 3, dlqMsg.Attempts)
}

func TestManager_GetMessage_NotFound(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	// Add a message first to create the queue
	msg := &nectar.Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Try to get a message that doesn't exist
	_, err = mgr.GetMessage(ctx, namespace, hiveName, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message not found")
}

func TestManager_GetMessage_QueueNotFound(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "nonexistent-hive"

	// Try to get a message from a queue that doesn't exist
	_, err := mgr.GetMessage(ctx, namespace, hiveName, "msg-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DLQ not found")
}

func TestManager_DeleteMessage(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	// Add a message
	msg := &nectar.Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Verify message exists
	messages, err := mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	dlqMsgID := messages[0].ID

	// Delete the message using the DLQ message ID
	err = mgr.DeleteMessage(ctx, namespace, hiveName, dlqMsgID)
	require.NoError(t, err)

	// Verify message was deleted
	messages, err = mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestManager_DeleteMessage_NotFound(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	// Add a message
	msg := &nectar.Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Try to delete a message that doesn't exist
	err = mgr.DeleteMessage(ctx, namespace, hiveName, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message not found")

	// Verify original message still exists
	messages, err := mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
}

func TestManager_ClearMessages(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	// Add multiple messages
	for i := 0; i < 5; i++ {
		msg := &nectar.Message{
			ID:        "msg-" + string(rune('0'+i)),
			Type:      "test",
			Payload:   []byte("test payload"),
			Timestamp: time.Now(),
		}
		err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", i+1)
		require.NoError(t, err)
	}

	// Verify messages exist
	messages, err := mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Len(t, messages, 5)

	// Clear all messages
	err = mgr.ClearMessages(ctx, namespace, hiveName)
	require.NoError(t, err)

	// Verify all messages were cleared
	messages, err = mgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestManager_ClearMessages_EmptyQueue(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	// Clear an empty queue (should not error)
	err := mgr.ClearMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
}

func TestManager_ClearMessages_Isolated(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"

	// Add messages to different hives
	msg1 := &nectar.Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := mgr.AddMessage(ctx, namespace, "hive-1", msg1, "failure 1", 1)
	require.NoError(t, err)

	msg2 := &nectar.Message{
		ID:        "msg-2",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err = mgr.AddMessage(ctx, namespace, "hive-2", msg2, "failure 2", 2)
	require.NoError(t, err)

	// Clear only one hive
	err = mgr.ClearMessages(ctx, namespace, "hive-1")
	require.NoError(t, err)

	// Verify hive-1 is cleared but hive-2 still has messages
	messages1, err := mgr.ListMessages(ctx, namespace, "hive-1")
	require.NoError(t, err)
	assert.Empty(t, messages1)

	messages2, err := mgr.ListMessages(ctx, namespace, "hive-2")
	require.NoError(t, err)
	assert.Len(t, messages2, 1)
	assert.Equal(t, "msg-2", messages2[0].OriginalMsg.ID)
}

func TestManager_GetQueueSize(t *testing.T) {
	logger := zap.NewNop()
	mgr := NewManager(logger)

	ctx := context.Background()
	namespace := "test-ns"
	hiveName := "test-hive"

	// Empty queue
	size := mgr.GetQueueSize(namespace, hiveName)
	assert.Equal(t, 0, size)

	// Add messages
	for i := 0; i < 3; i++ {
		msg := &nectar.Message{
			ID:        "msg-" + string(rune('0'+i)),
			Type:      "test",
			Payload:   []byte("test payload"),
			Timestamp: time.Now(),
		}
		err := mgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", i+1)
		require.NoError(t, err)
	}

	// Check queue size
	size = mgr.GetQueueSize(namespace, hiveName)
	assert.Equal(t, 3, size)
}

func TestGetDLQTopic(t *testing.T) {
	namespace := "test-ns"
	hiveName := "test-hive"

	// Test with default topic
	topic := GetDLQTopic(namespace, hiveName, "")
	assert.Equal(t, "dlq.test-ns.test-hive", topic)

	// Test with configured topic
	configuredTopic := "custom.dlq.topic"
	topic = GetDLQTopic(namespace, hiveName, configuredTopic)
	assert.Equal(t, configuredTopic, topic)
}
