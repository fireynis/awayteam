# AI Dashboard Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a general-purpose AI agent dashboard with Kanban swimlanes, full-screen conversation views, and bidirectional Claude Code integration via PTY proxy.

**Architecture:** Go backend (`awayteam` CLI) serves an embedded Next.js frontend. Events flow in via HTTP POST (from hooks) and PTY output streaming. WebSocket hub broadcasts to the UI. Bidirectional agent-specific WebSocket channels allow responding to agent questions from the browser.

**Tech Stack:** Go 1.25, creack/pty, gorilla/websocket, modernc.org/sqlite, Next.js 16, React 19, Zustand 5, Tailwind CSS 4

---

## Project Structure

```
ai-dashboard/
├── cmd/awayteam/main.go                    # CLI entry point (serve, agent, install, hook)
├── internal/
│   ├── config/config.go               # YAML config with defaults
│   ├── events/events.go               # Event model + validation
│   ├── store/
│   │   ├── store.go                   # Store interface + types
│   │   └── sqlite.go                  # SQLite implementation
│   ├── ws/hub.go                      # WebSocket broadcast hub
│   ├── server/
│   │   ├── server.go                  # HTTP server, routing, WS handlers
│   │   ├── handlers.go                # REST endpoint handlers
│   │   └── middleware.go              # CORS middleware
│   ├── agent/proxy.go                 # PTY proxy (awayteam agent)
│   ├── hook/hook.go                   # Hook payload processors (awayteam hook)
│   └── frontend/embed.go             # Embedded frontend FS
├── web/
│   ├── package.json
│   ├── next.config.ts
│   ├── postcss.config.mjs
│   ├── tsconfig.json
│   └── src/
│       ├── app/
│       │   ├── layout.tsx
│       │   ├── globals.css
│       │   ├── page.tsx               # Kanban board
│       │   └── agents/[id]/page.tsx   # Conversation page
│       ├── components/
│       │   ├── layout-shell.tsx
│       │   ├── agent-card.tsx
│       │   ├── conversation-view.tsx
│       │   └── response-input.tsx
│       ├── store/agents.ts
│       ├── hooks/useWebSocket.ts
│       └── lib/types.ts
├── config.example.yaml
├── Makefile
├── go.mod
└── go.sum
```

---

## Task 1: Go Module + Makefile + Config

**Files:**
- Create: `go.mod`
- Create: `cmd/awayteam/main.go`
- Create: `internal/config/config.go`
- Create: `config.example.yaml`
- Create: `Makefile`

**Step 1: Initialize Go module**

```bash
cd /home/jeremy/projects/ai-dashboard
go mod init github.com/jeremy/awayteam
```

**Step 2: Create config package**

Create `internal/config/config.go`:

```go
package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type StorageConfig struct {
	SQLitePath string `yaml:"sqlite_path"`
}

func defaults() Config {
	return Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Storage: StorageConfig{
			SQLitePath: "./awayteam.db",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := defaults()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
```

**Step 3: Create minimal CLI entry point**

Create `cmd/awayteam/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jeremy/awayteam/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: awayteam<command> [args]\n")
		fmt.Fprintf(os.Stderr, "Commands: serve, agent, install, hook\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Printf("awayteam dashboard starting on :%d", cfg.Server.Port)
	// Server will be wired in Task 4
}
```

**Step 4: Create config example and Makefile**

Create `config.example.yaml`:

```yaml
server:
  port: 8080

storage:
  sqlite_path: ./awayteam.db
```

Create `Makefile`:

```makefile
.PHONY: build run dev clean frontend

build: frontend
	go build -o awayteam ./cmd/awayteam

run: build
	./awayteamserve

dev:
	go run ./cmd/awayteam serve

frontend:
	cd web && npm run build
	rm -rf internal/frontend/dist
	cp -r web/out internal/frontend/dist

clean:
	rm -f awayteam
	rm -rf internal/frontend/dist
	rm -rf web/out web/.next

test:
	go test ./...
```

**Step 5: Install Go dependencies and verify build**

```bash
go get gopkg.in/yaml.v3
go build ./cmd/awayteam
```

Expected: binary builds without errors.

**Step 6: Commit**

```bash
git add -A
git commit -m "feat: project scaffolding with Go module, config, and Makefile"
```

---

## Task 2: Event Model

**Files:**
- Create: `internal/events/events.go`
- Create: `internal/events/events_test.go`

**Step 1: Write failing tests for event validation**

Create `internal/events/events_test.go`:

```go
package events

import (
	"testing"
	"time"
)

func TestValidate_ValidEvent(t *testing.T) {
	e := Event{
		ID:        "test-123",
		Type:      "session.start",
		Timestamp: time.Now(),
		AgentID:   "agent-1",
		AgentName: "test-agent",
		Status:    StatusActive,
	}
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid event, got error: %v", err)
	}
}

func TestValidate_MissingID(t *testing.T) {
	e := Event{
		Type:      "session.start",
		Timestamp: time.Now(),
		AgentID:   "agent-1",
		Status:    StatusActive,
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestValidate_MissingType(t *testing.T) {
	e := Event{
		ID:        "test-123",
		Timestamp: time.Now(),
		AgentID:   "agent-1",
		Status:    StatusActive,
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestValidate_MissingAgentID(t *testing.T) {
	e := Event{
		ID:        "test-123",
		Type:      "session.start",
		Timestamp: time.Now(),
		Status:    StatusActive,
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for missing agent_id")
	}
}

func TestValidate_InvalidStatus(t *testing.T) {
	e := Event{
		ID:        "test-123",
		Type:      "session.start",
		Timestamp: time.Now(),
		AgentID:   "agent-1",
		Status:    "invalid",
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for invalid status")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/events/ -v
```

Expected: compilation errors (package doesn't exist yet).

**Step 3: Implement event model**

Create `internal/events/events.go`:

```go
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Status string

const (
	StatusQueued  Status = "queued"
	StatusActive  Status = "active"
	StatusWaiting Status = "waiting"
	StatusDone    Status = "done"
	StatusError   Status = "error"
)

var validStatuses = map[Status]bool{
	StatusQueued:  true,
	StatusActive:  true,
	StatusWaiting: true,
	StatusDone:    true,
	StatusError:   true,
}

type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	AgentID   string          `json:"agent_id"`
	AgentType string          `json:"agent_type,omitempty"`
	AgentName string          `json:"agent_name,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Status    Status          `json:"status"`
}

func (e *Event) Validate() error {
	if e.ID == "" {
		return errors.New("id is required")
	}
	if e.Type == "" {
		return errors.New("type is required")
	}
	if e.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	if e.AgentID == "" {
		return errors.New("agent_id is required")
	}
	if !validStatuses[e.Status] {
		return fmt.Errorf("invalid status: %q", e.Status)
	}
	return nil
}
```

**Step 4: Run tests**

```bash
go test ./internal/events/ -v
```

Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/events/
git commit -m "feat: event model with validation"
```

