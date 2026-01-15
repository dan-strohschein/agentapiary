package comb

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// BenchmarkComb_Set benchmarks setting a value in Comb.
func BenchmarkComb_Set(b *testing.B) {
	logger := zap.NewNop()
	comb := NewStore(logger)

	ctx := context.Background()
	sessionID := "session-1"
	key := "test-key"
	value := "test-value"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := comb.Set(ctx, sessionID, key, value, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComb_Get benchmarks getting a value from Comb.
func BenchmarkComb_Get(b *testing.B) {
	logger := zap.NewNop()
	comb := NewStore(logger)

	ctx := context.Background()
	sessionID := "session-1"
	key := "test-key"
	value := "test-value"

	// Pre-populate
	if err := comb.Set(ctx, sessionID, key, value, 0); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := comb.Get(ctx, sessionID, key)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComb_Incr benchmarks incrementing a value in Comb.
func BenchmarkComb_Incr(b *testing.B) {
	logger := zap.NewNop()
	comb := NewStore(logger)

	ctx := context.Background()
	sessionID := "session-1"
	key := "counter"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := comb.Incr(ctx, sessionID, key)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComb_ConcurrentOperations benchmarks concurrent Comb operations.
func BenchmarkComb_ConcurrentOperations(b *testing.B) {
	logger := zap.NewNop()
	comb := NewStore(logger)

	ctx := context.Background()
	sessionID := "session-1"

	b.ResetTimer()
	b.ReportAllocs()

		b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "key-1"
			value := "value-1"
			if err := comb.Set(ctx, sessionID, key, value, 0); err != nil {
				b.Fatal(err)
			}
			if _, err := comb.Get(ctx, sessionID, key); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}
