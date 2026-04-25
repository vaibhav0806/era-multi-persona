package queue_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/audit"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/diffscan"
	"github.com/vaibhav0806/era/internal/queue"
)

type fakeRunner struct {
	branch    string
	summary   string
	tokens    int64
	costCents int
	audits    []audit.Entry
	err       error
	calls     int
	lastID    int64
	lastDes   string
	lastToken string
	lastRepo  string
}

func (f *fakeRunner) Run(ctx context.Context, taskID int64, desc string, ghToken string, repo string) (string, string, int64, int, []audit.Entry, error) {
	f.calls++
	f.lastID = taskID
	f.lastDes = desc
	f.lastToken = ghToken
	f.lastRepo = repo
	return f.branch, f.summary, f.tokens, f.costCents, f.audits, f.err
}

type fakeTokens struct {
	token string
	err   error
}

func (f *fakeTokens) InstallationToken(ctx context.Context) (string, error) {
	return f.token, f.err
}

func newRunQueue(t *testing.T, r queue.Runner) (*queue.Queue, *db.Repo) {
	t.Helper()
	return newRunQueueWithTokens(t, r, nil)
}

func newRunQueueWithTokens(t *testing.T, r queue.Runner, tokens queue.TokenSource) (*queue.Queue, *db.Repo) {
	t.Helper()
	return newRunQueueWithDeps(t, r, tokens, nil, "")
}

func newRunQueueWithDeps(t *testing.T, r queue.Runner, tokens queue.TokenSource, compare queue.DiffSource, repoFQN string) (*queue.Queue, *db.Repo) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	repo := db.NewRepo(h)
	return queue.New(repo, r, tokens, compare, repoFQN), repo
}

type fakeCompare struct {
	diffs []diffscan.FileDiff
	err   error
	calls int
}

func (f *fakeCompare) Compare(ctx context.Context, repo, base, head string) ([]diffscan.FileDiff, error) {
	f.calls++
	return f.diffs, f.err
}

func TestQueue_RunNext_Success(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	id, err := q.CreateTask(ctx, "do x", "", "default")
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

	id, err := q.CreateTask(ctx, "boom", "", "default")
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

	id, _ := q.CreateTask(ctx, "x", "", "default")
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

	id, _ := q.CreateTask(ctx, "x", "", "default")
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

type completedArgs struct {
	ID        int64
	Repo      string
	Branch    string
	PRURL     string
	Summary   string
	Tokens    int64
	CostCents int
}
type failedArgs struct {
	ID     int64
	Reason string
}

type needsReviewArgs struct {
	ID        int64
	Branch    string
	Summary   string
	Tokens    int64
	CostCents int
	Findings  []diffscan.Finding
	Diffs     []diffscan.FileDiff
	PRURL     string
}

type fakeNotifier struct {
	completed   []completedArgs
	failed      []failedArgs
	needsReview []needsReviewArgs
	cancelled   []int64
}

func (f *fakeNotifier) NotifyCompleted(ctx context.Context, id int64, repo, b, prURL, s string, t int64, c int) {
	f.completed = append(f.completed, completedArgs{id, repo, b, prURL, s, t, c})
}
func (f *fakeNotifier) NotifyFailed(ctx context.Context, id int64, r string) {
	f.failed = append(f.failed, failedArgs{id, r})
}
func (f *fakeNotifier) NotifyNeedsReview(ctx context.Context, a queue.NeedsReviewArgs) {
	f.needsReview = append(f.needsReview, needsReviewArgs{
		ID:        a.TaskID,
		Branch:    a.Branch,
		Summary:   a.Summary,
		Tokens:    a.Tokens,
		CostCents: a.CostCents,
		Findings:  a.Findings,
		Diffs:     a.Diffs,
		PRURL:     a.PRURL,
	})
}

func (f *fakeNotifier) NotifyCancelled(ctx context.Context, id int64) {
	f.cancelled = append(f.cancelled, id)
}

var _ queue.Notifier = (*fakeNotifier)(nil)

func TestQueue_Notifier_OnSuccess(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/ok", summary: "done"}
	q, _ := newRunQueue(t, fr)
	n := &fakeNotifier{}
	q.SetNotifier(n)

	id, _ := q.CreateTask(ctx, "work", "", "default")
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

	id, _ := q.CreateTask(ctx, "work", "", "default")
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

	_, _ = q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
}

func TestQueue_RunNext_RecordsTokensAndCost(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "b", summary: "s", tokens: 4321, costCents: 9}
	q, repo := newRunQueue(t, fr)
	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
	got, err := repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, int64(4321), got.TokensUsed)
	require.Equal(t, int64(9), got.CostCents)
}