---

## Task 3: SQLite Store

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/sqlite.go`
- Create: `internal/store/sqlite_test.go`

**Step 1: Write store interface**

Create `internal/store/store.go`:

```go
package store

import (
	"context"

	"github.com/jeremy/awayteam/internal/events"
)

type AgentState struct {
	AgentID   string        `json:"agent_id"`
	AgentType string        `json:"agent_type,omitempty"`
	AgentName string        `json:"agent_name,omitempty"`
	Status    events.Status `json:"status"`
	LastEvent string        `json:"last_event"`
}

type Store interface {
	SaveEvent(ctx context.Context, event events.Event) error
	GetAgents(ctx context.Context) ([]AgentState, error)
	GetAgentEvents(ctx context.Context, agentID string, limit int) ([]events.Event, error)
	Close() error
}
```

**Step 2: Write failing tests for SQLite store**

Create `internal/store/sqlite_test.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jeremy/awayteam/internal/events"
)

func testEvent(agentID, typ string, status events.Status) events.Event {
	return events.Event{
		ID:        "evt-" + typ,
		Type:      typ,
		Timestamp: time.Now(),
		AgentID:   agentID,
		AgentType: "test",
		AgentName: "test-agent",
		Data:      json.RawMessage(`{}`),
		Status:    status,
	}
}

func TestSQLiteStore_SaveAndGetAgents(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	evt := testEvent("agent-1", "session.start", events.StatusActive)
	if err := s.SaveEvent(ctx, evt); err != nil {
		t.Fatalf("save event: %v", err)
	}

	agents, err := s.GetAgents(ctx)
	if err != nil {
		t.Fatalf("get agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].AgentID != "agent-1" {
		t.Fatalf("expected agent-1, got %s", agents[0].AgentID)
	}
	if agents[0].Status != events.StatusActive {
		t.Fatalf("expected active, got %s", agents[0].Status)
	}
}

func TestSQLiteStore_GetAgentEvents(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	e1 := testEvent("agent-1", "session.start", events.StatusActive)
	e1.ID = "evt-1"
	e1.Timestamp = time.Now().Add(-2 * time.Second)

	e2 := testEvent("agent-1", "tool.call", events.StatusActive)
	e2.ID = "evt-2"
	e2.Timestamp = time.Now().Add(-1 * time.Second)

	s.SaveEvent(ctx, e1)
	s.SaveEvent(ctx, e2)

	evts, err := s.GetAgentEvents(ctx, "agent-1", 10)
	if err != nil {
		t.Fatalf("get agent events: %v", err)
	}
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evts))
	}
}

