package nectar

import (
	"encoding/json"
	"fmt"
	"time"
)

// encodeMessage encodes a message to bytes.
func encodeMessage(msg *Message) ([]byte, error) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Simple JSON encoding for now
	// Can be extended with binary codec for performance
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	return data, nil
}

// decodeMessage decodes bytes to a message.
func decodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}
