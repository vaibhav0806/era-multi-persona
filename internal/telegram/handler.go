package telegram

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/vaibhav0806/era/internal/budget"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/replyprompt"
	"github.com/vaibhav0806/era/internal/stats"
)

// repoFmtRE matches owner/repo, allowing word chars, dots, dashes.
// Examples it matches: vaibhav0806/era, alice/foo-bar, x/y.z
// Examples it doesn't: just-text, /no-leading-slash, foo
var repoFmtRE = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

// parseTaskArgs splits the argument to /task into (repo, description).
// If the first token looks like owner/repo, it's the repo and remaining
// text is the description. Otherwise repo is empty (caller uses default).
func parseTaskArgs(s string) (repo, desc string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	parts := strings.SplitN(s, " ", 2)
	if len(parts) >= 1 && repoFmtRE.MatchString(parts[0]) {
		if len(parts) == 2 {
			return parts[0], strings.TrimSpace(parts[1])
		}
		return parts[0], "" // repo only, no description
	}
	return "", s
}

var askRepoPattern = regexp.MustCompile(`^([\w.-]+/[\w.-]+)\s+(.+)$`)

// parseAskArgs splits "/ask <owner>/<repo> <question>" args into (repo, desc).
// Returns ("", "") if the format doesn't match.
func parseAskArgs(args string) (repo, desc string) {
	m := askRepoPattern.FindStringSubmatch(strings.TrimSpace(args))
	if len(m) != 3 {
		return "", ""
	}
	return m[1], m[2]
}

// ErrTaskNotFound is returned by Ops.TaskStatus when the task ID is unknown.
var ErrTaskNotFound = errors.New("task not found")

type TaskSummary struct {
	ID          int64
	Description string
	Status      string
	BranchName  string
}

// Ops is the subset of orchestrator functionality the handler needs. Kept
// narrow so we can stub it in tests. Implemented by internal/queue.Queue in
// Task 10.
type Ops interface {
	CreateTask(ctx context.Context, desc, targetRepo, profile string) (int64, error)
	CreateAskTask(ctx context.Context, desc, targetRepo string) (int64, error)
	TaskStatus(ctx context.Context, id int64) (string, error)
	ListRecent(ctx context.Context, limit int) ([]TaskSummary, error)
	HandleApproval(ctx context.Context, data string) (replyText string, err error)
	// M3-9 command wires: the interface methods are declared here;
	// the handler dispatch lines are added in M3-9.
	CancelTask(ctx context.Context, id int64) error
	RetryTask(ctx context.Context, id int64) (newID int64, err error)
	Stats(ctx context.Context) (stats.Stats, error)
}

type Handler struct {
	client      Client
	ops         Ops
	repo        *db.Repo // for GetTaskByCompletionMessageID
	sandboxRepo string   // reply-DM fallback when target_repo empty
}

func NewHandler(c Client, ops Ops, repo *db.Repo, sandboxRepo string) *Handler {
	return &Handler{client: c, ops: ops, repo: repo, sandboxRepo: sandboxRepo}
}

