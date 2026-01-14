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

// Hierarchical orchestrates a supervisor-worker hierarchical pattern.
type Hierarchical struct {
	hive         *apiary.Hive
	bus          nectar.MessageBus
	logger       *zap.Logger
	supervisorID string
	workerIDs    []string
	mu           sync.RWMutex
	state        HierarchicalState
	cancel       context.CancelFunc
	taskQueue    chan *Task
	results      map[string]*TaskResult
	subs         []nectar.Subscription
	resultExpiry time.Duration
	lastCleanup  time.Time
}

// Task represents a task to be delegated to workers.
type Task struct {
	ID        string
	Payload   *waggle.Message
	WorkerID  string
	AssignedAt time.Time
}

// TaskResult represents a result from a worker.
type TaskResult struct {
	TaskID    string
	WorkerID  string
	Result    *waggle.Message
	Error     error
	CompletedAt time.Time
}

// HierarchicalState represents the state of hierarchical execution.
type HierarchicalState struct {
	Status      HierarchicalStatus
	ActiveTasks int
	CompletedTasks int
	FailedTasks int
	StartedAt   time.Time
	CompletedAt *time.Time
}

// HierarchicalStatus represents the status of hierarchical execution.
type HierarchicalStatus string

const (
	HierarchicalStatusPending   HierarchicalStatus = "pending"
	HierarchicalStatusRunning   HierarchicalStatus = "running"
	HierarchicalStatusCompleted HierarchicalStatus = "completed"
	HierarchicalStatusFailed    HierarchicalStatus = "failed"
)

// NewHierarchical creates a new hierarchical orchestrator.
func NewHierarchical(hive *apiary.Hive, bus nectar.MessageBus, logger *zap.Logger) *Hierarchical {
	return &Hierarchical{
		hive:         hive,
		bus:          bus,
		logger:       logger,
		taskQueue:    make(chan *Task, 100),
		results:      make(map[string]*TaskResult),
		subs:         make([]nectar.Subscription, 0),
		resultExpiry: 1 * time.Hour, // Results expire after 1 hour
		lastCleanup:  time.Now(),
		state: HierarchicalState{
			Status:    HierarchicalStatusPending,
			StartedAt: time.Now(),
		},
	}
}

