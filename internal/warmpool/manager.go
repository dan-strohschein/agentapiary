// Package warmpool provides warm pool management for Drones.
package warmpool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// Manager manages warm pools of Drones.
type Manager struct {
	store     apiary.ResourceStore
	launcher  apiary.AgentLauncher
	scheduler apiary.Scheduler
	logger    *zap.Logger
	
	mu            sync.RWMutex
	warmPools     map[string]*WarmPool // key: namespace/agentspec
	idleTimeout   time.Duration
	checkInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// WarmPool represents a warm pool for an AgentSpec.
type WarmPool struct {
	AgentSpec      string
	Namespace      string
	MinSize        int
	CurrentSize    int
	WarmDrones     []*apiary.Drone
	LastUsed       time.Time
	mu             sync.RWMutex
}

// Config holds warm pool manager configuration.
type Config struct {
	Store         apiary.ResourceStore
	Launcher      apiary.AgentLauncher
	Scheduler     apiary.Scheduler
	IdleTimeout   time.Duration
	CheckInterval time.Duration
	Logger        *zap.Logger
}

// NewManager creates a new warm pool manager.
func NewManager(cfg Config) *Manager {
	idleTimeout := cfg.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = 30 * time.Minute // Default: 30 minutes
	}
	
	checkInterval := cfg.CheckInterval
	if checkInterval == 0 {
		checkInterval = 5 * time.Minute // Default: check every 5 minutes
	}
	
	return &Manager{
		store:         cfg.Store,
		launcher:      cfg.Launcher,
		scheduler:     cfg.Scheduler,
		logger:        cfg.Logger,
		warmPools:     make(map[string]*WarmPool),
		idleTimeout:   idleTimeout,
		checkInterval: checkInterval,
		stopCh:        make(chan struct{}),
	}
}

// Start starts the warm pool manager.
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Warm pool manager starting")
	
	// Start background goroutine to maintain pools and cleanup idle ones
	m.wg.Add(1)
	go m.maintenanceLoop(ctx)
	
	m.logger.Info("Warm pool manager started")
	return nil
}

// Stop stops the warm pool manager.
func (m *Manager) Stop() {
	m.logger.Info("Warm pool manager stopping")
	close(m.stopCh)
	m.wg.Wait()
	m.logger.Info("Warm pool manager stopped")
}

// EnsurePool ensures a warm pool exists for an AgentSpec with the specified minimum size.
func (m *Manager) EnsurePool(ctx context.Context, spec *apiary.AgentSpec, minSize int) error {
	key := fmt.Sprintf("%s/%s", spec.GetNamespace(), spec.GetName())
	
	m.mu.Lock()
	pool, exists := m.warmPools[key]
	if !exists {
		pool = &WarmPool{
			AgentSpec:   spec.GetName(),
			Namespace:   spec.GetNamespace(),
			MinSize:     minSize,
			WarmDrones:  make([]*apiary.Drone, 0),
			LastUsed:    time.Now(),
		}
		m.warmPools[key] = pool
	} else {
		pool.MinSize = minSize
		pool.LastUsed = time.Now()
	}
	m.mu.Unlock()
	
	// Ensure pool is filled to minimum size
	return m.fillPool(ctx, pool, spec, minSize)
}

// GetWarmDrone gets a warm Drone from the pool, or returns nil if none available.
func (m *Manager) GetWarmDrone(ctx context.Context, namespace, agentspec string) (*apiary.Drone, error) {
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	
	m.mu.RLock()
	pool, exists := m.warmPools[key]
	m.mu.RUnlock()
	
	if !exists {
		return nil, nil // No warm pool for this AgentSpec
	}
	
	pool.mu.Lock()
	defer pool.mu.Unlock()
	
	if len(pool.WarmDrones) == 0 {
		return nil, nil // No warm Drones available
	}
	
	// Get the first warm Drone
	drone := pool.WarmDrones[0]
	pool.WarmDrones = pool.WarmDrones[1:]
	pool.CurrentSize = len(pool.WarmDrones)
	pool.LastUsed = time.Now()
	
	// Verify the Drone still exists and is running
	existing, err := m.store.Get(ctx, "Drone", drone.GetName(), namespace)
	if err != nil {
		m.logger.Warn("Warm Drone no longer exists",
			zap.String("drone", drone.GetName()),
			zap.Error(err),
		)
		return nil, nil
	}
	
	existingDrone, ok := existing.(*apiary.Drone)
	if !ok || existingDrone.Status.Phase != apiary.DronePhaseRunning {
		m.logger.Warn("Warm Drone is not in running state",
			zap.String("drone", drone.GetName()),
			zap.String("phase", string(existingDrone.Status.Phase)),
		)
		return nil, nil
	}
	
	m.logger.Info("Retrieved warm Drone from pool",
		zap.String("drone", drone.GetName()),
		zap.String("agentspec", agentspec),
		zap.Int("remainingInPool", pool.CurrentSize),
	)
	
	return existingDrone, nil
}

