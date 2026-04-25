package runner_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/progress"
	"github.com/vaibhav0806/era/internal/runner"
)

func TestParseResult(t *testing.T) {
	out := bytes.NewBufferString("some log line\nanother log\n" +
		`RESULT {"branch":"agent/3/foo","summary":"dummy-commit-ok","tokens":0,"cost_cents":0}` + "\n")
	o, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/3/foo", o.Branch)
	require.Equal(t, "dummy-commit-ok", o.Summary)
}

func TestParseResult_Missing(t *testing.T) {
	out := bytes.NewBufferString("nope\nnothing here\n")
	_, err := runner.ParseResult(out)
	require.ErrorIs(t, err, runner.ErrNoResult)
}

func TestParseResult_OnlyBranchNoSummary(t *testing.T) {
	// The entrypoint always emits both, but parser should tolerate a
	// RESULT line with only branch. summary stays empty.
	out := bytes.NewBufferString(`RESULT {"branch":"agent/7/x","summary":"","tokens":0,"cost_cents":0}` + "\n")
	o, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/7/x", o.Branch)
	require.Equal(t, "", o.Summary)
}

func TestParseResult_MultipleRESULTLinesUsesFirst(t *testing.T) {
	// If two RESULT lines somehow appear, parser picks the first (most
	// conservative — later work may have failed).
	out := bytes.NewBufferString(
		`RESULT {"branch":"agent/1/a","summary":"one","tokens":0,"cost_cents":0}` + "\n" +
			`RESULT {"branch":"agent/1/b","summary":"two","tokens":0,"cost_cents":0}` + "\n")
	o, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/1/a", o.Branch)
	require.Equal(t, "one", o.Summary)
}

func TestParseResult_ExtendedWithTokensAndCost(t *testing.T) {
	r := bytes.NewBufferString(`RESULT {"branch":"a/1/x","summary":"ok","tokens":12345,"cost_cents":17}` + "\n")
	o, err := runner.ParseResult(r)
	require.NoError(t, err)
	require.Equal(t, "a/1/x", o.Branch)
	require.Equal(t, "ok", o.Summary)
	require.Equal(t, int64(12345), o.Tokens)
	require.Equal(t, 17, o.CostCents)
}

func TestParseResult_SummaryWithSpacesAndNewlines(t *testing.T) {
	combined := strings.NewReader(`RESULT {"branch":"foo","summary":"hello world\nand a newline","tokens":100,"cost_cents":5}` + "\n")
	out, err := runner.ParseResult(combined)
	require.NoError(t, err)
	require.Equal(t, "hello world\nand a newline", out.Summary)
}

func TestBuildDockerArgs_IncludesNameFlag(t *testing.T) {
	d := &runner.Docker{Image: "test-img", PiModel: "m"}
	in := runner.RunInput{
		TaskID:        42,
		Repo:          "o/r",
		Description:   "x",
		GitHubToken:   "tok",
		ContainerName: "era-runner-42-xyz",
	}
	args := d.BuildDockerArgs(in)
	var found bool
	for i, a := range args {
		if a == "--name" && i+1 < len(args) && args[i+1] == "era-runner-42-xyz" {
			found = true
			break
		}
	}
	require.True(t, found, "--name era-runner-42-xyz missing: %v", args)
}

func TestBuildDockerArgs_OmitsNameWhenBlank(t *testing.T) {
	d := &runner.Docker{Image: "test-img", PiModel: "m"}
	in := runner.RunInput{TaskID: 1, Repo: "o/r", Description: "x"}
	args := d.BuildDockerArgs(in)
	for i, a := range args {
		if a == "--name" && i+1 < len(args) {
			t.Fatalf("--name should be omitted when ContainerName empty; got %v", args)
		}
	}
}

func TestBuildDockerArgs_PerTaskCapsOverrideDefaults(t *testing.T) {
	d := &runner.Docker{
		Image:          "test:v1",
		MaxIterations:  30,
		MaxCostCents:   5,
		MaxWallSeconds: 600,
	}
	in := runner.RunInput{
		TaskID:      1,
		Repo:        "o/r",
		Description: "x",
		MaxIter:     120,
		MaxCents:    100,
		MaxWallSec:  3600,
	}
	args := d.BuildDockerArgs(in)
	requireEnvSet(t, args, "ERA_MAX_ITERATIONS=120")
	requireEnvSet(t, args, "ERA_MAX_COST_CENTS=100")
	requireEnvSet(t, args, "ERA_MAX_WALL_SECONDS=3600")
}

func TestBuildDockerArgs_ZeroFieldsFallBackToDocker(t *testing.T) {
	d := &runner.Docker{
		Image:          "test:v1",
		MaxIterations:  60,
		MaxCostCents:   20,
		MaxWallSeconds: 1800,
	}
	in := runner.RunInput{TaskID: 1, Repo: "o/r"}
	args := d.BuildDockerArgs(in)
	requireEnvSet(t, args, "ERA_MAX_ITERATIONS=60")
	requireEnvSet(t, args, "ERA_MAX_COST_CENTS=20")
	requireEnvSet(t, args, "ERA_MAX_WALL_SECONDS=1800")
}

func TestStreamToWithProgress_FiresCallback(t *testing.T) {
	input := strings.Join([]string{
		"regular log line",
		`PROGRESS {"iter":1,"action":"read","tokens_cum":100,"cost_cents_cum":0}`,
		`PROGRESS {"iter":2,"action":"write","tokens_cum":500,"cost_cents_cum":1}`,
		`RESULT {"branch":"x","summary":"y","tokens":500,"cost_cents":1}`,
	}, "\n")

	var mu sync.Mutex
	var combined strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	var got []progress.Event
	onProgress := func(ev progress.Event) { got = append(got, ev) }
	go runner.StreamToWithProgress(&mu, strings.NewReader(input), &combined, &wg, onProgress)
	wg.Wait()

	require.Len(t, got, 2)
	require.Equal(t, 1, got[0].Iter)
	require.Equal(t, "read", got[0].Action)
	require.Equal(t, 2, got[1].Iter)
	require.Contains(t, combined.String(), "RESULT")
}

func TestStreamToWithProgress_MalformedJSON_Ignored(t *testing.T) {
	input := `PROGRESS {bad json` + "\n" + `RESULT {"branch":"x","summary":"y","tokens":0,"cost_cents":0}`
	var mu sync.Mutex
	var combined strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	called := 0
	onProgress := func(ev progress.Event) { called++ }
	go runner.StreamToWithProgress(&mu, strings.NewReader(input), &combined, &wg, onProgress)
	wg.Wait()
	require.Equal(t, 0, called)
}

func requireEnvSet(t *testing.T, args []string, want string) {
	t.Helper()
	for i, a := range args {
		if a == "-e" && i+1 < len(args) && args[i+1] == want {
			return
		}
	}
	t.Fatalf("expected %s in args; got: %v", want, args)
}