func TestSQLiteStore_AgentStatusUpdates(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	e1 := testEvent("agent-1", "session.start", events.StatusActive)
	e1.ID = "evt-1"
	s.SaveEvent(ctx, e1)

	e2 := testEvent("agent-1", "question.asked", events.StatusWaiting)
	e2.ID = "evt-2"
	s.SaveEvent(ctx, e2)

	agents, err := s.GetAgents(ctx)
	if err != nil {
		t.Fatalf("get agents: %v", err)
	}
	if agents[0].Status != events.StatusWaiting {
		t.Fatalf("expected waiting, got %s", agents[0].Status)
	}
}
```

**Step 3: Run tests to verify they fail**

```bash
go get modernc.org/sqlite
go test ./internal/store/ -v
```

Expected: compilation errors.

**Step 4: Implement SQLite store**

Create `internal/store/sqlite.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jeremy/awayteam/internal/events"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			agent_type TEXT DEFAULT '',
			agent_name TEXT DEFAULT '',
			data_json TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_events_agent ON events(agent_id);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

		CREATE TABLE IF NOT EXISTS agent_state (
			agent_id TEXT PRIMARY KEY,
			agent_type TEXT DEFAULT '',
			agent_name TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			last_event TEXT NOT NULL
		);
	`)
	return err
}

func (s *SQLiteStore) SaveEvent(ctx context.Context, evt events.Event) error {
	dataJSON := "{}"
	if evt.Data != nil {
		dataJSON = string(evt.Data)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO events (id, type, timestamp, agent_id, agent_type, agent_name, data_json, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.ID, evt.Type, evt.Timestamp.UTC().Format(time.RFC3339Nano),
		evt.AgentID, evt.AgentType, evt.AgentName, dataJSON, string(evt.Status))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO agent_state (agent_id, agent_type, agent_name, status, last_event)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id) DO UPDATE SET
		   agent_type = excluded.agent_type,
		   agent_name = excluded.agent_name,
		   status = excluded.status,
		   last_event = excluded.last_event`,
		evt.AgentID, evt.AgentType, evt.AgentName, string(evt.Status),
		evt.Timestamp.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert agent state: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetAgents(ctx context.Context) ([]AgentState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, agent_type, agent_name, status, last_event
		 FROM agent_state ORDER BY last_event DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []AgentState
	for rows.Next() {
		var a AgentState
		if err := rows.Scan(&a.AgentID, &a.AgentType, &a.AgentName, &a.Status, &a.LastEvent); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *SQLiteStore) GetAgentEvents(ctx context.Context, agentID string, limit int) ([]events.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, timestamp, agent_id, agent_type, agent_name, data_json, status
		 FROM events WHERE agent_id = ? ORDER BY timestamp ASC LIMIT ?`,
		agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evts []events.Event
	for rows.Next() {
		var e events.Event
		var ts, dataJSON string
		if err := rows.Scan(&e.ID, &e.Type, &ts, &e.AgentID, &e.AgentType, &e.AgentName, &dataJSON, &e.Status); err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		e.Data = json.RawMessage(dataJSON)
		evts = append(evts, e)
	}
	return evts, rows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
```

**Step 5: Run tests**

```bash
go test ./internal/store/ -v
```

Expected: all PASS.

**Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat: SQLite store with event persistence and agent state tracking"
```

---

## Task 4: WebSocket Hub

**Files:**
- Create: `internal/ws/hub.go`
- Create: `internal/ws/hub_test.go`

**Step 1: Write failing tests**

Create `internal/ws/hub_test.go`:

```go
package ws

import (
	"testing"
	"time"
)

func TestHub_RegisterAndBroadcast(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 256)
	h.Register(ch)

	if h.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", h.ClientCount())
	}

	h.Broadcast([]byte("hello"))

	select {
	case msg := <-ch:
		if string(msg) != "hello" {
			t.Fatalf("expected 'hello', got '%s'", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestHub_Unregister(t *testing.T) {
	h := NewHub()

	ch := make(chan []byte, 256)
	h.Register(ch)
	h.Unregister(ch)

	if h.ClientCount() != 0 {
		t.Fatalf("expected 0 clients, got %d", h.ClientCount())
	}
}

func TestHub_BroadcastSkipsFullChannel(t *testing.T) {
	h := NewHub()

	// Channel with buffer of 1
	ch := make(chan []byte, 1)
	h.Register(ch)

	// Fill the channel
	ch <- []byte("fill")

	// This should not block
	done := make(chan struct{})
	go func() {
		h.Broadcast([]byte("overflow"))
		close(done)
	}()

	select {
	case <-done:
		// Good, didn't block
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on full channel")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/ws/ -v
```

**Step 3: Implement hub**

Create `internal/ws/hub.go`:

```go
package ws

import "sync"

type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan []byte]struct{}),
	}
}

func (h *Hub) Register(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
}

func (h *Hub) Unregister(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
}

func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
```

**Step 4: Run tests**

```bash
go test ./internal/ws/ -v
```

Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/ws/
git commit -m "feat: WebSocket broadcast hub with non-blocking sends"
```

---

## Task 5: HTTP Server + REST API

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/handlers.go`
- Create: `internal/server/middleware.go`
- Create: `internal/server/server_test.go`

**Step 1: Write failing tests**

Create `internal/server/server_test.go`:

```go
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jeremy/awayteam/internal/config"
	"github.com/jeremy/awayteam/internal/events"
	"github.com/jeremy/awayteam/internal/store"
	"github.com/jeremy/awayteam/internal/ws"
)

func testServer(t *testing.T) (*Server, *store.SQLiteStore) {
	t.Helper()
	st, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	hub := ws.NewHub()
	cfg := config.Config{Server: config.ServerConfig{Port: 8080}}
	srv := New(cfg, st, hub)
	return srv, st
}

func TestPostEvent(t *testing.T) {
	srv, st := testServer(t)
	defer st.Close()

	evt := events.Event{
		ID:        "test-1",
		Type:      "session.start",
		Timestamp: time.Now(),
		AgentID:   "agent-1",
		AgentName: "test",
		Status:    events.StatusActive,
	}
	body, _ := json.Marshal(evt)

	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostEvent_InvalidJSON(t *testing.T) {
	srv, st := testServer(t)
	defer st.Close()

	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader([]byte("invalid")))
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetAgents(t *testing.T) {
	srv, st := testServer(t)
	defer st.Close()

	// Insert an event first
	st.SaveEvent(context.Background(), events.Event{
		ID: "e1", Type: "session.start", Timestamp: time.Now(),
		AgentID: "agent-1", AgentName: "test", Status: events.StatusActive,
	})

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var agents []store.AgentState
	json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}

func TestGetAgentEvents(t *testing.T) {
	srv, st := testServer(t)
	defer st.Close()

	st.SaveEvent(context.Background(), events.Event{
		ID: "e1", Type: "session.start", Timestamp: time.Now(),
		AgentID: "agent-1", AgentName: "test", Status: events.StatusActive,
	})

	req := httptest.NewRequest("GET", "/api/v1/agents/agent-1/events", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var evts []events.Event
	json.NewDecoder(w.Body).Decode(&evts)
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
}

func TestHealthz(t *testing.T) {
	srv, st := testServer(t)
	defer st.Close()

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go get github.com/gorilla/websocket
go test ./internal/server/ -v
```

**Step 3: Implement middleware**

Create `internal/server/middleware.go`:

```go
package server

import "net/http"

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
```

**Step 4: Implement handlers**

Create `internal/server/handlers.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	var evt events.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := evt.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.SaveEvent(r.Context(), evt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save event")
		return
	}

	data, err := json.Marshal(evt)
	if err == nil {
		s.hub.Broadcast(data)
	}

	writeJSON(w, http.StatusCreated, evt)
}

func (s *Server) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.GetAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get agents")
		return
	}
	if agents == nil {
		agents = []store.AgentState{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgentEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "agent id is required")
		return
	}

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	evts, err := s.store.GetAgentEvents(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get events")
		return
	}
	if evts == nil {
		evts = []events.Event{}
	}
	writeJSON(w, http.StatusOK, evts)
}
```

**Step 5: Implement server**

Create `internal/server/server.go`:

```go
package server

import (
	"io/fs"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/jeremy/awayteam/internal/config"
	"github.com/jeremy/awayteam/internal/events"
	"github.com/jeremy/awayteam/internal/store"
	"github.com/jeremy/awayteam/internal/ws"
)

type Server struct {
	store      store.Store
	hub        *ws.Hub
	config     config.Config
	upgrader   websocket.Upgrader
	frontendFS fs.FS
}

func New(cfg config.Config, st store.Store, h *ws.Hub) *Server {
	return &Server{
		store:  st,
		hub:    h,
		config: cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) SetFrontendFS(fsys fs.FS) {
	s.frontendFS = fsys
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Event ingestion
	mux.HandleFunc("POST /api/v1/events", s.handlePostEvent)

	// Agent queries
	mux.HandleFunc("GET /api/v1/agents", s.handleGetAgents)
	mux.HandleFunc("GET /api/v1/agents/{id}/events", s.handleGetAgentEvents)

	// WebSocket: broadcast all events
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)

	// WebSocket: agent-specific (bidirectional for responses)
	mux.HandleFunc("GET /api/v1/ws/agents/{id}", s.handleAgentWS)

	// Frontend SPA fallback
	if s.frontendFS != nil {
		mux.Handle("/", http.FileServerFS(s.frontendFS))
	}

	return corsMiddleware(mux)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	ch := make(chan []byte, 256)
	s.hub.Register(ch)

	go func() {
		defer conn.Close()
		for msg := range ch {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.hub.Unregister(ch)
}

// handleAgentWS is a bidirectional WS for a specific agent.
// Server pushes events for that agent; client sends responses.
func (s *Server) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Register for broadcast events (filtered to this agent)
	ch := make(chan []byte, 256)
	s.hub.Register(ch)

	// Write loop: send only events for this agent
	go func() {
		defer conn.Close()
		for msg := range ch {
			// Quick filter: check if this event is for our agent
			// For efficiency, we broadcast all and let the client filter,
			// but we could also filter here
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	// Read loop: client sends responses that need to reach the PTY proxy
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		// Route response to the agent's PTY proxy
		s.hub.RouteResponse(agentID, msg)
	}

	s.hub.Unregister(ch)
}
```

Note: We need to add `RouteResponse` to the hub. Update `internal/ws/hub.go` to add agent response routing:

Add to `internal/ws/hub.go`:

```go
// Add these fields to Hub struct:
type Hub struct {
	mu             sync.RWMutex
	clients        map[chan []byte]struct{}
	agentListeners map[string]chan []byte // agentID -> response channel
}

// Update NewHub:
func NewHub() *Hub {
	return &Hub{
		clients:        make(map[chan []byte]struct{}),
		agentListeners: make(map[string]chan []byte),
	}
}

// Add these methods:
func (h *Hub) RegisterAgent(agentID string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agentListeners[agentID] = ch
}

func (h *Hub) UnregisterAgent(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.agentListeners[agentID]; ok {
		close(ch)
		delete(h.agentListeners, agentID)
	}
}

func (h *Hub) RouteResponse(agentID string, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if ch, ok := h.agentListeners[agentID]; ok {
		select {
		case ch <- data:
		default:
		}
	}
}
```

**Step 6: Run tests**

```bash
go test ./internal/server/ -v
```

Expected: all PASS.

**Step 7: Wire server into main.go**

Update `cmd/awayteam/main.go` `cmdServe` function:

```go
func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	st, err := store.NewSQLiteStore(cfg.Storage.SQLitePath)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	hub := ws.NewHub()
	srv := server.New(cfg, st, hub)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("awayteam dashboard starting on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
}
```

**Step 8: Run full test suite and verify build**

```bash
go test ./... -v
go build ./cmd/awayteam
```

Expected: all tests pass, binary builds.

**Step 9: Commit**

```bash
git add internal/server/ internal/ws/ cmd/awayteam/
git commit -m "feat: HTTP server with event ingestion, REST API, and WebSocket hub"
```

---

## Task 6: PTY Proxy (`awayteam agent`)

**Files:**
- Create: `internal/agent/proxy.go`
- Modify: `cmd/awayteam/main.go` (add `agent` subcommand)

**Step 1: Implement PTY proxy**

Create `internal/agent/proxy.go`:

```go
package agent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

type ProxyConfig struct {
	Name      string
	AgentType string
	ServerURL string // e.g., "http://localhost:8080"
	Command   string
	Args      []string
}

func RunProxy(cfg ProxyConfig) error {
	agentID := uuid.NewString()

	// Build the command
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = append(os.Environ(), "AWAYTEAM_AGENT_ID="+agentID)

	// Start with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}
	defer ptmx.Close()

	// Handle SIGWINCH
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	sigCh <- syscall.SIGWINCH
	defer func() { signal.Stop(sigCh); close(sigCh) }()

	// Put terminal in raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("make raw: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Post session.start event
	postEvent(cfg.ServerURL, newEvent(agentID, cfg.Name, cfg.AgentType, "session.start", "active", nil))

	// Connect agent WS for receiving dashboard responses
	var wsConn *websocket.Conn
	wsURL := toWSURL(cfg.ServerURL) + "/api/v1/ws/agents/" + agentID
	wsConn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Printf("warning: could not connect to dashboard WS: %v", err)
	}

	// Dashboard responses → PTY stdin
	if wsConn != nil {
		go func() {
			defer wsConn.Close()
			for {
				_, msg, err := wsConn.ReadMessage()
				if err != nil {
					return
				}
				// Response message: write directly to PTY stdin
				var resp struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if json.Unmarshal(msg, &resp) == nil && resp.Type == "response" {
					ptmx.Write([]byte(resp.Text))
				}
			}
		}()
	}

	// Local stdin → PTY
	go func() { io.Copy(ptmx, os.Stdin) }()

	// PTY → local stdout + dashboard streaming
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])

			// Stream to dashboard
			chunk := base64.StdEncoding.EncodeToString(buf[:n])
			data, _ := json.Marshal(map[string]string{"chunk": chunk})
			postEvent(cfg.ServerURL, newEvent(agentID, cfg.Name, cfg.AgentType, "output.stream", "active", data))
		}
		if err != nil {
			if errors.Is(err, syscall.EIO) {
				break // Normal exit on Linux
			}
			if err != io.EOF {
				log.Printf("pty read error: %v", err)
			}
			break
		}
	}

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// Post session.end event
	endData, _ := json.Marshal(map[string]any{"exit_code": exitCode})
	postEvent(cfg.ServerURL, newEvent(agentID, cfg.Name, cfg.AgentType, "session.end", "done", endData))

	return nil
}

