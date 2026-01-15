# Example Agents

This directory contains example agents that demonstrate how to create agents that work with Apiary.

## Echo Agent

**Location:** `echo/agent.py`

A simple agent that reads from stdin and echoes messages back with a prefix.

**Features:**
- Reads messages line-by-line
- Supports both JSON (Waggle format) and plain text
- Returns structured JSON responses

**Usage:**

```bash
# Make executable
chmod +x echo/agent.py

# Test locally
echo '{"type":"message","content":"Hello"}' | python3 echo/agent.py
```

**AgentSpec example:**

```yaml
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: echo-agent
spec:
  runtime:
    command: ["python3", "/path/to/echo/agent.py"]
  interface:
    type: stdin
```

## Calculator Agent

**Location:** `calculator/agent.py`

A calculator agent that performs mathematical operations.

**Features:**
- Parses mathematical expressions
- Supports basic operations (+, -, *, /)
- Returns structured results
- Input validation

**Usage:**

```bash
chmod +x calculator/agent.py
echo '{"type":"calculation","expression":"2+2"}' | python3 calculator/agent.py
```

**AgentSpec example:**

```yaml
apiVersion: apiary.io/v1
kind: AgentSpec
metadata:
  name: calculator-agent
spec:
  runtime:
    command: ["python3", "/path/to/calculator/agent.py"]
  interface:
    type: stdin
```

## Multi-Agent Coordinator

**Location:** `multi-agent/coordinator.py`

A coordinator agent that orchestrates multiple sub-agents.

**Features:**
- Validates input
- Processes data
- Formats output
- Demonstrates hierarchical coordination

**Usage:**

```bash
chmod +x multi-agent/coordinator.py
echo '{"type":"coordinate","data":"Hello World"}' | python3 multi-agent/coordinator.py
```

## Creating Your Own Agent

Agents must:
1. Read from stdin (for stdin interface type)
2. Output JSON responses to stdout
3. Output errors to stderr
4. Handle the Waggle message format (see `pkg/waggle/waggle.go`)

**Basic Structure:**

```python
#!/usr/bin/env python3
import sys
import json

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    
    try:
        message = json.loads(line)
        # Process message
        response = {
            'type': 'response',
            'content': 'Your response here'
        }
        print(json.dumps(response))
        sys.stdout.flush()
    except Exception as e:
        error = {'type': 'error', 'error': str(e)}
        print(json.dumps(error), file=sys.stderr)
        sys.stderr.flush()
```

## Waggle Message Format

Agents receive messages in Waggle format:

```json
{
  "id": "msg-123",
  "type": "message",
  "content": "Hello, agent!",
  "metadata": {...},
  "sessionId": "session-456",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

Agents should respond with:

```json
{
  "type": "response",
  "content": "Response content",
  "metadata": {...}
}
```

See `pkg/waggle/waggle.go` for the complete message format specification.
