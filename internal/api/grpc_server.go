// Package api provides the gRPC API server for Apiary.
// 
// This file contains the main gRPC server structure, constructor, and error handling.
// RPC method implementations are organized into separate files:
//   - grpc_cells.go: Cells RPC methods
//   - grpc_agentspecs.go: AgentSpecs RPC methods
//   - grpc_hives.go: Hives RPC methods
//   - grpc_secrets.go: Secrets RPC methods
//   - grpc_drones.go: Drones RPC methods
//   - grpc_convert.go: Conversion functions between protobuf and internal types
package api

import (
	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/internal/auth"
	"github.com/agentapiary/apiary/internal/comb"
	"github.com/agentapiary/apiary/internal/dlq"
	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/internal/session"
	"github.com/agentapiary/apiary/pkg/apiary"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"go.uber.org/zap"
)

// GRPCServer represents the gRPC API server for Apiary.
// It implements the ApiaryServiceServer interface defined in the protobuf schema.
type GRPCServer struct {
	v1.UnimplementedApiaryServiceServer
	store      apiary.ResourceStore
	sessionMgr *session.Manager
	comb       comb.Comb
	metrics    *metrics.Collector
	rbac       *auth.RBAC
	dlqMgr     *dlq.Manager
	logger     *zap.Logger
}

// NewGRPCServer creates a new gRPC API server with the provided configuration.
func NewGRPCServer(cfg Config) *GRPCServer {
	return &GRPCServer{
		store:      cfg.Store,
		sessionMgr: cfg.SessionMgr,
		comb:       cfg.Comb,
		metrics:    cfg.Metrics,
		rbac:       cfg.RBAC,
		dlqMgr:     cfg.DLQManager,
		logger:     cfg.Logger,
	}
}

// handleError converts an error to a gRPC status error.
// It maps internal error types to appropriate gRPC status codes.
func (s *GRPCServer) handleError(err error) error {
	if err == apiary.ErrNotFound {
		return status.Error(codes.NotFound, err.Error())
	}
	if err == apiary.ErrAlreadyExists {
		return status.Error(codes.AlreadyExists, err.Error())
	}
	if err == apiary.ErrInvalidInput {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if s.logger != nil {
		s.logger.Error("internal server error", zap.Error(err))
	}
	return status.Error(codes.Internal, "An internal error occurred")
}
