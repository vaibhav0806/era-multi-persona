package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era/internal/audit"
	"github.com/vaibhav0806/era/internal/budget"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/diffscan"
	"github.com/vaibhav0806/era/internal/githubpr"
	"github.com/vaibhav0806/era/internal/progress"
	"github.com/vaibhav0806/era/internal/swarm"
	"github.com/vaibhav0806/era/internal/telegram"
)

// TokenSource yields a fresh (or cached-still-valid) installation token for
// git operations. Implementations: *githubapp.Client (prod), fakeTokens (tests).
// May be nil in Queue.tokens — RunNext passes "" to runner.Run in that case.
type TokenSource interface {
	InstallationToken(ctx context.Context) (string, error)
}

// DiffSource fetches per-file diffs for a base..head comparison.
// Implemented by *githubcompare.Client.
type DiffSource interface {
	Compare(ctx context.Context, repo, base, head string) ([]diffscan.FileDiff, error)
}

// Runner executes a task. ghToken is a per-task GitHub installation token
// (or empty string if no TokenSource is configured). repo is the resolved
// target repo (owner/repo) for this task. maxIter/maxCents/maxWallSec are
// per-task cap overrides resolved from the budget profile; 0 means use the
// runner's own defaults. onProgress is called for each PROGRESS event emitted
// by the container; implementations may ignore it (pass nil internally).
type Runner interface {
	Run(ctx context.Context, taskID int64, description string, ghToken string, repo string,
		maxIter, maxCents, maxWallSec int, readOnly bool, onProgress progress.Callback) (branch, summary string, tokens int64, costCents int, audits []audit.Entry, err error)
}

// Swarm is the queue's view of the era-brain swarm: planner before runner.Run,
// reviewer after. Defined here so queue tests can inject a stub.
type Swarm interface {
	Plan(ctx context.Context, args swarm.PlanArgs) (swarm.PlanResult, error)
	Review(ctx context.Context, args swarm.ReviewArgs) (swarm.ReviewResult, error)
}

// NeedsReviewArgs bundles the approval-DM payload. Lives in queue so tests
// can assert shape without importing telegram or diffscan types up there.
type NeedsReviewArgs struct {
	TaskID           int64
	Branch           string
	Summary          string
	Tokens           int64
	CostCents        int
	Findings         []diffscan.Finding
	Diffs            []diffscan.FileDiff
	PRURL            string // was CompareURL; now PR html_url (or branch URL fallback when PR creation fails)
	PlannerPlan      string
	ReviewerCritique string
	ReviewerDecision string // "approve" or "flag"
	Receipts         []brain.Receipt // [planner, coder, reviewer] in order
}

// CompletedArgs bundles the completion-DM payload so we can extend persona
// breakdown without touching the Notifier signature again. Mirrors NeedsReviewArgs shape.
type CompletedArgs struct {
	TaskID           int64
	Repo             string
	Branch           string
	PRURL            string
	Summary          string
	Tokens           int64
	CostCents        int
	Receipts         []brain.Receipt // [planner, coder, reviewer] in order
	PlannerPlan      string
	ReviewerCritique string
	ReviewerDecision string // "approve" or "flag"
}

// Notifier is called by RunNext when a task finishes. All methods are
// fire-and-forget — the notifier is expected to log its own errors and
// return promptly.
type Notifier interface {
	NotifyCompleted(ctx context.Context, args CompletedArgs)
	NotifyFailed(ctx context.Context, taskID int64, reason string)
	NotifyNeedsReview(ctx context.Context, args NeedsReviewArgs)
	NotifyCancelled(ctx context.Context, taskID int64)
}

// BranchDeleter deletes a remote branch. Implemented by internal/githubbranch
// (M3-14) using App installation tokens; may be nil for tests that don't
// exercise the reject path.
type BranchDeleter interface {
	DeleteBranch(ctx context.Context, repo, branch string) error
}

// PRCreator opens/closes GitHub pull requests. Implemented by internal/githubpr.
// Optional: nil creator means the queue skips PR creation.
type PRCreator interface {
	Create(ctx context.Context, args githubpr.CreateArgs) (*githubpr.PR, error)
	Close(ctx context.Context, repo string, number int) error
	DefaultBranch(ctx context.Context, repo string) (string, error)
	ApprovePR(ctx context.Context, repo string, number int, body string) error
	AddLabel(ctx context.Context, repo string, number int, label string) error
	AddComment(ctx context.Context, repo string, number int, body string) error
}

