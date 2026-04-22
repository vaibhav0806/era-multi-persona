package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNoTasks = errors.New("no queued tasks")

type Repo struct {
	q *Queries
}

func NewRepo(h *Handle) *Repo {
	return &Repo{q: New(h.Raw())}
}

func (r *Repo) CreateTask(ctx context.Context, desc string) (Task, error) {
	return r.q.CreateTask(ctx, desc)
}

func (r *Repo) GetTask(ctx context.Context, id int64) (Task, error) {
	return r.q.GetTask(ctx, id)
}

func (r *Repo) ClaimNext(ctx context.Context) (Task, error) {
	t, err := r.q.ClaimNextQueuedTask(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNoTasks
	}
	if err != nil {
		return Task{}, fmt.Errorf("claim next: %w", err)
	}
	return t, nil
}

func (r *Repo) CompleteTask(ctx context.Context, id int64, branch, summary string, tokensUsed, costCents int64) error {
	return r.q.MarkTaskCompleted(ctx, MarkTaskCompletedParams{
		BranchName: sql.NullString{String: branch, Valid: true},
		Summary:    sql.NullString{String: summary, Valid: true},
		TokensUsed: tokensUsed,
		CostCents:  costCents,
		ID:         id,
	})
}

func (r *Repo) FailTask(ctx context.Context, id int64, reason string) error {
	return r.q.MarkTaskFailed(ctx, MarkTaskFailedParams{
		Error: sql.NullString{String: reason, Valid: true},
		ID:    id,
	})
}

func (r *Repo) AppendEvent(ctx context.Context, taskID int64, kind, payload string) error {
	return r.q.AppendEvent(ctx, AppendEventParams{TaskID: taskID, Kind: kind, Payload: payload})
}

func (r *Repo) ListEvents(ctx context.Context, taskID int64) ([]Event, error) {
	return r.q.ListEventsForTask(ctx, taskID)
}

func (r *Repo) ListRecent(ctx context.Context, limit int) ([]Task, error) {
	return r.q.ListRecentTasks(ctx, int64(limit))
}
