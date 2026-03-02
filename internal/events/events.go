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
