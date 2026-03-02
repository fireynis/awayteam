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
