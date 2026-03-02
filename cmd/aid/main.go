package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jeremy/ai-dashboard/internal/config"
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

	log.Printf("aid dashboard starting on :%d", cfg.Server.Port)
	// Server will be wired in later tasks
}
