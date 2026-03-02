# Awayteam

A general-purpose dashboard for observing and interacting with remote/background AI agent tasks. Displays agents on a Kanban swimlane board (Queued → Active → Waiting → Done) with full-screen conversation views. Bidirectional integration with Claude Code via PTY proxy — respond to agent questions from the web UI.

```
┌─────────────────────────────────────────────────────────────┐
│                    Dashboard UI (Next.js)                    │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────────┐  │
│  │   Kanban     │  │   Agent      │  │   Conversation    │  │
│  │   Board      │  │   Cards      │  │   Page + Reply    │  │
│  └──────▲──────┘  └──────▲───────┘  └─────▲──────│──────┘  │
│         └────────WebSocket─────────────────┘      │         │
└───────────────────────────────────────────────────│─────────┘
                                                    │ user reply
                                                    ▼
┌─────────────────────────────────────────────────────────────┐
│                  Dashboard Server (Go :8080)                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
│  │  Event   │  │  WS Hub  │  │ Response │  │  SQLite   │  │
│  │  Router  │  │ (fanout) │  │  Queue   │  │  Store    │  │
│  └────▲─────┘  └──────────┘  └────│─────┘  └───────────┘  │
└───────│────────────────────────────│────────────────────────┘
        │ POST /api/v1/events       │ response via WS
        │                           ▼
┌───────│───────────────────────────────────────┐
│       │        awayteam agent claude [args]    │
│  ┌────┴─────┐          ┌─────────────────┐   │
│  │  Hooks   │          │   PTY Proxy     │   │
│  │(struct'd │          │ ┌─────────────┐ │   │
│  │ events)  │          │ │ Claude Code │ │   │
│  └──────────┘          │ └─────────────┘ │   │
│                        │  stdin ← terminal   │
│                        │  stdin ← dashboard  │
│                        │  stdout → both      │
│                        └─────────────────────┘│
└───────────────────────────────────────────────┘
```

Single Go binary. Embedded frontend. SQLite storage. No external dependencies.

## Quick Start

### From Source

```bash
# Prerequisites: Go 1.25+, Node.js 20+
make build
./awayteam serve
```

Dashboard is at http://localhost:8080.

### Docker

```bash
docker run -d \
  --name awayteam \
  -p 8080:8080 \
  -v awayteam-data:/data \
  ghcr.io/fireynis/awayteam:latest
```

### Docker Compose

```yaml
services:
  awayteam:
    image: ghcr.io/fireynis/awayteam:latest
    ports:
      - "8080:8080"
    volumes:
      - awayteam-data:/data
    restart: unless-stopped

volumes:
  awayteam-data:
```

## Usage

### Running an Agent

Wrap any command with `awayteam agent` to stream its I/O to the dashboard:

```bash
# Generic command
awayteam agent --name "backup" ./scripts/backup.sh

# Claude Code with PTY proxy (recommended)
awayteam agent --name "feature-x" --type claude-code \
  claude --dangerously-skip-permissions
```

The PTY proxy allocates a pseudo-terminal, streams output to both your local terminal and the dashboard, and routes responses from the web UI back to the process's stdin. Either terminal or dashboard can provide input — first one wins.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | command name | Agent name shown in dashboard |
| `--type` | `generic` | Agent type label |
| `--server` | `http://localhost:8080` | Dashboard server URL |

### Claude Code Integration

#### Option A: PTY Proxy (recommended)

```bash
awayteam agent --name "feature-x" claude --dangerously-skip-permissions
```

This wraps Claude Code in a PTY proxy. The proxy sets these environment variables on the child process so hooks can correlate events:

| Variable | Description |
|----------|-------------|
| `AWAYTEAM_AGENT_ID` | UUID for this agent session |
| `AWAYTEAM_AGENT_NAME` | Agent name |
| `AWAYTEAM_SERVER_URL` | Dashboard server URL |

#### Option B: Hooks Only

Install hooks to send structured events without the PTY proxy:

```bash
awayteam install claude-code
```

This prints JSON to add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      { "type": "command", "command": "/path/to/awayteam hook post-tool-use" }
    ],
    "Notification": [
      { "type": "command", "command": "/path/to/awayteam hook notification" }
    ]
  }
}
```

Hooks read the payload from stdin, wrap it in an event envelope, and POST to the dashboard. They exit silently if the server is unreachable.

### Serving the Dashboard

```bash
awayteam serve
awayteam serve -config config.yaml
```

## Configuration

### Config File (YAML)

```yaml
server:
  port: 8080

storage:
  sqlite_path: ./awayteam.db
