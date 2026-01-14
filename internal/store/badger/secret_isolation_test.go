package badger

import (
	"context"
	"os"
	"testing"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_SecretIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Set a test passphrase
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase-123")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

	store, err := NewStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create secrets in different namespaces
	secret1 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "my-secret",
			Namespace: "namespace1",
		},
		Data: map[string][]byte{
			"key1": []byte("value1-from-ns1"),
			"key2": []byte("value2-from-ns1"),
		},
	}

	secret2 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "my-secret",
			Namespace: "namespace2",
		},
		Data: map[string][]byte{
			"key1": []byte("value1-from-ns2"),
			"key2": []byte("value2-from-ns2"),
		},
	}

	// Create both secrets
	err = store.Create(ctx, secret1)
	require.NoError(t, err)

	err = store.Create(ctx, secret2)
	require.NoError(t, err)

	// Retrieve secret1 from namespace1
	retrieved1, err := store.Get(ctx, "Secret", "my-secret", "namespace1")
	require.NoError(t, err)
	sec1, ok := retrieved1.(*apiary.Secret)
	require.True(t, ok)
	assert.Equal(t, "namespace1", sec1.GetNamespace())
	assert.Equal(t, "value1-from-ns1", string(sec1.Data["key1"]))
	assert.Equal(t, "value2-from-ns1", string(sec1.Data["key2"]))

	// Retrieve secret2 from namespace2
	retrieved2, err := store.Get(ctx, "Secret", "my-secret", "namespace2")
	require.NoError(t, err)
	sec2, ok := retrieved2.(*apiary.Secret)
	require.True(t, ok)
	assert.Equal(t, "namespace2", sec2.GetNamespace())
	assert.Equal(t, "value1-from-ns2", string(sec2.Data["key1"]))
	assert.Equal(t, "value2-from-ns2", string(sec2.Data["key2"]))

	// Verify secrets are isolated - getting secret1 from namespace2 should fail
	_, err = store.Get(ctx, "Secret", "my-secret", "namespace2")
	require.NoError(t, err) // It will succeed but return secret2
	retrievedWrong, _ := store.Get(ctx, "Secret", "my-secret", "namespace2")
	secWrong, _ := retrievedWrong.(*apiary.Secret)
	assert.Equal(t, "namespace2", secWrong.GetNamespace()) // Should be secret2, not secret1

	// List secrets in namespace1 - should only see secret1
	secrets1, err := store.List(ctx, "Secret", "namespace1", nil)
	require.NoError(t, err)
	assert.Len(t, secrets1, 1)
	assert.Equal(t, "namespace1", secrets1[0].GetNamespace())

	// List secrets in namespace2 - should only see secret2
	secrets2, err := store.List(ctx, "Secret", "namespace2", nil)
	require.NoError(t, err)
	assert.Len(t, secrets2, 1)
	assert.Equal(t, "namespace2", secrets2[0].GetNamespace())
}

func TestStore_SecretEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Set a test passphrase
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase-encryption")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

	store, err := NewStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create a secret with sensitive data
	secret := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "sensitive-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"api-key":    []byte("sk-1234567890abcdef"),
			"password":   []byte("super-secret-password"),
			"token":      []byte("bearer-token-xyz"),
		},
	}

	err = store.Create(ctx, secret)
	require.NoError(t, err)

	// Retrieve and verify data is decrypted correctly
	retrieved, err := store.Get(ctx, "Secret", "sensitive-secret", "default")
	require.NoError(t, err)
	sec, ok := retrieved.(*apiary.Secret)
	require.True(t, ok)
	
	// Verify all keys are present and values match
	assert.Equal(t, "sk-1234567890abcdef", string(sec.Data["api-key"]))
	assert.Equal(t, "super-secret-password", string(sec.Data["password"]))
	assert.Equal(t, "bearer-token-xyz", string(sec.Data["token"]))
}

func TestStore_SecretUpdate_Isolation(t *testing.T) {
	tmpDir := t.TempDir()
	
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

	store, err := NewStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create secret in namespace1
	secret := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-secret",
			Namespace: "namespace1",
		},
		Data: map[string][]byte{
			"key1": []byte("value1"),
		},
	}

	err = store.Create(ctx, secret)
	require.NoError(t, err)

	// Update secret in namespace1
	secret.Data["key1"] = []byte("updated-value1")
	secret.Data["key2"] = []byte("new-value2")
	err = store.Update(ctx, secret)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.Get(ctx, "Secret", "test-secret", "namespace1")
	require.NoError(t, err)
	updated, ok := retrieved.(*apiary.Secret)
	require.True(t, ok)
	assert.Equal(t, "updated-value1", string(updated.Data["key1"]))
	assert.Equal(t, "new-value2", string(updated.Data["key2"]))

	// Verify we cannot update secret in namespace2 (it doesn't exist)
	secret2 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-secret",
			Namespace: "namespace2",
		},
		Data: map[string][]byte{
			"key1": []byte("should-fail"),
		},
	}

	err = store.Update(ctx, secret2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStore_SecretDelete_Isolation(t *testing.T) {
	tmpDir := t.TempDir()
	
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

	store, err := NewStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create secrets in two namespaces
	secret1 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "shared-name",
			Namespace: "namespace1",
		},
		Data: map[string][]byte{
			"key1": []byte("value1"),
		},
	}

	secret2 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "shared-name",
			Namespace: "namespace2",
		},
		Data: map[string][]byte{
			"key1": []byte("value2"),
		},
	}

	err = store.Create(ctx, secret1)
	require.NoError(t, err)
	err = store.Create(ctx, secret2)
	require.NoError(t, err)

	// Delete secret from namespace1
	err = store.Delete(ctx, "Secret", "shared-name", "namespace1")
	require.NoError(t, err)

	// Verify secret1 is gone
	_, err = store.Get(ctx, "Secret", "shared-name", "namespace1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify secret2 still exists
	retrieved2, err := store.Get(ctx, "Secret", "shared-name", "namespace2")
	require.NoError(t, err)
	sec2, ok := retrieved2.(*apiary.Secret)
	require.True(t, ok)
	assert.Equal(t, "namespace2", sec2.GetNamespace())
	assert.Equal(t, "value2", string(sec2.Data["key1"]))
}
