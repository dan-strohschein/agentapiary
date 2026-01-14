package launcher

import (
	"context"
	"os"
	"testing"

	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSecret_EndToEnd_LauncherIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase-integration")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	logger := zap.NewNop()
	l := NewLauncher(logger, store)
	defer l.Shutdown(context.Background())

	ctx := context.Background()

	// Create cell
	cell := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "test-ns"}}
	require.NoError(t, store.Create(ctx, cell))

	// Create secret
	secret := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "api-credentials",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"api-key":    []byte("sk-1234567890abcdef"),
			"api-secret": []byte("secret-abcdef1234567890"),
		},
	}
	require.NoError(t, store.Create(ctx, secret))

	// Create AgentSpec that references the secret
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "$API_KEY"},
				Env: []apiary.EnvVar{
					{
						Name: "API_KEY",
						ValueFrom: &apiary.EnvVarSource{
							SecretKeyRef: &apiary.SecretKeySelector{
								Name: "api-credentials",
								Key:  "api-key",
							},
						},
					},
					{
						Name: "API_SECRET",
						ValueFrom: &apiary.EnvVarSource{
							SecretKeyRef: &apiary.SecretKeySelector{
								Name: "api-credentials",
								Key:  "api-secret",
							},
						},
					},
				},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	// Launch agent - should successfully resolve secrets
	process, err := l.Launch(ctx, spec)
	require.NoError(t, err)
	assert.NotNil(t, process)

	// Cleanup
	_ = l.Stop(ctx, process.ID)
}

func TestSecret_EndToEnd_MultipleSecrets(t *testing.T) {
	tmpDir := t.TempDir()
	
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase-multi")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	logger := zap.NewNop()
	l := NewLauncher(logger, store)
	defer l.Shutdown(context.Background())

	ctx := context.Background()

	// Create cell
	cell := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "test-ns"}}
	require.NoError(t, store.Create(ctx, cell))

	// Create multiple secrets
	secret1 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "database-creds",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"username": []byte("dbuser"),
			"password": []byte("dbpass"),
		},
	}

	secret2 := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "api-creds",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"key": []byte("api-key-123"),
		},
	}

	require.NoError(t, store.Create(ctx, secret1))
	require.NoError(t, store.Create(ctx, secret2))

	// Create AgentSpec that references both secrets
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "test"},
				Env: []apiary.EnvVar{
					{
						Name: "DB_USER",
						ValueFrom: &apiary.EnvVarSource{
							SecretKeyRef: &apiary.SecretKeySelector{
								Name: "database-creds",
								Key:  "username",
							},
						},
					},
					{
						Name: "DB_PASS",
						ValueFrom: &apiary.EnvVarSource{
							SecretKeyRef: &apiary.SecretKeySelector{
								Name: "database-creds",
								Key:  "password",
							},
						},
					},
					{
						Name: "API_KEY",
						ValueFrom: &apiary.EnvVarSource{
							SecretKeyRef: &apiary.SecretKeySelector{
								Name: "api-creds",
								Key:  "key",
							},
						},
					},
				},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	// Launch agent - should successfully resolve all secrets
	process, err := l.Launch(ctx, spec)
	require.NoError(t, err)
	assert.NotNil(t, process)

	// Cleanup
	_ = l.Stop(ctx, process.ID)
}

func TestSecret_EndToEnd_MixedEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	
	os.Setenv("APIARY_SECRET_PASSPHRASE", "test-passphrase-mixed")
	defer os.Unsetenv("APIARY_SECRET_PASSPHRASE")

	store, err := badger.NewStore(tmpDir)
	require.NoError(t, err)
	defer store.Close()

	logger := zap.NewNop()
	l := NewLauncher(logger, store)
	defer l.Shutdown(context.Background())

	ctx := context.Background()

	// Create cell
	cell := &apiary.Cell{ObjectMeta: apiary.ObjectMeta{Name: "test-ns"}}
	require.NoError(t, store.Create(ctx, cell))

	// Create secret
	secret := &apiary.Secret{
		TypeMeta: apiary.TypeMeta{APIVersion: "apiary.io/v1", Kind: "Secret"},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "api-key",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"value": []byte("secret-api-key"),
		},
	}
	require.NoError(t, store.Create(ctx, secret))

	// Create AgentSpec with both plain env vars and secret references
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "test"},
				Env: []apiary.EnvVar{
					{
						Name:  "NODE_ENV",
						Value: "production", // Plain env var
					},
					{
						Name: "API_KEY",
						ValueFrom: &apiary.EnvVarSource{
							SecretKeyRef: &apiary.SecretKeySelector{
								Name: "api-key",
								Key:  "value",
							},
						},
					},
					{
						Name:  "DEBUG",
						Value: "false", // Plain env var
					},
				},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	// Launch agent - should handle both types of env vars
	process, err := l.Launch(ctx, spec)
	require.NoError(t, err)
	assert.NotNil(t, process)

	// Cleanup
	_ = l.Stop(ctx, process.ID)
}
