// Package observability provides trace correlation utilities for logging.
package observability

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TraceContextFromContext extracts trace context from a context and returns zap fields for logging.
func TraceContextFromContext(ctx context.Context) []zap.Field {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}

	spanContext := span.SpanContext()
	if !spanContext.IsValid() {
		return nil
	}

	fields := []zap.Field{
		zap.String("trace_id", spanContext.TraceID().String()),
		zap.String("span_id", spanContext.SpanID().String()),
	}

	if spanContext.TraceFlags().IsSampled() {
		fields = append(fields, zap.Bool("trace_sampled", true))
	}

	return fields
}

// LoggerWithTraceContext creates a logger with trace context fields added.
func LoggerWithTraceContext(ctx context.Context, baseLogger *zap.Logger) *zap.Logger {
	if baseLogger == nil {
		return nil
	}

	fields := TraceContextFromContext(ctx)
	if len(fields) == 0 {
		return baseLogger
	}

	return baseLogger.With(fields...)
}