// ReturnDrone returns a Drone to the warm pool if it's still warm and pool needs it.
func (m *Manager) ReturnDrone(ctx context.Context, drone *apiary.Drone, spec *apiary.AgentSpec) {
	key := fmt.Sprintf("%s/%s", drone.GetNamespace(), spec.GetName())
	
	m.mu.RLock()
	pool, exists := m.warmPools[key]
	m.mu.RUnlock()
	
	if !exists {
		return // No warm pool for this AgentSpec
	}
	
	// Check if Drone is still in good state
	existing, err := m.store.Get(ctx, "Drone", drone.GetName(), drone.GetNamespace())
	if err != nil {
		return // Drone no longer exists
	}
	
	existingDrone, ok := existing.(*apiary.Drone)
	if !ok || existingDrone.Status.Phase != apiary.DronePhaseRunning {
		return // Drone is not in running state
	}
	
	pool.mu.Lock()
	defer pool.mu.Unlock()
	
	// Only return if pool is below min size
	if pool.CurrentSize < pool.MinSize {
		pool.WarmDrones = append(pool.WarmDrones, existingDrone)
		pool.CurrentSize = len(pool.WarmDrones)
		pool.LastUsed = time.Now()
		
		m.logger.Debug("Returned Drone to warm pool",
			zap.String("drone", drone.GetName()),
			zap.Int("poolSize", pool.CurrentSize),
		)
	}
}

// fillPool fills the warm pool to the minimum size.
func (m *Manager) fillPool(ctx context.Context, pool *WarmPool, spec *apiary.AgentSpec, minSize int) error {
	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Check current size and calculate how many to create
		pool.mu.Lock()
		currentSize := pool.CurrentSize
		pool.mu.Unlock()
		
		if currentSize >= minSize {
			return nil // Pool is already filled
		}
		
		toCreate := minSize - currentSize
		
		// Limit concurrent creation to avoid overwhelming the system
		batchSize := 1
		if toCreate > 1 {
			batchSize = 1 // Create one at a time to check size after each
		}
		
		m.logger.Info("Filling warm pool",
			zap.String("agentspec", spec.GetName()),
			zap.String("namespace", spec.GetNamespace()),
			zap.Int("currentSize", currentSize),
			zap.Int("minSize", minSize),
			zap.Int("toCreate", toCreate),
		)
		
		for i := 0; i < batchSize; i++ {
			// Check context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			
			// Re-check size before creating (another goroutine might have filled it)
			pool.mu.Lock()
			currentSize = pool.CurrentSize
			if currentSize >= minSize {
				pool.mu.Unlock()
				return nil // Pool is already filled
			}
			pool.mu.Unlock()
			
			// Get scheduling decision
			_, err := m.scheduler.Schedule(ctx, spec)
			if err != nil {
				return fmt.Errorf("scheduling failed: %w", err)
			}
			
			// Launch agent process
			process, err := m.launcher.Launch(ctx, spec)
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
					Name:      fmt.Sprintf("%s-warm-%s", spec.GetName(), process.ID),
					Namespace: spec.GetNamespace(),
					UID:       process.ID,
					Labels: apiary.Labels{
						"agentspec": spec.GetName(),
						"warmpool":  "true",
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
			if err := m.store.Create(ctx, drone); err != nil {
				// Cleanup process if store fails
				_ = m.launcher.Stop(ctx, process.ID)
				return fmt.Errorf("failed to store drone: %w", err)
			}
			
			// Wait a bit for Drone to become ready (in production, would wait for health check)
			time.Sleep(2 * time.Second)
			
			// Update Drone status to Running
			drone.Status.Phase = apiary.DronePhaseRunning
			if err := m.store.Update(ctx, drone); err != nil {
				m.logger.Warn("Failed to update Drone status to Running",
					zap.String("drone", drone.GetName()),
					zap.Error(err),
				)
			}
			
			// Add to pool only if we haven't exceeded minSize
			pool.mu.Lock()
			if pool.CurrentSize < minSize {
				pool.WarmDrones = append(pool.WarmDrones, drone)
				pool.CurrentSize = len(pool.WarmDrones)
				m.logger.Info("Created warm Drone for pool",
					zap.String("drone", drone.GetName()),
					zap.String("agentspec", spec.GetName()),
					zap.Int("poolSize", pool.CurrentSize),
				)
			} else {
				// Pool was filled by another goroutine, cleanup this Drone
				pool.mu.Unlock()
				_ = m.launcher.Stop(ctx, process.ID)
				_ = m.store.Delete(ctx, "Drone", drone.GetName(), drone.GetNamespace())
				return nil
			}
			pool.mu.Unlock()
		}
	}
}

// maintenanceLoop maintains warm pools and cleans up idle ones.
func (m *Manager) maintenanceLoop(ctx context.Context) {
	defer m.wg.Done()
	
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.maintainPools(ctx)
			m.cleanupIdlePools(ctx)
		}
	}
}

