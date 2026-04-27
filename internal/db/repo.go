package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNoTasks = errors.New("no queued tasks")

type Repo struct {
	q *Queries
}

func NewRepo(h *Handle) *Repo {
	return &Repo{q: New(h.Raw())}
}

func (r *Repo) CreateTask(ctx context.Context, desc, targetRepo, profile, personaName string) (Task, error) {
	return r.q.CreateTask(ctx, CreateTaskParams{
		Description:   desc,
		TargetRepo:    targetRepo,
		BudgetProfile: profile,
		PersonaName:   personaName,
	})
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

func (r *Repo) SetStatus(ctx context.Context, id int64, status string) error {
	return r.q.SetTaskStatus(ctx, SetTaskStatusParams{Status: status, ID: id})
}

func (r *Repo) ListBetween(ctx context.Context, from, to time.Time) ([]Task, error) {
	return r.q.ListTasksBetween(ctx, ListTasksBetweenParams{
		CreatedAt:   from.UTC(),
		CreatedAt_2: to.UTC(),
	})
}

func (r *Repo) SetPRNumber(ctx context.Context, id, pr int64) error {
	return r.q.SetPRNumber(ctx, SetPRNumberParams{PrNumber: sql.NullInt64{Int64: pr, Valid: true}, ID: id})
}

func (r *Repo) ListRunningTaskIDs(ctx context.Context) ([]int64, error) {
	return r.q.ListRunningTaskIDs(ctx)
}

func (r *Repo) MarkRunningTasksFailed(ctx context.Context, reason string) (int64, error) {
	return r.q.MarkRunningTasksFailed(ctx, sql.NullString{String: reason, Valid: true})
}

func (r *Repo) SetBudgetProfile(ctx context.Context, id int64, profile string) error {
	return r.q.SetBudgetProfile(ctx, SetBudgetProfileParams{
		BudgetProfile: profile,
		ID:            id,
	})
}

func (r *Repo) SetCompletionMessageID(ctx context.Context, id, msgID int64) error {
	return r.q.SetCompletionMessageID(ctx, SetCompletionMessageIDParams{
		CompletionMessageID: sql.NullInt64{Int64: msgID, Valid: true},
		ID:                  id,
	})
}

func (r *Repo) GetTaskByCompletionMessageID(ctx context.Context, msgID int64) (Task, error) {
	return r.q.GetTaskByCompletionMessageID(ctx, sql.NullInt64{Int64: msgID, Valid: true})
}

func (r *Repo) CreateAskTask(ctx context.Context, desc, targetRepo string) (Task, error) {
	return r.q.CreateAskTask(ctx, CreateAskTaskParams{
		Description: desc,
		TargetRepo:  targetRepo,
	})
}

func (r *Repo) CountTasksByStatusSince(ctx context.Context, since time.Time) ([]CountTasksByStatusSinceRow, error) {
	return r.q.CountTasksByStatusSince(ctx, since)
}

func (r *Repo) SumTokensSince(ctx context.Context, since time.Time) (int64, error) {
	v, err := r.q.SumTokensSince(ctx, since)
	if err != nil {
		return 0, err
	}
	n, _ := v.(int64)
	return n, nil
}

func (r *Repo) SumCostCentsSince(ctx context.Context, since time.Time) (int64, error) {
	v, err := r.q.SumCostCentsSince(ctx, since)
	if err != nil {
		return 0, err
	}
	n, _ := v.(int64)
	return n, nil
}

func (r *Repo) CountQueuedTasks(ctx context.Context) (int64, error) {
	return r.q.CountQueuedTasks(ctx)
}
