// Package patterns provides orchestration patterns for agent coordination.
//
// This file implements the ComposedPattern orchestrator, which chains multiple
// patterns together in sequence, routing messages from one pattern's output to
// the next pattern's input.
package patterns

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/agentapiary/apiary/pkg/waggle"
	"go.uber.org/zap"
)

// ComposedPattern orchestrates a sequence of patterns, routing messages between them.
type ComposedPattern struct {
	hive      *apiary.Hive
	bus       nectar.MessageBus
	logger    *zap.Logger
	segments  []*PatternSegmentInstance
	mu        sync.RWMutex
	state     ComposedPatternState
	cancel    context.CancelFunc
}

// PatternSegmentInstance represents an instantiated pattern segment.
type PatternSegmentInstance struct {
	Segment   apiary.PatternSegment
	Pattern   interface{} // The actual pattern instance (Pipeline, Hierarchical, etc.)
	InputSub  nectar.Subscription
	OutputSub nectar.Subscription
	Index     int
}

// ComposedPatternState represents the state of a composed pattern execution.
type ComposedPatternState struct {
	Status      ComposedPatternStatus
	CurrentSegment int
	StartedAt   time.Time
	CompletedAt *time.Time
	Error       error
}

// ComposedPatternStatus represents the status of a composed pattern.
type ComposedPatternStatus string

const (
	ComposedPatternStatusPending   ComposedPatternStatus = "pending"
	ComposedPatternStatusRunning   ComposedPatternStatus = "running"
	ComposedPatternStatusCompleted ComposedPatternStatus = "completed"
	ComposedPatternStatusFailed    ComposedPatternStatus = "failed"
)

// NewComposedPattern creates a new composed pattern orchestrator.
func NewComposedPattern(hive *apiary.Hive, bus nectar.MessageBus, logger *zap.Logger) *ComposedPattern {
	return &ComposedPattern{
		hive:   hive,
		bus:    bus,
		logger: logger,
		state: ComposedPatternState{
			Status:    ComposedPatternStatusPending,
			StartedAt: time.Now(),
		},
	}
}

// Initialize initializes the composed pattern by creating all pattern segments.
func (c *ComposedPattern) Initialize() error {
	if len(c.hive.Spec.PatternSegments) == 0 {
		return fmt.Errorf("no pattern segments specified")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.segments = make([]*PatternSegmentInstance, len(c.hive.Spec.PatternSegments))

	namespace := c.hive.GetNamespace()
	hiveName := c.hive.GetName()

	for i, segmentSpec := range c.hive.Spec.PatternSegments {
		// Create a temporary Hive for this segment to use with NewOrchestrator
		segmentHive := &apiary.Hive{
			TypeMeta:   c.hive.TypeMeta,
			ObjectMeta: c.hive.ObjectMeta,
			Spec: apiary.HiveSpec{
				Pattern: segmentSpec.Pattern,
				Stages:  c.extractStagesFromConfig(segmentSpec.Config),
			},
		}

		// Create the pattern instance
		pattern, err := createPatternForSegment(segmentHive, c.bus, c.logger, segmentSpec)
		if err != nil {
			return fmt.Errorf("failed to create pattern for segment %s: %w", segmentSpec.Name, err)
		}

		// Determine input and output topics
		inputTopic := segmentSpec.InputTopic
		if inputTopic == "" && i > 0 {
			// Use previous segment's output topic
			inputTopic = c.segments[i-1].Segment.OutputTopic
		}
		if inputTopic == "" {
			// Default input topic for first segment
			inputTopic = nectar.PrefixTopic(namespace, fmt.Sprintf("composed.%s.%s.input", hiveName, segmentSpec.Name))
		}

		outputTopic := segmentSpec.OutputTopic
		if outputTopic == "" {
			// Default output topic
			outputTopic = nectar.PrefixTopic(namespace, fmt.Sprintf("composed.%s.%s.output", hiveName, segmentSpec.Name))
		}

		c.segments[i] = &PatternSegmentInstance{
			Segment: segmentSpec,
			Pattern: pattern,
			Index:   i,
		}

		// Store topics in segment for routing
		c.segments[i].Segment.InputTopic = inputTopic
		c.segments[i].Segment.OutputTopic = outputTopic
	}

	return nil
}

// Start starts the composed pattern execution.
func (c *ComposedPattern) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.state.Status != ComposedPatternStatusPending {
		c.mu.Unlock()
		return fmt.Errorf("composed pattern already started")
	}
	c.state.Status = ComposedPatternStatusRunning
	c.state.CurrentSegment = 0

	// Create cancel function
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()

	// Initialize if not already done
	if err := c.Initialize(); err != nil {
		c.mu.Lock()
		c.state.Status = ComposedPatternStatusFailed
		c.state.Error = err
		c.mu.Unlock()
		cancel()
		return err
	}

	// Set up message routing between segments
	if err := c.setupRouting(ctx); err != nil {
		c.mu.Lock()
		c.state.Status = ComposedPatternStatusFailed
		c.state.Error = err
		c.mu.Unlock()
		cancel()
		return fmt.Errorf("failed to setup routing: %w", err)
	}

	// Start each pattern segment
	for i, segment := range c.segments {
		if err := c.startSegment(ctx, segment, i); err != nil {
			c.mu.Lock()
			c.state.Status = ComposedPatternStatusFailed
			c.state.Error = err
			c.mu.Unlock()
			cancel()
			return fmt.Errorf("failed to start segment %s: %w", segment.Segment.Name, err)
		}
	}

	c.logger.Info("Composed pattern started",
		zap.String("hive", c.hive.GetName()),
		zap.Int("segments", len(c.segments)),
	)

	return nil
}

