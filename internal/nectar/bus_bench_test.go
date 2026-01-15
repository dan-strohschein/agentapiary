package nectar

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// BenchmarkBus_Publish benchmarks message publishing.
func BenchmarkBus_Publish(b *testing.B) {
	logger := zap.NewNop()
	bus, err := NewBus(Config{
		Logger:     logger,
		Port:       -1, // Random port
		Persistent: false,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer bus.Close()

	ctx := context.Background()
	topic := "bench.topic"

	msg := &Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("hello world"),
		Metadata:  map[string]string{"key": "value"},
		Source:    "bench-source",
		SessionID: "session-1",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg.ID = "msg-1"
		if err := bus.Publish(ctx, topic, msg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBus_PublishSubscribe benchmarks end-to-end publish/subscribe latency.
func BenchmarkBus_PublishSubscribe(b *testing.B) {
	logger := zap.NewNop()
	bus, err := NewBus(Config{
		Logger:     logger,
		Port:       -1, // Random port
		Persistent: false,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer bus.Close()

	ctx := context.Background()
	topic := "bench.topic"

	// Subscribe
	received := make(chan *Message, b.N)
	handler := func(ctx context.Context, msg *Message) error {
		received <- msg
		return nil
	}

	sub, err := bus.Subscribe(ctx, topic, handler)
	if err != nil {
		b.Fatal(err)
	}
	defer sub.Unsubscribe()

	msg := &Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("hello world"),
		Metadata:  map[string]string{"key": "value"},
		Source:    "bench-source",
		SessionID: "session-1",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg.ID = "msg-1"
		if err := bus.Publish(ctx, topic, msg); err != nil {
			b.Fatal(err)
		}
		// Wait for message to be received
		select {
		case <-received:
		case <-time.After(1 * time.Second):
			b.Fatal("Message not received")
		}
	}
}

// BenchmarkBus_ConcurrentPublish benchmarks concurrent publishing.
func BenchmarkBus_ConcurrentPublish(b *testing.B) {
	logger := zap.NewNop()
	bus, err := NewBus(Config{
		Logger:     logger,
		Port:       -1, // Random port
		Persistent: false,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer bus.Close()

	ctx := context.Background()
	topic := "bench.topic"

	msg := &Message{
		ID:        "msg-1",
		Type:      "test",
		Payload:   []byte("hello world"),
		Metadata:  map[string]string{"key": "value"},
		Source:    "bench-source",
		SessionID: "session-1",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := bus.Publish(ctx, topic, msg); err != nil {
				b.Fatal(err)
			}
		}
	})
}