func TestQueue_RunNext_PersistsAuditEntries(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{
		branch: "b", summary: "s", tokens: 1, costCents: 1,
		audits: []audit.Entry{
			{Method: "GET", Path: "/health", Status: 200},
			{Method: "CONNECT", Host: "github.com", Status: 200},
		},
	}
	q, repo := newRunQueue(t, fr)
	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)

	events, err := repo.ListEvents(ctx, id)
	require.NoError(t, err)
	// Expect: started, completed, + 2 http_request events = 4
	httpReqs := 0
	for _, e := range events {
		if e.Kind == "http_request" {
			httpReqs++
		}
	}
	require.Equal(t, 2, httpReqs)
}

func TestQueue_RunNext_PassesGhTokenFromSource(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "b", summary: "s"}
	tokens := &fakeTokens{token: "ghs_test_token_123"}
	q, _ := newRunQueueWithTokens(t, fr, tokens)
	_, _ = q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
	require.Equal(t, "ghs_test_token_123", fr.lastToken, "runner should receive the minted token")
}

func TestQueue_RunNext_TokenMintFailure(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{}
	tokens := &fakeTokens{err: errors.New("github down")}
	q, repo := newRunQueueWithTokens(t, fr, tokens)
	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.Error(t, err)
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "failed", task.Status)
	require.Contains(t, task.Error.String, "token mint")
}

func TestQueue_RunNext_CleanDiff_StaysCompleted(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
	fc := &fakeCompare{diffs: []diffscan.FileDiff{
		{Path: "foo.go", Added: []string{"foo"}},
	}}
	q, repo := newRunQueueWithDeps(t, fr, nil, fc, "a/b")
	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "completed", task.Status)
	require.Equal(t, 1, fc.calls, "compare should have been called exactly once")
}

func TestQueue_RunNext_FlaggedDiff_SetsNeedsReview(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
	fc := &fakeCompare{diffs: []diffscan.FileDiff{
		{Path: "foo_test.go", Removed: []string{"func TestBar(t *testing.T) {}"}},
	}}
	q, repo := newRunQueueWithDeps(t, fr, nil, fc, "a/b")
	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "needs_review", task.Status)

	events, _ := repo.ListEvents(ctx, id)
	sawFlag := false
	for _, e := range events {
		if e.Kind == "diffscan_flagged" {
			sawFlag = true
			require.Contains(t, e.Payload, "removed_test")
		}
	}
	require.True(t, sawFlag)
}

func TestQueue_RunNext_CompareError_LogsEventButDoesntBlock(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
	fc := &fakeCompare{err: errors.New("github 404")}
	q, repo := newRunQueueWithDeps(t, fr, nil, fc, "a/b")
	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "completed", task.Status)

	events, _ := repo.ListEvents(ctx, id)
	sawErr := false
	for _, e := range events {
		if e.Kind == "diffscan_error" {
			sawErr = true
		}
	}
	require.True(t, sawErr)
}

func TestQueue_RunNext_NoCompareClient_NoDiffscan(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
	q, repo := newRunQueueWithDeps(t, fr, nil, nil, "") // compare == nil
	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "completed", task.Status)
}

func TestQueue_RunNext_FlaggedDiff_CallsNotifyNeedsReview(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "s", tokens: 100, costCents: 1}
	fc := &fakeCompare{diffs: []diffscan.FileDiff{
		{Path: "foo_test.go", Removed: []string{"func TestBar(t *testing.T) {}"}},
	}}
	q, _ := newRunQueueWithDeps(t, fr, nil, fc, "a/b")
	n := &fakeNotifier{}
	q.SetNotifier(n)

	id, _ := q.CreateTask(ctx, "x", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)

	// Completion DM should NOT have fired (task was flagged)
	require.Empty(t, n.completed, "clean completion DM should not fire for flagged task")
	// Approval DM SHOULD have fired with findings attached
	require.Len(t, n.needsReview, 1)
	require.Equal(t, id, n.needsReview[0].ID)
	require.Equal(t, "agent/1/x", n.needsReview[0].Branch)
	require.NotEmpty(t, n.needsReview[0].Findings)
	require.Equal(t, "removed_test", n.needsReview[0].Findings[0].Rule)
}

