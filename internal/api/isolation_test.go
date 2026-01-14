package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentapiary/apiary/internal/comb"
	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/internal/session"
	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupIsolationTestServer(t *testing.T) (*Server, func()) {
	logger := zap.NewNop()

	// Create temporary directory for BadgerDB
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	combStore := comb.NewStore(logger)

	sessionMgr := session.NewManager(session.Config{
		Store:  store,
		Logger: logger,
	})

	server, err := NewServer(Config{
		Port:       8080,
		Store:      store,
		SessionMgr: sessionMgr,
		Comb:       combStore,
		Metrics:    metrics.NewCollector(logger),
		Logger:     logger,
	})
	require.NoError(t, err)

	cleanup := func() {
		store.Close()
		combStore.Close()
	}

	return server, cleanup
}

func TestAPI_NamespaceIsolation_AgentSpecs(t *testing.T) {
	server, cleanup := setupIsolationTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create AgentSpec in namespace1 via store
	spec1 := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace1",
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "namespace1"},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	err := server.store.Create(ctx, spec1)
	require.NoError(t, err)

	// Get AgentSpec from namespace1 via API
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace1/agentspecs/test-agent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/cells/:namespace/agentspecs/:name")
	c.SetParamNames("namespace", "name")
	c.SetParamValues("namespace1", "test-agent")

	err = server.getAgentSpec(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response apiary.AgentSpec
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "namespace1", response.GetNamespace())
	assert.Equal(t, "test-agent", response.GetName())

	// Try to get resource from namespace2 (should return 404)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace2/agentspecs/test-agent", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/cells/:namespace/agentspecs/:name")
	c2.SetParamNames("namespace", "name")
	c2.SetParamValues("namespace2", "test-agent")

	err = server.getAgentSpec(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

func TestAPI_NamespaceIsolation_List(t *testing.T) {
	server, cleanup := setupIsolationTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create AgentSpecs in namespace1
	for i := 0; i < 2; i++ {
		spec := &apiary.AgentSpec{
			TypeMeta: apiary.TypeMeta{
				APIVersion: "apiary.io/v1",
				Kind:       "AgentSpec",
			},
			ObjectMeta: apiary.ObjectMeta{
				Name:      "agent1-" + string(rune('a'+i)),
				Namespace: "namespace1",
			},
			Spec: apiary.AgentSpecSpec{
				Runtime: apiary.RuntimeConfig{
					Command: []string{"echo", "test"},
				},
				Interface: apiary.InterfaceConfig{
					Type: "stdin",
				},
			},
		}
		err := server.store.Create(ctx, spec)
		require.NoError(t, err)
	}

	// Create AgentSpecs in namespace2
	for i := 0; i < 3; i++ {
		spec := &apiary.AgentSpec{
			TypeMeta: apiary.TypeMeta{
				APIVersion: "apiary.io/v1",
				Kind:       "AgentSpec",
			},
			ObjectMeta: apiary.ObjectMeta{
				Name:      "agent2-" + string(rune('x'+i)),
				Namespace: "namespace2",
			},
			Spec: apiary.AgentSpecSpec{
				Runtime: apiary.RuntimeConfig{
					Command: []string{"echo", "test"},
				},
				Interface: apiary.InterfaceConfig{
					Type: "stdin",
				},
			},
		}
		err := server.store.Create(ctx, spec)
		require.NoError(t, err)
	}

	// List namespace1
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace1/agentspecs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/cells/:namespace/agentspecs")
	c.SetParamNames("namespace")
	c.SetParamValues("namespace1")

	err := server.listAgentSpecs(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response1 []*apiary.AgentSpec
	err = json.Unmarshal(rec.Body.Bytes(), &response1)
	require.NoError(t, err)
	assert.Len(t, response1, 2)
	for _, spec := range response1 {
		assert.Equal(t, "namespace1", spec.GetNamespace())
	}

	// List namespace2
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace2/agentspecs", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/cells/:namespace/agentspecs")
	c2.SetParamNames("namespace")
	c2.SetParamValues("namespace2")

	err = server.listAgentSpecs(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var response2 []*apiary.AgentSpec
	err = json.Unmarshal(rec2.Body.Bytes(), &response2)
	require.NoError(t, err)
	assert.Len(t, response2, 3)
	for _, spec := range response2 {
		assert.Equal(t, "namespace2", spec.GetNamespace())
	}
}

func TestAPI_NamespaceIsolation_Create(t *testing.T) {
	server, cleanup := setupIsolationTestServer(t)
	defer cleanup()

	// Create AgentSpec in namespace1 via API
	specBody := apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace1",
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "test"},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	body, err := json.Marshal(specBody)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/namespace1/agentspecs", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/cells/:namespace/agentspecs")
	c.SetParamNames("namespace")
	c.SetParamValues("namespace1")

	err = server.createAgentSpec(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var response apiary.AgentSpec
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "namespace1", response.GetNamespace())

	// Verify it's not accessible from namespace2
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace2/agentspecs/test-agent", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/cells/:namespace/agentspecs/:name")
	c2.SetParamNames("namespace", "name")
	c2.SetParamValues("namespace2", "test-agent")

	err = server.getAgentSpec(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

func TestAPI_NamespaceIsolation_NamespaceMismatch(t *testing.T) {
	server, cleanup := setupIsolationTestServer(t)
	defer cleanup()

	// Try to create AgentSpec with mismatched namespace
	specBody := apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace2", // Different from URL
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "test"},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	body, err := json.Marshal(specBody)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/namespace1/agentspecs", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/cells/:namespace/agentspecs")
	c.SetParamNames("namespace")
	c.SetParamValues("namespace1")

	err = server.createAgentSpec(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errorResp errorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &errorResp)
	require.NoError(t, err)
	assert.Contains(t, errorResp.Message, "namespace")
}