```

### Environment Variables

Environment variables override config file values:

| Variable | Config Key | Default |
|----------|-----------|---------|
| `AWAYTEAM_PORT` | `server.port` | `8080` |
| `AWAYTEAM_DB_PATH` | `storage.sqlite_path` | `./awayteam.db` |

## Dashboard UI

### Kanban Board (`/`)

Four columns: **Queued** → **Active** → **Waiting** → **Done**.

Each agent card shows name, type, current status, and elapsed time. Cards in the "Waiting" column pulse to indicate the agent needs input. Browser notifications fire when a `question.asked` event arrives.

Click any card to open the full conversation view.

### Conversation Page (`/agent/?id=...`)

Full-screen conversation view showing:

- User messages as blue chat bubbles
- Agent messages as gray chat bubbles
- Tool calls and results as collapsible blocks
- Questions highlighted in amber with response options
- Text input at the bottom for sending responses

Responses typed in the web UI are routed through the WebSocket hub to the PTY proxy and written to the agent's stdin.

## Event Model

Any process that can POST JSON can participate. The event envelope:

```json
{
  "id": "uuid",
  "type": "message.assistant",
  "timestamp": "2026-03-02T10:00:00Z",
  "agent_id": "uuid",
  "agent_type": "claude-code",
  "agent_name": "feature-x",
  "data": { "content": "I'll implement that now." },
  "status": "active"
}
```

The `status` field determines the Kanban column: `queued`, `active`, `waiting`, `done`, `error`.

### Event Types

| Type | When | Data |
|------|------|------|
| `session.start` | Agent begins | `{command, cwd, args}` |
| `session.end` | Agent exits | `{exit_code, reason}` |
| `message.user` | User sends prompt | `{content}` |
| `message.assistant` | Agent responds | `{content}` |
| `tool.call` | Tool invoked | `{tool, params}` |
| `tool.result` | Tool completes | `{tool, result, duration_ms}` |
| `question.asked` | Agent needs input | `{question, options[], question_id}` |
| `question.answered` | User responds | `{question_id, answer}` |
| `output.stream` | Raw terminal output | `{chunk}` (base64) |
| `error` | Something went wrong | `{message, severity}` |

## API

### REST

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check |
| `POST` | `/api/v1/events` | Ingest an event |
| `GET` | `/api/v1/agents` | List active agents |
| `GET` | `/api/v1/agents/:id/events?limit=200` | Event history for agent |

### WebSocket

| Path | Direction | Description |
|------|-----------|-------------|
| `/api/v1/ws` | Server → Client | Live stream of all events |
| `/api/v1/ws/agents/:id` | Bidirectional | Agent-specific events + response channel |

The agent-specific WebSocket is bidirectional: the server pushes events, the client sends responses that route to the PTY proxy.

## Development

### Prerequisites

- Go 1.25+
- Node.js 20+

### Build Commands

```bash
make build      # Build frontend + Go binary
make run        # Build and run
make dev        # Run with go run (no build step)
make frontend   # Rebuild frontend only
make test       # Run Go tests
make clean      # Remove build artifacts
```

### Project Structure

```
cmd/awayteam/main.go          CLI entry point (serve, agent, hook, install)
internal/
  agent/proxy.go               PTY proxy + WebSocket client
  config/config.go             YAML + env var configuration
  events/events.go             Event types and validation
  frontend/embed.go            go:embed for static frontend
  hook/hook.go                 Claude Code hook processor
  server/server.go             HTTP server + WebSocket upgrade
  server/handlers.go           REST API handlers
  server/middleware.go          CORS middleware
  store/store.go               Store interface
  store/sqlite.go              SQLite implementation (WAL mode)
  ws/hub.go                    WebSocket broadcast hub
web/
  src/app/                     Next.js app router pages
  src/components/              React components (Kanban, conversation, input)
  src/hooks/useWebSocket.ts    WebSocket with exponential backoff reconnect
  src/store/agents.ts          Zustand state management
  src/lib/types.ts             TypeScript type definitions
```

### Tech Stack

| Component | Technology |
|-----------|-----------|
| CLI + Server | Go 1.25, `creack/pty`, `gorilla/websocket`, `modernc.org/sqlite` |
| Frontend | Next.js 16, React 19, Zustand 5, Tailwind CSS 4 |
| Storage | SQLite (WAL mode) |
| Fonts | Geist, Geist Mono |

## Docker

### Build Locally

```bash
docker build -t awayteam .
docker run -p 8080:8080 -v awayteam-data:/data awayteam
```

### GitHub Container Registry

Images are published automatically on push to `main`:

```bash
docker pull ghcr.io/fireynis/awayteam:latest
```

Available tags:
- `latest` — latest build from main
- `main` — same as latest
- `sha-<commit>` — pinned to specific commit

## License

MIT
