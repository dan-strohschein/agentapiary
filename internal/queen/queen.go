// Package queen provides the central orchestrator for Apiary.
package queen

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/internal/audit"
	"github.com/agentapiary/apiary/internal/autoscaler"
	"github.com/agentapiary/apiary/internal/circuitbreaker"
	"github.com/agentapiary/apiary/internal/warmpool"
	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// Queen is the central orchestrator that reconciles desired state with actual state.
type Queen struct {
	store         apiary.ResourceStore
	scheduler     apiary.Scheduler
	launcher      apiary.AgentLauncher
	autoscaler    *autoscaler.Autoscaler
	warmPool      *warmpool.Manager
	logger        *zap.Logger
	auditLogger   *audit.Logger // Audit logger for structured logging
	droneReg      *droneRegistry
	circuitBreaker *circuitbreaker.Manager
	reconcileCh   chan struct{}
	stopCh        chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
}

// Config holds Queen configuration.
type Config struct {
	Store      apiary.ResourceStore
	Scheduler  apiary.Scheduler
	Launcher   apiary.AgentLauncher
	Autoscaler *autoscaler.Autoscaler
	WarmPool   *warmpool.Manager
	Logger     *zap.Logger
	// EtcdEndpoints is a comma-separated list of etcd endpoints for leader election.
	// If empty, leader election is disabled (single-instance mode).
	// Example: "http://etcd1:2379,http://etcd2:2379,http://etcd3:2379"
	EtcdEndpoints string
	// LeaderKey is the etcd key used for leader election. Defaults to "/apiary/leader" if empty.
	LeaderKey string
	// LeaderLeaseTTL is the TTL for the leader lease in seconds. Defaults to 30 if zero.
	LeaderLeaseTTL int
}

// NewQueen creates a new Queen orchestrator.
func NewQueen(cfg Config) *Queen {
	return &Queen{
		store:          cfg.Store,
		scheduler:      cfg.Scheduler,
		launcher:       cfg.Launcher,
		autoscaler:     cfg.Autoscaler,
		warmPool:       cfg.WarmPool,
		logger:         cfg.Logger,
		auditLogger:    audit.NewLogger(cfg.Logger), // Initialize audit logger
		droneReg:       newDroneRegistry(),
		circuitBreaker: circuitbreaker.NewManager(circuitbreaker.Config{}),
		reconcileCh:    make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
	}
}

// Start starts the Queen and begins reconciliation.
func (q *Queen) Start(ctx context.Context) error {
	q.logger.Info("Queen starting")

	// Start warm pool manager if available
	if q.warmPool != nil {
		if err := q.warmPool.Start(ctx); err != nil {
			return fmt.Errorf("failed to start warm pool manager: %w", err)
		}
	}

	// Start watch for AgentSpecs
	q.wg.Add(1)
	go q.watchAgentSpecs(ctx)

	// Start reconciliation loop
	q.wg.Add(1)
	go q.reconciliationLoop(ctx)

	// Trigger initial reconciliation
	select {
	case q.reconcileCh <- struct{}{}:
	default:
	}

	q.logger.Info("Queen started")
	return nil
}

