package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/vaibhav0806/era/internal/config"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/queue"
	"github.com/vaibhav0806/era/internal/runner"
	"github.com/vaibhav0806/era/internal/telegram"
)

var version = "0.0.1-m0"

// pollInterval is how often the orchestrator checks for queued tasks.
// 2s is short enough to feel responsive and long enough to stay cheap.
const pollInterval = 2 * time.Second

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Info(".env not loaded", "err", err)
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handle, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		fail(err)
	}
	defer handle.Close()
	repo := db.NewRepo(handle)

	docker := &runner.Docker{
		Image:            "era-runner:m1",
		SandboxRepo:      cfg.GitHubSandboxRepo,
		GitHubPAT:        cfg.GitHubPAT,
		OpenRouterAPIKey: cfg.OpenRouterAPIKey,
		PiModel:          cfg.PiModel,
		MaxTokens:        cfg.MaxTokensPerTask,
		MaxCostCents:     cfg.MaxCostCentsPerTask,
		MaxIterations:    cfg.MaxIterationsPerTask,
		MaxWallSeconds:   cfg.MaxWallClockSeconds,
	}
	q := queue.New(repo, runner.QueueAdapter{D: docker})

	client, err := telegram.NewClient(cfg.TelegramToken, cfg.TelegramAllowedUserID)
	if err != nil {
		fail(err)
	}
	q.SetNotifier(&tgNotifier{
		client: client,
		chatID: cfg.TelegramAllowedUserID,
		repo:   cfg.GitHubSandboxRepo,
	})
	handler := telegram.NewHandler(client, q)

	updates, err := client.Updates(ctx)
	if err != nil {
		fail(err)
	}

	// Task-execution loop: poll the queue and run one task per tick.
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ran, err := q.RunNext(ctx)
				if err != nil {
					slog.Error("run next", "err", err)
				}
				if ran {
					slog.Info("task run cycle finished")
				}
			}
		}
	}()

	slog.Info("orchestrator ready",
		"version", version,
		"db_path", cfg.DBPath,
		"sandbox_repo", cfg.GitHubSandboxRepo,
	)

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case u, ok := <-updates:
			if !ok {
				slog.Info("updates channel closed")
				return
			}
			if err := handler.Handle(ctx, u); err != nil {
				slog.Error("handler", "err", err)
			}
		}
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
	os.Exit(1)
}

type tgNotifier struct {
	client telegram.Client
	chatID int64
	repo   string // "owner/repo"
}

func (n *tgNotifier) NotifyCompleted(ctx context.Context, id int64, branch, summary string, tokens int64, costCents int) {
	var msg string
	if branch == "" {
		msg = fmt.Sprintf("task #%d: no changes\nsummary: %s\ntokens: %d  cost: $%.2f",
			id, summary, tokens, float64(costCents)/100.0)
	} else {
		msg = fmt.Sprintf(
			"task #%d completed\nbranch: %s\nhttps://github.com/%s/tree/%s\nsummary: %s\ntokens: %d  cost: $%.2f",
			id, branch, n.repo, branch, summary, tokens, float64(costCents)/100.0,
		)
	}
	if err := n.client.SendMessage(ctx, n.chatID, msg); err != nil {
		slog.Error("notify completed", "err", err, "task", id)
	}
}

func (n *tgNotifier) NotifyFailed(ctx context.Context, id int64, reason string) {
	msg := fmt.Sprintf("task #%d failed: %s", id, reason)
	if err := n.client.SendMessage(ctx, n.chatID, msg); err != nil {
		slog.Error("notify failed", "err", err, "task", id)
	}
}

// compile-time assertion that tgNotifier satisfies queue.Notifier
var _ queue.Notifier = (*tgNotifier)(nil)