func newEvent(agentID, name, agentType, typ, status string, data json.RawMessage) map[string]any {
	evt := map[string]any{
		"id":         uuid.NewString(),
		"type":       typ,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":   agentID,
		"agent_type": agentType,
		"agent_name": name,
		"status":     status,
	}
	if data != nil {
		evt["data"] = json.RawMessage(data)
	}
	return evt
}

func postEvent(serverURL string, evt map[string]any) {
	body, err := json.Marshal(evt)
	if err != nil {
		return
	}
	resp, err := http.Post(serverURL+"/api/v1/events", "application/json", bytes.NewReader(body))
	if err != nil {
		return // Silent failure - dashboard might not be running
	}
	resp.Body.Close()
}

func toWSURL(httpURL string) string {
	u, err := url.Parse(httpURL)
	if err != nil {
		return strings.Replace(httpURL, "http://", "ws://", 1)
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	return u.String()
}
```

**Step 2: Add `agent` subcommand to main.go**

Add to the switch in `main.go`:

```go
case "agent":
    cmdAgent(os.Args[2:])
```

Add function:

```go
func cmdAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	name := fs.String("name", "", "agent name (shown in dashboard)")
	agentType := fs.String("type", "generic", "agent type")
	serverURL := fs.String("server", "http://localhost:8080", "dashboard server URL")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: awayteamagent [flags] <command> [args...]\n")
		os.Exit(1)
	}

	if *name == "" {
		*name = remaining[0]
	}

	cfg := agent.ProxyConfig{
		Name:      *name,
		AgentType: *agentType,
		ServerURL: *serverURL,
		Command:   remaining[0],
		Args:      remaining[1:],
	}

	if err := agent.RunProxy(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Install dependencies and build**

```bash
go get github.com/creack/pty
go get github.com/google/uuid
go get golang.org/x/term
go build ./cmd/awayteam
```

Expected: builds without errors.

**Step 4: Manual test**

```bash
# Terminal 1: start the dashboard
./awayteamserve

# Terminal 2: run a simple command through the proxy
./awayteamagent --name "test" bash -c "echo hello world && sleep 1"
```

Expected: "hello world" appears in terminal 2, events posted to dashboard.

**Step 5: Commit**

```bash
git add internal/agent/ cmd/awayteam/
git commit -m "feat: PTY proxy for bidirectional agent communication"
```

---

## Task 7: Hook Commands (`awayteam hook`)

**Files:**
- Create: `internal/hook/hook.go`
- Modify: `cmd/awayteam/main.go` (add `hook` subcommand)

**Step 1: Implement hook processor**

Create `internal/hook/hook.go`:

```go
package hook

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

// ProcessHook reads a Claude Code hook payload from stdin and POSTs
// a structured event to the dashboard server.
func ProcessHook(hookType, serverURL string) error {
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	agentID := os.Getenv("AWAYTEAM_AGENT_ID")
	if agentID == "" {
		// Not running under awayteam agent - generate a transient ID from CWD
		agentID = "hook-" + uuid.NewString()[:8]
	}

	var eventType, status string
	switch hookType {
	case "post-tool-use":
		eventType = "tool.result"
		status = "active"
	case "notification":
		eventType = "message.assistant"
		status = "active"
	case "user-prompt-submit":
		eventType = "message.user"
		status = "active"
	default:
		eventType = "hook." + hookType
		status = "active"
	}

	evt := map[string]any{
		"id":         uuid.NewString(),
		"type":       eventType,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":   agentID,
		"agent_type": "claude-code",
		"agent_name": os.Getenv("AWAYTEAM_AGENT_NAME"),
		"status":     status,
		"data":       json.RawMessage(payload),
	}

	body, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	resp, err := http.Post(serverURL+"/api/v1/events", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil // Silent failure
	}
	resp.Body.Close()
	return nil
}
```

**Step 2: Add `hook` subcommand to main.go**

```go
case "hook":
    cmdHook(os.Args[2:])
```

```go
func cmdHook(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: awayteamhook <type>\n")
		fmt.Fprintf(os.Stderr, "Types: post-tool-use, notification, user-prompt-submit\n")
		os.Exit(1)
	}

	serverURL := os.Getenv("AWAYTEAM_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	if err := hook.ProcessHook(args[0], serverURL); err != nil {
		// Silent failure - don't disrupt Claude Code
		os.Exit(0)
	}
}
```

**Step 3: Build and verify**

```bash
go build ./cmd/awayteam
echo '{"tool":"Read","result":"ok"}' | ./awayteamhook post-tool-use
```

Expected: no errors (event posted silently).

**Step 4: Commit**

```bash
git add internal/hook/ cmd/awayteam/
git commit -m "feat: hook command for Claude Code event ingestion"
```

---

## Task 8: Install Command (`awayteam install`)

**Files:**
- Modify: `cmd/awayteam/main.go` (add `install` subcommand)

**Step 1: Implement install command**

Add to `main.go`:

```go
case "install":
    cmdInstall(os.Args[2:])
```

```go
func cmdInstall(args []string) {
	if len(args) == 0 || args[0] != "claude-code" {
		fmt.Fprintf(os.Stderr, "Usage: awayteaminstall claude-code\n")
		os.Exit(1)
	}

	// Find the awayteam binary path
	aidPath, err := os.Executable()
	if err != nil {
		log.Fatalf("could not determine awayteam binary path: %v", err)
	}

	hookConfig := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []map[string]string{
				{"type": "command", "command": aidPath + " hook post-tool-use"},
			},
			"Notification": []map[string]string{
				{"type": "command", "command": aidPath + " hook notification"},
			},
		},
	}

	data, _ := json.MarshalIndent(hookConfig, "", "  ")
	fmt.Println("Add the following to your ~/.claude/settings.json or project .claude/settings.json:")
	fmt.Println()
	fmt.Println(string(data))
	fmt.Println()
	fmt.Printf("Or run: awayteam agent --name '<name>' claude\n")
	fmt.Println("to start Claude Code with the PTY proxy (recommended).")
}
```

**Step 2: Build and test**

```bash
go build ./cmd/awayteam
./awayteaminstall claude-code
```

Expected: prints hook configuration JSON.

**Step 3: Commit**

```bash
git add cmd/awayteam/
git commit -m "feat: install command for Claude Code hook setup"
```

---

## Task 9: Frontend Setup

**Files:**
- Create: `web/package.json`
- Create: `web/next.config.ts`
- Create: `web/postcss.config.mjs`
- Create: `web/tsconfig.json`
- Create: `web/src/app/layout.tsx`
- Create: `web/src/app/globals.css`
- Create: `web/src/lib/types.ts`

**Step 1: Initialize Next.js project**

```bash
cd /home/jeremy/projects/ai-dashboard
mkdir -p web/src/app web/src/components web/src/store web/src/hooks web/src/lib
```

Create `web/package.json`:

```json
{
  "name": "awayteam-dashboard",
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "dev": "next dev --port 3000",
    "build": "next build",
    "lint": "eslint"
  },
  "dependencies": {
    "next": "16.1.6",
    "react": "19.2.3",
    "react-dom": "19.2.3",
    "zustand": "^5.0.11"
  },
  "devDependencies": {
    "@tailwindcss/postcss": "^4",
    "@types/node": "^20",
    "@types/react": "^19",
    "@types/react-dom": "^19",
    "eslint": "^9",
    "eslint-config-next": "16.1.6",
    "tailwindcss": "^4",
    "typescript": "^5"
  }
}
```

Create `web/next.config.ts`:

```typescript
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
};

export default nextConfig;
```

Create `web/postcss.config.mjs`:

```javascript
const config = {
  plugins: {
    "@tailwindcss/postcss": {},
  },
};

export default config;
```

Create `web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2017",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "react-jsx",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

**Step 2: Create types**

Create `web/src/lib/types.ts`:

```typescript
export type AgentStatus = "queued" | "active" | "waiting" | "done" | "error";

export interface DashboardEvent {
  id: string;
  type: string;
  timestamp: string;
  agent_id: string;
  agent_type: string;
  agent_name: string;
  data?: Record<string, unknown>;
  status: AgentStatus;
}

export interface AgentState {
  agent_id: string;
  agent_type: string;
  agent_name: string;
  status: AgentStatus;
  last_event: string;
}
```

**Step 3: Create global styles**

Create `web/src/app/globals.css`:

```css
@import "tailwindcss";

@theme inline {
  --color-background: #0a0a0a;
  --color-foreground: #ededed;
  --font-sans: 'Geist', Arial, Helvetica, sans-serif;
  --font-mono: 'Geist Mono', ui-monospace, monospace;
  --color-gray-750: #2a2f3a;
}

body {
  background: var(--color-background);
  color: var(--color-foreground);
}
```

**Step 4: Create layout**

Create `web/src/app/layout.tsx`:

```tsx
import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'AI Dashboard',
  description: 'Monitor and interact with AI agents',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <head>
        <link
          href="https://fonts.googleapis.com/css2?family=Geist:wght@400;500;600;700&family=Geist+Mono:wght@400;500;600&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="font-[Geist] antialiased">
        {children}
      </body>
    </html>
  );
}
```

**Step 5: Install dependencies and verify build**

```bash
cd web && npm install && npm run build && cd ..
```

Expected: builds to `web/out/` without errors.

**Step 6: Commit**

```bash
git add web/package.json web/next.config.ts web/postcss.config.mjs web/tsconfig.json web/src/
echo "node_modules/" >> .gitignore
echo "web/.next/" >> .gitignore
echo "web/out/" >> .gitignore
echo "internal/frontend/dist/" >> .gitignore
echo "*.db" >> .gitignore
echo "awayteam" >> .gitignore
git add .gitignore
git commit -m "feat: Next.js frontend scaffolding with Tailwind dark theme"
```

---

## Task 10: Zustand Store + WebSocket Hook

**Files:**
- Create: `web/src/store/agents.ts`
- Create: `web/src/hooks/useWebSocket.ts`
- Create: `web/src/components/layout-shell.tsx`
- Modify: `web/src/app/layout.tsx` (add LayoutShell)

**Step 1: Create Zustand store**

Create `web/src/store/agents.ts`:

```typescript
import { create } from 'zustand';
import type { AgentState, AgentStatus, DashboardEvent } from '@/lib/types';

