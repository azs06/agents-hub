# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Build the binary
go build ./cmd/agents-hub

# Run the hub (launches TUI by default)
./agents-hub

# Start hub in foreground mode
./agents-hub start --foreground

# Start with options
./agents-hub start --foreground --http-port 8080 --socket /tmp/a2a-hub.sock --verbose

# Stop the hub
./agents-hub stop

# Check hub status
./agents-hub status

# List agents with health info
./agents-hub agents --health

# Send a message to an agent
./agents-hub send codex "Write a hello world function in Go"

# List recent tasks
./agents-hub tasks --limit 20

# Launch TUI explicitly
./agents-hub tui
```

## Architecture Overview

This is a Go implementation of an A2A (Agent-to-Agent) Local Hub that enables CLI-based AI coding agents (Claude Code, Gemini CLI, OpenAI Codex, Mistral Vibe) to communicate using JSON-RPC 2.0.

### Core Components

**Hub Server** (`internal/hub/server.go`): Central server that coordinates all components. Manages agent registry, task manager, context manager, and JSON-RPC handler. Registers all RPC method handlers (`hub/status`, `hub/agents/list`, `message/send`, etc.).

**Agent Registry** (`internal/hub/registry.go`): Thread-safe registry of agents with periodic health checks (30-second intervals). Stores agent info including card, health status, and registration time.

**Agent Interface** (`internal/agents/base.go`): All agents implement this interface with methods: `ID()`, `Name()`, `Initialize()`, `Shutdown()`, `GetCard()`, `GetCapabilities()`, `CheckHealth()`, `Execute()`, `Cancel()`.

**CLI Agent** (`internal/agents/cli_agent.go`): Generic wrapper for CLI-based agents. Spawns external processes, captures output, handles timeouts. Used by Claude, Gemini, Codex, and Vibe agents.

**Orchestrator** (`internal/agents/orchestrator.go`): Special agent that delegates tasks to other agents. Splits prompts by newlines, semicolons, or " and " and distributes work across configured delegate agents in round-robin fashion.

**Transport Layer**:
- `internal/transport/unix.go`: Unix domain socket (NDJSON) - primary transport at `/tmp/a2a-hub.sock`
- `internal/transport/http.go`: HTTP server with JSON-RPC endpoint, health check, agent cards, and SSE streaming

**JSON-RPC Handler** (`internal/jsonrpc/handler.go`): Dispatches JSON-RPC 2.0 requests to registered method handlers.

**TUI** (`internal/tui/app.go`): Bubble Tea-based terminal UI with tabs for Status, Agents, Tasks, Send, History, and Settings. Uses command palette (`/` or `esc`) for navigation.

### Data Flow

1. Client sends JSON-RPC request via Unix socket or HTTP
2. Handler validates request and routes to appropriate method
3. For `message/send`: creates task, invokes target agent's `Execute()`
4. Agent wrapper spawns CLI process with prompt
5. Output is captured and returned as task result
6. Task state transitions: submitted -> working -> completed/failed

### JSON-RPC Methods

- `hub/status`: Get hub version, uptime, agent counts, task stats
- `hub/agents/list`: List registered agents (with optional health info)
- `hub/agents/get`: Get single agent by ID
- `hub/agents/health`: Get agent health status
- `hub/tasks/list`: List tasks (filterable by contextId, state, limit, offset)
- `hub/contexts/list`: List conversation contexts
- `message/send`: Send message to agent, returns completed task
- `tasks/get`: Get task by ID
- `tasks/cancel`: Cancel a running task

### Key Types (`internal/types/a2a.go`)

- `Message`: JSON-RPC message with parts (text/file/data)
- `Task`: Execution unit with status, history, artifacts
- `TaskState`: submitted, working, completed, failed, canceled, etc.
- `AgentCard`: Agent metadata following A2A protocol
- `ExecutionContext`/`ExecutionResult`: Agent execution params and results

### Persistence

State is stored in `~/.a2a-hub/`:
- `tasks.json`: Task history
- `contexts.json`: Conversation contexts
- `settings.json`: TUI settings including orchestrator delegates
- `hub.pid`: Process ID file

### Environment Variables

- `ORCHESTRATOR_AGENTS`: Comma-separated agent IDs for orchestrator (or "none" to disable)
- `CLAUDE_CMD`, `GEMINI_CMD`, `CODEX_CMD`, `VIBE_CMD`: Override agent executable paths