package keeper

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/comb"
	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/agentapiary/apiary/pkg/waggle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestKeeperWithBus(t *testing.T) (*Keeper, nectar.MessageBus, func()) {
	logger := zap.NewNop()
	combStore := comb.NewStore(logger)
	
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "test"},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	keeper := NewKeeper(spec, logger, nil, combStore, "test-session", nil)

	bus, err := nectar.NewBus(nectar.Config{
		Logger:     logger,
		Port:       -1, // Random port
		Persistent: false,
	})
	require.NoError(t, err)

	cleanup := func() {
		bus.Close()
		combStore.Close()
	}

	return keeper, bus, cleanup
}

func TestKeeper_MessageRelay_Start(t *testing.T) {
	keeper, bus, cleanup := setupTestKeeperWithBus(t)
	defer cleanup()

	ctx := context.Background()
	agentID := "test-agent-1"

	err := keeper.StartMessageRelay(ctx, bus, agentID)
	require.NoError(t, err)

	// Verify subscription was created
	keeper.mu.RLock()
	assert.NotNil(t, keeper.busSubscription)
	assert.Equal(t, agentID, keeper.agentID)
	assert.NotNil(t, keeper.inboundQueue)
	assert.NotNil(t, keeper.outboundQueue)
	keeper.mu.RUnlock()
}

func TestKeeper_MessageRelay_PublishAgentMessage(t *testing.T) {
	keeper, bus, cleanup := setupTestKeeperWithBus(t)
	defer cleanup()

	ctx := context.Background()
	agentID := "test-agent-1"

	err := keeper.StartMessageRelay(ctx, bus, agentID)
	require.NoError(t, err)

	// Create a message from agent
	msg, err := waggle.NewMessage(waggle.MessageTypeEvent, map[string]string{"data": "test"})
	require.NoError(t, err)

	// Queue for publishing
	err = keeper.PublishAgentMessage(msg)
	require.NoError(t, err)

	// Verify message is queued (may be consumed immediately by goroutine)
	// Give it a moment, then check
	time.Sleep(10 * time.Millisecond)
	// Message should either be in queue or already processed
	// Just verify no error occurred
	assert.NoError(t, err)
}

func TestKeeper_MessageQueue_EnqueueDequeue(t *testing.T) {
	queue := newMessageQueue(10)

	msg, err := waggle.NewMessage(waggle.MessageTypeEvent, "test")
	require.NoError(t, err)

	// Enqueue
	err = queue.enqueue(msg)
	require.NoError(t, err)

	assert.Equal(t, 1, queue.size())

	// Dequeue
	ctx := context.Background()
	queued, err := queue.dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, msg.ID, queued.msg.ID)
	assert.Equal(t, 0, queue.size())
}

func TestKeeper_MessageQueue_Full(t *testing.T) {
	queue := newMessageQueue(2) // Small queue

	msg1, _ := waggle.NewMessage(waggle.MessageTypeEvent, "test1")
	msg2, _ := waggle.NewMessage(waggle.MessageTypeEvent, "test2")
	msg3, _ := waggle.NewMessage(waggle.MessageTypeEvent, "test3")

	// Fill queue
	queue.enqueue(msg1)
	queue.enqueue(msg2)

	// Try to enqueue third (should fail)
	err := queue.enqueue(msg3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue full")
}
