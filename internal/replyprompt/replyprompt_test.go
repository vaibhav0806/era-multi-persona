package replyprompt_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/replyprompt"
)

func TestComposeReplyPrompt_HappyPath(t *testing.T) {
	orig := db.Task{
		ID:          5,
		Description: "build a URL shortener",
		BranchName:  sql.NullString{String: "agent/5/foo", Valid: true},
		PrNumber:    sql.NullInt64{Int64: 12, Valid: true},
		Summary:     sql.NullString{String: "I built it", Valid: true},
		Status:      "completed",
	}
	out := replyprompt.ComposeReplyPrompt(orig, "now add tests")
	require.Contains(t, out, "task #5")
	require.Contains(t, out, "build a URL shortener")
	require.Contains(t, out, "agent/5/foo")
	require.Contains(t, out, "#12")
	require.Contains(t, out, "I built it")
	require.Contains(t, out, "now add tests")
}

func TestComposeReplyPrompt_NoBranchNoSummary(t *testing.T) {
	orig := db.Task{
		ID:          7,
		Description: "what is in main.go",
		Status:      "completed",
	}
	out := replyprompt.ComposeReplyPrompt(orig, "tell me more")
	require.Contains(t, out, "task #7")
	require.Contains(t, out, "tell me more")
	require.NotContains(t, out, "branch")
	require.NotContains(t, out, "PR")
}

func TestComposeReplyPrompt_FailedTask(t *testing.T) {
	orig := db.Task{
		ID:          9,
		Description: "thing that broke",
		Status:      "failed",
		Error:       sql.NullString{String: "exit status 137", Valid: true},
	}
	out := replyprompt.ComposeReplyPrompt(orig, "try again with smaller scope")
	require.Contains(t, out, "failed")
	require.Contains(t, out, "exit status 137")
	require.Contains(t, out, "try again")
}
