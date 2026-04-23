package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/vaibhav0806/era/internal/config"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/diffscan"
	"github.com/vaibhav0806/era/internal/digest"
	"github.com/vaibhav0806/era/internal/githubapp"
	"github.com/vaibhav0806/era/internal/githubbranch"
	"github.com/vaibhav0806/era/internal/githubcompare"
	"github.com/vaibhav0806/era/internal/queue"
	"github.com/vaibhav0806/era/internal/runner"
	"github.com/vaibhav0806/era/internal/telegram"
)

// Defense-in-depth secret scrubbing at the Telegram boundary. The runner
// already scrubs in cmd/runner/git.go; this catches anything that slips past.
var (
	tokenizedURLPat = regexp.MustCompile(`(https://x-access-token:)[^@]+@`)
	classicPATPat   = regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`)
	finePATPat      = regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)
)

func scrubSecrets(s string) string {
	s = tokenizedURLPat.ReplaceAllString(s, "$1***@")
	s = classicPATPat.ReplaceAllString(s, "ghp_***")
	s = finePATPat.ReplaceAllString(s, "github_pat_***")
	return s
}

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
		Image:            "era-runner:m2",
		SandboxRepo:      cfg.GitHubSandboxRepo,
		OpenRouterAPIKey: cfg.OpenRouterAPIKey,
		PiModel:          cfg.PiModel,
		MaxTokens:        cfg.MaxTokensPerTask,
		MaxCostCents:     cfg.MaxCostCentsPerTask,
		MaxIterations:    cfg.MaxIterationsPerTask,
		MaxWallSeconds:   cfg.MaxWallClockSeconds,
	}

	appClient, err := githubapp.New(githubapp.Config{
		AppID:            cfg.GitHubAppID,
		InstallationID:   cfg.GitHubAppInstallationID,
		PrivateKeyBase64: cfg.GitHubAppPrivateKeyBase64,
	})
	if err != nil {
		fail(fmt.Errorf("github app init: %w", err))
	}
	var tokenSource queue.TokenSource = appClient
	slog.Info("github app token source configured",
		"app_id", cfg.GitHubAppID, "installation_id", cfg.GitHubAppInstallationID)

	compareClient := githubcompare.New("", appClient)
	branchDeleter := githubbranch.New("", appClient)
	q := queue.New(repo, runner.QueueAdapter{D: docker}, tokenSource, compareClient, cfg.GitHubSandboxRepo)
	q.SetBranchDeleter(branchDeleter)

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

	hour, minute, _ := config.ParseDigestTime(cfg.DigestTimeUTC)
	go runDigestScheduler(ctx, hour, minute, repo, client, cfg.TelegramAllowedUserID)

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
	msg := fmt.Sprintf("task #%d failed: %s", id, scrubSecrets(reason))
	if err := n.client.SendMessage(ctx, n.chatID, msg); err != nil {
		slog.Error("notify failed", "err", err, "task", id)
	}
}

func (n *tgNotifier) NotifyNeedsReview(ctx context.Context, a queue.NeedsReviewArgs) {
	body := formatNeedsReviewMessage(a)

	buttons := [][]telegram.InlineButton{
		{
			{Text: "✓ Approve", CallbackData: fmt.Sprintf("approve:%d", a.TaskID)},
			{Text: "✗ Reject", CallbackData: fmt.Sprintf("reject:%d", a.TaskID)},
		},
	}

	_, err := n.client.SendMessageWithButtons(ctx, n.chatID, body, buttons)
	if err != nil {
		slog.Error("notify needs_review", "err", err, "task", a.TaskID)
	}
}

// telegramMaxChars is the budget we allow for the approval DM body.
// Telegram caps messages at 4096 chars; we leave ~300 chars headroom.
const telegramMaxChars = 3800

// formatNeedsReviewMessage composes the human-readable approval DM.
func formatNeedsReviewMessage(a queue.NeedsReviewArgs) string {
	var b strings.Builder
	fmt.Fprintf(&b, "task #%d — needs review\n", a.TaskID)
	fmt.Fprintf(&b, "branch: %s\n", a.Branch)
	fmt.Fprintf(&b, "tokens: %d  cost: $%.4f\n\n", a.Tokens, float64(a.CostCents)/100.0)
	b.WriteString("findings:\n")
	for _, f := range a.Findings {
		fmt.Fprintf(&b, "  • %s (%s): %s\n", f.Rule, f.Path, truncate(f.Message, 100))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "compare: %s\n\n", a.CompareURL)
	b.WriteString("diff preview:\n")
	diffPreview := buildDiffPreview(a.Diffs, telegramMaxChars-b.Len()-200)
	b.WriteString(diffPreview)
	return b.String()
}

// buildDiffPreview renders up to budget chars of unified-diff-style preview
// across all files in order.
func buildDiffPreview(files []diffscan.FileDiff, budget int) string {
	if budget <= 0 {
		return "(diff too large to preview; open the compare link)"
	}
	var b strings.Builder
	for _, f := range files {
		if b.Len() > budget {
			b.WriteString("\n…(truncated)")
			return b.String()
		}
		deletedSuffix := map[bool]string{true: " (deleted)", false: ""}[f.Deleted]
		fmt.Fprintf(&b, "--- %s%s\n", f.Path, deletedSuffix)
		for _, line := range f.Removed {
			if b.Len() > budget {
				b.WriteString("\n…(truncated)")
				return b.String()
			}
			fmt.Fprintf(&b, "- %s\n", line)
		}
		for _, line := range f.Added {
			if b.Len() > budget {
				b.WriteString("\n…(truncated)")
				return b.String()
			}
			fmt.Fprintf(&b, "+ %s\n", line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// compile-time assertion that tgNotifier satisfies queue.Notifier
var _ queue.Notifier = (*tgNotifier)(nil)

// runDigestScheduler fires once per day at hour:minute UTC and sends a
// digest message to chatID. Respects ctx for graceful shutdown.
func runDigestScheduler(ctx context.Context, hour, minute int, repo *db.Repo, client telegram.Client, chatID int64) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		wait := time.Until(next)
		slog.Info("digest scheduled", "fires_at_utc", next.Format(time.RFC3339), "in", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		// Fire: fetch last 24h, render, send.
		to := time.Now().UTC()
		from := to.Add(-24 * time.Hour)
		tasks, err := repo.ListBetween(ctx, from, to)
		if err != nil {
			slog.Error("digest listbetween", "err", err)
			continue
		}
		msg := digest.Render(tasks, from, to)
		if err := client.SendMessage(ctx, chatID, msg); err != nil {
			slog.Error("digest send", "err", err)
			continue
		}
		slog.Info("digest sent", "tasks", len(tasks))
	}
}
