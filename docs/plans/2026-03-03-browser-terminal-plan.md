# Browser Terminal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an xterm.js terminal to the dashboard that attaches to tmux sessions for each agent, with the existing chat view as a fallback tab.

**Architecture:** `awayteam agent` creates a tmux session for each child process (when tmux is available). The Go server exposes a new WebSocket endpoint that spawns `tmux attach` in a PTY and bridges I/O to the browser. The frontend uses xterm.js to render the terminal, with a tab to switch back to the existing chat view.

**Tech Stack:** Go (creack/pty, gorilla/websocket), tmux, xterm.js (@xterm/xterm, @xterm/addon-fit, @xterm/addon-web-links), Next.js/React/TypeScript

---

### Task 1: Agent-side tmux detection and session creation

**Files:**
- Modify: `internal/agent/proxy.go:26-136`

**Step 1: Add tmux helper functions**

Add these functions after the `toWSURL` function (after line 177) in `internal/agent/proxy.go`:

```go
// hasTmux checks if tmux is available on the system.
func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// tmuxSessionName returns a short, deterministic tmux session name for an agent.
func tmuxSessionName(agentID string) string {
	short := agentID
	if len(short) > 8 {
		short = short[:8]
	}
	return "awayteam-" + short
}

// startTmuxSession creates a new tmux session running the given command.
// Returns the session name, or empty string if tmux is not available or fails.
func startTmuxSession(name string, cmd string, args []string, env []string) (string, error) {
	tmuxArgs := []string{"new-session", "-d", "-s", name, "-x", "200", "-y", "50", "--"}
	tmuxArgs = append(tmuxArgs, cmd)
	tmuxArgs = append(tmuxArgs, args...)

	tmuxCmd := exec.Command("tmux", tmuxArgs...)
	tmuxCmd.Env = env
	if err := tmuxCmd.Run(); err != nil {
		return "", fmt.Errorf("tmux new-session: %w", err)
	}
	return name, nil
}

// killTmuxSession kills a tmux session by name. Errors are ignored.
func killTmuxSession(name string) {
	exec.Command("tmux", "kill-session", "-t", name).Run()
}
```

**Step 2: Add connection info to ProxyConfig and session.start event**

Add `os/user` to the imports. Modify `RunProxy` to detect tmux, create the session, populate connection info, and change the PTY attachment approach. The key changes:

In `RunProxy`, after generating `agentID` (line 35), before creating the command (line 37):

```go
func RunProxy(cfg ProxyConfig) error {
	agentID := uuid.NewString()
	sessionName := tmuxSessionName(agentID)

	childEnv := append(os.Environ(),
		"AWAYTEAM_AGENT_ID="+agentID,
		"AWAYTEAM_AGENT_NAME="+cfg.Name,
		"AWAYTEAM_SERVER_URL="+cfg.ServerURL,
	)

	var tmuxUsed bool
	if hasTmux() {
		if _, err := startTmuxSession(sessionName, cfg.Command, cfg.Args, childEnv); err != nil {
			log.Printf("warning: tmux session failed, falling back to direct PTY: %v", err)
		} else {
			tmuxUsed = true
			defer killTmuxSession(sessionName)
		}
	}

	// Build connection info for session.start event
	hostname, _ := os.Hostname()
	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	connInfo := map[string]any{
		"hostname": hostname,
		"username": username,
	}
	if tmuxUsed {
		connInfo["tmux_session"] = sessionName
		connInfo["ssh_command"] = fmt.Sprintf("ssh %s@%s", username, hostname)
		connInfo["tmux_command"] = fmt.Sprintf("tmux attach -t %s", sessionName)
	}

	var cmd *exec.Cmd
	var ptmx *os.File
	var err error

	if tmuxUsed {
		// Attach to the tmux session we just created
		cmd = exec.Command("tmux", "attach-session", "-t", sessionName)
		ptmx, err = pty.Start(cmd)
	} else {
		cmd = exec.Command(cfg.Command, cfg.Args...)
		cmd.Env = childEnv
		ptmx, err = pty.Start(cmd)
	}
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}
	defer ptmx.Close()
```

Update the `session.start` event to include connection info:

```go
	startData, _ := json.Marshal(connInfo)
	postEvent(cfg.ServerURL, newEvent(agentID, cfg.Name, cfg.AgentType, "session.start", "active", startData))
```

The rest of RunProxy (SIGWINCH, raw mode, WS connection, I/O loop, cleanup) stays exactly the same.

**Step 3: Run `go build ./...` to verify compilation**

Run: `go build ./...`
Expected: compiles cleanly

