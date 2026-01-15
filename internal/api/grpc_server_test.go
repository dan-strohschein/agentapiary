// Package api provides tests for the gRPC API server.
package api

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/internal/auth"
	"github.com/agentapiary/apiary/internal/comb"
	"github.com/agentapiary/apiary/internal/dlq"
	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/internal/session"
	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// setupTestGRPCServer creates a test gRPC server with all dependencies.
func setupTestGRPCServer(t *testing.T) (v1.ApiaryServiceClient, *grpc.ClientConn, func()) {
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	logger := zap.NewNop()
	combStore := comb.NewStore(logger)
	metricsCollector := metrics.NewCollector(logger)
	rbac := auth.NewRBAC()
	dlqMgr := dlq.NewManager(logger)

	sessionMgr := session.NewManager(session.Config{
		Store:  store,
		Logger: logger,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = sessionMgr.Start(ctx)
	require.NoError(t, err)

	cfg := Config{
		Store:      store,
		SessionMgr: sessionMgr,
		Comb:       combStore,
		Metrics:    metricsCollector,
		RBAC:       rbac,
		DLQManager: dlqMgr,
		Logger:     logger,
	}

	grpcServer := NewGRPCServer(cfg)

	// Create gRPC server
	srv := grpc.NewServer()
	v1.RegisterApiaryServiceServer(srv, grpcServer)

	// Start server on a random port
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	go func() {
		_ = srv.Serve(listener)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create client connection
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	client := v1.NewApiaryServiceClient(conn)

	cleanup := func() {
		conn.Close()
		srv.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sessionMgr.Stop(ctx)
		combStore.Close()
		store.Close()
	}

	return client, conn, cleanup
}

// TestGRPCServer_ListCells tests the ListCells RPC method.
func TestGRPCServer_ListCells(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// Initially, should return empty list
	resp, err := client.ListCells(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Empty(t, resp.GetCells())
}

// TestGRPCServer_GetCell tests the GetCell RPC method.
func TestGRPCServer_GetCell(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// Get non-existent cell
	_, err := client.GetCell(ctx, &v1.GetCellRequest{Name: "non-existent"})
	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

// TestGRPCServer_CreateCell tests the CreateCell RPC method.
func TestGRPCServer_CreateCell(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	cell := &v1.Cell{
		TypeMeta: &v1.TypeMeta{
			ApiVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		Metadata: &v1.ObjectMeta{
			Name: "test-cell",
		},
	}

	resp, err := client.CreateCell(ctx, &v1.CreateCellRequest{Cell: cell})
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-cell", resp.GetMetadata().GetName())

	// Try to create again (should fail with AlreadyExists)
	_, err = client.CreateCell(ctx, &v1.CreateCellRequest{Cell: cell})
	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
}

// TestGRPCServer_UpdateCell tests the UpdateCell RPC method.
func TestGRPCServer_UpdateCell(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// First create a cell
	cell := &v1.Cell{
		TypeMeta: &v1.TypeMeta{
			ApiVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		Metadata: &v1.ObjectMeta{
			Name: "test-cell",
		},
	}
	_, err := client.CreateCell(ctx, &v1.CreateCellRequest{Cell: cell})
	require.NoError(t, err)

	// Update the cell
	cell.Spec = &v1.CellSpec{
		ResourceQuota: &v1.ResourceQuota{
			MaxHives: 10,
		},
	}
	resp, err := client.UpdateCell(ctx, &v1.UpdateCellRequest{
		Name: "test-cell",
		Cell: cell,
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, int32(10), resp.GetSpec().GetResourceQuota().GetMaxHives())
}

// TestGRPCServer_DeleteCell tests the DeleteCell RPC method.
func TestGRPCServer_DeleteCell(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// First create a cell
	cell := &v1.Cell{
		TypeMeta: &v1.TypeMeta{
			ApiVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		Metadata: &v1.ObjectMeta{
			Name: "test-cell",
		},
	}
	_, err := client.CreateCell(ctx, &v1.CreateCellRequest{Cell: cell})
	require.NoError(t, err)

	// Delete the cell
	_, err = client.DeleteCell(ctx, &v1.DeleteCellRequest{Name: "test-cell"})
	require.NoError(t, err)

	// Try to delete again (should fail with NotFound)
	_, err = client.DeleteCell(ctx, &v1.DeleteCellRequest{Name: "test-cell"})
	assert.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}
