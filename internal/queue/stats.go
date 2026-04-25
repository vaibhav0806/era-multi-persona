package queue

import (
	"context"
	"time"
)

type PeriodStats struct {
	TasksTotal int
	TasksOK    int
	Tokens     int64
	CostCents  int64
}

func (p PeriodStats) SuccessRate() float64 {
	if p.TasksTotal == 0 {
		return 0
	}
	return float64(p.TasksOK) / float64(p.TasksTotal)
}

type Stats struct {
	Last24h, Last7d, Last30d PeriodStats
	PendingQueue             int
}

func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	targets := []*PeriodStats{&s.Last24h, &s.Last7d, &s.Last30d}
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
