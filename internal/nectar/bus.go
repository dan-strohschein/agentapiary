// Package nectar provides the message bus (Nectar) for agent communication.
package nectar

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// subjectForTopic constructs a NATS subject from a topic.
// Optimized: Uses strings.Builder to avoid fmt.Sprintf overhead.
func subjectForTopic(topic string) string {
	var b strings.Builder
	b.Grow(len(topic) + 7) // "nectar." + topic
	b.WriteString("nectar.")
	b.WriteString(topic)
	return b.String()
}

// MessageBus provides message bus functionality for agent communication.
type MessageBus interface {
	// Publish publishes a message to a topic.
	Publish(ctx context.Context, topic string, msg *Message) error
	
	// Subscribe subscribes to a topic with a handler.
	Subscribe(ctx context.Context, topic string, handler MessageHandler) (Subscription, error)
	
	// QueueSubscribe subscribes to a topic with queue group for load balancing.
	QueueSubscribe(ctx context.Context, topic, queue string, handler MessageHandler) (Subscription, error)
	
	// Close closes the message bus.
	Close() error
}

// Message represents a message on the bus.
type Message struct {
	ID        string
	Type      string
	Payload   []byte
	Metadata  map[string]string
	Timestamp time.Time
	Source    string
	SessionID string
}

// MessageHandler handles incoming messages.
type MessageHandler func(ctx context.Context, msg *Message) error

// Subscription represents a message subscription.
type Subscription interface {
	// Unsubscribe unsubscribes from the topic.
	Unsubscribe() error
}

// Bus implements MessageBus using embedded NATS.
type Bus struct {
	server     *server.Server
	conn       *nats.Conn
	logger     *zap.Logger
	subs       map[string]*nats.Subscription
	mu         sync.RWMutex
	persistent bool
	dataDir    string
}

// Config holds message bus configuration.
type Config struct {
	Logger     *zap.Logger
	Port       int
	Persistent bool
	DataDir    string
	// External NATS URLs (comma-separated). If provided, connects to external NATS cluster instead of starting embedded server.
	// Example: "nats://nats1:4222,nats://nats2:4222,nats://nats3:4222"
	ExternalURLs string
}

// NewBus creates a new message bus. If ExternalURLs is provided, connects to external NATS cluster.
// Otherwise, starts an embedded NATS server (default behavior).
func NewBus(cfg Config) (*Bus, error) {
	var (
		conn       *nats.Conn
		natsServer *server.Server
		err        error
	)

	// Check if external NATS URLs are provided
	if cfg.ExternalURLs != "" {
		// Connect to external NATS cluster
		conn, err = nats.Connect(cfg.ExternalURLs, nats.Name("apiary-nectar"))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to external NATS cluster: %w", err)
		}
	} else {
		// Start embedded NATS server (default behavior)
		opts := &server.Options{
			Host:       "127.0.0.1",
			Port:       cfg.Port,
			JetStream:  cfg.Persistent,
			StoreDir:   cfg.DataDir,
			MaxPayload: 10 * 1024 * 1024, // 10MB max message size
		}

		// Create and start NATS server
		natsServer, err = server.NewServer(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create NATS server: %w", err)
		}

		// Start server in goroutine
		go natsServer.Start()

		// Wait for server to be ready
		if !natsServer.ReadyForConnections(10 * time.Second) {
			natsServer.Shutdown()
			return nil, fmt.Errorf("NATS server failed to start")
		}

		// Connect to the embedded server
		conn, err = nats.Connect(natsServer.ClientURL())
		if err != nil {
			natsServer.Shutdown()
			return nil, fmt.Errorf("failed to connect to NATS server: %w", err)
		}
	}

	bus := &Bus{
		server:     natsServer,
		conn:       conn,
		logger:     cfg.Logger,
		subs:       make(map[string]*nats.Subscription),
		persistent: cfg.Persistent,
		dataDir:    cfg.DataDir,
	}

	if cfg.Persistent {
		// Set up JetStream for persistence
		js, err := conn.JetStream()
		if err != nil {
			conn.Close()
			if natsServer != nil {
				natsServer.Shutdown()
			}
			return nil, fmt.Errorf("failed to create JetStream context: %w", err)
		}

		// Create stream for persistent messages (only for embedded server, external cluster should have stream already)
		if natsServer != nil {
			_, err = js.AddStream(&nats.StreamConfig{
				Name:     "NECTAR",
				Subjects: []string{"nectar.>"},
				Storage:  nats.FileStorage,
			})
			if err != nil && !strings.Contains(err.Error(), "stream name already in use") {
				conn.Close()
				natsServer.Shutdown()
				return nil, fmt.Errorf("failed to create JetStream stream: %w", err)
			}
		}
	}

	return bus, nil
}

// Publish publishes a message to a topic.
func (b *Bus) Publish(ctx context.Context, topic string, msg *Message) error {
	// Encode message
	data, err := encodeMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	// Use JetStream if persistent, otherwise regular NATS
	if b.persistent {
		js, err := b.conn.JetStream()
		if err != nil {
			return fmt.Errorf("failed to get JetStream context: %w", err)
		}

		subject := subjectForTopic(topic)
		_, err = js.Publish(subject, data)
		return err
	}

	subject := subjectForTopic(topic)
	return b.conn.Publish(subject, data)
}

