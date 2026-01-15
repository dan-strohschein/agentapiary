# Apiary API Reference

Apiary provides both REST and gRPC APIs for managing resources. All APIs follow a consistent structure and use the same authentication and authorization mechanisms.

## Base URL

- **REST API**: `http://localhost:8080/api/v1`
- **gRPC API**: `localhost:8080` (default port)

## Authentication

Currently, Apiary uses header-based authentication (MVP implementation). Set the following headers:

```
X-User-ID: <user-id>
X-User-Name: <user-name>
```

**Note**: This is an MVP implementation. Production should use JWT tokens or OAuth2/OIDC.

## REST API

### Cells

Cells provide namespace isolation. They are not namespaced themselves.

#### List Cells

```http
GET /api/v1/cells
```

**Response**: Array of Cell objects

#### Get Cell

```http
GET /api/v1/cells/:name
```

#### Create Cell

```http
POST /api/v1/cells
Content-Type: application/json

{
  "apiVersion": "apiary.io/v1",
  "kind": "Cell",
  "metadata": {
    "name": "my-cell"
  },
  "spec": {
    "resourceQuota": {
      "maxAgentSpecs": 100,
      "maxHives": 50
    }
  }
}
```

#### Update Cell

```http
PUT /api/v1/cells/:name
Content-Type: application/json

{...}
```

#### Delete Cell

```http
DELETE /api/v1/cells/:name
```

### AgentSpecs

AgentSpecs define how to run an agent. They are namespaced to a Cell.

#### List AgentSpecs

```http
GET /api/v1/cells/:namespace/agentspecs
```

#### Get AgentSpec

```http
GET /api/v1/cells/:namespace/agentspecs/:name
```

#### Create AgentSpec

```http
POST /api/v1/cells/:namespace/agentspecs
Content-Type: application/json

{
  "apiVersion": "apiary.io/v1",
  "kind": "AgentSpec",
  "metadata": {
    "name": "my-agent",
    "namespace": "default"
  },
  "spec": {
    "runtime": {
      "command": ["python", "agent.py"]
    },
    "interface": {
      "type": "stdin"
    },
    "resources": {
      "requests": {
        "cpu": 1000,
        "memory": 512000000
      }
    }
  }
}
```

#### Update AgentSpec

```http
PUT /api/v1/cells/:namespace/agentspecs/:name
```

#### Delete AgentSpec

```http
DELETE /api/v1/cells/:namespace/agentspecs/:name
```

#### Scale AgentSpec

```http
POST /api/v1/cells/:namespace/agentspecs/:name/scale
Content-Type: application/json

{
  "replicas": 3
}
```

#### Drain AgentSpec

```http
POST /api/v1/cells/:namespace/agentspecs/:name/drain
Content-Type: application/json

{
  "timeout": "30s",
  "force": false
}
```

### Hives

Hives orchestrate agents using various patterns. They are namespaced to a Cell.

#### List Hives

```http
GET /api/v1/cells/:namespace/hives
```

#### Get Hive

```http
GET /api/v1/cells/:namespace/hives/:name
```

#### Create Hive

```http
POST /api/v1/cells/:namespace/hives
Content-Type: application/json

{
  "apiVersion": "apiary.io/v1",
  "kind": "Hive",
  "metadata": {
    "name": "my-hive",
    "namespace": "default"
  },
  "spec": {
    "pattern": "pipeline",
    "stages": [...]
  }
}
```

#### Update Hive

```http
PUT /api/v1/cells/:namespace/hives/:name
```

#### Delete Hive

```http
DELETE /api/v1/cells/:namespace/hives/:name
```

#### Scale Hive

```http
POST /api/v1/cells/:namespace/hives/:name/scale
Content-Type: application/json

{
  "replicas": 2,
  "stage": "stage1"  // optional
}
```

### Secrets

Secrets store sensitive data (e.g., API keys). They are encrypted at rest.

#### List Secrets

```http
GET /api/v1/cells/:namespace/secrets
```

#### Get Secret

```http
GET /api/v1/cells/:namespace/secrets/:name
```

#### Create Secret

