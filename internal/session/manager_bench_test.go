package session

import (
	"context"
	"testing"
	"time"

	"github.com/agentapiary/apiary/internal/store/badger"
	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// BenchmarkManager_CreateSession benchmarks session creation.
func BenchmarkManager_CreateSession(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := badger.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	logger := zap.NewNop()
	mgr := NewManager(Config{
		Store:  store,
		Logger: logger,
	})

	if err := mgr.Start(context.Background()); err != nil {
		b.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mgr.Stop(ctx)
	}()

	ctx := context.Background()
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

	if err := store.Create(ctx, cell); err != nil {
		b.Fatal(err)
	}

	hive := &apiary.Hive{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Hive",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-hive",
			Namespace: "default",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.HiveSpec{
			Session: apiary.SessionConfig{
				TimeoutMinutes:     30,
				MaxDurationMinutes: 120,
			},
		},
	}

	if err := store.Create(ctx, hive); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		config := SessionConfig{
			Namespace:          "default",
			HiveName:           "bench-hive",
			TimeoutMinutes:     30,
			MaxDurationMinutes: 120,
		}
		_, err := mgr.CreateSession(ctx, config)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkManager_GetSession benchmarks session retrieval.
func BenchmarkManager_GetSession(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := badger.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	logger := zap.NewNop()
	mgr := NewManager(Config{
		Store:  store,
		Logger: logger,
	})

	if err := mgr.Start(context.Background()); err != nil {
		b.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mgr.Stop(ctx)
	}()

	ctx := context.Background()
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

	if err := store.Create(ctx, cell); err != nil {
		b.Fatal(err)
	}

	hive := &apiary.Hive{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Hive",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-hive",
			Namespace: "default",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.HiveSpec{
			Session: apiary.SessionConfig{
				TimeoutMinutes:     30,
				MaxDurationMinutes: 120,
			},
		},
	}

	if err := store.Create(ctx, hive); err != nil {
		b.Fatal(err)
	}

	// Pre-create session
	config := SessionConfig{
		Namespace:          "default",
		HiveName:           "bench-hive",
		TimeoutMinutes:     30,
		MaxDurationMinutes: 120,
	}
	session, err := mgr.CreateSession(ctx, config)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := mgr.GetSession(ctx, session.GetUID())
		if err != nil {
			b.Fatal(err)
		}
	}
}
