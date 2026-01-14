package autoscaler

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestAutoscaler(t *testing.T) (*Autoscaler, *metrics.Collector) {
	logger := zap.NewNop()
	metricsCollector := metrics.NewCollector(logger)
	
	// Create a mock store, launcher, and scheduler
	// For now, we'll just test the evaluator logic
	return nil, metricsCollector
}

func TestEvaluator_EvaluateScaling_NoScalingConfig(t *testing.T) {
	logger := zap.NewNop()
	metricsCollector := metrics.NewCollector(logger)
	evaluator := NewEvaluator(metricsCollector, logger)
	
	spec := &apiary.AgentSpec{
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: apiary.AgentSpecSpec{
			Scaling: apiary.ScalingConfig{},
		},
	}
	
	desired, err := evaluator.EvaluateScaling(context.Background(), spec, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, desired) // Should keep current
	
	desired, err = evaluator.EvaluateScaling(context.Background(), spec, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, desired) // Should default to 1
}

func TestEvaluator_EvaluateScaling_MinMaxConstraints(t *testing.T) {
	logger := zap.NewNop()
	metricsCollector := metrics.NewCollector(logger)
	evaluator := NewEvaluator(metricsCollector, logger)
	
	spec := &apiary.AgentSpec{
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: apiary.AgentSpecSpec{
			Scaling: apiary.ScalingConfig{
				MinReplicas: 2,
				MaxReplicas: 5,
			},
		},
	}
	
	desired, err := evaluator.EvaluateScaling(context.Background(), spec, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, desired) // Should respect min
	
	desired, err = evaluator.EvaluateScaling(context.Background(), spec, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, desired) // Should respect max
}

func TestEvaluator_MakeDecision(t *testing.T) {
	logger := zap.NewNop()
	metricsCollector := metrics.NewCollector(logger)
	evaluator := NewEvaluator(metricsCollector, logger)
	
	spec := &apiary.AgentSpec{
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: apiary.AgentSpecSpec{
			Scaling: apiary.ScalingConfig{
				MinReplicas: 1,
				MaxReplicas: 10,
			},
		},
	}
	
	decision, err := evaluator.MakeDecision(context.Background(), spec, 3)
	require.NoError(t, err)
	assert.Equal(t, "test-agent", decision.AgentSpec)
	assert.Equal(t, "default", decision.Namespace)
	assert.Equal(t, 3, decision.CurrentReplicas)
}

func TestAutoscaler_Cooldown(t *testing.T) {
	logger := zap.NewNop()
	metricsCollector := metrics.NewCollector(logger)
	
	autoscaler := &Autoscaler{
		evaluator:     NewEvaluator(metricsCollector, logger),
		logger:        logger,
		lastScaling:   make(map[string]time.Time),
		cooldownPeriod: 60 * time.Second,
	}
	
	// Set recent scaling time
	key := "default/test-agent"
	autoscaler.mu.Lock()
	autoscaler.lastScaling[key] = time.Now()
	autoscaler.mu.Unlock()
	
	// Should be in cooldown
	last, exists := autoscaler.GetLastScalingTime("default", "test-agent")
	assert.True(t, exists)
	assert.WithinDuration(t, time.Now(), last, time.Second)
}
