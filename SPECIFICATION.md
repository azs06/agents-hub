# A2A Local Hub - Technical Specification

## Overview

The A2A Local Hub is a local multi-agent orchestration system that enables CLI-based AI coding agents (Claude Code, Gemini CLI, OpenAI Codex, Mistral Vibe) to communicate with each other using the A2A (Agent-to-Agent) Protocol.

### Goals

1. **Unified Interface**: Provide a single entry point to interact with multiple AI coding agents
2. **Inter-Agent Communication**: Enable agents to delegate tasks to other agents
3. **Protocol Compliance**: Follow Google's A2A Protocol specification for agent communication
4. **Local-First**: Run entirely on localhost using Unix sockets and HTTP

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        A2A Local Hub                            │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │   CLI       │  │  HTTP API   │  │   Unix Socket API       │  │
│  │  Interface  │  │  :8080      │  │   /tmp/a2a-hub.sock     │  │
│  └──────┬──────┘  └──────┬──────┘  └───────────┬─────────────┘  │
│         │                │                     │                │
│         └────────────────┼─────────────────────┘                │
│                          ▼                                      │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                    Hub Server                             │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐   │  │
│  │  │ JSON-RPC    │  │   Task      │  │    Context      │   │  │
│  │  │ Handler     │  │   Manager   │  │    Manager      │   │  │
│  │  └─────────────┘  └─────────────┘  └─────────────────┘   │  │
│  └───────────────────────────────────────────────────────────┘  │
│                          │                                      │
│                          ▼                                      │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                   Agent Registry                          │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │  │
│  │  │ Claude   │ │ Gemini   │ │ Codex    │ │ Vibe     │     │  │
│  │  │ Agent    │ │ Agent    │ │ Agent    │ │ Agent    │     │  │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘     │  │
│  └───────┼────────────┼────────────┼────────────┼───────────┘  │
└──────────┼────────────┼────────────┼────────────┼──────────────┘
           ▼            ▼            ▼            ▼
      ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
      │ claude  │  │ gemini  │  │ codex   │  │ vibe    │
      │  CLI    │  │  CLI    │  │  CLI    │  │  CLI    │
      └─────────┘  └─────────┘  └─────────┘  └─────────┘
```

---

## Protocol Specification

### Transport Layer

The hub supports two transport mechanisms:

#### 1. Unix Domain Socket (Primary)

- **Path**: `/tmp/a2a-hub.sock`
- **Protocol**: Newline-delimited JSON (NDJSON)
- **Message Format**: Each JSON-RPC message is terminated by `\n`

#### 2. HTTP (Secondary)

- **Host**: `127.0.0.1`
- **Port**: `8080`
- **Endpoints**:
  - `POST /` - JSON-RPC endpoint
  - `GET /health` - Health check
  - `GET /.well-known/agent.json` - Hub's agent card
  - `GET /.well-known/agents` - List all agent cards
  - `GET /.well-known/agents/{agentId}.json` - Individual agent card
  - `POST /stream` - SSE streaming endpoint

---

## JSON-RPC 2.0 Methods

All communication uses JSON-RPC 2.0 format:

```json
{
  "jsonrpc": "2.0",
  "method": "method/name",
  "params": { ... },
  "id": "unique-request-id"
}
```

### Hub Methods

#### `hub/status`

Get hub status and statistics.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "hub/status",
  "params": {},
  "id": "1"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "version": "1.0.0",
    "uptime": 12345,
    "agents": [
      { "id": "claude-code", "name": "Claude Code CLI", "status": "healthy" },
      { "id": "gemini", "name": "Gemini CLI", "status": "healthy" }
    ],
    "activeTasks": 0,
    "totalTasks": 5,
    "total": 4,
    "healthy": 4,
    "degraded": 0,
    "unhealthy": 0,
    "unknown": 0
  },
  "id": "1"
}
```

#### `hub/agents/list`

List all registered agents.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "hub/agents/list",
  "params": {
    "includeHealth": true
  },
  "id": "2"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": [
    {
      "id": "claude-code",
      "name": "Claude Code CLI",
      "card": { /* AgentCard */ },
      "health": { "status": "healthy", "latencyMs": 50 },
      "registeredAt": "2025-01-01T00:00:00.000Z"
    }
  ],
  "id": "2"
}
```

#### `hub/agents/get`

Get details of a specific agent.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "hub/agents/get",
  "params": {
    "agentId": "claude-code"
  },
  "id": "3"
}
```

#### `hub/agents/health`

