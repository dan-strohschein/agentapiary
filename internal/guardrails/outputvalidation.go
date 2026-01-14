// Package guardrails provides safety guardrails for agent execution.
package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// OutputValidator validates agent output against JSON schemas.
type OutputValidator struct {
	schema *jsonschema.Schema
	mu     sync.RWMutex
}

// NewOutputValidator creates a new output validator from a JSON schema.
func NewOutputValidator(schema map[string]interface{}) (*OutputValidator, error) {
	if len(schema) == 0 {
		// No schema means no validation
		return &OutputValidator{}, nil
	}

	// Convert schema to JSON bytes
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Compile schema
	compiler := jsonschema.NewCompiler()
	schemaID := "schema.json"
	if err := compiler.AddResource(schemaID, bytes.NewReader(schemaBytes)); err != nil {
		return nil, fmt.Errorf("failed to add schema resource: %w", err)
	}

	compiledSchema, err := compiler.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
	}

	return &OutputValidator{
		schema: compiledSchema,
	}, nil
}

// Validate validates output data against the schema.
func (v *OutputValidator) Validate(ctx context.Context, output []byte) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// No schema means no validation (always valid)
	if v.schema == nil {
		return nil
	}

	// Unmarshal output to validate
	var data interface{}
	if err := json.Unmarshal(output, &data); err != nil {
		return &ValidationError{
			Message: fmt.Sprintf("failed to unmarshal output: %v", err),
			Invalid: false,
		}
	}

	// Validate against schema
	if err := v.schema.Validate(data); err != nil {
		return &ValidationError{
			Message: err.Error(),
			Invalid: true,
			Details: err,
		}
	}

	return nil
}

// HasSchema returns true if a schema is configured.
func (v *OutputValidator) HasSchema() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.schema != nil
}

// ValidationError indicates that output validation failed.
type ValidationError struct {
	Message string
	Invalid bool
	Details error
}

func (e *ValidationError) Error() string {
	if e.Invalid {
		return fmt.Sprintf("output validation failed: %s", e.Message)
	}
	return fmt.Sprintf("output validation error: %s", e.Message)
}

func (e *ValidationError) Unwrap() error {
	return e.Details
}
