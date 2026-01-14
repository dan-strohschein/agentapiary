package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/comb"
	"github.com/agentapiary/apiary/internal/session"
	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestServerWithComb(t *testing.T) (*Server, *session.Manager, func()) {
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	logger := zap.NewNop()
	combStore := comb.NewStore(logger)
	sessionMgr := session.NewManager(session.Config{
		Store:  store,
		Logger: logger,
	})

	ctx := context.Background()
	err = sessionMgr.Start(ctx)
	require.NoError(t, err)

	server, err := NewServer(Config{
		Store:      store,
		SessionMgr: sessionMgr,
		Comb:       combStore,
		Logger:     logger,
	})
	require.NoError(t, err)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sessionMgr.Stop(ctx)
		combStore.Close()
		store.Close()
	}

	return server, sessionMgr, cleanup
}

func TestAPI_CombIsolation_CrossNamespaceAccess(t *testing.T) {
	server, sessionMgr, cleanup := setupTestServerWithComb(t)
	defer cleanup()

	ctx := context.Background()

	// Create session in namespace1
	session1, err := sessionMgr.CreateSession(ctx, session.SessionConfig{
		Namespace: "cell1",
	})
	require.NoError(t, err)

	// Create session in namespace2
	session2, err := sessionMgr.CreateSession(ctx, session.SessionConfig{
		Namespace: "cell2",
	})
	require.NoError(t, err)

	// Set data in session1 via API
	reqBody := map[string]interface{}{
		"type":    "user",
		"content": "message for session1",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/cell1/sessions/"+session1.GetUID()+"/messages", bytes.NewReader(bodyBytes))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Try to access session1's memory from namespace2 (should fail)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cells/cell2/sessions/"+session1.GetUID()+"/memory", nil)
	rec = httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Try to access session2's memory from namespace1 (should fail)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cells/cell1/sessions/"+session2.GetUID()+"/memory", nil)
	rec = httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Access session1's memory from namespace1 (should succeed)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/cells/cell1/sessions/"+session1.GetUID()+"/memory", nil)
	rec = httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAPI_CombIsolation_DeleteSession(t *testing.T) {
	server, sessionMgr, cleanup := setupTestServerWithComb(t)
	defer cleanup()

	ctx := context.Background()

	// Create session in namespace1
	session1, err := sessionMgr.CreateSession(ctx, session.SessionConfig{
		Namespace: "cell1",
	})
	require.NoError(t, err)

	// Try to delete session1 from namespace2 (should fail)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/cell2/sessions/"+session1.GetUID(), nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Delete session1 from namespace1 (should succeed)
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/cells/cell1/sessions/"+session1.GetUID(), nil)
	rec = httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
