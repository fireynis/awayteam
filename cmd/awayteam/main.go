package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	ioFS "io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeremy/awayteam/internal/agent"
	"github.com/jeremy/awayteam/internal/config"
	"github.com/jeremy/awayteam/internal/frontend"
	"github.com/jeremy/awayteam/internal/hook"
	"github.com/jeremy/awayteam/internal/server"
	"github.com/jeremy/awayteam/internal/store"
	"github.com/jeremy/awayteam/internal/ws"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: awayteam <command> [args]\n")
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

	// Add embedded frontend
	frontendFS, err := ioFS.Sub(frontend.Dist, "dist")
	if err != nil {
		log.Printf("warning: no embedded frontend: %v", err)
	} else {
		srv.SetFrontendFS(frontendFS)
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("awayteam dashboard starting on %s", addr)
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
		fmt.Fprintf(os.Stderr, "Usage: awayteam agent [flags] <command> [args...]\n")
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
		fmt.Fprintf(os.Stderr, "Usage: awayteam hook <type>\n")
		fmt.Fprintf(os.Stderr, "Types: post-tool-use, notification, user-prompt-submit\n")
		os.Exit(1)
	}

	serverURL := os.Getenv("AWAYTEAM_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	if err := hook.ProcessHook(args[0], serverURL); err != nil {
		os.Exit(0)
	}
}

func cmdInstall(args []string) {
	if len(args) == 0 || args[0] != "claude-code" {
		fmt.Fprintf(os.Stderr, "Usage: awayteam install claude-code\n")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("install", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print what would be added without modifying settings")
	fs.Parse(args[1:])

	awayteamPath, err := os.Executable()
	if err != nil {
		log.Fatalf("could not determine awayteam binary path: %v", err)
	}

	wantHooks := map[string]string{
		"PostToolUse":      awayteamPath + " hook post-tool-use",
		"Notification":     awayteamPath + " hook notification",
		"UserPromptSubmit": awayteamPath + " hook user-prompt-submit",
	}

	settingsPath := settingsFilePath()

	settings, err := readJSONFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("failed to read %s: %v", settingsPath, err)
	}
	if settings == nil {
		settings = map[string]any{}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	added := 0
	for hookName, command := range wantHooks {
		if hookListContainsCommand(hooks[hookName], command) {
			continue
		}
		existing, _ := hooks[hookName].([]any)
		entry := map[string]any{
			"matcher": "",
			"hooks": []map[string]string{
				{"type": "command", "command": command},
			},
		}
		hooks[hookName] = append(existing, entry)
		added++
	}

	if added == 0 {
		fmt.Println("awayteam hooks already installed in", settingsPath)
		return
	}

	settings["hooks"] = hooks

	if *dryRun {
		data, _ := json.MarshalIndent(settings, "", "  ")
		fmt.Println("Would write to", settingsPath+":")
		fmt.Println(string(data))
		return
	}

	if err := writeJSONFile(settingsPath, settings); err != nil {
		log.Fatalf("failed to write %s: %v", settingsPath, err)
	}

	fmt.Printf("Added %d hook(s) to %s\n", added, settingsPath)
}

func settingsFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("could not determine home directory: %v", err)
	}
	return home + "/.claude/settings.json"
}

func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return result, nil
}

func writeJSONFile(path string, data map[string]any) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0644)
}

// hookListContainsCommand checks if a hook list already contains a specific command.
func hookListContainsCommand(hookList any, command string) bool {
	list, ok := hookList.([]any)
	if !ok {
		return false
	}
	for _, entry := range list {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := hMap["command"].(string); cmd == command {
				return true
			}
		}
	}
	return false
}
