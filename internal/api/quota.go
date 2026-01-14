package api

import (
	"context"
	"fmt"

	"github.com/agentapiary/apiary/pkg/apiary"
)

// checkResourceQuota checks if creating a resource would exceed the Cell's quota.
func (s *Server) checkResourceQuota(ctx context.Context, namespace string, kind string) error {
	if namespace == "" {
		// Cells are not namespaced, skip quota check
		return nil
	}

	// Get the Cell
	cell, err := s.store.Get(ctx, "Cell", namespace, "")
	if err == apiary.ErrNotFound {
		// Cell doesn't exist, allow creation (or return error based on policy)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get cell: %w", err)
	}

	cellResource, ok := cell.(*apiary.Cell)
	if !ok {
		return fmt.Errorf("resource is not a Cell")
	}

	quota := cellResource.Spec.ResourceQuota

	// Check quotas based on resource kind
	switch kind {
	case "Hive":
		if quota.MaxHives > 0 {
			hives, err := s.store.List(ctx, "Hive", namespace, nil)
			if err != nil {
				return fmt.Errorf("failed to list hives: %w", err)
			}
			if len(hives) >= quota.MaxHives {
				return fmt.Errorf("quota exceeded: max hives (%d) reached in cell %s", quota.MaxHives, namespace)
			}
		}

	case "AgentSpec":
		// AgentSpecs don't have a direct quota, but they create Drones
		// We check Drone quota when creating Drones

	case "Drone":
		// Check total drones quota
		if quota.MaxTotalDrones > 0 {
			drones, err := s.store.List(ctx, "Drone", namespace, nil)
			if err != nil {
				return fmt.Errorf("failed to list drones: %w", err)
			}
			if len(drones) >= quota.MaxTotalDrones {
				return fmt.Errorf("quota exceeded: max total drones (%d) reached in cell %s", quota.MaxTotalDrones, namespace)
			}
		}

		// Check memory quota if specified
		if quota.MaxMemoryGB > 0 {
			drones, err := s.store.List(ctx, "Drone", namespace, nil)
			if err != nil {
				return fmt.Errorf("failed to list drones: %w", err)
			}

			var totalMemory int64
			for _, r := range drones {
				if drone, ok := r.(*apiary.Drone); ok && drone.Spec != nil {
					memory := drone.Spec.Spec.Resources.Requests.Memory
					if memory == 0 {
						memory = 512 * 1024 * 1024 // Default 512MB
					}
					totalMemory += memory
				}
			}

			// Convert to GB and check
			memoryUsedGB := totalMemory / (1024 * 1024 * 1024)
			if memoryUsedGB >= int64(quota.MaxMemoryGB) {
				return fmt.Errorf("quota exceeded: max memory (%d GB) reached in cell %s", quota.MaxMemoryGB, namespace)
			}
		}

		// Check drones per hive quota (if this drone belongs to a hive)
		if quota.MaxDronesPerHive > 0 {
			// This would need to be checked based on the drone's hive association
			// For now, we'll check this in the Queen when creating drones
		}
	}

	return nil
}

// getQuotaUsage calculates current quota usage for a Cell.
func (s *Server) getQuotaUsage(ctx context.Context, namespace string) (*apiary.QuotaUsage, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Get the Cell to read quota limits
	cell, err := s.store.Get(ctx, "Cell", namespace, "")
	if err == apiary.ErrNotFound {
		return &apiary.QuotaUsage{}, nil // Return empty usage if cell doesn't exist
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cell: %w", err)
	}

	cellResource, ok := cell.(*apiary.Cell)
	if !ok {
		return nil, fmt.Errorf("resource is not a Cell")
	}

	quota := cellResource.Spec.ResourceQuota

	// Count Hives
	hives, err := s.store.List(ctx, "Hive", namespace, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list hives: %w", err)
	}

	// Count Drones and calculate memory usage
	drones, err := s.store.List(ctx, "Drone", namespace, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list drones: %w", err)
	}

	var totalMemory int64
	for _, r := range drones {
		if drone, ok := r.(*apiary.Drone); ok && drone.Spec != nil {
			memory := drone.Spec.Spec.Resources.Requests.Memory
			if memory == 0 {
				memory = 512 * 1024 * 1024 // Default 512MB
			}
			totalMemory += memory
		}
	}

	memoryUsedGB := totalMemory / (1024 * 1024 * 1024)

	return &apiary.QuotaUsage{
		Hives:         len(hives),
		TotalDrones:   len(drones),
		MemoryUsedGB:  memoryUsedGB,
		MemoryLimitGB: quota.MaxMemoryGB,
	}, nil
}
