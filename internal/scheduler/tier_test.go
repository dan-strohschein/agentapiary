package scheduler

import (
	"testing"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestTierEvaluator_EvaluateTier(t *testing.T) {
	logger := zap.NewNop()
	evaluator := NewTierEvaluator(logger)
	
	tests := []struct {
		name     string
		taskTier string
		expected TaskTier
	}{
		{
			name:     "simple tier",
			taskTier: "simple",
			expected: TaskTierSimple,
		},
		{
			name:     "standard tier",
			taskTier: "standard",
			expected: TaskTierStandard,
		},
		{
			name:     "complex tier",
			taskTier: "complex",
			expected: TaskTierComplex,
		},
		{
			name:     "empty tier defaults to standard",
			taskTier: "",
			expected: TaskTierStandard,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &apiary.AgentSpec{
				ObjectMeta: apiary.ObjectMeta{
					Name: "test-agent",
				},
				Spec: apiary.AgentSpecSpec{
					TaskTier: tt.taskTier,
				},
			}
			
			hints := evaluator.EvaluateTier(spec)
			assert.Equal(t, tt.expected, hints.Tier)
		})
	}
}

func TestTierEvaluator_GetPriority(t *testing.T) {
	logger := zap.NewNop()
	evaluator := NewTierEvaluator(logger)
	
	assert.Equal(t, 1, evaluator.getPriority(TaskTierSimple))
	assert.Equal(t, 5, evaluator.getPriority(TaskTierStandard))
	assert.Equal(t, 10, evaluator.getPriority(TaskTierComplex))
}

func TestTierEvaluator_GetResourceMultiplier(t *testing.T) {
	logger := zap.NewNop()
	evaluator := NewTierEvaluator(logger)
	
	assert.Equal(t, 0.8, evaluator.getResourceMultiplier(TaskTierSimple))
	assert.Equal(t, 1.0, evaluator.getResourceMultiplier(TaskTierStandard))
	assert.Equal(t, 1.5, evaluator.getResourceMultiplier(TaskTierComplex))
}

func TestTierEvaluator_GetCostPreference(t *testing.T) {
	logger := zap.NewNop()
	evaluator := NewTierEvaluator(logger)
	
	assert.Equal(t, CostPreferenceMinimize, evaluator.getCostPreference(TaskTierSimple))
	assert.Equal(t, CostPreferenceBalance, evaluator.getCostPreference(TaskTierStandard))
	assert.Equal(t, CostPreferenceMaximize, evaluator.getCostPreference(TaskTierComplex))
}

func TestTierEvaluator_ApplyTierHints(t *testing.T) {
	logger := zap.NewNop()
	evaluator := NewTierEvaluator(logger)
	
	base := apiary.ResourceRequirements{
		CPU:    1000, // 1 core
		Memory: 1024 * 1024 * 1024, // 1GB
	}
	
	// Simple tier should reduce resources
	hintsSimple := &TierHints{
		Tier:              TaskTierSimple,
		ResourceMultiplier: 0.8,
	}
	adjustedSimple := evaluator.ApplyTierHints(base, hintsSimple)
	assert.Equal(t, int64(800), adjustedSimple.CPU)
	assert.Equal(t, int64(858993459), adjustedSimple.Memory) // 0.8 * 1GB
	
	// Standard tier should keep resources the same
	hintsStandard := &TierHints{
		Tier:              TaskTierStandard,
		ResourceMultiplier: 1.0,
	}
	adjustedStandard := evaluator.ApplyTierHints(base, hintsStandard)
	assert.Equal(t, int64(1000), adjustedStandard.CPU)
	assert.Equal(t, int64(1073741824), adjustedStandard.Memory) // 1GB
	
	// Complex tier should increase resources
	hintsComplex := &TierHints{
		Tier:              TaskTierComplex,
		ResourceMultiplier: 1.5,
	}
	adjustedComplex := evaluator.ApplyTierHints(base, hintsComplex)
	assert.Equal(t, int64(1500), adjustedComplex.CPU)
	assert.Equal(t, int64(1610612736), adjustedComplex.Memory) // 1.5 * 1GB
}

func TestTierEvaluator_ApplyTierHints_MinimumValues(t *testing.T) {
	logger := zap.NewNop()
	evaluator := NewTierEvaluator(logger)
	
	// Test that minimum values are enforced
	base := apiary.ResourceRequirements{
		CPU:    50, // Below minimum
		Memory: 64 * 1024 * 1024, // Below minimum
	}
	
	hints := &TierHints{
		Tier:              TaskTierSimple,
		ResourceMultiplier: 0.8, // Would reduce further
	}
	adjusted := evaluator.ApplyTierHints(base, hints)
	
	// Should enforce minimums
	assert.GreaterOrEqual(t, adjusted.CPU, int64(100))
	assert.GreaterOrEqual(t, adjusted.Memory, int64(128*1024*1024))
}
