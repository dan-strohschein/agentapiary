// Package audit provides structured audit logging for Apiary.
//
// Audit Log Format:
//
// All audit events are logged as structured JSON using zap logger with the following fields:
//   - audit_type: The type of event (agent_input, agent_output, resource_create, etc.)
//   - audit_timestamp: ISO 8601 timestamp of when the event occurred
//   - audit_namespace: The Cell/namespace where the event occurred (optional)
//   - audit_hiveName: The Hive name (optional, for Hive-related events)
//   - audit_agentSpec: The AgentSpec name (optional, for agent-related events)
//   - audit_droneID: The Drone ID (optional, for Drone-related events)
//   - audit_sessionID: The Session ID (optional, for session-related events)
//   - audit_messageID: The message ID (optional, for message-related events)
//   - audit_resourceType: The resource type (optional, for resource events)
//   - audit_resourceName: The resource name (optional, for resource events)
//   - audit_guardrailType: The guardrail type (optional, for guardrail violations)
//   - audit_details: Additional event-specific details (map[string]interface{})
//
// Log Rotation/Retention:
//   Log rotation and retention should be configured at the system level using:
//   - logrotate (for file-based logging)
//   - journald (for systemd-based deployments)
//   - External log aggregation systems (e.g., ELK, Splunk, Loki)
//
// Example audit log entry:
//   {
//     "level": "info",
//     "ts": 1234567890.123,
//     "msg": "Audit event",
//     "audit_type": "agent_input",
//     "audit_timestamp": "2026-01-14T10:44:43.400-0500",
//     "audit_namespace": "default",
//     "audit_agentSpec": "my-agent",
//     "audit_droneID": "drone-123",
//     "audit_sessionID": "session-456",
//     "audit_messageID": "msg-789",
//     "audit_details": {
//       "messageType": "request",
//       "payloadSize": 100
//     }
//   }
package audit

import (
	"time"

	"go.uber.org/zap"
)

// EventType represents the type of audit event.
type EventType string

const (
	// EventTypeAgentInput represents an agent input message.
	EventTypeAgentInput EventType = "agent_input"
	// EventTypeAgentOutput represents an agent output message.
	EventTypeAgentOutput EventType = "agent_output"
	// EventTypeResourceCreate represents a resource creation.
	EventTypeResourceCreate EventType = "resource_create"
	// EventTypeResourceUpdate represents a resource update.
	EventTypeResourceUpdate EventType = "resource_update"
	// EventTypeResourceDelete represents a resource deletion.
	EventTypeResourceDelete EventType = "resource_delete"
	// EventTypeScalingUp represents a scaling up event.
	EventTypeScalingUp EventType = "scaling_up"
	// EventTypeScalingDown represents a scaling down event.
	EventTypeScalingDown EventType = "scaling_down"
	// EventTypeGuardrailViolation represents a guardrail violation.
	EventTypeGuardrailViolation EventType = "guardrail_violation"
)

