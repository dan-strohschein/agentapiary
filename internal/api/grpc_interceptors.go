// Package api provides gRPC interceptors for authentication, logging, and metrics.
//
// This file implements gRPC interceptors:
//   - LoggingInterceptor: Logs RPC calls with method, status, latency
//   - AuthInterceptor: Extracts principal from metadata and adds to context
//   - AuthorizationInterceptor: Checks permissions based on RPC method
//   - MetricsInterceptor: Records metrics for RPC calls
package api

import (
	"context"
	"time"

	"github.com/agentapiary/apiary/internal/auth"
	"github.com/agentapiary/apiary/internal/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// extractPrincipalFromMetadata extracts principal from gRPC metadata (equivalent to HTTP headers).
//
// SECURITY WARNING: This is an unauthenticated, metadata-based approach that trusts
// user-provided metadata without verification. This MUST NOT be used in production.
//
// In production, implement proper authentication:
//   - JWT validation with signature verification
//   - API keys with HMAC validation
//   - OAuth2/OIDC integration
//   - At minimum, add request signing/HMAC verification
//
// This MVP implementation should be removed before any production deployment.
func extractPrincipalFromMetadata(ctx context.Context) *auth.Principal {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	// For MVP, use x-user-id metadata (UNVERIFIED - DO NOT USE IN PRODUCTION)
	userIDs := md.Get("x-user-id")
	if len(userIDs) == 0 || userIDs[0] == "" {
		return nil
	}
	userID := userIDs[0]

	// Extract user name from metadata (optional)
	userNames := md.Get("x-user-name")
	userName := userID
	if len(userNames) > 0 && userNames[0] != "" {
		userName = userNames[0]
	}

	return &auth.Principal{
		ID:   userID,
		Name: userName,
	}
}

// getPermissionFromMethod determines the required permission based on the RPC method name.
func getPermissionFromMethod(method string) auth.Permission {
	// Extract method name (format: /apiary.v1.ApiaryService/MethodName)
	methodName := method
	if idx := len(method) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if method[i] == '/' {
				if i+1 < len(method) {
					methodName = method[i+1:]
				}
				break
			}
		}
	}

	// Determine permission based on method name prefix
	switch {
	case methodName == "ListCells" || methodName == "GetCell" ||
		methodName == "ListAgentSpecs" || methodName == "GetAgentSpec" ||
		methodName == "ListHives" || methodName == "GetHive" ||
		methodName == "ListSecrets" || methodName == "GetSecret" ||
		methodName == "ListDrones" || methodName == "GetDrone" ||
		methodName == "GetDroneLogs":
		return auth.PermissionRead
	case methodName == "CreateCell" || methodName == "CreateAgentSpec" ||
		methodName == "CreateHive" || methodName == "CreateSecret":
		return auth.PermissionCreate
	case methodName == "UpdateCell" || methodName == "UpdateAgentSpec" ||
		methodName == "UpdateHive" || methodName == "UpdateSecret" ||
		methodName == "ScaleAgentSpec" || methodName == "ScaleHive":
		return auth.PermissionUpdate
	case methodName == "DeleteCell" || methodName == "DeleteAgentSpec" ||
		methodName == "DeleteHive" || methodName == "DeleteSecret" ||
		methodName == "DeleteDrone" || methodName == "DrainDrone" ||
		methodName == "DrainAgentSpec":
		return auth.PermissionDelete
	default:
		// For methods we don't recognize, default to read permission
		return auth.PermissionRead
	}
}

// isNamespacedMethod checks if an RPC method requires namespace-based authorization.
func isNamespacedMethod(method string) bool {
	// Extract method name
	methodName := method
	if idx := len(method) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if method[i] == '/' {
				if i+1 < len(method) {
					methodName = method[i+1:]
				}
				break
			}
		}
	}

	// ListCells is not namespaced
	if methodName == "ListCells" {
		return false
	}

	// All other methods are namespaced (they take namespace as a parameter)
	return true
}

// UnaryAuthInterceptor creates a unary interceptor that extracts principal from metadata.
func UnaryAuthInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract principal from metadata
		principal := extractPrincipalFromMetadata(ctx)
		if principal != nil {
			ctx = auth.WithPrincipal(ctx, principal)
		}
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor creates a stream interceptor that extracts principal from metadata.
func StreamAuthInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		// Extract principal from metadata
		principal := extractPrincipalFromMetadata(ctx)
		if principal != nil {
			ctx = auth.WithPrincipal(ctx, principal)
		}
		// Create a new stream with the updated context
		// Note: gRPC doesn't provide a way to change context on a stream,
		// so we rely on the handler to use the context from the stream
		return handler(srv, ss)
	}
}

// UnaryAuthorizationInterceptor creates a unary interceptor that checks authorization.
func UnaryAuthorizationInterceptor(rbac *auth.RBAC, requireAuth bool) grpc.UnaryServerInterceptor {
	if rbac == nil {
		// No RBAC configured, return passthrough interceptor
		return func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip authorization for non-namespaced methods (ListCells)
		if !isNamespacedMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		// Get principal from context
		principal, ok := auth.PrincipalFromContext(ctx)
		if !ok {
			if requireAuth {
				return nil, status.Error(codes.Unauthenticated, "Authentication required")
			}
			// Development/testing mode: allow unauthenticated access
			return handler(ctx, req)
		}

		// Extract namespace from request
		// For now, we'll extract namespace from the request based on common patterns
		// This is a simplified approach - in production, we'd use reflection or a registry
		namespace := extractNamespaceFromRequest(req)
		if namespace == "" {
			// No namespace in request, allow (handler will validate)
			return handler(ctx, req)
		}

		// Determine permission based on method
		permission := getPermissionFromMethod(info.FullMethod)

		// Check permission
		if !rbac.CheckPermission(principal, namespace, permission) {
			return nil, status.Error(codes.PermissionDenied, "Insufficient permissions")
		}

		return handler(ctx, req)
	}
}

