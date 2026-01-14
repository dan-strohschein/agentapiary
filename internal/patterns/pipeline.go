// Package patterns provides orchestration patterns for agent coordination.
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

// Pipeline orchestrates a sequential pipeline of agents.
type Pipeline struct {
	hive      *apiary.Hive
	bus       nectar.MessageBus
	logger    *zap.Logger
	stages    []*PipelineStage
	mu        sync.RWMutex
	state     PipelineState
	cancel    context.CancelFunc
}

// PipelineStage represents a stage in the pipeline.
type PipelineStage struct {
	Name      string
	AgentRef  string
	Replicas  int
	StageNum  int
	droneIDs  []string
	completed bool
	mu        sync.RWMutex
}

// PipelineState represents the state of a pipeline execution.
type PipelineState struct {
	CurrentStage int
	Status       PipelineStatus
	Error        error
	StartedAt    time.Time
	CompletedAt  *time.Time
}

// PipelineStatus represents the status of a pipeline.
type PipelineStatus string

const (
	PipelineStatusPending   PipelineStatus = "pending"
	PipelineStatusRunning   PipelineStatus = "running"
	PipelineStatusCompleted PipelineStatus = "completed"
	PipelineStatusFailed    PipelineStatus = "failed"
	PipelineStatusAborted   PipelineStatus = "aborted"
)

// NewPipeline creates a new pipeline orchestrator.
func NewPipeline(hive *apiary.Hive, bus nectar.MessageBus, logger *zap.Logger) *Pipeline {
	return &Pipeline{
		hive:   hive,
		bus:    bus,
		logger: logger,
		state: PipelineState{
			Status:    PipelineStatusPending,
			StartedAt: time.Now(),
		},
	}
}

// Initialize initializes the pipeline from Hive spec.
func (p *Pipeline) Initialize() error {
	if p.hive.Spec.Pattern != "pipeline" {
		return fmt.Errorf("hive pattern is not pipeline: %s", p.hive.Spec.Pattern)
	}

	if len(p.hive.Spec.Stages) == 0 {
		return fmt.Errorf("pipeline must have at least one stage")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.stages = make([]*PipelineStage, len(p.hive.Spec.Stages))
	for i, stageSpec := range p.hive.Spec.Stages {
		p.stages[i] = &PipelineStage{
			Name:     stageSpec.Name,
			AgentRef: stageSpec.AgentRef,
			Replicas: stageSpec.Replicas,
			StageNum: i,
			droneIDs: make([]string, 0),
		}
	}

	return nil
}

// Start starts the pipeline execution.
func (p *Pipeline) Start(ctx context.Context, input *waggle.Message) error {
	p.mu.Lock()
	if p.state.Status != PipelineStatusPending {
		p.mu.Unlock()
		return fmt.Errorf("pipeline already started")
	}
	p.state.Status = PipelineStatusRunning
	p.state.CurrentStage = 0
	
	// Create cancel function from parent context
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	p.cancel = cancel
	p.mu.Unlock()

	// Start first stage with input
	return p.executeStage(ctx, 0, input)
}

// executeStage executes a pipeline stage.
func (p *Pipeline) executeStage(ctx context.Context, stageNum int, input *waggle.Message) error {
	if stageNum >= len(p.stages) {
		// Pipeline complete
		p.mu.Lock()
		now := time.Now()
		p.state.Status = PipelineStatusCompleted
		p.state.CompletedAt = &now
		p.mu.Unlock()

		p.logger.Info("Pipeline completed",
			zap.String("hive", p.hive.GetName()),
		)
		return nil
	}

	stage := p.stages[stageNum]
	p.logger.Info("Executing pipeline stage",
		zap.String("hive", p.hive.GetName()),
		zap.Int("stage", stageNum),
		zap.String("stageName", stage.Name),
		zap.String("agentRef", stage.AgentRef),
	)

	// Update current stage
	p.mu.Lock()
	p.state.CurrentStage = stageNum
	p.mu.Unlock()

	// Subscribe to stage output (prefixed with Cell/namespace)
	p.mu.RLock()
	namespace := p.hive.GetNamespace()
	p.mu.RUnlock()
	
	baseOutputTopic := fmt.Sprintf("pipeline.%s.stage.%d.output", p.hive.GetName(), stageNum)
	outputTopic := nectar.PrefixTopic(namespace, baseOutputTopic)
	outputCh := make(chan *waggle.Message, 1)
	
	sub, err := p.bus.Subscribe(ctx, outputTopic, func(ctx context.Context, busMsg *nectar.Message) error {
		waggleMsg, err := waggle.DecodeMessage(busMsg.Payload)
		if err != nil {
			return err
		}
		select {
		case outputCh <- waggleMsg:
		default:
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to stage output: %w", err)
	}
	defer sub.Unsubscribe()

	// Publish input to stage input topic (prefixed with Cell/namespace)
	baseInputTopic := fmt.Sprintf("pipeline.%s.stage.%d.input", p.hive.GetName(), stageNum)
	inputTopic := nectar.PrefixTopic(namespace, baseInputTopic)
	inputPayload, err := input.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode input: %w", err)
	}

	busMsg := &nectar.Message{
		ID:        input.ID,
		Type:      string(input.Type),
		Payload:   inputPayload,
		Metadata:  input.Metadata,
		Timestamp: input.Timestamp,
		SessionID: input.SessionID,
	}

	if err := p.bus.Publish(ctx, inputTopic, busMsg); err != nil {
		return fmt.Errorf("failed to publish to stage: %w", err)
	}

	// Wait for output with timeout
	timeout := 5 * time.Minute // Default timeout per stage

	select {
	case output := <-outputCh:
		// Mark stage as completed
		stage.mu.Lock()
		stage.completed = true
		stage.mu.Unlock()

		// Execute next stage with output as input
		return p.executeStage(ctx, stageNum+1, output)
	case <-time.After(timeout):
		p.mu.Lock()
		p.state.Status = PipelineStatusFailed
		p.state.Error = fmt.Errorf("stage %d timed out", stageNum)
		p.mu.Unlock()
		return fmt.Errorf("stage %d timed out after %v", stageNum, timeout)
	case <-ctx.Done():
		p.mu.Lock()
		p.state.Status = PipelineStatusAborted
		p.mu.Unlock()
		return ctx.Err()
	}
}

// GetState returns the current pipeline state.
func (p *Pipeline) GetState() PipelineState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// Stop stops the pipeline execution.
func (p *Pipeline) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	if p.state.Status == PipelineStatusRunning {
		p.state.Status = PipelineStatusAborted
	}
	p.mu.Unlock()
	
	// Cancel context
	if cancel != nil {
		cancel()
	}
}
