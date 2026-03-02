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

func ProcessHook(hookType, serverURL string) error {
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	agentID := os.Getenv("AID_AGENT_ID")
	if agentID == "" {
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
		"agent_name": os.Getenv("AID_AGENT_NAME"),
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
