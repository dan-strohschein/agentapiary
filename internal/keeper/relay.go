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

	// Create cancelable context for message relay goroutines
	relayCtx, cancel := context.WithCancel(ctx)

	k.mu.Lock()
	k.messageBus = bus
	k.agentID = agentID
	k.inboundQueue = newMessageQueue(1000)  // Buffer up to 1000 inbound messages
	k.outboundQueue = newMessageQueue(1000) // Buffer up to 1000 outbound messages
	k.relayCancel = cancel // Store cancel function for cleanup
	k.mu.Unlock()

	// Subscribe to agent's inbound topic (prefixed with Cell/namespace)
	namespace := k.spec.GetNamespace()
	baseTopic := fmt.Sprintf("agent.%s.inbound", agentID)
	topic := nectar.PrefixTopic(namespace, baseTopic)
	sub, err := bus.Subscribe(relayCtx, topic, func(ctx context.Context, busMsg *nectar.Message) error {
		return k.handleInboundMessage(ctx, busMsg)
	})
	if err != nil {
		cancel() // Clean up context on error
		return fmt.Errorf("failed to subscribe to inbound topic: %w", err)
	}

	k.mu.Lock()
	k.busSubscription = sub
	k.mu.Unlock()

	// Start message delivery goroutine (bus -> agent)
	k.relayWg.Add(1)
	go func() {
		defer k.relayWg.Done()
		k.deliverMessages(relayCtx)
	}()

	// Start message publishing goroutine (agent -> bus)
	k.relayWg.Add(1)
	go func() {
		defer k.relayWg.Done()
		k.publishMessages(relayCtx)
	}()

	// Start reading from agent stdout (if stdin interface)
	if k.spec.Spec.Interface.Type == "stdin" {
		k.relayWg.Add(1)
		go func() {
			defer k.relayWg.Done()
			k.readAgentOutput(relayCtx)
		}()
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
			// Audit log guardrail violation
			if k.auditLogger != nil && k.spec != nil {
				sessionID := busMsg.SessionID
				if sessionID == "" {
					sessionID = k.sessionID // Fallback to Keeper's session ID
				}
				hiveName, _ := k.getHiveNameFromSession(ctx, sessionID)
				rateLimitErr := &guardrails.RateLimitExceededError{
					RequestsPerMinute: k.rateLimiter.RequestsPerMinute(),
				}
				k.auditLogger.LogGuardrailViolation(
					"rate_limit",
					k.spec.GetNamespace(),
					hiveName,
					k.spec.GetName(),
					k.agentID,
					sessionID,
					busMsg.ID,
					rateLimitErr.Error(),
					map[string]interface{}{
						"requestsPerMinute": k.rateLimiter.RequestsPerMinute(),
					},
				)
			}
			// Fire webhook for rate limit violation
			k.fireWebhook(ctx, "rate_limit", busMsg.ID, &guardrails.RateLimitExceededError{
				RequestsPerMinute: k.rateLimiter.RequestsPerMinute(),
			})
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

	// Audit log agent input
	if k.auditLogger != nil && k.spec != nil {
		details := map[string]interface{}{
			"messageType": string(waggleMsg.Type),
			"sourceAgentID": waggleMsg.SourceAgentID,
			"correlationID": waggleMsg.CorrelationID,
		}
		// Include payload size (not full payload for security/performance)
		if len(waggleMsg.Payload) > 0 {
			details["payloadSize"] = len(waggleMsg.Payload)
		}
		k.auditLogger.LogAgentInput(
			k.spec.GetNamespace(),
			k.spec.GetName(),
			k.agentID,
			waggleMsg.SessionID,
			waggleMsg.ID,
			details,
		)
	}

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
				// Audit log guardrail violation
				if k.auditLogger != nil && k.spec != nil {
					hiveName, _ := k.getHiveNameFromSession(ctx, k.sessionID)
					k.auditLogger.LogGuardrailViolation(
						"output_validation",
						k.spec.GetNamespace(),
						hiveName,
						k.spec.GetName(),
						k.agentID,
						queuedMsg.msg.SessionID,
						queuedMsg.msg.ID,
						err.Error(),
						map[string]interface{}{
							"payloadSize": len(queuedMsg.msg.Payload),
						},
					)
				}
				// Fire webhook for output validation failure
				k.fireWebhook(ctx, "output_validation", queuedMsg.msg.ID, err)
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
					// Audit log guardrail violation
					if k.auditLogger != nil && k.spec != nil {
						hiveName, _ := k.getHiveNameFromSession(ctx, k.sessionID)
						k.auditLogger.LogGuardrailViolation(
							"token_budget",
							k.spec.GetNamespace(),
							hiveName,
							k.spec.GetName(),
							k.agentID,
							queuedMsg.msg.SessionID,
							queuedMsg.msg.ID,
							err.Error(),
							map[string]interface{}{
								"tokenCount": tokenCount,
							},
						)
					}
					// Fire webhook for token budget violation
					k.fireWebhook(ctx, "token_budget", queuedMsg.msg.ID, err)
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
			// Audit log agent output
			if k.auditLogger != nil && k.spec != nil {
				details := map[string]interface{}{
					"messageType": string(queuedMsg.msg.Type),
					"topic": topic,
					"correlationID": queuedMsg.msg.CorrelationID,
				}
				// Include payload size (not full payload for security/performance)
				if len(queuedMsg.msg.Payload) > 0 {
					details["payloadSize"] = len(queuedMsg.msg.Payload)
				}
				// Include token count if available
				if tokenCount := guardrails.ExtractTokenCount(queuedMsg.msg.Metadata); tokenCount > 0 {
					details["tokenCount"] = tokenCount
				}
				k.auditLogger.LogAgentOutput(
					k.spec.GetNamespace(),
					k.spec.GetName(),
					k.agentID,
					queuedMsg.msg.SessionID,
					queuedMsg.msg.ID,
					details,
				)
			}

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
	
	// For now, wait for context cancellation to ensure proper cleanup
	<-ctx.Done()
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
