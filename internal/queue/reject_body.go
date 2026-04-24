package queue

import (
	"fmt"
	"strings"

	"github.com/vaibhav0806/era/internal/diffscan"
)

// RejectionCommentBody composes the GitHub PR comment posted before closing
// a rejected task. Findings, when present, are listed for traceability.
func RejectionCommentBody(findings []diffscan.Finding) string {
	var b strings.Builder
	b.WriteString("✗ Rejected via era Telegram bot. Branch deleted.\n")
	if len(findings) > 0 {
		b.WriteString("\nFindings:\n")
		for _, f := range findings {
			fmt.Fprintf(&b, "  • %s (%s): %s\n", f.Rule, f.Path, f.Message)
		}
	}
	return b.String()
}