// maintainPools ensures all pools are filled to their minimum size.
func (m *Manager) maintainPools(ctx context.Context) {
	m.mu.RLock()
	pools := make([]*WarmPool, 0, len(m.warmPools))
	for _, pool := range m.warmPools {
		pools = append(pools, pool)
	}
	m.mu.RUnlock()
	
	for _, pool := range pools {
		// Get AgentSpec
		spec, err := m.store.Get(ctx, "AgentSpec", pool.AgentSpec, pool.Namespace)
		if err != nil {
			m.logger.Warn("Failed to get AgentSpec for pool maintenance",
				zap.String("agentspec", pool.AgentSpec),
				zap.String("namespace", pool.Namespace),
				zap.Error(err),
			)
			continue
		}
		
		agentSpec, ok := spec.(*apiary.AgentSpec)
		if !ok {
			continue
		}
		
		// Fill pool if needed
		pool.mu.RLock()
		currentSize := pool.CurrentSize
		minSize := pool.MinSize
		pool.mu.RUnlock()
		
		if currentSize < minSize {
			if err := m.fillPool(ctx, pool, agentSpec, minSize); err != nil {
				m.logger.Warn("Failed to fill warm pool",
					zap.String("agentspec", pool.AgentSpec),
					zap.Error(err),
				)
			}
		}
	}
}

// cleanupIdlePools removes idle warm pools and scales down their Drones.
func (m *Manager) cleanupIdlePools(ctx context.Context) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return
	default:
	}
	
	now := time.Now()
	type poolCleanupInfo struct {
		key      string
		pool     *WarmPool
		toRemove int
		remove   bool
	}
	
	var poolsToCleanup []poolCleanupInfo
	
	// Identify pools that need cleanup (read-only, release lock quickly)
	m.mu.RLock()
	for key, pool := range m.warmPools {
		pool.mu.RLock()
		idleTime := now.Sub(pool.LastUsed)
		poolSize := pool.CurrentSize
		minSize := pool.MinSize
		pool.mu.RUnlock()
		
		// If pool has been idle for too long and is above min size, scale it down
		if idleTime > m.idleTimeout && poolSize > minSize {
			poolsToCleanup = append(poolsToCleanup, poolCleanupInfo{
				key:      key,
				pool:     pool,
				toRemove: poolSize - minSize,
				remove:   false,
			})
		}
		
		// If pool has been idle for 2x timeout and has no min size, remove it
		if idleTime > m.idleTimeout*2 && minSize == 0 {
			pool.mu.RLock()
			poolSize = pool.CurrentSize
			pool.mu.RUnlock()
			
			poolsToCleanup = append(poolsToCleanup, poolCleanupInfo{
				key:      key,
				pool:     pool,
				toRemove: poolSize,
				remove:   true,
			})
		}
	}
	m.mu.RUnlock()
	
	// Process cleanup operations without holding manager lock
	for _, info := range poolsToCleanup {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		// Scale down pool (releases locks internally)
		m.scaleDownPool(ctx, info.pool, info.toRemove)
		
		// Remove pool from manager if needed
		if info.remove {
			m.mu.Lock()
			// Double-check pool still exists
			if _, exists := m.warmPools[info.key]; exists {
				delete(m.warmPools, info.key)
				m.mu.Unlock()
				
				m.logger.Info("Removed idle warm pool",
					zap.String("key", info.key),
				)
			} else {
				m.mu.Unlock()
			}
		}
	}
}

// scaleDownPool scales down a warm pool by removing the specified number of Drones.
func (m *Manager) scaleDownPool(ctx context.Context, pool *WarmPool, count int) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return
	default:
	}
	
	// Read drone references while holding lock (quick operation)
	pool.mu.Lock()
	if count > len(pool.WarmDrones) {
		count = len(pool.WarmDrones)
	}
	
	// Copy drone references
	dronesToRemove := make([]*apiary.Drone, count)
	copy(dronesToRemove, pool.WarmDrones[:count])
	
	// Update pool immediately (remove references)
	pool.WarmDrones = pool.WarmDrones[count:]
	pool.CurrentSize = len(pool.WarmDrones)
	pool.mu.Unlock()
	
	// Perform I/O operations without holding lock
	removed := 0
	for i := 0; i < len(dronesToRemove); i++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			m.logger.Info("Scale down cancelled by context",
				zap.String("agentspec", pool.AgentSpec),
				zap.Int("removed", removed),
				zap.Int("pending", len(dronesToRemove)-i),
			)
			return
		default:
		}
		
		drone := dronesToRemove[i]
		
		// Stop the process (I/O operation)
		if err := m.launcher.Stop(ctx, drone.GetUID()); err != nil {
			m.logger.Warn("Failed to stop warm Drone during scale down",
				zap.String("drone", drone.GetName()),
				zap.Error(err),
			)
		}
		
		// Delete from store (I/O operation)
		if err := m.store.Delete(ctx, "Drone", drone.GetName(), drone.GetNamespace()); err != nil {
			m.logger.Warn("Failed to delete warm Drone during scale down",
				zap.String("drone", drone.GetName()),
				zap.Error(err),
			)
		} else {
			removed++
		}
	}
	
	m.logger.Info("Scaled down warm pool",
		zap.String("agentspec", pool.AgentSpec),
		zap.Int("removed", removed),
		zap.Int("remaining", pool.CurrentSize),
	)
}
