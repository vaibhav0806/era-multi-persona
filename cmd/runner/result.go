package main

import (
	"fmt"
	"io"
	"strings"
)

// runResult is what the runner emits as its final RESULT line, parsed by the
// orchestrator's ParseResult.
type runResult struct {
	Branch    string
	Summary   string
	Tokens    int64
	CostCents int
}

// writeResult emits the RESULT line to w. Whitespace in summary is replaced
// with underscores so the line is space-delimited key=value parseable.
func writeResult(w io.Writer, r runResult) {
	summary := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' {
			return '_'
		}
		return r
	}, r.Summary)
	fmt.Fprintf(w, "RESULT branch=%s summary=%s tokens=%d cost_cents=%d\n",
		r.Branch, summary, r.Tokens, r.CostCents)
}
