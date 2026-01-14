package badger

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStore(t *testing.T) (*Store, func()) {
	tmpDir, err := os.MkdirTemp("", "badger-test-*")
	require.NoError(t, err)

	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestStore_Create(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			Labels:    apiary.Labels{"app": "test"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	err := store.Create(ctx, spec)
	assert.NoError(t, err)

	// Try to create again - should fail
	err = store.Create(ctx, spec)
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrAlreadyExists, err)
}

func TestStore_Create_AutoGeneratesUID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent-no-uid",
			Namespace: "default",
			// UID is intentionally empty
			Labels:    apiary.Labels{"app": "test"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	// UID should be empty before Create
	assert.Empty(t, spec.GetUID())

	err := store.Create(ctx, spec)
	assert.NoError(t, err)

	// UID should be auto-generated after Create
	assert.NotEmpty(t, spec.GetUID())

	// Verify the stored resource also has the UID
	retrieved, err := store.Get(ctx, "AgentSpec", "test-agent-no-uid", "default")
	assert.NoError(t, err)
	assert.NotEmpty(t, retrieved.GetUID())
	assert.Equal(t, spec.GetUID(), retrieved.GetUID())
}

func TestStore_Get(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	err := store.Create(ctx, spec)
	require.NoError(t, err)

	// Get the resource
	resource, err := store.Get(ctx, "AgentSpec", "test-agent", "default")
	assert.NoError(t, err)
	assert.NotNil(t, resource)

	retrievedSpec, ok := resource.(*apiary.AgentSpec)
	assert.True(t, ok)
	assert.Equal(t, spec.GetName(), retrievedSpec.GetName())
	assert.Equal(t, spec.GetNamespace(), retrievedSpec.GetNamespace())

	// Get non-existent resource
	_, err = store.Get(ctx, "AgentSpec", "non-existent", "default")
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrNotFound, err)
}

func TestStore_Update(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	err := store.Create(ctx, spec)
	require.NoError(t, err)

	// Update the resource
	spec.Spec.Runtime.Command = []string{"echo", "updated"}
	spec.ObjectMeta.UpdatedAt = time.Now()

	err = store.Update(ctx, spec)
	assert.NoError(t, err)

	// Verify update
	resource, err := store.Get(ctx, "AgentSpec", "test-agent", "default")
	assert.NoError(t, err)
	retrievedSpec := resource.(*apiary.AgentSpec)
	assert.Equal(t, []string{"echo", "updated"}, retrievedSpec.Spec.Runtime.Command)

	// Update non-existent resource
	nonExistent := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "non-existent",
			Namespace: "default",
		},
	}
	err = store.Update(ctx, nonExistent)
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrNotFound, err)
}

func TestStore_Delete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	err := store.Create(ctx, spec)
	require.NoError(t, err)

	// Delete the resource
	err = store.Delete(ctx, "AgentSpec", "test-agent", "default")
	assert.NoError(t, err)

	// Verify deletion
	_, err = store.Get(ctx, "AgentSpec", "test-agent", "default")
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrNotFound, err)

	// Delete non-existent resource
	err = store.Delete(ctx, "AgentSpec", "non-existent", "default")
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrNotFound, err)
}

