package comb

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ConversationMessage represents a message in conversation history.
type ConversationMessage struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	AgentID   string                 `json:"agentId,omitempty"`
	Type      string                 `json:"type"` // user, assistant, system
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ArtifactMetadata represents metadata for an artifact.
type ArtifactMetadata struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Type      string    `json:"type,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Structured provides structured access patterns for Comb.
type Structured interface {
	// Conversation history
	AppendConversation(ctx context.Context, sessionID string, msg ConversationMessage) error
	GetConversation(ctx context.Context, sessionID string, limit int) ([]ConversationMessage, error)
	
	// Artifacts
	StoreArtifact(ctx context.Context, sessionID, name string, data []byte) error
	GetArtifact(ctx context.Context, sessionID, name string) ([]byte, error)
	ListArtifacts(ctx context.Context, sessionID string) ([]ArtifactMetadata, error)
	
	// State machine
	SetState(ctx context.Context, sessionID, state string) error
	GetState(ctx context.Context, sessionID string) (string, error)
	TransitionState(ctx context.Context, sessionID, from, to string) error
	GetStateHistory(ctx context.Context, sessionID string) ([]StateTransition, error)
	
	// Scratchpad
	SetScratchpad(ctx context.Context, sessionID, key, value string) error
	GetScratchpad(ctx context.Context, sessionID, key string) (string, error)
	ClearScratchpad(ctx context.Context, sessionID string) error
}

// StateTransition represents a state transition.
type StateTransition struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason,omitempty"`
}

// structuredStore extends Store with structured access patterns.
type structuredStore struct {
	*Store
}

// conversationKey returns the key for conversation history.
func conversationKey(sessionID string) string {
	return fmt.Sprintf("_conversation:%s", sessionID)
}

// artifactKey returns the key for an artifact.
func artifactKey(sessionID, name string) string {
	return fmt.Sprintf("_artifact:%s:%s", sessionID, name)
}

// artifactListKey returns the key for artifact list.
func artifactListKey(sessionID string) string {
	return fmt.Sprintf("_artifacts:%s", sessionID)
}

// stateKey returns the key for current state.
func stateKey(sessionID string) string {
	return fmt.Sprintf("_state:%s", sessionID)
}

// stateHistoryKey returns the key for state history.
func stateHistoryKey(sessionID string) string {
	return fmt.Sprintf("_state_history:%s", sessionID)
}

// scratchpadKey returns the key for scratchpad.
func scratchpadKey(sessionID, key string) string {
	return fmt.Sprintf("_scratchpad:%s:%s", sessionID, key)
}

// AppendConversation appends a message to conversation history.
func (s *Store) AppendConversation(ctx context.Context, sessionID string, msg ConversationMessage) error {
	// Get existing conversation
	convKey := conversationKey(sessionID)
	existingJSON, err := s.Get(ctx, sessionID, convKey)
	
	var messages []ConversationMessage
	if err == nil {
		if err := json.Unmarshal([]byte(existingJSON), &messages); err != nil {
			// If unmarshal fails, start fresh
			messages = []ConversationMessage{}
		}
	}

	// Set message ID if not set
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Append message
	messages = append(messages, msg)

	// Store back
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation: %w", err)
	}

	return s.Set(ctx, sessionID, convKey, string(messagesJSON), 0)
}

// GetConversation retrieves conversation history.
func (s *Store) GetConversation(ctx context.Context, sessionID string, limit int) ([]ConversationMessage, error) {
	convKey := conversationKey(sessionID)
	existingJSON, err := s.Get(ctx, sessionID, convKey)
	if err != nil {
		return []ConversationMessage{}, nil // Return empty if not found
	}

	var messages []ConversationMessage
	if err := json.Unmarshal([]byte(existingJSON), &messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal conversation: %w", err)
	}

	// Apply limit (get last N messages)
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	return messages, nil
}

// StoreArtifact stores an artifact.
func (s *Store) StoreArtifact(ctx context.Context, sessionID, name string, data []byte) error {
	// Store artifact data (base64 encoded for string storage)
	artifactKey := artifactKey(sessionID, name)
	dataStr := string(data) // For now, store as string; in production, use base64
	
	if err := s.Set(ctx, sessionID, artifactKey, dataStr, 0); err != nil {
		return err
	}

	// Update artifact list
	listKey := artifactListKey(sessionID)
	existingJSON, _ := s.Get(ctx, sessionID, listKey)
	
	var artifacts []ArtifactMetadata
	if existingJSON != "" {
		json.Unmarshal([]byte(existingJSON), &artifacts)
	}

	// Add or update artifact metadata
	found := false
	for i, art := range artifacts {
		if art.Name == name {
			artifacts[i] = ArtifactMetadata{
				Name:      name,
				Size:      int64(len(data)),
				CreatedAt: time.Now(),
			}
			found = true
			break
		}
	}
	if !found {
		artifacts = append(artifacts, ArtifactMetadata{
			Name:      name,
			Size:      int64(len(data)),
			CreatedAt: time.Now(),
		})
	}

	artifactsJSON, err := json.Marshal(artifacts)
	if err != nil {
		return fmt.Errorf("failed to marshal artifacts: %w", err)
	}

	return s.Set(ctx, sessionID, listKey, string(artifactsJSON), 0)
}