**Step 4: Commit**

```bash
git add internal/agent/proxy.go
git commit -m "feat(agent): tmux session creation with connection info"
```

---

### Task 2: Store helper to retrieve agent connection info

**Files:**
- Modify: `internal/store/store.go:17-22`
- Modify: `internal/store/sqlite.go:121-143`

**Step 1: Add GetAgentSessionData to the Store interface**

In `internal/store/store.go`, add a new method to the `Store` interface:

```go
type Store interface {
	SaveEvent(ctx context.Context, event events.Event) error
	GetAgents(ctx context.Context) ([]AgentState, error)
	GetAgentEvents(ctx context.Context, agentID string, limit int) ([]events.Event, error)
	GetAgentSessionData(ctx context.Context, agentID string) (json.RawMessage, error)
	Close() error
}
```

Add `"encoding/json"` to the imports.

**Step 2: Implement GetAgentSessionData in SQLiteStore**

Add to `internal/store/sqlite.go` after `GetAgentEvents`:

```go
func (s *SQLiteStore) GetAgentSessionData(ctx context.Context, agentID string) (json.RawMessage, error) {
	var dataJSON string
	err := s.db.QueryRowContext(ctx,
		`SELECT data_json FROM events
		 WHERE agent_id = ? AND type = 'session.start'
		 ORDER BY timestamp DESC LIMIT 1`,
		agentID).Scan(&dataJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return json.RawMessage(dataJSON), nil
}
```

**Step 3: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly

**Step 4: Commit**

```bash
git add internal/store/store.go internal/store/sqlite.go
git commit -m "feat(store): add GetAgentSessionData for connection info lookup"
```

---

### Task 3: Terminal WebSocket handler

**Files:**
- Create: `internal/terminal/handler.go`

**Step 1: Create the terminal package**

Create `internal/terminal/handler.go`:

```go
package terminal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type Handler struct {
	upgrader websocket.Upgrader
}

func NewHandler() *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// HandleTerminal serves a terminal WebSocket for a specific command.
// If tmuxSession is non-empty, it attaches to that tmux session.
// If tmuxSession is empty and shell is true, it spawns a login shell.
// The fallback (streaming PTY output) is handled separately by the caller.
func (h *Handler) ServeWebSocket(w http.ResponseWriter, r *http.Request, tmuxSession string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var cmd *exec.Cmd
	if tmuxSession != "" {
		cmd = exec.Command("tmux", "attach-session", "-t", tmuxSession)
	} else {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
		cmd = exec.Command(shell)
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("terminal: failed to start pty: %v", err)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "failed to start terminal"))
		return
	}
	defer ptmx.Close()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			ptmx.Close()
			if cmd.Process != nil {
				cmd.Process.Signal(syscall.SIGHUP)
			}
		})
	}
	defer cleanup()

	// PTY -> WebSocket (binary frames)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		cleanup()
		conn.Close()
	}()

	// WebSocket -> PTY
	// Text frames with {"type":"resize"} are resize commands.
	// Binary frames are raw terminal input.
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if msgType == websocket.TextMessage {
			var rm resizeMsg
			if json.Unmarshal(msg, &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
				pty.Setsize(ptmx, &pty.Winsize{Cols: rm.Cols, Rows: rm.Rows})
			}
			continue
		}

		if msgType == websocket.BinaryMessage {
			if _, err := ptmx.Write(msg); err != nil {
				if err == io.EOF {
					break
				}
			}
		}
	}

	cleanup()
	cmd.Wait()
}
```

**Step 2: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly

**Step 3: Commit**

```bash
git add internal/terminal/handler.go
git commit -m "feat(terminal): WebSocket-to-PTY bridge for tmux attach and shell"
```

---

### Task 4: Wire terminal endpoints into server

**Files:**
- Modify: `internal/server/server.go:1-56`
- Modify: `internal/server/handlers.go`

**Step 1: Add terminal handler to Server struct**

In `internal/server/server.go`, add the import and field:

```go
import (
	// ... existing imports ...
	"github.com/jeremy/awayteam/internal/terminal"
)

type Server struct {
	store      store.Store
	hub        *ws.Hub
	config     config.Config
	upgrader   websocket.Upgrader
	frontendFS fs.FS
	terminal   *terminal.Handler
}

func New(cfg config.Config, st store.Store, h *ws.Hub) *Server {
	return &Server{
		store:    st,
		hub:      h,
		config:   cfg,
		terminal: terminal.NewHandler(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}
```

Add routes to `Handler()`, before the frontend file server:

```go
	mux.HandleFunc("GET /api/v1/ws/terminal/{id}", s.handleTerminalWS)
	mux.HandleFunc("GET /api/v1/ws/terminal", s.handleShellWS)
```

**Step 2: Add terminal handler methods to handlers.go**

Add to `internal/server/handlers.go`:

```go
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}

	// Look up tmux session from the agent's session.start event
	sessionData, err := s.store.GetAgentSessionData(r.Context(), agentID)
	if err != nil {
		http.Error(w, "failed to look up agent", http.StatusInternalServerError)
		return
	}

	var tmuxSession string
	if sessionData != nil {
		var info struct {
			TmuxSession string `json:"tmux_session"`
		}
		if json.Unmarshal(sessionData, &info) == nil {
			tmuxSession = info.TmuxSession
		}
	}

	s.terminal.ServeWebSocket(w, r, tmuxSession)
}

func (s *Server) handleShellWS(w http.ResponseWriter, r *http.Request) {
	s.terminal.ServeWebSocket(w, r, "")
}
```

**Step 3: Verify compilation**

Run: `go build ./...`
Expected: compiles cleanly

**Step 4: Commit**

```bash
git add internal/server/server.go internal/server/handlers.go
git commit -m "feat(server): wire terminal WebSocket endpoints"
```

---

### Task 5: Install frontend xterm.js dependencies

**Files:**
- Modify: `web/package.json`

**Step 1: Install xterm.js packages**

Run from `web/` directory:

```bash
cd web && npm install @xterm/xterm @xterm/addon-fit @xterm/addon-web-links
```

**Step 2: Verify it builds**

```bash
cd web && npm run build
```

**Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "feat(web): add xterm.js dependencies"
```

---

### Task 6: Create TerminalView component

**Files:**
- Create: `web/src/components/terminal-view.tsx`

**Step 1: Create the component**

Create `web/src/components/terminal-view.tsx`:

```tsx
'use client';

import { useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';

interface TerminalViewProps {
  agentId: string;
  tmuxSession?: string | null;
}

export function TerminalView({ agentId, tmuxSession }: TerminalViewProps) {
  const termRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected'>('connecting');

  useEffect(() => {
    if (!termRef.current) return;

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
      theme: {
        background: '#0d1117',
        foreground: '#c9d1d9',
        cursor: '#58a6ff',
        selectionBackground: '#264f78',
      },
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    terminal.loadAddon(fitAddon);
    terminal.loadAddon(webLinksAddon);
    terminal.open(termRef.current);
    fitAddon.fit();

    terminalRef.current = terminal;
    fitAddonRef.current = fitAddon;

    // WebSocket connection
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/api/v1/ws/terminal/${agentId}`;

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus('connected');
      // Send initial size
      const dims = fitAddon.proposeDimensions();
      if (dims) {
        ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
      }
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        terminal.write(new Uint8Array(event.data));
      }
    };

    ws.onclose = () => setStatus('disconnected');
    ws.onerror = () => ws.close();

    // Terminal input -> WebSocket
    terminal.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        const encoder = new TextEncoder();
        ws.send(encoder.encode(data));
      }
    });

    // Handle resize
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
      const dims = fitAddon.proposeDimensions();
      if (dims && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
      }
    });
    resizeObserver.observe(termRef.current);

    return () => {
      resizeObserver.disconnect();
      ws.close();
      terminal.dispose();
    };
  }, [agentId]);

  const statusColor = {
    connecting: 'text-yellow-400',
    connected: 'text-emerald-400',
    disconnected: 'text-red-400',
  }[status];

  const statusDot = {
    connecting: 'bg-yellow-500',
    connected: 'bg-emerald-500',
    disconnected: 'bg-red-500',
  }[status];

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-4 py-2 bg-gray-900/50 border-b border-gray-800 text-xs">
        <span className={`h-2 w-2 rounded-full ${statusDot}`} />
        <span className={statusColor}>
          {status === 'connected' && tmuxSession
            ? `tmux: ${tmuxSession}`
            : status === 'connected'
              ? 'PTY stream'
              : status}
        </span>
      </div>
      <div ref={termRef} className="flex-1 bg-[#0d1117]" />
    </div>
  );
}
```

**Step 2: Verify frontend builds**

```bash
cd web && npm run build
```

**Step 3: Commit**

```bash
git add web/src/components/terminal-view.tsx
git commit -m "feat(web): add TerminalView component with xterm.js"
```

---

### Task 7: Create ConnectionInfo component

**Files:**
- Create: `web/src/components/connection-info.tsx`

**Step 1: Create the component**

Create `web/src/components/connection-info.tsx`:

```tsx
'use client';

import { useState } from 'react';

interface ConnectionInfoProps {
  hostname?: string;
  username?: string;
  tmuxSession?: string;
  sshCommand?: string;
  tmuxCommand?: string;
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <button
      onClick={handleCopy}
      className="text-xs text-gray-400 hover:text-white px-2 py-0.5 rounded bg-gray-700 hover:bg-gray-600"
    >
      {copied ? 'Copied' : 'Copy'}
    </button>
  );
}

export function ConnectionInfo({ hostname, username, tmuxSession, sshCommand, tmuxCommand }: ConnectionInfoProps) {
  const [expanded, setExpanded] = useState(false);

  if (!hostname && !tmuxSession) return null;

  return (
    <div className="border-b border-gray-800 bg-gray-900/30">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-4 py-2 text-xs text-gray-400 hover:text-gray-200"
      >
        <span className="font-mono">{expanded ? '[-]' : '[+]'}</span>
        <span>Connection Info</span>
        {tmuxSession && (
          <span className="text-emerald-500 font-mono">{tmuxSession}</span>
        )}
        {hostname && (
          <span className="text-gray-500">@{hostname}</span>
        )}
      </button>

      {expanded && (
        <div className="px-4 pb-3 space-y-2">
          {tmuxSession && (
            <div className="flex items-center gap-2">
              <span className="text-xs text-gray-500 w-20">tmux:</span>
              <code className="text-xs text-gray-300 font-mono flex-1">{tmuxCommand ?? `tmux attach -t ${tmuxSession}`}</code>
              <CopyButton text={tmuxCommand ?? `tmux attach -t ${tmuxSession}`} />
            </div>
          )}
          {sshCommand && (
            <div className="flex items-center gap-2">
              <span className="text-xs text-gray-500 w-20">SSH:</span>
              <code className="text-xs text-gray-300 font-mono flex-1">{sshCommand}</code>
              <CopyButton text={sshCommand} />
            </div>
          )}
          {sshCommand && tmuxCommand && (
            <div className="flex items-center gap-2">
              <span className="text-xs text-gray-500 w-20">Full:</span>
              <code className="text-xs text-gray-300 font-mono flex-1">{sshCommand} -t {tmuxCommand}</code>
              <CopyButton text={`${sshCommand} -t '${tmuxCommand}'`} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
```

**Step 2: Verify frontend builds**

```bash
cd web && npm run build
```

**Step 3: Commit**

```bash
git add web/src/components/connection-info.tsx
git commit -m "feat(web): add ConnectionInfo component with copy buttons"
```

---

### Task 8: Update agent page with tab bar

**Files:**
- Modify: `web/src/app/agent/page.tsx:1-59`
- Modify: `web/src/store/agents.ts` (to store connection info from session.start events)

**Step 1: Update Zustand store to track connection info**

In `web/src/store/agents.ts`, update `AgentState` handling in `handleEvent` to extract connection info from session.start events. Add a new map to the store:

```ts
interface AgentStore {
  agents: Map<string, AgentState>;
  agentEvents: Map<string, DashboardEvent[]>;
  agentConnectionInfo: Map<string, Record<string, string>>;
  recentEvents: DashboardEvent[];
  setAgents: (agents: AgentState[]) => void;
  handleEvent: (event: DashboardEvent) => void;
}
```

Initialize `agentConnectionInfo: new Map()` in the store. In `handleEvent`, add after the agents.set() call:

```ts
      // Extract connection info from session.start events
      if (event.type === 'session.start' && event.data) {
        const connInfo = new Map(state.agentConnectionInfo);
        connInfo.set(event.agent_id, event.data as Record<string, string>);
        // include agentConnectionInfo in the return
      }
```

**Step 2: Update agent page with tabs**

Replace `web/src/app/agent/page.tsx` content:

```tsx
'use client';

import { useSearchParams } from 'next/navigation';
import { Suspense, useState } from 'react';
import { useAgentStore } from '@/store/agents';
import { ConversationView } from '@/components/conversation-view';
import { ResponseInput } from '@/components/response-input';
import { TerminalView } from '@/components/terminal-view';
import { ConnectionInfo } from '@/components/connection-info';
import Link from 'next/link';

function AgentPageContent() {
  const searchParams = useSearchParams();
  const agentId = searchParams.get('id') ?? '';
  const [activeTab, setActiveTab] = useState<'terminal' | 'chat'>('terminal');

  const agent = useAgentStore((s) => s.agents.get(agentId));
  const events = useAgentStore((s) => s.agentEvents.get(agentId) ?? []);
  const connectionInfo = useAgentStore((s) => s.agentConnectionInfo.get(agentId));

  if (!agentId) {
    return (
      <div className="text-gray-500 text-center py-12">
        No agent ID specified. <Link href="/" className="text-blue-400 hover:underline">Go back</Link>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-[calc(100vh-73px)]">
      <div className="border-b border-gray-800 px-6 py-4 flex items-center gap-4">
        <Link href="/" className="text-gray-400 hover:text-white text-sm">
          &larr; Back
        </Link>
        <h1 className="text-xl font-bold">
          {agent?.agent_name || agentId.slice(0, 8)}
        </h1>
        {agent && (
          <>
            <span className="rounded-full bg-gray-700 px-2 py-0.5 text-xs text-gray-400">
              {agent.agent_type}
            </span>
            <span className="text-sm text-gray-500 capitalize">{agent.status}</span>
          </>
        )}

        {/* Tab bar */}
        <div className="ml-auto flex rounded-lg bg-gray-800 p-0.5">
          <button
            onClick={() => setActiveTab('terminal')}
            className={`px-3 py-1 text-sm rounded-md transition-colors ${
              activeTab === 'terminal'
                ? 'bg-gray-700 text-white'
                : 'text-gray-400 hover:text-gray-200'
            }`}
          >
            Terminal
          </button>
          <button
            onClick={() => setActiveTab('chat')}
            className={`px-3 py-1 text-sm rounded-md transition-colors ${
              activeTab === 'chat'
                ? 'bg-gray-700 text-white'
                : 'text-gray-400 hover:text-gray-200'
            }`}
          >
            Chat
          </button>
        </div>
      </div>

      {connectionInfo && (
        <ConnectionInfo
          hostname={connectionInfo.hostname}
          username={connectionInfo.username}
          tmuxSession={connectionInfo.tmux_session}
          sshCommand={connectionInfo.ssh_command}
          tmuxCommand={connectionInfo.tmux_command}
        />
      )}

      {activeTab === 'terminal' ? (
        <div className="flex-1 min-h-0">
          <TerminalView
            agentId={agentId}
            tmuxSession={connectionInfo?.tmux_session}
          />
        </div>
      ) : (
        <>
          <div className="flex-1 overflow-y-auto px-6 py-4">
            <ConversationView events={events} />
          </div>
          <ResponseInput agentId={agentId} isWaiting={agent?.status === 'waiting'} />
        </>
      )}
    </div>
  );
}

export default function AgentPage() {
  return (
    <Suspense fallback={<div className="text-gray-500 text-center py-12">Loading...</div>}>
      <AgentPageContent />
    </Suspense>
  );
}
```

**Step 3: Verify frontend builds**

```bash
cd web && npm run build
```

**Step 4: Commit**

```bash
git add web/src/app/agent/page.tsx web/src/store/agents.ts
git commit -m "feat(web): add Terminal/Chat tab bar to agent page"
```

---

### Task 9: Full build and manual test

**Files:** None (integration testing)

**Step 1: Full build**

```bash
make build
```

Expected: builds cleanly, produces `./awayteam` binary

**Step 2: Manual test — start server**

```bash
./awayteam serve &
```

**Step 3: Manual test — start agent with tmux**

```bash
./awayteam agent --name "test-agent" -- bash -c "echo hello; sleep 30"
```

Verify:
- tmux session `awayteam-<id>` is created: `tmux ls`
- Dashboard at localhost:8080 shows agent
- Clicking agent opens terminal tab by default
- Terminal shows the bash session
- Chat tab shows structured events
- Connection info panel shows hostname, tmux session name

**Step 4: Manual test — general shell**

Open browser console, verify `/api/v1/ws/terminal` endpoint works (this will be used by a future "Open Shell" button)

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: address integration test issues"
```

---

### Task 10: Update CLAUDE.md with new routes

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Add terminal routes to API Routes section**

Add to the API Routes section in CLAUDE.md:

```markdown
- `GET /api/v1/ws/terminal/:id` — agent terminal WebSocket (tmux attach or PTY fallback, binary frames)
- `GET /api/v1/ws/terminal` — general shell WebSocket (spawns login shell)
```

**Step 2: Add terminal package to Go Backend table**

```markdown
| `terminal` | WebSocket-to-PTY bridge for browser terminal. Handles tmux attach and general shell sessions |
```

**Step 3: Add xterm.js to Frontend section**

Note that xterm.js dependencies were added and that the agent page has Terminal/Chat tabs.

**Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with terminal WebSocket endpoints"
```