```http
POST /api/v1/cells/:namespace/secrets
Content-Type: application/json

{
  "apiVersion": "apiary.io/v1",
  "kind": "Secret",
  "metadata": {
    "name": "my-secret",
    "namespace": "default"
  },
  "data": {
    "api-key": "base64-encoded-value"
  }
}
```

#### Update Secret

```http
PUT /api/v1/cells/:namespace/secrets/:name
```

#### Delete Secret

```http
DELETE /api/v1/cells/:namespace/secrets/:name
```

### Drones

Drones are running instances of agents. They are namespaced to a Cell.

#### List Drones

```http
GET /api/v1/cells/:namespace/drones
```

#### Get Drone

```http
GET /api/v1/cells/:namespace/drones/:id
```

#### Delete Drone

```http
DELETE /api/v1/cells/:namespace/drones/:id
```

#### Get Drone Logs

```http
GET /api/v1/cells/:namespace/drones/:id/logs?follow=true&tail=100
```

**Query Parameters:**
- `follow`: Stream logs (default: false)
- `tail`: Number of lines from end (default: all)
- `since`: Show logs since timestamp

#### Exec in Drone

```http
POST /api/v1/cells/:namespace/drones/:id/exec
Content-Type: application/json

{
  "command": ["ls", "-la"],
  "stdin": "..."
}
```

**Response**: Streaming response with stdout/stderr

#### Drain Drone

```http
POST /api/v1/cells/:namespace/drones/:id/drain
Content-Type: application/json

{
  "timeout": "30s",
  "force": false
}
```

### Sessions

Sessions provide execution context with shared memory.

#### List Sessions

```http
GET /api/v1/cells/:namespace/sessions
```

#### Get Session

```http
GET /api/v1/cells/:namespace/sessions/:id
```

#### Create Session

```http
POST /api/v1/cells/:namespace/hives/:hive/sessions
Content-Type: application/json

{}
```

#### Send Message to Session

```http
POST /api/v1/cells/:namespace/sessions/:id/messages
Content-Type: application/json

{
  "content": "Hello, agent!",
  "metadata": {...}
}
```

#### Get Session Memory

```http
GET /api/v1/cells/:namespace/sessions/:id/memory
```

#### Delete Session

```http
DELETE /api/v1/cells/:namespace/sessions/:id
```

### Metrics

#### Get Global Metrics

```http
GET /api/v1/metrics
```

#### Get Namespace Metrics

```http
GET /api/v1/cells/:namespace/metrics
```

#### Get AgentSpec Metrics

```http
GET /api/v1/cells/:namespace/metrics/:agentspec
```

### Quota

#### Get Quota Usage

```http
GET /api/v1/cells/:namespace/quota
```

### RBAC

#### List Roles

```http
GET /api/v1/cells/:namespace/roles
```

#### Assign Role

```http
POST /api/v1/cells/:namespace/roles
Content-Type: application/json

{
  "userId": "user-123",
  "role": "developer"
}
```

#### Remove Role

```http
DELETE /api/v1/cells/:namespace/roles/:userId
```

### Dead Letter Queue (DLQ)

#### List DLQ Messages

```http
GET /api/v1/cells/:namespace/hives/:hive/dlq
```

#### Get DLQ Message

```http
GET /api/v1/cells/:namespace/hives/:hive/dlq/:messageId
```

#### Replay DLQ Message

```http
POST /api/v1/cells/:namespace/hives/:hive/dlq/:messageId/replay
```

#### Delete DLQ Message

```http
DELETE /api/v1/cells/:namespace/hives/:hive/dlq/:messageId
```

#### Clear DLQ

```http
DELETE /api/v1/cells/:namespace/hives/:hive/dlq
```

### Health Checks

#### Liveness Probe

```http
GET /healthz
```

**Response**: `{"status": "ok"}`

#### Readiness Probe

```http
GET /ready
```

**Response**: `{"status": "ready"}` or `{"status": "not_ready", "reason": "store_unavailable"}`

## gRPC API

The gRPC API mirrors all REST endpoints. The service is defined in `api/proto/v1/apiary.proto`.

### Service: ApiaryService

#### Cells RPCs

