# Browser Terminal for Awayteam

## Problem

The dashboard chat view reconstructs agent interactions from structured hook events, but users want to see the real Claude Code TUI — colors, interactive prompts, full terminal experience. Users also need connection info to manually attach to remote agents.

## Solution

Replace the default agent view with an xterm.js terminal that attaches to tmux sessions, with the existing chat view as a secondary tab.

## Architecture

### Agent-side tmux integration

When `awayteam agent` starts:

1. Check for tmux via `tmux -V`
2. If available: create named tmux session `awayteam-<agent_id_short>`, run child process inside it via `tmux new-session -d -s <name> -- <command>`. Proxy attaches to the session's PTY for output streaming.
3. If unavailable: current direct-PTY behavior (no change)
4. Report connection info in `session.start` event data:
   - `tmux_session`: session name (or null)
   - `hostname`: machine hostname
   - `username`: current OS user
   - `ssh_command`: formatted `ssh <user>@<hostname>`
5. On `session.end`: kill tmux session if one was created

Existing PTY output streaming continues unchanged as the fallback data source.

### Server-side terminal WebSocket

New endpoint: `GET /api/v1/ws/terminal/{agent_id}`

1. Browser connects via WebSocket
2. Server looks up agent's `tmux_session` from session.start event data
3. If tmux session exists and is local: spawn `tmux attach-session -t <name>` in a new PTY. Pipe PTY I/O bidirectionally over WebSocket as binary frames. Multiple browsers each get their own tmux attach.
4. If no tmux session: replay stored `output.stream` events, then live-stream new ones. Input routed through `hub.RouteResponse()`.
5. On disconnect: kill spawned tmux-attach process and close PTY.

General shell endpoint: `GET /api/v1/ws/terminal` (no agent_id)
- Spawns login shell in a PTY
- User can SSH to remote machines from here

Terminal resize: JSON text frame `{"type": "resize", "cols": N, "rows": N}`. All other browser→server messages are binary (raw keystrokes). Server→browser is binary (raw PTY output).

### Frontend

Dependencies: `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-web-links`

Agent page changes:
- Tab bar: **Terminal** (default) | **Chat**
- Chat tab = existing ConversationView + ResponseInput (unchanged)
- Tab in URL param `&view=terminal` / `&view=chat`

TerminalView component:
- xterm.js instance
- WebSocket to `/api/v1/ws/terminal/{agent_id}`
- Binary WS → `terminal.write()`, `terminal.onData()` → binary WS
- Resize via addon-fit → JSON resize message
- Status: "Connected to tmux session" or "Streaming PTY output"

Connection info panel:
- Above terminal, collapsible
- Shows hostname, tmux session name, copyable SSH+tmux command
- "Open Shell" button → general terminal at `/api/v1/ws/terminal`

### Data flows

Tmux mode (primary):
```
xterm.js keys → binary WS → server → tmux-attach PTY → tmux → child stdin
child stdout → tmux → tmux-attach PTY → server → binary WS → xterm.js
```

PTY fallback:
```
xterm.js keys → binary WS → server → hub.RouteResponse → agent proxy → PTY
PTY output → agent proxy → output.stream event → server broadcast → binary WS → xterm.js
```

### Edge cases

- Agent not running: WS close with reason, show "Agent not active" + connection info
- Tmux session dies: PTY EOF detected, WS close, show reconnect option
- Multiple tabs: each gets own tmux attach (native tmux behavior)
- Remote agent: no tmux attach, show connection info, user can open general shell and SSH

### File changes

New Go:
- `internal/terminal/handler.go` — terminal WS handler, PTY management

Modified Go:
- `internal/agent/proxy.go` — tmux detection, session creation, connection info
- `internal/server/server.go` — new routes
- `internal/store/store.go` — helper to retrieve agent session.start data

New frontend:
- `web/src/components/terminal-view.tsx`
- `web/src/components/connection-info.tsx`

Modified frontend:
- `web/src/app/agent/page.tsx` — tab bar
- `web/package.json` — xterm.js dependencies

New API routes:
- `GET /api/v1/ws/terminal/{id}`
- `GET /api/v1/ws/terminal`
