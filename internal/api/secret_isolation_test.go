package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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

func setupTestServerForSecrets(t *testing.T) (*Server, func()) {
	tmpDir := t.TempDir()
	
	// Set a test passphrase
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase-api")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

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
		Metrics:    metrics.NewCollector(logger),
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

	return server, cleanup
}

func TestAPI_SecretIsolation_Create(t *testing.T) {
	server, cleanup := setupTestServerForSecrets(t)
	defer cleanup()

	e := echo.New()

	// Create cells
	cell1 := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "namespace1"}}
	cell2 := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "namespace2"}}
	ctx := context.Background()
	require.NoError(t, server.store.Create(ctx, cell1))
	require.NoError(t, server.store.Create(ctx, cell2))

	// Create secret in namespace1 (data must be base64 encoded)
	secret1 := map[string]interface{}{
		"apiVersion": "apiary.io/v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      "my-secret",
			"namespace": "namespace1",
		},
		"data": map[string]string{
			"key1": base64.StdEncoding.EncodeToString([]byte("value1-from-ns1")),
			"key2": base64.StdEncoding.EncodeToString([]byte("value2-from-ns1")),
		},
	}

	body1, _ := json.Marshal(secret1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/cells/namespace1/secrets", bytes.NewReader(body1))
	req1.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetPath("/api/v1/cells/:namespace/secrets")
	c1.SetParamNames("namespace")
	c1.SetParamValues("namespace1")

	err := server.createSecret(c1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec1.Code)

	// Create secret with same name in namespace2 (data must be base64 encoded)
	secret2 := map[string]interface{}{
		"apiVersion": "apiary.io/v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      "my-secret",
			"namespace": "namespace2",
		},
		"data": map[string]string{
			"key1": base64.StdEncoding.EncodeToString([]byte("value1-from-ns2")),
		},
	}

	body2, _ := json.Marshal(secret2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/cells/namespace2/secrets", bytes.NewReader(body2))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/cells/:namespace/secrets")
	c2.SetParamNames("namespace")
	c2.SetParamValues("namespace2")

	err = server.createSecret(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec2.Code)

	// Verify secrets are isolated - get secret from namespace1
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace1/secrets/my-secret", nil)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req3, rec3)
	c3.SetPath("/api/v1/cells/:namespace/secrets/:name")
	c3.SetParamNames("namespace", "name")
	c3.SetParamValues("namespace1", "my-secret")

	err = server.getSecret(c3)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec3.Code)

	var retrievedSecret apiary.Secret
	err = json.Unmarshal(rec3.Body.Bytes(), &retrievedSecret)
	require.NoError(t, err)
	assert.Equal(t, "namespace1", retrievedSecret.GetNamespace())
	assert.Equal(t, "value1-from-ns1", string(retrievedSecret.Data["key1"]))
	assert.Equal(t, "value2-from-ns1", string(retrievedSecret.Data["key2"]))

	// Verify secrets are isolated - get secret from namespace2
	req4 := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace2/secrets/my-secret", nil)
	rec4 := httptest.NewRecorder()
	c4 := e.NewContext(req4, rec4)
	c4.SetPath("/api/v1/cells/:namespace/secrets/:name")
	c4.SetParamNames("namespace", "name")
	c4.SetParamValues("namespace2", "my-secret")

	err = server.getSecret(c4)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec4.Code)

	var retrievedSecret2 apiary.Secret
	err = json.Unmarshal(rec4.Body.Bytes(), &retrievedSecret2)
	require.NoError(t, err)
	assert.Equal(t, "namespace2", retrievedSecret2.GetNamespace())
	assert.Equal(t, "value1-from-ns2", string(retrievedSecret2.Data["key1"]))
}

