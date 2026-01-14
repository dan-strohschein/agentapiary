// Package autoscaler provides autoscaling logic for Drones.
package autoscaler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// Autoscaler orchestrates automatic scaling of Drones.
type Autoscaler struct {
	evaluator *Evaluator
	executor  *Executor
	logger    *zap.Logger
	
	mu            sync.RWMutex
	lastScaling   map[string]time.Time // key: namespace/agentspec
	cooldownPeriod time.Duration
}

// Config holds autoscaler configuration.
type Config struct {
	Metrics       *metrics.Collector
	Store         apiary.ResourceStore
	Launcher      apiary.AgentLauncher
	Scheduler     apiary.Scheduler
	CooldownPeriod time.Duration
	Logger        *zap.Logger
}

// NewAutoscaler creates a new autoscaler.
func NewAutoscaler(cfg Config) *Autoscaler {
	cooldownPeriod := cfg.CooldownPeriod
	if cooldownPeriod == 0 {
		cooldownPeriod = 60 * time.Second // Default 60s cooldown
	}
	
	return &Autoscaler{
		evaluator:     NewEvaluator(cfg.Metrics, cfg.Logger),
		executor:      NewExecutor(cfg.Store, cfg.Launcher, cfg.Scheduler, cfg.Logger),
		logger:        cfg.Logger,
		lastScaling:   make(map[string]time.Time),
		cooldownPeriod: cooldownPeriod,
	}
}

// EvaluateAndScale evaluates metrics and performs scaling if needed.
func (a *Autoscaler) EvaluateAndScale(ctx context.Context, spec *apiary.AgentSpec, currentReplicas int) error {
	key := fmt.Sprintf("%s/%s", spec.GetNamespace(), spec.GetName())
	
	// Check cooldown period
	a.mu.RLock()
	lastScaling, inCooldown := a.lastScaling[key]
	a.mu.RUnlock()
	
	if inCooldown {
		timeSinceLastScaling := time.Since(lastScaling)
		if timeSinceLastScaling < a.cooldownPeriod {
			remaining := a.cooldownPeriod - timeSinceLastScaling
			a.logger.Debug("Scaling in cooldown period",
				zap.String("agentspec", spec.GetName()),
				zap.String("namespace", spec.GetNamespace()),
				zap.Duration("remaining", remaining),
			)
			return nil // Skip scaling during cooldown
		}
	}
	
	// Check if scaling is already in progress
	if a.executor.IsScalingInProgress(spec.GetNamespace(), spec.GetName()) {
		a.logger.Debug("Scaling already in progress",
			zap.String("agentspec", spec.GetName()),
			zap.String("namespace", spec.GetNamespace()),
		)
		return nil
	}
	
	// Evaluate scaling
	decision, err := a.evaluator.MakeDecision(ctx, spec, currentReplicas)
	if err != nil {
		return fmt.Errorf("failed to evaluate scaling: %w", err)
	}
	
	if decision.Action == ScalingActionNone {
		return nil // No scaling needed
	}
	
	// Execute scaling
	if err := a.executor.ExecuteScaling(ctx, decision, spec); err != nil {
		return fmt.Errorf("failed to execute scaling: %w", err)
	}
	
	// Record scaling timestamp
	a.mu.Lock()
	a.lastScaling[key] = time.Now()
	a.mu.Unlock()
	
	a.logger.Info("Autoscaling completed",
		zap.String("agentspec", spec.GetName()),
		zap.String("namespace", spec.GetNamespace()),
		zap.String("action", string(decision.Action)),
		zap.Int("currentReplicas", decision.CurrentReplicas),
		zap.Int("desiredReplicas", decision.DesiredReplicas),
	)
	
	return nil
}

// GetLastScalingTime returns the last scaling time for an AgentSpec.
func (a *Autoscaler) GetLastScalingTime(namespace, agentspec string) (time.Time, bool) {
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	last, exists := a.lastScaling[key]
	return last, exists
}