// GetArtifact retrieves an artifact.
func (s *Store) GetArtifact(ctx context.Context, sessionID, name string) ([]byte, error) {
	artifactKey := artifactKey(sessionID, name)
	dataStr, err := s.Get(ctx, sessionID, artifactKey)
	if err != nil {
		return nil, err
	}
	return []byte(dataStr), nil
}

// ListArtifacts lists all artifacts for a session.
func (s *Store) ListArtifacts(ctx context.Context, sessionID string) ([]ArtifactMetadata, error) {
	listKey := artifactListKey(sessionID)
	existingJSON, err := s.Get(ctx, sessionID, listKey)
	if err != nil {
		return []ArtifactMetadata{}, nil
	}

	var artifacts []ArtifactMetadata
	if err := json.Unmarshal([]byte(existingJSON), &artifacts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal artifacts: %w", err)
	}

	return artifacts, nil
}

// SetState sets the current state.
func (s *Store) SetState(ctx context.Context, sessionID, state string) error {
	stateKey := stateKey(sessionID)
	return s.Set(ctx, sessionID, stateKey, state, 0)
}

// GetState retrieves the current state.
func (s *Store) GetState(ctx context.Context, sessionID string) (string, error) {
	stateKey := stateKey(sessionID)
	state, err := s.Get(ctx, sessionID, stateKey)
	if err != nil {
		return "", err
	}
	return state, nil
}

// TransitionState transitions from one state to another.
func (s *Store) TransitionState(ctx context.Context, sessionID, from, to string) error {
	// Get current state
	currentState, err := s.GetState(ctx, sessionID)
	if err != nil && err.Error() != "key not found: "+stateKey(sessionID) {
		return err
	}

	// Validate transition
	if currentState != "" && currentState != from {
		return fmt.Errorf("invalid transition: current state is %s, expected %s", currentState, from)
	}

	// Set new state
	if err := s.SetState(ctx, sessionID, to); err != nil {
		return err
	}

	// Record transition in history
	historyKey := stateHistoryKey(sessionID)
	existingJSON, _ := s.Get(ctx, sessionID, historyKey)
	
	var history []StateTransition
	if existingJSON != "" {
		json.Unmarshal([]byte(existingJSON), &history)
	}

	history = append(history, StateTransition{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
	})

	historyJSON, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("failed to marshal state history: %w", err)
	}

	return s.Set(ctx, sessionID, historyKey, string(historyJSON), 0)
}

// GetStateHistory retrieves state transition history.
func (s *Store) GetStateHistory(ctx context.Context, sessionID string) ([]StateTransition, error) {
	historyKey := stateHistoryKey(sessionID)
	existingJSON, err := s.Get(ctx, sessionID, historyKey)
	if err != nil {
		return []StateTransition{}, nil
	}

	var history []StateTransition
	if err := json.Unmarshal([]byte(existingJSON), &history); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state history: %w", err)
	}

	return history, nil
}

// SetScratchpad sets a scratchpad value.
func (s *Store) SetScratchpad(ctx context.Context, sessionID, key, value string) error {
	scratchKey := scratchpadKey(sessionID, key)
	return s.Set(ctx, sessionID, scratchKey, value, 0)
}

// GetScratchpad retrieves a scratchpad value.
func (s *Store) GetScratchpad(ctx context.Context, sessionID, key string) (string, error) {
	scratchKey := scratchpadKey(sessionID, key)
	return s.Get(ctx, sessionID, scratchKey)
}

// ClearScratchpad clears all scratchpad values for a session.
func (s *Store) ClearScratchpad(ctx context.Context, sessionID string) error {
	// Get all keys for the session
	allKeys, err := s.Keys(ctx, sessionID, "*")
	if err != nil {
		return err
	}

	// Delete all scratchpad keys (keys starting with _scratchpad:sessionID:)
	prefix := fmt.Sprintf("_scratchpad:%s:", sessionID)
	for _, key := range allKeys {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			if err := s.Delete(ctx, sessionID, key); err != nil {
				// Continue on error
				continue
			}
		}
	}

	return nil
}