func TestStore_List(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple resources
	for i := 0; i < 3; i++ {
		spec := &apiary.AgentSpec{
			TypeMeta: apiary.TypeMeta{
				APIVersion: "apiary.io/v1",
				Kind:       "AgentSpec",
			},
			ObjectMeta: apiary.ObjectMeta{
				Name:      fmt.Sprintf("test-agent-%d", i),
				Namespace: "default",
				UID:       fmt.Sprintf("test-uid-%d", i),
				Labels:    apiary.Labels{"app": "test"},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Spec: apiary.AgentSpecSpec{
				Runtime: apiary.RuntimeConfig{
					Command: []string{"echo", "hello"},
				},
			},
		}
		err := store.Create(ctx, spec)
		require.NoError(t, err)
	}

	// List all resources
	resources, err := store.List(ctx, "AgentSpec", "default", nil)
	assert.NoError(t, err)
	assert.Len(t, resources, 3)

	// List with selector
	selector := apiary.Labels{"app": "test"}
	resources, err = store.List(ctx, "AgentSpec", "default", selector)
	assert.NoError(t, err)
	assert.Len(t, resources, 3)

	// List with non-matching selector
	selector = apiary.Labels{"app": "other"}
	resources, err = store.List(ctx, "AgentSpec", "default", selector)
	assert.NoError(t, err)
	assert.Len(t, resources, 0)
}

func TestStore_Watch(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := store.Watch(ctx, "AgentSpec", "default")
	require.NoError(t, err)

	// Create a resource
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	err = store.Create(ctx, spec)
	require.NoError(t, err)

	// Wait for event
	select {
	case event := <-ch:
		assert.Equal(t, apiary.EventTypeCreated, event.Type)
		assert.NotNil(t, event.Resource)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Update resource
	spec.ObjectMeta.UpdatedAt = time.Now()
	err = store.Update(ctx, spec)
	require.NoError(t, err)

	// Wait for update event
	select {
	case event := <-ch:
		assert.Equal(t, apiary.EventTypeUpdated, event.Type)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for update event")
	}

	// Delete resource
	err = store.Delete(ctx, "AgentSpec", "test-agent", "default")
	require.NoError(t, err)

	// Wait for delete event
	select {
	case event := <-ch:
		assert.Equal(t, apiary.EventTypeDeleted, event.Type)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for delete event")
	}
}

func TestStore_GetVersion(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "v1"},
			},
		},
	}

	err := store.Create(ctx, spec)
	require.NoError(t, err)

	// Update to create version 2
	spec.Spec.Runtime.Command = []string{"echo", "v2"}
	spec.ObjectMeta.UpdatedAt = time.Now()
	err = store.Update(ctx, spec)
	require.NoError(t, err)

	// Get version 1
	v1, err := store.GetVersion(ctx, "AgentSpec", "test-agent", "default", 1)
	assert.NoError(t, err)
	v1Spec := v1.(*apiary.AgentSpec)
	assert.Equal(t, []string{"echo", "v1"}, v1Spec.Spec.Runtime.Command)

	// Get version 2
	v2, err := store.GetVersion(ctx, "AgentSpec", "test-agent", "default", 2)
	assert.NoError(t, err)
	v2Spec := v2.(*apiary.AgentSpec)
	assert.Equal(t, []string{"echo", "v2"}, v2Spec.Spec.Runtime.Command)
}

func TestStore_ListVersions(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "v1"},
			},
		},
	}

	err := store.Create(ctx, spec)
	require.NoError(t, err)

	// Create multiple versions
	for i := 2; i <= 3; i++ {
		spec.Spec.Runtime.Command = []string{"echo", fmt.Sprintf("v%d", i)}
		spec.ObjectMeta.UpdatedAt = time.Now()
		err = store.Update(ctx, spec)
		require.NoError(t, err)
	}

	versions, err := store.ListVersions(ctx, "AgentSpec", "test-agent", "default")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(versions), 3)
}

func BenchmarkStore_Create(b *testing.B) {
	store, cleanup := setupTestStore(&testing.T{})
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-agent",
			Namespace: "default",
			UID:       "bench-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		spec.ObjectMeta.Name = fmt.Sprintf("bench-agent-%d", i)
		spec.ObjectMeta.UID = fmt.Sprintf("bench-uid-%d", i)
		_ = store.Create(ctx, spec)
	}
}

func BenchmarkStore_Get(b *testing.B) {
	store, cleanup := setupTestStore(&testing.T{})
	defer cleanup()

	ctx := context.Background()

	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-agent",
			Namespace: "default",
			UID:       "bench-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
		},
	}

	store.Create(ctx, spec)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(ctx, "AgentSpec", "bench-agent", "default")
	}
}
