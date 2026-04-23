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
// (or empty string if no TokenSource is configured).
type Runner interface {
	Run(ctx context.Context, taskID int64, description string, ghToken string) (branch, summary string, tokens int64, costCents int, audits []audit.Entry, err error)
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
	CompareURL string // e.g. https://github.com/<repo>/compare/main...<branch>
}

// Notifier is called by RunNext when a task finishes. All methods are
// fire-and-forget — the notifier is expected to log its own errors and
// return promptly.
type Notifier interface {
	NotifyCompleted(ctx context.Context, taskID int64, branch, summary string, tokens int64, costCents int)
	NotifyFailed(ctx context.Context, taskID int64, reason string)
	NotifyNeedsReview(ctx context.Context, args NeedsReviewArgs)
}

// BranchDeleter deletes a remote branch. Implemented by internal/githubbranch
// (M3-14) using App installation tokens; may be nil for tests that don't
// exercise the reject path.
type BranchDeleter interface {
	DeleteBranch(ctx context.Context, repo, branch string) error
}

type Queue struct {
	repo          *db.Repo
	runner        Runner
	notifier      Notifier
	tokens        TokenSource   // may be nil
	compare       DiffSource    // may be nil
	repoFQN       string        // owner/repo for compare lookups
	branchDeleter BranchDeleter // may be nil
}

func New(repo *db.Repo, runner Runner, tokens TokenSource, compare DiffSource, repoFQN string) *Queue {
	return &Queue{
		repo:    repo,
		runner:  runner,
		tokens:  tokens,
		compare: compare,
		repoFQN: repoFQN,
	}
}

// SetNotifier attaches a Notifier to this Queue. Safe to call once at
// startup; do not change mid-flight — RunNext reads the field without a lock.
func (q *Queue) SetNotifier(n Notifier) { q.notifier = n }

func (q *Queue) CreateTask(ctx context.Context, desc string) (int64, error) {
	t, err := q.repo.CreateTask(ctx, desc)
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

	branch, summary, tokens, costCents, audits, runErr := q.runner.Run(ctx, t.ID, t.Description, ghToken)
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
	for _, ae := range audits {
		payload, _ := json.Marshal(ae)
		_ = q.repo.AppendEvent(ctx, t.ID, "http_request", string(payload))
	}
	var flaggedFindings []diffscan.Finding
	var flaggedDiffs []diffscan.FileDiff
	if q.compare != nil && branch != "" {
		diffs, err := q.compare.Compare(ctx, q.repoFQN, "main", branch)
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
			compareURL := fmt.Sprintf("https://github.com/%s/compare/main...%s", q.repoFQN, branch)
			q.notifier.NotifyNeedsReview(ctx, NeedsReviewArgs{
				TaskID:     t.ID,
				Branch:     branch,
				Summary:    summary,
				Tokens:     tokens,
				CostCents:  costCents,
				Findings:   flaggedFindings,
				Diffs:      flaggedDiffs,
				CompareURL: compareURL,
			})
		} else {
			q.notifier.NotifyCompleted(ctx, t.ID, branch, summary, tokens, costCents)
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

// RejectTask transitions needs_review → rejected and deletes the branch.
// No-op on already-rejected (status stays, no re-delete). Errors on other
// states.
func (q *Queue) RejectTask(ctx context.Context, id int64) error {
	task, err := q.repo.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	switch task.Status {
	case "rejected":
		return nil // idempotent
	case "needs_review":
		if err := q.repo.SetStatus(ctx, id, "rejected"); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
		_ = q.repo.AppendEvent(ctx, id, "rejected", "{}")
		if q.branchDeleter != nil && task.BranchName.Valid && task.BranchName.String != "" {
			if err := q.branchDeleter.DeleteBranch(ctx, q.repoFQN, task.BranchName.String); err != nil {
				// Don't roll back the status change — user intent has been
				// recorded. Surface the error so the caller (callback handler)
				// can show it in the toast.
				return fmt.Errorf("branch delete: %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("cannot reject task in state %q", task.Status)
	}
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

// CancelTask — real implementation in M3-19.
func (q *Queue) CancelTask(ctx context.Context, id int64) error {
	return fmt.Errorf("CancelTask: not implemented until M3-19")
}

// RetryTask — real implementation in M3-20.
func (q *Queue) RetryTask(ctx context.Context, id int64) (int64, error) {
	return 0, fmt.Errorf("RetryTask: not implemented until M3-20")
}

// compile-time assertion that Queue satisfies telegram.Ops
var _ telegram.Ops = (*Queue)(nil)