const MAX_EVENTS_PER_AGENT = 500;
const MAX_RECENT_EVENTS = 100;

interface AgentStore {
  agents: Map<string, AgentState>;
  agentEvents: Map<string, DashboardEvent[]>;
  recentEvents: DashboardEvent[];
  setAgents: (agents: AgentState[]) => void;
  handleEvent: (event: DashboardEvent) => void;
}

export const useAgentStore = create<AgentStore>((set) => ({
  agents: new Map(),
  agentEvents: new Map(),
  recentEvents: [],

  setAgents: (agents: AgentState[]) => {
    const map = new Map<string, AgentState>();
    for (const a of agents) {
      map.set(a.agent_id, a);
    }
    set({ agents: map });
  },

  handleEvent: (event: DashboardEvent) => {
    set((state) => {
      const agents = new Map(state.agents);
      const agentEvents = new Map(state.agentEvents);

      // Update agent state
      agents.set(event.agent_id, {
        agent_id: event.agent_id,
        agent_type: event.agent_type,
        agent_name: event.agent_name,
        status: event.status as AgentStatus,
        last_event: event.timestamp,
      });

      // Append event to agent's event list (skip output.stream for conversation view)
      if (event.type !== 'output.stream') {
        const existing = agentEvents.get(event.agent_id) ?? [];
        const updated = [...existing, event].slice(-MAX_EVENTS_PER_AGENT);
        agentEvents.set(event.agent_id, updated);
      }

      // Update recent events
      const recentEvents = [event, ...state.recentEvents].slice(0, MAX_RECENT_EVENTS);

      return { agents, agentEvents, recentEvents };
    });
  },
}));
```

**Step 2: Create WebSocket hook**

Create `web/src/hooks/useWebSocket.ts`:

```typescript
'use client';

