// Package manifest provides parsing, validation, and serialization of Apiary resource manifests.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/agentapiary/apiary/pkg/apiary"
	"gopkg.in/yaml.v3"
)

// Manifest represents a parsed resource manifest.
type Manifest struct {
	APIVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   apiary.ObjectMeta      `yaml:"metadata" json:"metadata"`
	Spec       map[string]interface{} `yaml:"spec" json:"spec"`
	Resource   apiary.Resource        `yaml:"-" json:"-"`
}

// ParseManifest parses a manifest from raw bytes (YAML or JSON).
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest

	// Try YAML first
	if err := yaml.Unmarshal(data, &m); err != nil {
		// Try JSON
		if err2 := json.Unmarshal(data, &m); err2 != nil {
			return nil, fmt.Errorf("failed to parse as YAML: %w, failed to parse as JSON: %v", err, err2)
		}
	}

	// Validate critical fields that would cause panics if missing
	// Full validation should be done via Validate() function
	if m.APIVersion == "" {
		return nil, fmt.Errorf("apiVersion is required")
	}
	if m.Kind == "" {
		return nil, fmt.Errorf("kind is required")
	}
	// Note: metadata.name validation is done in Validate() to allow
	// parsing incomplete manifests for validation purposes, but we
	// validate it before converting to resource to prevent panics

	// Convert to typed resource (only if metadata.name is present)
	// This allows parsing incomplete manifests for validation purposes
	if m.Metadata.Name != "" {
		resource, err := m.toResource()
		if err != nil {
			return nil, fmt.Errorf("failed to convert to resource: %w", err)
		}
		m.Resource = resource
	}
	// If metadata.name is missing, Resource will be nil, which Validate() will catch

	return &m, nil
}

// ParseManifestFile parses a manifest from a file.
func ParseManifestFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ParseManifest(data)
}

// toResource converts a manifest to a typed Resource.
func (m *Manifest) toResource() (apiary.Resource, error) {
	// Marshal spec to JSON for re-unmarshaling into typed struct
	specData, err := json.Marshal(m.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}

	switch m.Kind {
	case "AgentSpec":
		var spec apiary.AgentSpec
		spec.TypeMeta = apiary.TypeMeta{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
		}
		spec.ObjectMeta = m.Metadata
		if err := json.Unmarshal(specData, &spec.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal AgentSpec spec: %w", err)
		}
		return &spec, nil

	case "Hive":
		var hive apiary.Hive
		hive.TypeMeta = apiary.TypeMeta{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
		}
		hive.ObjectMeta = m.Metadata
		if err := json.Unmarshal(specData, &hive.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Hive spec: %w", err)
		}
		return &hive, nil

	case "Cell":
		var cell apiary.Cell
		cell.TypeMeta = apiary.TypeMeta{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
		}
		cell.ObjectMeta = m.Metadata
		if err := json.Unmarshal(specData, &cell.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Cell spec: %w", err)
		}
		return &cell, nil

	case "Session":
		var session apiary.Session
		session.TypeMeta = apiary.TypeMeta{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
		}
		session.ObjectMeta = m.Metadata
		if err := json.Unmarshal(specData, &session.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Session spec: %w", err)
		}
		return &session, nil

	case "Secret":
		var secret apiary.Secret
		secret.TypeMeta = apiary.TypeMeta{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
		}
		secret.ObjectMeta = m.Metadata
		// Secrets have a different structure - data is map[string]string in YAML but []byte in Go
		if dataMap, ok := m.Spec["data"].(map[string]interface{}); ok {
			secret.Data = make(map[string][]byte)
			for k, v := range dataMap {
				if str, ok := v.(string); ok {
					secret.Data[k] = []byte(str)
				}
			}
		}
		return &secret, nil

	default:
		return nil, fmt.Errorf("unknown resource kind: %s", m.Kind)
	}
}

