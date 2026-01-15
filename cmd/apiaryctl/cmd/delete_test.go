package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDeletePath(t *testing.T) {
	tests := []struct {
		resourceType string
		name         string
		namespace    string
		expected     string
		err          bool
	}{
		{"cell", "my-cell", "", "/api/v1/cells/my-cell", false},
		{"agentspec", "my-agent", "default", "/api/v1/cells/default/agentspecs/my-agent", false},
		{"hive", "my-hive", "default", "/api/v1/cells/default/hives/my-hive", false},
		{"secret", "my-secret", "default", "/api/v1/cells/default/secrets/my-secret", false},
		{"drone", "my-drone", "default", "/api/v1/cells/default/drones/my-drone", false},
		{"session", "my-session", "default", "/api/v1/cells/default/sessions/my-session", false},
		{"unknown", "test", "default", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			path, err := getDeletePath(tt.resourceType, tt.name, tt.namespace)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, path)
			}
		})
	}
}
