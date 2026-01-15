package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewClient(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		Timeout:   5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
		Logger:    logger,
	}

	client := NewClient(cfg)
	assert.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, 5*time.Second, client.httpClient.Timeout)
}

func TestNewClient_Defaults(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		Logger: logger,
	}

	client := NewClient(cfg)
	assert.NotNil(t, client)
	assert.Equal(t, 10*time.Second, client.httpClient.Timeout)
}

func TestClient_Send_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer token123", r.Header.Get("Authorization"))

		var event Event
		err := json.NewDecoder(r.Body).Decode(&event)
		require.NoError(t, err)
		assert.Equal(t, "rate_limit", event.Type)
		assert.Equal(t, "test-ns", event.Namespace)
		assert.Equal(t, "test-hive", event.HiveName)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zap.NewNop()
	client := NewClient(Config{Logger: logger})

	event := &Event{
		Type:      "rate_limit",
		Timestamp: time.Now(),
		Namespace: "test-ns",
		HiveName:  "test-hive",
		Error:     "rate limit exceeded",
	}

	err := client.Send(context.Background(), server.URL, event, "Bearer token123")
	assert.NoError(t, err)
}

func TestClient_Send_Retry(t *testing.T) {
	attempts := 0
	// Create test server that fails twice then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	logger := zap.NewNop()
	client := NewClient(Config{
		Logger:     logger,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond, // Fast retry for testing
	})

	event := &Event{
		Type:      "token_budget",
		Timestamp: time.Now(),
		Namespace: "test-ns",
		HiveName:  "test-hive",
	}

	err := client.Send(context.Background(), server.URL, event, "")
	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestClient_Send_AllRetriesFail(t *testing.T) {
	// Create test server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := zap.NewNop()
	client := NewClient(Config{
		Logger:     logger,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	})

	event := &Event{
		Type:      "timeout",
		Timestamp: time.Now(),
	}

	err := client.Send(context.Background(), server.URL, event, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook failed after retries")
}

func TestClient_Send_ContextCancellation(t *testing.T) {
	// Create test server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zap.NewNop()
	client := NewClient(Config{
		Logger:   logger,
		Timeout:  50 * time.Millisecond, // Short timeout
		MaxRetries: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := &Event{
		Type:      "output_validation",
		Timestamp: time.Now(),
	}

	err := client.Send(ctx, server.URL, event, "")
	assert.Error(t, err)
	// Context cancellation may be wrapped in retry error
	assert.Contains(t, err.Error(), "context canceled")
}

func TestNewManager(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{Logger: logger}

	manager := NewManager(cfg, nil)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.client)
}

func TestManager_SendEvent_NoURL(t *testing.T) {
	logger := zap.NewNop()
	manager := NewManager(Config{Logger: logger}, nil)

	event := &Event{
		Type:      "rate_limit",
		Timestamp: time.Now(),
	}

	// Should not error when URL is empty
	err := manager.SendEvent(context.Background(), "", "", event)
	assert.NoError(t, err)
}

func TestManager_SendEvent_WithMetrics(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zap.NewNop()
	mockMetrics := &MockMetricsCollector{}
	manager := NewManager(Config{Logger: logger}, mockMetrics)

	event := &Event{
		Type:      "rate_limit",
		Timestamp: time.Now(),
		Namespace: "test-ns",
		HiveName:  "test-hive",
	}

	err := manager.SendEvent(context.Background(), server.URL, "", event)
	assert.NoError(t, err)

	// Wait a bit for async webhook to complete
	time.Sleep(100 * time.Millisecond)

	// Verify metrics were called
	assert.True(t, mockMetrics.sentCalled)
	assert.Equal(t, "test-ns", mockMetrics.sentNamespace)
	assert.Equal(t, "test-hive", mockMetrics.sentHive)
	assert.Equal(t, "rate_limit", mockMetrics.sentEventType)
}

func TestManager_SendEvent_WithMetrics_Failure(t *testing.T) {
	// Create test server that fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := zap.NewNop()
	mockMetrics := &MockMetricsCollector{}
	manager := NewManager(Config{
		Logger:     logger,
		MaxRetries: 1,
		RetryDelay: 10 * time.Millisecond,
	}, mockMetrics)

	event := &Event{
		Type:      "token_budget",
		Timestamp: time.Now(),
		Namespace: "test-ns",
		HiveName:  "test-hive",
	}

	err := manager.SendEvent(context.Background(), server.URL, "", event)
	assert.NoError(t, err) // SendEvent itself doesn't error (fire and forget)

	// Wait longer for async webhook to complete (with retries)
	time.Sleep(500 * time.Millisecond)

	// Verify failure metrics were called
	assert.True(t, mockMetrics.failedCalled, "Webhook failure metric should have been called")
	assert.Equal(t, "test-ns", mockMetrics.failedNamespace)
	assert.Equal(t, "test-hive", mockMetrics.failedHive)
	assert.Equal(t, "token_budget", mockMetrics.failedEventType)
}

func TestManager_SendEvent_SetsTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event Event
		err := json.NewDecoder(r.Body).Decode(&event)
		require.NoError(t, err)
		assert.False(t, event.Timestamp.IsZero())
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zap.NewNop()
	manager := NewManager(Config{Logger: logger}, nil)

	event := &Event{
		Type: "timeout",
		// Timestamp not set
	}

	err := manager.SendEvent(context.Background(), server.URL, "", event)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
}

// MockMetricsCollector is a mock implementation for testing
type MockMetricsCollector struct {
	sentCalled      bool
	sentNamespace   string
	sentHive        string
	sentEventType   string
	failedCalled    bool
	failedNamespace string
	failedHive      string
	failedEventType string
}

func (m *MockMetricsCollector) RecordWebhookSent(namespace, hive, eventType string) {
	m.sentCalled = true
	m.sentNamespace = namespace
	m.sentHive = hive
	m.sentEventType = eventType
}

func (m *MockMetricsCollector) RecordWebhookFailed(namespace, hive, eventType string) {
	m.failedCalled = true
	m.failedNamespace = namespace
	m.failedHive = hive
	m.failedEventType = eventType
}
