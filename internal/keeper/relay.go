// Package keeper provides the Keeper sidecar for managing agent processes.
package keeper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/internal/guardrails"
	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/pkg/waggle"
	"go.uber.org/zap"
)

// messageQueue provides buffering for messages.
type messageQueue struct {
	queue   chan *queuedMessage
	mu      sync.RWMutex
	maxSize int
}

type queuedMessage struct {
	msg      *waggle.Message
	attempts int
	nextTry  time.Time
}

// newMessageQueue creates a new message queue.
func newMessageQueue(maxSize int) *messageQueue {
	return &messageQueue{
		queue:   make(chan *queuedMessage, maxSize),
		maxSize: maxSize,
	}
}

// enqueue adds a message to the queue (non-blocking).
func (q *messageQueue) enqueue(msg *waggle.Message) error {
	select {
	case q.queue <- &queuedMessage{msg: msg, attempts: 0}:
		return nil
	default:
		return fmt.Errorf("message queue full")
	}
}

// dequeue removes a message from the queue.
func (q *messageQueue) dequeue(ctx context.Context) (*queuedMessage, error) {
	select {
	case msg := <-q.queue:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// size returns the current queue size.
func (q *messageQueue) size() int {
	return len(q.queue)
}

// StartMessageRelay starts the message relay between agent and message bus.
func (k *Keeper) StartMessageRelay(ctx context.Context, bus nectar.MessageBus, agentID string) error {
	if bus == nil {
		return fmt.Errorf("message bus is required")
	}

	k.mu.Lock()
	k.messageBus = bus
	k.agentID = agentID
	k.inboundQueue = newMessageQueue(1000)  // Buffer up to 1000 inbound messages
	k.outboundQueue = newMessageQueue(1000) // Buffer up to 1000 outbound messages
	k.mu.Unlock()

	// Subscribe to agent's inbound topic (prefixed with Cell/namespace)
	namespace := k.spec.GetNamespace()
	baseTopic := fmt.Sprintf("agent.%s.inbound", agentID)
	topic := nectar.PrefixTopic(namespace, baseTopic)
	sub, err := bus.Subscribe(ctx, topic, func(ctx context.Context, busMsg *nectar.Message) error {
		return k.handleInboundMessage(ctx, busMsg)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to inbound topic: %w", err)
	}

	k.mu.Lock()
	k.busSubscription = sub
	k.mu.Unlock()

	// Start message delivery goroutine (bus -> agent)
	go k.deliverMessages(ctx)

	// Start message publishing goroutine (agent -> bus)
	go k.publishMessages(ctx)

	// Start reading from agent stdout (if stdin interface)
	if k.spec.Spec.Interface.Type == "stdin" {
		go k.readAgentOutput(ctx)
	}

	k.logger.Info("Message relay started",
		zap.String("agent", agentID),
		zap.String("topic", topic),
	)

	return nil
}

// handleInboundMessage handles a message received from the bus.
func (k *Keeper) handleInboundMessage(ctx context.Context, busMsg *nectar.Message) error {
	// Check rate limit before processing
	if k.rateLimiter != nil {
		if !k.rateLimiter.Allow(ctx) {
			k.logger.Warn("Rate limit exceeded, rejecting request",
				zap.String("busMsgID", busMsg.ID),
			)
			// Record rate limit metric
			if k.metrics != nil && k.spec != nil {
				k.metrics.RecordRateLimitExceeded(
					k.spec.GetName(),
					k.spec.GetNamespace(),
					k.agentID,
				)
			}
			return &guardrails.RateLimitExceededError{
				RequestsPerMinute: k.rateLimiter.RequestsPerMinute(),
			}
		}
	}

	// Decode Waggle message from bus message
	waggleMsg, err := waggle.DecodeMessage(busMsg.Payload)
	if err != nil {
		k.logger.Error("Failed to decode Waggle message",
			zap.String("busMsgID", busMsg.ID),
			zap.Error(err),
		)
		return err
	}

	// Set destination agent ID
	waggleMsg.DestAgentID = k.agentID

	// Queue for delivery to agent
	if err := k.inboundQueue.enqueue(waggleMsg); err != nil {
		k.logger.Warn("Failed to queue inbound message, queue full",
			zap.String("msgID", waggleMsg.ID),
			zap.Error(err),
		)
		// Return error to trigger retry at bus level
		return err
	}

	// Update queue depth metric
	if k.metrics != nil {
		k.metrics.SetQueueDepth(
			k.spec.GetName(),
			k.spec.GetNamespace(),
			k.agentID,
			k.inboundQueue.size(),
		)
	}

	return nil
}

// deliverMessages delivers queued messages to the agent.
func (k *Keeper) deliverMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Dequeue message
			queuedMsg, err := k.inboundQueue.dequeue(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				continue
			}

			// Check context cancellation before starting retry loop
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Deliver message with retry
			if err := k.deliverMessageWithRetry(ctx, queuedMsg); err != nil {
				k.logger.Error("Failed to deliver message after retries",
					zap.String("msgID", queuedMsg.msg.ID),
					zap.Int("attempts", queuedMsg.attempts),
					zap.Error(err),
				)
				// Message is dropped after max retries
			}
		}
	}
}

