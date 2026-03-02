package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeremy/ai-dashboard/internal/agent"
	"github.com/jeremy/ai-dashboard/internal/config"
	"github.com/jeremy/ai-dashboard/internal/hook"
	"github.com/jeremy/ai-dashboard/internal/server"
	"github.com/jeremy/ai-dashboard/internal/store"
	"github.com/jeremy/ai-dashboard/internal/ws"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: aid <command> [args]\n")
		fmt.Fprintf(os.Stderr, "Commands: serve, agent, install, hook\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "agent":
		cmdAgent(os.Args[2:])
	case "hook":
		cmdHook(os.Args[2:])
	case "install":
		cmdInstall(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	st, err := store.NewSQLiteStore(cfg.Storage.SQLitePath)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	hub := ws.NewHub()
	srv := server.New(cfg, st, hub)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("aid dashboard starting on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
}

func cmdAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	name := fs.String("name", "", "agent name (shown in dashboard)")
	agentType := fs.String("type", "generic", "agent type")
	serverURL := fs.String("server", "http://localhost:8080", "dashboard server URL")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: aid agent [flags] <command> [args...]\n")
		os.Exit(1)
	}

	if *name == "" {
		*name = remaining[0]
	}

	cfg := agent.ProxyConfig{
		Name:      *name,
		AgentType: *agentType,
		ServerURL: *serverURL,
		Command:   remaining[0],
		Args:      remaining[1:],
	}

	if err := agent.RunProxy(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "agent error: %v\n", err)
		os.Exit(1)
	}
}

func cmdHook(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: aid hook <type>\n")
		fmt.Fprintf(os.Stderr, "Types: post-tool-use, notification, user-prompt-submit\n")
		os.Exit(1)
	}

	serverURL := os.Getenv("AID_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	if err := hook.ProcessHook(args[0], serverURL); err != nil {
		os.Exit(0)
	}
}

func cmdInstall(args []string) {
	if len(args) == 0 || args[0] != "claude-code" {
		fmt.Fprintf(os.Stderr, "Usage: aid install claude-code\n")
		os.Exit(1)
	}

	aidPath, err := os.Executable()
	if err != nil {
		log.Fatalf("could not determine aid binary path: %v", err)
	}

	hookConfig := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []map[string]string{
				{"type": "command", "command": aidPath + " hook post-tool-use"},
			},
			"Notification": []map[string]string{
				{"type": "command", "command": aidPath + " hook notification"},
			},
		},
	}

	data, _ := json.MarshalIndent(hookConfig, "", "  ")
	fmt.Println("Add the following to your ~/.claude/settings.json or project .claude/settings.json:")
	fmt.Println()
	fmt.Println(string(data))
	fmt.Println()
	fmt.Printf("Or run: aid agent --name '<name>' claude\n")
	fmt.Println("to start Claude Code with the PTY proxy (recommended).")
}
