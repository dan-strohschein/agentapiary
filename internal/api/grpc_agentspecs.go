// Package api provides gRPC RPC methods for AgentSpecs.
//
// This file implements the AgentSpecs RPC methods:
//   - ListAgentSpecs: List all AgentSpecs in a namespace
//   - GetAgentSpec: Get an AgentSpec by name
//   - CreateAgentSpec: Create a new AgentSpec
//   - UpdateAgentSpec: Update an existing AgentSpec
//   - DeleteAgentSpec: Delete an AgentSpec by name
//   - ScaleAgentSpec: Scale an AgentSpec
//   - DrainAgentSpec: Drain all Drones for an AgentSpec
package api

import (
	"context"
	"fmt"
	"time"

	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/pkg/apiary"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// checkResourceQuota checks if creating a resource would exceed the Cell's quota.
// This is a helper method for GRPCServer, similar to the one in Server.
func (s *GRPCServer) checkResourceQuota(ctx context.Context, namespace string, kind string) error {
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
	}

	return nil
}

// ListAgentSpecs lists all AgentSpecs in a namespace.
// This RPC method mirrors GET /api/v1/cells/:namespace/agentspecs.
func (s *GRPCServer) ListAgentSpecs(ctx context.Context, req *v1.ListAgentSpecsRequest) (*v1.ListAgentSpecsResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	// Validate namespace
	if err := validateNamespace(req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	resources, err := s.store.List(ctx, "AgentSpec", req.GetNamespace(), nil)
	if err != nil {
		return nil, s.handleError(err)
	}

	specs := make([]*v1.AgentSpec, 0, len(resources))
	for _, r := range resources {
		if spec, ok := r.(*apiary.AgentSpec); ok {
			specs = append(specs, agentSpecToProto(spec))
		}
	}

	return &v1.ListAgentSpecsResponse{
		Agentspecs: specs,
	}, nil
}

// GetAgentSpec retrieves an AgentSpec by name.
// This RPC method mirrors GET /api/v1/cells/:namespace/agentspecs/:name.
func (s *GRPCServer) GetAgentSpec(ctx context.Context, req *v1.GetAgentSpecRequest) (*v1.AgentSpec, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Validate namespace
	if err := validateNamespace(req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	resource, err := s.store.Get(ctx, "AgentSpec", req.GetName(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	spec, ok := resource.(*apiary.AgentSpec)
	if !ok {
		return nil, status.Error(codes.Internal, "resource is not an AgentSpec")
	}

	return agentSpecToProto(spec), nil
}

// CreateAgentSpec creates a new AgentSpec.
// This RPC method mirrors POST /api/v1/cells/:namespace/agentspecs.
func (s *GRPCServer) CreateAgentSpec(ctx context.Context, req *v1.CreateAgentSpecRequest) (*v1.AgentSpec, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetAgentspec() == nil {
		return nil, status.Error(codes.InvalidArgument, "agentspec is required")
	}

	// Validate namespace
	if err := validateNamespace(req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	spec := agentSpecFromProto(req.GetAgentspec())

	// Validate required fields
	if len(spec.Spec.Runtime.Command) == 0 {
		return nil, status.Error(codes.InvalidArgument, "spec.runtime.command is required")
	}
	if spec.Spec.Interface.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "spec.interface.type is required")
	}

	// Set namespace if not set
	if spec.ObjectMeta.Namespace == "" {
		spec.ObjectMeta.Namespace = req.GetNamespace()
	} else if spec.ObjectMeta.Namespace != req.GetNamespace() {
		return nil, status.Error(codes.InvalidArgument, "namespace in request does not match namespace in agentspec")
	}

	// Check resource quota before creating
	if err := s.checkResourceQuota(ctx, req.GetNamespace(), "AgentSpec"); err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}

	if err := s.store.Create(ctx, spec); err != nil {
		return nil, s.handleError(err)
	}

	return agentSpecToProto(spec), nil
}

// UpdateAgentSpec updates an existing AgentSpec.
// This RPC method mirrors PUT /api/v1/cells/:namespace/agentspecs/:name.
func (s *GRPCServer) UpdateAgentSpec(ctx context.Context, req *v1.UpdateAgentSpecRequest) (*v1.AgentSpec, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetAgentspec() == nil {
		return nil, status.Error(codes.InvalidArgument, "agentspec is required")
	}

	// Validate namespace
	if err := validateNamespace(req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	spec := agentSpecFromProto(req.GetAgentspec())

	// Validate namespace matches
	if err := validateResourceNamespace(spec.GetNamespace(), req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Validate required fields
	if len(spec.Spec.Runtime.Command) == 0 {
		return nil, status.Error(codes.InvalidArgument, "spec.runtime.command is required")
	}
	if spec.Spec.Interface.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "spec.interface.type is required")
	}

	if spec.GetName() != req.GetName() {
		return nil, status.Error(codes.InvalidArgument, "name in request does not match name in agentspec")
	}

	if err := s.store.Update(ctx, spec); err != nil {
		return nil, s.handleError(err)
	}

	return agentSpecToProto(spec), nil
}

// DeleteAgentSpec deletes an AgentSpec by name.
// This RPC method mirrors DELETE /api/v1/cells/:namespace/agentspecs/:name.
func (s *GRPCServer) DeleteAgentSpec(ctx context.Context, req *v1.DeleteAgentSpecRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if err := s.store.Delete(ctx, "AgentSpec", req.GetName(), req.GetNamespace()); err != nil {
		return nil, s.handleError(err)
	}

	return &emptypb.Empty{}, nil
}

// ScaleAgentSpec scales an AgentSpec.
// This RPC method mirrors PUT /api/v1/cells/:namespace/agentspecs/:name/scale.
func (s *GRPCServer) ScaleAgentSpec(ctx context.Context, req *v1.ScaleAgentSpecRequest) (*v1.ScaleAgentSpecResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetReplicas() < 0 {
		return nil, status.Error(codes.InvalidArgument, "replicas must be >= 0")
	}

	// Get the AgentSpec
	resource, err := s.store.Get(ctx, "AgentSpec", req.GetName(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	spec, ok := resource.(*apiary.AgentSpec)
	if !ok {
		return nil, status.Error(codes.Internal, "failed to cast resource to AgentSpec")
	}

	replicas := int(req.GetReplicas())

	// Validate replicas against min/max
	if spec.Spec.Scaling.MinReplicas > 0 && replicas < spec.Spec.Scaling.MinReplicas {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("replicas (%d) must be >= minReplicas (%d)", replicas, spec.Spec.Scaling.MinReplicas))
	}

	if spec.Spec.Scaling.MaxReplicas > 0 && replicas > spec.Spec.Scaling.MaxReplicas {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("replicas (%d) must be <= maxReplicas (%d)", replicas, spec.Spec.Scaling.MaxReplicas))
	}

	// Update scaling config (set min and max to the desired replicas for manual scaling)
	// This is a simplified approach - in production, you might want to track desired replicas separately
	spec.Spec.Scaling.MinReplicas = replicas
	spec.Spec.Scaling.MaxReplicas = replicas

	// Update the AgentSpec
	if err := s.store.Update(ctx, spec); err != nil {
		return nil, s.handleError(err)
	}

	return &v1.ScaleAgentSpecResponse{
		Message:  fmt.Sprintf("AgentSpec %s scaled to %d replicas", req.GetName(), replicas),
		Replicas: req.GetReplicas(),
	}, nil
}

// DrainAgentSpec drains all Drones for an AgentSpec.
// This RPC method mirrors POST /api/v1/cells/:namespace/agentspecs/:name/drain.
func (s *GRPCServer) DrainAgentSpec(ctx context.Context, req *v1.DrainAgentSpecRequest) (*v1.DrainAgentSpecResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Get the AgentSpec
	_, err := s.store.Get(ctx, "AgentSpec", req.GetName(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	force := req.GetForce()
	timeoutStr := req.GetTimeout()
	if timeoutStr == "" {
		timeoutStr = "30s"
	}

	// List all Drones for this AgentSpec
	drones, err := s.store.List(ctx, "Drone", req.GetNamespace(), apiary.Labels{
		"agentspec": req.GetName(),
	})
	if err != nil {
		return nil, s.handleError(err)
	}

	drainedCount := int32(0)
	errors := []string{}

	// Drain each Drone
	for _, resource := range drones {
		drone, ok := resource.(*apiary.Drone)
		if !ok {
			continue
		}

		// Mark Drone as draining
		if drone.ObjectMeta.Annotations == nil {
			drone.ObjectMeta.Annotations = make(map[string]string)
		}
		drone.ObjectMeta.Annotations["draining"] = "true"
		drone.ObjectMeta.Annotations["drain-requested-at"] = time.Now().Format(time.RFC3339)
		drone.Status.Phase = apiary.DronePhaseStopping

		if err := s.store.Update(ctx, drone); err != nil {
			errors = append(errors, fmt.Sprintf("failed to mark drone %s for draining: %v", drone.GetName(), err))
			continue
		}

		// Delete immediately if force, otherwise let graceful drain happen
		if force {
			if err := s.store.Delete(ctx, "Drone", drone.GetName(), req.GetNamespace()); err != nil {
				errors = append(errors, fmt.Sprintf("failed to delete drone %s: %v", drone.GetName(), err))
			} else {
				drainedCount++
			}
		} else {
			// For graceful drain, we'd wait for completion
			// For now, mark as draining and let Queen handle it
			drainedCount++
		}
	}

	message := fmt.Sprintf("Drained %d Drone(s) for AgentSpec %s", drainedCount, req.GetName())
	if len(errors) > 0 {
		message += fmt.Sprintf(" (%d errors)", len(errors))
	}

	return &v1.DrainAgentSpecResponse{
		Message:     message,
		DrainedCount: drainedCount,
		Errors:      errors,
	}, nil
}
