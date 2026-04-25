package main

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakePi is a deterministic replacement for the real `pi --mode json` process.
// Tests supply canned stdout (JSONL events); the driver consumes them as if
// they came from a real Pi invocation.
type fakePi struct {
	stdout  io.Reader
	stderr  io.Reader
	waitErr error
	waited  bool
}

func (f *fakePi) Stdout() (io.Reader, error) { return f.stdout, nil }
func (f *fakePi) Stderr() (io.Reader, error) { return f.stderr, nil }
func (f *fakePi) Start() error               { return nil }
func (f *fakePi) Wait() error                { f.waited = true; return f.waitErr }
func (f *fakePi) Abort() error               { return nil }

func TestPi_DrainsEventsAndAggregates(t *testing.T) {
	f := &fakePi{
		stdout: strings.NewReader(`{"type":"tool_execution_end","tool":"bash"}
{"type":"message_end","message":{"usage":{"totalTokens":10,"cost":{"total":0.01}},"stopReason":"toolUse"}}
{"type":"message_end","message":{"usage":{"totalTokens":20,"cost":{"total":0.02}},"stopReason":"endTurn"}}
{"type":"agent_end"}
`),
		stderr: strings.NewReader(""),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	summary, err := runPi(ctx, f, nopObserver{}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(30), summary.TotalTokens)
	require.InDelta(t, 0.03, summary.TotalCostUSD, 1e-9)
	require.Equal(t, 1, summary.ToolUseCount)
	require.True(t, f.waited)
}

type nopObserver struct{}

func (nopObserver) onEvent(e *piEvent) error { return nil }

func TestRunPi_TracksLastAssistantText(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"tool_execution_end","tool":"read"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"first answer"}],"usage":{"totalTokens":10,"cost":{"total":0.001}}}}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"final answer wins"}],"usage":{"totalTokens":20,"cost":{"total":0.002}}}}`,
		`{"type":"agent_end"}`,
	}, "\n")
	p := &fakePi{stdout: strings.NewReader(jsonl)}
	obs := nopObserver{}
	s, err := runPi(context.Background(), p, obs, nil)
	require.NoError(t, err)
	require.Equal(t, "final answer wins", s.LastText)
	require.Equal(t, int64(30), s.TotalTokens)
}

func TestRunPi_LastTextEmptyWhenNoAssistantMessage(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"tool_execution_end","tool":"ls"}`,
		`{"type":"agent_end"}`,
	}, "\n")
	p := &fakePi{stdout: strings.NewReader(jsonl)}
	s, err := runPi(context.Background(), p, nopObserver{}, nil)
	require.NoError(t, err)
	require.Equal(t, "", s.LastText)
}

func TestRunPi_FiresProgressOnToolExecution(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"tool_execution_end","tool":"read"}`,
		`{"type":"tool_execution_end","tool":"write"}`,
		`{"type":"agent_end"}`,
	}, "\n")
	p := &fakePi{stdout: strings.NewReader(jsonl)}

	var got []struct {
		iter   int
		action string
	}
	onProgress := func(iter int, action string, tokens int64, cost float64) {
		got = append(got, struct {
			iter   int
			action string
		}{iter, action})
	}
	_, err := runPi(context.Background(), p, nopObserver{}, onProgress)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, 1, got[0].iter)
	require.Equal(t, "read", got[0].action)
	require.Equal(t, 2, got[1].iter)
	require.Equal(t, "write", got[1].action)
}

func TestRunPi_NilProgressIsSafe(t *testing.T) {
	jsonl := `{"type":"tool_execution_end","tool":"x"}` + "\n" + `{"type":"agent_end"}`
	p := &fakePi{stdout: strings.NewReader(jsonl)}
	_, err := runPi(context.Background(), p, nopObserver{}, nil)
	require.NoError(t, err)
}

// TestNewRealPi_Flags checks that newRealPi builds the right exec.Cmd:
//   - uses --provider openrouter (not a raw API-key provider)
//   - does NOT embed a real OpenRouter key anywhere in Args or Env
//   - sets OPENROUTER_API_KEY to the dummy sentinel value
//   - sets PI_CODING_AGENT_DIR to /tmp/pi-state so Pi picks up models.json
//
// This is a regression guard: if someone re-adds --provider openrouter with
// the raw key in env, or switches to a different provider accidentally, this
// test will catch it.
func TestNewRealPi_Flags(t *testing.T) {
	// newRealPi shells out to "pi" via exec.LookPath internally; skip if not
	// present (CI without Pi installed). We only need the Cmd built, not run.
	if _, err := exec.LookPath("pi"); err != nil {
		t.Skip("pi binary not in PATH; skipping cmd-args test")
	}

	ctx := context.Background()
	p, err := newRealPi(ctx, "moonshotai/kimi-k2.6", "/tmp", "say hi")
	require.NoError(t, err)

	args := p.cmd.Args
	argsStr := strings.Join(args, " ")

	// Must use openrouter provider so models.json override routes through sidecar.
	require.Contains(t, argsStr, "--provider openrouter", "Pi must use --provider openrouter")

	// Must NOT contain a real-looking OpenRouter key (sk-or- prefix).
	for _, a := range args {
		require.NotContains(t, a, "sk-or-", "Pi args must not contain a real OpenRouter key")
	}

	// Env checks.
	var orKey, piDir string
	for _, e := range p.cmd.Env {
		if strings.HasPrefix(e, "OPENROUTER_API_KEY=") {
			orKey = strings.TrimPrefix(e, "OPENROUTER_API_KEY=")
		}
		if strings.HasPrefix(e, "PI_CODING_AGENT_DIR=") {
			piDir = strings.TrimPrefix(e, "PI_CODING_AGENT_DIR=")
		}
		// Must not carry a real key.
		require.NotContains(t, e, "sk-or-", "Pi env must not contain a real OpenRouter key")
	}
	require.Equal(t, "dummy-sidecar-injects-real", orKey,
		"OPENROUTER_API_KEY must be the dummy sentinel so sidecar injects the real one")
	require.Equal(t, "/tmp/pi-state", piDir,
		"PI_CODING_AGENT_DIR must be /tmp/pi-state so Pi picks up models.json")
}
