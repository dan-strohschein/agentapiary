package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	logger := zap.NewNop()

	cfg := Config{
		Port:   8080,
		Store:  store,
		Logger: logger,
	}

	server, err := NewServer(cfg)
	require.NoError(t, err)

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		store.Close()
	}

	return server, cleanup
}

func TestServer_CreateCell(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	cell := apiary.Cell{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-cell",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	body, err := json.Marshal(cell)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var created apiary.Cell
	err = json.Unmarshal(rec.Body.Bytes(), &created)
	require.NoError(t, err)
	assert.Equal(t, "test-cell", created.GetName())
}

func TestServer_GetCell(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()
	cell := &apiary.Cell{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-cell",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	err := server.store.Create(ctx, cell)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/test-cell", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var retrieved apiary.Cell
	err = json.Unmarshal(rec.Body.Bytes(), &retrieved)
	require.NoError(t, err)
	assert.Equal(t, "test-cell", retrieved.GetName())
}

func TestServer_CreateAgentSpec(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// First create a cell
	cell := &apiary.Cell{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "default",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	spec := apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	body, err := json.Marshal(spec)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/default/agentspecs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestServer_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/non-existent", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
