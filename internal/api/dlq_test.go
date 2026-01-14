package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/dlq"
	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestServerWithDLQ(t *testing.T) (*Server, *dlq.Manager, func()) {
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	logger := zap.NewNop()
	dlqMgr := dlq.NewManager(logger)

	cfg := Config{
		Port:      8080,
		Store:     store,
		DLQManager: dlqMgr,
		Logger:    logger,
	}

	server, err := NewServer(cfg)
	require.NoError(t, err)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		store.Close()
	}

	return server, dlqMgr, cleanup
}

func createTestHive(t *testing.T, server *Server, namespace, hiveName string) {
	hive := apiary.Hive{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Hive",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      hiveName,
			Namespace: namespace,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.HiveSpec{
			Pattern: "pipeline",
		},
	}

	body, err := json.Marshal(hive)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/"+namespace+"/hives", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestServer_ListDLQMessages(t *testing.T) {
	server, dlqMgr, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Add a message to DLQ
	ctx := context.Background()
	msg := &nectar.Message{
		ID:        "test-msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := dlqMgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// List DLQ messages
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var messages []map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &messages)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Contains(t, messages[0]["id"], "test-msg-1")
}

func TestServer_ListDLQMessages_Empty(t *testing.T) {
	server, _, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// List DLQ messages (empty queue)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var messages []map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &messages)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestServer_ListDLQMessages_HiveNotFound(t *testing.T) {
	server, _, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "nonexistent-hive"

	// List DLQ messages for non-existent Hive
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServer_GetDLQMessage(t *testing.T) {
	server, dlqMgr, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Add a message to DLQ
	ctx := context.Background()
	msg := &nectar.Message{
		ID:        "test-msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := dlqMgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Get the DLQ message ID
	messages, err := dlqMgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	dlqMsgID := messages[0].ID

	// Get DLQ message
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq/"+dlqMsgID, nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var dlqMsg map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &dlqMsg)
	require.NoError(t, err)
	assert.Equal(t, dlqMsgID, dlqMsg["id"])
	assert.Equal(t, "test failure", dlqMsg["failureReason"])
	assert.Equal(t, float64(3), dlqMsg["attempts"])
}

func TestServer_GetDLQMessage_NotFound(t *testing.T) {
	server, _, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Get non-existent DLQ message
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq/nonexistent", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServer_DeleteDLQMessage(t *testing.T) {
	server, dlqMgr, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Add a message to DLQ
	ctx := context.Background()
	msg := &nectar.Message{
		ID:        "test-msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := dlqMgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Get the DLQ message ID
	messages, err := dlqMgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	dlqMsgID := messages[0].ID

	// Delete DLQ message
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq/"+dlqMsgID, nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify message was deleted
	messages, err = dlqMgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestServer_DeleteDLQMessage_NotFound(t *testing.T) {
	server, _, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Delete non-existent DLQ message
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq/nonexistent", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServer_ClearDLQ(t *testing.T) {
	server, dlqMgr, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Add multiple messages to DLQ
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		msg := &nectar.Message{
			ID:        "test-msg-" + string(rune('0'+i)),
			Type:      "test",
			Payload:   []byte("test payload"),
			Timestamp: time.Now(),
		}
		err := dlqMgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", i+1)
		require.NoError(t, err)
	}

	// Verify messages exist
	messages, err := dlqMgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Len(t, messages, 3)

	// Clear DLQ
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify messages were cleared
	messages, err = dlqMgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestServer_ClearDLQ_Empty(t *testing.T) {
	server, _, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Clear empty DLQ
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestServer_ReplayDLQMessage(t *testing.T) {
	server, dlqMgr, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Add a message to DLQ
	ctx := context.Background()
	msg := &nectar.Message{
		ID:        "test-msg-1",
		Type:      "test",
		Payload:   []byte("test payload"),
		Timestamp: time.Now(),
	}
	err := dlqMgr.AddMessage(ctx, namespace, hiveName, msg, "test failure", 3)
	require.NoError(t, err)

	// Get the DLQ message ID
	messages, err := dlqMgr.ListMessages(ctx, namespace, hiveName)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	dlqMsgID := messages[0].ID

	// Replay DLQ message
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq/"+dlqMsgID+"/replay", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Message retrieved for replay", response["message"])
	assert.NotNil(t, response["dlqMessage"])
}

func TestServer_ReplayDLQMessage_NotFound(t *testing.T) {
	server, _, cleanup := setupTestServerWithDLQ(t)
	defer cleanup()

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Replay non-existent DLQ message
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq/nonexistent/replay", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServer_DLQ_NoDLQManager(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	logger := zap.NewNop()

	cfg := Config{
		Port:   8080,
		Store:  store,
		Logger: logger,
		// DLQManager is nil
	}

	server, err := NewServer(cfg)
	require.NoError(t, err)

	namespace := "test-ns"
	hiveName := "test-hive"

	// Create Hive
	createTestHive(t, server, namespace, hiveName)

	// Try to list DLQ messages without DLQ manager
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/"+namespace+"/hives/"+hiveName+"/dlq", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var errorResp map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &errorResp)
	require.NoError(t, err)
	assert.Equal(t, "service_unavailable", errorResp["error"])
}
