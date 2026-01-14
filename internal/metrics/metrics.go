// Package metrics provides metrics collection and aggregation for Apiary.
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Collector collects and aggregates metrics.
type Collector struct {
	registry *prometheus.Registry

	// Per-agent metrics
	responseLatency          *prometheus.HistogramVec
	tokenUsage               *prometheus.CounterVec
	queueDepth               *prometheus.GaugeVec
	requestRate              *prometheus.CounterVec
	circuitBreakerState      *prometheus.GaugeVec
	circuitBreakerFailures   *prometheus.CounterVec
	outputValidationFailures *prometheus.CounterVec
	rateLimitExceeded        *prometheus.CounterVec
	dlqMessagesAdded         *prometheus.CounterVec
	dlqQueueSize             *prometheus.GaugeVec

	// Aggregated metrics per AgentSpec
	aggregatedLatency map[string]*LatencyStats
	aggregatedTokens  map[string]int64
	aggregatedQueue   map[string]int
	aggregatedRate    map[string]float64

	// Track last access time for cleanup
	lastAccess map[string]time.Time

	mu sync.RWMutex

	logger *zap.Logger

	// Cleanup configuration
	maxIdleTime     time.Duration
	cleanupInterval time.Duration
	cleanupTicker   *time.Ticker
	cleanupStopCh   chan struct{}
	cleanupWg       sync.WaitGroup
}

// LatencyStats holds latency statistics.
type LatencyStats struct {
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Count int64
	mu    sync.RWMutex
}

// NewCollector creates a new metrics collector.
func NewCollector(logger *zap.Logger) *Collector {
	registry := prometheus.NewRegistry()

	// Response latency histogram (in milliseconds)
	responseLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "apiary_response_latency_ms",
			Help:    "Response latency in milliseconds",
			Buckets: prometheus.ExponentialBuckets(10, 2, 10), // 10ms to 5.12s
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// Token usage counter
	tokenUsage := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "apiary_tokens_used_total",
			Help: "Total tokens used",
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// Queue depth gauge
	queueDepth := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "apiary_queue_depth",
			Help: "Current queue depth",
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// Request rate counter
	requestRate := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "apiary_requests_total",
			Help: "Total requests processed",
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// Circuit breaker state gauge (0=closed, 1=half-open, 2=open)
	circuitBreakerState := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "apiary_circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// Circuit breaker failures counter
	circuitBreakerFailures := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "apiary_circuit_breaker_failures_total",
			Help: "Total circuit breaker failures",
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// Output validation failures counter
	outputValidationFailures := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "apiary_output_validation_failures_total",
			Help: "Total output validation failures",
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// Rate limit exceeded counter
	rateLimitExceeded := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "apiary_rate_limit_exceeded_total",
			Help: "Total rate limit exceeded events",
		},
		[]string{"agentspec", "namespace", "drone_id"},
	)

	// DLQ messages added counter
	dlqMessagesAdded := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "apiary_dlq_messages_added_total",
			Help: "Total messages added to dead letter queue",
		},
		[]string{"namespace", "hive"},
	)

	// DLQ queue size gauge
	dlqQueueSize := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "apiary_dlq_queue_size",
			Help: "Current size of dead letter queue",
		},
		[]string{"namespace", "hive"},
	)

	registry.MustRegister(responseLatency)
	registry.MustRegister(tokenUsage)
	registry.MustRegister(queueDepth)
	registry.MustRegister(requestRate)
	registry.MustRegister(circuitBreakerState)
	registry.MustRegister(circuitBreakerFailures)
	registry.MustRegister(outputValidationFailures)
	registry.MustRegister(rateLimitExceeded)
	registry.MustRegister(dlqMessagesAdded)
	registry.MustRegister(dlqQueueSize)

	c := &Collector{
		registry:                 registry,
		responseLatency:          responseLatency,
		tokenUsage:               tokenUsage,
		queueDepth:               queueDepth,
		requestRate:              requestRate,
		circuitBreakerState:      circuitBreakerState,
		circuitBreakerFailures:   circuitBreakerFailures,
		outputValidationFailures: outputValidationFailures,
		rateLimitExceeded:        rateLimitExceeded,
		dlqMessagesAdded:         dlqMessagesAdded,
		dlqQueueSize:             dlqQueueSize,
		aggregatedLatency:        make(map[string]*LatencyStats),
		aggregatedTokens:         make(map[string]int64),
		aggregatedQueue:          make(map[string]int),
		aggregatedRate:           make(map[string]float64),
		lastAccess:               make(map[string]time.Time),
		logger:                   logger,
		maxIdleTime:              24 * time.Hour, // Default: cleanup after 24 hours of inactivity
		cleanupInterval:          1 * time.Hour,  // Default: run cleanup every hour
		cleanupStopCh:            make(chan struct{}),
	}

	// Start cleanup goroutine
	c.startCleanup()

	return c
}

