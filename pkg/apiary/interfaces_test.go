package apiary

import (
	"testing"
	"time"
)

// TestLabels_Match tests the Labels.Match method.
func TestLabels_Match(t *testing.T) {
	tests := []struct {
		name     string
		labels   Labels
		selector Labels
		want     bool
	}{
		{
			name:     "empty selector matches all",
			labels:   Labels{"app": "test"},
			selector: Labels{},
			want:     true,
		},
		{
			name:     "exact match",
			labels:   Labels{"app": "test", "env": "prod"},
			selector: Labels{"app": "test"},
			want:     true,
		},
		{
			name:     "multiple selector matches",
			labels:   Labels{"app": "test", "env": "prod"},
			selector: Labels{"app": "test", "env": "prod"},
			want:     true,
		},
		{
			name:     "missing key",
			labels:   Labels{"app": "test"},
			selector: Labels{"env": "prod"},
			want:     false,
		},
		{
			name:     "value mismatch",
			labels:   Labels{"app": "test"},
			selector: Labels{"app": "prod"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.labels.Match(tt.selector); got != tt.want {
				t.Errorf("Labels.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestResource_AgentSpec tests AgentSpec implements Resource interface.
func TestResource_AgentSpec(t *testing.T) {
	now := time.Now()
	spec := &AgentSpec{
		TypeMeta: TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "AgentSpec",
		},
		ObjectMeta: ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			UID:       "test-uid",
			Labels:    Labels{"app": "test"},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	// Verify Resource interface implementation
	var _ Resource = spec

	if spec.GetKind() != "AgentSpec" {
		t.Errorf("GetKind() = %v, want AgentSpec", spec.GetKind())
	}
	if spec.GetName() != "test-agent" {
		t.Errorf("GetName() = %v, want test-agent", spec.GetName())
	}
	if spec.GetNamespace() != "default" {
		t.Errorf("GetNamespace() = %v, want default", spec.GetNamespace())
	}
	if spec.GetUID() != "test-uid" {
		t.Errorf("GetUID() = %v, want test-uid", spec.GetUID())
	}
}

// TestResource_Drone tests Drone implements Resource interface.
func TestResource_Drone(t *testing.T) {
	now := time.Now()
	drone := &Drone{
		TypeMeta: TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Drone",
		},
		ObjectMeta: ObjectMeta{
			Name:      "test-drone",
			Namespace: "default",
			UID:       "drone-uid",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	var _ Resource = drone

	if drone.GetKind() != "Drone" {
		t.Errorf("GetKind() = %v, want Drone", drone.GetKind())
	}
}

// TestResource_Hive tests Hive implements Resource interface.
func TestResource_Hive(t *testing.T) {
	now := time.Now()
	hive := &Hive{
		TypeMeta: TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Hive",
		},
		ObjectMeta: ObjectMeta{
			Name:      "test-hive",
			Namespace: "default",
			UID:       "hive-uid",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	var _ Resource = hive

	if hive.GetKind() != "Hive" {
		t.Errorf("GetKind() = %v, want Hive", hive.GetKind())
	}
}

// TestResource_Cell tests Cell implements Resource interface.
func TestResource_Cell(t *testing.T) {
	now := time.Now()
	cell := &Cell{
		TypeMeta: TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Cell",
		},
		ObjectMeta: ObjectMeta{
			Name:      "test-cell",
			UID:       "cell-uid",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	var _ Resource = cell

	if cell.GetKind() != "Cell" {
		t.Errorf("GetKind() = %v, want Cell", cell.GetKind())
	}
	if cell.GetNamespace() != "" {
		t.Errorf("GetNamespace() = %v, want empty string", cell.GetNamespace())
	}
}

// TestResource_Session tests Session implements Resource interface.
func TestResource_Session(t *testing.T) {
	now := time.Now()
	session := &Session{
		TypeMeta: TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Session",
		},
		ObjectMeta: ObjectMeta{
			Name:      "test-session",
			Namespace: "default",
			UID:       "session-uid",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	var _ Resource = session

	if session.GetKind() != "Session" {
		t.Errorf("GetKind() = %v, want Session", session.GetKind())
	}
}

// TestResource_Secret tests Secret implements Resource interface.
func TestResource_Secret(t *testing.T) {
	now := time.Now()
	secret := &Secret{
		TypeMeta: TypeMeta{
			APIVersion: "apiary.io/v1",
			Kind:       "Secret",
		},
		ObjectMeta: ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
			UID:       "secret-uid",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	var _ Resource = secret

	if secret.GetKind() != "Secret" {
		t.Errorf("GetKind() = %v, want Secret", secret.GetKind())
	}
}

// BenchmarkLabels_Match benchmarks the Labels.Match method.
func BenchmarkLabels_Match(b *testing.B) {
	labels := Labels{
		"app":     "test",
		"env":     "prod",
		"version": "1.0",
		"tier":    "backend",
	}
	selector := Labels{
		"app": "test",
		"env": "prod",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = labels.Match(selector)
	}
}