// ProgressEvent is the queue-layer counterpart to progress.Event.
// We re-declare here so callers (e.g., tgNotifier in cmd/orchestrator) can
// implement queue.ProgressNotifier without depending on internal/progress.
type ProgressEvent struct {
	Iter      int
	Action    string
	Tokens    int64
	CostCents int
}

type ProgressNotifier interface {
	NotifyProgress(ctx context.Context, taskID int64, ev ProgressEvent)
}

type Queue struct {
	repo             *db.Repo
	runner           Runner
	notifier         Notifier
	progressNotifier ProgressNotifier
	tokens           TokenSource     // may be nil
	compare          DiffSource      // may be nil
	repoFQN          string          // owner/repo for compare lookups
	branchDeleter    BranchDeleter   // may be nil
	prCreator        PRCreator       // may be nil
	killer           ContainerKiller // may be nil
	running          *RunningSet     // initialized in New
	swarm            Swarm           // may be nil
	userID           string
}

func New(repo *db.Repo, runner Runner, tokens TokenSource, compare DiffSource, repoFQN string) *Queue {
	return &Queue{
		repo:    repo,
		runner:  runner,
		tokens:  tokens,
		compare: compare,
		repoFQN: repoFQN,
		running: NewRunningSet(),
	}
}

// SetNotifier attaches a Notifier to this Queue. Safe to call once at
// startup; do not change mid-flight — RunNext reads the field without a lock.
func (q *Queue) SetNotifier(n Notifier) { q.notifier = n }

func (q *Queue) SetProgressNotifier(p ProgressNotifier) { q.progressNotifier = p }

// SetSwarm attaches a Swarm to this Queue. Safe to call once at startup.
func (q *Queue) SetSwarm(s Swarm) { q.swarm = s }

// SetUserID sets the user identity threaded into swarm Plan/Review calls.
func (q *Queue) SetUserID(id string) { q.userID = id }

func (q *Queue) CreateAskTask(ctx context.Context, desc, targetRepo string) (int64, error) {
	task, err := q.repo.CreateAskTask(ctx, desc, targetRepo)
	if err != nil {
		return 0, fmt.Errorf("create ask task: %w", err)
	}
	return task.ID, nil
}

func (q *Queue) CreateTask(ctx context.Context, desc, targetRepo, profile string) (int64, error) {
	if profile == "" {
		profile = "default"
	}
	t, err := q.repo.CreateTask(ctx, desc, targetRepo, profile)
	if err != nil {
		return 0, err
	}
	return t.ID, nil
}

func (q *Queue) TaskStatus(ctx context.Context, id int64) (string, error) {
	t, err := q.repo.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", telegram.ErrTaskNotFound
	}
	if err != nil {
		return "", err
	}
	return t.Status, nil
}

func (q *Queue) ListRecent(ctx context.Context, limit int) ([]telegram.TaskSummary, error) {
	rows, err := q.repo.ListRecent(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]telegram.TaskSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, telegram.TaskSummary{
			ID:          r.ID,
			Description: r.Description,
			Status:      r.Status,
			BranchName:  r.BranchName.String,
		})
	}
	return out, nil
}

