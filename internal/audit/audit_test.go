package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestNewLogger(t *testing.T) {
	logger := zap.NewNop()
	auditLogger := NewLogger(logger)
	assert.NotNil(t, auditLogger)
	assert.Equal(t, logger, auditLogger.logger)
}

func TestLogEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	event := &Event{
		Type:      EventTypeAgentInput,
		Timestamp: time.Now(),
		Namespace: "test-ns",
		AgentSpec: "test-agent",
		DroneID:   "test-drone",
		SessionID: "test-session",
		MessageID: "test-msg",
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	// Should not panic
	auditLogger.LogEvent(event)
}

func TestLogEvent_NilEvent(t *testing.T) {
	logger := zap.NewNop()
	auditLogger := NewLogger(logger)

	// Should not panic
	auditLogger.LogEvent(nil)
}

func TestLogEvent_ZeroTimestamp(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	event := &Event{
		Type: EventTypeAgentInput,
		// Timestamp is zero
	}

	auditLogger.LogEvent(event)
	// Timestamp should be set (we can't easily verify this without inspecting logs)
}

func TestLogAgentInput(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"messageType": "request",
		"payloadSize": 100,
	}

	auditLogger.LogAgentInput("test-ns", "test-agent", "test-drone", "test-session", "test-msg", details)
	// Should not panic
}

func TestLogAgentOutput(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"messageType": "response",
		"payloadSize": 200,
		"tokenCount":   50,
	}

	auditLogger.LogAgentOutput("test-ns", "test-agent", "test-drone", "test-session", "test-msg", details)
	// Should not panic
}

func TestLogResourceCreate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"agentSpec": "test-agent",
		"uid":       "test-uid",
	}

	auditLogger.LogResourceCreate("Drone", "test-drone", "test-ns", details)
	// Should not panic
}

func TestLogResourceUpdate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"previousVersion": "v1",
		"newVersion":      "v2",
	}

	auditLogger.LogResourceUpdate("AgentSpec", "test-agent", "test-ns", details)
	// Should not panic
}

func TestLogResourceDelete(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"agentSpec": "test-agent",
		"uid":       "test-uid",
	}

	auditLogger.LogResourceDelete("Drone", "test-drone", "test-ns", details)
	// Should not panic
}

func TestLogScalingUp(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"reason": "autoscaling",
	}

	auditLogger.LogScalingUp("test-ns", "test-agent", 2, 5, details)
	// Should not panic
	// Details should include fromReplicas and toReplicas
}

func TestLogScalingUp_NilDetails(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	auditLogger.LogScalingUp("test-ns", "test-agent", 2, 5, nil)
	// Should not panic
	// Details should be created and include fromReplicas and toReplicas
}

func TestLogScalingDown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"reason": "reconciliation",
	}

	auditLogger.LogScalingDown("test-ns", "test-agent", 5, 2, details)
	// Should not panic
	// Details should include fromReplicas and toReplicas
}

func TestLogScalingDown_NilDetails(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	auditLogger.LogScalingDown("test-ns", "test-agent", 5, 2, nil)
	// Should not panic
	// Details should be created and include fromReplicas and toReplicas
}

func TestLogGuardrailViolation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	details := map[string]interface{}{
		"requestsPerMinute": 100,
	}

	auditLogger.LogGuardrailViolation(
		"rate_limit",
		"test-ns",
		"test-hive",
		"test-agent",
		"test-drone",
		"test-session",
		"test-msg",
		"rate limit exceeded",
		details,
	)
	// Should not panic
}

func TestLogGuardrailViolation_NilDetails(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	auditLogger.LogGuardrailViolation(
		"token_budget",
		"test-ns",
		"test-hive",
		"test-agent",
		"test-drone",
		"test-session",
		"test-msg",
		"token budget exceeded",
		nil,
	)
	// Should not panic
	// Details should be created and include error
}

func TestLogGuardrailViolation_EmptyError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	auditLogger := NewLogger(logger)

	auditLogger.LogGuardrailViolation(
		"timeout",
		"test-ns",
		"test-hive",
		"test-agent",
		"test-drone",
		"test-session",
		"test-msg",
		"", // Empty error
		nil,
	)
	// Should not panic
}

func TestEventTypes(t *testing.T) {
	// Verify all event types are defined
	assert.Equal(t, EventType("agent_input"), EventTypeAgentInput)
	assert.Equal(t, EventType("agent_output"), EventTypeAgentOutput)
	assert.Equal(t, EventType("resource_create"), EventTypeResourceCreate)
	assert.Equal(t, EventType("resource_update"), EventTypeResourceUpdate)
	assert.Equal(t, EventType("resource_delete"), EventTypeResourceDelete)
	assert.Equal(t, EventType("scaling_up"), EventTypeScalingUp)
	assert.Equal(t, EventType("scaling_down"), EventTypeScalingDown)
	assert.Equal(t, EventType("guardrail_violation"), EventTypeGuardrailViolation)
}
