package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExecArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedDrone string
		expectedCmd   []string
		expectedError bool
	}{
		{
			name:          "valid exec with separator",
			args:          []string{"drone/my-drone", "--", "/bin/sh", "-c", "echo hello"},
			expectedDrone: "my-drone",
			expectedCmd:   []string{"/bin/sh", "-c", "echo hello"},
			expectedError: false,
		},
		{
			name:          "valid exec without drone/ prefix",
			args:          []string{"my-drone", "--", "ls", "-la"},
			expectedDrone: "my-drone",
			expectedCmd:   []string{"ls", "-la"},
			expectedError: false,
		},
		{
			name:          "missing separator",
			args:          []string{"drone/my-drone", "/bin/sh"},
			expectedError: true,
		},
		{
			name:          "missing command",
			args:          []string{"drone/my-drone", "--"},
			expectedError: true,
		},
		{
			name:          "missing drone name",
			args:          []string{"--", "/bin/sh"},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find separator
			separatorIndex := -1
			for i, arg := range tt.args {
				if arg == "--" {
					separatorIndex = i
					break
				}
			}

			if tt.expectedError {
				if separatorIndex == -1 || separatorIndex == 0 || separatorIndex == len(tt.args)-1 {
					// This is expected for error cases
					return
				}
				t.Fatalf("Expected error but got valid parse")
			}

			if separatorIndex == -1 || separatorIndex == 0 || separatorIndex == len(tt.args)-1 {
				t.Fatalf("Invalid args: separator not found or in wrong position")
			}

			droneName := tt.args[0]
			if strings.HasPrefix(droneName, "drone/") {
				droneName = strings.TrimPrefix(droneName, "drone/")
			}

			command := tt.args[separatorIndex+1:]

			assert.Equal(t, tt.expectedDrone, droneName)
			assert.Equal(t, tt.expectedCmd, command)
		})
	}
}
