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
