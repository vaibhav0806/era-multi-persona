package runner_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/pi-agent/internal/runner"
)

func TestParseResult(t *testing.T) {
	out := bytes.NewBufferString(`some log line
another log
RESULT branch=agent/3/foo summary=dummy-commit-ok
`)
	branch, summary, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/3/foo", branch)
	require.Equal(t, "dummy-commit-ok", summary)
}

func TestParseResult_Missing(t *testing.T) {
	out := bytes.NewBufferString("nope\nnothing here\n")
	_, _, err := runner.ParseResult(out)
	require.ErrorIs(t, err, runner.ErrNoResult)
}

func TestParseResult_OnlyBranchNoSummary(t *testing.T) {
	// The entrypoint always emits both, but parser should tolerate a
	// RESULT line with only branch=. summary stays empty.
	out := bytes.NewBufferString("RESULT branch=agent/7/x\n")
	branch, summary, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/7/x", branch)
	require.Equal(t, "", summary)
}

func TestParseResult_MultipleRESULTLinesUsesFirst(t *testing.T) {
	// If two RESULT lines somehow appear, parser picks the first (most
	// conservative — later work may have failed).
	out := bytes.NewBufferString(`RESULT branch=agent/1/a summary=one
RESULT branch=agent/1/b summary=two
`)
	branch, summary, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/1/a", branch)
	require.Equal(t, "one", summary)
}