// Stop stops the composed pattern execution.
func (c *ComposedPattern) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}

	// Stop all segments
	for _, segment := range c.segments {
		if err := c.stopSegment(ctx, segment); err != nil {
			c.logger.Warn("Failed to stop segment",
				zap.String("segment", segment.Segment.Name),
				zap.Error(err),
			)
		}
	}

	// Unsubscribe from all topics
	for _, segment := range c.segments {
		if segment.InputSub != nil {
			_ = segment.InputSub.Unsubscribe()
		}
		if segment.OutputSub != nil {
			_ = segment.OutputSub.Unsubscribe()
		}
	}

	c.state.Status = ComposedPatternStatusCompleted
	now := time.Now()
	c.state.CompletedAt = &now

	return nil
}

// setupRouting sets up message routing between pattern segments.
func (c *ComposedPattern) setupRouting(ctx context.Context) error {
	for i := 0; i < len(c.segments)-1; i++ {
		currentSegment := c.segments[i]
		nextSegment := c.segments[i+1]

		// Subscribe to current segment's output and forward to next segment's input
		outputTopic := currentSegment.Segment.OutputTopic
		inputTopic := nextSegment.Segment.InputTopic

		sub, err := c.bus.Subscribe(ctx, outputTopic, func(ctx context.Context, msg *nectar.Message) error {
			// Forward message to next segment's input
			return c.bus.Publish(ctx, inputTopic, msg)
		})
		if err != nil {
			return fmt.Errorf("failed to subscribe to segment %s output: %w", currentSegment.Segment.Name, err)
		}

		currentSegment.OutputSub = sub
		c.logger.Debug("Set up routing",
			zap.String("from", outputTopic),
			zap.String("to", inputTopic),
			zap.String("segment", currentSegment.Segment.Name),
		)
	}

	return nil
}

// startSegment starts a pattern segment.
func (c *ComposedPattern) startSegment(ctx context.Context, segment *PatternSegmentInstance, index int) error {
	switch pattern := segment.Pattern.(type) {
	case *Pipeline:
		// Pipeline needs an input message to start
		// For the first segment, we'll wait for external input
		// For subsequent segments, routing will handle it
		if index == 0 {
			// Subscribe to input topic and start pipeline when message arrives
			sub, err := c.bus.Subscribe(ctx, segment.Segment.InputTopic, func(ctx context.Context, msg *nectar.Message) error {
				waggleMsg, err := waggle.DecodeMessage(msg.Payload)
				if err != nil {
					return err
				}
				return pattern.Start(ctx, waggleMsg)
			})
			if err != nil {
				return err
			}
			segment.InputSub = sub
		}
		return pattern.Initialize()

	case *Hierarchical:
		// Hierarchical pattern initialization
		supervisorID := c.getStringFromConfig(segment.Segment.Config, "supervisorID", "")
		workerIDs := c.getStringSliceFromConfig(segment.Segment.Config, "workerIDs", []string{})
		if err := pattern.Initialize(supervisorID, workerIDs); err != nil {
			return err
		}
		return pattern.Start(ctx)

	case *Swarm:
		// Swarm pattern initialization
		agentIDs := c.getStringSliceFromConfig(segment.Segment.Config, "agentIDs", []string{})
		if err := pattern.Initialize(agentIDs); err != nil {
			return err
		}
		return pattern.Start(ctx)

	case *EventDriven:
		// Event-driven pattern initialization
		eventSource := c.getStringFromConfig(segment.Segment.Config, "eventSource", "")
		subscribers := c.extractSubscribersFromConfig(segment.Segment.Config)
		if err := pattern.Initialize(eventSource, subscribers); err != nil {
			return err
		}
		return pattern.Start(ctx)

	default:
		return fmt.Errorf("unknown pattern type for segment %s", segment.Segment.Name)
	}
}

