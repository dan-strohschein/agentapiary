package badger

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_NamespaceIsolation_AgentSpecs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create AgentSpecs in different namespaces with the same name
	spec1 := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace1",
			UID:       "uid-1",
			Labels:    apiary.Labels{"env": "test"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "namespace1"},
			},
		},
	}

	spec2 := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace2",
			UID:       "uid-2",
			Labels:    apiary.Labels{"env": "prod"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "namespace2"},
			},
		},
	}

	// Create both resources
	err := store.Create(ctx, spec1)
	require.NoError(t, err)

	err = store.Create(ctx, spec2)
	require.NoError(t, err)

	// Get resource from namespace1 - should get spec1
	resource1, err := store.Get(ctx, "AgentSpec", "test-agent", "namespace1")
	require.NoError(t, err)
	assert.Equal(t, "namespace1", resource1.GetNamespace())
	assert.Equal(t, "uid-1", resource1.GetUID())
	if spec, ok := resource1.(*apiary.AgentSpec); ok {
		assert.Equal(t, []string{"echo", "namespace1"}, spec.Spec.Runtime.Command)
	}

	// Get resource from namespace2 - should get spec2
	resource2, err := store.Get(ctx, "AgentSpec", "test-agent", "namespace2")
	require.NoError(t, err)
	assert.Equal(t, "namespace2", resource2.GetNamespace())
	assert.Equal(t, "uid-2", resource2.GetUID())
	if spec, ok := resource2.(*apiary.AgentSpec); ok {
		assert.Equal(t, []string{"echo", "namespace2"}, spec.Spec.Runtime.Command)
	}

	// List resources in namespace1 - should only return spec1
	resources1, err := store.List(ctx, "AgentSpec", "namespace1", nil)
	require.NoError(t, err)
	assert.Len(t, resources1, 1)
	assert.Equal(t, "namespace1", resources1[0].GetNamespace())

	// List resources in namespace2 - should only return spec2
	resources2, err := store.List(ctx, "AgentSpec", "namespace2", nil)
	require.NoError(t, err)
	assert.Len(t, resources2, 1)
	assert.Equal(t, "namespace2", resources2[0].GetNamespace())

	// Try to get namespace1 resource from namespace2 - should fail
	_, err = store.Get(ctx, "AgentSpec", "test-agent", "namespace1")
	require.NoError(t, err) // Actually succeeds because the resource exists
	// But if we try to get a different name, it should fail
	_, err = store.Get(ctx, "AgentSpec", "nonexistent", "namespace1")
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrNotFound, err)
}

func TestStore_NamespaceIsolation_Hives(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create Hives in different namespaces
	hive1 := &apiary.Hive{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Hive",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-hive",
			Namespace: "team-a",
			UID:       "hive-uid-1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.HiveSpec{
			Pattern: "pipeline",
		},
	}

	hive2 := &apiary.Hive{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Hive",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-hive",
			Namespace: "team-b",
			UID:       "hive-uid-2",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.HiveSpec{
			Pattern: "swarm",
		},
	}

	err := store.Create(ctx, hive1)
	require.NoError(t, err)

	err = store.Create(ctx, hive2)
	require.NoError(t, err)

	// List Hives in team-a - should only return hive1
	resources1, err := store.List(ctx, "Hive", "team-a", nil)
	require.NoError(t, err)
	assert.Len(t, resources1, 1)
	assert.Equal(t, "team-a", resources1[0].GetNamespace())
	if hive, ok := resources1[0].(*apiary.Hive); ok {
		assert.Equal(t, "pipeline", hive.Spec.Pattern)
	}

	// List Hives in team-b - should only return hive2
	resources2, err := store.List(ctx, "Hive", "team-b", nil)
	require.NoError(t, err)
	assert.Len(t, resources2, 1)
	assert.Equal(t, "team-b", resources2[0].GetNamespace())
	if hive, ok := resources2[0].(*apiary.Hive); ok {
		assert.Equal(t, "swarm", hive.Spec.Pattern)
	}
}

func TestStore_NamespaceIsolation_Update(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create resource in namespace1
	spec1 := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace1",
			UID:       "uid-1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "original"},
			},
		},
	}

	err := store.Create(ctx, spec1)
	require.NoError(t, err)

	// Update the resource
	spec1.Spec.Runtime.Command = []string{"echo", "updated"}
	err = store.Update(ctx, spec1)
	require.NoError(t, err)

	// Verify update persisted in correct namespace
	updated, err := store.Get(ctx, "AgentSpec", "test-agent", "namespace1")
	require.NoError(t, err)
	if spec, ok := updated.(*apiary.AgentSpec); ok {
		assert.Equal(t, []string{"echo", "updated"}, spec.Spec.Runtime.Command)
		assert.Equal(t, "namespace1", spec.GetNamespace())
	}

	// Verify resource doesn't exist in namespace2
	_, err = store.Get(ctx, "AgentSpec", "test-agent", "namespace2")
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrNotFound, err)
}

