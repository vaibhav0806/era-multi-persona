package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/vaibhav0806/era/internal/audit"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/diffscan"
	"github.com/vaibhav0806/era/internal/githubpr"
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
// target repo (owner/repo) for this task.
type Runner interface {
	Run(ctx context.Context, taskID int64, description string, ghToken string, repo string) (branch, summary string, tokens int64, costCents int, audits []audit.Entry, err error)
}

// NeedsReviewArgs bundles the approval-DM payload. Lives in queue so tests
// can assert shape without importing telegram or diffscan types up there.
type NeedsReviewArgs struct {
	TaskID     int64
	Branch     string
	Summary    string
	Tokens     int64
	CostCents  int
	Findings   []diffscan.Finding
	Diffs      []diffscan.FileDiff
	PRURL string // was CompareURL; now PR html_url (or branch URL fallback when PR creation fails)
}

// Notifier is called by RunNext when a task finishes. All methods are
// fire-and-forget — the notifier is expected to log its own errors and
// return promptly.
type Notifier interface {
	NotifyCompleted(ctx context.Context, taskID int64, repo, branch, prURL, summary string, tokens int64, costCents int)
	NotifyFailed(ctx context.Context, taskID int64, reason string)
	NotifyNeedsReview(ctx context.Context, args NeedsReviewArgs)
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
}

type Queue struct {
	repo          *db.Repo
	runner        Runner
	notifier      Notifier
	tokens        TokenSource   // may be nil
	compare       DiffSource    // may be nil
	repoFQN       string        // owner/repo for compare lookups
	branchDeleter BranchDeleter // may be nil
	prCreator     PRCreator     // may be nil
	killer        ContainerKiller // may be nil
	running       *RunningSet     // initialized in New
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

func (q *Queue) CreateTask(ctx context.Context, desc, targetRepo string) (int64, error) {
	t, err := q.repo.CreateTask(ctx, desc, targetRepo)
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

	branch, summary, tokens, costCents, audits, runErr := q.runner.Run(ctx, t.ID, t.Description, ghToken, effectiveRepo)
	if runErr != nil {
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
	if q.notifier != nil {
		if len(flaggedFindings) > 0 {
			q.notifier.NotifyNeedsReview(ctx, NeedsReviewArgs{
				TaskID:    t.ID,
				Branch:    branch,
				Summary:   summary,
				Tokens:    tokens,
				CostCents: costCents,
				Findings:  flaggedFindings,
				Diffs:     flaggedDiffs,
				PRURL:     prURL,
			})
		} else {
			q.notifier.NotifyCompleted(ctx, t.ID, effectiveRepo, branch, prURL,
				summary, tokens, costCents)
		}
	}
	return true, nil
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
// Errors on any other current status.
func (q *Queue) ApproveTask(ctx context.Context, id int64) error {
	task, err := q.repo.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	switch task.Status {
	case "approved":
		return nil // idempotent
	case "needs_review":
		if err := q.repo.SetStatus(ctx, id, "approved"); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
		_ = q.repo.AppendEvent(ctx, id, "approved", "{}")
		return nil
	default:
		return fmt.Errorf("cannot approve task in state %q", task.Status)
	}
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

	// 1. Close PR first
	if t.PrNumber.Valid && q.prCreator != nil {
		if err := q.prCreator.Close(ctx, effectiveRepo, int(t.PrNumber.Int64)); err != nil {
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

// CancelTask transitions queued → cancelled. No-op on already-cancelled.
// Running tasks cannot be cancelled in M3 (requires docker kill; deferred).
// Other states error.
func (q *Queue) CancelTask(ctx context.Context, id int64) error {
	task, err := q.repo.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	switch task.Status {
	case "cancelled":
		return nil
	case "queued":
		if err := q.repo.SetStatus(ctx, id, "cancelled"); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
		_ = q.repo.AppendEvent(ctx, id, "cancelled", "{}")
		return nil
	case "running":
		return fmt.Errorf("running tasks cannot be cancelled (running task #%d will hit its wall-clock cap)", id)
	default:
		return fmt.Errorf("cannot cancel task in state %q", task.Status)
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
	newTask, err := q.repo.CreateTask(ctx, orig.Description, "")
	if err != nil {
		return 0, fmt.Errorf("create retry task: %w", err)
	}
	_ = q.repo.AppendEvent(ctx, newTask.ID, "retried_from",
		fmt.Sprintf(`{"original_task_id":%d}`, id))
	return newTask.ID, nil
}

// compile-time assertion that Queue satisfies telegram.Ops
var _ telegram.Ops = (*Queue)(nil)
