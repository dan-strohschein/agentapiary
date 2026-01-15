// Package api provides gRPC RPC methods for Drones.
//
// This file implements the Drones RPC methods:
//   - ListDrones: List all Drones in a namespace
//   - GetDrone: Get a Drone by ID
//   - DeleteDrone: Delete a Drone by ID
//   - GetDroneLogs: Stream logs from a Drone (server-side streaming)
//   - ExecDrone: Execute a command in a Drone (bidirectional streaming)
//   - DrainDrone: Drain a Drone gracefully
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentapiary/apiary/api/proto/v1"
	"github.com/agentapiary/apiary/pkg/apiary"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ListDrones lists all Drones in a namespace.
// This RPC method mirrors GET /api/v1/cells/:namespace/drones.
func (s *GRPCServer) ListDrones(ctx context.Context, req *v1.ListDronesRequest) (*v1.ListDronesResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	resources, err := s.store.List(ctx, "Drone", req.GetNamespace(), nil)
	if err != nil {
		return nil, s.handleError(err)
	}

	drones := make([]*v1.Drone, 0, len(resources))
	for _, r := range resources {
		if drone, ok := r.(*apiary.Drone); ok {
			drones = append(drones, droneToProto(drone))
		}
	}

	return &v1.ListDronesResponse{
		Drones: drones,
	}, nil
}

// GetDrone retrieves a Drone by ID.
// This RPC method mirrors GET /api/v1/cells/:namespace/drones/:id.
func (s *GRPCServer) GetDrone(ctx context.Context, req *v1.GetDroneRequest) (*v1.Drone, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	resource, err := s.store.Get(ctx, "Drone", req.GetId(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	drone, ok := resource.(*apiary.Drone)
	if !ok {
		return nil, status.Error(codes.Internal, "resource is not a Drone")
	}

	return droneToProto(drone), nil
}

// DeleteDrone deletes a Drone by ID.
// This RPC method mirrors DELETE /api/v1/cells/:namespace/drones/:id.
func (s *GRPCServer) DeleteDrone(ctx context.Context, req *v1.DeleteDroneRequest) (*emptypb.Empty, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.store.Delete(ctx, "Drone", req.GetId(), req.GetNamespace()); err != nil {
		return nil, s.handleError(err)
	}

	return &emptypb.Empty{}, nil
}

// GetDroneLogs streams logs from a Drone (server-side streaming).
// This RPC method mirrors GET /api/v1/cells/:namespace/drones/:id/logs.
func (s *GRPCServer) GetDroneLogs(req *v1.GetDroneLogsRequest, stream grpc.ServerStreamingServer[v1.GetDroneLogsResponse]) error {
	if req == nil || req.GetNamespace() == "" {
		return status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetId() == "" {
		return status.Error(codes.InvalidArgument, "id is required")
	}

	ctx := stream.Context()

	// Get the Drone
	resource, err := s.store.Get(ctx, "Drone", req.GetId(), req.GetNamespace())
	if err != nil {
		return s.handleError(err)
	}

	drone, ok := resource.(*apiary.Drone)
	if !ok {
		return status.Error(codes.Internal, "failed to cast resource to Drone")
	}

	// Extract Keeper address from Drone status
	keeperAddr := drone.Status.KeeperAddr
	if keeperAddr == "" {
		return status.Error(codes.NotFound, "Keeper address not found for drone")
	}

	// Build Keeper logs URL
	logsURL := fmt.Sprintf("http://%s/logs", keeperAddr)
	params := []string{}
	if req.GetTail() != "" {
		params = append(params, fmt.Sprintf("tail=%s", req.GetTail()))
	}
	if req.GetSince() != "" {
		params = append(params, fmt.Sprintf("since=%s", req.GetSince()))
	}
	if len(params) > 0 {
		logsURL += "?" + strings.Join(params, "&")
	}

	// Fetch logs from Keeper
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, logsURL, nil)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to create request: %v", err))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to fetch logs from Keeper: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return status.Error(codes.Internal, fmt.Sprintf("Keeper returned status %d: %s", resp.StatusCode, string(body)))
	}

	// Stream logs in chunks
	buf := make([]byte, 4096) // 4KB chunks
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if err := stream.Send(&v1.GetDroneLogsResponse{
				Logs: buf[:n],
			}); err != nil {
				return err
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("failed to read logs: %v", err))
		}
	}

	return nil
}

