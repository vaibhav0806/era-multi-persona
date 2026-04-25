package replyprompt

import (
	"fmt"
	"strings"

	"github.com/vaibhav0806/era/internal/db"
)

// ComposeReplyPrompt builds the prompt for a reply-threaded task.
// Non-transitive: orig is the task the user replied to, not a chain.
func ComposeReplyPrompt(orig db.Task, replyBody string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You previously completed task #%d: %q\n", orig.ID, orig.Description)
	if orig.BranchName.Valid && orig.BranchName.String != "" {
		fmt.Fprintf(&b, "You made changes on branch %s.\n", orig.BranchName.String)
	}
	if orig.PrNumber.Valid {
		fmt.Fprintf(&b, "The pull request is #%d.\n", orig.PrNumber.Int64)
	}
	if orig.Summary.Valid && strings.TrimSpace(orig.Summary.String) != "" {
		fmt.Fprintf(&b, "\nSummary of what you did:\n%s\n", orig.Summary.String)
	}
	if orig.Status == "failed" && orig.Error.Valid {
		fmt.Fprintf(&b, "\nThat task failed with: %s\n", orig.Error.String)
	}
	fmt.Fprintf(&b, "\nNow the user has a follow-up: %s", replyBody)
	return b.String()
}
