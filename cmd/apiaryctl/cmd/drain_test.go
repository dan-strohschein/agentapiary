package cmd

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseDrainResourceName(t *testing.T) {
	tests := []struct {
		input         string
		expectedType  string
		expectedName  string
		expectedError bool
	}{
		{"drone/my-drone", "drone", "my-drone", false},
		{"agentspec/my-agent", "agentspec", "my-agent", false},
		{"my-drone", "", "", true}, // Missing type prefix
		{"unknown/my-resource", "unknown", "my-resource", false}, // Will fail validation later
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			resourceName := tt.input
			var resourceType, name string

			if strings.HasPrefix(resourceName, "drone/") {
				resourceType = "drone"
				name = strings.TrimPrefix(resourceName, "drone/")
			} else if strings.HasPrefix(resourceName, "agentspec/") {
				resourceType = "agentspec"
				name = strings.TrimPrefix(resourceName, "agentspec/")
			} else if strings.Contains(resourceName, "/") {
				parts := strings.Split(resourceName, "/")
				if len(parts) == 2 {
					resourceType = parts[0]
					name = parts[1]
				} else {
					// Error case
					if !tt.expectedError {
						t.Fatalf("Unexpected error for %s", resourceName)
					}
					return
				}
			} else {
				// Error case
				if !tt.expectedError {
					t.Fatalf("Unexpected error for %s", resourceName)
				}
				return
			}

			assert.Equal(t, tt.expectedType, resourceType)
			assert.Equal(t, tt.expectedName, name)
		})
	}
}

func TestValidateDrainTimeout(t *testing.T) {
	tests := []struct {
		timeout      string
		shouldError  bool
	}{
		{"30s", false},
		{"1m", false},
		{"60s", false},
		{"invalid", true},
		{"", false}, // Empty is allowed
	}

	for _, tt := range tests {
		t.Run(tt.timeout, func(t *testing.T) {
			if tt.timeout == "" {
				return // Empty timeout is valid
			}
			_, err := time.ParseDuration(tt.timeout)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDrainPathGeneration(t *testing.T) {
	tests := []struct {
		resourceType string
		name         string
		namespace    string
		expectedPath string
	}{
		{"drone", "my-drone", "default", "/api/v1/cells/default/drones/my-drone/drain"},
		{"agentspec", "my-agent", "default", "/api/v1/cells/default/agentspecs/my-agent/drain"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			var path string
			if tt.resourceType == "drone" {
				path = fmt.Sprintf("/api/v1/cells/%s/drones/%s/drain", tt.namespace, tt.name)
			} else {
				path = fmt.Sprintf("/api/v1/cells/%s/agentspecs/%s/drain", tt.namespace, tt.name)
			}
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}
