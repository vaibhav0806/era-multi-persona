package queue

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vaibhav0806/pi-agent/internal/db"
	"github.com/vaibhav0806/pi-agent/internal/telegram"
)

// Runner is wired in Task 15; nil-safe for Phase C.
type Runner interface {
	Run(ctx context.Context, taskID int64, description string) (branch, summary string, err error)
}

type Queue struct {
	repo   *db.Repo
	runner Runner
}

func New(repo *db.Repo, runner Runner) *Queue { return &Queue{repo: repo, runner: runner} }

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

// compile-time assertion that Queue satisfies telegram.Ops
var _ telegram.Ops = (*Queue)(nil)
