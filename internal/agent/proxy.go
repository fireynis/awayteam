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
	ServerURL string
	Command   string
	Args      []string
}

func RunProxy(cfg ProxyConfig) error {
	agentID := uuid.NewString()

	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = append(os.Environ(),
		"AWAYTEAM_AGENT_ID="+agentID,
		"AWAYTEAM_AGENT_NAME="+cfg.Name,
		"AWAYTEAM_SERVER_URL="+cfg.ServerURL,
	)

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
		wsConn = nil
	}

	// Dashboard responses -> PTY stdin
	if wsConn != nil {
		go func() {
			defer wsConn.Close()
			for {
				_, msg, err := wsConn.ReadMessage()
				if err != nil {
					return
				}
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

	// Local stdin -> PTY
	go func() { io.Copy(ptmx, os.Stdin) }()

	// PTY -> local stdout + dashboard streaming
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])

			chunk := base64.StdEncoding.EncodeToString(buf[:n])
			data, _ := json.Marshal(map[string]string{"chunk": chunk})
			postEvent(cfg.ServerURL, newEvent(agentID, cfg.Name, cfg.AgentType, "output.stream", "active", data))
		}
		if err != nil {
			if errors.Is(err, syscall.EIO) {
				break
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
		return
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
