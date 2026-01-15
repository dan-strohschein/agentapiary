// Package api provides tracing middleware for the REST API server.
package api

import (
	"net/http"
	"strconv"

	"github.com/agentapiary/apiary/internal/observability"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// tracingMiddleware creates OpenTelemetry tracing middleware for Echo.
func tracingMiddleware(obs *observability.Observability) echo.MiddlewareFunc {
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			req := c.Request()

			// Extract trace context from HTTP headers
			ctx = propagator.Extract(ctx, propagation.HeaderCarrier(req.Header))

			// Start span
			spanName := req.Method + " " + c.Path()
			ctx, span := obs.Tracer().Start(
				ctx,
				spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.method", req.Method),
					attribute.String("http.url", req.URL.String()),
					attribute.String("http.route", c.Path()),
					attribute.String("http.user_agent", req.UserAgent()),
				),
			)
			defer span.End()

			// Store span in context
			c.SetRequest(req.WithContext(ctx))

			// Execute handler
			err := next(c)

			// Record response status and attributes
			status := c.Response().Status
			span.SetAttributes(
				attribute.Int("http.status_code", status),
				attribute.Int("http.response.size", int(c.Response().Size)),
			)

			// Set span status based on HTTP status code
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			} else if status >= http.StatusBadRequest {
				span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(status))
			} else {
				span.SetStatus(codes.Ok, "")
			}

			return err
		}
	}
}
