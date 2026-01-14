// Package waggle provides the Waggle communication protocol for agents.
package waggle

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Message represents a Waggle protocol message.
type Message struct {
	ID            string            `json:"id"`
	Version       string            `json:"version"`
	Type          MessageType       `json:"type"`
	Payload       json.RawMessage   `json:"payload"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Timestamp     time.Time         `json:"timestamp"`
	SourceAgentID string            `json:"sourceAgentId,omitempty"`
	DestAgentID   string            `json:"destAgentId,omitempty"`
	SessionID     string            `json:"sessionId,omitempty"`
	CorrelationID string            `json:"correlationId,omitempty"`
}

// MessageType indicates the type of message.
type MessageType string

const (
	MessageTypeTask     MessageType = "task"
	MessageTypeResult   MessageType = "result"
	MessageTypeRequest  MessageType = "request"
	MessageTypeResponse MessageType = "response"
	MessageTypeEvent    MessageType = "event"
	MessageTypeError    MessageType = "error"
)

// ProtocolVersion is the current Waggle protocol version.
const ProtocolVersion = "1.0"

// EncodeMessage encodes a message to JSON bytes.
func EncodeMessage(msg *Message) ([]byte, error) {
	return json.Marshal(msg)
}

// DecodeMessage decodes a message from JSON bytes.
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid message: %w", err)
	}

	return &msg, nil
}

// NewMessage creates a new message.
func NewMessage(msgType MessageType, payload interface{}) (*Message, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return &Message{
		ID:        uuid.New().String(),
		Version:   ProtocolVersion,
		Type:      msgType,
		Payload:   payloadBytes,
		Metadata:  make(map[string]string),
		Timestamp: time.Now(),
	}, nil
}

// Validate validates a Waggle message.
func (m *Message) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("message ID is required")
	}
	if m.Version == "" {
		return fmt.Errorf("message version is required")
	}
	if m.Type == "" {
		return fmt.Errorf("message type is required")
	}
	if m.Timestamp.IsZero() {
		return fmt.Errorf("message timestamp is required")
	}

	// Validate version compatibility
	if m.Version != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version: %s (expected %s)", m.Version, ProtocolVersion)
	}

	// Validate message type
	validTypes := map[MessageType]bool{
		MessageTypeTask:     true,
		MessageTypeResult:   true,
		MessageTypeRequest:  true,
		MessageTypeResponse: true,
		MessageTypeEvent:    true,
		MessageTypeError:    true,
	}
	if !validTypes[m.Type] {
		return fmt.Errorf("invalid message type: %s", m.Type)
	}

	return nil
}

// SetMetadata sets a metadata key-value pair.
func (m *Message) SetMetadata(key, value string) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}
	m.Metadata[key] = value
}

// GetMetadata gets a metadata value.
func (m *Message) GetMetadata(key string) string {
	if m.Metadata == nil {
		return ""
	}
	return m.Metadata[key]
}

// UnmarshalPayload unmarshals the message payload into the provided type.
func (m *Message) UnmarshalPayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// Encode encodes a message to JSON bytes.
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// ResponseCreator is a function that creates a response message.
type ResponseCreator func(interface{}) (*Message, error)

// NewRequestResponsePair creates a request message and returns a function to create the response.
func NewRequestResponsePair(requestType MessageType, payload interface{}) (*Message, ResponseCreator, error) {
	req, err := NewMessage(requestType, payload)
	if err != nil {
		return nil, nil, err
	}

	correlationID := req.ID
	req.CorrelationID = correlationID

	createResponse := func(responsePayload interface{}) (*Message, error) {
		resp, err := NewMessage(MessageTypeResponse, responsePayload)
		if err != nil {
			return nil, err
		}
		resp.CorrelationID = correlationID
		resp.DestAgentID = req.SourceAgentID
		return resp, nil
	}

	return req, createResponse, nil
}
