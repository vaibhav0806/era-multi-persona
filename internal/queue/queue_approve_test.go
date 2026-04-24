package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApproveTask_LabelsAndReviewsPR(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{}
	q.SetPRCreator(pc)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetPRNumber(ctx, task.ID, 7)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	require.NoError(t, q.ApproveTask(ctx, task.ID))

	// Both API calls happened with correct args.
	require.Len(t, pc.labeled, 1)
	require.Equal(t, "owner/repo", pc.labeled[0].Repo)
	require.Equal(t, 7, pc.labeled[0].Number)
	require.Equal(t, "era-approved", pc.labeled[0].Label)

	require.Len(t, pc.approved, 1)
	require.Equal(t, "owner/repo", pc.approved[0].Repo)
	require.Equal(t, 7, pc.approved[0].Number)
	require.Contains(t, pc.approved[0].Body, "Approved via era")

	// Task status flips.
	got, _ := repo.GetTask(ctx, task.ID)
	require.Equal(t, "approved", got.Status)
}

func TestApproveTask_NullPRNumber_SkipsGH(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{}
	q.SetPRCreator(pc)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")
	// No SetPRNumber — pr_number stays NULL.

	require.NoError(t, q.ApproveTask(ctx, task.ID))

	require.Len(t, pc.labeled, 0, "must not call GH when pr_number null")
	require.Len(t, pc.approved, 0)

	got, _ := repo.GetTask(ctx, task.ID)
	require.Equal(t, "approved", got.Status)
}

func TestApproveTask_LabelErrorLoggedButNotBlocking(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{labelErr: errors.New("network blip")}
	q.SetPRCreator(pc)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo")
	_ = repo.CompleteTask(ctx, task.ID, "agent/5/b", "s", 0, 0)
	_ = repo.SetPRNumber(ctx, task.ID, 7)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	// Status transition must succeed despite label failure.
	require.NoError(t, q.ApproveTask(ctx, task.ID))

	events, _ := repo.ListEvents(ctx, task.ID)
	foundErr := false
	for _, e := range events {
		if e.Kind == "pr_label_error" {
			foundErr = true
		}
	}
	require.True(t, foundErr, "pr_label_error event must be logged")

	got, _ := repo.GetTask(ctx, task.ID)
	require.Equal(t, "approved", got.Status)
}

func TestApproveTask_IdempotentOnAlreadyApproved(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueue(t, &fakeRunner{})
	pc := &fakePRCreator{}
	q.SetPRCreator(pc)

	task, _ := repo.CreateTask(ctx, "x", "owner/repo")
	_ = repo.SetStatus(ctx, task.ID, "approved")

	require.NoError(t, q.ApproveTask(ctx, task.ID))
	require.Len(t, pc.labeled, 0, "already-approved must not re-label")
}
