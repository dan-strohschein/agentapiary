package apiary

import (
	"errors"
	"time"
)

// Common errors
var (
	ErrNotFound      = errors.New("resource not found")
	ErrAlreadyExists = errors.New("resource already exists")
	ErrInvalidInput  = errors.New("invalid input")
)

// AgentSpec defines the specification for an agent.
type AgentSpec struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Spec       AgentSpecSpec `json:"spec"`
}

// GetKind implements Resource.
func (a *AgentSpec) GetKind() string { return "AgentSpec" }

// GetName implements Resource.
func (a *AgentSpec) GetName() string { return a.ObjectMeta.Name }

// GetNamespace implements Resource.
func (a *AgentSpec) GetNamespace() string { return a.ObjectMeta.Namespace }

// GetUID implements Resource.
func (a *AgentSpec) GetUID() string { return a.ObjectMeta.UID }

// GetLabels implements Resource.
func (a *AgentSpec) GetLabels() Labels { return a.ObjectMeta.Labels }

// GetAnnotations implements Resource.
func (a *AgentSpec) GetAnnotations() map[string]string { return a.ObjectMeta.Annotations }

// GetCreatedAt implements Resource.
func (a *AgentSpec) GetCreatedAt() time.Time { return a.ObjectMeta.CreatedAt }

// GetUpdatedAt implements Resource.
func (a *AgentSpec) GetUpdatedAt() time.Time { return a.ObjectMeta.UpdatedAt }

// AgentSpecSpec contains the agent specification details.
type AgentSpecSpec struct {
	Runtime    RuntimeConfig    `json:"runtime"`
	Interface  InterfaceConfig  `json:"interface"`
	Resources  ResourceConfig   `json:"resources"`
	Scaling    ScalingConfig    `json:"scaling,omitempty"`
	TaskTier   string           `json:"taskTier,omitempty"`
	Guardrails GuardrailConfig  `json:"guardrails,omitempty"`
}

// RuntimeConfig specifies how to run the agent.
type RuntimeConfig struct {
	Command     []string          `json:"command"`
	WorkingDir  string            `json:"workingDir,omitempty"`
	Env         []EnvVar          `json:"env,omitempty"`
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Name      string              `json:"name"`
	Value     string              `json:"value,omitempty"`
	ValueFrom *EnvVarSource       `json:"valueFrom,omitempty"`
}

// EnvVarSource represents a source for an environment variable.
type EnvVarSource struct {
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// SecretKeySelector selects a key from a secret.
type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// InterfaceConfig specifies how the Keeper communicates with the agent.
type InterfaceConfig struct {
	Type       string `json:"type"` // stdin, http, grpc, unix-socket
	Port       int    `json:"port,omitempty"`
	HealthPath string `json:"healthPath,omitempty"`
	MessagePath string `json:"messagePath,omitempty"`
}

// ResourceConfig specifies resource requirements and limits.
type ResourceConfig struct {
	Requests             ResourceRequirements `json:"requests"`
	Limits               ResourceRequirements `json:"limits"`
	TokenBudgetPerRequest int64               `json:"tokenBudgetPerRequest,omitempty"`
}

// ScalingConfig defines autoscaling behavior.
type ScalingConfig struct {
	MinReplicas        int     `json:"minReplicas"`
	MaxReplicas        int     `json:"maxReplicas"`
	TargetLatencyMs    int     `json:"targetLatencyMs,omitempty"`
	ScaleUpThreshold   float64 `json:"scaleUpThreshold,omitempty"`
	ScaleDownThreshold float64 `json:"scaleDownThreshold,omitempty"`
	CooldownSeconds    int     `json:"cooldownSeconds,omitempty"`
}

// GuardrailConfig defines safety guardrails.
type GuardrailConfig struct {
	ResponseTimeoutSeconds int                    `json:"responseTimeoutSeconds,omitempty"`
	RateLimitPerMinute     int                    `json:"rateLimitPerMinute,omitempty"`
	OutputSchema           map[string]interface{} `json:"outputSchema,omitempty"`
	TokenBudgetPerRequest  int64                  `json:"tokenBudgetPerRequest,omitempty"`
	TokenBudgetPerSession  int64                  `json:"tokenBudgetPerSession,omitempty"`
	TokenBudgetPerMinute   int64                  `json:"tokenBudgetPerMinute,omitempty"`
}

// Drone represents a running instance of an agent.
type Drone struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Status     DroneStatus `json:"status"`
	Spec       *AgentSpec  `json:"spec"`
}

