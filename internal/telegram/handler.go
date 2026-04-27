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
	"github.com/vaibhav0806/era/internal/persona"
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

// PersonaMintResult bundles the persona-mint output the handler renders into
// the Telegram DM. ENSSubname may be empty when ENS isn't wired or when the
// post-mint sync failed (Phase 5 reconcile retries those cases). MintTxHash
// is populated by the iNFT provider; SystemPromptURI is the 0G Storage URI
// returned by PromptStorage.UploadPrompt before the mint call.
type PersonaMintResult struct {
	TokenID         string
	MintTxHash      string
	ENSSubname      string
	SystemPromptURI string
}

// Ops is the subset of orchestrator functionality the handler needs. Kept
// narrow so we can stub it in tests. Implemented by internal/queue.Queue in
// Task 10.
//
// M7-F.3 added MintPersona + ListPersonas for the /persona-mint and
// /personas commands. The persona row type is internal/persona.Persona —
// imported directly here to avoid a queue ↔ telegram import cycle (the
// queue package re-exports it as queue.Persona for its own callers).
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

	// M7-F.3: persona registry ops.
	MintPersona(ctx context.Context, name, systemPrompt string) (PersonaMintResult, error)
	ListPersonas(ctx context.Context) ([]persona.Persona, error)
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

	case strings.HasPrefix(text, "/persona-mint "):
		args := strings.TrimSpace(strings.TrimPrefix(text, "/persona-mint "))
		name, prompt, perr := parsePersonaMintArgs(args)
		if perr != nil {
			_, sErr := h.client.SendMessage(ctx, u.ChatID,
				"usage: /persona-mint <name> <prompt>\n"+
					"  name: 3-32 chars, lowercase letters/digits/dashes, not 'planner'/'coder'/'reviewer'\n"+
					"  prompt: 20-4000 chars\n"+
					"  error: "+perr.Error())
			return sErr
		}
		res, err := h.ops.MintPersona(ctx, name, prompt)
		if errors.Is(err, persona.ErrPersonaNameTaken) {
			_, sErr := h.client.SendMessage(ctx, u.ChatID,
				fmt.Sprintf("name %q already taken — pick another", name))
			return sErr
		}
		if err != nil {
			_, sErr := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("mint failed: %v", err))
			return sErr
		}
		body := fmt.Sprintf("✓ persona %q minted as token #%s", name, res.TokenID)
		if res.MintTxHash != "" {
			body += fmt.Sprintf("\n  chainscan: https://chainscan-galileo.0g.ai/tx/%s", res.MintTxHash)
		}
		if res.ENSSubname != "" {
			body += fmt.Sprintf("\n  ens: https://sepolia.app.ens.domains/%s", res.ENSSubname)
		}
		if res.SystemPromptURI != "" {
			body += fmt.Sprintf("\n  prompt: %s", res.SystemPromptURI)
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, body)
		return err

	case text == "/persona-mint":
		_, err := h.client.SendMessage(ctx, u.ChatID, "usage: /persona-mint <name> <prompt>")
		return err

	case text == "/personas":
		list, err := h.ops.ListPersonas(ctx)
		if err != nil {
			_, sErr := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
			return sErr
		}
		_, err = h.client.SendMessage(ctx, u.ChatID, formatPersonasDM(list))
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

// personaNameRE matches the on-chain canonical persona name shape: 3-32
// chars, lowercase alphanumerics + dashes only. Mirrors the constraint
// applied by the registry contract so we reject bad names client-side
// before paying gas.
var personaNameRE = regexp.MustCompile(`^[a-z0-9-]{3,32}$`)

// reservedPersonaNames are pre-minted by the orchestrator boot path
// (planner=token #0, coder=token #1, reviewer=token #2). Re-minting them
// would conflict; reject early.
var reservedPersonaNames = map[string]bool{
	"planner":  true,
	"coder":    true,
	"reviewer": true,
}

// parsePersonaMintArgs splits "<name> <prompt>" and validates both halves.
// Returns (name, prompt, nil) on success. On any failure the error message
// is human-readable and surfaced verbatim to the Telegram user.
func parsePersonaMintArgs(s string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(s), " ", 2)
	if len(parts) < 2 {
		return "", "", errors.New("missing prompt")
	}
	name := parts[0]
	prompt := strings.TrimSpace(parts[1])

	if !personaNameRE.MatchString(name) {
		return "", "", errors.New("invalid name (must be 3-32 lowercase alphanumerics + dashes)")
	}
	if reservedPersonaNames[name] {
		return "", "", fmt.Errorf("name %q is reserved", name)
	}
	if len(prompt) < 20 {
		return "", "", errors.New("prompt too short (min 20 chars)")
	}
	if len(prompt) > 4000 {
		return "", "", errors.New("prompt too long (max 4000 chars)")
	}
	return name, prompt, nil
}

// formatPersonasDM renders the /personas listing for the user. Empty list
// gets a hint to mint instead of a blank message.
func formatPersonasDM(personas []persona.Persona) string {
	if len(personas) == 0 {
		return "no personas yet — try /persona-mint <name> <prompt>"
	}
	var b strings.Builder
	b.WriteString("era personas\n────────────\n")
	for _, p := range personas {
		desc := p.Description
		if len(desc) > 50 {
			desc = desc[:50] + "…"
		}
		ens := p.ENSSubname
		if ens == "" {
			ens = "(no ens)"
		}
		fmt.Fprintf(&b, "#%s  %s · %s\n", p.TokenID, ens, desc)
	}
	return b.String()
}

func pctStr(x float64) string { return fmt.Sprintf("%.0f%%", x*100) }
func kStr(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%dk", n/1000)
}
func costStr(c int64) string { return fmt.Sprintf("$%.2f", float64(c)/100.0) }
