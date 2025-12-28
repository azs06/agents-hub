# A2A Local Hub (Go)

This is a Go implementation of the A2A Local Hub defined in `SPECIFICATION.md`. It exposes a JSON-RPC 2.0 API over a Unix domain socket and HTTP, provides a CLI, and includes a basic terminal UI (TUI).

## Features

- JSON-RPC 2.0 hub methods and message send flow
- Unix socket transport (NDJSON)
- HTTP transport with health endpoint, agent cards, and SSE
- CLI for start/stop/status/agents/send/tasks
- Interactive TUI with real-time streaming output
- Local persistence for tasks/contexts in `~/.a2a-hub`
- **Claude flexibility**: Model selection, tool restrictions, session continuation
- **Codex flexibility**: Model/profile selection, sandbox + approval policy, web search
- Multi-agent support with @mentions (`@claude task1, @gemini task2`)

## Installation

### From Source

```bash
git clone https://github.com/soikat/agents-hub.git
cd agents-hub
go install ./cmd/agents-hub
```

This installs the `agents-hub` binary to your `$GOPATH/bin` (or `$GOBIN` if set).

### Build Only

```bash
go build ./cmd/agents-hub
```

## Quickstart

```bash
./agents-hub
```

In another terminal:

```bash
./agents-hub send codex "Write a hello world function in Go"
```

## Run the Hub

Start the hub in the foreground:

```bash
./agents-hub start --foreground
```

Common options:

- `--socket /tmp/a2a-hub.sock`
- `--http-port 8080`
- `--no-http`
- `--verbose`
- `--orchestrator-agents codex,gemini` (or `none` to disable)
- `--orchestrator-router vibe` (agent ID to enable LLM-driven routing)

Environment:

- `ORCHESTRATOR_AGENTS=codex,gemini` (or `none` to disable)
- `ORCHESTRATOR_ROUTER=vibe` (agent ID to enable LLM-driven routing)
- `CLAUDE_CMD=/path/to/claude` (override agent executable)
- `GEMINI_CMD=/path/to/gemini`
- `CODEX_CMD=/path/to/codex`
- `VIBE_CMD=/path/to/vibe`

Stop the hub:

```bash
./agents-hub stop
```

## CLI Usage

Check hub status:

```bash
./agents-hub status
```

List agents (with health):

```bash
./agents-hub agents --health
```

Send a message to an agent:

```bash
./agents-hub send codex "Write a hello world function in Go"
```

List recent tasks:

```bash
./agents-hub tasks --limit 20
```

## TUI

Launch the Bubble Tea terminal UI (default when no subcommand is used):

```bash
./agents-hub
```

Explicit launch:

```bash
./agents-hub tui
```

TUI options:

- `--socket /tmp/a2a-hub.sock`
- `--no-socket`
- `--http-port 8080`
- `--no-http`
- `--orchestrator-agents codex,gemini` (or `none` to disable)
- `--orchestrator-router vibe` (agent ID to enable LLM-driven routing)

Commands inside the TUI:

- `tab` / `shift+tab` to switch tabs
- `r` refresh
- `q` quit
- `enter` send message (Send tab)
- `/` or `esc` open command palette

Command palette commands:

- `/status`, `/agents`, `/tasks`, `/history`, `/settings` - navigate tabs
- `/send <agent> <msg>` - send a message
- `/agent <id>` - set target agent
- `/claude-model <opus|sonnet|haiku>` - set Claude model
- `/claude-tools <safe|normal|full>` - set Claude tool profile
- `/claude-continue` - toggle session continuation
- `/codex-model <model>` - set Codex model
- `/codex-profile <profile>` - set Codex config profile
- `/codex-sandbox <read-only|workspace-write|danger-full-access>` - set Codex sandbox
- `/codex-approval <untrusted|on-failure|on-request|never>` - set Codex approval policy
- `/codex-search` - toggle Codex web search
- `/help` - show help overlay

## HTTP API

Default host/port: `127.0.0.1:8080`

- `POST /` JSON-RPC endpoint
- `POST /a2a` A2A JSON-RPC endpoint
- `GET /health`
- `GET /.well-known/agent.json`
- `GET /.well-known/agents`
- `GET /.well-known/agents/{agentId}.json`
- `POST /stream` SSE endpoint

Example JSON-RPC call:

```bash
curl -s http://127.0.0.1:8080/ \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"hub/status","params":{},"id":"1"}'
```

## Persistence

The hub stores tasks and contexts locally:

- `~/.a2a-hub/tasks.json`
- `~/.a2a-hub/contexts.json`
- `~/.a2a-hub/settings.json` (TUI settings including Claude + Codex configuration)

State is loaded on startup.

## Claude Settings

Claude has enhanced flexibility with configurable options:

### Model Selection

Choose which Claude model to use:

```bash
# Via TUI command
/claude-model sonnet

# Options: opus, sonnet, haiku (blank for default)
```

### Tool Restrictions

Control which tools Claude can use:

| Profile | Tools Allowed |
|---------|---------------|
| `safe` | Read, Glob, Grep, LSP (read-only) |
| `normal` | + Edit, Write, WebFetch, WebSearch |
| `full` | All tools (default) |

```bash
# Via TUI command
/claude-tools safe
```

### Session Continuation

Enable Claude to continue previous conversations:

```bash
# Toggle via TUI command
/claude-continue
```

When enabled, Claude uses `--continue` to resume the most recent session context.

### Settings UI

In the TUI Settings tab, use Tab/Shift+Tab to navigate between:

1. Orchestrator delegates
2. Claude model
3. Claude tool profile
4. Continue mode checkbox (Space to toggle)
5. Codex model
6. Codex profile
7. Codex sandbox
8. Codex approval policy
9. Codex web search (Space to toggle)

Press Enter to save each field.

### Claude Skills

Claude exposes these skills for intelligent routing:

- `code-review` - Review code for quality and bugs
- `refactor` - Restructure code
- `write-tests` - Write unit/integration tests
- `debug` - Diagnose and fix bugs
- `explain` - Explain code architecture
- `document` - Write documentation
- `architect` - Design system architecture
- `implement` - Write new code

## Codex Settings

Codex supports additional runtime configuration:

### Model Selection

```bash
# Via TUI command
/codex-model o3
```

### Config Profile

```bash
# Use a named profile from ~/.codex/config.toml
/codex-profile my-profile
```

### Sandbox + Approval Policy

```bash
# Sandbox modes: read-only, workspace-write, danger-full-access
/codex-sandbox workspace-write

# Approval policies: untrusted, on-failure, on-request, never
/codex-approval on-request
```

### Web Search

```bash
/codex-search
```

## Gemini Settings

Gemini supports runtime configuration:

### Model Selection

```bash
# Via TUI command
/gemini-model gemini-1.5-pro
```

### Settings UI

In the TUI Settings tab, you can configure the default Gemini model.

## Multi-Agent Messaging

Send tasks to multiple agents using @mentions:

```bash
# In TUI Send tab
@claude write the API, @gemini write the frontend

# Comma or "and" separated
@codex implement auth and @vibe review it
```

Each agent runs concurrently with its own streaming output.

## Notes

- Agent CLIs (claude, gemini, codex, vibe) must be installed and available in `PATH`.
- Unix socket is the default transport used by the CLI/TUI (CLI `send` will try A2A over HTTP first when available).
- CLI/TUI send the current working directory to agents when available (Codex uses it for `--cd`).
