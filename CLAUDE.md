# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Awayteam is a dashboard for observing and interacting with remote/background AI agent tasks. It displays agents on a Kanban board (Queued → Active → Waiting → Done) with full-screen conversation views and bidirectional I/O via PTY proxy. Single Go binary with embedded Next.js frontend, SQLite storage, no external dependencies.

## Build & Development Commands

```bash
make build          # Build frontend (Next.js export) + Go binary → ./awayteam
make run            # Build and run the server
make dev            # Run server with go run (no build step, uses last frontend build)
make frontend       # Rebuild frontend only (cd web && npm run build, copies to internal/frontend/dist)
make test           # Run Go tests: go test ./...
make clean          # Remove build artifacts
```

Frontend dev server (separate from Go server):
```bash
cd web && npm run dev    # Next.js dev server on :3000
cd web && npx eslint .   # Lint frontend
```

Run a single Go test:
```bash
go test ./internal/store/ -run TestSaveEvent
```

Prerequisites: Go 1.25+, Node.js 20+.

## Architecture

### Data Flow

Events flow one direction: **agent process → HTTP POST → Go server → SQLite + WebSocket broadcast → React UI**. User responses flow back: **React UI → agent-specific WebSocket → PTY proxy → agent's stdin**.

### CLI Commands (`cmd/awayteam/main.go`)

Four subcommands dispatched via `os.Args[1]`:
- `serve` — starts HTTP server with embedded frontend, SQLite store, and WebSocket hub
- `agent` — PTY proxy: wraps a child process, streams I/O to dashboard, receives dashboard responses. Creates a tmux session when tmux is available (fallback: direct PTY). Sets `AWAYTEAM_AGENT_ID`, `AWAYTEAM_AGENT_NAME`, `AWAYTEAM_SERVER_URL` env vars on the child
- `hook` — processes Claude Code hook payloads from stdin, POSTs events to server. Reads `AWAYTEAM_AGENT_ID`/`AWAYTEAM_SERVER_URL` from env
- `install` — prints JSON hook config for `~/.claude/settings.json`

### Go Backend (`internal/`)

| Package | Role |
|---------|------|
| `agent` | PTY proxy using `creack/pty`. Allocates pseudo-terminal, streams output (base64-encoded chunks), connects WebSocket for dashboard→agent responses. Creates tmux sessions when available, reports connection info (hostname, tmux session, SSH command) in `session.start` event |
| `config` | YAML config with env var overrides (`AWAYTEAM_PORT`, `AWAYTEAM_DB_PATH`) |
| `events` | Event type definitions and validation. Status enum: queued/active/waiting/done/error |
| `frontend` | `go:embed all:dist` — embeds the Next.js static export. Must run `make frontend` before `go build` |
| `hook` | Reads hook payload from stdin, wraps in event envelope, POSTs to server. Exits silently on server unreachable |
| `server` | HTTP handlers + WebSocket upgrade. Uses Go 1.22+ `http.ServeMux` method routing (`GET /path`, `POST /path`) |
| `store` | `Store` interface + SQLite implementation (WAL mode, `modernc.org/sqlite` pure-Go driver). Two tables: `events`, `agent_state` |
| `terminal` | WebSocket-to-PTY bridge for browser terminal. Handles tmux attach and general shell sessions |
| `ws` | WebSocket hub: broadcast to all clients, per-agent listener routing for responses |

### Frontend (`web/`)

Next.js 16 with `output: "export"` (static site, no SSR). Served by Go's `http.FileServerFS`.

| Path | Purpose |
|------|---------|
| `src/app/page.tsx` | Kanban board — groups agents by status into columns |
| `src/app/agent/page.tsx` | Agent detail view with Terminal/Chat tab bar. Terminal tab (default) shows xterm.js connected to tmux session; Chat tab shows structured event history |
| `src/store/agents.ts` | Zustand store — agent state, event buffering (500/agent, 100 recent), connection info from session.start events |
| `src/hooks/useWebSocket.ts` | WebSocket with exponential backoff reconnect (1s → 30s cap) |
| `src/lib/types.ts` | Shared TypeScript types: `DashboardEvent`, `AgentState`, `AgentStatus` |
| `src/components/` | `agent-card.tsx`, `conversation-view.tsx`, `layout-shell.tsx`, `response-input.tsx`, `terminal-view.tsx` (xterm.js terminal), `connection-info.tsx` (SSH/tmux connection panel) |

### Event Model

Events are JSON envelopes with `id`, `type`, `timestamp`, `agent_id`, `agent_name`, `agent_type`, `data` (raw JSON), and `status`. The `status` field drives Kanban column placement. Key event types: `session.start`, `session.end`, `message.user`, `message.assistant`, `tool.call`, `tool.result`, `question.asked`, `question.answered`, `output.stream`.

### Embedding Pipeline

The frontend must be built before the Go binary: `make frontend` runs `next build`, copies `web/out/` to `internal/frontend/dist/`, which is then embedded via `go:embed`. The `make build` target handles this automatically.

## API Routes

- `POST /api/v1/events` — ingest event (validates, saves to SQLite, broadcasts via WebSocket)
- `GET /api/v1/agents` — list all agents with current state
- `GET /api/v1/agents/:id/events?limit=200` — event history for an agent
- `GET /api/v1/ws` — global WebSocket (server → client broadcast)
- `GET /api/v1/ws/agents/:id` — bidirectional agent WebSocket (events down, responses up)
- `GET /api/v1/ws/terminal/:id` — agent terminal WebSocket (tmux attach or PTY fallback, binary frames)
- `GET /api/v1/ws/terminal` — general shell WebSocket (spawns login shell)
- `GET /healthz` — health check

## Key Conventions

- Go HTTP handlers use Go 1.22+ method-pattern routing (e.g., `mux.HandleFunc("GET /path", handler)`)
- SQLite uses pure-Go `modernc.org/sqlite` (no CGO required), WAL mode
- Frontend is a static export — no server-side rendering, no API routes in Next.js
- WebSocket connections use `gorilla/websocket` with permissive CORS (`CheckOrigin: true`)
- Hooks fail silently when the dashboard server is unreachable
