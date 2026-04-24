package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
)

// makefileTestTarget matches a `test:` target at the start of a line.
// Excludes `test-*:`, `test_*:`, commented lines, and indented recipe lines.
var makefileTestTarget = regexp.MustCompile(`(?m)^test\s*:`)

// HasMakefileTest returns true iff workspace/Makefile exists and contains a
// `test` target declaration at column 0.
func HasMakefileTest(workspace string) bool {
	f, err := os.Open(filepath.Join(workspace, "Makefile"))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if makefileTestTarget.MatchString(sc.Text()) {
			return true
		}
	}
	return false
}
