package nectar

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestBus(t *testing.T) (*Bus, func()) {
	logger := zap.NewNop()
	bus, err := NewBus(Config{
		Logger:     logger,
		Port:       -1, // Random port
		Persistent: false,
	})
	require.NoError(t, err)

	cleanup := func() {
		bus.Close()
	}

	return bus, cleanup
}

func TestBus_PublishSubscribe(t *testing.T) {
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	ctx := context.Background()
	topic := "test.topic"

	// Subscribe
	received := make(chan *Message, 1)
	handler := func(ctx context.Context, msg *Message) error {
		received <- msg
		return nil
	}

	sub, err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)
	defer sub.Unsubscribe()

	// Publish
	msg := &Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("hello"),
		Metadata:  map[string]string{"key": "value"},
		Source:    "test-source",
		SessionID: "session-1",
	}

	err = bus.Publish(ctx, topic, msg)
	require.NoError(t, err)

	// Wait for message
	select {
	case receivedMsg := <-received:
		assert.Equal(t, msg.ID, receivedMsg.ID)
		assert.Equal(t, msg.Type, receivedMsg.Type)
		assert.Equal(t, msg.Payload, receivedMsg.Payload)
		assert.Equal(t, msg.Source, receivedMsg.Source)
	case <-time.After(2 * time.Second):
		t.Fatal("Message not received")
	}
}

func TestBus_QueueSubscribe(t *testing.T) {
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	ctx := context.Background()
	topic := "test.queue"
	queue := "workers"

	// Subscribe with queue group
	received1 := make(chan *Message, 1)
	handler1 := func(ctx context.Context, msg *Message) error {
		received1 <- msg
		return nil
	}

	sub1, err := bus.QueueSubscribe(ctx, topic, queue, handler1)
	require.NoError(t, err)
	defer sub1.Unsubscribe()

	// Subscribe with same queue group (load balancing)
	received2 := make(chan *Message, 1)
	handler2 := func(ctx context.Context, msg *Message) error {
		received2 <- msg
		return nil
	}

	sub2, err := bus.QueueSubscribe(ctx, topic, queue, handler2)
	require.NoError(t, err)
	defer sub2.Unsubscribe()

	// Publish multiple messages
	for i := 0; i < 10; i++ {
		msg := &Message{
			ID:      fmt.Sprintf("msg-%d", i),
			Type:    "test",
			Payload: []byte(fmt.Sprintf("message-%d", i)),
		}
		err := bus.Publish(ctx, topic, msg)
		require.NoError(t, err)
	}

	// Wait a bit for messages to be distributed
	time.Sleep(100 * time.Millisecond)

	// Messages should be distributed between the two subscribers
	// We can't guarantee exact distribution, but both should receive some
	receivedCount := len(received1) + len(received2)
	assert.Greater(t, receivedCount, 0)
	assert.LessOrEqual(t, receivedCount, 10)
}

func TestBus_MultipleTopics(t *testing.T) {
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	ctx := context.Background()

	// Subscribe to multiple topics
	received1 := make(chan *Message, 1)
	sub1, err := bus.Subscribe(ctx, "topic.1", func(ctx context.Context, msg *Message) error {
		received1 <- msg
		return nil
	})
	require.NoError(t, err)
	defer sub1.Unsubscribe()

	received2 := make(chan *Message, 1)
	sub2, err := bus.Subscribe(ctx, "topic.2", func(ctx context.Context, msg *Message) error {
		received2 <- msg
		return nil
	})
	require.NoError(t, err)
	defer sub2.Unsubscribe()

	// Publish to topic 1
	msg1 := &Message{ID: "msg-1", Type: "test", Payload: []byte("topic1")}
	err = bus.Publish(ctx, "topic.1", msg1)
	require.NoError(t, err)

	// Publish to topic 2
	msg2 := &Message{ID: "msg-2", Type: "test", Payload: []byte("topic2")}
	err = bus.Publish(ctx, "topic.2", msg2)
	require.NoError(t, err)

	// Verify messages received on correct topics
	select {
	case m := <-received1:
		assert.Equal(t, "msg-1", m.ID)
		assert.Equal(t, []byte("topic1"), m.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("Message not received on topic.1")
	}

	select {
	case m := <-received2:
		assert.Equal(t, "msg-2", m.ID)
		assert.Equal(t, []byte("topic2"), m.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("Message not received on topic.2")
	}
}
