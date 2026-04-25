package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeKiller struct {
	killed []string
	err    error
}

func (f *fakeKiller) Kill(ctx context.Context, name string) error {
	f.killed = append(f.killed, name)
	return f.err
}

func TestCancelTask_RunningInvokesKiller(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	k := &fakeKiller{}
	q.SetKiller(k)

	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.SetStatus(ctx, task.ID, "running")
	q.Running().Register(task.ID, "era-runner-1-abc")

	require.NoError(t, q.CancelTask(ctx, task.ID))
	require.Equal(t, []string{"era-runner-1-abc"}, k.killed)
	require.True(t, q.Running().WasKilled(task.ID))
}

func TestCancelTask_RunningNoContainerYet_Errors(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	q.SetKiller(&fakeKiller{})
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.SetStatus(ctx, task.ID, "running")
	err := q.CancelTask(ctx, task.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "retry shortly")
}

func TestCancelTask_QueuedStillWorks(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	require.NoError(t, q.CancelTask(ctx, task.ID))
	got, _ := repo.GetTask(ctx, task.ID)
	require.Equal(t, "cancelled", got.Status)
}

func TestCancelTask_CompletedFails(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.SetStatus(ctx, task.ID, "completed")
	err := q.CancelTask(ctx, task.ID)
	require.Error(t, err)
}

func TestCancelTask_KillError_Propagates(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	k := &fakeKiller{err: errors.New("docker: container not found")}
	q.SetKiller(k)
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.SetStatus(ctx, task.ID, "running")
	q.Running().Register(task.ID, "era-runner-1")
	err := q.CancelTask(ctx, task.ID)
	require.Error(t, err)
}
