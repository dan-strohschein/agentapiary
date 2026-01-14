// Package apiary provides core interfaces and types for the Apiary orchestration platform.
package apiary

import (
	"context"
	"time"
)

// Resource is the base interface for all Apiary resources.
// All resources must implement this interface to be stored and managed.
type Resource interface {
	// GetKind returns the resource kind (e.g., "AgentSpec", "Hive", "Cell").
	GetKind() string
	
	// GetName returns the resource name.
	GetName() string
	
	// GetNamespace returns the resource namespace (Cell name).
	GetNamespace() string
	
	// GetUID returns a unique identifier for this resource instance.
	GetUID() string
	
	// GetLabels returns the resource labels.
	GetLabels() Labels
	
	// GetAnnotations returns the resource annotations.
	GetAnnotations() map[string]string
	
	// GetCreatedAt returns when the resource was created.
	GetCreatedAt() time.Time
	
	// GetUpdatedAt returns when the resource was last updated.
	GetUpdatedAt() time.Time
}

// ResourceStore provides persistent storage for Apiary resources.
// Implementations must be thread-safe and support concurrent access.
type ResourceStore interface {
	// Create stores a new resource. Returns an error if the resource already exists.
	Create(ctx context.Context, resource Resource) error
	
	// Get retrieves a resource by kind, name, and namespace.
	// Returns ErrNotFound if the resource does not exist.
	Get(ctx context.Context, kind, name, namespace string) (Resource, error)
	
	// Update updates an existing resource. Returns ErrNotFound if the resource does not exist.
	Update(ctx context.Context, resource Resource) error
	
	// Delete removes a resource. Returns ErrNotFound if the resource does not exist.
	Delete(ctx context.Context, kind, name, namespace string) error
	
	// List returns all resources of the given kind in the namespace.
	// If namespace is empty, returns resources across all namespaces.
	// Selector filters resources by labels.
	List(ctx context.Context, kind, namespace string, selector Labels) ([]Resource, error)
	
	// Watch returns a channel that emits events when resources change.
	// The channel is closed when the context is cancelled.
	Watch(ctx context.Context, kind, namespace string) (<-chan ResourceEvent, error)
	
	// Close closes the store and releases all resources.
	Close() error
}

// ResourceEvent represents a change to a resource.
type ResourceEvent struct {
	Type     EventType
	Resource Resource
}

// EventType indicates the type of resource change.
type EventType string

const (
	EventTypeCreated EventType = "created"
	EventTypeUpdated EventType = "updated"
	EventTypeDeleted EventType = "deleted"
)

// Orchestrator is the interface for the Queen orchestrator.
// It reconciles desired state (from manifests) with actual state (running Drones).
type Orchestrator interface {
	// Start starts the orchestrator and begins reconciliation.
	Start(ctx context.Context) error
	
	// Stop gracefully stops the orchestrator.
	Stop(ctx context.Context) error
	
	// Reconcile triggers immediate reconciliation of all resources.
	Reconcile(ctx context.Context) error
	
	// GetDroneStatus returns the current status of a Drone.
	GetDroneStatus(ctx context.Context, droneID string) (*DroneStatus, error)
	
	// ListDrones returns all Drones matching the selector.
	ListDrones(ctx context.Context, namespace string, selector Labels) ([]*Drone, error)
}

// AgentLauncher is responsible for launching and managing agent processes.
type AgentLauncher interface {
	// Launch starts an agent process based on the AgentSpec.
	// Returns the process ID and any error.
	Launch(ctx context.Context, spec *AgentSpec) (*AgentProcess, error)
	
	// Stop gracefully stops an agent process.
	Stop(ctx context.Context, processID string) error
	
	// GetProcess returns information about a running process.
	GetProcess(ctx context.Context, processID string) (*AgentProcess, error)
}

// AgentProcess represents a running agent process.
type AgentProcess struct {
	ID        string
	PID       int
	Status    ProcessStatus
	StartedAt time.Time
	Spec      *AgentSpec
}

// ProcessStatus indicates the status of an agent process.
type ProcessStatus string

const (
	ProcessStatusStarting ProcessStatus = "starting"
	ProcessStatusRunning  ProcessStatus = "running"
	ProcessStatusStopping ProcessStatus = "stopping"
	ProcessStatusStopped   ProcessStatus = "stopped"
	ProcessStatusFailed    ProcessStatus = "failed"
)

// HealthChecker checks the health of a component.
type HealthChecker interface {
	// CheckHealth performs a health check and returns the status.
	CheckHealth(ctx context.Context) (*HealthStatus, error)
}

// HealthStatus represents the health of a component.
type HealthStatus struct {
	Healthy   bool
	Message   string
	Timestamp time.Time
	Details   map[string]interface{}
}

// Scheduler decides where and when to launch Drones.
type Scheduler interface {
	// Schedule determines where to schedule a Drone based on the AgentSpec.
	// Returns a scheduling decision or an error if scheduling is not possible.
	Schedule(ctx context.Context, spec *AgentSpec) (*ScheduleDecision, error)
	
	// GetResourceAvailability returns available resources for scheduling.
	GetResourceAvailability(ctx context.Context) (*ResourceAvailability, error)
}

// ScheduleDecision represents a scheduling decision.
type ScheduleDecision struct {
	NodeID    string
	Resources *ResourceRequirements
	Reason    string
}

// ResourceAvailability indicates available resources for scheduling.
type ResourceAvailability struct {
	AvailableCPU    int64 // in millicores
	AvailableMemory int64 // in bytes
	TotalCPU        int64
	TotalMemory     int64
}

// ResourceRequirements specifies resource requirements for a Drone.
type ResourceRequirements struct {
	CPU    int64 // in millicores
	Memory int64 // in bytes
}

// Labels is a map of key-value pairs used for resource selection and organization.
type Labels map[string]string

// Match returns true if the labels match the selector.
func (l Labels) Match(selector Labels) bool {
	if len(selector) == 0 {
		return true
	}
	
	for k, v := range selector {
		if labelValue, ok := l[k]; !ok || labelValue != v {
			return false
		}
	}
	
	return true
}
