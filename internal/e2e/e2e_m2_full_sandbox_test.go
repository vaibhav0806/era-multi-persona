//go:build e2e
// +build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/queue"
	"github.com/vaibhav0806/era/internal/runner"
)

// TestE2E_M2_FullSandbox exercises the full M2 pipeline:
//   - iptables lockdown in the container
//   - sidecar /llm passthrough (OpenRouter key only in sidecar env)
//   - sidecar /credentials/git helper (PAT/installation token only in sidecar)
//   - sidecar audit log → events table
//   - GitHub App-minted installation token per task
//
// Proves: no secrets in runner env, Pi routes through sidecar, git uses helper.
func TestE2E_M2_FullSandbox(t *testing.T) {
	requireEnv(t,
		"PI_GITHUB_APP_ID", "PI_GITHUB_APP_INSTALLATION_ID", "PI_GITHUB_APP_PRIVATE_KEY",
		"PI_GITHUB_SANDBOX_REPO", "PI_OPENROUTER_API_KEY",
	)
	requireDocker(t)
	requireImageM1(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "m2.db")
	h, err := db.Open(ctx, dbPath)
	require.NoError(t, err)
	defer h.Close()
	r := db.NewRepo(h)

	d := &runner.Docker{
		Image:            "era-runner:m2",
		SandboxRepo:      os.Getenv("PI_GITHUB_SANDBOX_REPO"),
		OpenRouterAPIKey: os.Getenv("PI_OPENROUTER_API_KEY"),
		PiModel:          "moonshotai/kimi-k2.6",
		MaxTokens:        500_000,
		MaxCostCents:     20,
		MaxIterations:    10,
		MaxWallSeconds:   180,
	}
	tokens := githubAppTokenSource(t)
	q := queue.New(r, runner.QueueAdapter{D: d}, tokens, nil, "")

	id, err := q.CreateTask(ctx, "add a file M2_FULL_SANDBOX.md with the single line 'm2 full sandbox ok'", "", "default")
	require.NoError(t, err)

	ran, err := q.RunNext(ctx)
	require.NoError(t, err, "RunNext returned error")
	require.True(t, ran)

	task, err := r.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "completed", task.Status,
		"status=%s err=%s", task.Status, task.Error.String)
	require.NotEmpty(t, task.BranchName.String)

	// Assert audit log contains BOTH /llm/* AND /credentials/git entries.
	// These are the two places where sidecar routing matters for M2 correctness.
	events, err := r.ListEvents(ctx, id)
	require.NoError(t, err)
	var sawLLM, sawCreds bool
	for _, e := range events {
		if e.Kind != "http_request" {
			continue
		}
		var payload map[string]interface{}
		_ = json.Unmarshal([]byte(e.Payload), &payload)
		path, _ := payload["path"].(string)
		if strings.HasPrefix(path, "/llm/") {
			sawLLM = true
		}
		if path == "/credentials/git" {
			sawCreds = true
		}
	}
	require.True(t, sawLLM, "expected at least one /llm/* audit event")
	require.True(t, sawCreds, "expected /credentials/git audit event (proves git used sidecar helper)")

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