type fakeBranchDeleter struct {
	deleted []string
	err     error
}

func (f *fakeBranchDeleter) DeleteBranch(ctx context.Context, repo, branch string) error {
	f.deleted = append(f.deleted, branch)
	return f.err
}

func TestQueue_ApproveTask_NeedsReviewToApproved(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	bd := &fakeBranchDeleter{}
	q.SetBranchDeleter(bd)
	id, _ := q.CreateTask(ctx, "x", "", "default")
	require.NoError(t, repo.SetStatus(ctx, id, "needs_review"))

	require.NoError(t, q.ApproveTask(ctx, id))
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "approved", task.Status)
	require.Empty(t, bd.deleted, "approve should NOT delete branch")

	// Idempotent second call
	require.NoError(t, q.ApproveTask(ctx, id))
	task, _ = repo.GetTask(ctx, id)
	require.Equal(t, "approved", task.Status)
}

func TestQueue_RejectTask_NeedsReviewToRejected_DeletesBranch(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	bd := &fakeBranchDeleter{}
	q.SetBranchDeleter(bd)
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.SetStatus(ctx, task.ID, "needs_review")
	require.NoError(t, repo.CompleteTask(ctx, task.ID, "agent/1/foo", "s", 0, 0))
	_ = repo.SetStatus(ctx, task.ID, "needs_review") // re-set since CompleteTask sets completed

	require.NoError(t, q.RejectTask(ctx, task.ID))
	got, _ := repo.GetTask(ctx, task.ID)
	require.Equal(t, "rejected", got.Status)
	require.Equal(t, []string{"agent/1/foo"}, bd.deleted, "reject must delete branch")

	// Idempotent
	require.NoError(t, q.RejectTask(ctx, task.ID))
	require.Equal(t, []string{"agent/1/foo"}, bd.deleted, "double-reject does NOT re-delete")
}

func TestQueue_ApproveTask_AlreadyRejected_Errors(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	q.SetBranchDeleter(&fakeBranchDeleter{})
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.SetStatus(ctx, task.ID, "rejected")

	err := q.ApproveTask(ctx, task.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rejected")
}

func TestQueue_RejectTask_AlreadyApproved_Errors(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	q.SetBranchDeleter(&fakeBranchDeleter{})
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.SetStatus(ctx, task.ID, "approved")

	err := q.RejectTask(ctx, task.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "approved")
}

func TestQueue_RejectTask_BranchDeleterError_LoggedNotPropagated(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	bd := &fakeBranchDeleter{err: errors.New("github 422")}
	q.SetBranchDeleter(bd)
	task, _ := repo.CreateTask(ctx, "x", "", "default")
	_ = repo.CompleteTask(ctx, task.ID, "agent/1/bar", "s", 0, 0)
	_ = repo.SetStatus(ctx, task.ID, "needs_review")

	// Branch delete errors are logged as events but do not block the transition.
	err := q.RejectTask(ctx, task.ID)
	require.NoError(t, err)
	got, _ := repo.GetTask(ctx, task.ID)
	require.Equal(t, "rejected", got.Status, "status changes even if delete fails")

	events, _ := repo.ListEvents(ctx, task.ID)
	sawErr := false
	for _, e := range events {
		if e.Kind == "branch_delete_error" {
			sawErr = true
		}
	}
	require.True(t, sawErr, "branch_delete_error event must be logged")
}

func TestQueue_ApproveTask_WrongStatus_Errors(t *testing.T) {
	ctx := context.Background()
	q, _ := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	q.SetBranchDeleter(&fakeBranchDeleter{})
	// queued status — can't approve
	id, _ := q.CreateTask(ctx, "x", "", "default")
	err := q.ApproveTask(ctx, id)
	require.Error(t, err)
}

func TestQueue_CancelTask_QueuedToCancelled(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	id, _ := q.CreateTask(ctx, "x", "", "default")

	require.NoError(t, q.CancelTask(ctx, id))
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "cancelled", task.Status)

	// Idempotent
	require.NoError(t, q.CancelTask(ctx, id))
	task, _ = repo.GetTask(ctx, id)
	require.Equal(t, "cancelled", task.Status)
}

