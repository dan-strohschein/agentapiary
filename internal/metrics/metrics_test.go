package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCollector_RecordLatency(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)

	collector.RecordLatency("agent1", "default", "drone1", 100*time.Millisecond)
	collector.RecordLatency("agent1", "default", "drone1", 200*time.Millisecond)

	stats, err := collector.GetLatencyStats("default", "agent1")
	require.NoError(t, err)
	assert.Greater(t, stats.Count, int64(0))
}

func TestCollector_RecordTokens(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)

	collector.RecordTokens("agent1", "default", "drone1", 100)
	collector.RecordTokens("agent1", "default", "drone1", 50)

	tokens, err := collector.GetTokenUsage("default", "agent1")
	require.NoError(t, err)
	assert.Equal(t, int64(150), tokens)
}

func TestCollector_SetQueueDepth(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)

	collector.SetQueueDepth("agent1", "default", "drone1", 5)
	collector.SetQueueDepth("agent1", "default", "drone1", 10)

	depth, err := collector.GetQueueDepth("default", "agent1")
	require.NoError(t, err)
	assert.Equal(t, 10, depth) // Should be max
}

func TestCollector_RecordRequest(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)

	collector.RecordRequest("agent1", "default", "drone1")
	collector.RecordRequest("agent1", "default", "drone1")

	rate, err := collector.GetRequestRate("default", "agent1")
	require.NoError(t, err)
	assert.Equal(t, float64(2), rate)
}

func TestCollector_Handler(t *testing.T) {
	logger := zap.NewNop()
	collector := NewCollector(logger)

	handler := collector.Handler()
	assert.NotNil(t, handler)
}
