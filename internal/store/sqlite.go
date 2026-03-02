package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jeremy/ai-dashboard/internal/events"
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
