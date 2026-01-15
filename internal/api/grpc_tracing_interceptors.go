// Package api provides OpenTelemetry tracing interceptors for the gRPC API server.
package api

import (
	"context"
	"time"

	"github.com/agentapiary/apiary/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UnaryTracingInterceptor creates a unary gRPC interceptor for OpenTelemetry tracing.
func UnaryTracingInterceptor(obs *observability.Observability) grpc.UnaryServerInterceptor {
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract trace context from gRPC metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = propagator.Extract(ctx, metadataTextMapCarrier(md))
		}

		// Start span
		ctx, span := obs.Tracer().Start(
			ctx,
			info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", info.FullMethod),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		startTime := time.Now()

		// Execute handler
		resp, err := handler(ctx, req)

		// Record duration
		duration := time.Since(startTime)
		span.SetAttributes(
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		// Record response status
		if err != nil {
			st, ok := status.FromError(err)
			if ok {
				span.SetAttributes(
					attribute.Int("rpc.grpc.status_code", int(st.Code())),
				)
				span.SetStatus(codes.Error, st.Message())
			} else {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
		} else {
			span.SetAttributes(
				attribute.Int("rpc.grpc.status_code", 0), // OK
			)
			span.SetStatus(codes.Ok, "")
		}

		return resp, err
	}
}

// StreamTracingInterceptor creates a stream gRPC interceptor for OpenTelemetry tracing.
func StreamTracingInterceptor(obs *observability.Observability) grpc.StreamServerInterceptor {
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		// Extract trace context from gRPC metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			ctx = propagator.Extract(ctx, metadataTextMapCarrier(md))
		}

		// Start span
		ctx, span := obs.Tracer().Start(
			ctx,
			info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", info.FullMethod),
				attribute.String("rpc.method", info.FullMethod),
				attribute.Bool("rpc.stream", true),
			),
		)
		defer span.End()

		startTime := time.Now()

		// Wrap server stream with context
		wrappedStream := &tracingServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// Execute handler
		err := handler(srv, wrappedStream)

		// Record duration
		duration := time.Since(startTime)
		span.SetAttributes(
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		// Record response status
		if err != nil {
			st, ok := status.FromError(err)
			if ok {
				span.SetAttributes(
					attribute.Int("rpc.grpc.status_code", int(st.Code())),
				)
				span.SetStatus(codes.Error, st.Message())
			} else {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
		} else {
			span.SetAttributes(
				attribute.Int("rpc.grpc.status_code", 0), // OK
			)
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// tracingServerStream wraps a gRPC ServerStream to propagate context.
type tracingServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *tracingServerStream) Context() context.Context {
	return w.ctx
}

// metadataTextMapCarrier adapts gRPC metadata to OpenTelemetry's TextMapCarrier.
type metadataTextMapCarrier metadata.MD

func (m metadataTextMapCarrier) Get(key string) string {
	values := metadata.MD(m).Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (m metadataTextMapCarrier) Set(key, value string) {
	metadata.MD(m).Set(key, value)
}

func (m metadataTextMapCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range metadata.MD(m) {
		keys = append(keys, k)
	}
	return keys
}