// StreamAuthorizationInterceptor creates a stream interceptor that checks authorization.
func StreamAuthorizationInterceptor(rbac *auth.RBAC, requireAuth bool) grpc.StreamServerInterceptor {
	if rbac == nil {
		// No RBAC configured, return passthrough interceptor
		return func(
			srv interface{},
			ss grpc.ServerStream,
			info *grpc.StreamServerInfo,
			handler grpc.StreamHandler,
		) error {
			return handler(srv, ss)
		}
	}

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		// Skip authorization for non-namespaced methods
		if !isNamespacedMethod(info.FullMethod) {
			return handler(srv, ss)
		}

		// Get principal from context
		principal, ok := auth.PrincipalFromContext(ctx)
		if !ok {
			if requireAuth {
				return status.Error(codes.Unauthenticated, "Authentication required")
			}
			// Development/testing mode: allow unauthenticated access
			return handler(srv, ss)
		}

		// For streaming methods, we can't easily extract namespace from the first message
		// For now, we'll allow and let the handler validate
		// In production, we might read the first message to extract namespace
		_ = principal // Use principal to avoid unused variable warning
		return handler(srv, ss)
	}
}

// extractNamespaceFromRequest extracts namespace from a request message.
// This is a simplified approach - in production, we'd use reflection or a registry.
func extractNamespaceFromRequest(req interface{}) string {
	// Use type assertion to extract namespace from common request types
	// This is a simplified approach - in production, we'd use reflection
	switch r := req.(type) {
	case interface{ GetNamespace() string }:
		return r.GetNamespace()
	default:
		return ""
	}
}

// UnaryLoggingInterceptor creates a unary interceptor that logs RPC calls.
func UnaryLoggingInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	if logger == nil {
		// No logger configured, return passthrough interceptor
		return func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)
		code := status.Code(err)

		logger.Info("grpc request",
			zap.String("method", info.FullMethod),
			zap.String("code", code.String()),
			zap.Duration("latency", duration),
			zap.Error(err),
		)

		return resp, err
	}
}

// StreamLoggingInterceptor creates a stream interceptor that logs RPC calls.
func StreamLoggingInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	if logger == nil {
		// No logger configured, return passthrough interceptor
		return func(
			srv interface{},
			ss grpc.ServerStream,
			info *grpc.StreamServerInfo,
			handler grpc.StreamHandler,
		) error {
			return handler(srv, ss)
		}
	}

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		err := handler(srv, ss)

		duration := time.Since(start)
		code := status.Code(err)

		logger.Info("grpc stream",
			zap.String("method", info.FullMethod),
			zap.String("code", code.String()),
			zap.Duration("latency", duration),
			zap.Error(err),
		)

		return err
	}
}

// UnaryMetricsInterceptor creates a unary interceptor that records metrics.
func UnaryMetricsInterceptor(metrics *metrics.Collector) grpc.UnaryServerInterceptor {
	if metrics == nil {
		// No metrics configured, return passthrough interceptor
		return func(
			ctx context.Context,
			req interface{},
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		// Extract method name for metrics
		methodName := info.FullMethod
		if idx := len(methodName) - 1; idx >= 0 {
			for i := idx; i >= 0; i-- {
				if methodName[i] == '/' {
					if i+1 < len(methodName) {
						methodName = methodName[i+1:]
					}
					break
				}
			}
		}

		// Record metrics (simplified - in production, we'd record more detailed metrics)
		_ = methodName // Use methodName to avoid unused variable warning
		_ = duration   // Use duration to avoid unused variable warning
		// TODO: Record metrics when metrics collector supports gRPC metrics
		// For now, we'll just record the call
		// metrics.RecordRPC(methodName, duration, err)

		return resp, err
	}
}

// StreamMetricsInterceptor creates a stream interceptor that records metrics.
func StreamMetricsInterceptor(metrics *metrics.Collector) grpc.StreamServerInterceptor {
	if metrics == nil {
		// No metrics configured, return passthrough interceptor
		return func(
			srv interface{},
			ss grpc.ServerStream,
			info *grpc.StreamServerInfo,
			handler grpc.StreamHandler,
		) error {
			return handler(srv, ss)
		}
	}

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		err := handler(srv, ss)

		duration := time.Since(start)

		// Extract method name for metrics
		methodName := info.FullMethod
		if idx := len(methodName) - 1; idx >= 0 {
			for i := idx; i >= 0; i-- {
				if methodName[i] == '/' {
					if i+1 < len(methodName) {
						methodName = methodName[i+1:]
					}
					break
				}
			}
		}

		// Record metrics (simplified - in production, we'd record more detailed metrics)
		_ = methodName // Use methodName to avoid unused variable warning
		_ = duration   // Use duration to avoid unused variable warning
		// TODO: Record metrics when metrics collector supports gRPC metrics
		// For now, we'll just record the call
		// metrics.RecordRPC(methodName, duration, err)

		return err
	}
}