func TestAPI_SecretIsolation_NamespaceMismatch(t *testing.T) {
	server, cleanup := setupTestServerForSecrets(t)
	defer cleanup()

	e := echo.New()

	// Create cell
	cell := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "namespace1"}}
	ctx := context.Background()
	require.NoError(t, server.store.Create(ctx, cell))

	// Try to create secret with namespace mismatch (data must be base64 encoded)
	secret := map[string]interface{}{
		"apiVersion": "apiary.io/v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      "my-secret",
			"namespace": "namespace2", // Different from URL
		},
		"data": map[string]string{
			"key1": base64.StdEncoding.EncodeToString([]byte("value1")),
		},
	}

	body, _ := json.Marshal(secret)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cells/namespace1/secrets", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/cells/:namespace/secrets")
	c.SetParamNames("namespace")
	c.SetParamValues("namespace1")

	err := server.createSecret(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp errorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Message, "namespace in URL does not match")
}

func TestAPI_SecretIsolation_List(t *testing.T) {
	server, cleanup := setupTestServerForSecrets(t)
	defer cleanup()

	e := echo.New()

	ctx := context.Background()

	// Create cells
	cell1 := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "namespace1"}}
	cell2 := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "namespace2"}}
	require.NoError(t, server.store.Create(ctx, cell1))
	require.NoError(t, server.store.Create(ctx, cell2))

	// Create secrets in both namespaces
	secret1 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "secret1",
			Namespace: "namespace1",
		},
		Data: map[string][]byte{"key1": []byte("value1")},
	}
	secret2 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "secret2",
			Namespace: "namespace1",
		},
		Data: map[string][]byte{"key1": []byte("value1")},
	}
	secret3 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "secret3",
			Namespace: "namespace2",
		},
		Data: map[string][]byte{"key1": []byte("value1")},
	}

	require.NoError(t, server.store.Create(ctx, secret1))
	require.NoError(t, server.store.Create(ctx, secret2))
	require.NoError(t, server.store.Create(ctx, secret3))

	// List secrets in namespace1
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace1/secrets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/cells/:namespace/secrets")
	c.SetParamNames("namespace")
	c.SetParamValues("namespace1")

	err := server.listSecrets(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var secrets []*apiary.Secret
	err = json.Unmarshal(rec.Body.Bytes(), &secrets)
	require.NoError(t, err)
	assert.Len(t, secrets, 2)
	for _, sec := range secrets {
		assert.Equal(t, "namespace1", sec.GetNamespace())
	}

	// List secrets in namespace2
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/cells/namespace2/secrets", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/cells/:namespace/secrets")
	c2.SetParamNames("namespace")
	c2.SetParamValues("namespace2")

	err = server.listSecrets(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var secrets2 []*apiary.Secret
	err = json.Unmarshal(rec2.Body.Bytes(), &secrets2)
	require.NoError(t, err)
	assert.Len(t, secrets2, 1)
	assert.Equal(t, "namespace2", secrets2[0].GetNamespace())
}

func TestAPI_SecretIsolation_Delete(t *testing.T) {
	server, cleanup := setupTestServerForSecrets(t)
	defer cleanup()

	ctx := context.Background()

	// Create cells
	cell1 := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "namespace1"}}
	cell2 := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "namespace2"}}
	require.NoError(t, server.store.Create(ctx, cell1))
	require.NoError(t, server.store.Create(ctx, cell2))

	// Create secrets in both namespaces with same name
	secret1 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "shared-secret",
			Namespace: "namespace1",
		},
		Data: map[string][]byte{"key1": []byte("value1")},
	}
	secret2 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "shared-secret",
			Namespace: "namespace2",
		},
		Data: map[string][]byte{"key1": []byte("value2")},
	}

	require.NoError(t, server.store.Create(ctx, secret1))
	require.NoError(t, server.store.Create(ctx, secret2))

	e := echo.New()

	// Delete secret from namespace1
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cells/namespace1/secrets/shared-secret", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/cells/:namespace/secrets/:name")
	c.SetParamNames("namespace", "name")
	c.SetParamValues("namespace1", "shared-secret")

	err := server.deleteSecret(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify secret1 is gone
	_, err = server.store.Get(ctx, "Secret", "shared-secret", "namespace1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify secret2 still exists
	retrieved, err := server.store.Get(ctx, "Secret", "shared-secret", "namespace2")
	require.NoError(t, err)
	sec, ok := retrieved.(*apiary.Secret)
	require.True(t, ok)
	assert.Equal(t, "namespace2", sec.GetNamespace())
	assert.Equal(t, "value2", string(sec.Data["key1"]))
}
