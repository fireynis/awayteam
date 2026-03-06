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
	"os/user"
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

	childEnv := append(os.Environ(),
		"AWAYTEAM_AGENT_ID="+agentID,
		"AWAYTEAM_AGENT_NAME="+cfg.Name,
		"AWAYTEAM_SERVER_URL="+cfg.ServerURL,
	)

	// Detect tmux and attempt to create a session
	useTmux := false
	sessionName := ""
	if hasTmux() {
		sessionName = tmuxSessionName(agentID)
		_, err := startTmuxSession(sessionName, cfg.Command, cfg.Args, childEnv)
		if err != nil {
			log.Printf("warning: tmux session creation failed, falling back to direct PTY: %v", err)
		} else {
			useTmux = true
		}
	}

	// Build connection info
	connInfo := map[string]string{}
	if hostname, err := os.Hostname(); err == nil {
		connInfo["hostname"] = hostname
	}
	if u, err := user.Current(); err == nil {
		connInfo["username"] = u.Username
	}
	if useTmux {
		connInfo["tmux_session"] = sessionName
		connInfo["tmux_socket"] = awayteamSocketPath()
		if hostname, ok := connInfo["hostname"]; ok {
			if username, ok := connInfo["username"]; ok {
				connInfo["ssh_command"] = fmt.Sprintf("ssh %s@%s", username, hostname)
			}
		}
		connInfo["tmux_command"] = fmt.Sprintf("tmux -S %s attach-session -t %s", awayteamSocketPath(), sessionName)
	}

	// Set up the command to run in the PTY
	var cmd *exec.Cmd
	if useTmux {
		// Attach to the tmux session instead of running the child command directly
		socketPath := awayteamSocketPath()
		cmd = exec.Command("tmux", "-S", socketPath, "attach-session", "-t", sessionName)
		cmd.Env = childEnv
		defer killTmuxSession(sessionName)
	} else {
		cmd = exec.Command(cfg.Command, cfg.Args...)
		cmd.Env = childEnv
	}

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

	// Post session.start event with connection info
	connInfoData, _ := json.Marshal(connInfo)
	postEvent(cfg.ServerURL, newEvent(agentID, cfg.Name, cfg.AgentType, "session.start", "active", connInfoData))

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

// hasTmux checks if tmux is available on the system.
func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// tmuxSessionName returns a short, deterministic tmux session name for an agent.
func tmuxSessionName(agentID string) string {
	short := agentID
	if len(short) > 8 {
		short = short[:8]
	}
	return "awayteam-" + short
}

// awayteamSocketPath returns a dedicated tmux socket path for awayteam sessions.
func awayteamSocketPath() string {
	dir := os.TempDir()
	return fmt.Sprintf("%s/awayteam-tmux-%d", dir, os.Getuid())
}

// startTmuxSession creates a new tmux session running the given command.
func startTmuxSession(name string, cmd string, args []string, env []string) (string, error) {
	socketPath := awayteamSocketPath()
	tmuxArgs := []string{"-S", socketPath, "new-session", "-d", "-s", name, "-x", "200", "-y", "50", "--"}
	tmuxArgs = append(tmuxArgs, cmd)
	tmuxArgs = append(tmuxArgs, args...)

	tmuxCmd := exec.Command("tmux", tmuxArgs...)
	tmuxCmd.Env = env
	var stderr bytes.Buffer
	tmuxCmd.Stderr = &stderr
	if err := tmuxCmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("tmux new-session: %w (%s)", err, detail)
		}
		return "", fmt.Errorf("tmux new-session: %w", err)
	}
	// Set env var so terminal handler can find the socket
	os.Setenv("AWAYTEAM_TMUX_SOCKET", socketPath)
	return name, nil
}

// killTmuxSession kills a tmux session by name. Errors are ignored.
func killTmuxSession(name string) {
	socketPath := awayteamSocketPath()
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", name).Run()
}
