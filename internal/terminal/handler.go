package terminal

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type Handler struct {
	upgrader websocket.Upgrader
}

func NewHandler() *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// ServeWebSocket serves a terminal WebSocket for a specific command.
// If tmuxSession is non-empty, it attaches to that tmux session.
// If tmuxSession is empty, it spawns a login shell.
func (h *Handler) ServeWebSocket(w http.ResponseWriter, r *http.Request, tmuxSession string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var cmd *exec.Cmd
	if tmuxSession != "" {
		cmd = exec.Command("tmux", "attach-session", "-t", tmuxSession)
	} else {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
		cmd = exec.Command(shell)
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("terminal: failed to start pty: %v", err)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "failed to start terminal"))
		return
	}
	defer ptmx.Close()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			ptmx.Close()
			if cmd.Process != nil {
				cmd.Process.Signal(syscall.SIGHUP)
			}
		})
	}
	defer cleanup()

	// PTY -> WebSocket (binary frames)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		cleanup()
		conn.Close()
	}()

	// WebSocket -> PTY
	// Text frames with {"type":"resize"} are resize commands.
	// Binary frames are raw terminal input.
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if msgType == websocket.TextMessage {
			var rm resizeMsg
			if json.Unmarshal(msg, &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
				pty.Setsize(ptmx, &pty.Winsize{Cols: rm.Cols, Rows: rm.Rows})
			}
			continue
		}

		if msgType == websocket.BinaryMessage {
			if _, err := ptmx.Write(msg); err != nil {
				if err == io.EOF {
					break
				}
			}
		}
	}

	cleanup()
	cmd.Wait()
}