// RecordLatency records response latency for a drone.
func (c *Collector) RecordLatency(agentspec, namespace, droneID string, latency time.Duration) {
	c.responseLatency.WithLabelValues(agentspec, namespace, droneID).Observe(float64(latency.Milliseconds()))

	// Update aggregated stats
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	stats, exists := c.aggregatedLatency[key]
	if !exists {
		stats = &LatencyStats{}
		c.aggregatedLatency[key] = stats
	}
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()

	// Simple aggregation: track recent latencies for percentile calculation
	// In production, use a proper histogram aggregation
	stats.mu.Lock()
	stats.Count++
	// Simple approximation: update percentiles based on count
	// For production, use proper percentile calculation
	if stats.Count == 1 {
		stats.P50 = latency
		stats.P95 = latency
		stats.P99 = latency
	} else {
		// Approximate percentiles (simplified)
		if latency > stats.P95 {
			stats.P95 = latency
		}
		if latency > stats.P99 {
			stats.P99 = latency
		}
		// P50 is median - simplified to average for now
		stats.P50 = time.Duration((int64(stats.P50) + int64(latency)) / 2)
	}
	stats.mu.Unlock()
}

// RecordTokens records token usage for a drone.
func (c *Collector) RecordTokens(agentspec, namespace, droneID string, tokens int64) {
	c.tokenUsage.WithLabelValues(agentspec, namespace, droneID).Add(float64(tokens))

	// Update aggregated
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	c.aggregatedTokens[key] += tokens
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()
}

// SetQueueDepth sets the queue depth for a drone.
func (c *Collector) SetQueueDepth(agentspec, namespace, droneID string, depth int) {
	c.queueDepth.WithLabelValues(agentspec, namespace, droneID).Set(float64(depth))

	// Update aggregated (max depth)
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	if depth > c.aggregatedQueue[key] {
		c.aggregatedQueue[key] = depth
	}
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()
}

// RecordRequest records a request for a drone.
func (c *Collector) RecordRequest(agentspec, namespace, droneID string) {
	c.requestRate.WithLabelValues(agentspec, namespace, droneID).Inc()

	// Update aggregated rate
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	c.aggregatedRate[key]++
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()
}

// RecordOutputValidationFailure records an output validation failure for a drone.
func (c *Collector) RecordOutputValidationFailure(agentspec, namespace, droneID string) {
	c.outputValidationFailures.WithLabelValues(agentspec, namespace, droneID).Inc()
}

// RecordRateLimitExceeded records a rate limit exceeded event for a drone.
func (c *Collector) RecordRateLimitExceeded(agentspec, namespace, droneID string) {
	c.rateLimitExceeded.WithLabelValues(agentspec, namespace, droneID).Inc()
}

// RecordDLQMessageAdded records a message added to the dead letter queue.
func (c *Collector) RecordDLQMessageAdded(namespace, hive string) {
	c.dlqMessagesAdded.WithLabelValues(namespace, hive).Inc()
}

// SetDLQQueueSize sets the current size of the dead letter queue.
func (c *Collector) SetDLQQueueSize(namespace, hive string, size int) {
	c.dlqQueueSize.WithLabelValues(namespace, hive).Set(float64(size))
}

// GetLatencyStats returns aggregated latency statistics for an AgentSpec.
func (c *Collector) GetLatencyStats(namespace, agentspec string) (*LatencyStats, error) {
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()

	c.mu.RLock()
	stats, exists := c.aggregatedLatency[key]
	if !exists {
		c.mu.RUnlock()
		return nil, fmt.Errorf("no metrics found for %s/%s", namespace, agentspec)
	}

	// Return a copy
	stats.mu.RLock()
	c.mu.RUnlock()
	defer stats.mu.RUnlock()

	return &LatencyStats{
		P50:   stats.P50,
		P95:   stats.P95,
		P99:   stats.P99,
		Count: stats.Count,
	}, nil
}

// GetTokenUsage returns aggregated token usage for an AgentSpec.
func (c *Collector) GetTokenUsage(namespace, agentspec string) (int64, error) {
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()

	c.mu.RLock()
	tokens, exists := c.aggregatedTokens[key]
	c.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("no metrics found for %s/%s", namespace, agentspec)
	}

	return tokens, nil
}

// GetQueueDepth returns aggregated queue depth for an AgentSpec.
func (c *Collector) GetQueueDepth(namespace, agentspec string) (int, error) {
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()

	c.mu.RLock()
	depth, exists := c.aggregatedQueue[key]
	c.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("no metrics found for %s/%s", namespace, agentspec)
	}

	return depth, nil
}

// GetRequestRate returns aggregated request rate for an AgentSpec.
func (c *Collector) GetRequestRate(namespace, agentspec string) (float64, error) {
	key := fmt.Sprintf("%s/%s", namespace, agentspec)
	c.mu.Lock()
	c.lastAccess[key] = time.Now()
	c.mu.Unlock()

	c.mu.RLock()
	rate, exists := c.aggregatedRate[key]
	c.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("no metrics found for %s/%s", namespace, agentspec)
	}

	return rate, nil
}

