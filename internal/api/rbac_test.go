package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/auth"
	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestServerWithRBAC(t *testing.T) (*Server, *auth.RBAC, func()) {
	tmpDir := t.TempDir()
	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)

	logger := zap.NewNop()
	rbac := auth.NewRBAC()

	cfg := Config{
		Port:   8080,
		Store:  store,
		RBAC:   rbac,
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

	return server, rbac, cleanup
}

func TestRBAC_AssignRole(t *testing.T) {
	server, _, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell first
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	reqBody := map[string]interface{}{
		"userId": "user1",
		"role":   "admin",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/test-cell/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestRBAC_ListRoles(t *testing.T) {
	server, rbac, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell first
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	// Assign some roles
	err = rbac.AssignRole("test-cell", "user1", auth.RoleAdmin)
	require.NoError(t, err)
	err = rbac.AssignRole("test-cell", "user2", auth.RoleDeveloper)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/test-cell/roles", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var roles map[string]string
	err = json.Unmarshal(rec.Body.Bytes(), &roles)
	require.NoError(t, err)
	assert.Len(t, roles, 2)
	assert.Equal(t, "admin", roles["user1"])
	assert.Equal(t, "developer", roles["user2"])
}

func TestRBAC_RemoveRole(t *testing.T) {
	server, rbac, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell first
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	// Assign a role
	err = rbac.AssignRole("test-cell", "user1", auth.RoleAdmin)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/test-cell/roles/user1", nil)
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	_, exists := rbac.GetRole("test-cell", "user1")
	assert.False(t, exists)
}

func TestRBAC_Authorization_Admin(t *testing.T) {
	server, rbac, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell and agent spec
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	// Assign admin role
	err = rbac.AssignRole("test-cell", "user1", auth.RoleAdmin)
	require.NoError(t, err)

	// Try to create an agent spec with admin role
	spec := apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-cell",
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/test-cell/agentspecs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user1")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestRBAC_Authorization_Viewer_CannotCreate(t *testing.T) {
	server, rbac, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	// Assign viewer role
	err = rbac.AssignRole("test-cell", "user1", auth.RoleViewer)
	require.NoError(t, err)

	// Try to create an agent spec with viewer role (should fail)
	spec := apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-cell",
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/test-cell/agentspecs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user1")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRBAC_Authorization_Viewer_CanRead(t *testing.T) {
	server, rbac, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell and agent spec
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-cell",
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
	err = server.store.Create(context.Background(), spec)
	require.NoError(t, err)

	// Assign viewer role
	err = rbac.AssignRole("test-cell", "user1", auth.RoleViewer)
	require.NoError(t, err)

	// Try to read an agent spec with viewer role (should succeed)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/test-cell/agentspecs/test-agent", nil)
	req.Header.Set("X-User-ID", "user1")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRBAC_Authorization_Developer_CannotDelete(t *testing.T) {
	server, rbac, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell and agent spec
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-cell",
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
	err = server.store.Create(context.Background(), spec)
	require.NoError(t, err)

	// Assign developer role
	err = rbac.AssignRole("test-cell", "user1", auth.RoleDeveloper)
	require.NoError(t, err)

	// Try to delete an agent spec with developer role (should fail)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/test-cell/agentspecs/test-agent", nil)
	req.Header.Set("X-User-ID", "user1")
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRBAC_Authorization_NoPrincipal_Allowed(t *testing.T) {
	server, _, cleanup := setupTestServerWithRBAC(t)
	defer cleanup()

	// Create a cell
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
	err := server.store.Create(context.Background(), cell)
	require.NoError(t, err)

	// Try to create an agent spec without principal (should be allowed for MVP)
	spec := apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-cell",
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/test-cell/agentspecs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-User-ID header
	rec := httptest.NewRecorder()

	server.echo.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}
