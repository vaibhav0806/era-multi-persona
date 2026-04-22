package queue_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/queue"
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

type fakeNotifier struct {
	completed []completedArgs
	failed    []failedArgs
}
type completedArgs struct {
	ID      int64
	Branch  string
	Summary string
}
type failedArgs struct {
	ID     int64
	Reason string
}

func (f *fakeNotifier) NotifyCompleted(ctx context.Context, id int64, branch, summary string) {
	f.completed = append(f.completed, completedArgs{id, branch, summary})
}
func (f *fakeNotifier) NotifyFailed(ctx context.Context, id int64, reason string) {
	f.failed = append(f.failed, failedArgs{id, reason})
}

func TestQueue_Notifier_OnSuccess(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/ok", summary: "done"}
	q, _ := newRunQueue(t, fr)
	n := &fakeNotifier{}
	q.SetNotifier(n)

	id, _ := q.CreateTask(ctx, "work")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)

	require.Len(t, n.completed, 1)
	require.Equal(t, id, n.completed[0].ID)
	require.Equal(t, "agent/1/ok", n.completed[0].Branch)
	require.Equal(t, "done", n.completed[0].Summary)
	require.Len(t, n.failed, 0)
}

func TestQueue_Notifier_OnFailure(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{err: errors.New("boom")}
	q, _ := newRunQueue(t, fr)
	n := &fakeNotifier{}
	q.SetNotifier(n)

	id, _ := q.CreateTask(ctx, "work")
	_, _ = q.RunNext(ctx)

	require.Len(t, n.failed, 1)
	require.Equal(t, id, n.failed[0].ID)
	require.Contains(t, n.failed[0].Reason, "boom")
	require.Len(t, n.completed, 0)
}

func TestQueue_Notifier_NilSafe(t *testing.T) {
	// If no notifier is attached, RunNext must not panic.
	ctx := context.Background()
	fr := &fakeRunner{branch: "b", summary: "s"}
	q, _ := newRunQueue(t, fr)
	// intentionally no SetNotifier

	_, _ = q.CreateTask(ctx, "x")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
}