// stopSegment stops a pattern segment.
func (c *ComposedPattern) stopSegment(ctx context.Context, segment *PatternSegmentInstance) error {
	switch pattern := segment.Pattern.(type) {
	case *Pipeline:
		pattern.Stop()
		return nil
	case *Hierarchical:
		pattern.Stop()
		return nil
	case *Swarm:
		pattern.Stop()
		return nil
	case *EventDriven:
		pattern.Stop()
		return nil
	default:
		return fmt.Errorf("unknown pattern type for segment %s", segment.Segment.Name)
	}
}

// Helper functions

// createPatternForSegment creates a pattern instance for a segment.
func createPatternForSegment(hive *apiary.Hive, bus nectar.MessageBus, logger *zap.Logger, segment apiary.PatternSegment) (interface{}, error) {
	switch segment.Pattern {
	case "pipeline":
		return NewPipeline(hive, bus, logger), nil
	case "hierarchical":
		return NewHierarchical(hive, bus, logger), nil
	case "swarm":
		return NewSwarm(hive, bus, logger), nil
	case "event-driven":
		return NewEventDriven(hive, bus, logger), nil
	default:
		return nil, fmt.Errorf("unknown pattern type: %s", segment.Pattern)
	}
}

// extractStagesFromConfig extracts stages from a pattern segment's config.
func (c *ComposedPattern) extractStagesFromConfig(config map[string]interface{}) []apiary.Stage {
	if config == nil {
		return nil
	}

	stagesInterface, ok := config["stages"]
	if !ok {
		return nil
	}

	stagesSlice, ok := stagesInterface.([]interface{})
	if !ok {
		return nil
	}

	stages := make([]apiary.Stage, 0, len(stagesSlice))
	for _, stageInterface := range stagesSlice {
		stageMap, ok := stageInterface.(map[string]interface{})
		if !ok {
			continue
		}

		stage := apiary.Stage{}
		if name, ok := stageMap["name"].(string); ok {
			stage.Name = name
		}
		if agentRef, ok := stageMap["agentRef"].(string); ok {
			stage.AgentRef = agentRef
		}
		if replicas, ok := stageMap["replicas"].(float64); ok {
			stage.Replicas = int(replicas)
		}
		if parallel, ok := stageMap["parallel"].(bool); ok {
			stage.Parallel = parallel
		}
		if waitForAll, ok := stageMap["waitForAll"].(bool); ok {
			stage.WaitForAll = waitForAll
		}

		stages = append(stages, stage)
	}

	return stages
}

// getStringFromConfig extracts a string value from config.
func (c *ComposedPattern) getStringFromConfig(config map[string]interface{}, key string, defaultValue string) string {
	if config == nil {
		return defaultValue
	}
	if value, ok := config[key].(string); ok {
		return value
	}
	return defaultValue
}

// getStringSliceFromConfig extracts a string slice from config.
func (c *ComposedPattern) getStringSliceFromConfig(config map[string]interface{}, key string, defaultValue []string) []string {
	if config == nil {
		return defaultValue
	}
	if value, ok := config[key].([]interface{}); ok {
		result := make([]string, 0, len(value))
		for _, v := range value {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return defaultValue
}

// extractSubscribersFromConfig extracts subscribers from config for event-driven pattern.
func (c *ComposedPattern) extractSubscribersFromConfig(config map[string]interface{}) []*SubscriberConfig {
	if config == nil {
		return nil
	}

	subscribersInterface, ok := config["subscribers"]
	if !ok {
		return nil
	}

	subscribersSlice, ok := subscribersInterface.([]interface{})
	if !ok {
		return nil
	}

	subscribers := make([]*SubscriberConfig, 0, len(subscribersSlice))
	for _, subInterface := range subscribersSlice {
		subMap, ok := subInterface.(map[string]interface{})
		if !ok {
			continue
		}

		subscriber := &SubscriberConfig{}
		if agentID, ok := subMap["agentID"].(string); ok {
			subscriber.AgentID = agentID
		}

		// Extract filter
		if filterMap, ok := subMap["filter"].(map[string]interface{}); ok {
			filter := EventFilter{}
			if filterType, ok := filterMap["type"].(string); ok {
				filter.Type = filterType
			}
			if priority, ok := filterMap["priority"].(string); ok {
				filter.Priority = priority
			}
			if metadataMap, ok := filterMap["metadata"].(map[string]interface{}); ok {
				filter.Metadata = make(map[string]string)
				for k, v := range metadataMap {
					if str, ok := v.(string); ok {
						filter.Metadata[k] = str
					}
				}
			}
			subscriber.Filter = filter
		}

		subscribers = append(subscribers, subscriber)
	}

	return subscribers
}
