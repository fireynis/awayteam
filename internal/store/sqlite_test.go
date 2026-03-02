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
