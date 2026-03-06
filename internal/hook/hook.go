package hook

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

func ProcessHook(hookType, serverURL string) error {
	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	agentID := os.Getenv("AWAYTEAM_AGENT_ID")
	if agentID == "" {
		agentID = "hook-" + uuid.NewString()[:8]
	}

	var parsed map[string]any
	json.Unmarshal(payload, &parsed)

	var eventType, status string
	var data map[string]any

	switch hookType {
	case "user-prompt-submit":
		eventType = "message.user"
		status = "active"
		prompt, _ := parsed["prompt"].(string)
		data = map[string]any{"content": prompt}

	case "notification":
		eventType = "message.assistant"
		status = "active"
		message, _ := parsed["message"].(string)
		if message == "" {
			message, _ = parsed["title"].(string)
		}
		data = map[string]any{"content": message}

	case "post-tool-use":
		eventType = "tool.result"
		status = "active"
		toolName := stringField(parsed, "tool_name", "toolName")
		data = map[string]any{
			"tool":   toolName,
			"result": anyField(parsed, "tool_response", "toolResponse", "tool_result", "toolResult"),
		}

	case "stop":
		eventType = "message.assistant"
		status = "active"
		transcriptPath := stringField(parsed, "transcript_path", "transcriptPath")
		content := extractLastAssistantMessage(transcriptPath)
		if content == "" {
			return nil
		}
		data = map[string]any{"content": content}

	default:
		eventType = "hook." + hookType
		status = "active"
		data = parsed
	}

	dataJSON, _ := json.Marshal(data)

	evt := map[string]any{
		"id":         uuid.NewString(),
		"type":       eventType,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"agent_id":   agentID,
		"agent_type": "claude-code",
		"agent_name": os.Getenv("AWAYTEAM_AGENT_NAME"),
		"status":     status,
		"data":       json.RawMessage(dataJSON),
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

// extractLastAssistantMessage reads a Claude Code transcript JSONL file
// and returns the text content of the last assistant message.
func extractLastAssistantMessage(transcriptPath string) string {
	if transcriptPath == "" {
		return ""
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastText string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry["type"] != "assistant" {
			continue
		}
		msg, ok := entry["message"].(map[string]any)
		if !ok {
			continue
		}
		content, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		var texts []string
		for _, c := range content {
			block, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			}
		}
		if len(texts) > 0 {
			lastText = strings.Join(texts, "\n")
		}
	}

	return lastText
}

// stringField returns the first non-empty string value found among the given keys.
func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// anyField returns the first non-nil value found among the given keys.
func anyField(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}