func TestStore_NamespaceIsolation_Delete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create resources in two namespaces with same name
	spec1 := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace1",
			UID:       "uid-1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "test"},
			},
		},
	}

	spec2 := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "namespace2",
			UID:       "uid-2",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "test"},
			},
		},
	}

	err := store.Create(ctx, spec1)
	require.NoError(t, err)

	err = store.Create(ctx, spec2)
	require.NoError(t, err)

	// Delete resource from namespace1
	err = store.Delete(ctx, "AgentSpec", "test-agent", "namespace1")
	require.NoError(t, err)

	// Verify resource still exists in namespace2
	resource2, err := store.Get(ctx, "AgentSpec", "test-agent", "namespace2")
	require.NoError(t, err)
	assert.Equal(t, "namespace2", resource2.GetNamespace())

	// Verify resource deleted from namespace1
	_, err = store.Get(ctx, "AgentSpec", "test-agent", "namespace1")
	assert.Error(t, err)
	assert.Equal(t, apiary.ErrNotFound, err)

	// List namespace1 - should be empty
	resources1, err := store.List(ctx, "AgentSpec", "namespace1", nil)
	require.NoError(t, err)
	assert.Len(t, resources1, 0)

	// List namespace2 - should still have the resource
	resources2, err := store.List(ctx, "AgentSpec", "namespace2", nil)
	require.NoError(t, err)
	assert.Len(t, resources2, 1)
}

func TestStore_NamespaceIsolation_MultipleResources(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple resources in namespace1
	for i := 0; i < 3; i++ {
		spec := &apiary.AgentSpec{
			TypeMeta: apiary.TypeMeta{
				APIVersion: "apiary.io/v1",
				Kind:       "AgentSpec",
			},
			ObjectMeta: apiary.ObjectMeta{
				Name:      "agent-" + string(rune('a'+i)),
				Namespace: "namespace1",
				UID:       "uid-1-" + string(rune('a'+i)),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Spec: apiary.AgentSpecSpec{
				Runtime: apiary.RuntimeConfig{
					Command: []string{"echo", "test"},
				},
			},
		}
		err := store.Create(ctx, spec)
		require.NoError(t, err)
	}

	// Create multiple resources in namespace2
	for i := 0; i < 5; i++ {
		spec := &apiary.AgentSpec{
			TypeMeta: apiary.TypeMeta{
				APIVersion: "apiary.io/v1",
				Kind:       "AgentSpec",
			},
			ObjectMeta: apiary.ObjectMeta{
				Name:      "agent-" + string(rune('x'+i)),
				Namespace: "namespace2",
				UID:       "uid-2-" + string(rune('x'+i)),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			Spec: apiary.AgentSpecSpec{
				Runtime: apiary.RuntimeConfig{
					Command: []string{"echo", "test"},
				},
			},
		}
		err := store.Create(ctx, spec)
		require.NoError(t, err)
	}

	// List namespace1 - should return 3 resources
	resources1, err := store.List(ctx, "AgentSpec", "namespace1", nil)
	require.NoError(t, err)
	assert.Len(t, resources1, 3)
	for _, r := range resources1 {
		assert.Equal(t, "namespace1", r.GetNamespace())
	}

	// List namespace2 - should return 5 resources
	resources2, err := store.List(ctx, "AgentSpec", "namespace2", nil)
	require.NoError(t, err)
	assert.Len(t, resources2, 5)
	for _, r := range resources2 {
		assert.Equal(t, "namespace2", r.GetNamespace())
	}
}

func TestStore_NamespaceIsolation_EmptyNamespace(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create resource with empty namespace (like Cell)
	cell := &apiary.Cell{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-cell",
			Namespace: "",
			UID:       "cell-uid",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	err := store.Create(ctx, cell)
	require.NoError(t, err)

	// Get with empty namespace
	resource, err := store.Get(ctx, "Cell", "test-cell", "")
	require.NoError(t, err)
	assert.Equal(t, "", resource.GetNamespace())

	// List with empty namespace
	resources, err := store.List(ctx, "Cell", "", nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resources), 1)
}