// Event represents a structured audit log event.
type Event struct {
	// Type is the event type.
	Type EventType `json:"type"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Namespace is the Cell/namespace where the event occurred.
	Namespace string `json:"namespace,omitempty"`
	// HiveName is the Hive name (if applicable).
	HiveName string `json:"hiveName,omitempty"`
	// AgentSpec is the AgentSpec name (if applicable).
	AgentSpec string `json:"agentSpec,omitempty"`
	// DroneID is the Drone ID (if applicable).
	DroneID string `json:"droneID,omitempty"`
	// SessionID is the Session ID (if applicable).
	SessionID string `json:"sessionID,omitempty"`
	// MessageID is the message ID (if applicable).
	MessageID string `json:"messageID,omitempty"`
	// ResourceType is the resource type (if applicable).
	ResourceType string `json:"resourceType,omitempty"`
	// ResourceName is the resource name (if applicable).
	ResourceName string `json:"resourceName,omitempty"`
	// GuardrailType is the guardrail type (if applicable).
	GuardrailType string `json:"guardrailType,omitempty"`
	// Details contains additional event-specific details.
	Details map[string]interface{} `json:"details,omitempty"`
}

// Logger provides structured audit logging.
type Logger struct {
	logger *zap.Logger
}

// NewLogger creates a new audit logger.
func NewLogger(baseLogger *zap.Logger) *Logger {
	return &Logger{
		logger: baseLogger,
	}
}

// LogEvent logs an audit event as structured JSON.
func (l *Logger) LogEvent(event *Event) {
	if event == nil {
		return
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Build zap fields from event
	fields := []zap.Field{
		zap.String("audit_type", string(event.Type)),
		zap.Time("audit_timestamp", event.Timestamp),
	}

	if event.Namespace != "" {
		fields = append(fields, zap.String("audit_namespace", event.Namespace))
	}
	if event.HiveName != "" {
		fields = append(fields, zap.String("audit_hiveName", event.HiveName))
	}
	if event.AgentSpec != "" {
		fields = append(fields, zap.String("audit_agentSpec", event.AgentSpec))
	}
	if event.DroneID != "" {
		fields = append(fields, zap.String("audit_droneID", event.DroneID))
	}
	if event.SessionID != "" {
		fields = append(fields, zap.String("audit_sessionID", event.SessionID))
	}
	if event.MessageID != "" {
		fields = append(fields, zap.String("audit_messageID", event.MessageID))
	}
	if event.ResourceType != "" {
		fields = append(fields, zap.String("audit_resourceType", event.ResourceType))
	}
	if event.ResourceName != "" {
		fields = append(fields, zap.String("audit_resourceName", event.ResourceName))
	}
	if event.GuardrailType != "" {
		fields = append(fields, zap.String("audit_guardrailType", event.GuardrailType))
	}
	if event.Details != nil && len(event.Details) > 0 {
		fields = append(fields, zap.Any("audit_details", event.Details))
	}

	// Log at Info level (audit logs should always be at Info or above)
	l.logger.Info("Audit event", fields...)
}

// LogAgentInput logs an agent input message.
func (l *Logger) LogAgentInput(namespace, agentSpec, droneID, sessionID, messageID string, details map[string]interface{}) {
	l.LogEvent(&Event{
		Type:      EventTypeAgentInput,
		Namespace: namespace,
		AgentSpec: agentSpec,
		DroneID:   droneID,
		SessionID: sessionID,
		MessageID: messageID,
		Details:   details,
	})
}

// LogAgentOutput logs an agent output message.
func (l *Logger) LogAgentOutput(namespace, agentSpec, droneID, sessionID, messageID string, details map[string]interface{}) {
	l.LogEvent(&Event{
		Type:      EventTypeAgentOutput,
		Namespace: namespace,
		AgentSpec: agentSpec,
		DroneID:   droneID,
		SessionID: sessionID,
		MessageID: messageID,
		Details:   details,
	})
}

// LogResourceCreate logs a resource creation.
func (l *Logger) LogResourceCreate(resourceType, resourceName, namespace string, details map[string]interface{}) {
	l.LogEvent(&Event{
		Type:         EventTypeResourceCreate,
		ResourceType: resourceType,
		ResourceName: resourceName,
		Namespace:    namespace,
		Details:      details,
	})
}

// LogResourceUpdate logs a resource update.
func (l *Logger) LogResourceUpdate(resourceType, resourceName, namespace string, details map[string]interface{}) {
	l.LogEvent(&Event{
		Type:         EventTypeResourceUpdate,
		ResourceType: resourceType,
		ResourceName: resourceName,
		Namespace:    namespace,
		Details:      details,
	})
}

// LogResourceDelete logs a resource deletion.
func (l *Logger) LogResourceDelete(resourceType, resourceName, namespace string, details map[string]interface{}) {
	l.LogEvent(&Event{
		Type:         EventTypeResourceDelete,
		ResourceType: resourceType,
		ResourceName: resourceName,
		Namespace:    namespace,
		Details:      details,
	})
}

// LogScalingUp logs a scaling up event.
func (l *Logger) LogScalingUp(namespace, agentSpec string, fromReplicas, toReplicas int, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["fromReplicas"] = fromReplicas
	details["toReplicas"] = toReplicas
	l.LogEvent(&Event{
		Type:      EventTypeScalingUp,
		Namespace: namespace,
		AgentSpec: agentSpec,
		Details:   details,
	})
}

// LogScalingDown logs a scaling down event.
func (l *Logger) LogScalingDown(namespace, agentSpec string, fromReplicas, toReplicas int, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["fromReplicas"] = fromReplicas
	details["toReplicas"] = toReplicas
	l.LogEvent(&Event{
		Type:      EventTypeScalingDown,
		Namespace: namespace,
		AgentSpec: agentSpec,
		Details:   details,
	})
}

// LogGuardrailViolation logs a guardrail violation.
func (l *Logger) LogGuardrailViolation(guardrailType, namespace, hiveName, agentSpec, droneID, sessionID, messageID, errorMsg string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	if errorMsg != "" {
		details["error"] = errorMsg
	}
	l.LogEvent(&Event{
		Type:          EventTypeGuardrailViolation,
		GuardrailType: guardrailType,
		Namespace:     namespace,
		HiveName:      hiveName,
		AgentSpec:     agentSpec,
		DroneID:       droneID,
		SessionID:     sessionID,
		MessageID:     messageID,
		Details:       details,
	})
}
