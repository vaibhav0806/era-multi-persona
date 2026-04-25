package queue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/queue"
)

func TestStats_EmptyDB_ReturnsZeros(t *testing.T) {
	ctx := context.Background()
	q, _ := newRunQueue(t, &fakeRunner{})
	s, err := q.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, s.Last24h.TasksTotal)
	require.Equal(t, 0, s.Last7d.TasksTotal)
	require.Equal(t, 0, s.Last30d.TasksTotal)
	require.Equal(t, 0, s.PendingQueue)
}

func TestStats_MixedStatuses_CountsSuccessRate(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})

	// seed creates a task and completes it; CompleteTask always sets status="completed",
	// so we call SetStatus("failed") after CompleteTask to override for failed tasks.
	seed := func(status string, tokens, cents int64) {
		task, err := repo.CreateTask(ctx, "x", "", "default")
		require.NoError(t, err)
		require.NoError(t, repo.CompleteTask(ctx, task.ID, "br", "s", tokens, cents))
		if status != "completed" {
			require.NoError(t, repo.SetStatus(ctx, task.ID, status))
		}
	}
	seed("completed", 100, 1)
	seed("completed", 200, 2)
	seed("failed", 50, 0)

	s, err := q.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, s.Last24h.TasksTotal)
	require.Equal(t, 2, s.Last24h.TasksOK)
	require.Equal(t, int64(350), s.Last24h.Tokens)
	require.Equal(t, int64(3), s.Last24h.CostCents)
}

func TestPeriodStats_SuccessRate(t *testing.T) {
	p := queue.PeriodStats{}
	require.Equal(t, 0.0, p.SuccessRate())
	p2 := queue.PeriodStats{TasksTotal: 10, TasksOK: 7}
	require.InDelta(t, 0.7, p2.SuccessRate(), 0.001)
}
