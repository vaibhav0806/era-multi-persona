package queue_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReconcile_RunningToFailed(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	t1, _ := repo.CreateTask(ctx, "x", "", "default", "")
	_ = repo.SetStatus(ctx, t1.ID, "running")
	t2, _ := repo.CreateTask(ctx, "y", "", "default", "")
	// t2 stays queued.

	n, err := q.Reconcile(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	got, _ := repo.GetTask(ctx, t1.ID)
	require.Equal(t, "failed", got.Status)
	require.Contains(t, got.Error.String, "orchestrator restart")

	events, _ := repo.ListEvents(ctx, t1.ID)
	foundEvent := false
	for _, e := range events {
		if e.Kind == "reconciled_failed" {
			foundEvent = true
			break
		}
	}
	require.True(t, foundEvent, "reconciled_failed event must be logged")

	got2, _ := repo.GetTask(ctx, t2.ID)
	require.Equal(t, "queued", got2.Status)
}
