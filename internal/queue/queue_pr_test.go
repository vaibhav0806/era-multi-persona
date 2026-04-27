package queue_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/githubpr"
)

type fakePRCreator struct {
	mu              sync.Mutex
	created         []githubpr.CreateArgs
	createReturns   *githubpr.PR
	createErr       error
	closed          []closedRecord
	closeErr        error
	defaultBranch   string
	defaultBranchEr error
	approved        []approvedRecord
	approveErr      error
	labeled         []labeledRecord
	labelErr        error
	commented       []commentedRecord
	commentErr      error
}
type closedRecord struct {
	Repo   string
	Number int
}
type approvedRecord struct {
	Repo   string
	Body   string
	Number int
}
type labeledRecord struct {
	Repo   string
	Label  string
	Number int
}
type commentedRecord struct {
	Repo   string
	Body   string
	Number int
}

func (f *fakePRCreator) Create(ctx context.Context, a githubpr.CreateArgs) (*githubpr.PR, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, a)
	return f.createReturns, f.createErr
}
func (f *fakePRCreator) Close(ctx context.Context, repo string, n int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, closedRecord{repo, n})
	return f.closeErr
}
func (f *fakePRCreator) DefaultBranch(ctx context.Context, repo string) (string, error) {
	if f.defaultBranch == "" {
		return "main", f.defaultBranchEr
	}
	return f.defaultBranch, f.defaultBranchEr
}
func (f *fakePRCreator) ApprovePR(ctx context.Context, repo string, n int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approved = append(f.approved, approvedRecord{repo, body, n})
	return f.approveErr
}
func (f *fakePRCreator) AddLabel(ctx context.Context, repo string, n int, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.labeled = append(f.labeled, labeledRecord{repo, label, n})
	return f.labelErr
}
func (f *fakePRCreator) AddComment(ctx context.Context, repo string, n int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commented = append(f.commented, commentedRecord{repo, body, n})
	return f.commentErr
}

func TestQueue_RunNext_OpensPROnSuccess(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/1/ok", summary: "pi answer"}
	q, repo := newRunQueue(t, fr)
	pc := &fakePRCreator{
		createReturns: &githubpr.PR{Number: 7, HTMLURL: "https://github.com/owner/repo/pull/7"},
		defaultBranch: "main",
	}
	q.SetPRCreator(pc)
	n := &fakeNotifier{}
	q.SetNotifier(n)

	_, err := repo.CreateTask(ctx, "do thing", "owner/repo", "default", "")
	require.NoError(t, err)
	_, err = q.RunNext(ctx)
	require.NoError(t, err)

	require.Len(t, pc.created, 1)
	require.Equal(t, "owner/repo", pc.created[0].Repo)
	require.Equal(t, "agent/1/ok", pc.created[0].Head)
	require.Equal(t, "main", pc.created[0].Base)
	require.Contains(t, pc.created[0].Title, "do thing")
	require.Contains(t, n.completed[0].PRURL, "/pull/7")
}

func TestQueue_RunNext_PRCreateFails_FallsBackToBranchURL(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/2/x", summary: "answer"}
	q, repo := newRunQueue(t, fr)
	pc := &fakePRCreator{
		createErr:     errors.New("422 unprocessable"),
		defaultBranch: "main",
	}
	q.SetPRCreator(pc)
	n := &fakeNotifier{}
	q.SetNotifier(n)

	task, err := repo.CreateTask(ctx, "failing pr task", "owner/repo", "default", "")
	require.NoError(t, err)
	_, err = q.RunNext(ctx)
	require.NoError(t, err)

	require.Len(t, n.completed, 1)
	require.Contains(t, n.completed[0].PRURL, "/tree/agent/2/x")
	events, _ := repo.ListEvents(ctx, task.ID)
	found := false
	for _, e := range events {
		if e.Kind == "pr_create_error" {
			found = true
			break
		}
	}
	require.True(t, found, "pr_create_error event must be logged")
}

func TestQueue_RunNext_DefaultBranchFails_FallsBackToMain(t *testing.T) {
	ctx := context.Background()
	fr := &fakeRunner{branch: "agent/3/y", summary: "z"}
	q, repo := newRunQueue(t, fr)
	pc := &fakePRCreator{
		createReturns:   &githubpr.PR{Number: 1, HTMLURL: "https://github.com/owner/repo/pull/1"},
		defaultBranchEr: errors.New("no network"),
	}
	q.SetPRCreator(pc)
	n := &fakeNotifier{}
	q.SetNotifier(n)

	_, err := repo.CreateTask(ctx, "x", "owner/repo", "default", "")
	require.NoError(t, err)
	_, err = q.RunNext(ctx)
	require.NoError(t, err)

	require.Len(t, pc.created, 1)
	require.Equal(t, "main", pc.created[0].Base)
}