func (h *Handler) Handle(ctx context.Context, u Update) error {
	// M6 AI: reply-to-continue. A non-command text message with ReplyToMessageID
	// threads a follow-up task off the original. Commands always win.
	if u.ReplyToMessageID != 0 && u.Callback == nil && !strings.HasPrefix(u.Text, "/") {
		return h.handleReply(ctx, u)
	}

	// M3: callback queries (button taps)
	if u.Callback != nil {
		return h.handleCallback(ctx, u)
	}

	text := strings.TrimSpace(u.Text)
	switch {
	case strings.HasPrefix(text, "/task"):
		body := strings.TrimSpace(strings.TrimPrefix(text, "/task"))
		if body == "" {
			_, err := h.client.SendMessage(ctx, u.ChatID, "usage: /task [--budget=quick|default|deep] [owner/repo] <description>")
			return err
		}
		profile, body := budget.ParseBudgetFlag(body)
		repo, desc := parseTaskArgs(body)
		if desc == "" {
			_, err := h.client.SendMessage(ctx, u.ChatID, "usage: /task [--budget=quick|default|deep] [owner/repo] <description>")
			return err
		}
		id, err := h.ops.CreateTask(ctx, desc, repo, profile)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
			return err
		}
		if repo != "" {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d queued (repo: %s, profile: %s)", id, repo, profile))
			return err
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d queued (profile: %s)", id, profile))
		return err

	case strings.HasPrefix(text, "/status "):
		raw := strings.TrimSpace(strings.TrimPrefix(text, "/status "))
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, "usage: /status <task-id>")
			return err
		}
		status, err := h.ops.TaskStatus(ctx, id)
		if errors.Is(err, ErrTaskNotFound) {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d not found", id))
			return err
		}
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
			return err
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d: %s", id, status))
		return err

	case text == "/list":
		items, err := h.ops.ListRecent(ctx, 10)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
			return err
		}
		var b strings.Builder
		if len(items) == 0 {
			b.WriteString("no tasks yet")
		}
		for _, it := range items {
			fmt.Fprintf(&b, "#%d [%s] %s\n", it.ID, it.Status, it.Description)
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, b.String())
		return err

	case strings.HasPrefix(text, "/cancel "):
		raw := strings.TrimSpace(strings.TrimPrefix(text, "/cancel "))
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, "usage: /cancel <task-id>")
			return err
		}
		if err := h.ops.CancelTask(ctx, id); err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("cancel #%d failed: %v", id, err))
			return err
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d cancel requested", id))
		return err

	case strings.HasPrefix(text, "/retry "):
		raw := strings.TrimSpace(strings.TrimPrefix(text, "/retry "))
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, "usage: /retry <task-id>")
			return err
		}
		newID, err := h.ops.RetryTask(ctx, id)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("retry failed: %v", err))
			return err
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("retry queued as #%d (from #%d)", newID, id))
		return err

	case strings.HasPrefix(text, "/ask "):
		args := strings.TrimSpace(strings.TrimPrefix(text, "/ask "))
		repo, desc := parseAskArgs(args)
		if repo == "" {
			_, err := h.client.SendMessage(ctx, u.ChatID,
				"usage: /ask <owner>/<repo> <question>")
			return err
		}
		id, err := h.ops.CreateAskTask(ctx, desc, repo)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
			return err
		}
		_, err = h.client.SendMessage(ctx, u.ChatID,
			fmt.Sprintf("task #%d queued (ask, repo: %s)", id, repo))
		return err

	case text == "/ask":
		_, err := h.client.SendMessage(ctx, u.ChatID,
			"usage: /ask <owner>/<repo> <question>")
		return err

	case text == "/stats":
		s, err := h.ops.Stats(ctx)
		if err != nil {
			_, err := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
			return err
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, formatStatsDM(s))
		return err

	default:
		_, err := h.client.SendMessage(ctx, u.ChatID, "unknown command. try /task, /ask, /status, /list, /cancel, /retry, /stats")
		return err
	}
}

func (h *Handler) handleReply(ctx context.Context, u Update) error {
	orig, err := h.repo.GetTaskByCompletionMessageID(ctx, int64(u.ReplyToMessageID))
	if errors.Is(err, sql.ErrNoRows) {
		_, err := h.client.SendMessage(ctx, u.ChatID,
			"sorry, couldn't find the task you're replying to")
		return err
	}
	if err != nil {
		return fmt.Errorf("get task by message id: %w", err)
	}
	prompt := replyprompt.ComposeReplyPrompt(orig, u.Text)
	targetRepo := orig.TargetRepo
	if targetRepo == "" {
		targetRepo = h.sandboxRepo
	}
	id, err := h.ops.CreateTask(ctx, prompt, targetRepo, "default")
	if err != nil {
		return fmt.Errorf("queue reply task: %w", err)
	}
	_, err = h.client.SendMessage(ctx, u.ChatID,
		fmt.Sprintf("task #%d queued (reply to #%d, repo: %s)", id, orig.ID, targetRepo))
	return err
}

func (h *Handler) handleCallback(ctx context.Context, u Update) error {
	reply, err := h.ops.HandleApproval(ctx, u.Callback.Data)
	if err != nil {
		// Answer with error message so user sees it as a toast; do not
		// bubble up (Telegram callback errors shouldn't crash the loop).
		_ = h.client.AnswerCallback(ctx, u.Callback.ID, "error: "+err.Error())
		return nil
	}
	return h.client.AnswerCallback(ctx, u.Callback.ID, reply)
}

func formatStatsDM(s stats.Stats) string {
	return fmt.Sprintf(
		`era stats
────────────
            24h    7d     30d
tasks:      %-6d %-6d %-d
success:    %-6s %-6s %s
tokens:     %-6s %-6s %s
cost:       %-6s %-6s %s
queue: %d pending`,
		s.Last24h.TasksTotal, s.Last7d.TasksTotal, s.Last30d.TasksTotal,
		pctStr(s.Last24h.SuccessRate()), pctStr(s.Last7d.SuccessRate()), pctStr(s.Last30d.SuccessRate()),
		kStr(s.Last24h.Tokens), kStr(s.Last7d.Tokens), kStr(s.Last30d.Tokens),
		costStr(s.Last24h.CostCents), costStr(s.Last7d.CostCents), costStr(s.Last30d.CostCents),
		s.PendingQueue,
	)
}

func pctStr(x float64) string { return fmt.Sprintf("%.0f%%", x*100) }
func kStr(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%dk", n/1000)
}
func costStr(c int64) string { return fmt.Sprintf("$%.2f", float64(c)/100.0) }
