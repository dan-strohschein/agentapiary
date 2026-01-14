package guardrails

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewOutputValidator_ValidSchema(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"result": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"result"},
	}

	validator, err := NewOutputValidator(schema)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if validator == nil {
		t.Fatal("Expected validator to be non-nil")
	}
	if !validator.HasSchema() {
		t.Error("Expected validator to have schema")
	}
}

func TestNewOutputValidator_EmptySchema(t *testing.T) {
	validator, err := NewOutputValidator(map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if validator == nil {
		t.Fatal("Expected validator to be non-nil")
	}
	if validator.HasSchema() {
		t.Error("Expected validator to not have schema (empty schema)")
	}
}

func TestNewOutputValidator_InvalidSchema(t *testing.T) {
	invalidSchema := map[string]interface{}{
		"type": "invalid-type-value",
	}

	validator, err := NewOutputValidator(invalidSchema)
	if err == nil {
		t.Error("Expected error for invalid schema")
	}
	if validator != nil {
		t.Error("Expected validator to be nil on error")
	}
}

func TestOutputValidator_Validate_ValidOutput(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"result": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"result"},
	}

	validator, err := NewOutputValidator(schema)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	validOutput := map[string]interface{}{
		"result": "success",
	}
	outputBytes, _ := json.Marshal(validOutput)

	ctx := context.Background()
	err = validator.Validate(ctx, outputBytes)
	if err != nil {
		t.Errorf("Expected validation to pass, got error: %v", err)
	}
}

func TestOutputValidator_Validate_InvalidOutput(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"result": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"result"},
	}

	validator, err := NewOutputValidator(schema)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Missing required field
	invalidOutput := map[string]interface{}{
		"other": "value",
	}
	outputBytes, _ := json.Marshal(invalidOutput)

	ctx := context.Background()
	err = validator.Validate(ctx, outputBytes)
	if err == nil {
		t.Error("Expected validation to fail for invalid output")
	}

	// Check that it's a ValidationError
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}
	if !validationErr.Invalid {
		t.Error("Expected Invalid to be true")
	}
}

func TestOutputValidator_Validate_WrongType(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"count": map[string]interface{}{
				"type": "integer",
			},
		},
	}

	validator, err := NewOutputValidator(schema)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Wrong type for count field
	invalidOutput := map[string]interface{}{
		"count": "not-a-number",
	}
	outputBytes, _ := json.Marshal(invalidOutput)

	ctx := context.Background()
	err = validator.Validate(ctx, outputBytes)
	if err == nil {
		t.Error("Expected validation to fail for wrong type")
	}
}

func TestOutputValidator_Validate_InvalidJSON(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
	}

	validator, err := NewOutputValidator(schema)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	invalidJSON := []byte("{ invalid json }")

	ctx := context.Background()
	err = validator.Validate(ctx, invalidJSON)
	if err == nil {
		t.Error("Expected validation to fail for invalid JSON")
	}

	// Check that it's a ValidationError
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}
	if validationErr.Invalid {
		t.Error("Expected Invalid to be false for JSON parsing errors")
	}
}

func TestOutputValidator_Validate_NoSchema(t *testing.T) {
	validator := &OutputValidator{} // No schema

	outputBytes := []byte(`{"anything": "goes"}`)

	ctx := context.Background()
	err := validator.Validate(ctx, outputBytes)
	if err != nil {
		t.Errorf("Expected no validation error when no schema is set, got %v", err)
	}
}

func TestOutputValidator_HasSchema(t *testing.T) {
	// Validator with schema
	schema := map[string]interface{}{
		"type": "object",
	}
	validator, err := NewOutputValidator(schema)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}
	if !validator.HasSchema() {
		t.Error("Expected HasSchema to return true")
	}

	// Validator without schema
	emptyValidator := &OutputValidator{}
	if emptyValidator.HasSchema() {
		t.Error("Expected HasSchema to return false for empty validator")
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Message: "test error",
		Invalid: true,
	}

	errorMsg := err.Error()
	if errorMsg == "" {
		t.Error("Expected error message to be non-empty")
	}
	if errorMsg == "test error" {
		t.Error("Expected formatted error message")
	}
}
