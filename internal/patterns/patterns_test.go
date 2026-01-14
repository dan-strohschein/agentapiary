package patterns

import (
	"context"
	"testing"

	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/agentapiary/apiary/pkg/waggle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestBus(t *testing.T) (nectar.MessageBus, func()) {
	logger := zap.NewNop()
	bus, err := nectar.NewBus(nectar.Config{
		Logger:     logger,
		Port:       -1,
		Persistent: false,
	})
	require.NoError(t, err)

	cleanup := func() {
		bus.Close()
	}

	return bus, cleanup
}

func createTestHive(pattern string) *apiary.Hive {
	return &apiary.Hive{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Hive",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-hive",
			Namespace: "default",
		},
		Spec: apiary.HiveSpec{
			Pattern: pattern,
			Stages: []apiary.Stage{
				{Name: "stage1", AgentRef: "agent1", Replicas: 1},
				{Name: "stage2", AgentRef: "agent2", Replicas: 1},
			},
		},
	}
}

func TestSwarm_Initialize(t *testing.T) {
	hive := createTestHive("swarm")
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	logger := zap.NewNop()
	swarm := NewSwarm(hive, bus, logger)

	err := swarm.Initialize([]string{"agent1", "agent2", "agent3"})
	require.NoError(t, err)

	swarm.mu.RLock()
	assert.Equal(t, 3, len(swarm.agentIDs))
	swarm.mu.RUnlock()
}

func TestSwarm_Publish(t *testing.T) {
	hive := createTestHive("swarm")
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	logger := zap.NewNop()
	swarm := NewSwarm(hive, bus, logger)

	err := swarm.Initialize([]string{"agent1"})
	require.NoError(t, err)

	ctx := context.Background()
	err = swarm.Start(ctx)
	require.NoError(t, err)
	defer swarm.Stop()

	msg, err := waggle.NewMessage(waggle.MessageTypeEvent, map[string]string{"data": "test"})
	require.NoError(t, err)
	msg.SourceAgentID = "agent1"

	err = swarm.Publish(ctx, msg)
	require.NoError(t, err)
}

func TestEventDriven_Initialize(t *testing.T) {
	hive := createTestHive("event-driven")
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	logger := zap.NewNop()
	ed := NewEventDriven(hive, bus, logger)

	subscribers := []*SubscriberConfig{
		{AgentID: "agent1", Filter: EventFilter{Type: "task"}},
		{AgentID: "agent2", Filter: EventFilter{Type: "event"}},
	}

	err := ed.Initialize("source1", subscribers)
	require.NoError(t, err)

	ed.mu.RLock()
	assert.Equal(t, "source1", ed.eventSource)
	assert.Equal(t, 2, len(ed.subscribers))
	ed.mu.RUnlock()
}

func TestEventDriven_MatchesFilter(t *testing.T) {
	hive := createTestHive("event-driven")
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	logger := zap.NewNop()
	ed := NewEventDriven(hive, bus, logger)

	msg, _ := waggle.NewMessage(waggle.MessageTypeTask, "test")
	msg.SetMetadata("priority", "high")

	// Test type filter
	filter := EventFilter{Type: "task"}
	assert.True(t, ed.matchesFilter(msg, filter))

	filter = EventFilter{Type: "event"}
	assert.False(t, ed.matchesFilter(msg, filter))

	// Test priority filter
	filter = EventFilter{Priority: "high"}
	assert.True(t, ed.matchesFilter(msg, filter))

	filter = EventFilter{Priority: "low"}
	assert.False(t, ed.matchesFilter(msg, filter))

	// Test metadata filter
	filter = EventFilter{Metadata: map[string]string{"priority": "high"}}
	assert.True(t, ed.matchesFilter(msg, filter))

	filter = EventFilter{Metadata: map[string]string{"priority": "low"}}
	assert.False(t, ed.matchesFilter(msg, filter))

	// Test empty filter (matches all)
	filter = EventFilter{}
	assert.True(t, ed.matchesFilter(msg, filter))
}

func TestPipeline_Initialize(t *testing.T) {
	hive := createTestHive("pipeline")
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	logger := zap.NewNop()
	pipeline := NewPipeline(hive, bus, logger)

	err := pipeline.Initialize()
	require.NoError(t, err)

	pipeline.mu.RLock()
	assert.Equal(t, 2, len(pipeline.stages))
	pipeline.mu.RUnlock()
}

func TestHierarchical_Initialize(t *testing.T) {
	hive := createTestHive("hierarchical")
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	logger := zap.NewNop()
	hierarchical := NewHierarchical(hive, bus, logger)

	err := hierarchical.Initialize("supervisor1", []string{"worker1", "worker2"})
	require.NoError(t, err)

	hierarchical.mu.RLock()
	assert.Equal(t, "supervisor1", hierarchical.supervisorID)
	assert.Equal(t, 2, len(hierarchical.workerIDs))
	hierarchical.mu.RUnlock()
}

func TestNewOrchestrator(t *testing.T) {
	bus, cleanup := setupTestBus(t)
	defer cleanup()

	logger := zap.NewNop()

	// Test pipeline
	hive := createTestHive("pipeline")
	orch, err := NewOrchestrator(hive, bus, logger)
	require.NoError(t, err)
	_, ok := orch.(*Pipeline)
	assert.True(t, ok)

	// Test hierarchical
	hive.Spec.Pattern = "hierarchical"
	orch, err = NewOrchestrator(hive, bus, logger)
	require.NoError(t, err)
	_, ok = orch.(*Hierarchical)
	assert.True(t, ok)

	// Test swarm
	hive.Spec.Pattern = "swarm"
	orch, err = NewOrchestrator(hive, bus, logger)
	require.NoError(t, err)
	_, ok = orch.(*Swarm)
	assert.True(t, ok)

	// Test event-driven
	hive.Spec.Pattern = "event-driven"
	orch, err = NewOrchestrator(hive, bus, logger)
	require.NoError(t, err)
	_, ok = orch.(*EventDriven)
	assert.True(t, ok)

	// Test unknown pattern
	hive.Spec.Pattern = "unknown"
	_, err = NewOrchestrator(hive, bus, logger)
	assert.Error(t, err)
	
	// Test type assertions work
	hive.Spec.Pattern = "pipeline"
	orch, err = NewOrchestrator(hive, bus, logger)
	require.NoError(t, err)
	p, ok := orch.(*Pipeline)
	assert.True(t, ok)
	assert.NotNil(t, p)
}