// Serialize serializes a manifest to bytes in the specified format.
func (m *Manifest) Serialize(format string) ([]byte, error) {
	if m.Resource == nil {
		return nil, fmt.Errorf("manifest has no resource")
	}

	// Convert resource back to manifest structure
	m.Kind = m.Resource.GetKind()
	m.APIVersion = "apiary.io/v1"
	m.Metadata = apiary.ObjectMeta{
		Name:        m.Resource.GetName(),
		Namespace:   m.Resource.GetNamespace(),
		UID:         m.Resource.GetUID(),
		Labels:      m.Resource.GetLabels(),
		Annotations: m.Resource.GetAnnotations(),
		CreatedAt:   m.Resource.GetCreatedAt(),
		UpdatedAt:   m.Resource.GetUpdatedAt(),
	}

	// Extract spec based on kind - marshal to JSON then unmarshal to map
	specData, err := json.Marshal(m.Resource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource: %w", err)
	}

	var resourceMap map[string]interface{}
	if err := json.Unmarshal(specData, &resourceMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource: %w", err)
	}

	if spec, ok := resourceMap["spec"].(map[string]interface{}); ok {
		m.Spec = spec
	} else {
		// For Secret, handle data field specially
		if secret, ok := m.Resource.(*apiary.Secret); ok {
			data := make(map[string]string)
			for k, v := range secret.Data {
				data[k] = string(v) // In real implementation, would base64 encode
			}
			m.Spec = map[string]interface{}{
				"data": data,
			}
		}
	}

	format = strings.ToLower(format)
	switch format {
	case "yaml", "yml":
		return yaml.Marshal(m)
	case "json":
		return json.MarshalIndent(m, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s (use yaml or json)", format)
	}
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate validates a manifest.
func Validate(manifest *Manifest) ([]ValidationError, error) {
	var errors []ValidationError

	// Validate APIVersion
	if manifest.APIVersion == "" {
		errors = append(errors, ValidationError{
			Field:   "apiVersion",
			Message: "apiVersion is required",
		})
	} else if manifest.APIVersion != "apiary.io/v1" {
		errors = append(errors, ValidationError{
			Field:   "apiVersion",
			Message: fmt.Sprintf("unsupported apiVersion: %s (expected apiary.io/v1)", manifest.APIVersion),
		})
	}

	if manifest.Kind == "" {
		errors = append(errors, ValidationError{
			Field:   "kind",
			Message: "kind is required",
		})
	}

	// Validate Metadata
	if manifest.Metadata.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "metadata.name",
			Message: "metadata.name is required",
		})
	}

	// Validate spec based on kind
	if manifest.Resource != nil {
		switch r := manifest.Resource.(type) {
		case *apiary.AgentSpec:
			errors = append(errors, validateAgentSpec(r)...)
		case *apiary.Hive:
			errors = append(errors, validateHive(r)...)
		case *apiary.Cell:
			errors = append(errors, validateCell(r)...)
		}
	}
	
	// Also validate from raw spec for incomplete manifests or to catch issues
	// This ensures validation works even when Resource is nil
	if manifest.Kind == "AgentSpec" {
		if runtimeMap, ok := manifest.Spec["runtime"].(map[string]interface{}); ok {
			if command, ok := runtimeMap["command"].([]interface{}); !ok || len(command) == 0 {
				// Only add if not already validated from Resource
				found := false
				for _, e := range errors {
					if e.Field == "spec.runtime.command" {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, ValidationError{
						Field:   "spec.runtime.command",
						Message: "spec.runtime.command is required",
					})
				}
			}
		}
		if interfaceMap, ok := manifest.Spec["interface"].(map[string]interface{}); ok {
			if ifaceType, ok := interfaceMap["type"].(string); !ok || ifaceType == "" {
				// Only add if not already validated from Resource
				found := false
				for _, e := range errors {
					if e.Field == "spec.interface.type" {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, ValidationError{
						Field:   "spec.interface.type",
						Message: "spec.interface.type is required",
					})
				}
			}
		}
	}

	return errors, nil
}

