package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteResult(t *testing.T) {
	var buf bytes.Buffer
	writeResult(&buf, runResult{
		Branch:    "agent/1/foo",
		Summary:   "added hello",
		Tokens:    12345,
		CostCents: 17,
	})
	got := buf.String()
	require.Equal(t, "RESULT branch=agent/1/foo summary=added_hello tokens=12345 cost_cents=17\n", got)
}

func TestWriteResult_EmptySummaryOK(t *testing.T) {
	var buf bytes.Buffer
	writeResult(&buf, runResult{Branch: "b", Summary: "", Tokens: 1, CostCents: 1})
	require.Equal(t, "RESULT branch=b summary= tokens=1 cost_cents=1\n", buf.String())
}
