package swarm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

func TestPlannerShaper_FormatsObservation(t *testing.T) {
	in := brain.Input{TaskDescription: "add /healthz endpoint to the existing HTTP server"}
	out := brain.Output{Text: "1. Find the router file\n2. Add a new GET handler\n3. Add a passing test"}
	got := plannerShaper(in, out)
	require.Contains(t, got, "task: ")
	require.Contains(t, got, "plan: ")
	require.Contains(t, got, "Find the router file")
	require.LessOrEqual(t, len(got), 250, "observation should stay bounded")
}

func TestPlannerShaper_TruncatesLongTask(t *testing.T) {
	longTask := strings.Repeat("x", 200)
	in := brain.Input{TaskDescription: longTask}
	out := brain.Output{Text: "1. step"}
	got := plannerShaper(in, out)
	require.LessOrEqual(t, len(got), 250)
}

func TestReviewerShaper_FormatsObservation(t *testing.T) {
	in := brain.Input{TaskDescription: "add /healthz"}
	out := brain.Output{Text: "no issues found\nDECISION: approve"}
	got := reviewerShaper(in, out)
	require.Contains(t, got, "task: ")
	require.Contains(t, got, "decision: approve")
	require.LessOrEqual(t, len(got), 250)
}

func TestReviewerShaper_FlagDecision(t *testing.T) {
	in := brain.Input{TaskDescription: "remove cache layer"}
	out := brain.Output{Text: "(a) Deviations from plan: tests removed\nDECISION: flag"}
	got := reviewerShaper(in, out)
	require.Contains(t, got, "decision: flag")
}
