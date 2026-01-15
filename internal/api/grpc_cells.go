// Package api provides gRPC RPC methods for Cells.
//
// This file implements the Cells RPC methods:
//   - ListCells: List all Cells
//   - GetCell: Get a Cell by name
//   - CreateCell: Create a new Cell
//   - UpdateCell: Update an existing Cell
//   - DeleteCell: Delete a Cell by name
package api

import (
	"context"

	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/pkg/apiary"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ListCells lists all Cells.
// This RPC method mirrors GET /api/v1/cells.
func (s *GRPCServer) ListCells(ctx context.Context, _ *emptypb.Empty) (*v1.ListCellsResponse, error) {
	resources, err := s.store.List(ctx, "Cell", "", nil)
	if err != nil {
		return nil, s.handleError(err)
	}

	cells := make([]*v1.Cell, 0, len(resources))
	for _, r := range resources {
		if cell, ok := r.(*apiary.Cell); ok {
			cells = append(cells, cellToProto(cell))
		}
	}

	return &v1.ListCellsResponse{
		Cells: cells,
	}, nil
}

// GetCell retrieves a Cell by name.
// This RPC method mirrors GET /api/v1/cells/:name.
func (s *GRPCServer) GetCell(ctx context.Context, req *v1.GetCellRequest) (*v1.Cell, error) {
	if req == nil || req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	resource, err := s.store.Get(ctx, "Cell", req.GetName(), "")
	if err != nil {
		return nil, s.handleError(err)
	}

	cell, ok := resource.(*apiary.Cell)
	if !ok {
		return nil, status.Error(codes.Internal, "resource is not a Cell")
	}

	return cellToProto(cell), nil
}

// CreateCell creates a new Cell.
// This RPC method mirrors POST /api/v1/cells.
func (s *GRPCServer) CreateCell(ctx context.Context, req *v1.CreateCellRequest) (*v1.Cell, error) {
	if req == nil || req.GetCell() == nil {
		return nil, status.Error(codes.InvalidArgument, "cell is required")
	}

	cell := cellFromProto(req.GetCell())

	// Validate namespace (cell name)
	if err := validateNamespace(cell.GetName()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.store.Create(ctx, cell); err != nil {
		return nil, s.handleError(err)
	}

	return cellToProto(cell), nil
}

// UpdateCell updates an existing Cell.
// This RPC method mirrors PUT /api/v1/cells/:name.
func (s *GRPCServer) UpdateCell(ctx context.Context, req *v1.UpdateCellRequest) (*v1.Cell, error) {
	if req == nil || req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetCell() == nil {
		return nil, status.Error(codes.InvalidArgument, "cell is required")
	}

	cell := cellFromProto(req.GetCell())

	// Validate name matches
	if cell.GetName() != req.GetName() {
		return nil, status.Error(codes.InvalidArgument, "name in request does not match name in cell")
	}

	if err := s.store.Update(ctx, cell); err != nil {
		return nil, s.handleError(err)
	}

	return cellToProto(cell), nil
}

// DeleteCell deletes a Cell by name.
// This RPC method mirrors DELETE /api/v1/cells/:name.
func (s *GRPCServer) DeleteCell(ctx context.Context, req *v1.DeleteCellRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if err := s.store.Delete(ctx, "Cell", req.GetName(), ""); err != nil {
		return nil, s.handleError(err)
	}

	return &emptypb.Empty{}, nil
}
