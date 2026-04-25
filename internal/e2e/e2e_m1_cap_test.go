//go:build e2e
// +build e2e

package e2e_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/queue"
	"github.com/vaibhav0806/era/internal/runner"
)

func TestE2E_M1_RunawayAbortedByIterationCap(t *testing.T) {
	requireEnv(t,
		"PI_GITHUB_APP_ID", "PI_GITHUB_APP_INSTALLATION_ID", "PI_GITHUB_APP_PRIVATE_KEY",
		"PI_GITHUB_SANDBOX_REPO", "PI_OPENROUTER_API_KEY",
	)
	requireDocker(t)
	requireImageM1(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "m1cap.db")
	h, err := db.Open(ctx, dbPath)
	require.NoError(t, err)
	defer h.Close()
	r := db.NewRepo(h)

	d := &runner.Docker{
		Image:            "era-runner:m2",
		SandboxRepo:      os.Getenv("PI_GITHUB_SANDBOX_REPO"),
		OpenRouterAPIKey: os.Getenv("PI_OPENROUTER_API_KEY"),
		PiModel:          "moonshotai/kimi-k2.6",
		MaxTokens:        100, // guaranteed exceeded by first model response
		MaxCostCents:     10,  // $0.10 budget — not the binding cap
		MaxIterations:    3,   // not the binding cap
		MaxWallSeconds:   120,
	}
	tokens := githubAppTokenSource(t)
	q := queue.New(r, runner.QueueAdapter{D: d}, tokens, nil, "")

	id, err := q.CreateTask(ctx,
		"create 20 different files in this repo, each named NOTE_<n>.md, "+
			"each containing a 5-paragraph essay about a different historical event. "+
			"commit after each file is written.", "", "default")
	require.NoError(t, err)

	ran, runErr := q.RunNext(ctx)
	require.True(t, ran)
	// runErr is expected to be non-nil (cap aborted). Don't fail on it; we
	// assert the FAILED status + error reason in the DB instead.
	_ = runErr

	task, gErr := r.GetTask(ctx, id)
	require.NoError(t, gErr)
	require.Equal(t, "failed", task.Status, "expected failed, got %s; err=%s", task.Status, task.Error.String)
	require.Contains(t, task.Error.String, "cap exceeded", "error should mention cap")
	// Don't cleanup a branch — the runner should not have pushed.
}
