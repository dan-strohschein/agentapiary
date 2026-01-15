package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeResourceType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"cell", "cell"},
		{"cells", "cell"},
		{"agentspec", "agentspec"},
		{"agentspecs", "agentspec"},
		{"hive", "hive"},
		{"hives", "hive"},
		{"secret", "secret"},
		{"secrets", "secret"},
		{"drone", "drone"},
		{"drones", "drone"},
		{"session", "session"},
		{"sessions", "session"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeResourceType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetGetPath(t *testing.T) {
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
			path, err := getGetPath(tt.resourceType, tt.name, tt.namespace)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, path)
			}
		})
	}
}

func TestGetListPath(t *testing.T) {
	tests := []struct {
		resourceType string
		namespace    string
		expected     string
		err          bool
	}{
		{"cell", "", "/api/v1/cells", false},
		{"agentspec", "default", "/api/v1/cells/default/agentspecs", false},
		{"hive", "default", "/api/v1/cells/default/hives", false},
		{"secret", "default", "/api/v1/cells/default/secrets", false},
		{"drone", "default", "/api/v1/cells/default/drones", false},
		{"session", "default", "/api/v1/cells/default/sessions", false},
		{"unknown", "default", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			path, err := getListPath(tt.resourceType, tt.namespace)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, path)
			}
		})
	}
}

func TestFilterBySelector(t *testing.T) {
	resources := []interface{}{
		map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "resource1",
				"labels": map[string]interface{}{
					"app": "myapp",
					"env": "prod",
				},
			},
		},
		map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "resource2",
				"labels": map[string]interface{}{
					"app": "otherapp",
					"env": "prod",
				},
			},
		},
		map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "resource3",
				"labels": map[string]interface{}{
					"env": "prod",
				},
			},
		},
	}

	// Filter by app=myapp
	filtered := filterBySelector(resources, "app=myapp")
	assert.Len(t, filtered, 1)

	// Filter by env=prod
	filtered = filterBySelector(resources, "env=prod")
	assert.Len(t, filtered, 3)

	// Filter by app=otherapp
	filtered = filterBySelector(resources, "app=otherapp")
	assert.Len(t, filtered, 1)
}
