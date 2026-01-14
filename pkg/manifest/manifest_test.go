package manifest

import (
	"testing"
	"time"

	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest_YAML(t *testing.T) {
	yamlData := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: test-agent
  namespace: default
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: stdin
`

	manifest, err := ParseManifest([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, manifest)
	assert.Equal(t, "AgentSpec", manifest.Kind)
	assert.Equal(t, "test-agent", manifest.Metadata.Name)
	assert.NotNil(t, manifest.Resource)

	spec, ok := manifest.Resource.(*apiary.AgentSpec)
	require.True(t, ok)
	assert.Equal(t, []string{"echo", "hello"}, spec.Spec.Runtime.Command)
	assert.Equal(t, "stdin", spec.Spec.Interface.Type)
}

func TestParseManifest_JSON(t *testing.T) {
	jsonData := `{
  "apiVersion": "apiary.io/v1",
  "kind": "AgentSpec",
  "metadata": {
    "name": "test-agent",
    "namespace": "default"
  },
  "spec": {
    "runtime": {
      "command": ["echo", "hello"]
    },
    "interface": {
      "type": "stdin"
    }
  }
}`

	manifest, err := ParseManifest([]byte(jsonData))
	require.NoError(t, err)
	assert.NotNil(t, manifest)
	assert.Equal(t, "AgentSpec", manifest.Kind)
}

func TestParseManifest_Invalid(t *testing.T) {
	invalidData := `this is not valid yaml or json`

	_, err := ParseManifest([]byte(invalidData))
	assert.Error(t, err)
}

func TestValidate_ValidAgentSpec(t *testing.T) {
	yamlData := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: test-agent
  namespace: default
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: stdin
`

	manifest, err := ParseManifest([]byte(yamlData))
	require.NoError(t, err)

	errors, err := Validate(manifest)
	require.NoError(t, err)
	assert.Empty(t, errors)
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	yamlData := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  namespace: default
spec:
  runtime:
    command: []
  interface:
    type: stdin
`

	manifest, err := ParseManifest([]byte(yamlData))
	require.NoError(t, err)

	errors, err := Validate(manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, errors)

	// Check for expected errors
	errorMap := make(map[string]string)
	for _, e := range errors {
		errorMap[e.Field] = e.Message
	}

	assert.Contains(t, errorMap, "metadata.name")
	assert.Contains(t, errorMap, "spec.runtime.command")
}

func TestValidate_InvalidAPIVersion(t *testing.T) {
	yamlData := `
apiVersion: v1
kind: AgentSpec
metadata:
  name: test-agent
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: stdin
`

	manifest, err := ParseManifest([]byte(yamlData))
	require.NoError(t, err)

	errors, err := Validate(manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, errors)

	errorMap := make(map[string]string)
	for _, e := range errors {
		errorMap[e.Field] = e.Message
	}

	assert.Contains(t, errorMap, "apiVersion")
}

func TestValidate_InvalidInterfaceType(t *testing.T) {
	yamlData := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: test-agent
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: invalid
`

	manifest, err := ParseManifest([]byte(yamlData))
	require.NoError(t, err)

	errors, err := Validate(manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, errors)

	errorMap := make(map[string]string)
	for _, e := range errors {
		errorMap[e.Field] = e.Message
	}

	assert.Contains(t, errorMap, "spec.interface.type")
}

func TestValidate_Hive(t *testing.T) {
	yamlData := `
apiVersion: apiary.io/v1
kind: Hive
metadata:
  name: test-hive
  namespace: default
spec:
  pattern: pipeline
  stages:
    - name: stage1
      agentRef: agent1
      replicas: 1
`

	manifest, err := ParseManifest([]byte(yamlData))
	require.NoError(t, err)

	errors, err := Validate(manifest)
	require.NoError(t, err)
	assert.Empty(t, errors)
}

func TestValidate_Hive_InvalidPattern(t *testing.T) {
	yamlData := `
apiVersion: apiary.io/v1
kind: Hive
metadata:
  name: test-hive
  namespace: default
spec:
  pattern: invalid-pattern
`

	manifest, err := ParseManifest([]byte(yamlData))
	require.NoError(t, err)

	errors, err := Validate(manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, errors)
}

func TestSerialize_YAML(t *testing.T) {
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			Labels:    apiary.Labels{"app": "test"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	manifest := &Manifest{
		Resource: spec,
	}

	data, err := manifest.Serialize("yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "kind: AgentSpec")
	assert.Contains(t, string(data), "name: test-agent")
}

func TestSerialize_JSON(t *testing.T) {
	spec := &apiary.AgentSpec{
		TypeMeta: apiary.TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: apiary.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: apiary.AgentSpecSpec{
			Runtime: apiary.RuntimeConfig{
				Command: []string{"echo", "hello"},
			},
			Interface: apiary.InterfaceConfig{
				Type: "stdin",
			},
		},
	}

	manifest := &Manifest{
		Resource: spec,
	}

	data, err := manifest.Serialize("json")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	// JSON is pretty-printed, so check for kind field
	assert.Contains(t, string(data), `"kind"`)
	assert.Contains(t, string(data), `AgentSpec`)
}

func TestParseAndValidate(t *testing.T) {
	yamlData := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: test-agent
  namespace: default
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: stdin
`

	manifest, errors, err := ParseAndValidate([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, manifest)
	assert.Empty(t, errors)
}

func TestRoundTrip(t *testing.T) {
	originalYAML := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: test-agent
  namespace: default
  labels:
    app: test
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: stdin
`

	// Parse
	manifest1, err := ParseManifest([]byte(originalYAML))
	require.NoError(t, err)

	// Serialize to YAML
	yamlData, err := manifest1.Serialize("yaml")
	require.NoError(t, err)

	// Parse again
	manifest2, err := ParseManifest(yamlData)
	require.NoError(t, err)

	// Compare
	spec1 := manifest1.Resource.(*apiary.AgentSpec)
	spec2 := manifest2.Resource.(*apiary.AgentSpec)

	assert.Equal(t, spec1.GetName(), spec2.GetName())
	assert.Equal(t, spec1.GetNamespace(), spec2.GetNamespace())
	assert.Equal(t, spec1.Spec.Runtime.Command, spec2.Spec.Runtime.Command)
}

func BenchmarkParseManifest(b *testing.B) {
	yamlData := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: test-agent
  namespace: default
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: stdin
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseManifest([]byte(yamlData))
	}
}

func BenchmarkValidate(b *testing.B) {
	yamlData := `
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: test-agent
  namespace: default
spec:
  runtime:
    command: ["echo", "hello"]
  interface:
    type: stdin
`

	manifest, err := ParseManifest([]byte(yamlData))
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Validate(manifest)
	}
}