// deliverMessageWithRetry delivers a message with exponential backoff retry.
func (k *Keeper) deliverMessageWithRetry(ctx context.Context, queuedMsg *queuedMessage) error {
	const maxRetries = 3
	const baseDelay = 100 * time.Millisecond

	for queuedMsg.attempts < maxRetries {
		// Check if it's time to retry
		if queuedMsg.attempts > 0 && time.Now().Before(queuedMsg.nextTry) {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Try to deliver
		err := k.SendMessage(queuedMsg.msg)
		if err == nil {
			// Success - start tracking this request for timeout
			if k.timeoutTracker != nil {
				k.timeoutTracker.StartRequest(queuedMsg.msg.ID)
			}
			return nil
		}

		queuedMsg.attempts++
		if queuedMsg.attempts >= maxRetries {
			return fmt.Errorf("max retries exceeded: %w", err)
		}

		// Calculate exponential backoff
		delay := baseDelay * time.Duration(1<<uint(queuedMsg.attempts-1))
		queuedMsg.nextTry = time.Now().Add(delay)

		k.logger.Warn("Message delivery failed, will retry",
			zap.String("msgID", queuedMsg.msg.ID),
			zap.Int("attempt", queuedMsg.attempts),
			zap.Duration("delay", delay),
			zap.Error(err),
		)

		// Wait before retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to retry
		}
	}

	return fmt.Errorf("max retries exceeded")
}

// publishMessages publishes queued messages from agent to bus.
func (k *Keeper) publishMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Dequeue message from agent
			queuedMsg, err := k.outboundQueue.dequeue(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				continue
			}

			// Check context cancellation before starting retry loop
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Publish to bus with retry
			if err := k.publishMessageWithRetry(ctx, queuedMsg); err != nil {
				k.logger.Error("Failed to publish message after retries",
					zap.String("msgID", queuedMsg.msg.ID),
					zap.Int("attempts", queuedMsg.attempts),
					zap.Error(err),
				)
			}
		}
	}
}

