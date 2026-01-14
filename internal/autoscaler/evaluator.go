// Package autoscaler provides autoscaling logic for Drones.
package autoscaler

import (
	"context"
	"fmt"
	"time"

	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// Evaluator evaluates metrics and determines desired replica count.
type Evaluator struct {
	metrics *metrics.Collector
	logger  *zap.Logger
}

// NewEvaluator creates a new scaling evaluator.
func NewEvaluator(metricsCollector *metrics.Collector, logger *zap.Logger) *Evaluator {
	return &Evaluator{
		metrics: metricsCollector,
		logger:  logger,
	}
}

// EvaluateScaling evaluates metrics and returns the desired replica count.
func (e *Evaluator) EvaluateScaling(ctx context.Context, spec *apiary.AgentSpec, currentReplicas int) (int, error) {
	scaling := spec.Spec.Scaling
	
	// If no scaling config, use min replicas or current count
	if scaling.MinReplicas == 0 && scaling.MaxReplicas == 0 {
		if currentReplicas == 0 {
			return 1, nil // Default to 1 replica
		}
		return currentReplicas, nil // Keep current
	}
	
	// Ensure min/max are set with defaults
	minReplicas := scaling.MinReplicas
	if minReplicas == 0 {
		minReplicas = 1
	}
	maxReplicas := scaling.MaxReplicas
	if maxReplicas == 0 {
		maxReplicas = 10 // Default max
	}
	
	// Start with current replicas
	desiredReplicas := currentReplicas
	
	// Evaluate latency-based scaling
	if scaling.TargetLatencyMs > 0 {
		latencyStats, err := e.metrics.GetLatencyStats(spec.GetNamespace(), spec.GetName())
		if err == nil && latencyStats.Count > 0 {
			p95LatencyMs := latencyStats.P95.Milliseconds()
			
			if p95LatencyMs > int64(scaling.TargetLatencyMs) {
				// Scale up: calculate how many replicas needed
				// Simple heuristic: scale by ratio of actual to target latency
				scaleFactor := float64(p95LatencyMs) / float64(scaling.TargetLatencyMs)
				desiredReplicas = int(float64(currentReplicas) * scaleFactor)
				
				e.logger.Info("Latency-based scale up decision",
					zap.String("agentspec", spec.GetName()),
					zap.Int64("p95LatencyMs", p95LatencyMs),
					zap.Int("targetLatencyMs", scaling.TargetLatencyMs),
					zap.Int("currentReplicas", currentReplicas),
					zap.Int("desiredReplicas", desiredReplicas),
				)
			} else if p95LatencyMs < int64(scaling.TargetLatencyMs)*80/100 && currentReplicas > minReplicas {
				// Scale down: if latency is 20% below target
				desiredReplicas = currentReplicas - 1
				
				e.logger.Info("Latency-based scale down decision",
					zap.String("agentspec", spec.GetName()),
					zap.Int64("p95LatencyMs", p95LatencyMs),
					zap.Int("targetLatencyMs", scaling.TargetLatencyMs),
					zap.Int("currentReplicas", currentReplicas),
					zap.Int("desiredReplicas", desiredReplicas),
				)
			}
		}
	}
	
	// Evaluate queue depth scaling (use request rate as proxy)
	// Scale up if queue depth is high
	queueDepth, err := e.metrics.GetQueueDepth(spec.GetNamespace(), spec.GetName())
	if err == nil && queueDepth > 10 {
		// Scale up if queue depth exceeds 10
		scaleFactor := 1.0 + float64(queueDepth)/10.0
		calculatedReplicas := int(float64(currentReplicas) * scaleFactor)
		if calculatedReplicas > desiredReplicas {
			desiredReplicas = calculatedReplicas
		}
		
		e.logger.Info("Queue depth-based scale up decision",
			zap.String("agentspec", spec.GetName()),
			zap.Int("queueDepth", queueDepth),
			zap.Int("currentReplicas", currentReplicas),
			zap.Int("desiredReplicas", desiredReplicas),
		)
	}
	
	// Apply min/max constraints
	if desiredReplicas < minReplicas {
		desiredReplicas = minReplicas
	}
	if desiredReplicas > maxReplicas {
		desiredReplicas = maxReplicas
	}
	
	return desiredReplicas, nil
}

// ScalingDecision represents a scaling decision.
type ScalingDecision struct {
	AgentSpec        string
	Namespace        string
	CurrentReplicas  int
	DesiredReplicas  int
	Action           ScalingAction
	Reason           string
	Timestamp        time.Time
}

// ScalingAction represents the type of scaling action.
type ScalingAction string

const (
	ScalingActionNone  ScalingAction = "none"
	ScalingActionUp    ScalingAction = "up"
	ScalingActionDown  ScalingAction = "down"
)

// MakeDecision creates a scaling decision from evaluation.
func (e *Evaluator) MakeDecision(ctx context.Context, spec *apiary.AgentSpec, currentReplicas int) (*ScalingDecision, error) {
	desiredReplicas, err := e.EvaluateScaling(ctx, spec, currentReplicas)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate scaling: %w", err)
	}
	
	var action ScalingAction
	var reason string
	
	if desiredReplicas > currentReplicas {
		action = ScalingActionUp
		reason = fmt.Sprintf("scale up from %d to %d replicas", currentReplicas, desiredReplicas)
	} else if desiredReplicas < currentReplicas {
		action = ScalingActionDown
		reason = fmt.Sprintf("scale down from %d to %d replicas", currentReplicas, desiredReplicas)
	} else {
		action = ScalingActionNone
		reason = "no scaling needed"
	}
	
	return &ScalingDecision{
		AgentSpec:       spec.GetName(),
		Namespace:       spec.GetNamespace(),
		CurrentReplicas: currentReplicas,
		DesiredReplicas: desiredReplicas,
		Action:          action,
		Reason:          reason,
		Timestamp:       time.Now(),
	}, nil
}