// ExecDrone executes a command in a Drone (bidirectional streaming).
// This RPC method mirrors POST /api/v1/cells/:namespace/drones/:id/exec.
func (s *GRPCServer) ExecDrone(stream grpc.BidiStreamingServer[v1.ExecDroneRequest, v1.ExecDroneResponse]) error {
	ctx := stream.Context()

	// Receive initial request to get namespace and ID
	req, err := stream.Recv()
	if err == io.EOF {
		return status.Error(codes.InvalidArgument, "no request received")
	}
	if err != nil {
		return err
	}

	if req.GetNamespace() == "" {
		return status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetId() == "" {
		return status.Error(codes.InvalidArgument, "id is required")
	}

	// Get the Drone
	resource, err := s.store.Get(ctx, "Drone", req.GetId(), req.GetNamespace())
	if err != nil {
		return s.handleError(err)
	}

	drone, ok := resource.(*apiary.Drone)
	if !ok {
		return status.Error(codes.Internal, "failed to cast resource to Drone")
	}

	// Extract Keeper address from Drone status
	keeperAddr := drone.Status.KeeperAddr
	if keeperAddr == "" {
		return status.Error(codes.NotFound, "Keeper address not found for drone")
	}

	// Validate command
	if len(req.GetCommand()) == 0 {
		return status.Error(codes.InvalidArgument, "command is required")
	}

	// Build Keeper exec URL
	execURL := fmt.Sprintf("http://%s/exec", keeperAddr)

	// Create HTTP request body
	reqBody := struct {
		Command []string `json:"command"`
		Stdin   bool     `json:"stdin"`
		TTY     bool     `json:"tty"`
	}{
		Command: req.GetCommand(),
		Stdin:   req.GetStdin(),
		TTY:     req.GetTty(),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to marshal request: %v", err))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, execURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to create request: %v", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to execute command on Keeper: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return status.Error(codes.Internal, fmt.Sprintf("Keeper returned status %d: %s", resp.StatusCode, string(body)))
	}

	// Stream output in chunks
	buf := make([]byte, 4096) // 4KB chunks
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if err := stream.Send(&v1.ExecDroneResponse{
				Stdout: buf[:n],
			}); err != nil {
				return err
			}
		}
		if err == io.EOF {
			// Send final response with exit code
			if err := stream.Send(&v1.ExecDroneResponse{
				ExitCode: 0, // In a real implementation, we'd get this from Keeper
			}); err != nil {
				return err
			}
			break
		}
		if err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("failed to read output: %v", err))
		}
	}

	return nil
}

// DrainDrone drains a Drone gracefully.
// This RPC method mirrors POST /api/v1/cells/:namespace/drones/:id/drain.
func (s *GRPCServer) DrainDrone(ctx context.Context, req *v1.DrainDroneRequest) (*v1.DrainDroneResponse, error) {
	if req == nil || req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Get the Drone
	resource, err := s.store.Get(ctx, "Drone", req.GetId(), req.GetNamespace())
	if err != nil {
		return nil, s.handleError(err)
	}

	drone, ok := resource.(*apiary.Drone)
	if !ok {
		return nil, status.Error(codes.Internal, "failed to cast resource to Drone")
	}

	// Parse timeout
	timeoutDuration := 30 * time.Second
	if req.GetTimeout() != "" {
		var err error
		timeoutDuration, err = time.ParseDuration(req.GetTimeout())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid timeout format: %v", err))
		}
	}

	// Mark Drone as draining by adding annotation
	if drone.ObjectMeta.Annotations == nil {
		drone.ObjectMeta.Annotations = make(map[string]string)
	}
	drone.ObjectMeta.Annotations["draining"] = "true"
	drone.ObjectMeta.Annotations["drain-requested-at"] = time.Now().Format(time.RFC3339)

	// Update Drone phase to stopping
	drone.Status.Phase = apiary.DronePhaseStopping

	// Update the Drone
	if err := s.store.Update(ctx, drone); err != nil {
		return nil, s.handleError(err)
	}

	// If force is true, delete immediately
	if req.GetForce() {
		if err := s.store.Delete(ctx, "Drone", req.GetId(), req.GetNamespace()); err != nil {
			return nil, s.handleError(err)
		}
		return &v1.DrainDroneResponse{
			Message: fmt.Sprintf("Drone %s force drained and deleted", req.GetId()),
		}, nil
	}

	// For graceful drain, we would:
	// 1. Wait for in-flight requests to complete (via Keeper health check)
	// 2. Remove from rotation (Queen will stop routing to it)
	// 3. Wait for timeout or completion
	// 4. Delete the Drone

	// Create a context with timeout for draining
	drainCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	// Poll for drain completion
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-drainCtx.Done():
			// Timeout reached, force delete
			if err := s.store.Delete(ctx, "Drone", req.GetId(), req.GetNamespace()); err != nil {
				return nil, s.handleError(err)
			}
			return &v1.DrainDroneResponse{
				Message: fmt.Sprintf("Drone %s drained (timeout reached) and deleted", req.GetId()),
			}, nil
		case <-ticker.C:
			// Check if Drone is ready to be deleted
			// In a full implementation, we'd check Keeper health and in-flight requests
			// For now, we'll wait a bit and then delete
			updatedResource, err := s.store.Get(ctx, "Drone", req.GetId(), req.GetNamespace())
			if err != nil {
				// Drone already deleted
				return &v1.DrainDroneResponse{
					Message: fmt.Sprintf("Drone %s drained and deleted", req.GetId()),
				}, nil
			}

			updatedDrone, ok := updatedResource.(*apiary.Drone)
			if !ok {
				return nil, status.Error(codes.Internal, "failed to cast resource to Drone")
			}

			// Check if Drone is stopped
			if updatedDrone.Status.Phase == apiary.DronePhaseStopped {
				// Delete the Drone
				if err := s.store.Delete(ctx, "Drone", req.GetId(), req.GetNamespace()); err != nil {
					return nil, s.handleError(err)
				}
				return &v1.DrainDroneResponse{
					Message: fmt.Sprintf("Drone %s drained and deleted", req.GetId()),
				}, nil
			}
		}
	}
}