// publishMessageWithRetry publishes a message with exponential backoff retry.
func (k *Keeper) publishMessageWithRetry(ctx context.Context, queuedMsg *queuedMessage) error {
	const maxRetries = 3
	const baseDelay = 100 * time.Millisecond

	for queuedMsg.attempts < maxRetries {
		// Check if it's time to retry
		if queuedMsg.attempts > 0 && time.Now().Before(queuedMsg.nextTry) {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Determine topic based on message destination
		topic := k.determineTopic(queuedMsg.msg)

		// Encode message
		payload, err := queuedMsg.msg.Encode()
		if err != nil {
			return fmt.Errorf("failed to encode message: %w", err)
		}

		// Validate output if validator is configured
		if k.outputValidator != nil {
			if err := k.outputValidator.Validate(ctx, queuedMsg.msg.Payload); err != nil {
				k.logger.Error("Output validation failed",
					zap.String("msgID", queuedMsg.msg.ID),
					zap.Error(err),
				)
				// Record validation failure metric
				if k.metrics != nil && k.spec != nil {
					k.metrics.RecordOutputValidationFailure(
						k.spec.GetName(),
						k.spec.GetNamespace(),
						k.agentID,
					)
				}
				// Validation failed - return error to prevent publishing
				return fmt.Errorf("output validation failed: %w", err)
			}
		}

		// Track token usage if tracker is configured
		if k.tokenTracker != nil {
			tokenCount := guardrails.ExtractTokenCount(queuedMsg.msg.Metadata)
			if tokenCount > 0 {
				if err := k.tokenTracker.RecordUsage(ctx, tokenCount); err != nil {
					k.logger.Error("Token budget exceeded",
						zap.String("msgID", queuedMsg.msg.ID),
						zap.Error(err),
					)
					// Budget exceeded - return error to prevent publishing
					// TODO: Report to Queen and terminate agent if needed
					return err
				}
			}
		}

		// Create bus message
		busMsg := &nectar.Message{
			ID:        queuedMsg.msg.ID,
			Type:      string(queuedMsg.msg.Type),
			Payload:   payload,
			Metadata:  queuedMsg.msg.Metadata,
			Timestamp: queuedMsg.msg.Timestamp,
			Source:   k.agentID,
			SessionID: queuedMsg.msg.SessionID,
		}

		// Publish
		err = k.messageBus.Publish(ctx, topic, busMsg)
		if err == nil {
			// Success - complete tracking for this response
			// Try to match by correlation ID first, then by message ID
			if k.timeoutTracker != nil {
				msgID := queuedMsg.msg.ID
				if queuedMsg.msg.CorrelationID != "" {
					// If this is a response with correlation ID, complete the original request
					k.timeoutTracker.CompleteRequest(queuedMsg.msg.CorrelationID)
				} else {
					// Otherwise, complete by message ID
					k.timeoutTracker.CompleteRequest(msgID)
				}
			}
			return nil
		}

		queuedMsg.attempts++
		if queuedMsg.attempts >= maxRetries {
			return fmt.Errorf("max retries exceeded: %w", err)
		}

		// Calculate exponential backoff
		delay := baseDelay * time.Duration(1<<uint(queuedMsg.attempts-1))
		queuedMsg.nextTry = time.Now().Add(delay)

		k.logger.Warn("Message publish failed, will retry",
			zap.String("msgID", queuedMsg.msg.ID),
			zap.String("topic", topic),
			zap.Int("attempt", queuedMsg.attempts),
			zap.Duration("delay", delay),
			zap.Error(err),
		)

		// Wait before retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to retry
		}
	}

	return fmt.Errorf("max retries exceeded")
}

// determineTopic determines the bus topic for a message (prefixed with Cell/namespace).
func (k *Keeper) determineTopic(msg *waggle.Message) string {
	namespace := k.spec.GetNamespace()
	var baseTopic string
	
	// If message has specific destination, use agent-specific topic
	if msg.DestAgentID != "" {
		baseTopic = fmt.Sprintf("agent.%s.inbound", msg.DestAgentID)
		return nectar.PrefixTopic(namespace, baseTopic)
	}

	// If message has correlation ID, it's a response
	if msg.CorrelationID != "" {
		// Response messages go to the original requester
		// For now, use a general response topic
		baseTopic = fmt.Sprintf("agent.%s.response", msg.SourceAgentID)
		return nectar.PrefixTopic(namespace, baseTopic)
	}

	// Default: broadcast to session
	if msg.SessionID != "" {
		baseTopic = fmt.Sprintf("session.%s", msg.SessionID)
		return nectar.PrefixTopic(namespace, baseTopic)
	}

	// Fallback: agent's outbound topic
	baseTopic = fmt.Sprintf("agent.%s.outbound", k.agentID)
	return nectar.PrefixTopic(namespace, baseTopic)
}

// readAgentOutput reads messages from agent stdout.
func (k *Keeper) readAgentOutput(ctx context.Context) {
	// TODO: Implement stdout reading for stdin interface
	// This would involve:
	// 1. Reading from process.Stdout
	// 2. Parsing Waggle messages (newline-delimited JSON)
	// 3. Queueing for publishing to bus
	k.logger.Debug("Agent output reading not yet fully implemented")
}

// PublishAgentMessage queues a message from the agent to be published to the bus.
func (k *Keeper) PublishAgentMessage(msg *waggle.Message) error {
	// Set source agent ID
	msg.SourceAgentID = k.agentID

	// Queue for publishing
	if err := k.outboundQueue.enqueue(msg); err != nil {
		return fmt.Errorf("failed to queue message: %w", err)
	}

	return nil
}
