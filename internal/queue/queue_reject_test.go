package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueue_RejectTask_ClosesPRBeforeDeletingBranch(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{}
	bd := &fakeBranchDeleter{}
	q.SetPRCreator(pc)
	q.SetBranchDeleter(bd)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo", "default")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetPRNumber(ctx, task.ID, 5)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	require.NoError(t, q.RejectTask(ctx, task.ID))

	require.Len(t, pc.closed, 1)
	require.Equal(t, "owner/repo", pc.closed[0].Repo)
	require.Equal(t, 5, pc.closed[0].Number)
	require.Len(t, bd.deleted, 1)
	require.Equal(t, "agent/5/b", bd.deleted[0])
}

func TestQueue_RejectTask_NullPRNumber_SkipsClose(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{}
	bd := &fakeBranchDeleter{}
	q.SetPRCreator(pc)
	q.SetBranchDeleter(bd)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo", "default")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	require.NoError(t, q.RejectTask(ctx, task.ID))

	require.Len(t, pc.closed, 0, "PR close must NOT be called when pr_number is null")
	require.Len(t, bd.deleted, 1)
}

func TestQueue_RejectTask_PRCloseFails_DoesNotBlockBranchDelete(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{closeErr: errors.New("409 conflict")}
	bd := &fakeBranchDeleter{}
	q.SetPRCreator(pc)
	q.SetBranchDeleter(bd)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo", "default")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetPRNumber(ctx, task.ID, 5)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	require.NoError(t, q.RejectTask(ctx, task.ID))
	require.Len(t, bd.deleted, 1, "branch delete must still run even if PR close fails")
}

func TestRejectTask_PostsCommentBeforeClose(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{}
	bd := &fakeBranchDeleter{}
	q.SetPRCreator(pc)
	q.SetBranchDeleter(bd)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo", "default")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetPRNumber(ctx, task.ID, 5)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	require.NoError(t, q.RejectTask(ctx, task.ID))

	// Comment posted to same PR
	require.Len(t, pc.commented, 1)
	require.Equal(t, "owner/repo", pc.commented[0].Repo)
	require.Equal(t, 5, pc.commented[0].Number)
	require.Contains(t, pc.commented[0].Body, "Rejected via era")

	// Close also called (existing behavior preserved)
	require.Len(t, pc.closed, 1)
	require.Equal(t, "owner/repo", pc.closed[0].Repo)
	require.Equal(t, 5, pc.closed[0].Number)
}

func TestRejectTask_CommentErrorDoesNotBlockClose(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{commentErr: errors.New("network blip")}
	bd := &fakeBranchDeleter{}
	q.SetPRCreator(pc)
	q.SetBranchDeleter(bd)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo", "default")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetPRNumber(ctx, task.ID, 5)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	require.NoError(t, q.RejectTask(ctx, task.ID))
	require.Len(t, pc.closed, 1, "close must still run even if comment failed")

	events, _ := repo.ListEvents(ctx, task.ID)
	foundErr := false
	for _, e := range events {
		if e.Kind == "pr_comment_error" {
			foundErr = true
		}
	}
	require.True(t, foundErr)
}