Check agent health.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "hub/agents/health",
  "params": {
    "agentId": "claude-code"
  },
  "id": "4"
}
```

#### `hub/tasks/list`

List tasks with optional filtering.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "hub/tasks/list",
  "params": {
    "contextId": "optional-context-id",
    "state": "completed",
    "limit": 20,
    "offset": 0
  },
  "id": "5"
}
```

#### `hub/contexts/list`

List conversation contexts.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "hub/contexts/list",
  "params": {
    "limit": 10
  },
  "id": "6"
}
```

### Message Methods

#### `message/send`

Send a message to an agent.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "message/send",
  "params": {
    "message": {
      "kind": "message",
      "messageId": "msg-123",
      "role": "user",
      "parts": [
        {
          "kind": "text",
          "text": "Write a hello world function in Python"
        }
      ],
      "contextId": "optional-context-id",
      "metadata": {
        "targetAgent": "claude-code"
      }
    },
    "configuration": {
      "historyLength": 10
    }
  },
  "id": "7"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "kind": "task",
    "id": "task-456",
    "contextId": "ctx-789",
    "status": {
      "state": "completed",
      "message": {
        "kind": "message",
        "messageId": "resp-123",
        "role": "agent",
        "parts": [
          {
            "kind": "text",
            "text": "def hello():\n    print('Hello, World!')"
          }
        ],
        "taskId": "task-456",
        "contextId": "ctx-789"
      },
      "timestamp": "2025-01-01T00:00:00.000Z"
    },
    "history": [ /* message history */ ],
    "artifacts": []
  },
  "id": "7"
}
```

### Task Methods

#### `tasks/get`

Get a specific task.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "tasks/get",
  "params": {
    "id": "task-456"
  },
  "id": "8"
}
```

#### `tasks/cancel`

Cancel a running task.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "tasks/cancel",
  "params": {
    "id": "task-456"
  },
  "id": "9"
}
```

---

## Data Types

### TaskState

Valid task states following A2A Protocol:

```typescript
type TaskState =
  | "submitted"      // Task received, not yet started
  | "working"        // Agent is processing
  | "input-required" // Agent needs more input
  | "completed"      // Task finished successfully
  | "canceled"       // Task was canceled
  | "failed"         // Task failed with error
  | "rejected"       // Task was rejected by agent
  | "auth-required"  // Authentication needed
  | "unknown";       // Unknown state
```

### State Transitions

```
submitted ──► working ──► completed
    │            │
    │            ├──► failed
    │            │
    │            ├──► canceled
    │            │
    │            ├──► input-required ──► working
    │            │
    │            └──► auth-required ──► working
    │
    ├──► canceled
    │
    ├──► rejected
    │
    └──► failed
```

### Message

```typescript
interface Message {
  kind: "message";
  messageId: string;           // UUID
  role: "user" | "agent";
  parts: Part[];               // Content parts
  taskId?: string;             // Associated task
  contextId?: string;          // Conversation context
  metadata?: Record<string, unknown>;
}
```

### Part Types

```typescript
// Text content
interface TextPart {
  kind: "text";
  text: string;
}

// File reference
interface FilePart {
  kind: "file";
  file: {
    name: string;
    mimeType: string;
    bytes?: string;  // Base64 encoded
    uri?: string;    // File URI
  };
}

// Structured data
interface DataPart {
  kind: "data";
  data: Record<string, unknown>;
}

type Part = TextPart | FilePart | DataPart;
```

### Task

```typescript
interface Task {
  kind: "task";
  id: string;                  // UUID
  contextId: string;           // Conversation context
  status: TaskStatus;
  history?: Message[];         // Conversation history
  artifacts?: Artifact[];      // Generated artifacts
  metadata?: Record<string, unknown>;
}

interface TaskStatus {
  state: TaskState;
  message?: Message;           // Latest message
  timestamp: string;           // ISO 8601
}
```

### Artifact

```typescript
interface Artifact {
  artifactId: string;
  name: string;
  description?: string;
  parts: Part[];
  metadata?: Record<string, unknown>;
}
```

### AgentCard

```typescript
interface AgentCard {
  protocolVersion: string;     // "1.0"
  name: string;
  description: string;
  url: string;                 // Agent endpoint URL
  version: string;
  provider: {
    name: string;
    url?: string;
  };
  skills: Skill[];
  capabilities: {
    streaming: boolean;
    pushNotifications: boolean;
    stateTransitionHistory: boolean;
  };
  securitySchemes?: Record<string, SecurityScheme>;
}

interface Skill {
  id: string;
  name: string;
  description: string;
  tags: string[];
  inputModes?: string[];       // MIME types
  outputModes?: string[];      // MIME types
}
```

### AgentHealth

