// Package observability provides OpenTelemetry tracing and metrics setup for Apiary.
package observability

import (
	"context"
	"fmt"
	"os"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.31.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Config holds observability configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Logger         *zap.Logger
}

// Observability manages OpenTelemetry tracing and metrics.
type Observability struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	tracer         trace.Tracer
	meter          metric.Meter
	logger         *zap.Logger
	
	// Metrics
	requestCounter     metric.Int64Counter
	requestDuration    metric.Float64Histogram
	requestSize        metric.Int64Histogram
	responseSize       metric.Int64Histogram
	
	mu sync.RWMutex
}

// NewObservability creates and initializes OpenTelemetry tracing and metrics.
func NewObservability(cfg Config) (*Observability, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "apiary"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "1.0.0"
	}
	if cfg.Environment == "" {
		cfg.Environment = "production"
	}

	obs := &Observability{
		logger: cfg.Logger,
	}

	// Create resource with service information
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize tracing
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
		stdouttrace.WithWriter(os.Stderr),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	obs.tracerProvider = tp
	obs.tracer = otel.Tracer(cfg.ServiceName)

	// Initialize metrics
	metricExporter, err := stdoutmetric.New(
		stdoutmetric.WithPrettyPrint(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(mp)
	obs.meterProvider = mp
	obs.meter = otel.Meter(cfg.ServiceName)

	// Create metrics instruments
	if err := obs.createMetrics(); err != nil {
		return nil, fmt.Errorf("failed to create metrics: %w", err)
	}

	return obs, nil
}

// createMetrics creates OpenTelemetry metrics instruments.
func (o *Observability) createMetrics() error {
	var err error

	o.requestCounter, err = o.meter.Int64Counter(
		"apiary_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request counter: %w", err)
	}

	o.requestDuration, err = o.meter.Float64Histogram(
		"apiary_request_duration_seconds",
		metric.WithDescription("Request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request duration histogram: %w", err)
	}

	o.requestSize, err = o.meter.Int64Histogram(
		"apiary_request_size_bytes",
		metric.WithDescription("Request size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request size histogram: %w", err)
	}

	o.responseSize, err = o.meter.Int64Histogram(
		"apiary_response_size_bytes",
		metric.WithDescription("Response size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return fmt.Errorf("failed to create response size histogram: %w", err)
	}

	return nil
}

// Tracer returns the OpenTelemetry tracer.
func (o *Observability) Tracer() trace.Tracer {
	return o.tracer
}

// Meter returns the OpenTelemetry meter.
func (o *Observability) Meter() metric.Meter {
	return o.meter
}

// RequestCounter returns the request counter metric.
func (o *Observability) RequestCounter() metric.Int64Counter {
	return o.requestCounter
}

// RequestDuration returns the request duration histogram.
func (o *Observability) RequestDuration() metric.Float64Histogram {
	return o.requestDuration
}

// RequestSize returns the request size histogram.
func (o *Observability) RequestSize() metric.Int64Histogram {
	return o.requestSize
}

// ResponseSize returns the response size histogram.
func (o *Observability) ResponseSize() metric.Int64Histogram {
	return o.responseSize
}

// StartSpan starts a new span with the given name and options.
func (o *Observability) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return o.tracer.Start(ctx, name, opts...)
}

// Shutdown gracefully shuts down the observability components.
func (o *Observability) Shutdown(ctx context.Context) error {
	var errs []error

	if o.tracerProvider != nil {
		if err := o.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
		}
	}

	if o.meterProvider != nil {
		if err := o.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}
