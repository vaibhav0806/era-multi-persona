package queue

import (
	"context"
	"time"

	"github.com/vaibhav0806/era/internal/stats"
)

func (q *Queue) Stats(ctx context.Context) (stats.Stats, error) {
	var s stats.Stats
	targets := []*stats.PeriodStats{&s.Last24h, &s.Last7d, &s.Last30d}
	durs := []time.Duration{24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour}
	now := time.Now().UTC()

	for i, d := range durs {
		since := now.Add(-d)
		rows, err := q.repo.CountTasksByStatusSince(ctx, since)
		if err != nil {
			return s, err
		}
		for _, r := range rows {
			targets[i].TasksTotal += int(r.Count)
			if r.Status == "completed" || r.Status == "approved" {
				targets[i].TasksOK += int(r.Count)
			}
		}
		toks, err := q.repo.SumTokensSince(ctx, since)
		if err != nil {
			return s, err
		}
		targets[i].Tokens = toks
		cost, err := q.repo.SumCostCentsSince(ctx, since)
		if err != nil {
			return s, err
		}
		targets[i].CostCents = cost
	}
	pending, err := q.repo.CountQueuedTasks(ctx)
	if err != nil {
		return s, err
	}
	s.PendingQueue = int(pending)
	return s, nil
}
