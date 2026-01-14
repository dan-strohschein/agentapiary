// Package scheduler provides scheduling logic for Drones.
package scheduler

import (
	"context"
	"fmt"
	"sync"

	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// Scheduler implements the apiary.Scheduler interface.
type Scheduler struct {
	logger        *zap.Logger
	tierEvaluator *TierEvaluator
	mu            sync.Mutex
	// In a real implementation, this would track available resources
	availableCPU    int64
	availableMemory int64
}

// NewScheduler creates a new scheduler.
func NewScheduler(logger *zap.Logger) *Scheduler {
	// Initialize with reasonable defaults
	return &Scheduler{
		logger:        logger,
		tierEvaluator: NewTierEvaluator(logger),
		availableCPU:    100000, // 100 cores in millicores
		availableMemory: 100 * 1024 * 1024 * 1024, // 100 GB
	}
}

// Schedule determines where to schedule a Drone.
func (s *Scheduler) Schedule(ctx context.Context, spec *apiary.AgentSpec) (*apiary.ScheduleDecision, error) {
	// Evaluate task tier and get scheduling hints
	tierHints := s.tierEvaluator.EvaluateTier(spec)
	
	// Get base resource requirements
	baseCPU := spec.Spec.Resources.Requests.CPU
	baseMemory := spec.Spec.Resources.Requests.Memory

	if baseCPU == 0 {
		baseCPU = 500 // Default 500m
	}
	if baseMemory == 0 {
		baseMemory = 512 * 1024 * 1024 // Default 512MB
	}
	
	baseRequirements := apiary.ResourceRequirements{
		CPU:    baseCPU,
		Memory: baseMemory,
	}

	// Apply tier hints to adjust resource requirements
	adjustedRequirements := s.tierEvaluator.ApplyTierHints(baseRequirements, tierHints)
	requiredCPU := adjustedRequirements.CPU
	requiredMemory := adjustedRequirements.Memory

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if resources are available
	if requiredCPU > s.availableCPU {
		return nil, fmt.Errorf("insufficient CPU: required %d, available %d", requiredCPU, s.availableCPU)
	}

	if requiredMemory > s.availableMemory {
		return nil, fmt.Errorf("insufficient memory: required %d, available %d", requiredMemory, s.availableMemory)
	}

	// Reserve resources (in a real implementation, this would be more sophisticated)
	s.availableCPU -= requiredCPU
	s.availableMemory -= requiredMemory

	// Build reason string including tier information
	reason := fmt.Sprintf("scheduled on local node (tier: %s, preference: %s)", 
		tierHints.Tier, tierHints.CostPreference)

	decision := &apiary.ScheduleDecision{
		NodeID: "local",
		Resources: &apiary.ResourceRequirements{
			CPU:    requiredCPU,
			Memory: requiredMemory,
		},
		Reason: reason,
	}

	s.logger.Info("Scheduled drone",
		zap.String("agentspec", spec.GetName()),
		zap.String("tier", string(tierHints.Tier)),
		zap.Int64("cpu", requiredCPU),
		zap.Int64("memory", requiredMemory),
		zap.String("costPreference", string(tierHints.CostPreference)),
	)

	return decision, nil
}

// GetResourceAvailability returns available resources.
func (s *Scheduler) GetResourceAvailability(ctx context.Context) (*apiary.ResourceAvailability, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return &apiary.ResourceAvailability{
		AvailableCPU:    s.availableCPU,
		AvailableMemory: s.availableMemory,
		TotalCPU:        100000,
		TotalMemory:     100 * 1024 * 1024 * 1024,
	}, nil
}
