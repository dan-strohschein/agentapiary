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

// Swarm orchestrates a peer-to-peer swarm pattern.
type Swarm struct {
	hive      *apiary.Hive
	bus       nectar.MessageBus
	logger    *zap.Logger
	agentIDs  []string
	subs      []nectar.Subscription
	mu        sync.RWMutex
	cancel    context.CancelFunc
}

// NewSwarm creates a new swarm orchestrator.
func NewSwarm(hive *apiary.Hive, bus nectar.MessageBus, logger *zap.Logger) *Swarm {
	return &Swarm{
		hive:   hive,
		bus:    bus,
		logger: logger,
	}
}

// Initialize initializes the swarm from Hive spec.
func (s *Swarm) Initialize(agentIDs []string) error {
	if s.hive.Spec.Pattern != "swarm" {
		return fmt.Errorf("hive pattern is not swarm: %s", s.hive.Spec.Pattern)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.agentIDs = agentIDs
	return nil
}

// Start starts the swarm pattern.
func (s *Swarm) Start(ctx context.Context) error {
	s.mu.Lock()
	
	// Create cancel function from parent context
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	s.cancel = cancel
	
	s.mu.Unlock()

	// In swarm pattern, agents communicate peer-to-peer
	// Each agent subscribes to a shared topic and can publish to it
	s.mu.RLock()
	namespace := s.hive.GetNamespace()
	s.mu.RUnlock()
	
	baseSharedTopic := fmt.Sprintf("swarm.%s", s.hive.GetName())
	sharedTopic := nectar.PrefixTopic(namespace, baseSharedTopic)

	// Subscribe to shared topic for each agent
	// In a real implementation, each agent would have its own subscription
	// For now, we set up a general subscription that routes messages
	sub, err := s.bus.Subscribe(ctx, sharedTopic, func(ctx context.Context, busMsg *nectar.Message) error {
		return s.handleSwarmMessage(ctx, busMsg)
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to subscribe to swarm topic: %w", err)
	}

	s.mu.Lock()
	s.subs = append(s.subs, sub)
	s.mu.Unlock()

	s.mu.RLock()
	agentCount := len(s.agentIDs)
	s.mu.RUnlock()
	
	s.logger.Info("Swarm pattern started",
		zap.String("hive", s.hive.GetName()),
		zap.String("topic", sharedTopic),
		zap.Int("agents", agentCount),
	)

	return nil
}

// handleSwarmMessage handles messages in the swarm.
func (s *Swarm) handleSwarmMessage(ctx context.Context, busMsg *nectar.Message) error {
	waggleMsg, err := waggle.DecodeMessage(busMsg.Payload)
	if err != nil {
		return err
	}

	// In swarm pattern, messages are broadcast to all agents
	// Agents filter messages based on their own logic
	// No central coordination needed

	s.logger.Debug("Swarm message received",
		zap.String("msgID", waggleMsg.ID),
		zap.String("source", waggleMsg.SourceAgentID),
		zap.String("type", string(waggleMsg.Type)),
	)

	return nil
}

// Publish publishes a message to the swarm.
func (s *Swarm) Publish(ctx context.Context, msg *waggle.Message) error {
	s.mu.RLock()
	namespace := s.hive.GetNamespace()
	s.mu.RUnlock()
	
	baseSharedTopic := fmt.Sprintf("swarm.%s", s.hive.GetName())
	sharedTopic := nectar.PrefixTopic(namespace, baseSharedTopic)

	payload, err := msg.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
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

	return s.bus.Publish(ctx, sharedTopic, busMsg)
}

// Stop stops the swarm pattern.
func (s *Swarm) Stop() {
	s.mu.Lock()
	subs := make([]nectar.Subscription, len(s.subs))
	copy(subs, s.subs)
	s.subs = nil
	cancel := s.cancel
	s.mu.Unlock()
	
	// Cancel context
	if cancel != nil {
		cancel()
	}
	
	// Unsubscribe all
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
}
