# apiaryctl CLI Reference

`apiaryctl` is a command-line tool for managing Apiary resources, similar to `kubectl` for Kubernetes.

## Installation

Build from source:

```bash
go build -o bin/apiaryctl ./cmd/apiaryctl
```

Or use the Makefile:

```bash
make build
```

## Global Flags

All commands support these global flags:

- `--server, -s`: API server address (default: `http://localhost:8080`)
- `--namespace, -n`: Namespace (Cell) to use (default: `default`)
- `--output, -o`: Output format (`yaml`, `json`, `wide`, `name`)
- `--config`: Config file path (default: `$HOME/.apiaryctl.yaml`)
- `--user-id`: User ID for authentication (or set `APIARY_USER_ID` env var)
- `--user-name`: User name for authentication (or set `APIARY_USER_NAME` env var)

## Commands

### apply

Apply a manifest file to create or update resources.

```bash
apiaryctl apply -f manifest.yaml
```

**Flags:**
- `-f, --filename`: Manifest file(s) to apply (required)
- `--dry-run`: Validate manifest without applying
- `-R, --recursive`: Process directory recursively

**Examples:**

```bash
# Apply a single manifest
apiaryctl apply -f manifest.yaml

# Apply multiple manifests
apiaryctl apply -f manifest1.yaml -f manifest2.yaml

# Dry-run to validate
apiaryctl apply -f manifest.yaml --dry-run

# Apply all manifests in a directory
apiaryctl apply -f ./manifests -R
```

### get

Display one or many resources.

```bash
apiaryctl get <resource-type> [name] [-n namespace]
```

**Resource Types:**
- `cells`, `cell`
- `agentspecs`, `agentspec`
- `hives`, `hive`
- `secrets`, `secret`
- `drones`, `drone`
- `sessions`, `session`

**Flags:**
- `-l, --selector`: Label selector (e.g., `app=myapp`)
- `-w, --watch`: Watch for changes
- `-o, --output`: Output format

**Examples:**

```bash
# List all Hives in a namespace
apiaryctl get hives -n default

# Get a specific Hive
apiaryctl get hive my-hive -n default

# Get in YAML format
apiaryctl get hives -n default -o yaml

# Get with label selector
apiaryctl get hives -n default -l app=myapp

# Watch for changes
apiaryctl get hives -n default -w

# List all AgentSpecs
apiaryctl get agentspecs -n default
```

### describe

Show detailed information about a specific resource.

```bash
apiaryctl describe <resource-type> <name> [-n namespace]
```

**Examples:**

```bash
# Describe a Hive
apiaryctl describe hive my-hive -n default

# Describe an AgentSpec
apiaryctl describe agentspec my-agent -n default

# Describe a Drone
apiaryctl describe drone my-drone -n default

# Describe a Cell
apiaryctl describe cell my-cell
```

### delete

Delete one or more resources.

```bash
apiaryctl delete <resource-type> <name> [-n namespace]
```

**Flags:**
- `-f, --filename`: Delete resources from manifest file
- `--force`: Force deletion without confirmation

**Examples:**

```bash
# Delete a Hive
apiaryctl delete hive my-hive -n default

# Delete from manifest
apiaryctl delete -f manifest.yaml

# Force delete
apiaryctl delete hive my-hive -n default --force
```

### logs

Print logs from a Drone.

```bash
apiaryctl logs drone/<name> [-n namespace]
```

**Flags:**
- `-f, --follow`: Follow log output (stream)
- `--tail`: Number of lines to show from end (default: all)
- `--since`: Show logs since timestamp (e.g., `1h`, `30m`, `2021-01-01T00:00:00Z`)

**Examples:**

```bash
# View logs from a Drone
apiaryctl logs drone/my-drone -n default

# Follow logs (stream)
apiaryctl logs drone/my-drone -n default -f

# Show last 100 lines
apiaryctl logs drone/my-drone -n default --tail=100

# Show logs since 1 hour ago
apiaryctl logs drone/my-drone -n default --since=1h
```

### exec

Execute a command in a Drone's Keeper.

```bash
apiaryctl exec drone/<name> [-n namespace] -- <command> [args...]
```

**Examples:**

```bash
# Execute a command
apiaryctl exec drone/my-drone -n default -- ls -la

# Execute with stdin
echo "input" | apiaryctl exec drone/my-drone -n default -- cat

# Execute interactive shell
apiaryctl exec drone/my-drone -n default -- /bin/sh
```

### scale

Scale a resource (AgentSpec or Hive).

```bash
apiaryctl scale <resource-type>/<name> --replicas=<count> [-n namespace]
```

**Flags:**
- `--replicas`: Number of replicas (required)
- `--stage`: Stage name (for Hive scaling only)

**Examples:**

```bash
# Scale an AgentSpec to 3 replicas
apiaryctl scale agentspec/my-agent --replicas=3 -n default

# Scale a Hive stage
apiaryctl scale hive/my-hive --replicas=2 --stage=stage1 -n default
```

### drain

Drain a resource (gracefully stop and remove).

```bash
apiaryctl drain <resource-type>/<name> [-n namespace]
```

**Resource Types:**
- `drone/<name>`
- `agentspec/<name>`

**Flags:**
- `--force`: Force drain without graceful shutdown
- `--timeout`: Timeout for graceful shutdown (default: `30s`)

**Examples:**

```bash
# Drain a Drone gracefully
apiaryctl drain drone/my-drone -n default

# Drain with custom timeout
apiaryctl drain drone/my-drone -n default --timeout=60s

# Force drain
apiaryctl drain drone/my-drone -n default --force

# Drain all Drones for an AgentSpec
apiaryctl drain agentspec/my-agent -n default
```

## Configuration

### Config File

Create `$HOME/.apiaryctl.yaml`:

```yaml
server: http://localhost:8080
namespace: default
output: yaml
user-id: my-user-id
user-name: My Name
```

### Environment Variables

- `APIARY_SERVER`: API server address
- `APIARY_NAMESPACE`: Default namespace
- `APIARY_OUTPUT`: Default output format
- `APIARY_USER_ID`: User ID for authentication
- `APIARY_USER_NAME`: User name for authentication

## Output Formats

### yaml

YAML format (default):

```bash
apiaryctl get hive my-hive -n default -o yaml
```

### json

JSON format:

```bash
apiaryctl get hive my-hive -n default -o json
```

### wide

Wide format with additional columns:

```bash
apiaryctl get hives -n default -o wide
```

### name

Show only resource names:

```bash
apiaryctl get hives -n default -o name
```

## Examples

### Complete Workflow

```bash
# Create a Cell
apiaryctl apply -f cell.yaml

# Create an AgentSpec
apiaryctl apply -f agentspec.yaml -n default

# Create a Hive
apiaryctl apply -f hive.yaml -n default

# List resources
apiaryctl get hives -n default

# View Hive details
apiaryctl describe hive my-hive -n default

# View logs
apiaryctl logs drone/my-drone -n default -f

# Scale the Hive
apiaryctl scale hive/my-hive --replicas=3 -n default

# Drain when done
apiaryctl drain agentspec/my-agent -n default
```

## Error Handling

Common errors and solutions:

- **Resource not found**: Check namespace and resource name
- **Connection refused**: Ensure API server is running
- **Authentication failed**: Set `--user-id` and `--user-name` or environment variables
- **Permission denied**: Check RBAC permissions for your user
