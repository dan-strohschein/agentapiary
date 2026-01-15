package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterfaceSliceToStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected []string
	}{
		{"strings", []interface{}{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"mixed", []interface{}{"a", 1, true}, []string{"a", "1", "true"}},
		{"empty", []interface{}{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interfaceSliceToStringSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFloat64(t *testing.T) {
	m := map[string]interface{}{
		"value": 42.5,
		"zero":  0.0,
		"str":   "not a number",
	}

	assert.Equal(t, 42.5, getFloat64(m, "value"))
	assert.Equal(t, 0.0, getFloat64(m, "zero"))
	assert.Equal(t, 0.0, getFloat64(m, "nonexistent"))
	assert.Equal(t, 0.0, getFloat64(m, "str"))
}