- `ListCells(Empty) returns (ListCellsResponse)`
- `GetCell(GetCellRequest) returns (Cell)`
- `CreateCell(CreateCellRequest) returns (Cell)`
- `UpdateCell(UpdateCellRequest) returns (Cell)`
- `DeleteCell(DeleteCellRequest) returns (Empty)`

#### AgentSpecs RPCs

- `ListAgentSpecs(ListAgentSpecsRequest) returns (ListAgentSpecsResponse)`
- `GetAgentSpec(GetAgentSpecRequest) returns (AgentSpec)`
- `CreateAgentSpec(CreateAgentSpecRequest) returns (AgentSpec)`
- `UpdateAgentSpec(UpdateAgentSpecRequest) returns (AgentSpec)`
- `DeleteAgentSpec(DeleteAgentSpecRequest) returns (Empty)`
- `ScaleAgentSpec(ScaleAgentSpecRequest) returns (ScaleAgentSpecResponse)`
- `DrainAgentSpec(DrainAgentSpecRequest) returns (DrainAgentSpecResponse)`

#### Hives RPCs

- `ListHives(ListHivesRequest) returns (ListHivesResponse)`
- `GetHive(GetHiveRequest) returns (Hive)`
- `CreateHive(CreateHiveRequest) returns (Hive)`
- `UpdateHive(UpdateHiveRequest) returns (Hive)`
- `DeleteHive(DeleteHiveRequest) returns (Empty)`
- `ScaleHive(ScaleHiveRequest) returns (ScaleHiveResponse)`

#### Secrets RPCs

- `ListSecrets(ListSecretsRequest) returns (ListSecretsResponse)`
- `GetSecret(GetSecretRequest) returns (Secret)`
- `CreateSecret(CreateSecretRequest) returns (Secret)`
- `UpdateSecret(UpdateSecretRequest) returns (Secret)`
- `DeleteSecret(DeleteSecretRequest) returns (Empty)`

#### Drones RPCs

- `ListDrones(ListDronesRequest) returns (ListDronesResponse)`
- `GetDrone(GetDroneRequest) returns (Drone)`
- `DeleteDrone(DeleteDroneRequest) returns (Empty)`
- `GetDroneLogs(GetDroneLogsRequest) returns (stream GetDroneLogsResponse)` - Server-side streaming
- `ExecDrone(stream ExecDroneRequest) returns (stream ExecDroneResponse)` - Bidirectional streaming
- `DrainDrone(DrainDroneRequest) returns (DrainDroneResponse)`

### Authentication

gRPC uses metadata headers for authentication:

```go
md := metadata.New(map[string]string{
    "x-user-id": "user-123",
    "x-user-name": "John Doe",
})
ctx := metadata.NewOutgoingContext(context.Background(), md)
```

### Error Codes

gRPC uses standard error codes:
- `NotFound` (5): Resource not found
- `AlreadyExists` (6): Resource already exists
- `InvalidArgument` (3): Invalid request
- `Internal` (13): Internal server error

## Response Formats

### Success Responses

- **200 OK**: Successful GET/PUT/DELETE
- **201 Created**: Successful POST (resource created)
- **204 No Content**: Successful DELETE (no content to return)

### Error Responses

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "Resource not found",
    "details": {...}
  }
}
```

Common error codes:
- `NOT_FOUND`: Resource does not exist
- `ALREADY_EXISTS`: Resource already exists
- `INVALID_INPUT`: Invalid request data
- `UNAUTHORIZED`: Authentication required
- `FORBIDDEN`: Insufficient permissions
- `INTERNAL_ERROR`: Internal server error

## Rate Limiting

Rate limiting is enforced per Cell and per user. Default limits:
- 100 requests/minute per user
- 1000 requests/minute per Cell

## Pagination

List endpoints support pagination:

```
GET /api/v1/cells/:namespace/hives?limit=100&offset=0
```

**Query Parameters:**
- `limit`: Maximum number of items to return (default: 100)
- `offset`: Number of items to skip (default: 0)

## Filtering and Label Selectors

Resources can be filtered using label selectors:

```
GET /api/v1/cells/:namespace/hives?labelSelector=app=myapp,env=prod
```

Label selector format: `key1=value1,key2=value2`
