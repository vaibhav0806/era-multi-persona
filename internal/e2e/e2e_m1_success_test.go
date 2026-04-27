//go:build e2e
// +build e2e

package e2e_test

import (
	"context"
	"fmt"
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

const m1RunnerImage = "era-runner:m2"

func TestE2E_M1_TinyCodingTask(t *testing.T) {
	requireEnv(t,
		"PI_GITHUB_APP_ID", "PI_GITHUB_APP_INSTALLATION_ID", "PI_GITHUB_APP_PRIVATE_KEY",
		"PI_GITHUB_SANDBOX_REPO", "PI_OPENROUTER_API_KEY",
	)
	requireDocker(t)
	requireImageM1(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "m1.db")
	h, err := db.Open(ctx, dbPath)
	require.NoError(t, err)
	defer h.Close()
	r := db.NewRepo(h)

	d := &runner.Docker{
		Image:            m1RunnerImage,
		SandboxRepo:      os.Getenv("PI_GITHUB_SANDBOX_REPO"),
		OpenRouterAPIKey: os.Getenv("PI_OPENROUTER_API_KEY"),
		PiModel:          "moonshotai/kimi-k2.6",
		MaxTokens:        500_000,
		MaxCostCents:     20, // cap at $0.20 for the test
		MaxIterations:    10,
		MaxWallSeconds:   180,
	}
	tokens := githubAppTokenSource(t)
	q := queue.New(r, runner.QueueAdapter{D: d}, tokens, nil, "")

	id, err := q.CreateTask(ctx, "add a file HELLO_ERA.md with the single line 'hello from era M1'", "", "default", "")
	require.NoError(t, err)

	ran, err := q.RunNext(ctx)
	require.NoError(t, err, "RunNext returned error")
	require.True(t, ran)

	task, err := r.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "completed", task.Status, "status=%s err=%s", task.Status, task.Error.String)
	require.NotEmpty(t, task.BranchName.String)
	require.Greater(t, task.TokensUsed, int64(0))
	require.LessOrEqual(t, task.CostCents, int64(20))

	// Cleanup branch.
	t.Cleanup(func() {
		branch := task.BranchName.String
		if branch == "" {
			return
		}
		url := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git",
			mintGhToken(t), os.Getenv("PI_GITHUB_SANDBOX_REPO"))
		if out, err := exec.Command("git", "push", url, "--delete", branch).CombinedOutput(); err != nil {
			t.Logf("cleanup failed: %v\n%s", err, out)
		}
	})
}

func requireEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if os.Getenv(n) == "" {
			t.Skipf("%s not set", n)
		}
	}
}

func requireImageM1(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", m1RunnerImage).Run(); err != nil {
		t.Skipf("docker image %s not built (run: make docker-runner)", m1RunnerImage)
	}
}