import { useEffect, useRef } from 'react';
import { useAgentStore } from '@/store/agents';
import type { AgentState, DashboardEvent } from '@/lib/types';

export function useWebSocket(url: string) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backoffRef = useRef(1000);
  const unmountedRef = useRef(false);

  useEffect(() => {
    unmountedRef.current = false;

    function deriveRestUrl(wsUrl: string): string {
      const parsed = new URL(wsUrl);
      parsed.protocol = parsed.protocol === 'wss:' ? 'https:' : 'http:';
      parsed.pathname = '/api/v1/agents';
      return parsed.toString();
    }

    function connect() {
      if (unmountedRef.current) return;

      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = async () => {
        backoffRef.current = 1000;
        try {
          const res = await fetch(deriveRestUrl(url));
          if (res.ok) {
            const agents: AgentState[] = await res.json();
            useAgentStore.getState().setAgents(agents);
          }
        } catch { /* ignore */ }
      };

      ws.onmessage = (msg) => {
        try {
          const event: DashboardEvent = JSON.parse(msg.data);
          useAgentStore.getState().handleEvent(event);
        } catch { /* ignore */ }
      };

      ws.onclose = () => {
        if (unmountedRef.current) return;
        scheduleReconnect();
      };

      ws.onerror = () => { ws.close(); };
    }

    function scheduleReconnect() {
      if (unmountedRef.current) return;
      const delay = backoffRef.current;
      backoffRef.current = Math.min(delay * 2, 30000);
      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null;
        connect();
      }, delay);
    }

    connect();

    return () => {
      unmountedRef.current = true;
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      if (wsRef.current) wsRef.current.close();
    };
  }, [url]);
}
```

**Step 3: Create LayoutShell**

Create `web/src/components/layout-shell.tsx`:

```tsx
'use client';

import { useWebSocket } from '@/hooks/useWebSocket';
import Link from 'next/link';

