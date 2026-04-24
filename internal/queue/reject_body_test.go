package queue_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/diffscan"
	"github.com/vaibhav0806/era/internal/queue"
)

func TestRejectionCommentBody_WithFindings(t *testing.T) {
	body := queue.RejectionCommentBody([]diffscan.Finding{
		{Rule: "skip_directive", Path: "test_sample.js", Message: "describe.skip()"},
		{Rule: "removed_test", Path: "foo_test.go", Message: "TestFoo removed"},
	})
	require.Contains(t, body, "Rejected via era")
	require.Contains(t, body, "Branch deleted")
	require.Contains(t, body, "skip_directive")
	require.Contains(t, body, "test_sample.js")
	require.Contains(t, body, "removed_test")
}

func TestRejectionCommentBody_NoFindings(t *testing.T) {
	body := queue.RejectionCommentBody(nil)
	require.Contains(t, body, "Rejected via era")
	require.Contains(t, body, "Branch deleted")
	// Empty findings should not produce "Findings:" header
	require.NotContains(t, body, "Findings:")
}
