package queue_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/pi-agent/internal/db"
	"github.com/vaibhav0806/pi-agent/internal/queue"
	"github.com/vaibhav0806/pi-agent/internal/telegram"
)

func newQueue(t *testing.T) (*queue.Queue, *db.Repo) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	r := db.NewRepo(h)
	q := queue.New(r, nil) // runner nil for now; wired Task 15
	return q, r
}

func TestQueue_CreateTask_ReturnsID(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	id, err := q.CreateTask(ctx, "hello")
	require.NoError(t, err)
	require.Greater(t, id, int64(0))
}

func TestQueue_TaskStatus_NotFound(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	_, err := q.TaskStatus(ctx, 999)
	require.ErrorIs(t, err, telegram.ErrTaskNotFound)
}

func TestQueue_TaskStatus_Found(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	id, _ := q.CreateTask(ctx, "x")
	s, err := q.TaskStatus(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "queued", s)
}

func TestQueue_ListRecent(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	_, _ = q.CreateTask(ctx, "a")
	_, _ = q.CreateTask(ctx, "b")
	list, err := q.ListRecent(ctx, 5)
	require.NoError(t, err)
	require.Len(t, list, 2)
}
