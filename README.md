# A2A Local Hub (Go)

This is a Go implementation of the A2A Local Hub defined in `SPECIFICATION.md`. It exposes a JSON-RPC 2.0 API over a Unix domain socket and HTTP, provides a CLI, and includes a basic terminal UI (TUI).

## Features

- JSON-RPC 2.0 hub methods and message send flow
- Unix socket transport (NDJSON)
- HTTP transport with health endpoint, agent cards, and SSE
- CLI for start/stop/status/agents/send/tasks
- Simple TUI for interactive use
- Local persistence for tasks/contexts in `~/.a2a-hub`

## Build

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

Environment:

- `ORCHESTRATOR_AGENTS=codex,gemini` (or `none` to disable)
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

Commands inside the TUI:

- `tab` / `shift+tab` to switch tabs
- `r` refresh
- `q` quit
- `enter` send message (Send tab)

## HTTP API

Default host/port: `127.0.0.1:8080`

- `POST /` JSON-RPC endpoint
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
- `~/.a2a-hub/settings.json` (TUI settings like orchestrator delegates)

State is loaded on startup.

## Notes

- Agent CLIs (claude, gemini, codex, vibe) must be installed and available in `PATH`.
- Unix socket is the default transport used by the CLI/TUI.