// RunNext claims the next queued task, runs it via the attached Runner, and
// records the outcome. Returns (ran, err): ran=true if a task was claimed
// (even if it failed), ran=false if the queue was empty.
//
// The runner error is returned as-is so callers can log/notify. The task is
// still marked failed in the DB and a "failed" event is appended.
func (q *Queue) RunNext(ctx context.Context) (bool, error) {
	t, err := q.repo.ClaimNext(ctx)
	if errors.Is(err, db.ErrNoTasks) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim next: %w", err)
	}

	_ = q.repo.AppendEvent(ctx, t.ID, "started", "{}")

	effectiveRepo := t.TargetRepo
	if effectiveRepo == "" {
		effectiveRepo = q.repoFQN
	}

	var ghToken string
	if q.tokens != nil {
		tok, err := q.tokens.InstallationToken(ctx)
		if err != nil {
			_ = q.repo.AppendEvent(ctx, t.ID, "failed", quoteJSON("token mint: "+err.Error()))
			_ = q.repo.FailTask(ctx, t.ID, "token mint: "+err.Error())
			if q.notifier != nil {
				q.notifier.NotifyFailed(ctx, t.ID, "token mint: "+err.Error())
			}
			return true, err
		}
		ghToken = tok
	}

	profile := budget.Profiles[t.BudgetProfile]
	if profile.Name == "" {
		profile = budget.Profiles["default"] // unknown stored profile; safe fallback
	}

	progressCB := func(ev progress.Event) {
		if q.progressNotifier != nil {
			q.progressNotifier.NotifyProgress(ctx, t.ID, ProgressEvent{
				Iter:      ev.Iter,
				Action:    ev.Action,
				Tokens:    ev.Tokens,
				CostCents: ev.CostCents,
			})
		}
	}

	// Plan: run planner persona before the container starts.
	var planText string
	var plannerReceipt brain.Receipt
	if q.swarm != nil {
		pr, perr := q.swarm.Plan(ctx, swarm.PlanArgs{
			TaskID:          fmt.Sprintf("%d", t.ID),
			TaskDescription: t.Description,
			UserID:          q.userID,
		})
		if perr != nil {
			// Planner failure shouldn't block the task — log and continue.
			_ = q.repo.AppendEvent(ctx, t.ID, "planner_failed", quoteJSON(perr.Error()))
		} else {
			planText = pr.PlanText
			plannerReceipt = pr.Receipt
			_ = q.repo.AppendEvent(ctx, t.ID, "planner_ok", quoteJSON(planText))
		}
	}

	effectiveDesc := swarm.InjectPlan(t.Description, planText)

	readOnly := t.ReadOnly == 1
	branch, summary, tokens, costCents, audits, runErr := q.runner.Run(ctx, t.ID, effectiveDesc, ghToken, effectiveRepo,
		profile.MaxIter, profile.MaxCents, profile.MaxWallSec, readOnly, progressCB)
	if runErr != nil {
		if q.running != nil && q.running.WasKilled(t.ID) {
			q.running.ClearKilled(t.ID)
			_ = q.repo.AppendEvent(ctx, t.ID, "cancelled", "{}")
			if err := q.repo.SetStatus(ctx, t.ID, "cancelled"); err != nil {
				return true, fmt.Errorf("set cancelled: %w", err)
			}
			if q.notifier != nil {
				q.notifier.NotifyCancelled(ctx, t.ID)
			}
			return true, nil
		}
		_ = q.repo.AppendEvent(ctx, t.ID, "failed", quoteJSON(runErr.Error()))
		if ferr := q.repo.FailTask(ctx, t.ID, runErr.Error()); ferr != nil {
			return true, fmt.Errorf("fail task: %w (original: %v)", ferr, runErr)
		}
		if q.notifier != nil {
			q.notifier.NotifyFailed(ctx, t.ID, runErr.Error())
		}
		return true, runErr
	}

	_ = q.repo.AppendEvent(ctx, t.ID, "completed", "{}")
	if err := q.repo.CompleteTask(ctx, t.ID, branch, summary, tokens, int64(costCents)); err != nil {
		return true, fmt.Errorf("complete task: %w", err)
	}

	var prURL string
	var prNumber int
	base := "main"
	if q.prCreator != nil && branch != "" {
		if db, err := q.prCreator.DefaultBranch(ctx, effectiveRepo); err != nil {
			_ = q.repo.AppendEvent(ctx, t.ID, "default_branch_fallback", quoteJSON(err.Error()))
		} else if db != "" {
			base = db
		}
		pr, prErr := q.prCreator.Create(ctx, githubpr.CreateArgs{
			Repo:  effectiveRepo,
			Head:  branch,
			Base:  base,
			Title: "[era] " + Truncate(t.Description, 60),
			Body:  ComposePRBody(t.ID, branch, summary, tokens, costCents),
		})
		if prErr != nil {
			_ = q.repo.AppendEvent(ctx, t.ID, "pr_create_error", quoteJSON(prErr.Error()))
			prURL = fmt.Sprintf("https://github.com/%s/tree/%s", effectiveRepo, branch)
		} else {
			prNumber = pr.Number
			prURL = pr.HTMLURL
			_ = q.repo.AppendEvent(ctx, t.ID, "pr_opened", quoteJSON(pr.HTMLURL))
			_ = q.repo.SetPRNumber(ctx, t.ID, int64(prNumber))
		}
	} else if branch != "" {
		prURL = fmt.Sprintf("https://github.com/%s/tree/%s", effectiveRepo, branch)
	}

	for _, ae := range audits {
		payload, _ := json.Marshal(ae)
		_ = q.repo.AppendEvent(ctx, t.ID, "http_request", string(payload))
	}
	var flaggedFindings []diffscan.Finding
	var flaggedDiffs []diffscan.FileDiff
	if q.compare != nil && branch != "" {
		diffs, err := q.compare.Compare(ctx, effectiveRepo, base, branch)
		if err != nil {
			_ = q.repo.AppendEvent(ctx, t.ID, "diffscan_error", quoteJSON(err.Error()))
		} else {
			findings := diffscan.ScanDiffs(diffs)
			if len(findings) > 0 {
				payload, _ := json.Marshal(findings)
				_ = q.repo.AppendEvent(ctx, t.ID, "diffscan_flagged", string(payload))
				if err := q.repo.SetStatus(ctx, t.ID, "needs_review"); err != nil {
					_ = q.repo.AppendEvent(ctx, t.ID, "diffscan_setstatus_error", quoteJSON(err.Error()))
				}
				flaggedFindings = findings
				flaggedDiffs = diffs
			}
		}
	}

	// Review: fetch diff (best-effort), run reviewer persona.
	var reviewerReceipt brain.Receipt
	var reviewCritique string
	reviewDecision := swarm.DecisionApprove // default: approve when no swarm wired (preserves old behavior)

	if q.swarm != nil && branch != "" {
		var diffText string
		if q.compare != nil {
			if files, derr := q.compare.Compare(ctx, effectiveRepo, base, branch); derr == nil {
				diffText = renderDiffText(files)
			} else {
				_ = q.repo.AppendEvent(ctx, t.ID, "diff_fetch_failed", quoteJSON(derr.Error()))
			}
		}
		priorSealed := map[string]bool{
			"planner": plannerReceipt.Sealed,
			"coder":   false, // Pi is always unsealed in M7-C scope
		}
		rr, rerr := q.swarm.Review(ctx, swarm.ReviewArgs{
			TaskID:             fmt.Sprintf("%d", t.ID),
			TaskDescription:    t.Description, // original, not effectiveDesc
			PlanText:           planText,
			DiffText:           diffText,
			UserID:             q.userID,
			PriorPersonaSealed: priorSealed,
		})
		if rerr != nil {
			_ = q.repo.AppendEvent(ctx, t.ID, "reviewer_failed", quoteJSON(rerr.Error()))
			reviewDecision = swarm.DecisionFlag
		} else {
			reviewerReceipt = rr.Receipt
			reviewCritique = rr.CritiqueText
			reviewDecision = rr.Decision
			_ = q.repo.AppendEvent(ctx, t.ID, "reviewer_ok", quoteJSON(string(reviewDecision)))
		}
	}

	if q.notifier != nil {
		clean := len(flaggedFindings) == 0 && reviewDecision == swarm.DecisionApprove
		if clean {
			q.notifier.NotifyCompleted(ctx, CompletedArgs{
				TaskID:           t.ID,
				Repo:             effectiveRepo,
				Branch:           branch,
				PRURL:            prURL,
				Summary:          summary,
				Tokens:           tokens,
				CostCents:        costCents,
				Receipts:         []brain.Receipt{plannerReceipt, synthCoderReceipt(), reviewerReceipt},
				PlannerPlan:      planText,
				ReviewerCritique: reviewCritique,
				ReviewerDecision: string(reviewDecision),
			})
		} else {
			q.notifier.NotifyNeedsReview(ctx, NeedsReviewArgs{
				TaskID:           t.ID,
				Branch:           branch,
				Summary:          summary,
				Tokens:           tokens,
				CostCents:        costCents,
				Findings:         flaggedFindings,
				Diffs:            flaggedDiffs,
				PRURL:            prURL,
				PlannerPlan:      planText,
				ReviewerCritique: reviewCritique,
				ReviewerDecision: string(reviewDecision),
				Receipts:         []brain.Receipt{plannerReceipt, synthCoderReceipt(), reviewerReceipt},
			})
		}
	}
	return true, nil
}

