# 🐝 Apiary

> **Kubernetes for AI Agents** — Declarative orchestration platform for building, deploying, and managing intelligent agent workflows at scale.

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg?style=flat-square)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen?style=flat-square)]()

**Apiary** is a production-ready orchestration platform that brings the power and simplicity of Kubernetes to the world of AI agents. Just as Kubernetes revolutionized container orchestration, Apiary revolutionizes how teams deploy, scale, and manage collections of AI agents working together toward common goals.

---

## 🚀 Why Apiary?

Building AI agent workflows shouldn't require custom infrastructure, complex message routing, or manual scaling. Apiary provides:

- **🎯 Declarative Configuration** — Define your agent workflows in YAML, just like Kubernetes
- **🔀 Advanced Orchestration Patterns** — Pipeline, Hierarchical, Swarm, Event-Driven, and Composed patterns out of the box
- **📈 Auto-Scaling** — Intelligent scaling based on latency, queue depth, and custom metrics
- **🔒 Multi-Tenancy** — Complete isolation with Cells (namespaces) for teams and environments
- **🛡️ Built-in Guardrails** — Token budgets, rate limits, timeouts, and output validation
- **📊 Observability** — OpenTelemetry tracing, structured logging, and comprehensive metrics
- **⚡ High Performance** — Optimized for low latency and high throughput
- **🔌 Production Ready** — Health checks, circuit breakers, dead letter queues, and graceful degradation

---

## 🏗️ Architecture

Apiary follows a modular, cloud-native architecture:

```
┌─────────────────────────────────────────────────────────────────┐
│                          apiaryctl (CLI)                        │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │   API Server        │
                    │  (REST + gRPC)      │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │      Queen          │
                    │  (Orchestrator)     │
                    └──────────┬──────────┘
                 ┌─────────────┼─────────────┐
                 │             │             │
        ┌────────▼────┐ ┌──────▼────┐ ┌──────▼────┐
        │  Scheduler  │ │  Launcher │ │ Autoscaler│
        └─────────────┘ └───────────┘ └───────────┘
                 │             │             │
        ┌────────┴─────────────┴─────────────┘
        │    Message Bus (Nectar) + Comb (Shared Memory)
        └────────┬────────────────────────────┘
                 │
        ┌────────▼────────┐
        │   Drones +      │
        │   Keepers       │
        └─────────────────┘
```

---

## ⚡ Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/yourorg/apiary.git
cd apiary

# Build all binaries
make build

# Or install directly
go install ./cmd/apiaryctl ./cmd/apiaryd
```

### Start Apiary

```bash
# Start the API server
./bin/apiaryd -data-dir /var/apiary -port 8080
```

### Deploy Your First Agent

1. **Create a Cell (namespace):**

```bash
cat <<EOF | ./bin/apiaryctl apply -f -
apiVersion: apiary.io/v1
kind: Cell
metadata:
  name: production
spec:
  resourceQuota:
    maxAgentSpecs: 100
    maxHives: 50
EOF
```

2. **Deploy an Agent:**

```bash
cat <<EOF | ./bin/apiaryctl apply -f - -n production
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: my-agent
  namespace: production
spec:
  runtime:
    command: ["python", "agent.py"]
  interface:
    type: stdin
  resources:
    requests:
      cpu: 500
      memory: 512000000
EOF
```

3. **Create a Hive (workflow):**

```bash
cat <<EOF | ./bin/apiaryctl apply -f - -n production
apiVersion: apiary.io/v1
kind: Hive
metadata:
  name: my-workflow
  namespace: production
spec:
  pattern: pipeline
  stages:
    - name: stage1
      agentRef: my-agent
      replicas: 2
EOF
```

**That's it!** Your agents are now running, scaling, and orchestrating automatically.

---

## 📚 Core Concepts

| Concept | Description | Kubernetes Analogy |
|---------|-------------|-------------------|
| **Hive** | A collection of agents working together for a singular purpose | Deployment + Service Mesh |
| **Agent** | A standalone AI process (any language, any framework) | Container |
| **AgentSpec** | Declarative definition of how to run an agent | PodSpec |
| **Drone** | A running instance of an agent | Pod |
| **Queen** | Central orchestrator managing all Hives and Drones | Control Plane |
| **Comb** | Session-scoped shared memory accessible by all agents | ConfigMap + Secrets |
| **Cell** | Namespace providing isolation and resource quotas | Namespace |
| **Keeper** | Sidecar process handling infrastructure concerns | Sidecar Container |

---

## 🎨 Orchestration Patterns

Apiary supports multiple orchestration patterns for different use cases:

### Pipeline Pattern
Sequential execution through multiple stages. Perfect for data processing workflows.

```yaml
pattern: pipeline
stages:
  - name: preprocessing
    agentRef: preprocessor
    replicas: 3
  - name: processing
    agentRef: processor
    replicas: 5
  - name: postprocessing
    agentRef: postprocessor
    replicas: 2
