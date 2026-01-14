package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantErr   bool
	}{
		{
			name:      "valid namespace",
			namespace: "default",
			wantErr:   false,
		},
		{
			name:      "valid namespace with hyphen",
			namespace: "my-namespace",
			wantErr:   false,
		},
		{
			name:      "valid namespace with numbers",
			namespace: "namespace123",
			wantErr:   false,
		},
		{
			name:      "empty namespace",
			namespace: "",
			wantErr:   true,
		},
		{
			name:      "namespace with uppercase",
			namespace: "Default",
			wantErr:   true,
		},
		{
			name:      "namespace with underscore",
			namespace: "my_namespace",
			wantErr:   true,
		},
		{
			name:      "namespace starting with hyphen",
			namespace: "-namespace",
			wantErr:   true,
		},
		{
			name:      "namespace ending with hyphen",
			namespace: "namespace-",
			wantErr:   true,
		},
		{
			name:      "namespace too long",
			namespace: "a" + string(make([]byte, 253)), // 254 chars
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNamespace(tt.namespace)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateResourceNamespace(t *testing.T) {
	tests := []struct {
		name             string
		resourceNamespace string
		urlNamespace     string
		wantErr          bool
	}{
		{
			name:              "matching namespaces",
			resourceNamespace: "default",
			urlNamespace:      "default",
			wantErr:           false,
		},
		{
			name:              "mismatched namespaces",
			resourceNamespace: "default",
			urlNamespace:      "production",
			wantErr:           true,
		},
		{
			name:              "empty resource namespace",
			resourceNamespace: "",
			urlNamespace:      "default",
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceNamespace(tt.resourceNamespace, tt.urlNamespace)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