func TestQueue_CancelTask_RunningErrors(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	id, _ := q.CreateTask(ctx, "x", "", "default")
	require.NoError(t, repo.SetStatus(ctx, id, "running"))

	err := q.CancelTask(ctx, id)
	require.Error(t, err)
	require.Contains(t, err.Error(), "running")
	// Status unchanged
	task, _ := repo.GetTask(ctx, id)
	require.Equal(t, "running", task.Status)
}

func TestQueue_CancelTask_CompletedErrors(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	id, _ := q.CreateTask(ctx, "x", "", "default")
	require.NoError(t, repo.CompleteTask(ctx, id, "agent/1/x", "s", 0, 0))

	err := q.CancelTask(ctx, id)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot cancel")
}

func TestQueue_RetryTask_ClonesDescription(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	oldID, _ := q.CreateTask(ctx, "refactor the auth middleware", "", "default")
	require.NoError(t, repo.SetStatus(ctx, oldID, "failed"))

	newID, err := q.RetryTask(ctx, oldID)
	require.NoError(t, err)
	require.NotEqual(t, oldID, newID)

	newTask, _ := repo.GetTask(ctx, newID)
	require.Equal(t, "refactor the auth middleware", newTask.Description)
	require.Equal(t, "queued", newTask.Status)

	// Old task unchanged
	oldTask, _ := repo.GetTask(ctx, oldID)
	require.Equal(t, "failed", oldTask.Status)
}

func TestQueue_RetryTask_NotFoundErrors(t *testing.T) {
	ctx := context.Background()
	q, _ := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	_, err := q.RetryTask(ctx, 999)
	require.Error(t, err)
}

func TestQueue_RetryTask_WorksForAnyStatus(t *testing.T) {
	ctx := context.Background()
	q, repo := newRunQueueWithDeps(t, &fakeRunner{}, nil, nil, "a/b")
	// Even approved/rejected/completed tasks can be retried — it just clones
	// the description. This is the "/retry ran a task that succeeded already
	// because I want the same thing done again" case.
	id, _ := q.CreateTask(ctx, "same thing twice", "", "default")
	require.NoError(t, repo.SetStatus(ctx, id, "approved"))
	newID, err := q.RetryTask(ctx, id)
	require.NoError(t, err)
	newTask, _ := repo.GetTask(ctx, newID)
	require.Equal(t, "same thing twice", newTask.Description)
	require.Equal(t, "queued", newTask.Status)
}

func TestQueue_RunNext_PassesEffectiveRepo_FromTask(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
	q, repo := newRunQueueWithDeps(t, fr, nil, nil, "default/repo")
	task, err := repo.CreateTask(ctx, "x", "alice/bob", "default")
	require.NoError(t, err)
	require.Equal(t, "alice/bob", task.TargetRepo)
	_, err = q.RunNext(ctx)
	require.NoError(t, err)
	require.Equal(t, "alice/bob", fr.lastRepo, "runner should receive task.TargetRepo, not default")
}

func TestQueue_RunNext_KilledTask_WritesCancelled(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{err: errors.New("exit status 137")}
	q, repo := newRunQueue(t, fr)
	n := &fakeNotifier{}
	q.SetNotifier(n)

	task, _ := repo.CreateTask(ctx, "x", "", "default")
	q.Running().MarkKilled(task.ID) // simulate /cancel already fired

	_, err := q.RunNext(ctx)
	require.NoError(t, err)

	got, _ := repo.GetTask(ctx, task.ID)
	require.Equal(t, "cancelled", got.Status)
	require.Equal(t, []int64{task.ID}, n.cancelled)
	require.Len(t, n.failed, 0, "NotifyFailed must NOT be called for killed tasks")
}

func TestQueue_RunNext_FallsBackToDefaultRepo(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
	q, _ := newRunQueueWithDeps(t, fr, nil, nil, "default/repo")
	_, _ = q.CreateTask(ctx, "no-repo task", "", "default")
	_, err := q.RunNext(ctx)
	require.NoError(t, err)
	require.Equal(t, "default/repo", fr.lastRepo, "empty TargetRepo falls back to default")
}
