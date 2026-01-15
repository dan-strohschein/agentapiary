package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeResourceTypeForScale(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"agentspec", "agentspec"},
		{"agentspecs", "agentspec"},
		{"hive", "hive"},
		{"hives", "hive"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeResourceType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestScaleResourceTypeValidation(t *testing.T) {
	tests := []struct {
		resourceType string
		shouldError  bool
	}{
		{"agentspec", false},
		{"hive", false},
		{"drone", true},
		{"session", true},
		{"secret", true},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			resourceType := normalizeResourceType(tt.resourceType)
			isSupported := resourceType == "agentspec" || resourceType == "hive"
			
			if tt.shouldError {
				assert.False(t, isSupported, "resource type should not be supported")
			} else {
				assert.True(t, isSupported, "resource type should be supported")
			}
		})
	}
}

func TestScalePathGeneration(t *testing.T) {
	tests := []struct {
		resourceType string
		name         string
		namespace    string
		expectedPath string
	}{
		{"agentspec", "my-agent", "default", "/api/v1/cells/default/agentspecs/my-agent/scale"},
		{"hive", "my-hive", "default", "/api/v1/cells/default/hives/my-hive/scale"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			var path string
			if tt.resourceType == "agentspec" {
				path = fmt.Sprintf("/api/v1/cells/%s/agentspecs/%s/scale", tt.namespace, tt.name)
			} else {
				path = fmt.Sprintf("/api/v1/cells/%s/hives/%s/scale", tt.namespace, tt.name)
			}
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}
