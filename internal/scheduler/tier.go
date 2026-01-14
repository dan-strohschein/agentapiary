// Package scheduler provides scheduling logic for Drones.
package scheduler

import (
	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// TierEvaluator evaluates task tiers and provides scheduling hints.
type TierEvaluator struct {
	logger *zap.Logger
}

// NewTierEvaluator creates a new tier evaluator.
func NewTierEvaluator(logger *zap.Logger) *TierEvaluator {
	return &TierEvaluator{
		logger: logger,
	}
}

// TaskTier represents a task tier level.
type TaskTier string

const (
	TaskTierSimple  TaskTier = "simple"   // Cost-effective for simple tasks
	TaskTierStandard TaskTier = "standard" // Balanced cost/capability
	TaskTierComplex TaskTier = "complex"   // High capability for complex tasks
)

// EvaluateTier evaluates the task tier and returns scheduling hints.
func (e *TierEvaluator) EvaluateTier(spec *apiary.AgentSpec) *TierHints {
	tier := TaskTier(spec.Spec.TaskTier)
	if tier == "" {
		tier = TaskTierStandard // Default to standard
	}
	
	hints := &TierHints{
		Tier:         tier,
		Priority:     e.getPriority(tier),
		ResourceMultiplier: e.getResourceMultiplier(tier),
		CostPreference: e.getCostPreference(tier),
	}
	
	e.logger.Debug("Evaluated task tier",
		zap.String("agentspec", spec.GetName()),
		zap.String("tier", string(tier)),
		zap.Int("priority", hints.Priority),
		zap.Float64("resourceMultiplier", hints.ResourceMultiplier),
	)
	
	return hints
}

// TierHints provides scheduling hints based on task tier.
type TierHints struct {
	Tier              TaskTier
	Priority          int                // Higher priority = prefer capable resources
	ResourceMultiplier float64           // Multiplier for resource requirements
	CostPreference    CostPreference     // Cost optimization preference
}

// CostPreference indicates cost optimization preference.
type CostPreference string

const (
	CostPreferenceMinimize CostPreference = "minimize" // Prefer cost-effective
	CostPreferenceBalance  CostPreference = "balance"  // Balance cost/capability
	CostPreferenceMaximize CostPreference = "maximize" // Prefer capable (higher cost OK)
)

// getPriority returns the priority for a task tier.
func (e *TierEvaluator) getPriority(tier TaskTier) int {
	switch tier {
	case TaskTierSimple:
		return 1 // Lower priority
	case TaskTierStandard:
		return 5 // Medium priority
	case TaskTierComplex:
		return 10 // Higher priority
	default:
		return 5 // Default to medium
	}
}

// getResourceMultiplier returns the resource multiplier for a task tier.
func (e *TierEvaluator) getResourceMultiplier(tier TaskTier) float64 {
	switch tier {
	case TaskTierSimple:
		return 0.8 // Use fewer resources (cost-effective)
	case TaskTierStandard:
		return 1.0 // Standard resource allocation
	case TaskTierComplex:
		return 1.5 // Use more resources (capable)
	default:
		return 1.0 // Default to standard
	}
}

// getCostPreference returns the cost preference for a task tier.
func (e *TierEvaluator) getCostPreference(tier TaskTier) CostPreference {
	switch tier {
	case TaskTierSimple:
		return CostPreferenceMinimize // Prefer cost-effective
	case TaskTierStandard:
		return CostPreferenceBalance // Balance cost/capability
	case TaskTierComplex:
		return CostPreferenceMaximize // Prefer capable
	default:
		return CostPreferenceBalance // Default to balance
	}
}

// ApplyTierHints applies tier hints to resource requirements.
func (e *TierEvaluator) ApplyTierHints(requests apiary.ResourceRequirements, hints *TierHints) apiary.ResourceRequirements {
	adjusted := apiary.ResourceRequirements{
		CPU:    int64(float64(requests.CPU) * hints.ResourceMultiplier),
		Memory: int64(float64(requests.Memory) * hints.ResourceMultiplier),
	}
	
	// Ensure minimum values
	if adjusted.CPU < 100 {
		adjusted.CPU = 100 // Minimum 100m CPU
	}
	if adjusted.Memory < 128*1024*1024 {
		adjusted.Memory = 128 * 1024 * 1024 // Minimum 128MB
	}
	
	return adjusted
}
