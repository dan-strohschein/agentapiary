// Package patterns provides orchestration patterns for agent coordination.
package patterns

import (
	"context"
	"fmt"
	"sync"

	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/agentapiary/apiary/pkg/waggle"
	"go.uber.org/zap"
)

// EventFilter defines a filter for event-driven subscriptions.
type EventFilter struct {
	Type     string            `json:"type,omitempty"`
	Priority string            `json:"priority,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EventDriven orchestrates an event-driven pattern.
type EventDriven struct {
	hive         *apiary.Hive
	bus          nectar.MessageBus
	logger       *zap.Logger
	eventSource  string
	subscribers  map[string]*SubscriberConfig
	subs         map[string]nectar.Subscription
	mu           sync.RWMutex
	cancel       context.CancelFunc
}

// SubscriberConfig configures an event subscriber.
type SubscriberConfig struct {
	AgentID string
	Filter  EventFilter
}

// NewEventDriven creates a new event-driven orchestrator.
func NewEventDriven(hive *apiary.Hive, bus nectar.MessageBus, logger *zap.Logger) *EventDriven {
	return &EventDriven{
		hive:        hive,
		bus:         bus,
		logger:      logger,
		subscribers: make(map[string]*SubscriberConfig),
		subs:        make(map[string]nectar.Subscription),
	}
}

// Initialize initializes the event-driven pattern from Hive spec.
func (e *EventDriven) Initialize(eventSource string, subscribers []*SubscriberConfig) error {
	if e.hive.Spec.Pattern != "event-driven" {
		return fmt.Errorf("hive pattern is not event-driven: %s", e.hive.Spec.Pattern)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.eventSource = eventSource
	for _, sub := range subscribers {
		e.subscribers[sub.AgentID] = sub
	}

	return nil
}

// Start starts the event-driven pattern.
func (e *EventDriven) Start(ctx context.Context) error {
	e.mu.Lock()
	
	// Create cancel function from parent context
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	e.cancel = cancel
	
	e.mu.Unlock()

	// Subscribe to event source topic (prefixed with Cell/namespace)
	namespace := e.hive.GetNamespace()
	baseEventTopic := fmt.Sprintf("events.%s.%s", e.hive.GetName(), e.eventSource)
	eventTopic := nectar.PrefixTopic(namespace, baseEventTopic)

	sub, err := e.bus.Subscribe(ctx, eventTopic, func(ctx context.Context, busMsg *nectar.Message) error {
		return e.handleEvent(ctx, busMsg)
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to subscribe to event topic: %w", err)
	}

	e.mu.Lock()
	e.subs[eventTopic] = sub
	e.mu.Unlock()

	// Set up individual subscriptions for each subscriber with their filters
	// The filter will be applied in handleEvent
	e.mu.RLock()
	subscribers := make([]string, 0, len(e.subscribers))
	for agentID := range e.subscribers {
		subscribers = append(subscribers, agentID)
	}
	e.mu.RUnlock()
	
	for _, agentID := range subscribers {
		e.logger.Info("Event subscriber configured",
			zap.String("agentID", agentID),
			zap.String("eventSource", e.eventSource),
		)
	}

	e.logger.Info("Event-driven pattern started",
		zap.String("hive", e.hive.GetName()),
		zap.String("eventSource", e.eventSource),
		zap.Int("subscribers", len(subscribers)),
	)

	return nil
}

// handleEvent handles an event and routes it to matching subscribers.
func (e *EventDriven) handleEvent(ctx context.Context, busMsg *nectar.Message) error {
	waggleMsg, err := waggle.DecodeMessage(busMsg.Payload)
	if err != nil {
		return err
	}

	e.mu.RLock()
	subscribers := make([]*SubscriberConfig, 0, len(e.subscribers))
	for _, config := range e.subscribers {
		subscribers = append(subscribers, config)
	}
	e.mu.RUnlock()

	// Route event to all matching subscribers
	e.mu.RLock()
	namespace := e.hive.GetNamespace()
	e.mu.RUnlock()
	
	for _, config := range subscribers {
		if e.matchesFilter(waggleMsg, config.Filter) {
			// Route to subscriber (prefixed with Cell/namespace)
			baseAgentTopic := fmt.Sprintf("agent.%s.inbound", config.AgentID)
			agentTopic := nectar.PrefixTopic(namespace, baseAgentTopic)
			
			payload, err := waggleMsg.Encode()
			if err != nil {
				e.logger.Error("Failed to encode event for subscriber",
					zap.String("agentID", config.AgentID),
					zap.Error(err),
				)
				continue
			}

			eventBusMsg := &nectar.Message{
				ID:        waggleMsg.ID,
				Type:      string(waggleMsg.Type),
				Payload:   payload,
				Metadata:  waggleMsg.Metadata,
				Timestamp: waggleMsg.Timestamp,
				Source:    waggleMsg.SourceAgentID,
				SessionID: waggleMsg.SessionID,
			}

			if err := e.bus.Publish(ctx, agentTopic, eventBusMsg); err != nil {
				e.logger.Error("Failed to route event to subscriber",
					zap.String("agentID", config.AgentID),
					zap.Error(err),
				)
			} else {
				e.logger.Debug("Event routed to subscriber",
					zap.String("agentID", config.AgentID),
					zap.String("eventID", waggleMsg.ID),
				)
			}
		}
	}

	return nil
}

// matchesFilter checks if a message matches an event filter.
func (e *EventDriven) matchesFilter(msg *waggle.Message, filter EventFilter) bool {
	// If no filter specified, match all
	if filter.Type == "" && filter.Priority == "" && len(filter.Metadata) == 0 {
		return true
	}

	// Check type
	if filter.Type != "" && string(msg.Type) != filter.Type {
		return false
	}

	// Check priority (from metadata)
	if filter.Priority != "" {
		msgPriority := msg.GetMetadata("priority")
		if msgPriority != filter.Priority {
			return false
		}
	}

	// Check metadata filters
	if len(filter.Metadata) > 0 {
		for key, value := range filter.Metadata {
			msgValue := msg.GetMetadata(key)
			if msgValue != value {
				return false
			}
		}
	}

	return true
}

// PublishEvent publishes an event to the event source.
func (e *EventDriven) PublishEvent(ctx context.Context, msg *waggle.Message) error {
	e.mu.RLock()
	namespace := e.hive.GetNamespace()
	e.mu.RUnlock()
	
	baseEventTopic := fmt.Sprintf("events.%s.%s", e.hive.GetName(), e.eventSource)
	eventTopic := nectar.PrefixTopic(namespace, baseEventTopic)

	payload, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}

	busMsg := &nectar.Message{
		ID:        msg.ID,
		Type:      string(msg.Type),
		Payload:   payload,
		Metadata:  msg.Metadata,
		Timestamp: msg.Timestamp,
		Source:    msg.SourceAgentID,
		SessionID: msg.SessionID,
	}

	return e.bus.Publish(ctx, eventTopic, busMsg)
}

// AddSubscriber adds a new event subscriber.
func (e *EventDriven) AddSubscriber(agentID string, filter EventFilter) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.subscribers[agentID] = &SubscriberConfig{
		AgentID: agentID,
		Filter:  filter,
	}
}

// RemoveSubscriber removes an event subscriber.
func (e *EventDriven) RemoveSubscriber(agentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.subscribers, agentID)
}

// Stop stops the event-driven pattern.
func (e *EventDriven) Stop() {
	e.mu.Lock()
	subs := make(map[string]nectar.Subscription)
	for k, v := range e.subs {
		subs[k] = v
	}
	e.subs = make(map[string]nectar.Subscription)
	cancel := e.cancel
	e.mu.Unlock()
	
	// Cancel context
	if cancel != nil {
		cancel()
	}
	
	// Unsubscribe all
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
}
