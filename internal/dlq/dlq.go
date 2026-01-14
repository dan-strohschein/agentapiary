// Package dlq provides dead letter queue functionality for failed messages.
package dlq

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/internal/nectar"
	"go.uber.org/zap"
)

// DLQMessage represents a message in the dead letter queue.
type DLQMessage struct {
	ID            string            `json:"id"`
	OriginalMsg   *nectar.Message   `json:"originalMsg"`
	FailureReason string            `json:"failureReason"`
	FailureTime   time.Time         `json:"failureTime"`
	Attempts      int               `json:"attempts"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Manager manages dead letter queues per Hive.
type Manager struct {
	queues map[string]*queue // key: namespace/hiveName
	mu     sync.RWMutex
	logger *zap.Logger
}

// queue represents a DLQ for a specific Hive.
type queue struct {
	messages []*DLQMessage
	mu       sync.RWMutex
	maxSize  int
}

// NewManager creates a new DLQ manager.
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		queues: make(map[string]*queue),
		logger: logger,
	}
}

// AddMessage adds a failed message to the DLQ for a Hive.
func (m *Manager) AddMessage(ctx context.Context, namespace, hiveName string, msg *nectar.Message, failureReason string, attempts int) error {
	key := fmt.Sprintf("%s/%s", namespace, hiveName)

	m.mu.Lock()
	q, exists := m.queues[key]
	if !exists {
		q = &queue{
			messages: make([]*DLQMessage, 0),
			maxSize:  1000, // Default max size per queue
		}
		m.queues[key] = q
	}
	m.mu.Unlock()

	dlqMsg := &DLQMessage{
		ID:            fmt.Sprintf("%s-%d", msg.ID, time.Now().UnixNano()),
		OriginalMsg:   msg,
		FailureReason: failureReason,
		FailureTime:   time.Now(),
		Attempts:      attempts,
		Metadata:      make(map[string]string),
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if queue is full
	if len(q.messages) >= q.maxSize {
		// Remove oldest message (FIFO)
		q.messages = q.messages[1:]
	}

	q.messages = append(q.messages, dlqMsg)

	m.logger.Info("Message added to DLQ",
		zap.String("namespace", namespace),
		zap.String("hive", hiveName),
		zap.String("msgID", msg.ID),
		zap.String("reason", failureReason),
	)

	return nil
}

// ListMessages lists all messages in the DLQ for a Hive.
func (m *Manager) ListMessages(ctx context.Context, namespace, hiveName string) ([]*DLQMessage, error) {
	key := fmt.Sprintf("%s/%s", namespace, hiveName)

	m.mu.RLock()
	q, exists := m.queues[key]
	m.mu.RUnlock()

	if !exists {
		return []*DLQMessage{}, nil
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	// Return a copy of messages
	messages := make([]*DLQMessage, len(q.messages))
	copy(messages, q.messages)

	return messages, nil
}

// GetMessage retrieves a specific message from the DLQ.
func (m *Manager) GetMessage(ctx context.Context, namespace, hiveName, messageID string) (*DLQMessage, error) {
	key := fmt.Sprintf("%s/%s", namespace, hiveName)

	m.mu.RLock()
	q, exists := m.queues[key]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("DLQ not found for %s/%s", namespace, hiveName)
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, msg := range q.messages {
		if msg.ID == messageID {
			return msg, nil
		}
	}

	return nil, fmt.Errorf("message not found: %s", messageID)
}

// ClearMessages clears all messages from the DLQ for a Hive.
func (m *Manager) ClearMessages(ctx context.Context, namespace, hiveName string) error {
	key := fmt.Sprintf("%s/%s", namespace, hiveName)

	m.mu.Lock()
	defer m.mu.Unlock()

	q, exists := m.queues[key]
	if !exists {
		return nil // Queue doesn't exist, nothing to clear
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	q.messages = make([]*DLQMessage, 0)

	m.logger.Info("DLQ cleared",
		zap.String("namespace", namespace),
		zap.String("hive", hiveName),
	)

	return nil
}

// DeleteMessage removes a specific message from the DLQ.
func (m *Manager) DeleteMessage(ctx context.Context, namespace, hiveName, messageID string) error {
	key := fmt.Sprintf("%s/%s", namespace, hiveName)

	m.mu.RLock()
	q, exists := m.queues[key]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("DLQ not found for %s/%s", namespace, hiveName)
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for i, msg := range q.messages {
		if msg.ID == messageID {
			q.messages = append(q.messages[:i], q.messages[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("message not found: %s", messageID)
}

// GetQueueSize returns the number of messages in the DLQ for a Hive.
func (m *Manager) GetQueueSize(namespace, hiveName string) int {
	key := fmt.Sprintf("%s/%s", namespace, hiveName)

	m.mu.RLock()
	defer m.mu.RUnlock()

	q, exists := m.queues[key]
	if !exists {
		return 0
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.messages)
}

// GetDLQTopic returns the DLQ topic for a Hive based on routing configuration.
// If DeadLetterTopic is configured in HiveSpec.Routing, use it.
// Otherwise, use default pattern: "dlq.{namespace}.{hiveName}".
func GetDLQTopic(namespace, hiveName, configuredTopic string) string {
	if configuredTopic != "" {
		// Use configured topic, but prefix with namespace if not already prefixed
		// For now, return as-is (caller should handle namespace prefixing if needed)
		return configuredTopic
	}
	// Default pattern
	return fmt.Sprintf("dlq.%s.%s", namespace, hiveName)
}