```

### Hierarchical Pattern
Supervisor-worker coordination. Ideal for task distribution and parallel processing.

### Swarm Pattern
All agents receive all messages. Great for consensus building and voting.

### Event-Driven Pattern
Asynchronous event processing. Perfect for reactive systems and event streams.

### Composed Pattern
Combine multiple patterns. Build complex workflows with maximum flexibility.

---

## 🛠️ Features

### 🔐 Multi-Tenancy & Isolation
- **Cells (Namespaces)**: Complete resource and network isolation
- **Resource Quotas**: Enforce limits per namespace
- **RBAC**: Role-based access control with Cell-scoped permissions

### 📊 Intelligent Scaling
- **Autoscaling**: Scale based on latency, queue depth, or custom metrics
- **Warm Pools**: Pre-warmed agents for instant response
- **Task Tier Scheduling**: Optimize for cost vs. capability

### 🛡️ Safety & Guardrails
- **Token Budgets**: Per-request, per-session, and per-minute limits
- **Rate Limiting**: Prevent abuse and control costs
- **Output Validation**: JSON schema validation for agent outputs
- **Circuit Breakers**: Automatic failure detection and recovery
- **Dead Letter Queue**: Capture and replay failed messages

### 📡 Communication
- **Message Bus (Nectar)**: NATS-based pub/sub with queue groups
- **Shared Memory (Comb)**: Session-scoped key-value store
- **Waggle Protocol**: Standardized message format for agents

### 🔍 Observability
- **OpenTelemetry**: Distributed tracing across all components
- **Structured Logging**: JSON logs with trace correlation
- **Metrics**: Request latency, throughput, error rates, and more

---

## 📖 Documentation

- 📘 [Architecture Guide](docs/ARCHITECTURE.md) — Deep dive into system design
- 🔌 [API Reference](docs/API.md) — Complete REST and gRPC API documentation
- ⌨️ [CLI Reference](docs/CLI.md) — Command-line tool guide
- 🚀 [Deployment Guide](docs/DEPLOYMENT.md) — Single-node, distributed, and Kubernetes
- 📝 [Examples](examples/) — Sample manifests and agents

---

## 🧪 Testing

### Unit Tests

```bash
go test ./...
```

### Benchmarks

Apiary includes comprehensive benchmark tests for all critical operations:

```bash
# Run all benchmarks with memory profiling
go test -bench=. -benchmem ./internal/...

# Benchmark specific components
go test -bench=. -benchmem ./internal/store/badger
go test -bench=. -benchmem ./internal/nectar
```

See the [README Benchmark Section](#benchmark-tests) below for detailed instructions.

### Integration Tests

```bash
go test -tags=integration ./...
```

---

## 📈 Performance

Apiary is optimized for performance:

- **< 100ms** API response time (P95 for CRUD operations)
- **< 50ms** Message bus overhead
- **< 500ms** Session creation
- **10,000+** messages/second throughput
- **1000+** concurrent sessions

See [build-docs/profiling.md](build-docs/profiling.md) for performance profiling guides.

---

## 🏃 Benchmark Tests

Benchmark tests are available for critical operations to measure and optimize performance.

### Running All Benchmarks

Run all benchmark tests with memory allocation tracking:

```bash
go test -bench=. -benchmem ./internal/...
```

### Running Specific Benchmarks

Run benchmarks for a specific component:

```bash
# Store operations
go test -bench=. -benchmem ./internal/store/badger

# Message bus (Nectar)
go test -bench=. -benchmem ./internal/nectar

# Shared memory (Comb)
go test -bench=. -benchmem ./internal/comb

# Session management
go test -bench=. -benchmem ./internal/session