export function LayoutShell({ children }: { children: React.ReactNode }) {
  const wsUrl =
    typeof window !== 'undefined'
      ? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws`
      : '';

  useWebSocket(wsUrl);

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <nav className="border-b border-gray-800 px-6 py-4">
        <div className="flex items-center gap-6">
          <Link href="/" className="text-xl font-bold">
            AI Dashboard
          </Link>
        </div>
      </nav>
      <main className="p-6">{children}</main>
    </div>
  );
}
```

**Step 4: Update layout.tsx to use LayoutShell**

Update `web/src/app/layout.tsx` body to:

```tsx
<body className="font-[Geist] antialiased">
  <LayoutShell>{children}</LayoutShell>
</body>
```

Add import: `import { LayoutShell } from '@/components/layout-shell';`

**Step 5: Build and verify**

```bash
cd web && npm run build && cd ..
```

Expected: builds successfully.

**Step 6: Commit**

```bash
git add web/src/
git commit -m "feat: Zustand agent store with WebSocket real-time updates"
```

---

## Task 11: Kanban Board Page

**Files:**
- Create: `web/src/components/agent-card.tsx`
- Create: `web/src/app/page.tsx`

**Step 1: Create AgentCard component**

Create `web/src/components/agent-card.tsx`:

```tsx
'use client';

import type { AgentState } from '@/lib/types';
import { useEffect, useState } from 'react';

function timeAgo(timestamp: string): string {
  const diff = Date.now() - new Date(timestamp).getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

function statusBorderColor(status: string): string {
  switch (status) {
    case 'active': return 'border-l-emerald-500';
    case 'waiting': return 'border-l-amber-500';
    case 'error': return 'border-l-red-500';
    case 'done': return 'border-l-gray-500';
    default: return 'border-l-blue-500';
  }
}

function statusDotColor(status: string): string {
  switch (status) {
    case 'active': return 'bg-emerald-500';
    case 'waiting': return 'bg-amber-500 animate-pulse';
    case 'error': return 'bg-red-500';
    case 'done': return 'bg-gray-500';
    default: return 'bg-blue-500';
  }
}

interface AgentCardProps {
  agent: AgentState;
}

export function AgentCard({ agent }: AgentCardProps) {
  const [ago, setAgo] = useState(timeAgo(agent.last_event));

  useEffect(() => {
    const interval = setInterval(() => {
      setAgo(timeAgo(agent.last_event));
    }, 5000);
    return () => clearInterval(interval);
  }, [agent.last_event]);

  return (
    <a href={`/agents/${encodeURIComponent(agent.agent_id)}`}>
      <div
        className={`rounded-lg bg-gray-800 border-l-4 ${statusBorderColor(agent.status)} p-4 hover:bg-gray-750 transition-colors cursor-pointer`}
      >
        <div className="flex items-start justify-between mb-2">
          <div className="flex items-center gap-2">
            <span className={`inline-block h-2.5 w-2.5 rounded-full ${statusDotColor(agent.status)}`} />
            <h3 className="text-lg font-bold text-gray-100 truncate">
              {agent.agent_name || agent.agent_id.slice(0, 8)}
            </h3>
          </div>
          <span className="text-xs text-gray-500 whitespace-nowrap ml-2">
            {ago}
          </span>
        </div>

        <div className="flex items-center gap-2 text-sm">
          <span className="rounded-full bg-gray-700 px-2 py-0.5 text-xs text-gray-400">
            {agent.agent_type || 'generic'}
          </span>
          <span className="text-gray-500 capitalize">{agent.status}</span>
        </div>
      </div>
    </a>
  );
}
```

**Step 2: Create Kanban board page**

Create `web/src/app/page.tsx`:

```tsx
'use client';

import { useAgentStore } from '@/store/agents';
import { AgentCard } from '@/components/agent-card';
import type { AgentState, AgentStatus } from '@/lib/types';

const COLUMNS: { status: AgentStatus; label: string; emptyText: string }[] = [
  { status: 'queued', label: 'Queued', emptyText: 'No queued agents' },
  { status: 'active', label: 'Active', emptyText: 'No active agents' },
  { status: 'waiting', label: 'Waiting', emptyText: 'No agents waiting' },
  { status: 'done', label: 'Done', emptyText: 'No completed agents' },
];

export default function KanbanPage() {
  const agents = useAgentStore((s) => s.agents);
  const allAgents = Array.from(agents.values());

  function agentsForStatus(status: AgentStatus): AgentState[] {
    return allAgents.filter((a) => a.status === status);
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Agents</h1>

      {allAgents.length === 0 && (
        <div className="text-gray-500 text-center py-12">
          No agents connected. Start one with: <code className="font-mono text-gray-400">awayteam agent --name &quot;my-task&quot; claude</code>
        </div>
      )}

      {allAgents.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          {COLUMNS.map((col) => {
            const colAgents = agentsForStatus(col.status);
            return (
              <div key={col.status}>
                <h2 className="text-sm font-semibold uppercase tracking-wide text-gray-400 mb-3">
                  {col.label}
                  {colAgents.length > 0 && (
                    <span className="ml-2 text-gray-500">({colAgents.length})</span>
                  )}
                </h2>
                <div className="space-y-3">
                  {colAgents.length === 0 && (
                    <p className="text-xs text-gray-600 py-4 text-center">{col.emptyText}</p>
                  )}
                  {colAgents.map((agent) => (
                    <AgentCard key={agent.agent_id} agent={agent} />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
```

**Step 3: Build and verify**

```bash
cd web && npm run build && cd ..
```

Expected: builds successfully.

**Step 4: Commit**

```bash
git add web/src/
git commit -m "feat: Kanban swimlane board with agent cards"
```

---

## Task 12: Conversation Page

**Files:**
- Create: `web/src/components/conversation-view.tsx`
- Create: `web/src/components/response-input.tsx`
- Create: `web/src/app/agents/[id]/page.tsx`

**Step 1: Create ConversationView component**

Create `web/src/components/conversation-view.tsx`:

```tsx
'use client';

import type { DashboardEvent } from '@/lib/types';

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString();
}

function EventBubble({ event }: { event: DashboardEvent }) {
  const isUser = event.type === 'message.user';
  const isAssistant = event.type === 'message.assistant';
  const isTool = event.type === 'tool.call' || event.type === 'tool.result';
  const isQuestion = event.type === 'question.asked';

  if (isTool) {
    const data = event.data as Record<string, unknown> | undefined;
    return (
      <div className="ml-4 border-l-2 border-gray-700 pl-3 py-1">
        <div className="flex items-center gap-2 text-xs text-gray-500">
          <span className="font-mono">{(data?.tool as string) ?? event.type}</span>
          <span>{formatTime(event.timestamp)}</span>
        </div>
        {data?.result && (
          <pre className="mt-1 text-xs text-gray-400 font-mono overflow-x-auto max-h-32 overflow-y-auto">
            {typeof data.result === 'string' ? data.result : JSON.stringify(data.result, null, 2)}
          </pre>
        )}
      </div>
    );
  }

  if (isQuestion) {
    const data = event.data as Record<string, unknown> | undefined;
    return (
      <div className="bg-amber-900/30 border border-amber-700 rounded-lg p-4 my-2">
        <div className="flex items-center gap-2 mb-2">
          <span className="text-amber-400 font-semibold text-sm">Question</span>
          <span className="text-xs text-gray-500">{formatTime(event.timestamp)}</span>
        </div>
        <p className="text-gray-200">{(data?.question as string) ?? 'Awaiting response...'}</p>
        {Array.isArray(data?.options) && (
          <div className="flex flex-wrap gap-2 mt-2">
            {(data.options as string[]).map((opt, i) => (
              <span key={i} className="rounded-full bg-amber-800/50 px-3 py-1 text-sm text-amber-200">
                {opt}
              </span>
            ))}
          </div>
        )}
      </div>
    );
  }

  const bubbleClass = isUser
    ? 'bg-blue-900/50 border-blue-700'
    : 'bg-gray-800 border-gray-700';

  const data = event.data as Record<string, unknown> | undefined;
  const content = (data?.content as string) ?? JSON.stringify(data ?? {});

  return (
    <div className={`rounded-lg border ${bubbleClass} p-3 my-2`}>
      <div className="flex items-center gap-2 mb-1">
        <span className="text-xs font-semibold text-gray-400">
          {isUser ? 'You' : 'Agent'}
        </span>
        <span className="text-xs text-gray-500">{formatTime(event.timestamp)}</span>
      </div>
      <div className="text-sm text-gray-200 whitespace-pre-wrap">{content}</div>
    </div>
  );
}

interface ConversationViewProps {
  events: DashboardEvent[];
}

export function ConversationView({ events }: ConversationViewProps) {
  const conversationEvents = events.filter(
    (e) =>
      e.type === 'message.user' ||
      e.type === 'message.assistant' ||
      e.type === 'tool.call' ||
      e.type === 'tool.result' ||
      e.type === 'question.asked' ||
      e.type === 'question.answered'
  );

  if (conversationEvents.length === 0) {
    return (
      <div className="text-gray-500 text-center py-8">
        No conversation events yet.
      </div>
    );
  }

  return (
    <div className="space-y-1">
      {conversationEvents.map((event) => (
        <EventBubble key={event.id} event={event} />
      ))}
    </div>
  );
}
```

**Step 2: Create ResponseInput component**

Create `web/src/components/response-input.tsx`:

```tsx
'use client';

import { useState, useRef, useEffect } from 'react';

interface ResponseInputProps {
  agentId: string;
  isWaiting: boolean;
}

export function ResponseInput({ agentId, isWaiting }: ResponseInputProps) {
  const [text, setText] = useState('');
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const wsUrl =
      `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws/agents/${agentId}`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onerror = () => ws.close();

    return () => { ws.close(); };
  }, [agentId]);

  function send() {
    if (!text.trim() || !wsRef.current) return;

    wsRef.current.send(JSON.stringify({
      type: 'response',
      text: text + '\n',
    }));
    setText('');
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  return (
    <div className="border-t border-gray-700 p-4 bg-gray-800/50">
      <div className="flex items-center gap-2 mb-2">
        <span className={`h-2 w-2 rounded-full ${connected ? 'bg-emerald-500' : 'bg-red-500'}`} />
        <span className="text-xs text-gray-500">
          {connected ? 'Connected' : 'Disconnected'}
        </span>
        {isWaiting && (
          <span className="text-xs text-amber-400 animate-pulse">Waiting for response...</span>
        )}
      </div>
      <div className="flex gap-2">
        <input
          type="text"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Type a response..."
          className="flex-1 rounded-lg bg-gray-900 border border-gray-700 px-3 py-2 text-sm text-gray-100 placeholder-gray-500 focus:border-blue-500 focus:outline-none"
        />
        <button
          onClick={send}
          disabled={!text.trim() || !connected}
          className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Send
        </button>
      </div>
    </div>
  );
}
```

**Step 3: Create conversation page**

Create `web/src/app/agents/[id]/page.tsx`:

```tsx
'use client';

import { use } from 'react';
import { useAgentStore } from '@/store/agents';
import { ConversationView } from '@/components/conversation-view';
import { ResponseInput } from '@/components/response-input';
import Link from 'next/link';

export default function AgentPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const agentId = decodeURIComponent(id);

  const agent = useAgentStore((s) => s.agents.get(agentId));
  const events = useAgentStore((s) => s.agentEvents.get(agentId) ?? []);

  return (
    <div className="flex flex-col h-[calc(100vh-73px)]">
      {/* Header */}
      <div className="border-b border-gray-800 px-6 py-4 flex items-center gap-4">
        <Link href="/" className="text-gray-400 hover:text-white text-sm">
          ← Back
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
      </div>

      {/* Conversation */}
      <div className="flex-1 overflow-y-auto px-6 py-4">
        <ConversationView events={events} />
      </div>

      {/* Response Input */}
      <ResponseInput agentId={agentId} isWaiting={agent?.status === 'waiting'} />
    </div>
  );
}
```

**Step 4: Build and verify**

```bash
cd web && npm run build && cd ..
```

Expected: builds successfully.

**Step 5: Commit**

```bash
git add web/src/
git commit -m "feat: full-screen conversation page with response input"
```

---

## Task 13: Frontend Embedding + End-to-End

**Files:**
- Create: `internal/frontend/embed.go`
- Create: `internal/frontend/dist/.gitkeep`
- Modify: `cmd/awayteam/main.go` (wire frontend FS)

**Step 1: Create embed.go**

Create `internal/frontend/embed.go`:

```go
package frontend

import "embed"

//go:embed all:dist
var Dist embed.FS
```

Create `internal/frontend/dist/.gitkeep` (placeholder until first build):

```bash
mkdir -p internal/frontend/dist
touch internal/frontend/dist/.gitkeep
```

**Step 2: Wire frontend into server**

Update `cmdServe` in `cmd/awayteam/main.go` to add frontend FS:

```go
// After creating srv:
frontendFS, err := fs.Sub(frontend.Dist, "dist")
if err != nil {
    log.Printf("warning: no embedded frontend: %v", err)
} else {
    srv.SetFrontendFS(frontendFS)
}
```

Add import: `"io/fs"` and `"github.com/jeremy/awayteam/internal/frontend"`

**Step 3: Build the full project**

```bash
cd web && npm install && npm run build && cd ..
rm -rf internal/frontend/dist
cp -r web/out internal/frontend/dist
go build ./cmd/awayteam
```

Expected: single `awayteam` binary with embedded frontend.

**Step 4: Manual end-to-end test**

```bash
# Start the dashboard
./awayteamserve &

# In another terminal, open browser to http://localhost:8080
# Should see the Kanban board (empty)

# Run a test agent
./awayteamagent --name "test-agent" bash -c 'echo "Starting..."; sleep 2; echo "Done!"'

# Should see agent appear in Kanban → Active → Done
```

**Step 5: Commit**

```bash
git add internal/frontend/ cmd/awayteam/
git commit -m "feat: embed frontend in Go binary, complete end-to-end flow"
```

---

## Task 14: Final Polish

**Files:**
- Modify: Various files for polish

**Step 1: Add browser notifications for waiting agents**

In `web/src/store/agents.ts`, add notification logic in `handleEvent`:

```typescript
// Inside handleEvent, after updating agent state:
if (event.type === 'question.asked' && typeof window !== 'undefined' && Notification.permission === 'granted') {
  const data = event.data as Record<string, unknown> | undefined;
  new Notification(`${event.agent_name || 'Agent'} needs input`, {
    body: (data?.question as string) ?? 'Response needed',
  });
}
```

Add notification permission request in `layout-shell.tsx`:

```typescript
// Inside LayoutShell, after useWebSocket:
useEffect(() => {
  if (typeof window !== 'undefined' && Notification.permission === 'default') {
    Notification.requestPermission();
  }
}, []);
```

**Step 2: Run full test suite**

```bash
go test ./... -v
cd web && npm run build && cd ..
```

Expected: all pass.

**Step 3: Final build**

```bash
make build
```

**Step 4: Commit**

```bash
git add -A
git commit -m "feat: browser notifications for waiting agents, final polish"
```

---

## Summary

| Task | Description | Key Files |
|------|-------------|-----------|
| 1 | Go module + config + Makefile | `go.mod`, `cmd/awayteam/main.go`, `internal/config/` |
| 2 | Event model with validation | `internal/events/` |
| 3 | SQLite store | `internal/store/` |
| 4 | WebSocket hub | `internal/ws/` |
| 5 | HTTP server + REST API | `internal/server/` |
| 6 | PTY proxy | `internal/agent/` |
| 7 | Hook commands | `internal/hook/` |
| 8 | Install command | `cmd/awayteam/main.go` |
| 9 | Frontend setup | `web/` scaffolding |
| 10 | Zustand store + WS hook | `web/src/store/`, `web/src/hooks/` |
| 11 | Kanban board | `web/src/app/page.tsx`, `web/src/components/agent-card.tsx` |
| 12 | Conversation page | `web/src/app/agents/[id]/`, conversation components |
| 13 | Frontend embedding | `internal/frontend/`, end-to-end wiring |
| 14 | Final polish | Notifications, cleanup |
