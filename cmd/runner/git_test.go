package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeBareAndClone(t *testing.T) (bare, clone string) {
	t.Helper()
	bare = filepath.Join(t.TempDir(), "remote.git")
	require.NoError(t, exec.Command("git", "init", "--bare", "--initial-branch=main", bare).Run())

	clone = filepath.Join(t.TempDir(), "work")
	require.NoError(t, exec.Command("git", "clone", bare, clone).Run())

	require.NoError(t, os.WriteFile(filepath.Join(clone, "README.md"), []byte("hi\n"), 0o644))
	runCmd(t, clone, "git", "config", "user.email", "t@t")
	runCmd(t, clone, "git", "config", "user.name", "t")
	runCmd(t, clone, "git", "add", "README.md")
	runCmd(t, clone, "git", "commit", "-m", "seed")
	runCmd(t, clone, "git", "push", "origin", "main")
	require.NoError(t, os.RemoveAll(clone))
	return bare, ""
}

func runCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	require.NoError(t, c.Run())
}

func TestGit_CloneBranchCommitPush(t *testing.T) {
	bare, _ := makeBareAndClone(t)

	workDir := filepath.Join(t.TempDir(), "workspace")
	g := &gitDriver{
		RemoteURL:   "file://" + bare,
		BranchName:  "agent/1/foo",
		CommitMsg:   "task #1: x",
		AuthorName:  "era",
		AuthorEmail: "era@local",
	}

	ctx := context.Background()
	require.NoError(t, g.Clone(ctx, workDir))

	// Simulate Pi editing a file.
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("hi\nnew line\n"), 0o644))

	require.NoError(t, g.CommitAndPush(ctx, workDir))

	// Verify the remote has the branch.
	out, err := exec.Command("git", "-C", bare, "branch", "--list", "agent/1/foo").Output()
	require.NoError(t, err)
	require.Contains(t, string(out), "agent/1/foo")
}

func TestGit_CommitAndPush_NoChanges(t *testing.T) {
	bare, _ := makeBareAndClone(t)
	workDir := filepath.Join(t.TempDir(), "workspace")
	g := &gitDriver{
		RemoteURL:   "file://" + bare,
		BranchName:  "agent/1/empty",
		CommitMsg:   "no-op",
		AuthorName:  "era",
		AuthorEmail: "era@local",
	}
	require.NoError(t, g.Clone(context.Background(), workDir))

	err := g.CommitAndPush(context.Background(), workDir)
	require.ErrorIs(t, err, errNoChanges)
}
