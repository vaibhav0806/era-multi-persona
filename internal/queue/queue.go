package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vaibhav0806/pi-agent/internal/db"
	"github.com/vaibhav0806/pi-agent/internal/telegram"
)

// Runner is wired in Task 15; nil-safe for Phase C.
type Runner interface {
	Run(ctx context.Context, taskID int64, description string) (branch, summary string, err error)
}

// Notifier is called by RunNext when a task finishes. Both methods are
// fire-and-forget — the notifier is expected to log its own errors and
// return promptly.
type Notifier interface {
	NotifyCompleted(ctx context.Context, taskID int64, branch, summary string)
	NotifyFailed(ctx context.Context, taskID int64, reason string)
}

type Queue struct {
	repo     *db.Repo
	runner   Runner
	notifier Notifier
}

func New(repo *db.Repo, runner Runner) *Queue { return &Queue{repo: repo, runner: runner} }

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

	branch, summary, runErr := q.runner.Run(ctx, t.ID, t.Description)
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
	if err := q.repo.CompleteTask(ctx, t.ID, branch, summary); err != nil {
		return true, fmt.Errorf("complete task: %w", err)
	}
	if q.notifier != nil {
		q.notifier.NotifyCompleted(ctx, t.ID, branch, summary)
	}
	return true, nil
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(map[string]string{"error": s})
	return string(b)
}

// compile-time assertion that Queue satisfies telegram.Ops
var _ telegram.Ops = (*Queue)(nil)
