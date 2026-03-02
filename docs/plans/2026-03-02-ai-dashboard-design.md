# AI Dashboard Design

A general-purpose dashboard for observing and interacting with remote/background AI agent tasks. Displays agents on a Kanban swimlane board (Queued → Active → Waiting → Done) with full-screen conversation views. Bidirectional integration with Claude Code via PTY proxy — respond to agent questions from the web UI.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Dashboard UI (Next.js)                    │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────────┐  │
│  │   Kanban     │  │   Agent      │  │   Conversation    │  │
│  │   Swimlane   │  │   Cards      │  │   Page (full)     │  │
│  │   Board      │  │   (status)   │  │   + Reply input   │  │
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
│       │        aid agent claude [args]         │
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

Three deliverables:

1. **`aid` CLI** — single Go binary: `serve`, `agent`, `install`, `hook` subcommands
2. **Dashboard Server** — event ingestion, WebSocket hub, response queue, SQLite storage
3. **Dashboard UI** — Next.js Kanban board + full-screen conversation pages

## Event Model

Generic event envelope. Any process that can POST JSON can participate.

```go
type Event struct {
    ID        string    `json:"id"`         // UUID
    Type      string    `json:"type"`       // Hierarchical event type
    Timestamp time.Time `json:"timestamp"`
    AgentID   string    `json:"agent_id"`   // Unique session identifier
    AgentType string    `json:"agent_type"` // "claude-code", "script", "ci", etc.
    AgentName string    `json:"agent_name"` // Human-readable label
    Data      any       `json:"data"`       // Type-specific payload
    Status    string    `json:"status"`     // "queued", "active", "waiting", "done", "error"
}
```

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
| `task.start` | Task claimed | `{task_id, description}` |
| `task.complete` | Task finished | `{task_id, result}` |
| `output.stream` | Raw terminal output | `{chunk}` (base64 PTY output) |
| `error` | Something went wrong | `{message, severity}` |

The `status` field on every event tells the Kanban board which column the agent belongs in. A `question.asked` event sets status to `"waiting"`.

## UI Design

### Kanban Board (main page `/`)

Four columns: Queued → Active → Waiting (needs you) → Done.

Each agent card shows:
- Agent name + type icon
- Current activity (last meaningful event summary)
- Elapsed time
- For "Waiting" column: question preview

"Waiting" cards pulse/glow. Browser notifications fire when a question arrives.

Clicking a card navigates to `/agents/:id`.

### Conversation Page (`/agents/:id`)

Full-screen conversation view:
- Messages rendered as chat bubbles (user, assistant)
- Tool calls as collapsible blocks with name, params, result
- Questions highlighted with response options/buttons
- Free-text response input at bottom
- Typing a response → sent to agent via PTY proxy → dashboard WS → `aid agent` → PTY stdin
- Back button returns to Kanban

### Sessions (`/sessions`)

Historical list of completed agent sessions. Click to view conversation replay.

## PTY Proxy (`aid agent`)

```
$ aid agent --name "feature-x" claude --dangerously-skip-permissions
```

1. Registers agent with dashboard server: `POST /api/v1/agents` → receives `agent_id`
2. Creates PTY, spawns target process as child
3. Event loop:
   - PTY stdout → local terminal (passthrough) + dashboard WS (`output.stream`)
   - Local stdin → PTY stdin (passthrough)
   - Dashboard WS "response" → PTY stdin (user typed in web UI)
4. Claude Code hooks fire independently, POSTing structured events to dashboard
5. On exit: sends `session.end` event, deregisters

Either terminal or dashboard can provide input — first one wins. If dashboard server unreachable, degrades gracefully (Claude works normally in terminal).

For generic agents: `aid agent --name "backup" ./scripts/backup.sh` — same PTY proxy, no hooks.

## Claude Code Integration

### Hook Installation

```bash
$ aid install claude-code
```

Adds hooks to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [{"command": "aid hook post-tool-use"}],
    "Notification": [{"command": "aid hook notification"}]
  }
}
```

`aid hook <type>` reads hook payload from stdin, extracts structured data, POSTs to dashboard. Exits silently if server is down.

### What hooks capture

- **PostToolUse**: tool name, parameters, result → `tool.call` + `tool.result` events
- **Notification**: assistant messages → `message.assistant` events
- **UserPromptSubmit**: user prompts → `message.user` events

## API Endpoints

### Event Ingestion
- `POST /api/v1/events` — ingest event, store, broadcast via WS
- `POST /api/v1/agents` — register a new agent session

### Queries
- `GET /api/v1/agents` — list active agents with current status
- `GET /api/v1/agents/:id` — agent detail + recent events
- `GET /api/v1/agents/:id/events` — full event history for agent
- `GET /api/v1/sessions` — paginated list of all sessions
- `GET /api/v1/sessions/:id` — session detail with full event timeline

### WebSocket
- `GET /api/v1/ws` — live event stream (all agents)
- `GET /api/v1/ws/agents/:id` — live stream for specific agent + response channel

The agent-specific WS is bidirectional: server pushes events, client sends responses (which route to the PTY proxy via the response queue).

## Tech Stack

| Component | Technology |
|-----------|-----------|
| CLI + Server | Go 1.25, `creack/pty`, `gorilla/websocket`, `modernc.org/sqlite` |
| Frontend | Next.js 16, React 19, Zustand, Tailwind CSS 4, dark theme |
| Storage | SQLite (WAL mode) |
| Fonts | Geist, Geist Mono |

## Non-Goals (v1)

- Multi-user auth (single-user local dashboard)
- Remote deployment (runs on localhost)
- Webhook dispatch to external services
- PostgreSQL support (SQLite only for v1)
