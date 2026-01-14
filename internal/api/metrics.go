package api

import (
	"net/http"

	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// Ensure metrics package is imported for type usage in this file
var _ *metrics.Collector = nil

// getMetrics handles GET /api/v1/metrics.
func (s *Server) getMetrics(c echo.Context) error {
	// Return Prometheus metrics if collector is available
	if s.metrics == nil {
		return c.JSON(http.StatusNotImplemented, errorResponse{
			Error:   "not_implemented",
			Message: "Metrics collection not enabled",
		})
	}

	// Return Prometheus format
	c.Response().Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.metrics.Handler().ServeHTTP(c.Response(), c.Request())
	return nil
}

// getNamespaceMetrics handles GET /api/v1/cells/:namespace/metrics.
func (s *Server) getNamespaceMetrics(c echo.Context) error {
	namespace := c.Param("namespace")

	if s.metrics == nil {
		return c.JSON(http.StatusNotImplemented, errorResponse{
			Error:   "not_implemented",
			Message: "Metrics collection not enabled",
		})
	}

	// Get all AgentSpecs in namespace
	ctx := c.Request().Context()
	resources, err := s.store.List(ctx, "AgentSpec", namespace, nil)
	if err != nil {
		return s.handleError(c, err)
	}

	// Aggregate metrics for all AgentSpecs in namespace
	namespaceMetrics := make(map[string]interface{})
	
		for _, resource := range resources {
			if spec, ok := resource.(*apiary.AgentSpec); ok {
				agentspec := spec.GetName()
				agentMetrics := make(map[string]interface{})
				
				// Get latency stats
				if latencyStats, err := s.metrics.GetLatencyStats(namespace, agentspec); err == nil {
					agentMetrics["latency"] = map[string]interface{}{
						"p50": latencyStats.P50.Milliseconds(),
						"p95": latencyStats.P95.Milliseconds(),
						"p99": latencyStats.P99.Milliseconds(),
					}
				}
				
				// Get token usage
				if tokens, err := s.metrics.GetTokenUsage(namespace, agentspec); err == nil {
					agentMetrics["tokens"] = tokens
				}
				
				// Get queue depth
				if depth, err := s.metrics.GetQueueDepth(namespace, agentspec); err == nil {
					agentMetrics["queueDepth"] = depth
				}
				
				namespaceMetrics[agentspec] = agentMetrics
			}
		}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"namespace": namespace,
		"metrics":   namespaceMetrics,
	})
}

// getAgentSpecMetrics handles GET /api/v1/cells/:namespace/metrics/:agentspec.
func (s *Server) getAgentSpecMetrics(c echo.Context) error {
	namespace := c.Param("namespace")
	agentspec := c.Param("agentspec")

	if s.metrics == nil {
		return c.JSON(http.StatusNotImplemented, errorResponse{
			Error:   "not_implemented",
			Message: "Metrics collection not enabled",
		})
	}

	// Get latency stats
	latencyStats, err := s.metrics.GetLatencyStats(namespace, agentspec)
	if err != nil {
		s.logger.Debug("No latency metrics found",
			zap.String("namespace", namespace),
			zap.String("agentspec", agentspec),
		)
	}

	// Get token usage
	tokenUsage, err := s.metrics.GetTokenUsage(namespace, agentspec)
	if err != nil {
		s.logger.Debug("No token metrics found",
			zap.String("namespace", namespace),
			zap.String("agentspec", agentspec),
		)
	}

	// Get queue depth
	queueDepth, err := s.metrics.GetQueueDepth(namespace, agentspec)
	if err != nil {
		s.logger.Debug("No queue metrics found",
			zap.String("namespace", namespace),
			zap.String("agentspec", agentspec),
		)
	}

	// Get request rate
	requestRate, err := s.metrics.GetRequestRate(namespace, agentspec)
	if err != nil {
		s.logger.Debug("No rate metrics found",
			zap.String("namespace", namespace),
			zap.String("agentspec", agentspec),
		)
	}

	response := map[string]interface{}{
		"namespace":   namespace,
		"agentspec":   agentspec,
		"tokenUsage":  tokenUsage,
		"queueDepth":  queueDepth,
		"requestRate": requestRate,
	}

	if latencyStats != nil {
		response["latency"] = map[string]interface{}{
			"p50": latencyStats.P50.Milliseconds(),
			"p95": latencyStats.P95.Milliseconds(),
			"p99": latencyStats.P99.Milliseconds(),
			"count": latencyStats.Count,
		}
	}

	return c.JSON(http.StatusOK, response)
}
