// Package autoscaler provides autoscaling logic for Drones.
package autoscaler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// Executor executes scaling actions.
type Executor struct {
	store      apiary.ResourceStore
	launcher   apiary.AgentLauncher
	scheduler  apiary.Scheduler
	logger     *zap.Logger
	mu         sync.RWMutex
	scalingOps map[string]*ScalingOperation // key: namespace/agentspec
}

// ScalingOperation tracks an ongoing scaling operation.
type ScalingOperation struct {
	AgentSpec      string
	Namespace      string
	TargetReplicas int
	StartedAt      time.Time
	CompletedAt    *time.Time
}

// NewExecutor creates a new scaling executor.
func NewExecutor(store apiary.ResourceStore, launcher apiary.AgentLauncher, scheduler apiary.Scheduler, logger *zap.Logger) *Executor {
	return &Executor{
		store:      store,
		launcher:   launcher,
		scheduler:  scheduler,
		logger:     logger,
		scalingOps: make(map[string]*ScalingOperation),
	}
}

// ExecuteScaling executes a scaling decision.
func (e *Executor) ExecuteScaling(ctx context.Context, decision *ScalingDecision, spec *apiary.AgentSpec) error {
	key := fmt.Sprintf("%s/%s", decision.Namespace, decision.AgentSpec)
	
	// Check if scaling is already in progress
	e.mu.Lock()
	if op, exists := e.scalingOps[key]; exists && op.CompletedAt == nil {
		e.mu.Unlock()
		e.logger.Info("Scaling already in progress",
			zap.String("agentspec", decision.AgentSpec),
			zap.String("namespace", decision.Namespace),
		)
		return fmt.Errorf("scaling already in progress for %s/%s", decision.Namespace, decision.AgentSpec)
	}
	e.mu.Unlock()
	
	if decision.Action == ScalingActionNone {
		return nil // No action needed
	}
	
	// List current Drones
	drones, err := e.store.List(ctx, "Drone", decision.Namespace, apiary.Labels{
		"agentspec": decision.AgentSpec,
	})
	if err != nil {
		return fmt.Errorf("failed to list drones: %w", err)
	}
	
	currentReplicas := len(drones)
	
	e.logger.Info("Executing scaling decision",
		zap.String("agentspec", decision.AgentSpec),
		zap.String("namespace", decision.Namespace),
		zap.String("action", string(decision.Action)),
		zap.Int("currentReplicas", currentReplicas),
		zap.Int("desiredReplicas", decision.DesiredReplicas),
		zap.String("reason", decision.Reason),
	)
	
	// Track operation
	e.mu.Lock()
	e.scalingOps[key] = &ScalingOperation{
		AgentSpec:      decision.AgentSpec,
		Namespace:      decision.Namespace,
		TargetReplicas: decision.DesiredReplicas,
		StartedAt:      time.Now(),
	}
	e.mu.Unlock()
	
	var execErr error
	if decision.Action == ScalingActionUp {
		execErr = e.scaleUp(ctx, spec, currentReplicas, decision.DesiredReplicas)
	} else if decision.Action == ScalingActionDown {
		execErr = e.scaleDown(ctx, drones, currentReplicas, decision.DesiredReplicas)
	}
	
	// Mark operation as completed
	e.mu.Lock()
	if op, exists := e.scalingOps[key]; exists {
		now := time.Now()
		op.CompletedAt = &now
	}
	e.mu.Unlock()
	
	return execErr
}

// scaleUp scales up the number of replicas.
func (e *Executor) scaleUp(ctx context.Context, spec *apiary.AgentSpec, current, desired int) error {
	toCreate := desired - current
	
	e.logger.Info("Scaling up",
		zap.String("agentspec", spec.GetName()),
		zap.Int("toCreate", toCreate),
	)
	
	for i := 0; i < toCreate; i++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Get scheduling decision
		_, err := e.scheduler.Schedule(ctx, spec)
		if err != nil {
			return fmt.Errorf("scheduling failed: %w", err)
		}
		
		// Launch agent process
		process, err := e.launcher.Launch(ctx, spec)
		if err != nil {
			return fmt.Errorf("failed to launch agent: %w", err)
		}
		
		// Create Drone resource
		now := time.Now()
		drone := &apiary.Drone{
			TypeMeta: apiary.TypeMeta{
				APIVersion: "apiary.io/v1",
				Kind:       "Drone",
			},
			ObjectMeta: apiary.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", spec.GetName(), process.ID),
				Namespace: spec.GetNamespace(),
				UID:       process.ID,
				Labels: apiary.Labels{
					"agentspec": spec.GetName(),
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			Status: apiary.DroneStatus{
				Phase:     apiary.DronePhaseStarting,
				StartedAt: &now,
			},
			Spec: spec,
		}
		
		// Store Drone
		if err := e.store.Create(ctx, drone); err != nil {
			// Cleanup process if store fails
			_ = e.launcher.Stop(ctx, process.ID)
			return fmt.Errorf("failed to store drone: %w", err)
		}
		
		e.logger.Info("Created drone for scale up",
			zap.String("drone", drone.GetName()),
			zap.String("agentspec", spec.GetName()),
		)
	}
	
	return nil
}

// scaleDown scales down the number of replicas.
func (e *Executor) scaleDown(ctx context.Context, drones []apiary.Resource, current, desired int) error {
	toDelete := current - desired
	
	e.logger.Info("Scaling down",
		zap.Int("toDelete", toDelete),
	)
	
	deleted := 0
	for i := 0; i < len(drones) && deleted < toDelete; i++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		drone, ok := drones[i].(*apiary.Drone)
		if !ok {
			continue
		}
		
		// Stop the process
		if err := e.launcher.Stop(ctx, drone.GetUID()); err != nil {
			e.logger.Warn("Failed to stop process during scale down",
				zap.String("drone", drone.GetName()),
				zap.Error(err),
			)
		}
		
		// Delete from store
		if err := e.store.Delete(ctx, "Drone", drone.GetName(), drone.GetNamespace()); err != nil {
			e.logger.Warn("Failed to delete drone during scale down",
				zap.String("drone", drone.GetName()),
				zap.Error(err),
			)
			continue
		}
		
		deleted++
		e.logger.Info("Deleted drone for scale down",
			zap.String("drone", drone.GetName()),
		)
	}
	
	return nil
}

// IsScalingInProgress checks if scaling is in progress for an AgentSpec.
func (e *Executor) IsScalingInProgress(namespace, agentspec string) bool {
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	op, exists := e.scalingOps[key]
	return exists && op != nil && op.CompletedAt == nil
}
