//go:build e2e
// +build e2e

// Package e2e holds tests that exercise the full orchestrator pipeline
// against a real GitHub sandbox repository. These tests are guarded by the
// `e2e` build tag so they do not run in the default `go test` invocation.
//
// Run: go test -tags e2e ./internal/e2e/...
// Requires env vars (or a loaded .env in the parent process):
//
//	PI_GITHUB_PAT           valid PAT with write access to the sandbox repo
//	PI_GITHUB_SANDBOX_REPO  owner/repo
//
// The test builds a Queue, enqueues a task, runs it through the Docker
// runner, and confirms the task transitions to "completed" with a branch
// name on GitHub. The test cleans up the branch it creates on success.
package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/queue"
	"github.com/vaibhav0806/era/internal/runner"
)

const runnerImage = "era-runner:m0"

func TestE2E_QueueToDockerToBranch(t *testing.T) {
	pat := os.Getenv("PI_GITHUB_PAT")
	repo := os.Getenv("PI_GITHUB_SANDBOX_REPO")
	if pat == "" || repo == "" {
		t.Skip("PI_GITHUB_PAT and PI_GITHUB_SANDBOX_REPO must be set")
	}
	requireDocker(t)
	requireImage(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "e2e.db")
	h, err := db.Open(ctx, dbPath)
	require.NoError(t, err)
	defer h.Close()
	r := db.NewRepo(h)

	d := &runner.Docker{
		Image:       runnerImage,
		SandboxRepo: repo,
		GitHubPAT:   pat,
	}
	q := queue.New(r, runner.QueueAdapter{D: d})

	id, err := q.CreateTask(ctx, "e2e smoke")
	require.NoError(t, err)

	ran, err := q.RunNext(ctx)
	require.NoError(t, err)
	require.True(t, ran)

	task, err := r.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "completed", task.Status, "task should be completed, got: status=%s error=%s", task.Status, task.Error.String)
	require.NotEmpty(t, task.BranchName.String)
	require.Contains(t, task.BranchName.String, fmt.Sprintf("agent/%d/dummy-", id))

	// Cleanup: delete the branch we created so the sandbox stays tidy.
	t.Cleanup(func() {
		branch := task.BranchName.String
		if branch == "" {
			return
		}
		url := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", pat, repo)
		cmd := exec.Command("git", "push", url, "--delete", branch)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("branch cleanup failed (manual cleanup needed): %v\n%s", err, out)
		}
	})

	// Sanity: confirm the branch actually appeared on GitHub by hitting the
	// API. A successful push + completed DB row should always be accompanied
	// by the ref existing on the remote.
	confirmBranchOnGitHub(t, pat, repo, task.BranchName.String)
}

func requireDocker(t *testing.T) {
	t.Helper()
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	if err := cmd.Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

func requireImage(t *testing.T) {
	t.Helper()
	cmd := exec.Command("docker", "image", "inspect", runnerImage)
	if err := cmd.Run(); err != nil {
		t.Skipf("docker image %s not built (run: docker build -t %s docker/runner/)", runnerImage, runnerImage)
	}
}

func confirmBranchOnGitHub(t *testing.T, pat, repo, branch string) {
	t.Helper()
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/refs/heads/%s", repo, branch)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "expected branch %s to exist on %s", branch, repo)
}