// renderDiffText composes a unified-diff-shaped string from []diffscan.FileDiff
// for the reviewer persona. Lossy (no @@ context lines) but enough for the
// reviewer LLM to spot test removals, weakened assertions, and plan deviations.
func renderDiffText(files []diffscan.FileDiff) string {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "--- %s\n+++ %s\n", f.Path, f.Path)
		for _, line := range f.Removed {
			b.WriteString("-")
			b.WriteString(line)
			b.WriteString("\n")
		}
		for _, line := range f.Added {
			b.WriteString("+")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// synthCoderReceipt synthesizes a placeholder receipt for the coder persona.
// Pi runs inside the container; era-brain has no view of its prompt or diff
// body. M7-D's iNFT recordInvocation skips coder receipts where Sealed=false
// and hashes are empty, so this placeholder doesn't pollute on-chain state.
func synthCoderReceipt() brain.Receipt {
	return brain.Receipt{
		Persona:       "coder",
		Sealed:        false,
		TimestampUnix: time.Now().Unix(),
	}
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(map[string]string{"error": s})
	return string(b)
}

// SetBranchDeleter attaches a BranchDeleter to this Queue.
func (q *Queue) SetBranchDeleter(bd BranchDeleter) { q.branchDeleter = bd }

// SetPRCreator attaches a PRCreator to this Queue.
func (q *Queue) SetPRCreator(p PRCreator) { q.prCreator = p }

// SetKiller attaches a ContainerKiller to this Queue.
func (q *Queue) SetKiller(k ContainerKiller) { q.killer = k }

// Running returns the RunningSet for this Queue.
func (q *Queue) Running() *RunningSet { return q.running }

// ApproveTask transitions needs_review → approved. No-op on already-approved.
// Errors on any other current status. If a PR number is set and a PRCreator is
// configured, it labels the PR "era-approved" and submits an APPROVED review;
// failures are logged as events but do not block the state transition.
func (q *Queue) ApproveTask(ctx context.Context, id int64) error {
	task, err := q.repo.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	switch task.Status {
	case "approved":
		return nil // idempotent
	case "needs_review":
		// fall through
	default:
		return fmt.Errorf("cannot approve task in state %q", task.Status)
	}

	effectiveRepo := task.TargetRepo
	if effectiveRepo == "" {
		effectiveRepo = q.repoFQN
	}

	if task.PrNumber.Valid && q.prCreator != nil {
		n := int(task.PrNumber.Int64)
		if err := q.prCreator.AddLabel(ctx, effectiveRepo, n, "era-approved"); err != nil {
			_ = q.repo.AppendEvent(ctx, id, "pr_label_error", quoteJSON(err.Error()))
		} else {
			_ = q.repo.AppendEvent(ctx, id, "pr_labeled", "{}")
		}
		if err := q.prCreator.AddComment(ctx, effectiveRepo, n, "✓ Approved via era Telegram bot. Branch safe to merge."); err != nil {
			_ = q.repo.AppendEvent(ctx, id, "pr_comment_error", quoteJSON(err.Error()))
		} else {
			_ = q.repo.AppendEvent(ctx, id, "pr_commented_approved", "{}")
		}
	}

	if err := q.repo.SetStatus(ctx, id, "approved"); err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	_ = q.repo.AppendEvent(ctx, id, "approved", "{}")
	return nil
}

// RejectTask transitions needs_review → rejected, closes the PR, and deletes
// the branch. No-op on already-rejected (idempotent). Errors on other states.
// PR-close and branch-delete failures are logged as events but do not block
// the transition.
func (q *Queue) RejectTask(ctx context.Context, id int64) error {
	t, err := q.repo.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	switch t.Status {
	case "rejected":
		return nil // idempotent
	case "needs_review":
		// fall through
	default:
		return fmt.Errorf("cannot reject task in state %q", t.Status)
	}

	effectiveRepo := t.TargetRepo
	if effectiveRepo == "" {
		effectiveRepo = q.repoFQN
	}

	// 1. Comment, then close PR
	if t.PrNumber.Valid && q.prCreator != nil {
		n := int(t.PrNumber.Int64)
		findings := loadFindings(ctx, q.repo, id)
		commentBody := RejectionCommentBody(findings)
		if err := q.prCreator.AddComment(ctx, effectiveRepo, n, commentBody); err != nil {
			_ = q.repo.AppendEvent(ctx, id, "pr_comment_error", quoteJSON(err.Error()))
		} else {
			_ = q.repo.AppendEvent(ctx, id, "pr_commented_rejected", "{}")
		}
		if err := q.prCreator.Close(ctx, effectiveRepo, n); err != nil {
			_ = q.repo.AppendEvent(ctx, id, "pr_close_error", quoteJSON(err.Error()))
		} else {
			_ = q.repo.AppendEvent(ctx, id, "pr_closed", "{}")
		}
	}

	// 2. Delete branch
	if t.BranchName.Valid && q.branchDeleter != nil && t.BranchName.String != "" {
		if err := q.branchDeleter.DeleteBranch(ctx, effectiveRepo, t.BranchName.String); err != nil {
			_ = q.repo.AppendEvent(ctx, id, "branch_delete_error", quoteJSON(err.Error()))
		} else {
			_ = q.repo.AppendEvent(ctx, id, "branch_deleted", "{}")
		}
	}

	// 3. Transition task
	if err := q.repo.SetStatus(ctx, id, "rejected"); err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	_ = q.repo.AppendEvent(ctx, id, "rejected", "{}")
	return nil
}

// loadFindings fetches the diffscan_flagged event payload for a task and
// returns the parsed findings. Nil on any error or if no findings event exists.
func loadFindings(ctx context.Context, r *db.Repo, id int64) []diffscan.Finding {
	events, err := r.ListEvents(ctx, id)
	if err != nil {
		return nil
	}
	for _, e := range events {
		if e.Kind == "diffscan_flagged" {
			var findings []diffscan.Finding
			if err := json.Unmarshal([]byte(e.Payload), &findings); err == nil {
				return findings
			}
		}
	}
	return nil
}

// HandleApproval parses callback data "approve:<id>" / "reject:<id>" and
// dispatches. Returns the reply text (used by the callback answer).
func (q *Queue) HandleApproval(ctx context.Context, data string) (string, error) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("bad callback data: %q", data)
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("bad id: %w", err)
	}
	switch parts[0] {
	case "approve":
		if err := q.ApproveTask(ctx, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("task #%d approved", id), nil
	case "reject":
		if err := q.RejectTask(ctx, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("task #%d rejected", id), nil
	default:
		return "", fmt.Errorf("unknown action: %q", parts[0])
	}
}

// CancelTask transitions queued → cancelled (idempotent on already-cancelled).
// Running tasks are killed via the ContainerKiller; the runner's goroutine
// observes the kill error, checks WasKilled, and writes the terminal state.
// Other states error.
func (q *Queue) CancelTask(ctx context.Context, id int64) error {
	t, err := q.repo.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	switch t.Status {
	case "cancelled":
		return nil // idempotent
	case "queued":
		if err := q.repo.SetStatus(ctx, id, "cancelled"); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
		_ = q.repo.AppendEvent(ctx, id, "cancelled", "{}")
		return nil
	case "running":
		name, ok := q.running.Get(id)
		if !ok {
			return fmt.Errorf("task #%d is running but container not registered yet, retry shortly", id)
		}
		if q.killer == nil {
			return fmt.Errorf("no killer configured; cannot cancel running task")
		}
		q.running.MarkKilled(id) // flag BEFORE kill so the race is safe
		if err := q.killer.Kill(ctx, name); err != nil {
			return fmt.Errorf("docker kill: %w", err)
		}
		// Don't write status=cancelled here. RunNext observes the killed runner's
		// error, checks WasKilled, and writes the terminal state itself.
		return nil
	default:
		return fmt.Errorf("cannot cancel task in state %q", t.Status)
	}
}

// RetryTask creates a new queued task with the same description as the
// referenced prior task. The prior task's state is unchanged. Returns the
// new task ID.
func (q *Queue) RetryTask(ctx context.Context, id int64) (int64, error) {
	orig, err := q.repo.GetTask(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("get original task: %w", err)
	}
	newTask, err := q.repo.CreateTask(ctx, orig.Description, orig.TargetRepo, orig.BudgetProfile)
	if err != nil {
		return 0, fmt.Errorf("create retry task: %w", err)
	}
	_ = q.repo.AppendEvent(ctx, newTask.ID, "retried_from",
		fmt.Sprintf(`{"original_task_id":%d}`, id))
	return newTask.ID, nil
}

// compile-time assertion that Queue satisfies telegram.Ops
var _ telegram.Ops = (*Queue)(nil)