```typescript
interface AgentHealth {
  status: "healthy" | "degraded" | "unhealthy" | "unknown";
  lastCheck: Date;
  latencyMs?: number;
  errorMessage?: string;
}
```

---

## Agent Wrapper Interface

Each CLI agent is wrapped with a common interface:

```typescript
abstract class BaseAgent {
  abstract agentId: string;
  abstract name: string;

  // Lifecycle
  abstract initialize(): Promise<void>;
  abstract shutdown(): Promise<void>;

  // Capabilities
  abstract getCard(): Promise<AgentCard>;
  abstract getCapabilities(): RuntimeCapabilities;
  abstract checkHealth(): Promise<AgentHealth>;

  // Execution
  abstract execute(context: ExecutionContext): Promise<ExecutionResult>;
  abstract executeStream(context: ExecutionContext): AsyncGenerator<StreamEvent>;
  abstract cancel(taskId: string): Promise<boolean>;
}

interface ExecutionContext {
  taskId: string;
  contextId: string;
  userMessage: Message;
  previousHistory?: Message[];
  workingDirectory?: string;
  timeout: number;
}

interface ExecutionResult {
  task: Task;
  artifacts: Artifact[];
  finalState: TaskState;
}

interface RuntimeCapabilities {
  supportsStreaming: boolean;
  supportsCancellation: boolean;
  maxConcurrentTasks: number;
  supportedInputModes: string[];
  supportedOutputModes: string[];
}
```

---

## Agent CLI Invocations

### Claude Code

```bash
# Health check
claude --version

# Execute prompt (non-interactive)
claude -p "your prompt here" --output-format text

# Important: Close stdin immediately after spawn
```

### Gemini

```bash
# Health check
gemini --version

# Execute prompt (positional argument)
gemini "your prompt here" -o text

# Streaming
gemini "your prompt here" -o stream-json
```

### Codex

```bash
# Health check
codex --version

# Execute prompt (requires trusted git directory)
codex exec "your prompt here"
```

### Vibe

```bash
# Health check
vibe --help

# Execute prompt
vibe -p "your prompt here" --output text

# Streaming
vibe -p "your prompt here" --output streaming
```

---

## Hub Server Components

### 1. HubServer

Main entry point that:
- Initializes Unix socket and HTTP transports
- Registers JSON-RPC method handlers
- Manages agent registry
- Handles graceful shutdown

**Configuration:**

```typescript
interface HubServerConfig {
  socket: {
    path: string;        // Default: /tmp/a2a-hub.sock
    enabled: boolean;    // Default: true
  };
  http: {
    enabled: boolean;    // Default: true
    host: string;        // Default: 127.0.0.1
    port: number;        // Default: 8080
  };
  logging: {
    level: "debug" | "info" | "warn" | "error";
    pretty: boolean;
  };
  dataDir: string;       // Default: ~/.a2a-hub
}
```

### 2. AgentRegistry

Manages agent lifecycle:
- Registration and deregistration
- Health monitoring (30-second intervals)
- Agent discovery via agent cards
- Circuit breaker pattern for unhealthy agents

### 3. TaskManager

Manages task lifecycle:
- Task creation and state transitions
- Task history tracking
- Concurrent task limits per agent
- Task timeout handling

### 4. ContextManager

Manages conversation contexts:
- Context creation and retrieval
- Task-to-context mapping
- Context expiration

### 5. JsonRpcHandler

Handles JSON-RPC 2.0 protocol:
- Method registration and dispatch
- Parameter validation (using Zod schemas)
- Error formatting per JSON-RPC spec

---

## Error Codes

Standard JSON-RPC 2.0 errors plus A2A-specific codes:

```typescript
const ErrorCodes = {
  // JSON-RPC standard errors
  PARSE_ERROR: -32700,
  INVALID_REQUEST: -32600,
  METHOD_NOT_FOUND: -32601,
  INVALID_PARAMS: -32602,
  INTERNAL_ERROR: -32603,

  // A2A specific errors
  TASK_NOT_FOUND: -32001,
  TASK_NOT_CANCELABLE: -32002,
  AGENT_NOT_FOUND: -32003,
  AGENT_UNAVAILABLE: -32004,
  UNSUPPORTED_OPERATION: -32005,
  AUTHENTICATION_ERROR: -32006,
  TIMEOUT_ERROR: -32007,
  CONTEXT_NOT_FOUND: -32008,
};
```

---

## CLI Commands

