// Package webhook provides webhook notification functionality for guardrail violations.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Event represents a guardrail violation event.
type Event struct {
	Type        string                 `json:"type"`        // e.g., "rate_limit", "token_budget", "timeout", "output_validation", "circuit_breaker"
	Timestamp   time.Time              `json:"timestamp"`
	Namespace   string                 `json:"namespace"`
	HiveName    string                 `json:"hiveName,omitempty"`
	AgentSpec   string                 `json:"agentSpec,omitempty"`
	DroneID     string                 `json:"droneID,omitempty"`
	SessionID   string                 `json:"sessionID,omitempty"`
	MessageID   string                 `json:"messageID,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// MetricsCollector defines the interface for recording webhook related metrics.
type MetricsCollector interface {
	RecordWebhookSent(namespace, hive, eventType string)
	RecordWebhookFailed(namespace, hive, eventType string)
}

// Client sends webhook notifications.
type Client struct {
	httpClient *http.Client
	logger     *zap.Logger
	maxRetries int
	retryDelay time.Duration
}

// Config holds webhook client configuration.
type Config struct {
	Timeout   time.Duration // HTTP request timeout
	MaxRetries int          // Maximum retry attempts
	RetryDelay time.Duration // Delay between retries
	Logger    *zap.Logger
}

// NewClient creates a new webhook client.
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = 1 * time.Second
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger:     cfg.Logger,
		maxRetries: cfg.MaxRetries,
		retryDelay: cfg.RetryDelay,
	}
}

// Send sends a webhook event to the specified URL with optional authentication.
func (c *Client) Send(ctx context.Context, url string, event *Event, authHeader string) error {
	if url == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Marshal event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook event: %w", err)
	}

	// Send with retry
	var lastErr error
	maxAttempts := c.maxRetries
	if maxAttempts == 0 {
		maxAttempts = 3 // Default
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			retryDelay := c.retryDelay
			if retryDelay == 0 {
				retryDelay = time.Duration(attempt) * time.Second // Default exponential
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to create webhook request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}

		// Send request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook request failed: %w", err)
			if c.logger != nil {
				c.logger.Warn("Webhook request failed, will retry",
					zap.String("url", url),
					zap.Int("attempt", attempt+1),
					zap.Error(err),
				)
			}
			continue
		}
		resp.Body.Close()

		// Check response status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if c.logger != nil {
				c.logger.Info("Webhook sent successfully",
					zap.String("url", url),
					zap.String("eventType", event.Type),
					zap.Int("statusCode", resp.StatusCode),
				)
			}
			return nil
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		if c.logger != nil {
			c.logger.Warn("Webhook returned error status, will retry",
				zap.String("url", url),
				zap.Int("attempt", attempt+1),
				zap.Int("statusCode", resp.StatusCode),
			)
		}
	}

	return fmt.Errorf("webhook failed after retries: %w", lastErr)
}

// Manager manages webhook notifications for multiple Hives.
type Manager struct {
	client          *Client
	mu              sync.RWMutex
	logger          *zap.Logger
	metricsCollector MetricsCollector // Optional metrics collector
}

// NewManager creates a new webhook manager.
func NewManager(cfg Config, metricsCollector MetricsCollector) *Manager {
	return &Manager{
		client:          NewClient(cfg),
		logger:          cfg.Logger,
		metricsCollector: metricsCollector,
	}
}

// SendEvent sends a webhook event for a Hive.
// If webhookURL is empty, no webhook is sent (returns nil).
func (m *Manager) SendEvent(ctx context.Context, webhookURL, authHeader string, event *Event) error {
	if webhookURL == "" {
		// No webhook configured, skip silently
		return nil
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Send webhook (fire and forget - don't block on webhook delivery)
	go func() {
		if err := m.client.Send(context.Background(), webhookURL, event, authHeader); err != nil {
			if m.logger != nil {
				m.logger.Error("Failed to send webhook",
					zap.String("url", webhookURL),
					zap.String("eventType", event.Type),
					zap.Error(err),
				)
			}
			// Record failure metric
			if m.metricsCollector != nil {
				m.metricsCollector.RecordWebhookFailed(event.Namespace, event.HiveName, event.Type)
			}
		} else {
			// Record success metric
			if m.metricsCollector != nil {
				m.metricsCollector.RecordWebhookSent(event.Namespace, event.HiveName, event.Type)
			}
		}
	}()

	return nil
}