// GetKind implements Resource.
func (d *Drone) GetKind() string { return "Drone" }

// GetName implements Resource.
func (d *Drone) GetName() string { return d.ObjectMeta.Name }

// GetNamespace implements Resource.
func (d *Drone) GetNamespace() string { return d.ObjectMeta.Namespace }

// GetUID implements Resource.
func (d *Drone) GetUID() string { return d.ObjectMeta.UID }

// GetLabels implements Resource.
func (d *Drone) GetLabels() Labels { return d.ObjectMeta.Labels }

// GetAnnotations implements Resource.
func (d *Drone) GetAnnotations() map[string]string { return d.ObjectMeta.Annotations }

// GetCreatedAt implements Resource.
func (d *Drone) GetCreatedAt() time.Time { return d.ObjectMeta.CreatedAt }

// GetUpdatedAt implements Resource.
func (d *Drone) GetUpdatedAt() time.Time { return d.ObjectMeta.UpdatedAt }

// DroneStatus represents the current status of a Drone.
type DroneStatus struct {
	Phase      DronePhase `json:"phase"`
	Message    string     `json:"message,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	KeeperAddr string     `json:"keeperAddr,omitempty"`
	Health     *HealthStatus `json:"health,omitempty"`
}

// DronePhase indicates the phase of a Drone's lifecycle.
type DronePhase string

const (
	DronePhasePending   DronePhase = "pending"
	DronePhaseStarting DronePhase = "starting"
	DronePhaseRunning  DronePhase = "running"
	DronePhaseStopping DronePhase = "stopping"
	DronePhaseStopped  DronePhase = "stopped"
	DronePhaseFailed   DronePhase = "failed"
)

// Hive represents a collection of agents working together.
type Hive struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Spec       HiveSpec   `json:"spec"`
	Status     HiveStatus `json:"status,omitempty"`
}

// GetKind implements Resource.
func (h *Hive) GetKind() string { return "Hive" }

// GetName implements Resource.
func (h *Hive) GetName() string { return h.ObjectMeta.Name }

// GetNamespace implements Resource.
func (h *Hive) GetNamespace() string { return h.ObjectMeta.Namespace }

// GetUID implements Resource.
func (h *Hive) GetUID() string { return h.ObjectMeta.UID }

// GetLabels implements Resource.
func (h *Hive) GetLabels() Labels { return h.ObjectMeta.Labels }

// GetAnnotations implements Resource.
func (h *Hive) GetAnnotations() map[string]string { return h.ObjectMeta.Annotations }

// GetCreatedAt implements Resource.
func (h *Hive) GetCreatedAt() time.Time { return h.ObjectMeta.CreatedAt }

// GetUpdatedAt implements Resource.
func (h *Hive) GetUpdatedAt() time.Time { return h.ObjectMeta.UpdatedAt }

// HiveSpec defines the Hive specification.
type HiveSpec struct {
	PoolMode  string         `json:"poolMode,omitempty"` // cold, warm, hybrid
	WarmPoolSize int         `json:"warmPoolSize,omitempty"`
	Session   SessionConfig  `json:"session,omitempty"`
	Pattern   string         `json:"pattern,omitempty"` // pipeline, hierarchical, swarm, event-driven (deprecated: use PatternSegments)
	Stages    []Stage        `json:"stages,omitempty"`
	Routing   RoutingConfig  `json:"routing,omitempty"`
	Guardrails GuardrailConfig `json:"guardrails,omitempty"`
	Webhook   WebhookConfig  `json:"webhook,omitempty"` // Webhook configuration for guardrail violations
	// PatternSegments defines a sequence of patterns to compose together.
	// If specified, patterns are executed in order, with each pattern's output
	// routed to the next pattern's input. If empty, falls back to Pattern field.
	PatternSegments []PatternSegment `json:"patternSegments,omitempty"`
}

// PatternSegment represents a single pattern in a composed workflow.
type PatternSegment struct {
	// Name is a unique identifier for this pattern segment.
	Name string `json:"name"`
	// Pattern is the pattern type: "pipeline", "hierarchical", "swarm", "event-driven"
	Pattern string `json:"pattern"`
	// Config contains pattern-specific configuration.
	// For pipeline: stages
	// For hierarchical: supervisor and worker IDs
	// For swarm: agent IDs
	// For event-driven: event source and subscribers
	Config map[string]interface{} `json:"config,omitempty"`
	// InputTopic is the NATS topic this segment subscribes to for input.
	// If empty, uses the previous segment's output topic or the Hive's default input.
	InputTopic string `json:"inputTopic,omitempty"`
	// OutputTopic is the NATS topic this segment publishes output to.
	// If empty, uses a default topic based on the segment name.
	OutputTopic string `json:"outputTopic,omitempty"`
}

// Stage represents a stage in a pipeline pattern.
type Stage struct {
	Name      string `json:"name"`
	AgentRef  string `json:"agentRef"`
	Replicas  int    `json:"replicas"`
	Parallel  bool   `json:"parallel,omitempty"`
	WaitForAll bool  `json:"waitForAll,omitempty"`
}

// RoutingConfig defines message routing configuration.
type RoutingConfig struct {
	DeadLetterTopic string      `json:"deadLetterTopic,omitempty"`
	RetryPolicy     RetryPolicy `json:"retryPolicy,omitempty"`
}

// RetryPolicy defines retry behavior.
type RetryPolicy struct {
	MaxRetries       int `json:"maxRetries"`
	BackoffMultiplier int `json:"backoffMultiplier"`
	InitialDelayMs   int `json:"initialDelayMs"`
}

// WebhookConfig defines webhook configuration for guardrail violations.
type WebhookConfig struct {
	URL         string `json:"url,omitempty"`         // Webhook URL to send notifications to
	AuthHeader  string `json:"authHeader,omitempty"`  // Optional Authorization header (e.g., "Bearer token" or "Basic base64")
}

// SessionConfig defines session configuration.
type SessionConfig struct {
	TimeoutMinutes      int  `json:"timeoutMinutes,omitempty"`
	MaxDurationMinutes  int  `json:"maxDurationMinutes,omitempty"`
	PersistOnTerminate  bool `json:"persistOnTerminate,omitempty"`
	PersistPath         string `json:"persistPath,omitempty"`
	MaxMemoryMB         int  `json:"maxMemoryMB,omitempty"`
}

// HiveStatus represents the current status of a Hive.
type HiveStatus struct {
	Phase      HivePhase `json:"phase"`
	Message    string    `json:"message,omitempty"`
	ActiveDrones int    `json:"activeDrones,omitempty"`
}

// HivePhase indicates the phase of a Hive's lifecycle.
type HivePhase string

const (
	HivePhasePending HivePhase = "pending"
	HivePhaseActive  HivePhase = "active"
	HivePhasePaused  HivePhase = "paused"
	HivePhaseFailed  HivePhase = "failed"
)

// Cell represents a namespace for resource isolation.
type Cell struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Spec       CellSpec `json:"spec"`
}

// GetKind implements Resource.
func (c *Cell) GetKind() string { return "Cell" }

// GetName implements Resource.
func (c *Cell) GetName() string { return c.ObjectMeta.Name }

// GetNamespace implements Resource.
func (c *Cell) GetNamespace() string { return "" } // Cells are not namespaced

// GetUID implements Resource.
func (c *Cell) GetUID() string { return c.ObjectMeta.UID }

// GetLabels implements Resource.
func (c *Cell) GetLabels() Labels { return c.ObjectMeta.Labels }

// GetAnnotations implements Resource.
func (c *Cell) GetAnnotations() map[string]string { return c.ObjectMeta.Annotations }

// GetCreatedAt implements Resource.
func (c *Cell) GetCreatedAt() time.Time { return c.ObjectMeta.CreatedAt }

// GetUpdatedAt implements Resource.
func (c *Cell) GetUpdatedAt() time.Time { return c.ObjectMeta.UpdatedAt }

// CellSpec defines the Cell specification.
type CellSpec struct {
	ResourceQuota ResourceQuota `json:"resourceQuota,omitempty"`
	Defaults      DefaultConfig  `json:"defaults,omitempty"`
}

// ResourceQuota defines resource limits for a Cell.
type ResourceQuota struct {
	MaxHives        int `json:"maxHives,omitempty"`
	MaxDronesPerHive int `json:"maxDronesPerHive,omitempty"`
	MaxTotalDrones  int `json:"maxTotalDrones,omitempty"`
	MaxMemoryGB     int `json:"maxMemoryGB,omitempty"`
}

// QuotaUsage represents current quota usage for a Cell.
type QuotaUsage struct {
	Hives         int   `json:"hives"`
	TotalDrones   int   `json:"totalDrones"`
	MemoryUsedGB  int64 `json:"memoryUsedGB"`
	MemoryLimitGB int   `json:"memoryLimitGB"`
}

// DefaultConfig defines default values for resources in a Cell.
type DefaultConfig struct {
	Guardrails GuardrailConfig `json:"guardrails,omitempty"`
}

// Session represents an execution context with shared memory.
type Session struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Status     SessionStatus `json:"status"`
	Spec       SessionConfig `json:"spec"`
}

// GetKind implements Resource.
func (s *Session) GetKind() string { return "Session" }

// GetName implements Resource.
func (s *Session) GetName() string { return s.ObjectMeta.Name }

// GetNamespace implements Resource.
func (s *Session) GetNamespace() string { return s.ObjectMeta.Namespace }

// GetUID implements Resource.
func (s *Session) GetUID() string { return s.ObjectMeta.UID }

// GetLabels implements Resource.
func (s *Session) GetLabels() Labels { return s.ObjectMeta.Labels }

// GetAnnotations implements Resource.
func (s *Session) GetAnnotations() map[string]string { return s.ObjectMeta.Annotations }

// GetCreatedAt implements Resource.
func (s *Session) GetCreatedAt() time.Time { return s.ObjectMeta.CreatedAt }

// GetUpdatedAt implements Resource.
func (s *Session) GetUpdatedAt() time.Time { return s.ObjectMeta.UpdatedAt }

// SessionStatus represents the current status of a Session.
type SessionStatus struct {
	Phase         SessionPhase `json:"phase"`
	LastActivity  time.Time    `json:"lastActivity"`
	ActiveAgents  int          `json:"activeAgents,omitempty"`
}

// SessionPhase indicates the phase of a Session's lifecycle.
type SessionPhase string

const (
	SessionPhaseCreated    SessionPhase = "created"
	SessionPhaseActive     SessionPhase = "active"
	SessionPhaseIdle       SessionPhase = "idle"
	SessionPhaseTerminated SessionPhase = "terminated"
)

// Secret represents a secret containing sensitive data.
type Secret struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata"`
	Data       map[string][]byte `json:"data"`
}

// GetKind implements Resource.
func (s *Secret) GetKind() string { return "Secret" }

// GetName implements Resource.
func (s *Secret) GetName() string { return s.ObjectMeta.Name }

// GetNamespace implements Resource.
func (s *Secret) GetNamespace() string { return s.ObjectMeta.Namespace }

// GetUID implements Resource.
func (s *Secret) GetUID() string { return s.ObjectMeta.UID }

// GetLabels implements Resource.
func (s *Secret) GetLabels() Labels { return s.ObjectMeta.Labels }

// GetAnnotations implements Resource.
func (s *Secret) GetAnnotations() map[string]string { return s.ObjectMeta.Annotations }

// GetCreatedAt implements Resource.
func (s *Secret) GetCreatedAt() time.Time { return s.ObjectMeta.CreatedAt }

// GetUpdatedAt implements Resource.
func (s *Secret) GetUpdatedAt() time.Time { return s.ObjectMeta.UpdatedAt }

// TypeMeta contains type information for resources.
type TypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// ObjectMeta contains metadata common to all resources.
type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	UID         string            `json:"uid,omitempty"`
	Labels      Labels            `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
	UpdatedAt   time.Time         `json:"updatedAt,omitempty"`
}
