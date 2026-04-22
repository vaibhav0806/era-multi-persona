package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/vaibhav0806/pi-agent/internal/config"
)

var version = "0.0.1-m0"

func main() {
	if err := godotenv.Load(); err != nil {
		// .env is optional in production; log and continue
		slog.Info(".env not loaded", "err", err)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	slog.Info("orchestrator starting",
		"version", version,
		"db_path", cfg.DBPath,
		"sandbox_repo", cfg.GitHubSandboxRepo,
	)
	slog.Info("orchestrator exiting (no work to do yet)")
}
