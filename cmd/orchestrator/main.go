package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"path/filepath"

	"github.com/joho/godotenv"
	brainsqlite "github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/openrouter"
	"github.com/vaibhav0806/era/internal/config"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/diffscan"
	"github.com/vaibhav0806/era/internal/digest"
	"github.com/vaibhav0806/era/internal/githubapp"
	"github.com/vaibhav0806/era/internal/githubbranch"
	"github.com/vaibhav0806/era/internal/githubcompare"
	"github.com/vaibhav0806/era/internal/githubpr"
	"github.com/vaibhav0806/era/internal/queue"
	"github.com/vaibhav0806/era/internal/runner"
	"github.com/vaibhav0806/era/internal/swarm"
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
	prClient := githubpr.New("", appClient)
	ra := &runner.QueueAdapter{D: docker}
	q := queue.New(repo, ra, tokenSource, compareClient, cfg.GitHubSandboxRepo)
	ra.SetRunning(q.Running())
	q.SetBranchDeleter(branchDeleter)
	q.SetPRCreator(prClient)
	q.SetKiller(queue.NewDockerKiller())

	// Task 3: wire era-brain swarm (planner + reviewer personas).
	plannerModel := envOrDefault("PI_BRAIN_PLANNER_MODEL", "openai/gpt-4o-mini")
	reviewerModel := envOrDefault("PI_BRAIN_REVIEWER_MODEL", "openai/gpt-4o-mini")
	plannerLLM := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: plannerModel})
	reviewerLLM := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: reviewerModel})
	brainDBPath := filepath.Join(filepath.Dir(cfg.DBPath), "era-brain.db")
	brainMem, err := brainsqlite.Open(brainDBPath)
	if err != nil {
		fail(fmt.Errorf("era-brain sqlite: %w", err))
	}
	defer brainMem.Close()
	sw := swarm.New(swarm.Config{
		PlannerLLM:  plannerLLM,
		ReviewerLLM: reviewerLLM,
		Memory:      brainMem,
	})
	q.SetSwarm(sw)

	if n, err := q.Reconcile(ctx); err != nil {
		slog.Error("reconcile", "err", err)
	} else if n > 0 {
		slog.Warn("reconciled orphan running tasks", "count", n)
	}

	client, err := telegram.NewClient(cfg.TelegramToken, cfg.TelegramAllowedUserID)
	if err != nil {
		fail(err)
	}
	notifier := &tgNotifier{
		client:      client,
		chatID:      cfg.TelegramAllowedUserID,
		sandboxRepo: cfg.GitHubSandboxRepo,
		repo:        repo,
	}
	q.SetNotifier(notifier)
	q.SetProgressNotifier(notifier)
	handler := telegram.NewHandler(client, q, repo, cfg.GitHubSandboxRepo)

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

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type tgNotifier struct {
	client       telegram.Client
	chatID       int64
	sandboxRepo  string   // "owner/repo"
	repo         *db.Repo // for SetCompletionMessageID
	progressMsgs sync.Map // taskID (int64) → telegram message ID (int64)
}

func (n *tgNotifier) NotifyCompleted(ctx context.Context, a queue.CompletedArgs) {
	repo := a.Repo
	if repo == "" {
		repo = n.sandboxRepo
	}
	_ = repo // used for fallback only; future Task 5 will render per-persona breakdown
	var msg string
	if a.Branch == "" {
		msg = fmt.Sprintf("task #%d: no changes\nsummary: %s\ntokens: %d  cost: $%.2f",
			a.TaskID, truncateForTelegram(a.Summary, 3500), a.Tokens, float64(a.CostCents)/100.0)
	} else {
		msg = fmt.Sprintf(
			"task #%d completed\nbranch: %s\n%s\nsummary: %s\ntokens: %d  cost: $%.2f",
			a.TaskID, a.Branch, a.PRURL, truncateForTelegram(a.Summary, 3500), a.Tokens, float64(a.CostCents)/100.0,
		)
	}
	msgID, err := n.client.SendMessage(ctx, n.chatID, msg)
	if err != nil {
		slog.Error("notify completed", "err", err, "task", a.TaskID)
		return
	}
	if err := n.repo.SetCompletionMessageID(ctx, a.TaskID, msgID); err != nil {
		slog.Warn("set completion message id", "err", err, "task", a.TaskID)
	}
}

func (n *tgNotifier) NotifyFailed(ctx context.Context, id int64, reason string) {
	msg := fmt.Sprintf("task #%d failed: %s", id, truncateForTelegram(scrubSecrets(reason), 3500))
	_, err := n.client.SendMessage(ctx, n.chatID, msg)
	if err != nil {
		slog.Error("notify failed", "err", err, "task", id)
	}
}

func (n *tgNotifier) NotifyCancelled(ctx context.Context, id int64) {
	msg := fmt.Sprintf("task #%d cancelled mid-run", id)
	_, err := n.client.SendMessage(ctx, n.chatID, msg)
	if err != nil {
		slog.Error("notify cancelled", "err", err, "task", id)
	}
}

func (n *tgNotifier) NotifyProgress(ctx context.Context, id int64, ev queue.ProgressEvent) {
	if ev.Action == "" {
		return
	}
	body := fmt.Sprintf("task #%d · iter %d · %s · $%.3f",
		id, ev.Iter, ev.Action, float64(ev.CostCents)/100.0)
	if existing, ok := n.progressMsgs.Load(id); ok {
		msgID := existing.(int64)
		if err := n.client.EditMessageText(ctx, n.chatID, int(msgID), body); err != nil {
			slog.Warn("edit progress", "err", err, "task", id)
		}
		return
	}
	msgID, err := n.client.SendMessage(ctx, n.chatID, body)
	if err != nil {
		slog.Warn("send progress", "err", err, "task", id)
		return
	}
	n.progressMsgs.Store(id, msgID)
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
	fmt.Fprintf(&b, "pr: %s\n\n", a.PRURL)
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

// truncateForTelegram caps s at `budget` bytes, appending a rune-safe footer
// if anything was dropped. Backs up past any partial multi-byte rune so the
// returned prefix is always valid UTF-8.
func truncateForTelegram(s string, budget int) string {
	if len(s) <= budget {
		return s
	}
	cut := budget
	// If s[cut] is a continuation byte (0b10xxxxxx), we're mid-rune.
	// Back up until cut sits at a rune-start boundary.
	for cut > 0 && cut < len(s) && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n…(%d bytes truncated)", len(s)-cut)
}

// compile-time assertions that tgNotifier satisfies both notifier interfaces
var _ queue.Notifier = (*tgNotifier)(nil)
var _ queue.ProgressNotifier = (*tgNotifier)(nil)

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
		_, err = client.SendMessage(ctx, chatID, msg)
		if err != nil {
			slog.Error("digest send", "err", err)
			continue
		}
		slog.Info("digest sent", "tasks", len(tasks))
	}
}
