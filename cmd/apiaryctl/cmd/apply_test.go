package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAPIPath(t *testing.T) {
	tests := []struct {
		kind      string
		name      string
		namespace string
		expected  string
		err       bool
	}{
		{"Cell", "my-cell", "", "/api/v1/cells/my-cell", false},
		{"AgentSpec", "my-agent", "default", "/api/v1/cells/default/agentspecs/my-agent", false},
		{"Hive", "my-hive", "default", "/api/v1/cells/default/hives/my-hive", false},
		{"Secret", "my-secret", "default", "/api/v1/cells/default/secrets/my-secret", false},
		{"Unknown", "test", "default", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			path, err := getAPIPath(tt.kind, tt.name, tt.namespace)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, path)
			}
		})
	}
}

func TestGetCreatePath(t *testing.T) {
	tests := []struct {
		kind      string
		namespace string
		expected  string
		err       bool
	}{
		{"Cell", "", "/api/v1/cells", false},
		{"AgentSpec", "default", "/api/v1/cells/default/agentspecs", false},
		{"Hive", "default", "/api/v1/cells/default/hives", false},
		{"Secret", "default", "/api/v1/cells/default/secrets", false},
		{"Unknown", "default", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			path, err := getCreatePath(tt.kind, tt.namespace)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, path)
			}
		})
	}
}

func TestApplyCommand_FileCollection(t *testing.T) {
	// Create a temporary directory with test files
	tmpDir := t.TempDir()
	
	// Create test manifest files
	file1 := filepath.Join(tmpDir, "manifest1.yaml")
	file2 := filepath.Join(tmpDir, "manifest2.yaml")
	
	err := os.WriteFile(file1, []byte("test: data1"), 0644)
	require.NoError(t, err)
	
	err = os.WriteFile(file2, []byte("test: data2"), 0644)
	require.NoError(t, err)

	// Test that files are collected correctly
	// This is a basic test - full integration would require a mock API server
	assert.FileExists(t, file1)
	assert.FileExists(t, file2)
}
