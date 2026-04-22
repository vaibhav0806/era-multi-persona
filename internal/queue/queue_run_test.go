package queue_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/pi-agent/internal/db"
	"github.com/vaibhav0806/pi-agent/internal/queue"
)

type fakeRunner struct {
	branch  string
	summary string
	err     error
	calls   int
	lastID  int64
	lastDes string
}

func (f *fakeRunner) Run(ctx context.Context, taskID int64, desc string) (string, string, error) {
	f.calls++
	f.lastID = taskID
	f.lastDes = desc
	return f.branch, f.summary, f.err
}

func newRunQueue(t *testing.T, r queue.Runner) (*queue.Queue, *db.Repo) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	repo := db.NewRepo(h)
	return queue.New(repo, r), repo
}

func TestQueue_RunNext_Success(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	id, err := q.CreateTask(ctx, "do x")
	require.NoError(t, err)

	ran, err := q.RunNext(ctx)
	require.NoError(t, err)
	require.True(t, ran)
	require.Equal(t, 1, fr.calls)
	require.Equal(t, id, fr.lastID)
	require.Equal(t, "do x", fr.lastDes)

	got, err := repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "completed", got.Status)
	require.Equal(t, "agent/1/x", got.BranchName.String)
	require.Equal(t, "ok", got.Summary.String)

	// No more tasks.
	ran, err = q.RunNext(ctx)
	require.NoError(t, err)
	require.False(t, ran)
}

func TestQueue_RunNext_Failure(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{err: errors.New("container exploded")}
	q, repo := newRunQueue(t, fr)

	id, err := q.CreateTask(ctx, "boom")
	require.NoError(t, err)

	ran, err := q.RunNext(ctx)
	require.True(t, ran)
	require.Error(t, err)
	require.Contains(t, err.Error(), "container exploded")

	got, err := repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "failed", got.Status)
	require.Contains(t, got.Error.String, "exploded")
}

func TestQueue_RunNext_EmitsEvents(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/y", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	id, _ := q.CreateTask(ctx, "x")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)

	events, err := repo.ListEvents(ctx, id)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "started", events[0].Kind)
	require.Equal(t, "completed", events[1].Kind)
}

func TestQueue_RunNext_FailureEmitsEvent(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{err: errors.New("nope")}
	q, repo := newRunQueue(t, fr)

	id, _ := q.CreateTask(ctx, "x")
	_, _ = q.RunNext(ctx)

	events, err := repo.ListEvents(ctx, id)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "started", events[0].Kind)
	require.Equal(t, "failed", events[1].Kind)
}

func TestQueue_RunNext_NoTasks(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{}
	q, _ := newRunQueue(t, fr)

	ran, err := q.RunNext(ctx)
	require.NoError(t, err)
	require.False(t, ran)
	require.Equal(t, 0, fr.calls)
}
