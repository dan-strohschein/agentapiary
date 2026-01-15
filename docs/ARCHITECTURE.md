# Apiary Architecture

## Overview

Apiary is an orchestration platform for AI agents (Hives) that provides declarative configuration, multi-tenancy, resource management, and advanced orchestration patterns. It follows a modular, decoupled architecture adhering to SOLID and DRY principles.

## Core Components

### 1. Queen (Central Orchestrator)

The Queen is the central orchestrator that reconciles desired state (from manifests) with actual state (running agents).

**Responsibilities:**
- Watches for resource changes (AgentSpecs, Hives)
- Manages agent lifecycle (start, stop, scale)
- Coordinates with Scheduler for resource allocation
- Manages warm pools for low-latency agent startup
- Handles autoscaling decisions
- Manages circuit breakers

**Location:** `internal/queen/`

**Key Interfaces:**
- Watches AgentSpecs and Hives from the store
- Creates and manages Drones (agent instances)
- Coordinates with Scheduler for resource allocation

### 2. API Server

The API server provides both REST and gRPC APIs for managing Apiary resources.

**Responsibilities:**
- REST API (Echo framework) on port 8080
- gRPC API (mirroring REST endpoints)
- Authentication and authorization (RBAC)
- Request validation
- Health checks (`/healthz`, `/ready`)

**Location:** `internal/api/`

**Endpoints:**
- Cells: `/api/v1/cells`
- AgentSpecs: `/api/v1/cells/:namespace/agentspecs`
- Hives: `/api/v1/cells/:namespace/hives`
- Secrets: `/api/v1/cells/:namespace/secrets`
- Drones: `/api/v1/cells/:namespace/drones`
- Sessions: `/api/v1/cells/:namespace/sessions`
- Metrics: `/api/v1/metrics`
- DLQ: `/api/v1/cells/:namespace/hives/:hive/dlq`

### 3. Scheduler

The Scheduler allocates resources (CPU, memory) to agents based on availability and task tier hints.

**Responsibilities:**
- Tracks available CPU and memory
- Schedules Drones based on resource requirements
- Supports task tier scheduling (simple, standard, complex)
- Manages resource quotas per Cell

**Location:** `internal/scheduler/`

**Features:**
- Resource-aware scheduling
- Task tier hints for cost optimization
- Quota enforcement

### 4. Launcher

The Launcher manages agent process lifecycle.

**Responsibilities:**
- Starts agent processes
- Stops agent processes gracefully
- Manages process health monitoring
- Handles environment variable injection (including secrets)

**Location:** `internal/launcher/`

**Process Management:**
- Supports stdin/stdout communication
- HTTP interface support
- Unix socket support
- Health check monitoring

### 5. Nectar (Message Bus)

Nectar provides the message bus for agent-to-agent communication using NATS.

**Responsibilities:**
- Publishes messages to topics
- Subscribes to topics (with handlers)
- Queue subscriptions for load balancing
- Supports both embedded and external NATS

**Location:** `internal/nectar/`

**Features:**
- Topic-based pub/sub
- Queue groups for load balancing
- JetStream support for persistence
- Cell/namespace isolation via topic prefixing

### 6. Comb (Shared Memory)

Comb provides session-scoped shared memory for agents.

**Responsibilities:**
- Key-value storage per session
- TTL support for expiration
- Memory limit enforcement
- Structured access patterns (conversations, artifacts, state)

**Location:** `internal/comb/`

**Features:**
- Session-scoped storage
- TTL-based expiration
- Memory limits per session
- Structured access patterns (conversations, artifacts, state machines)

### 7. Keeper (Agent Sidecar)

The Keeper is a sidecar process that runs alongside each agent, providing infrastructure services.

**Responsibilities:**
- Relays messages between agents and the message bus
- Manages Comb cache (local LRU cache)
- Handles message buffering and retry logic
- Provides logging and execution capabilities

**Location:** `internal/keeper/`

**Features:**
- Message relay with exponential backoff
- Local Comb cache for performance
- Log streaming
- Exec capabilities

### 8. Store (BadgerDB)

The Store provides persistent storage for Apiary resources using BadgerDB.

**Responsibilities:**
- CRUD operations for resources
- Resource versioning
- Watch/event notifications
- Resource isolation per Cell

**Location:** `internal/store/badger/`

**Features:**
- Embedded BadgerDB (default)
- Resource versioning
- Watch support for change notifications
- Automatic UID generation

### 9. Session Manager

The Session Manager manages session lifecycle.

**Responsibilities:**
- Creates and manages sessions
- Tracks session activity
- Enforces session timeouts
- Manages session persistence

**Location:** `internal/session/`

**Features:**
- Timeout management
- Activity tracking
- Persistence support
- Max duration limits

### 10. Autoscaler

The Autoscaler automatically scales agents based on metrics.

**Responsibilities:**
- Evaluates scaling metrics (latency, queue depth)
- Makes scaling decisions
- Executes scale operations

**Location:** `internal/autoscaler/`

**Features:**
- Latency-based scaling
- Queue depth scaling
- Configurable thresholds
- Cooldown periods

### 11. Warm Pool Manager

The Warm Pool Manager maintains pools of pre-warmed agents for low-latency startup.

**Responsibilities:**
- Maintains warm pools per AgentSpec
- Scales pools based on demand
- Cleans up idle pools

**Location:** `internal/warmpool/`

