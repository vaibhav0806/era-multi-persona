package main

import (
	"fmt"
	"log/slog"
	"os"
)

// Exit codes follow the M0 runner convention: 0 = success, non-zero = failure
// with details on stderr. The RESULT line (Task M1-9) goes to stdout on success.
func main() {
	cfg, err := loadRunnerConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "runner config: %v\n", err)
		os.Exit(2)
	}
	slog.Info("runner starting",
		"task_id", cfg.TaskID,
		"repo", cfg.GitHubRepo,
		"model", cfg.PiModel,
		"max_cost_cents", cfg.MaxCostCents,
	)
	// Real implementation starts in Task M1-6.
	fmt.Fprintln(os.Stderr, "runner scaffold OK — real implementation in Task M1-6")
	os.Exit(0)
}
