package badger

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
)


// BenchmarkStore_List benchmarks resource listing.
func BenchmarkStore_List(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Pre-populate with resources
	for i := 0; i < 100; i++ {
		spec := &apiary.AgentSpec{
			TypeMeta: apiary.TypeMeta{
				APIVersion: "apiary.io/v1",
				Kind:       "AgentSpec",
			},
			ObjectMeta: apiary.ObjectMeta{
				Name:      "bench-agent",
				Namespace: "default",
				UID:       "test-uid",
				Labels:    apiary.Labels{"app": "bench"},
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

		if err := store.Create(ctx, spec); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := store.List(ctx, "AgentSpec", "default", nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_Update benchmarks resource updates.
func BenchmarkStore_Update(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Pre-populate with a resource
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-agent",
			Namespace: "default",
			UID:       "test-uid",
			Labels:    apiary.Labels{"app": "bench"},
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

	if err := store.Create(ctx, spec); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		spec.UpdatedAt = time.Now()
		if err := store.Update(ctx, spec); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_ConcurrentReads benchmarks concurrent read operations.
func BenchmarkStore_ConcurrentReads(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Pre-populate with a resource
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-agent",
			Namespace: "default",
			UID:       "test-uid",
			Labels:    apiary.Labels{"app": "bench"},
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

	if err := store.Create(ctx, spec); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := store.Get(ctx, "AgentSpec", "bench-agent", "default")
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
