package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
)

func openTest(t *testing.T) *db.Repo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return db.NewRepo(h)
}

func TestRepo_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	created, err := r.CreateTask(ctx, "do the thing")
	require.NoError(t, err)
	require.Equal(t, "queued", created.Status)

	got, err := r.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "do the thing", got.Description)
}

func TestRepo_ClaimNextQueued(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	_, _ = r.CreateTask(ctx, "first")
	_, _ = r.CreateTask(ctx, "second")

	claimed, err := r.ClaimNext(ctx)
	require.NoError(t, err)
	require.Equal(t, "first", claimed.Description)
	require.Equal(t, "running", claimed.Status)

	// Second claim returns the next task
	claimed2, err := r.ClaimNext(ctx)
	require.NoError(t, err)
	require.Equal(t, "second", claimed2.Description)

	// Third claim: no tasks
	_, err = r.ClaimNext(ctx)
	require.ErrorIs(t, err, db.ErrNoTasks)
}

func TestRepo_CompleteAndFail(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	t1, _ := r.CreateTask(ctx, "a")
	require.NoError(t, r.CompleteTask(ctx, t1.ID, "agent/1/slug", "did stuff", 0, 0))
	got, _ := r.GetTask(ctx, t1.ID)
	require.Equal(t, "completed", got.Status)
	require.Equal(t, "agent/1/slug", got.BranchName.String)

	t2, _ := r.CreateTask(ctx, "b")
	require.NoError(t, r.FailTask(ctx, t2.ID, "boom"))
	got2, _ := r.GetTask(ctx, t2.ID)
	require.Equal(t, "failed", got2.Status)
	require.Equal(t, "boom", got2.Error.String)
}

func TestRepo_Events(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	task, _ := r.CreateTask(ctx, "x")
	require.NoError(t, r.AppendEvent(ctx, task.ID, "started", `{"pid":42}`))
	require.NoError(t, r.AppendEvent(ctx, task.ID, "progress", `{"pct":50}`))

	evts, err := r.ListEvents(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, evts, 2)
	require.Equal(t, "started", evts[0].Kind)
	require.Equal(t, "progress", evts[1].Kind)
}

func TestRepo_CompleteTask_RecordsTokensAndCost(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	task, err := r.CreateTask(ctx, "x")
	require.NoError(t, err)
	require.NoError(t, r.CompleteTask(ctx, task.ID, "agent/1/b", "done", 12345, 17))

	got, err := r.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", got.Status)
	require.Equal(t, int64(12345), got.TokensUsed)
	require.Equal(t, int64(17), got.CostCents)
}

func TestRepo_ListRecent(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	for i := 0; i < 5; i++ {
		_, _ = r.CreateTask(ctx, "t")
	}
	list, err := r.ListRecent(ctx, 3)
	require.NoError(t, err)
	require.Len(t, list, 3)
}
