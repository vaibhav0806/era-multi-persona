package runner_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/runner"
)

func TestParseResult(t *testing.T) {
	out := bytes.NewBufferString(`some log line
another log
RESULT branch=agent/3/foo summary=dummy-commit-ok
`)
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
	// RESULT line with only branch=. summary stays empty.
	out := bytes.NewBufferString("RESULT branch=agent/7/x\n")
	o, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/7/x", o.Branch)
	require.Equal(t, "", o.Summary)
}

func TestParseResult_MultipleRESULTLinesUsesFirst(t *testing.T) {
	// If two RESULT lines somehow appear, parser picks the first (most
	// conservative — later work may have failed).
	out := bytes.NewBufferString(`RESULT branch=agent/1/a summary=one
RESULT branch=agent/1/b summary=two
`)
	o, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/1/a", o.Branch)
	require.Equal(t, "one", o.Summary)
}

func TestParseResult_ExtendedWithTokensAndCost(t *testing.T) {
	r := bytes.NewBufferString("RESULT branch=a/1/x summary=ok tokens=12345 cost_cents=17\n")
	o, err := runner.ParseResult(r)
	require.NoError(t, err)
	require.Equal(t, "a/1/x", o.Branch)
	require.Equal(t, "ok", o.Summary)
	require.Equal(t, int64(12345), o.Tokens)
	require.Equal(t, 17, o.CostCents)
}