**Features:**
- Configurable pool sizes
- Idle timeout cleanup
- Automatic scaling

### 12. Guardrails

Guardrails enforce safety and limits on agent operations.

**Components:**
- **Token Budget**: Limits token usage per request/session/minute
- **Response Timeout**: Enforces maximum response time
- **Rate Limiting**: Limits requests per minute
- **Output Validation**: Validates agent output against JSON schema
- **Circuit Breaker**: Prevents cascading failures

**Location:** `internal/guardrails/`

## Data Flow

### Agent Deployment Flow

1. User creates/updates an AgentSpec manifest
2. API Server validates and stores the AgentSpec
3. Queen watches for changes and reconciles
4. Queen requests resources from Scheduler
5. Launcher starts agent process with Keeper sidecar
6. Keeper connects to message bus and Comb
7. Agent is ready to receive messages

### Message Flow

1. Message published to topic (via REST/gRPC API or another agent)
2. Message bus (Nectar) routes to subscribers
3. Keeper receives message and buffers it
4. Keeper sends message to agent (stdin/HTTP)
5. Agent processes message and responds
6. Keeper publishes response to output topic
7. Response flows back to caller or next stage in pipeline

### Session Flow

1. User creates session via API
2. Session Manager creates session record
3. Session-scoped Comb storage initialized
4. Messages routed to session
5. Session timeout monitored
6. Session cleaned up on timeout or deletion

## Multi-Tenancy (Cells)

Cells provide namespace isolation in Apiary:

- **Resource Isolation**: All resources (AgentSpecs, Hives, Secrets) are scoped to a Cell
- **Message Bus Isolation**: Topics are prefixed with Cell name (`cell.{namespace}.{topic}`)
- **Comb Isolation**: Keyspaces are scoped per Cell
- **Quota Enforcement**: Resource quotas are enforced per Cell
- **RBAC**: Permissions are scoped per Cell

## Orchestration Patterns

Apiary supports multiple orchestration patterns:

### 1. Pipeline Pattern
Sequential execution through multiple stages. Each stage's output becomes the next stage's input.

**Use Cases:**
- Data processing pipelines
- Multi-stage transformations
- Workflow orchestration

### 2. Hierarchical Pattern
Supervisor-worker pattern where a supervisor coordinates multiple workers.

**Use Cases:**
- Task distribution
- Parallel processing
- Coordination of specialized agents

### 3. Swarm Pattern
All agents in the swarm receive all messages and process independently.

**Use Cases:**
- Consensus building
- Redundant processing
- Voting mechanisms

### 4. Event-Driven Pattern
Agents subscribe to events and process them asynchronously.

**Use Cases:**
- Event processing
- Asynchronous workflows
- Reactive systems

### 5. Composed Pattern
Multiple patterns can be composed together in sequence.

**Use Cases:**
- Complex workflows
- Multi-stage processing with different patterns

## Design Decisions

### Declarative Configuration
All resources are defined using YAML/JSON manifests. The system continuously reconciles desired state with actual state.

### Embedded by Default
- NATS is embedded by default (can use external)
- BadgerDB is embedded by default (can use external Redis)
- Single-node operation is the default (can be distributed)

### Performance Optimizations
- String operations use `strings.Builder` instead of `fmt.Sprintf`
- Number formatting uses `strconv` instead of `fmt`
- Pre-allocated buffers where possible
- Lock-free structures where appropriate

### SOLID Principles
- **Single Responsibility**: Each component has a single, well-defined purpose
- **Open/Closed**: Components are extensible without modification
- **Liskov Substitution**: Interfaces allow different implementations
- **Interface Segregation**: Small, focused interfaces
- **Dependency Inversion**: Components depend on interfaces, not implementations

## Observability

### Logging
- Structured logging using Zap
- Log levels: Debug, Info, Warn, Error
- Trace correlation (trace ID, span ID)

### Metrics
- Prometheus metrics (currently internal)
- OpenTelemetry metrics
- Request latency, throughput, error rates

### Tracing
- OpenTelemetry distributed tracing
- Trace context propagation across components
- Span attributes for detailed context

### Health Checks
- `/healthz` - Liveness probe
- `/ready` - Readiness probe (checks store connectivity)

## Security

### Authentication
- MVP: Header-based authentication (not for production)
- TODO: JWT validation, OAuth2/OIDC integration

### Authorization (RBAC)
- Roles: `admin`, `developer`, `viewer`
- Permissions: `create`, `read`, `update`, `delete`
- Cell-scoped permissions

### Secrets Management
- Secrets encrypted at rest (AES-256-GCM)
- Secrets injected as environment variables
- Secrets scoped to Cells

## Scalability

### Horizontal Scaling
- Multiple Queen instances with leader election (etcd)
- External NATS cluster for message bus
- External Redis for shared state

### Vertical Scaling
- Resource quotas per Cell
- Autoscaling based on metrics
- Warm pools for low-latency startup

## Reliability

### Circuit Breakers
- Prevent cascading failures
- Automatic recovery
- Configurable thresholds

### Dead Letter Queue (DLQ)
- Messages that cannot be processed are stored in DLQ
- Can be replayed or analyzed
- Per-Hive DLQ

### Retry Logic
- Exponential backoff for transient failures
- Configurable retry limits
- Automatic retry in message relay

### Audit Logging
- All resource changes are logged
- Agent inputs/outputs logged
- Structured audit logs
