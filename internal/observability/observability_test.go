// Package observability provides tests for OpenTelemetry observability infrastructure.
package observability

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestNewObservability(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		Logger:         logger,
	}

	obs, err := NewObservability(cfg)
	if err != nil {
		t.Fatalf("Failed to create observability: %v", err)
	}

	if obs == nil {
		t.Fatal("Observability is nil")
	}

	if obs.Tracer() == nil {
		t.Error("Tracer is nil")
	}

	if obs.Meter() == nil {
		t.Error("Meter is nil")
	}

	if obs.RequestCounter() == nil {
		t.Error("RequestCounter is nil")
	}

	if obs.RequestDuration() == nil {
		t.Error("RequestDuration is nil")
	}

	// Test shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := obs.Shutdown(ctx); err != nil {
		t.Errorf("Failed to shutdown observability: %v", err)
	}
}

func TestStartSpan(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		ServiceName: "test-service",
		Logger:      logger,
	}

	obs, err := NewObservability(cfg)
	if err != nil {
		t.Fatalf("Failed to create observability: %v", err)
	}
	defer obs.Shutdown(context.Background())

	ctx := context.Background()
	ctx, span := obs.StartSpan(ctx, "test-operation")
	if span == nil {
		t.Fatal("Span is nil")
	}

	span.End()
}

func TestTraceContextFromContext(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		ServiceName: "test-service",
		Logger:      logger,
	}

	obs, err := NewObservability(cfg)
	if err != nil {
		t.Fatalf("Failed to create observability: %v", err)
	}
	defer obs.Shutdown(context.Background())

	ctx := context.Background()
	ctx, span := obs.StartSpan(ctx, "test-operation")
	defer span.End()

	fields := TraceContextFromContext(ctx)
	if len(fields) == 0 {
		t.Error("Trace context fields are empty")
	}

	// Check that trace_id and span_id are present
	foundTraceID := false
	foundSpanID := false
	for _, field := range fields {
		if field.Key == "trace_id" {
			foundTraceID = true
		}
		if field.Key == "span_id" {
			foundSpanID = true
		}
	}

	if !foundTraceID {
		t.Error("trace_id field not found")
	}
	if !foundSpanID {
		t.Error("span_id field not found")
	}
}

func TestLoggerWithTraceContext(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := Config{
		ServiceName: "test-service",
		Logger:      logger,
	}

	obs, err := NewObservability(cfg)
	if err != nil {
		t.Fatalf("Failed to create observability: %v", err)
	}
	defer obs.Shutdown(context.Background())

	ctx := context.Background()
	ctx, span := obs.StartSpan(ctx, "test-operation")
	defer span.End()

	traceLogger := LoggerWithTraceContext(ctx, logger)
	if traceLogger == nil {
		t.Fatal("Trace logger is nil")
	}

	// Log a message to verify it works
	traceLogger.Info("test message")
}