// Subscribe subscribes to a topic with a handler.
func (b *Bus) Subscribe(ctx context.Context, topic string, handler MessageHandler) (Subscription, error) {
	subject := fmt.Sprintf("nectar.%s", topic)

	var sub *nats.Subscription
	var err error

	if b.persistent {
		// Use JetStream for persistent subscriptions
		js, err := b.conn.JetStream()
		if err != nil {
			return nil, fmt.Errorf("failed to get JetStream context: %w", err)
		}

		sub, err = js.Subscribe(subject, func(natsMsg *nats.Msg) {
			msg, decodeErr := decodeMessage(natsMsg.Data)
			if decodeErr != nil {
				b.logger.Error("Failed to decode message",
					zap.String("topic", topic),
					zap.Error(decodeErr),
				)
				return
			}

			if handleErr := handler(ctx, msg); handleErr != nil {
				b.logger.Error("Message handler failed",
					zap.String("topic", topic),
					zap.Error(handleErr),
				)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("failed to subscribe: %w", err)
		}
	} else {
		// Regular NATS subscription
		sub, err = b.conn.Subscribe(subject, func(natsMsg *nats.Msg) {
			msg, decodeErr := decodeMessage(natsMsg.Data)
			if decodeErr != nil {
				b.logger.Error("Failed to decode message",
					zap.String("topic", topic),
					zap.Error(decodeErr),
				)
				return
			}

			if handleErr := handler(ctx, msg); handleErr != nil {
				b.logger.Error("Message handler failed",
					zap.String("topic", topic),
					zap.Error(handleErr),
				)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("failed to subscribe: %w", err)
		}
	}

	// Track subscription
	b.mu.Lock()
	b.subs[topic] = sub
	b.mu.Unlock()

	return &subscription{sub: sub}, nil
}

// QueueSubscribe subscribes to a topic with queue group for load balancing.
func (b *Bus) QueueSubscribe(ctx context.Context, topic, queue string, handler MessageHandler) (Subscription, error) {
	subject := subjectForTopic(topic)

	var sub *nats.Subscription
	var err error

	if b.persistent {
		// Use JetStream for persistent queue subscriptions
		js, err := b.conn.JetStream()
		if err != nil {
			return nil, fmt.Errorf("failed to get JetStream context: %w", err)
		}

		sub, err = js.QueueSubscribe(subject, queue, func(natsMsg *nats.Msg) {
			msg, decodeErr := decodeMessage(natsMsg.Data)
			if decodeErr != nil {
				b.logger.Error("Failed to decode message",
					zap.String("topic", topic),
					zap.String("queue", queue),
					zap.Error(decodeErr),
				)
				return
			}

			if handleErr := handler(ctx, msg); handleErr != nil {
				b.logger.Error("Message handler failed",
					zap.String("topic", topic),
					zap.String("queue", queue),
					zap.Error(handleErr),
				)
				// For queue subscriptions, we might want to nack on error
				_ = natsMsg.Nak()
			} else {
				_ = natsMsg.Ack()
			}
		})
		if err != nil {
			return nil, fmt.Errorf("failed to queue subscribe: %w", err)
		}
	} else {
		// Regular NATS queue subscription
		sub, err = b.conn.QueueSubscribe(subject, queue, func(natsMsg *nats.Msg) {
			msg, decodeErr := decodeMessage(natsMsg.Data)
			if decodeErr != nil {
				b.logger.Error("Failed to decode message",
					zap.String("topic", topic),
					zap.String("queue", queue),
					zap.Error(decodeErr),
				)
				return
			}

			if handleErr := handler(ctx, msg); handleErr != nil {
				b.logger.Error("Message handler failed",
					zap.String("topic", topic),
					zap.String("queue", queue),
					zap.Error(handleErr),
				)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("failed to queue subscribe: %w", err)
		}
	}

	// Track subscription - optimized: use strings.Builder instead of fmt.Sprintf
	b.mu.Lock()
	var key strings.Builder
	key.Grow(len(topic) + len(queue) + 1)
	key.WriteString(topic)
	key.WriteByte(':')
	key.WriteString(queue)
	b.subs[key.String()] = sub
	b.mu.Unlock()

	return &subscription{sub: sub}, nil
}

// Close closes the message bus.
func (b *Bus) Close() error {
	// Unsubscribe all
	b.mu.Lock()
	for _, sub := range b.subs {
		_ = sub.Unsubscribe()
	}
	b.subs = make(map[string]*nats.Subscription)
	b.mu.Unlock()

	// Close connection
	if b.conn != nil {
		b.conn.Close()
	}

	// Shutdown server
	if b.server != nil {
		b.server.Shutdown()
	}

	return nil
}

// subscription implements Subscription.
type subscription struct {
	sub *nats.Subscription
}

func (s *subscription) Unsubscribe() error {
	return s.sub.Unsubscribe()
}
