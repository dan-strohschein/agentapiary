// Package api provides gRPC RPC methods for Secrets.
//
// This file implements the Secrets RPC methods:
//   - ListSecrets: List all Secrets in a namespace
//   - GetSecret: Get a Secret by name
//   - CreateSecret: Create a new Secret
//   - UpdateSecret: Update an existing Secret
//   - DeleteSecret: Delete a Secret by name
package api

import (
	"context"

	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/pkg/apiary"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ListSecrets lists all Secrets in a namespace.
// This RPC method mirrors GET /api/v1/cells/:namespace/secrets.
func (s *GRPCServer) ListSecrets(ctx context.Context, req *v1.ListSecretsRequest) (*v1.ListSecretsResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	resources, err := s.store.List(ctx, "Secret", req.GetNamespace(), nil)
	if err != nil {
		return nil, s.handleError(err)
	}

	secrets := make([]*v1.Secret, 0, len(resources))
	for _, r := range resources {
		if secret, ok := r.(*apiary.Secret); ok {
			secrets = append(secrets, secretToProto(secret))
		}
	}

	return &v1.ListSecretsResponse{
		Secrets: secrets,
	}, nil
}

// GetSecret retrieves a Secret by name.
// This RPC method mirrors GET /api/v1/cells/:namespace/secrets/:name.
func (s *GRPCServer) GetSecret(ctx context.Context, req *v1.GetSecretRequest) (*v1.Secret, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	resource, err := s.store.Get(ctx, "Secret", req.GetName(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	secret, ok := resource.(*apiary.Secret)
	if !ok {
		return nil, status.Error(codes.Internal, "resource is not a Secret")
	}

	return secretToProto(secret), nil
}

// CreateSecret creates a new Secret.
// This RPC method mirrors POST /api/v1/cells/:namespace/secrets.
func (s *GRPCServer) CreateSecret(ctx context.Context, req *v1.CreateSecretRequest) (*v1.Secret, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetSecret() == nil {
		return nil, status.Error(codes.InvalidArgument, "secret is required")
	}

	// Validate namespace
	if err := validateNamespace(req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	secret := secretFromProto(req.GetSecret())

	// Set namespace if not set
	if secret.ObjectMeta.Namespace == "" {
		secret.ObjectMeta.Namespace = req.GetNamespace()
	} else if secret.ObjectMeta.Namespace != req.GetNamespace() {
		return nil, status.Error(codes.InvalidArgument, "namespace in request does not match namespace in secret")
	}

	// Validate secret data
	if secret.Data == nil || len(secret.Data) == 0 {
		return nil, status.Error(codes.InvalidArgument, "secret data cannot be empty")
	}

	// Validate resource namespace
	if err := validateResourceNamespace(secret.ObjectMeta.Namespace, req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.store.Create(ctx, secret); err != nil {
		return nil, s.handleError(err)
	}

	return secretToProto(secret), nil
}

// UpdateSecret updates an existing Secret.
// This RPC method mirrors PUT /api/v1/cells/:namespace/secrets/:name.
func (s *GRPCServer) UpdateSecret(ctx context.Context, req *v1.UpdateSecretRequest) (*v1.Secret, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetSecret() == nil {
		return nil, status.Error(codes.InvalidArgument, "secret is required")
	}

	// Validate namespace
	if err := validateNamespace(req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	secret := secretFromProto(req.GetSecret())

	// Validate name matches
	if secret.GetName() != req.GetName() {
		return nil, status.Error(codes.InvalidArgument, "name in request does not match name in secret")
	}

	// Validate namespace matches
	if err := validateResourceNamespace(secret.ObjectMeta.Namespace, req.GetNamespace()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.store.Update(ctx, secret); err != nil {
		return nil, s.handleError(err)
	}

	return secretToProto(secret), nil
}

// DeleteSecret deletes a Secret by name.
// This RPC method mirrors DELETE /api/v1/cells/:namespace/secrets/:name.
func (s *GRPCServer) DeleteSecret(ctx context.Context, req *v1.DeleteSecretRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if err := s.store.Delete(ctx, "Secret", req.GetName(), req.GetNamespace()); err != nil {
		return nil, s.handleError(err)
	}

	return &emptypb.Empty{}, nil
}
