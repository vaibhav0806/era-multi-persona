package queue_test

import (
	"context"
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
}
type closedRecord struct {
	Repo   string
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

	_, err := repo.CreateTask(ctx, "do thing", "owner/repo")
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