# API server
go test -bench=. -benchmem ./internal/api
```

### Running a Specific Benchmark

Run a single benchmark function:

```bash
go test -bench=BenchmarkStore_Get -benchmem ./internal/store/badger
```

### Benchmark Options

- `-bench=.` - Run all benchmarks
- `-benchmem` - Report memory allocation statistics
- `-benchtime=10s` - Run each benchmark for a specific duration (default: 1s)
- `-count=5` - Run each benchmark multiple times for averaging
- `-cpu=1,2,4` - Run benchmarks with different GOMAXPROCS values

Example with multiple options:

```bash
go test -bench=BenchmarkStore_Get -benchmem -benchtime=5s -count=3 ./internal/store/badger
```

### Benchmark Output

Benchmark output shows:
- **Benchmark function name** - Name of the benchmark
- **Iterations** - Number of times the operation was executed (e.g., `1000000`)
- **ns/op** - Nanoseconds per operation
- **B/op** - Bytes allocated per operation
- **allocs/op** - Number of allocations per operation

Example output:
```
BenchmarkStore_Get-8   	 1000000	      1234 ns/op	     256 B/op	       4 allocs/op
```

This means:
- The benchmark ran 1,000,000 iterations
- Each operation took 1,234 nanoseconds on average
- Each operation allocated 256 bytes
- Each operation performed 4 memory allocations

### Performance Profiling

For detailed performance analysis, see [build-docs/profiling.md](build-docs/profiling.md).

Generate CPU profile:

```bash
go test -bench=BenchmarkStore_Get -cpuprofile=cpu.prof ./internal/store/badger
go tool pprof cpu.prof
```

Generate memory profile:

```bash
go test -bench=BenchmarkStore_Create -memprofile=mem.prof ./internal/store/badger
go tool pprof mem.prof
```

---

## 🏗️ Development

### Prerequisites

- Go 1.21 or later
- `protoc` and Go plugins (for gRPC code generation)

### Building

```bash
# Build all binaries
make build

# Build individual components
go build -o bin/apiaryd ./cmd/apiaryd
go build -o bin/apiaryctl ./cmd/apiaryctl
go build -o bin/keeper ./cmd/keeper
```

### Code Generation

```bash
# Generate protobuf code
make proto-gen
```

### Linting

```bash
make lint
# or
go vet ./...
golangci-lint run
```

### Project Structure

```
apiary/
├── api/              # API definitions (protobuf)
├── cmd/              # Application entry points
│   ├── apiaryd/      # API server
│   ├── apiaryctl/    # CLI tool
│   └── keeper/       # Agent sidecar
├── docs/             # Documentation
│   ├── ARCHITECTURE.md
│   ├── API.md
│   ├── CLI.md
│   └── DEPLOYMENT.md
├── examples/         # Example manifests and agents
│   ├── manifests/    # Sample resource definitions
│   └── agents/       # Example agent implementations
├── internal/         # Internal packages
│   ├── api/          # REST/gRPC API server
│   ├── queen/        # Central orchestrator
│   ├── scheduler/    # Resource scheduling
│   ├── launcher/     # Agent process management
│   ├── nectar/       # Message bus (NATS)
│   ├── comb/         # Shared memory (Comb)
│   ├── session/      # Session management
│   ├── store/        # Resource store (BadgerDB)
│   └── ...
├── pkg/              # Public packages
│   ├── apiary/       # Core types and interfaces
│   ├── manifest/     # Manifest parsing
│   └── waggle/       # Message protocol
└── build-docs/       # Build and design documentation
```

---

## 🔄 Workflow Example

Here's a complete workflow from zero to running agents:

```bash
# 1. Start Apiary
./bin/apiaryd -data-dir /var/apiary -port 8080 &

# 2. Create a namespace
./bin/apiaryctl apply -f examples/manifests/01-cell.yaml

# 3. Create a secret for API keys
./bin/apiaryctl apply -f examples/manifests/02-secret.yaml

# 4. Deploy an agent
./bin/apiaryctl apply -f examples/manifests/03-agentspec-simple.yaml -n default

# 5. Create a workflow
./bin/apiaryctl apply -f examples/manifests/05-hive-pipeline.yaml -n default

# 6. Check status
./bin/apiaryctl get hives -n default
./bin/apiaryctl get drones -n default

# 7. View logs
./bin/apiaryctl logs drone/<drone-id> -n default -f

# 8. Scale the workflow
./bin/apiaryctl scale hive/pipeline-hive --replicas=5 -n default
```

---

## 🌟 Use Cases

- **Multi-Agent AI Systems**: Coordinate specialized AI agents working together
- **AI Workflows**: Build complex data processing pipelines with AI components
- **LLM Applications**: Deploy and scale language model applications
- **Autonomous Systems**: Orchestrate autonomous agents with shared context
- **Event Processing**: Process events with AI-powered agents
- **Research Platforms**: Scale AI research experiments across teams

---

## 🤝 Contributing

We welcome contributions! Please see our contributing guidelines (coming soon).

---

## 📄 License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

---

## 🙏 Acknowledgments

Apiary is inspired by Kubernetes and brings the same declarative, cloud-native principles to AI agent orchestration.

---

**Built with ❤️ for the AI community**
