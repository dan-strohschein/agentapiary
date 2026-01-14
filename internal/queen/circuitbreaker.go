// Package queen provides the central orchestrator for Apiary.
package queen

import (
	"context"

	"github.com/agentapiary/apiary/pkg/apiary"
)

// RecordDroneFailure records a failure for a Drone in the circuit breaker.
func (q *Queen) RecordDroneFailure(ctx context.Context, droneID string) {
	q.circuitBreaker.RecordFailure(ctx, droneID)
}

// RecordDroneSuccess records a success for a Drone in the circuit breaker.
func (q *Queen) RecordDroneSuccess(ctx context.Context, droneID string) {
	q.circuitBreaker.RecordSuccess(ctx, droneID)
}

// GetAvailableDrones returns Drones that are available for routing (circuit breaker closed).
func (q *Queen) GetAvailableDrones(ctx context.Context, namespace string, selector apiary.Labels) ([]*apiary.Drone, error) {
	// ListDrones already filters out Drones with open circuit breakers
	return q.ListDrones(ctx, namespace, selector)
}