// Stop gracefully stops the Queen.
func (q *Queen) Stop(ctx context.Context) error {
	q.logger.Info("Queen stopping")
	close(q.stopCh)
	
	// Stop warm pool manager if available
	if q.warmPool != nil {
		q.warmPool.Stop()
	}
	
	// Wait for goroutines to finish
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		q.logger.Info("Queen stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Reconcile triggers immediate reconciliation of all resources.
func (q *Queen) Reconcile(ctx context.Context) error {
	select {
	case q.reconcileCh <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetDroneStatus returns the current status of a Drone.
func (q *Queen) GetDroneStatus(ctx context.Context, droneID string) (*apiary.DroneStatus, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	drone, err := q.store.Get(ctx, "Drone", droneID, "")
	if err != nil {
		return nil, err
	}

	d, ok := drone.(*apiary.Drone)
	if !ok {
		return nil, fmt.Errorf("resource is not a Drone")
	}

	return &d.Status, nil
}

// ListDrones returns all Drones matching the selector.
// Drones with open circuit breakers are excluded from the result.
func (q *Queen) ListDrones(ctx context.Context, namespace string, selector apiary.Labels) ([]*apiary.Drone, error) {
	resources, err := q.store.List(ctx, "Drone", namespace, selector)
	if err != nil {
		return nil, err
	}

	drones := make([]*apiary.Drone, 0, len(resources))
	for _, r := range resources {
		if drone, ok := r.(*apiary.Drone); ok {
			// Filter out Drones with open circuit breakers
			if q.circuitBreaker.IsOpen(drone.GetUID()) {
				q.logger.Debug("Skipping drone with open circuit breaker",
					zap.String("droneID", drone.GetUID()),
					zap.String("name", drone.GetName()),
				)
				continue
			}
			drones = append(drones, drone)
		}
	}

	return drones, nil
}

// watchAgentSpecs watches for AgentSpec changes and triggers reconciliation.
func (q *Queen) watchAgentSpecs(ctx context.Context) {
	defer q.wg.Done()

	// Watch all namespaces
	eventCh, err := q.store.Watch(ctx, "AgentSpec", "")
	if err != nil {
		q.logger.Error("Failed to watch AgentSpecs", zap.Error(err))
		return
	}

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			q.logger.Info("AgentSpec event", 
				zap.String("type", string(event.Type)),
				zap.String("name", event.Resource.GetName()),
			)
			// Trigger reconciliation
			select {
			case q.reconcileCh <- struct{}{}:
			default:
			}
		case <-q.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// reconciliationLoop continuously reconciles desired vs actual state.
func (q *Queen) reconciliationLoop(ctx context.Context) {
	defer q.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.reconcileCh:
			q.performReconciliation(ctx)
		case <-ticker.C:
			q.performReconciliation(ctx)
		case <-q.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// performReconciliation performs a single reconciliation pass.
func (q *Queen) performReconciliation(ctx context.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// List all Hives to check for warm pool requirements
	if q.warmPool != nil {
		hives, err := q.store.List(ctx, "Hive", "", nil)
		if err == nil {
			for _, resource := range hives {
				if hive, ok := resource.(*apiary.Hive); ok {
					q.reconcileHiveWarmPool(ctx, hive)
				}
			}
		}
	}

	// List all AgentSpecs
	agentSpecs, err := q.store.List(ctx, "AgentSpec", "", nil)
	if err != nil {
		q.logger.Error("Failed to list AgentSpecs", zap.Error(err))
		return
	}

	// Reconcile each AgentSpec
	for _, resource := range agentSpecs {
		spec, ok := resource.(*apiary.AgentSpec)
		if !ok {
			continue
		}

		if err := q.reconcileAgentSpec(ctx, spec); err != nil {
			q.logger.Error("Failed to reconcile AgentSpec",
				zap.String("name", spec.GetName()),
				zap.Error(err),
			)
		}
	}
}

// reconcileHiveWarmPool reconciles warm pools for Hives.
func (q *Queen) reconcileHiveWarmPool(ctx context.Context, hive *apiary.Hive) {
	if q.warmPool == nil {
		return
	}
	
	// Check if Hive has warm pool configuration
	if hive.Spec.PoolMode != "warm" && hive.Spec.PoolMode != "hybrid" {
		return
	}
	
	warmPoolSize := hive.Spec.WarmPoolSize
	if warmPoolSize == 0 {
		warmPoolSize = 1 // Default to 1
	}
	
	// For each stage in the Hive, ensure warm pool
	for _, stage := range hive.Spec.Stages {
		// Get AgentSpec for this stage
		spec, err := q.store.Get(ctx, "AgentSpec", stage.AgentRef, hive.GetNamespace())
		if err != nil {
			q.logger.Debug("Failed to get AgentSpec for warm pool",
				zap.String("agentspec", stage.AgentRef),
				zap.String("namespace", hive.GetNamespace()),
				zap.Error(err),
			)
			continue
		}
		
		agentSpec, ok := spec.(*apiary.AgentSpec)
		if !ok {
			continue
		}
		
		// Ensure warm pool for this AgentSpec
		if err := q.warmPool.EnsurePool(ctx, agentSpec, warmPoolSize); err != nil {
			q.logger.Warn("Failed to ensure warm pool",
				zap.String("agentspec", stage.AgentRef),
				zap.String("namespace", hive.GetNamespace()),
				zap.Error(err),
			)
		}
	}
}

// reconcileAgentSpec reconciles a single AgentSpec.
func (q *Queen) reconcileAgentSpec(ctx context.Context, spec *apiary.AgentSpec) error {
	// List existing Drones for this AgentSpec
	drones, err := q.store.List(ctx, "Drone", spec.GetNamespace(), apiary.Labels{
		"agentspec": spec.GetName(),
	})
	if err != nil {
		return fmt.Errorf("failed to list drones: %w", err)
	}

	currentReplicas := len(drones)
	
	// Determine desired replica count using autoscaler if available
	var desiredReplicas int
	if q.autoscaler != nil && (spec.Spec.Scaling.MinReplicas > 0 || spec.Spec.Scaling.MaxReplicas > 0 || spec.Spec.Scaling.TargetLatencyMs > 0) {
		// Use autoscaler to determine desired replicas
		if err := q.autoscaler.EvaluateAndScale(ctx, spec, currentReplicas); err != nil {
			q.logger.Warn("Autoscaling evaluation failed, using static replicas",
				zap.String("agentspec", spec.GetName()),
				zap.Error(err),
			)
		}
		
		// Re-list drones to get updated count after autoscaling
		updatedDrones, err := q.store.List(ctx, "Drone", spec.GetNamespace(), apiary.Labels{
			"agentspec": spec.GetName(),
		})
		if err == nil {
			currentReplicas = len(updatedDrones)
			drones = updatedDrones
		}
		
		// Use current replicas as desired (autoscaler already made changes)
		desiredReplicas = currentReplicas
	} else {
		// Fallback to static min replicas
		desiredReplicas = 1
		if spec.Spec.Scaling.MinReplicas > 0 {
			desiredReplicas = spec.Spec.Scaling.MinReplicas
		}
	}

	// Create Drones if needed
	if currentReplicas < desiredReplicas {
		toCreate := desiredReplicas - currentReplicas
		// Audit log scaling up
		if q.auditLogger != nil {
			q.auditLogger.LogScalingUp(
				spec.GetNamespace(),
				spec.GetName(),
				currentReplicas,
				desiredReplicas,
				map[string]interface{}{
					"reason": "reconciliation",
				},
			)
		}
		for i := 0; i < toCreate; i++ {
			if err := q.createDrone(ctx, spec); err != nil {
				q.logger.Error("Failed to create drone", zap.Error(err))
				// Continue with other drones
			}
		}
	}

	// Delete excess Drones
	if currentReplicas > desiredReplicas {
		toDelete := currentReplicas - desiredReplicas
		// Audit log scaling down
		if q.auditLogger != nil {
			q.auditLogger.LogScalingDown(
				spec.GetNamespace(),
				spec.GetName(),
				currentReplicas,
				desiredReplicas,
				map[string]interface{}{
					"reason": "reconciliation",
				},
			)
		}
		for i := 0; i < toDelete && i < len(drones); i++ {
			drone, ok := drones[i].(*apiary.Drone)
			if !ok {
				q.logger.Error("Resource is not a Drone, skipping",
					zap.String("kind", drones[i].GetKind()),
					zap.String("name", drones[i].GetName()),
				)
				continue
			}
			if err := q.deleteDrone(ctx, drone); err != nil {
				q.logger.Error("Failed to delete drone", zap.Error(err))
			}
		}
	}

	return nil
}

// createDrone creates a new Drone for an AgentSpec.
func (q *Queen) createDrone(ctx context.Context, spec *apiary.AgentSpec) error {
	// Check resource quota for Drones in the namespace
	// Get the Cell to check quota
	cell, err := q.store.Get(ctx, "Cell", spec.GetNamespace(), "")
	if err == nil {
		if cellResource, ok := cell.(*apiary.Cell); ok {
			quota := cellResource.Spec.ResourceQuota
			if quota.MaxTotalDrones > 0 {
				drones, err := q.store.List(ctx, "Drone", spec.GetNamespace(), nil)
				if err == nil && len(drones) >= quota.MaxTotalDrones {
					return fmt.Errorf("quota exceeded: max total drones (%d) reached in cell %s", quota.MaxTotalDrones, spec.GetNamespace())
				}
			}
		}
	}

	// Get scheduling decision
	_, err = q.scheduler.Schedule(ctx, spec)
	if err != nil {
		return fmt.Errorf("scheduling failed: %w", err)
	}

	// Launch agent process
	process, err := q.launcher.Launch(ctx, spec)
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
	if err := q.store.Create(ctx, drone); err != nil {
		// Cleanup process if store fails
		stopErr := q.launcher.Stop(ctx, process.ID)
		if stopErr != nil {
			q.logger.Error("Failed to cleanup process after store failure",
				zap.String("processID", process.ID),
				zap.Error(stopErr),
			)
			// Continue to return the original error, but log the cleanup failure
		}
		return fmt.Errorf("failed to store drone: %w", err)
	}

	// Register in drone registry
	q.droneReg.add(drone.GetUID(), process)

	// Audit log resource creation
	if q.auditLogger != nil {
		q.auditLogger.LogResourceCreate(
			"Drone",
			drone.GetName(),
			drone.GetNamespace(),
			map[string]interface{}{
				"agentSpec": spec.GetName(),
				"uid":       drone.GetUID(),
			},
		)
	}

	q.logger.Info("Created drone",
		zap.String("name", drone.GetName()),
		zap.String("agentspec", spec.GetName()),
	)

	return nil
}

// deleteDrone deletes a Drone.
func (q *Queen) deleteDrone(ctx context.Context, drone *apiary.Drone) error {
	// Stop the process
	if process := q.droneReg.get(drone.GetUID()); process != nil {
		if err := q.launcher.Stop(ctx, process.ID); err != nil {
			q.logger.Warn("Failed to stop process", zap.Error(err))
		}
		q.droneReg.remove(drone.GetUID())
	}

	// Remove circuit breaker for this Drone
	q.circuitBreaker.Remove(drone.GetUID())

	// Delete from store
	if err := q.store.Delete(ctx, "Drone", drone.GetName(), drone.GetNamespace()); err != nil {
		return fmt.Errorf("failed to delete drone: %w", err)
	}

	// Audit log resource deletion
	if q.auditLogger != nil {
		q.auditLogger.LogResourceDelete(
			"Drone",
			drone.GetName(),
			drone.GetNamespace(),
			map[string]interface{}{
				"agentSpec": drone.Spec.GetName(),
				"uid":       drone.GetUID(),
			},
		)
	}

	q.logger.Info("Deleted drone", zap.String("name", drone.GetName()))
	return nil
}

// droneRegistry tracks running drone processes.
type droneRegistry struct {
	mu       sync.RWMutex
	processes map[string]*apiary.AgentProcess
}

func newDroneRegistry() *droneRegistry {
	return &droneRegistry{
		processes: make(map[string]*apiary.AgentProcess),
	}
}

func (r *droneRegistry) add(id string, process *apiary.AgentProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processes[id] = process
}

func (r *droneRegistry) get(id string) *apiary.AgentProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.processes[id]
}

func (r *droneRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.processes, id)
}