// Handler returns an HTTP handler for Prometheus metrics.
func (c *Collector) Handler() http.Handler {
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
}

// AggregateMetrics aggregates metrics from multiple Keepers.
func (c *Collector) AggregateMetrics(ctx context.Context, keeperMetrics map[string]KeeperMetrics) error {
	// Aggregate metrics from all Keepers
	for keeperID, metrics := range keeperMetrics {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		for _, metric := range metrics.Metrics {
			switch metric.Type {
			case "latency":
				if latency, ok := metric.Value.(time.Duration); ok {
					c.RecordLatency(metric.AgentSpec, metric.Namespace, keeperID, latency)
				} else {
					c.logger.Warn("Failed to type assert latency metric",
						zap.String("keeperID", keeperID),
						zap.String("agentspec", metric.AgentSpec),
						zap.String("namespace", metric.Namespace),
						zap.Any("value", metric.Value),
					)
				}
			case "tokens":
				if tokens, ok := metric.Value.(int64); ok {
					c.RecordTokens(metric.AgentSpec, metric.Namespace, keeperID, tokens)
				} else {
					c.logger.Warn("Failed to type assert tokens metric",
						zap.String("keeperID", keeperID),
						zap.String("agentspec", metric.AgentSpec),
						zap.String("namespace", metric.Namespace),
						zap.Any("value", metric.Value),
					)
				}
			case "queue_depth":
				if depth, ok := metric.Value.(int); ok {
					c.SetQueueDepth(metric.AgentSpec, metric.Namespace, keeperID, depth)
				} else {
					c.logger.Warn("Failed to type assert queue depth metric",
						zap.String("keeperID", keeperID),
						zap.String("agentspec", metric.AgentSpec),
						zap.String("namespace", metric.Namespace),
						zap.Any("value", metric.Value),
					)
				}
			case "request":
				c.RecordRequest(metric.AgentSpec, metric.Namespace, keeperID)
			default:
				c.logger.Warn("Unknown metric type",
					zap.String("keeperID", keeperID),
					zap.String("type", metric.Type),
				)
			}
		}
	}

	return nil
}

// KeeperMetrics represents metrics from a Keeper.
type KeeperMetrics struct {
	KeeperID string
	Metrics  []Metric
}

// Metric represents a single metric.
type Metric struct {
	Type      string
	AgentSpec string
	Namespace string
	Value     interface{}
	Timestamp time.Time
}

// startCleanup starts the background cleanup goroutine.
func (c *Collector) startCleanup() {
	c.cleanupTicker = time.NewTicker(c.cleanupInterval)
	c.cleanupWg.Add(1)

	go func() {
		defer c.cleanupWg.Done()

		for {
			select {
			case <-c.cleanupStopCh:
				c.cleanupTicker.Stop()
				return
			case <-c.cleanupTicker.C:
				c.cleanupIdleMetrics()
			}
		}
	}()
}

// cleanupIdleMetrics removes metrics for AgentSpecs that haven't been accessed recently.
func (c *Collector) cleanupIdleMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	type keyWithTime struct {
		key        string
		lastAccess time.Time
	}
	var keysToDelete []keyWithTime

	// Find keys that haven't been accessed recently
	for key, lastAccess := range c.lastAccess {
		if now.Sub(lastAccess) > c.maxIdleTime {
			keysToDelete = append(keysToDelete, keyWithTime{
				key:        key,
				lastAccess: lastAccess,
			})
		}
	}

	// Delete idle metrics
	for _, kt := range keysToDelete {
		delete(c.aggregatedLatency, kt.key)
		delete(c.aggregatedTokens, kt.key)
		delete(c.aggregatedQueue, kt.key)
		delete(c.aggregatedRate, kt.key)
		delete(c.lastAccess, kt.key)

		if c.logger != nil {
			c.logger.Debug("Cleaned up idle metrics",
				zap.String("key", kt.key),
				zap.Duration("idleTime", now.Sub(kt.lastAccess)),
			)
		}
	}

	if len(keysToDelete) > 0 && c.logger != nil {
		c.logger.Info("Cleaned up idle metrics",
			zap.Int("count", len(keysToDelete)),
		)
	}
}

// Close stops the cleanup goroutine and cleans up resources.
func (c *Collector) Close() {
	if c.cleanupStopCh != nil {
		close(c.cleanupStopCh)
		c.cleanupWg.Wait()
	}
}

// SetMaxIdleTime sets the maximum idle time before metrics are cleaned up.
func (c *Collector) SetMaxIdleTime(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxIdleTime = duration
}

// SetCleanupInterval sets the interval for running cleanup.
func (c *Collector) SetCleanupInterval(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}
	c.cleanupInterval = duration
	c.cleanupTicker = time.NewTicker(duration)
}
