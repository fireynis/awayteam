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