```bash
# Start the hub daemon
a2a-hub start [--foreground] [--http-port <port>] [--no-http]

# Stop the hub daemon
a2a-hub stop

# Check hub status
a2a-hub status

# List registered agents
a2a-hub agents [--health]

# Send a message to an agent
a2a-hub send <agent-id> "<message>" [--context <id>] [--timeout <ms>] [--stream]

# List tasks
a2a-hub tasks [--context <id>] [--state <state>] [--limit <n>]

# Global options
a2a-hub --socket <path>     # Unix socket path
a2a-hub --format <type>     # Output format: json, pretty
a2a-hub --verbose           # Enable debug logging
```

---

## File Structure

```
a2a-local-hub/
├── src/
│   ├── index.ts                     # Main exports
│   ├── types/
│   │   ├── index.ts                 # Type exports
│   │   ├── a2a.ts                   # A2A protocol types (Zod schemas)
│   │   └── jsonrpc.ts               # JSON-RPC types
│   ├── protocol/
│   │   ├── index.ts
│   │   ├── task-manager.ts          # Task lifecycle
│   │   ├── context-manager.ts       # Context tracking
│   │   └── jsonrpc-handler.ts       # RPC dispatch
│   ├── transport/
│   │   ├── index.ts
│   │   ├── base-transport.ts        # Transport interface
│   │   ├── unix-socket-transport.ts # Unix socket impl
│   │   └── http-transport.ts        # HTTP/SSE impl
│   ├── agents/
│   │   ├── index.ts
│   │   ├── base-agent.ts            # Agent interface
│   │   ├── claude-code-agent.ts     # Claude wrapper
│   │   ├── gemini-agent.ts          # Gemini wrapper
│   │   ├── codex-agent.ts           # Codex wrapper
│   │   └── vibe-agent.ts            # Vibe wrapper
│   ├── hub/
│   │   ├── index.ts
│   │   ├── hub-server.ts            # Main server
│   │   └── agent-registry.ts        # Agent management
│   ├── cli/
│   │   └── index.ts                 # CLI commands
│   └── utils/
│       ├── index.ts
│       ├── errors.ts                # Error classes
│       └── logger.ts                # Logging (Pino)
├── dist/                            # Compiled output
├── docs/
│   └── SPECIFICATION.md             # This file
├── package.json
├── tsconfig.json
└── tsup.config.ts
```

---

## Dependencies

### Runtime

- `express` - HTTP server
- `better-sse` - Server-Sent Events
- `zod` - Schema validation
- `uuid` - ID generation
- `pino` / `pino-pretty` - Logging
- `commander` - CLI framework

### Development

- `typescript` - Language
- `tsup` - Build tool
- `vitest` - Testing

---

## Implementation Notes for Other Languages

### Go Implementation Tips

1. **Transport**: Use `net.Listen("unix", socketPath)` for Unix sockets and `net/http` for HTTP.

2. **JSON-RPC**: Create a dispatcher map from method names to handler functions. Use `encoding/json` for marshaling.

3. **Process Spawning**: Use `os/exec` package. Remember to close stdin for non-interactive CLI modes:
   ```go
   cmd := exec.Command(executable, args...)
   stdin, _ := cmd.StdinPipe()
   stdin.Close() // Important!
   ```

4. **Concurrency**: Use goroutines for concurrent task execution and channels for communication.

5. **Health Monitoring**: Use `time.Ticker` for periodic health checks.

6. **Graceful Shutdown**: Handle `SIGINT` and `SIGTERM` with `os/signal` package.

### Key Patterns to Implement

1. **State Machine**: Implement task state transitions with validation.

2. **Circuit Breaker**: Mark agents unhealthy after consecutive failures.

3. **Request Timeout**: Use context.Context for cancellation.

4. **Connection Pooling**: Reuse connections for Unix socket clients.

5. **Event Emission**: Use channels or callback patterns for events.

---

## Example Flow

### Send Message Flow

```
1. Client sends JSON-RPC request to hub
   POST / {"jsonrpc":"2.0","method":"message/send","params":{...},"id":"1"}

2. Hub validates request using Zod schema

3. Hub identifies target agent from message.metadata.targetAgent

4. Hub checks agent health via AgentRegistry

5. Hub creates task via TaskManager
   - Generates task ID
   - Sets state to "submitted"

6. Hub creates/retrieves context via ContextManager

7. Hub calls agent.execute(context)
   - Agent spawns CLI process
   - Agent waits for output
   - Agent parses response

8. Hub updates task state to "completed"

9. Hub returns Task object to client
```

---

## Future Extensions

1. **Orchestration Patterns**:
   - Handoff: Transfer task between agents
   - Broadcast: Send to multiple agents
   - Parallel: Decompose and parallelize

2. **Persistence**: SQLite for task/context storage

3. **Authentication**: API keys, OAuth support

4. **Remote Agents**: Connect to agents over network

5. **MCP Bridge**: Integrate MCP-based tools
