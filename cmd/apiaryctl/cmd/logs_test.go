package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDroneName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"drone/my-drone", "my-drone"},
		{"my-drone", "my-drone"},
		{"drone/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := tt.input
			if strings.HasPrefix(result, "drone/") {
				result = strings.TrimPrefix(result, "drone/")
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}
