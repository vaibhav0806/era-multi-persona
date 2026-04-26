package swarm

import (
	"fmt"
	"strings"

	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

// plannerShaper records what the planner persona produced for a task.
// Format: `task: "<desc>" | plan: <first 3 plan lines>`.
// Outcome (approve/flag) isn't known at planner-write-time (reviewer hasn't
// run yet); the reviewer's own memory closes the loop on its side.
func plannerShaper(in brain.Input, out brain.Output) string {
	desc := truncateUTF8(in.TaskDescription, 80)
	plan := firstNLines(out.Text, 3)
	plan = truncateUTF8(plan, 100)
	return fmt.Sprintf("task: %q | plan: %s", desc, plan)
}

// reviewerShaper records reviewer's task + decision + critique snippet.
func reviewerShaper(in brain.Input, out brain.Output) string {
	desc := truncateUTF8(in.TaskDescription, 80)
	decision := string(parseDecision(out.Text)) // existing helper in swarm.go
	headline := firstNLines(out.Text, 1)
	headline = truncateUTF8(headline, 100)
	return fmt.Sprintf("task: %q | decision: %s | %s", desc, decision, headline)
}

// firstNLines returns the first n newline-delimited lines of s, joined back
// with spaces. If s has fewer lines, returns s unchanged.
func firstNLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " ")
}

// truncateUTF8 returns the first up-to-n bytes of s. If s is shorter, returns
// s unchanged. Conservative ASCII-byte cap; rune-safe truncation is overkill
// for shaper output (our prompts are ASCII-dominant).
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
