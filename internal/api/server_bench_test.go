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
	"go.uber.org/zap"
)

// BenchmarkServer_CreateCell benchmarks cell creation via API.
func BenchmarkServer_CreateCell(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := badger.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	logger := zap.NewNop()

	cfg := Config{
		Port:   8080,
		Store:  store,
		Logger: logger,
	}

	server, err := NewServer(cfg)
	if err != nil {
		b.Fatal(err)
	}

	cell := apiary.Cell{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-cell",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cell.Name = "bench-cell"
		body, err := json.Marshal(cell)
		if err != nil {
			b.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/cells", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.echo.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			b.Fatalf("Expected status %d, got %d", http.StatusCreated, rec.Code)
		}
	}
}

// BenchmarkServer_GetCell benchmarks cell retrieval via API.
func BenchmarkServer_GetCell(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := badger.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	logger := zap.NewNop()

	cfg := Config{
		Port:   8080,
		Store:  store,
		Logger: logger,
	}

	server, err := NewServer(cfg)
	if err != nil {
		b.Fatal(err)
	}

	// Pre-populate
	ctx := context.Background()
	cell := &apiary.Cell{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "bench-cell",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	if err := store.Create(ctx, cell); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/cells/bench-cell", nil)
		rec := httptest.NewRecorder()

		server.echo.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	}
}

// BenchmarkServer_Healthz benchmarks health check endpoint.
func BenchmarkServer_Healthz(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := badger.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	logger := zap.NewNop()

	cfg := Config{
		Port:   8080,
		Store:  store,
		Logger: logger,
	}

	server, err := NewServer(cfg)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		server.echo.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	}
}

// BenchmarkServer_Ready benchmarks readiness check endpoint.
func BenchmarkServer_Ready(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := badger.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()

	logger := zap.NewNop()

	cfg := Config{
		Port:   8080,
		Store:  store,
		Logger: logger,
	}

	server, err := NewServer(cfg)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()

		server.echo.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	}
}