// validateAgentSpec validates an AgentSpec.
func validateAgentSpec(spec *apiary.AgentSpec) []ValidationError {
	var errors []ValidationError

	if len(spec.Spec.Runtime.Command) == 0 {
		errors = append(errors, ValidationError{
			Field:   "spec.runtime.command",
			Message: "spec.runtime.command is required",
		})
	}

	if spec.Spec.Interface.Type == "" {
		errors = append(errors, ValidationError{
			Field:   "spec.interface.type",
			Message: "spec.interface.type is required",
		})
	}

	validInterfaceTypes := map[string]bool{
		"stdin":       true,
		"http":        true,
		"grpc":        true,
		"unix-socket": true,
	}
	if !validInterfaceTypes[spec.Spec.Interface.Type] {
		errors = append(errors, ValidationError{
			Field:   "spec.interface.type",
			Message: fmt.Sprintf("invalid interface type: %s (must be one of: stdin, http, grpc, unix-socket)", spec.Spec.Interface.Type),
		})
	}

	if spec.Spec.Scaling.MinReplicas < 0 {
		errors = append(errors, ValidationError{
			Field:   "spec.scaling.minReplicas",
			Message: "spec.scaling.minReplicas must be >= 0",
		})
	}

	if spec.Spec.Scaling.MaxReplicas > 0 && spec.Spec.Scaling.MaxReplicas < spec.Spec.Scaling.MinReplicas {
		errors = append(errors, ValidationError{
			Field:   "spec.scaling.maxReplicas",
			Message: "spec.scaling.maxReplicas must be >= spec.scaling.minReplicas",
		})
	}

	return errors
}

// validateHive validates a Hive.
func validateHive(hive *apiary.Hive) []ValidationError {
	var errors []ValidationError

	if hive.Spec.Pattern != "" {
		validPatterns := map[string]bool{
			"pipeline":      true,
			"hierarchical":  true,
			"swarm":         true,
			"event-driven":  true,
		}
		if !validPatterns[hive.Spec.Pattern] {
			errors = append(errors, ValidationError{
				Field:   "spec.pattern",
				Message: fmt.Sprintf("invalid pattern: %s (must be one of: pipeline, hierarchical, swarm, event-driven)", hive.Spec.Pattern),
			})
		}
	}

	if hive.Spec.Pattern == "pipeline" && len(hive.Spec.Stages) == 0 {
		errors = append(errors, ValidationError{
			Field:   "spec.stages",
			Message: "spec.stages is required for pipeline pattern",
		})
	}

	for i, stage := range hive.Spec.Stages {
		if stage.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.stages[%d].name", i),
				Message: "stage name is required",
			})
		}
		if stage.AgentRef == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.stages[%d].agentRef", i),
				Message: "stage agentRef is required",
			})
		}
		if stage.Replicas < 0 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.stages[%d].replicas", i),
				Message: "stage replicas must be >= 0",
			})
		}
	}

	return errors
}

// validateCell validates a Cell.
func validateCell(cell *apiary.Cell) []ValidationError {
	var errors []ValidationError

	if cell.Spec.ResourceQuota.MaxHives > 0 && cell.Spec.ResourceQuota.MaxHives < 0 {
		errors = append(errors, ValidationError{
			Field:   "spec.resourceQuota.maxHives",
			Message: "spec.resourceQuota.maxHives must be >= 0",
		})
	}

	return errors
}

// ParseAndValidate parses and validates a manifest in one step.
func ParseAndValidate(data []byte) (*Manifest, []ValidationError, error) {
	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, nil, err
	}

	errors, err := Validate(manifest)
	if err != nil {
		return nil, nil, err
	}

	return manifest, errors, nil
}

// ParseAndValidateFile parses and validates a manifest file.
func ParseAndValidateFile(path string) (*Manifest, []ValidationError, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ParseAndValidate(data)
}
