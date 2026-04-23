package diffscan

import (
	"bufio"
	"strings"
)

// Scan runs all rules over a unified-diff string and returns findings.
func Scan(diff string) ([]Finding, error) {
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		return nil, err
	}
	return ScanDiffs(files), nil
}

// ScanDiffs runs all rules over parsed FileDiff values. Used by callers
// that already have structured per-file diffs (e.g. the GitHub compare
// API path in internal/githubcompare).
func ScanDiffs(files []FileDiff) []Finding {
	var out []Finding
	for _, fd := range files {
		out = append(out, RuleRemovedTests(fd)...)
		out = append(out, RuleSkipDirective(fd)...)
		out = append(out, RuleWeakenedAssertion(fd)...)
		out = append(out, RuleDeletedTestFile(fd)...)
	}
	return out
}

// parseUnifiedDiff splits a unified-diff blob into per-file FileDiff values.
// Handles:
//
//	--- a/<path>           pre-image header
//	+++ b/<path>           post-image header (or /dev/null for deleted)
//	@@ -start,len +start,len @@
//	+added / -removed / context lines
func parseUnifiedDiff(diff string) ([]FileDiff, error) {
	var files []FileDiff
	var cur FileDiff
	var lastMinus string
	inHunk := false

	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	flush := func() {
		if cur.Path != "" || cur.Deleted {
			// Deleted files need Path recovered from lastMinus if not set.
			if cur.Path == "" && cur.Deleted {
				cur.Path = lastMinus
			}
			files = append(files, cur)
		}
		cur = FileDiff{}
		inHunk = false
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "--- "):
			flush()
			lastMinus = strings.TrimPrefix(strings.TrimPrefix(line, "--- "), "a/")
		case strings.HasPrefix(line, "+++ "):
			target := strings.TrimPrefix(line, "+++ ")
			if target == "/dev/null" {
				cur.Deleted = true
				cur.Path = lastMinus
			} else {
				cur.Path = strings.TrimPrefix(target, "b/")
			}
		case strings.HasPrefix(line, "@@ "):
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			cur.Added = append(cur.Added, line[1:])
		case inHunk && strings.HasPrefix(line, "-"):
			cur.Removed = append(cur.Removed, line[1:])
		}
	}
	flush()
	return files, sc.Err()
}
