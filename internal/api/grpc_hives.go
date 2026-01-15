// Package api provides gRPC RPC methods for Hives.
//
// This file implements the Hives RPC methods:
//   - ListHives: List all Hives in a namespace
//   - GetHive: Get a Hive by name
//   - CreateHive: Create a new Hive
//   - UpdateHive: Update an existing Hive
//   - DeleteHive: Delete a Hive by name
//   - ScaleHive: Scale a Hive or a specific stage
package api

import (
	"context"
	"fmt"

	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/pkg/apiary"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// checkResourceQuotaForHive checks if creating a Hive would exceed the Cell's quota.
// This is a helper method for GRPCServer, similar to the one in Server.
func (s *GRPCServer) checkResourceQuotaForHive(ctx context.Context, namespace string) error {
	if namespace == "" {
		return nil
	}

	// Get the Cell
	cell, err := s.store.Get(ctx, "Cell", namespace, "")
	if err == apiary.ErrNotFound {
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

	// Check Hive quota
	if quota.MaxHives > 0 {
		hives, err := s.store.List(ctx, "Hive", namespace, nil)
		if err != nil {
			return fmt.Errorf("failed to list hives: %w", err)
		}
		if len(hives) >= quota.MaxHives {
			return fmt.Errorf("quota exceeded: max hives (%d) reached in cell %s", quota.MaxHives, namespace)
		}
	}

	return nil
}

// ListHives lists all Hives in a namespace.
// This RPC method mirrors GET /api/v1/cells/:namespace/hives.
func (s *GRPCServer) ListHives(ctx context.Context, req *v1.ListHivesRequest) (*v1.ListHivesResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	resources, err := s.store.List(ctx, "Hive", req.GetNamespace(), nil)
	if err != nil {
		return nil, s.handleError(err)
	}

	hives := make([]*v1.Hive, 0, len(resources))
	for _, r := range resources {
		if hive, ok := r.(*apiary.Hive); ok {
			hives = append(hives, hiveToProto(hive))
		}
	}

	return &v1.ListHivesResponse{
		Hives: hives,
	}, nil
}

// GetHive retrieves a Hive by name.
// This RPC method mirrors GET /api/v1/cells/:namespace/hives/:name.
func (s *GRPCServer) GetHive(ctx context.Context, req *v1.GetHiveRequest) (*v1.Hive, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	resource, err := s.store.Get(ctx, "Hive", req.GetName(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	hive, ok := resource.(*apiary.Hive)
	if !ok {
		return nil, status.Error(codes.Internal, "resource is not a Hive")
	}

	return hiveToProto(hive), nil
}

// CreateHive creates a new Hive.
// This RPC method mirrors POST /api/v1/cells/:namespace/hives.
func (s *GRPCServer) CreateHive(ctx context.Context, req *v1.CreateHiveRequest) (*v1.Hive, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetHive() == nil {
		return nil, status.Error(codes.InvalidArgument, "hive is required")
	}

	// Check resource quota
	if err := s.checkResourceQuotaForHive(ctx, req.GetNamespace()); err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}

	hive := hiveFromProto(req.GetHive())

	// Set namespace if not set
	if hive.ObjectMeta.Namespace == "" {
		hive.ObjectMeta.Namespace = req.GetNamespace()
	} else if hive.ObjectMeta.Namespace != req.GetNamespace() {
		return nil, status.Error(codes.InvalidArgument, "namespace in request does not match namespace in hive")
	}

	if err := s.store.Create(ctx, hive); err != nil {
		return nil, s.handleError(err)
	}

	return hiveToProto(hive), nil
}

// UpdateHive updates an existing Hive.
// This RPC method mirrors PUT /api/v1/cells/:namespace/hives/:name.
func (s *GRPCServer) UpdateHive(ctx context.Context, req *v1.UpdateHiveRequest) (*v1.Hive, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetHive() == nil {
		return nil, status.Error(codes.InvalidArgument, "hive is required")
	}

	hive := hiveFromProto(req.GetHive())

	// Validate name matches
	if hive.GetName() != req.GetName() {
		return nil, status.Error(codes.InvalidArgument, "name in request does not match name in hive")
	}

	// Validate namespace matches
	if hive.ObjectMeta.Namespace != req.GetNamespace() {
		return nil, status.Error(codes.InvalidArgument, "namespace in request does not match namespace in hive")
	}

	if err := s.store.Update(ctx, hive); err != nil {
		return nil, s.handleError(err)
	}

	return hiveToProto(hive), nil
}

// DeleteHive deletes a Hive by name.
// This RPC method mirrors DELETE /api/v1/cells/:namespace/hives/:name.
func (s *GRPCServer) DeleteHive(ctx context.Context, req *v1.DeleteHiveRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if err := s.store.Delete(ctx, "Hive", req.GetName(), req.GetNamespace()); err != nil {
		return nil, s.handleError(err)
	}

	return &emptypb.Empty{}, nil
}

// ScaleHive scales a Hive or a specific stage.
// This RPC method mirrors PUT /api/v1/cells/:namespace/hives/:name/scale.
func (s *GRPCServer) ScaleHive(ctx context.Context, req *v1.ScaleHiveRequest) (*v1.ScaleHiveResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetReplicas() < 0 {
		return nil, status.Error(codes.InvalidArgument, "replicas must be >= 0")
	}

	// Get the Hive
	resource, err := s.store.Get(ctx, "Hive", req.GetName(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	hive, ok := resource.(*apiary.Hive)
	if !ok {
		return nil, status.Error(codes.Internal, "failed to cast resource to Hive")
	}

	replicas := int(req.GetReplicas())
	stageName := req.GetStage()

	// If stage is specified, scale that stage; otherwise scale all stages
	if stageName != "" {
		found := false
		for i := range hive.Spec.Stages {
			if hive.Spec.Stages[i].Name == stageName {
				hive.Spec.Stages[i].Replicas = replicas
				found = true
				break
			}
		}
		if !found {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("stage %s not found", stageName))
		}
	} else {
		// Scale all stages
		for i := range hive.Spec.Stages {
			hive.Spec.Stages[i].Replicas = replicas
		}
	}

	// Update the Hive
	if err := s.store.Update(ctx, hive); err != nil {
		return nil, s.handleError(err)
	}

	message := fmt.Sprintf("Hive %s scaled to %d replicas", req.GetName(), replicas)
	if stageName != "" {
		message = fmt.Sprintf("Hive %s stage %s scaled to %d replicas", req.GetName(), stageName, replicas)
	}

	return &v1.ScaleHiveResponse{
		Message:  message,
		Replicas: req.GetReplicas(),
	}, nil
}