// Initialize initializes the hierarchical pattern from Hive spec.
func (h *Hierarchical) Initialize(supervisorID string, workerIDs []string) error {
	if h.hive.Spec.Pattern != "hierarchical" {
		return fmt.Errorf("hive pattern is not hierarchical: %s", h.hive.Spec.Pattern)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.supervisorID = supervisorID
	h.workerIDs = workerIDs

	return nil
}

// Start starts the hierarchical execution.
func (h *Hierarchical) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.state.Status != HierarchicalStatusPending {
		h.mu.Unlock()
		return fmt.Errorf("hierarchical already started")
	}
	h.state.Status = HierarchicalStatusRunning
	
	// Create cancel function for this execution
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	h.cancel = cancel
	h.mu.Unlock()

	// Subscribe to supervisor input (prefixed with Cell/namespace)
	namespace := h.hive.GetNamespace()
	baseSupervisorTopic := fmt.Sprintf("hierarchical.%s.supervisor.input", h.hive.GetName())
	supervisorTopic := nectar.PrefixTopic(namespace, baseSupervisorTopic)
	sub1, err := h.bus.Subscribe(ctx, supervisorTopic, func(ctx context.Context, busMsg *nectar.Message) error {
		return h.handleSupervisorInput(ctx, busMsg)
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to subscribe to supervisor input: %w", err)
	}

	// Subscribe to worker outputs (prefixed with Cell/namespace)
	baseWorkerOutputTopic := fmt.Sprintf("hierarchical.%s.worker.output", h.hive.GetName())
	workerOutputTopic := nectar.PrefixTopic(namespace, baseWorkerOutputTopic)
	sub2, err := h.bus.Subscribe(ctx, workerOutputTopic, func(ctx context.Context, busMsg *nectar.Message) error {
		return h.handleWorkerOutput(ctx, busMsg)
	})
	if err != nil {
		_ = sub1.Unsubscribe()
		cancel()
		return fmt.Errorf("failed to subscribe to worker output: %w", err)
	}

	// Store subscriptions
	h.mu.Lock()
	h.subs = append(h.subs, sub1, sub2)
	h.mu.Unlock()

	// Start task delegation goroutine
	go h.delegateTasks(ctx)

	// Start cleanup goroutine
	go h.cleanupResults(ctx)

	return nil
}

// handleSupervisorInput handles input from supervisor.
func (h *Hierarchical) handleSupervisorInput(ctx context.Context, busMsg *nectar.Message) error {
	waggleMsg, err := waggle.DecodeMessage(busMsg.Payload)
	if err != nil {
		return err
	}

	// Create task
	task := &Task{
		ID:         waggleMsg.ID,
		Payload:    waggleMsg,
		AssignedAt: time.Now(),
	}

	// Queue for delegation - increment counter before sending to prevent race condition
	h.mu.Lock()
	h.state.ActiveTasks++
	h.mu.Unlock()
	
	select {
	case h.taskQueue <- task:
		return nil
	case <-ctx.Done():
		// Decrement if failed to queue
		h.mu.Lock()
		h.state.ActiveTasks--
		h.mu.Unlock()
		return ctx.Err()
	}
}

// delegateTasks delegates tasks to workers using round-robin.
func (h *Hierarchical) delegateTasks(ctx context.Context) {
	workerIndex := 0

	for {
		select {
		case <-ctx.Done():
			return
		case task := <-h.taskQueue:
			// Select worker using round-robin
			h.mu.RLock()
			if len(h.workerIDs) == 0 {
				h.mu.RUnlock()
				h.logger.Error("No workers available")
				continue
			}
			workerID := h.workerIDs[workerIndex%len(h.workerIDs)]
			workerIndex++
			h.mu.RUnlock()

			// Assign task to worker
			task.WorkerID = workerID

			// Publish task to worker (prefixed with Cell/namespace)
			h.mu.RLock()
			namespace := h.hive.GetNamespace()
			h.mu.RUnlock()
			baseWorkerTopic := fmt.Sprintf("agent.%s.inbound", workerID)
			workerTopic := nectar.PrefixTopic(namespace, baseWorkerTopic)
			payload, err := task.Payload.Encode()
			if err != nil {
				h.logger.Error("Failed to encode task",
					zap.String("taskID", task.ID),
					zap.Error(err),
				)
				// Decrement counter on encode failure to prevent leak
				h.mu.Lock()
				h.state.ActiveTasks--
				h.state.FailedTasks++
				h.mu.Unlock()
				continue
			}

			busMsg := &nectar.Message{
				ID:        task.ID,
				Type:      string(task.Payload.Type),
				Payload:   payload,
				Metadata:  task.Payload.Metadata,
				Timestamp: task.Payload.Timestamp,
				SessionID: task.Payload.SessionID,
			}

			if err := h.bus.Publish(ctx, workerTopic, busMsg); err != nil {
				h.logger.Error("Failed to publish task to worker",
					zap.String("taskID", task.ID),
					zap.String("workerID", workerID),
					zap.Error(err),
				)
				// Decrement counter on publish failure to prevent leak
				h.mu.Lock()
				h.state.ActiveTasks--
				h.state.FailedTasks++
				h.mu.Unlock()
				continue
			}

			h.logger.Debug("Delegated task to worker",
				zap.String("taskID", task.ID),
				zap.String("workerID", workerID),
			)
		}
	}
}

// handleWorkerOutput handles output from workers.
func (h *Hierarchical) handleWorkerOutput(ctx context.Context, busMsg *nectar.Message) error {
	waggleMsg, err := waggle.DecodeMessage(busMsg.Payload)
	if err != nil {
		return err
	}

	// Find corresponding task (using correlation ID or message ID)
	taskID := waggleMsg.CorrelationID
	if taskID == "" {
		taskID = waggleMsg.ID
	}

	// Store result
	h.mu.Lock()
	h.results[taskID] = &TaskResult{
		TaskID:      taskID,
		WorkerID:    waggleMsg.SourceAgentID,
		Result:      waggleMsg,
		CompletedAt: time.Now(),
	}
	h.state.ActiveTasks--
	h.state.CompletedTasks++
	h.mu.Unlock()

	// Send result back to supervisor (prefixed with Cell/namespace)
	h.mu.RLock()
	namespace := h.hive.GetNamespace()
	h.mu.RUnlock()
	baseSupervisorTopic := fmt.Sprintf("agent.%s.inbound", h.supervisorID)
	supervisorTopic := nectar.PrefixTopic(namespace, baseSupervisorTopic)
	resultPayload, err := waggleMsg.Encode()
	if err != nil {
		return err
	}

	resultBusMsg := &nectar.Message{
		ID:        waggleMsg.ID,
		Type:      string(waggleMsg.Type),
		Payload:   resultPayload,
		Metadata:  waggleMsg.Metadata,
		Timestamp: waggleMsg.Timestamp,
		SessionID: waggleMsg.SessionID,
	}

	return h.bus.Publish(ctx, supervisorTopic, resultBusMsg)
}

// GetState returns the current hierarchical state.
func (h *Hierarchical) GetState() HierarchicalState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state
}

// GetResult retrieves a task result.
func (h *Hierarchical) GetResult(taskID string) (*TaskResult, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result, exists := h.results[taskID]
	return result, exists
}

// Stop stops the hierarchical execution.
func (h *Hierarchical) Stop() {
	h.mu.Lock()
	subs := make([]nectar.Subscription, len(h.subs))
	copy(subs, h.subs)
	h.subs = make([]nectar.Subscription, 0)
	cancel := h.cancel
	if h.state.Status == HierarchicalStatusRunning {
		h.state.Status = HierarchicalStatusFailed
	}
	h.mu.Unlock()
	
	// Cancel context
	if cancel != nil {
		cancel()
	}
	
	// Unsubscribe all subscriptions
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
}

// cleanupResults periodically cleans up old results to prevent unbounded growth.
func (h *Hierarchical) cleanupResults(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.mu.Lock()
			now := time.Now()
			
			// Clean up results older than resultExpiry
			for taskID, result := range h.results {
				if now.Sub(result.CompletedAt) > h.resultExpiry {
					delete(h.results, taskID)
				}
			}
			
			h.lastCleanup = now
			h.mu.Unlock()
		}
	}
}
