# era — Milestone 3: Approvals + EOD Digest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the human-in-the-loop gap. Diff-scan every completed task for reward-hacking patterns (deleted tests, `.skip` directives, weakened assertions, deleted test files). When flagged, the orchestrator sends a Telegram DM with an inline diff preview plus Approve / Reject buttons; clean tasks auto-complete as today. Reject deletes the branch via GitHub App token. End of day (11 PM IST by default), a digest DM summarizes the day's activity. Small scope-creep bonuses: `/cancel <id>` and `/retry <id>` commands.

**Architecture:** Orchestrator-side. After a task completes (M2 path), `Queue.RunNext` fetches the diff via GitHub's `/compare/main...<branch>` endpoint (using the existing `githubapp.Client`), runs `internal/diffscan` over it, and either (a) sends the existing completion DM if no findings or (b) sets the task to `needs_review` and sends an approval DM with inline `Approve`/`Reject` buttons and a truncated inline diff preview. Telegram callback queries are routed through a new `Ops.HandleApproval` method — approve keeps the branch and marks the task `completed`; reject deletes the branch and marks the task `rejected`. A separate goroutine in `main.go` fires a cron tick once a day at the configured time and sends a digest message built by the new `internal/digest` package.

**Tech Stack:**
- Same as M2 + one new dep: none — `go-telegram-bot-api/v5` already supports inline keyboards + callback queries
- New packages: `internal/diffscan`, `internal/githubcompare`, `internal/digest`
- New migration: `0004_status_review.sql` (extends tasks.status CHECK)
- New queries: approvals table queries (table itself exists from migration 0001 but has never been used)

---

## NON-NEGOTIABLE TESTING PHILOSOPHY (still binding)

Same eight rules from M0/M1/M2, plus M3-specific:

1. TDD every task. Failing test first.
2. Every task ends with FULL suite green (`go test -race ./...`).
3. Every phase ends with a Regression Gate re-walking every prior phase's smoke checklist (M0 + M1 + M2 + every M3 phase done so far).
4. Regressions in old features block new work.
5. Manual smoke tests are written down, with expected outputs.
6. Fail loud. No swallowed errors.
7. No mocks for our own code. Mocks only at external boundaries (Telegram, GitHub API).
8. Commits are atomic and green.

**M3-specific:**

9. **Every approval transition has a positive AND negative test.** Approve path must be tested; reject path must be tested (branch deletion verified via mocked GitHub API). State-machine invariants (can't approve twice, can't reject an already-completed task) are tested explicitly.
10. **Every diff-scan rule has positive AND negative fixtures.** "Test removed" rule has a fixture diff that WOULD NOT trigger it (e.g. pure source edit) and one that WOULD. Both in the test table.
11. **Digest generation is deterministic.** Given the same task rows, the message is byte-for-byte identical. No wall-clock inside the template; the cron layer injects "as of" timestamps.
12. **Callback query handling is idempotent.** Tapping the same button twice does not advance the state machine twice. Tests cover the double-tap case.

We are cautious. We are serious. We are productive. We do not build blindly.

---

## Scope boundaries — explicit deferrals

**In scope for M3:**
- Diff-scan rule engine (4 rules: removed tests, skip directives, weakened assertions, deleted test files) + engine + integration
- GitHub `/compare` API client
- `tasks.status` enum extended with `needs_review`, `approved`, `rejected`
- Inline Telegram keyboard support on outbound messages
- Telegram callback query handling (the "tap the Approve button" path)
- Approval flow wired into `Queue.RunNext`: clean → completed + DM; flagged → needs_review + approval DM with inline diff preview + GitHub compare link
- Reject → orchestrator deletes the branch via GitHub App installation token
- EOD digest: cron goroutine + `internal/digest` package that renders a summary from the last 24h of tasks
- `/cancel <id>` — cancel a queued (not yet running) task
- `/retry <id>` — re-enqueue a task from a prior id's description

**Out of scope (M4+):**
- Mid-task approval gates (agent asks permission before tool use) — requires Pi cooperation we don't have
- Multi-user / approval delegation — FEATURE.md is explicit about single-user
- Web UI for diff preview — Telegram only
- Cancelling a currently-RUNNING task (docker kill) — scope creep; queued-only is enough for M3. Running tasks hit the M1 1-hour wall-clock cap anyway.
- Custom per-task approval policies
- Reopening a `rejected` task — user uses `/retry` for that

---

## Prerequisites (user action before Task M3-1)

None new. Everything needed is already in place after M2.

The GitHub App needs **Contents: Read and write** permission, which was already configured in M2's prereq. The branch-delete path (Phase R) uses the same App installation token.

---

## Before you start: verify M2 baseline

```bash
cd /Users/vaibhav/Documents/projects/pi-agent
git fetch origin && git checkout master && git pull --ff-only
go test -race -count=1 ./...                                  # PASS all 10 packages
go vet ./... && gofmt -l .                                    # clean
set -a && source .env && set +a
go test -tags e2e -count=1 -timeout 10m ./internal/e2e/...    # 4 tests PASS
PAT="$(grep -E '^PI_GITHUB_PAT=' .env | cut -d= -f2-)"
REPO="$(grep -E '^PI_GITHUB_SANDBOX_REPO=' .env | cut -d= -f2-)"
git ls-remote "https://x-access-token:${PAT}@github.com/${REPO}.git" | grep agent/ || echo clean
```

All must be green. If anything is red, fix it before M3-1.

---

## File Structure — M3 additions

```
era/
├── cmd/
│   └── orchestrator/main.go           # MODIFY: digest goroutine, callback routing
├── internal/
│   ├── config/                        # MODIFY: PI_DIGEST_TIME_UTC (default 17:30)
│   ├── diffscan/                      # NEW
│   │   ├── rules.go                   # individual rule implementations
│   │   ├── rules_test.go
│   │   ├── scan.go                    # engine
│   │   └── scan_test.go
│   ├── githubcompare/                 # NEW
│   │   ├── compare.go                 # HTTP client for /compare/{base}...{head}
│   │   └── compare_test.go
│   ├── digest/                        # NEW
│   │   ├── digest.go                  # aggregator + renderer
│   │   └── digest_test.go
│   ├── telegram/
│   │   ├── client.go                  # MODIFY: SendMessageWithButtons, Updates() emits callback queries
│   │   ├── client_test.go             # MODIFY: FakeClient supports inline keyboards + callbacks
│   │   ├── handler.go                 # MODIFY: route callback queries; add /cancel /retry
│   │   └── handler_test.go            # MODIFY
│   ├── queue/
│   │   ├── queue.go                   # MODIFY: diff-scan integration, approval state transitions
│   │   └── queue_run_test.go          # MODIFY
│   └── db/
│       ├── repo.go                    # MODIFY: new methods (SetStatus, RecordApproval, ListTasksSince, etc.)
│       └── repo_test.go               # MODIFY
├── migrations/
│   └── 0004_status_review.sql         # NEW: tasks.status CHECK extended
├── queries/
│   └── tasks.sql                      # MODIFY: SetStatus, RecordApproval, ListTasksBetween
├── scripts/smoke/
│   ├── phase_p_diffscan.sh            # NEW
│   ├── phase_q_callbacks.sh           # NEW
│   ├── phase_r_approvals.sh           # NEW
│   └── phase_s_digest.sh              # NEW
└── docs/superpowers/plans/
    └── 2026-04-23-m3-approvals-digest.md   # this file
```

**Package responsibility lines:**

- `internal/diffscan` — pure functions over unified-diff text. No I/O. No HTTP. Rule functions return `[]Finding`; the engine iterates rules and concatenates.
- `internal/githubcompare` — thin HTTP client around `GET /repos/{owner}/{repo}/compare/{base}...{head}`. Accepts a `Tokener` interface (the existing `githubapp.Client`) so tests don't need real GitHub.
- `internal/digest` — pure render. Takes `[]db.Task` + a time window, returns a `string` (Telegram-safe plain text). No DB. No Telegram. Only Go stdlib.
- `internal/telegram` — extended. `Client` interface gains `SendMessageWithButtons`. `Update` type gains a `Callback` variant. `Handler` routes callbacks to the `Ops` interface.
- `internal/queue` — orchestration. Imports diffscan + githubcompare. Adds `ApproveTask` / `RejectTask` methods callable from the handler.

---

## Phase overview

| Phase | Tasks | What ships | Ship-safe? |
|-------|-------|-----------|-----------|
| P. Diff-scan | M3-1 … M3-6 | diffscan rule engine + GitHub compare client + migration 0004 + Queue integration; tasks get flagged `needs_review` but approval UX not yet wired | yes (flagged tasks sit in `needs_review`; user still gets completion DM, just with a warning field) |
| Q. Inline buttons | M3-7 … M3-10 | Telegram client sends inline keyboards; handler receives callback queries; Ops interface gains `HandleApproval` | yes (new capability, no behavioral change yet) |
| R. Approval flow | M3-11 … M3-16 | `needs_review` tasks get approval DM with inline diff + Approve/Reject buttons; approve keeps branch; reject deletes via App token | yes (full approval loop working) |
| S. EOD digest + commands | M3-17 … M3-22 | daily digest at 11 PM IST; `/cancel` + `/retry` commands; m3-release | yes — M3 done |

~22 tasks total across 4 phases. Same discipline as M0/M1/M2 — each phase ends with a Regression Gate + tag, every task ends with a green suite.

---

## Phase P — Diff-scan rule engine + integration

### Task M3-1: `internal/diffscan/rules.go` — four rules

**Files:**
- Create: `internal/diffscan/rules.go`
- Create: `internal/diffscan/rules_test.go`

The rules work on a list of unified-diff hunks per file. GitHub's compare API returns exactly this shape: for each changed file, a `patch` string is unified diff starting with `@@ -old,len +new,len @@`.

Rules consume a `FileDiff` (path + added lines + removed lines) and return any `Finding` objects.

- [ ] **Step 1: Write failing rule tests**

```go
// internal/diffscan/rules_test.go
package diffscan

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleRemovedTests_Go(t *testing.T) {
	fd := FileDiff{
		Path: "foo_test.go",
		Removed: []string{
			"func TestBar(t *testing.T) {",
			"\trequire.Equal(t, 1, 1)",
			"}",
		},
	}
	findings := RuleRemovedTests(fd)
	require.NotEmpty(t, findings)
	require.Contains(t, findings[0].Message, "TestBar")
}

func TestRuleRemovedTests_Python(t *testing.T) {
	fd := FileDiff{
		Path: "test_foo.py",
		Removed: []string{"def test_bar():", "    assert 1 == 1"},
	}
	require.NotEmpty(t, RuleRemovedTests(fd))
}

func TestRuleRemovedTests_JS(t *testing.T) {
	fd := FileDiff{
		Path: "foo.test.js",
		Removed: []string{"  it('bar', () => {", "    expect(true).toBe(true);"},
	}
	require.NotEmpty(t, RuleRemovedTests(fd))
}

func TestRuleRemovedTests_NonTestChangeClean(t *testing.T) {
	// Removing a non-test line from a test file is fine — only test declarations count.
	fd := FileDiff{
		Path: "foo_test.go",
		Removed: []string{"\t// comment removed"},
	}
	require.Empty(t, RuleRemovedTests(fd))
}

func TestRuleSkipDirective_Go(t *testing.T) {
	fd := FileDiff{
		Path: "foo_test.go",
		Added: []string{"\tt.Skip(\"flaky\")"},
	}
	require.NotEmpty(t, RuleSkipDirective(fd))
}

func TestRuleSkipDirective_Pytest(t *testing.T) {
	fd := FileDiff{
		Path: "test_foo.py",
		Added: []string{"@pytest.mark.skip(reason=\"flaky\")"},
	}
	require.NotEmpty(t, RuleSkipDirective(fd))
}

func TestRuleSkipDirective_Jest(t *testing.T) {
	fd := FileDiff{Path: "foo.test.js", Added: []string{"it.skip('bar', ...)"}}
	require.NotEmpty(t, RuleSkipDirective(fd))
	fd = FileDiff{Path: "foo.test.js", Added: []string{"xit('bar', ...)"}}
	require.NotEmpty(t, RuleSkipDirective(fd))
}

func TestRuleSkipDirective_Clean(t *testing.T) {
	fd := FileDiff{Path: "foo_test.go", Added: []string{"\tt.Run(\"ok\", ...)"}}
	require.Empty(t, RuleSkipDirective(fd))
}

func TestRuleWeakenedAssertion(t *testing.T) {
	fd := FileDiff{
		Path: "foo_test.go",
		Added: []string{"\trequire.True(t, true)"},
	}
	require.NotEmpty(t, RuleWeakenedAssertion(fd))

	fd = FileDiff{Path: "foo.test.js", Added: []string{"    expect(true).toBe(true)"}}
	require.NotEmpty(t, RuleWeakenedAssertion(fd))

	fd = FileDiff{Path: "test_foo.py", Added: []string{"    assert True"}}
	require.NotEmpty(t, RuleWeakenedAssertion(fd))
}

func TestRuleWeakenedAssertion_Clean(t *testing.T) {
	fd := FileDiff{Path: "foo_test.go", Added: []string{"\trequire.Equal(t, 42, x)"}}
	require.Empty(t, RuleWeakenedAssertion(fd))
}

func TestRuleDeletedTestFile(t *testing.T) {
	fd := FileDiff{
		Path:    "foo_test.go",
		Deleted: true,
	}
	require.NotEmpty(t, RuleDeletedTestFile(fd))
}

func TestRuleDeletedTestFile_NonTestFileIgnored(t *testing.T) {
	fd := FileDiff{Path: "foo.go", Deleted: true}
	require.Empty(t, RuleDeletedTestFile(fd))
}
```

- [ ] **Step 2: Confirm failure**

Run: `go test ./internal/diffscan/...`
Expected: build error.

- [ ] **Step 3: Implement rules.go**

```go
// Package diffscan detects reward-hacking patterns in unified diffs —
// removed tests, skip directives, weakened assertions, deleted test files.
// Pure functions over FileDiff values; no I/O.
package diffscan

import (
	"regexp"
	"strings"
)

// FileDiff is the per-file view of a change. Added/Removed are lists of
// raw lines (without the leading +/- of unified diff). Deleted is true
// when the file is removed entirely.
type FileDiff struct {
	Path    string
	Added   []string
	Removed []string
	Deleted bool
}

// Finding is a single diffscan observation.
type Finding struct {
	Rule    string
	Path    string
	Line    string
	Message string
}

var (
	// Matches test declarations across Go, Python (pytest), JS/TS (jest, mocha).
	testDeclRE = regexp.MustCompile(
		`^\s*(func Test[A-Z]\w*|def test_\w+|it\s*\(|test\s*\(|describe\s*\()`,
	)

	// Skip directives across frameworks.
	skipRE = regexp.MustCompile(
		`(t\.Skip\b|t\.Skipf\b|\.skip\s*\(|^\s*xit\s*\(|^\s*xtest\s*\(|@pytest\.mark\.skip|@unittest\.skip|pytest\.skip\s*\()`,
	)

	// Weakened assertions: tautologies or bare truth.
	weakRE = regexp.MustCompile(
		`(require\.True\s*\(\s*t\s*,\s*true\b|assert\.True\s*\(\s*t\s*,\s*true\b|` +
			`expect\s*\(\s*true\s*\)\.(toBe|toEqual)\s*\(\s*true\s*\)|` +
			`assert\s+True\s*$|assert\s*\(\s*True\s*\)\s*$|` +
			`expect\s*\(\s*1\s*\)\.(toBe|toEqual)\s*\(\s*1\s*\))`,
	)

	// Filenames that count as test files.
	testFileRE = regexp.MustCompile(`(_test\.go|\.test\.(js|ts|jsx|tsx)|^test_[^/]+\.py|/test_[^/]+\.py)$`)
)

func isTestFile(path string) bool { return testFileRE.MatchString(path) }

func RuleRemovedTests(fd FileDiff) []Finding {
	var out []Finding
	if !isTestFile(fd.Path) {
		return out
	}
	for _, line := range fd.Removed {
		if testDeclRE.MatchString(line) {
			out = append(out, Finding{
				Rule: "removed_test", Path: fd.Path, Line: line,
				Message: "test declaration removed: " + strings.TrimSpace(line),
			})
		}
	}
	return out
}

func RuleSkipDirective(fd FileDiff) []Finding {
	var out []Finding
	for _, line := range fd.Added {
		if skipRE.MatchString(line) {
			out = append(out, Finding{
				Rule: "skip_directive", Path: fd.Path, Line: line,
				Message: "skip directive added: " + strings.TrimSpace(line),
			})
		}
	}
	return out
}

func RuleWeakenedAssertion(fd FileDiff) []Finding {
	var out []Finding
	if !isTestFile(fd.Path) {
		return out
	}
	for _, line := range fd.Added {
		if weakRE.MatchString(line) {
			out = append(out, Finding{
				Rule: "weakened_assertion", Path: fd.Path, Line: line,
				Message: "tautological/weak assertion added: " + strings.TrimSpace(line),
			})
		}
	}
	return out
}

func RuleDeletedTestFile(fd FileDiff) []Finding {
	if !fd.Deleted || !isTestFile(fd.Path) {
		return nil
	}
	return []Finding{{
		Rule: "deleted_test_file", Path: fd.Path,
		Message: "test file deleted: " + fd.Path,
	}}
}
```

- [ ] **Step 4: Tests pass**

Run: `go test -race -v ./internal/diffscan/...`
Expected: all 11 tests PASS.

- [ ] **Step 5: Full suite + commit**

```bash
go test -race ./...
git add internal/diffscan/
git commit -m "feat(diffscan): 4 reward-hacking rules (removed tests, skip, weakened, deleted)"
```

---

### Task M3-2: `internal/diffscan/scan.go` — engine + unified-diff parser

**Files:**
- Create: `internal/diffscan/scan.go`
- Create: `internal/diffscan/scan_test.go`

The engine turns a raw unified-diff string (or a list of GitHub compare API file entries) into `[]Finding` by running all rules against each file.

- [ ] **Step 1: Failing tests**

```go
package diffscan_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/diffscan"
)

func TestScan_UnifiedDiff_Clean(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
-func Hello() { return "hi" }
+func Hello() { return "hello" }
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	require.Empty(t, f)
}

func TestScan_UnifiedDiff_RemovedTest(t *testing.T) {
	diff := `--- a/foo_test.go
+++ b/foo_test.go
@@ -1,5 +1,1 @@
-func TestBar(t *testing.T) {
-    require.Equal(t, 1, 1)
-}
-
 func TestBaz(t *testing.T) {}
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	require.Len(t, f, 1)
	require.Equal(t, "removed_test", f[0].Rule)
}

func TestScan_UnifiedDiff_DeletedFile(t *testing.T) {
	diff := `--- a/bar_test.go
+++ /dev/null
@@ -1,5 +0,0 @@
-func TestBar(t *testing.T) {
-    require.Equal(t, 1, 1)
-}
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	// Should flag deleted_test_file (and possibly removed_test too)
	hasDeleted := false
	for _, fn := range f {
		if fn.Rule == "deleted_test_file" {
			hasDeleted = true
		}
	}
	require.True(t, hasDeleted)
}

func TestScan_Multifile(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,1 @@
-a
+b
--- a/bar_test.go
+++ b/bar_test.go
@@ -1,0 +1,1 @@
+t.Skip("flaky")
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	require.Len(t, f, 1)
	require.Equal(t, "skip_directive", f[0].Rule)
}
```

- [ ] **Step 2: Confirm failure**

Run: `go test ./internal/diffscan/...`
Expected: build error — `Scan` missing.

- [ ] **Step 3: Implement scan.go**

```go
package diffscan

import (
	"bufio"
	"strings"
)

// Scan runs all rules over a unified-diff string and returns the combined
// findings. File boundaries are detected by "--- a/<path>" / "+++ b/<path>"
// headers. A "+++ /dev/null" header marks a deleted file.
func Scan(diff string) ([]Finding, error) {
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, fd := range files {
		out = append(out, RuleRemovedTests(fd)...)
		out = append(out, RuleSkipDirective(fd)...)
		out = append(out, RuleWeakenedAssertion(fd)...)
		out = append(out, RuleDeletedTestFile(fd)...)
	}
	return out, nil
}

// ScanFiles is the structured entrypoint used by the GitHub compare-API
// caller: each file's patch is parsed separately. We concatenate + hand off
// to parseUnifiedDiff for simplicity; callers who already have FileDiff
// values can use ScanDiffs.
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

func parseUnifiedDiff(diff string) ([]FileDiff, error) {
	var files []FileDiff
	var cur FileDiff
	inHunk := false
	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	flush := func() {
		if cur.Path != "" {
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
			// Header reset; wait for +++ to determine path + deleted state.
		case strings.HasPrefix(line, "+++ "):
			target := strings.TrimPrefix(line, "+++ ")
			if target == "/dev/null" {
				cur.Deleted = true
				// keep cur.Path from `--- a/<path>` if we recorded it;
				// otherwise try to recover from the pre-image header.
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
	// Deleted files need path recovered from the "--- a/<path>" line,
	// which we lost above. Re-parse to grab it.
	// (Simpler than stateful tracking.)
	if err := recoverDeletedPaths(diff, files); err != nil {
		return nil, err
	}
	return files, sc.Err()
}

// recoverDeletedPaths fills in Path for files marked Deleted by reading the
// "--- a/<path>" preceding each "+++ /dev/null".
func recoverDeletedPaths(diff string, files []FileDiff) error {
	// Build index: walk diff, track lastMinus; when we hit "+++ /dev/null",
	// the matching file has Deleted=true and needs lastMinus as Path.
	var lastMinus string
	idx := 0
	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "--- ") {
			lastMinus = strings.TrimPrefix(strings.TrimPrefix(line, "--- "), "a/")
		}
		if strings.HasPrefix(line, "+++ ") {
			if idx < len(files) {
				if files[idx].Deleted && files[idx].Path == "" {
					files[idx].Path = lastMinus
				}
				idx++
			}
		}
	}
	return sc.Err()
}
```

- [ ] **Step 4: Tests pass**

Run: `go test -race -v ./internal/diffscan/...`
Expected: all tests PASS (11 rule tests + 4 scan tests = 15).

- [ ] **Step 5: Full suite + commit**

```bash
go test -race ./...
git add internal/diffscan/
git commit -m "feat(diffscan): unified-diff parser + Scan engine"
```

---

### Task M3-3: `internal/githubcompare` — compare API client

**Files:**
- Create: `internal/githubcompare/compare.go`
- Create: `internal/githubcompare/compare_test.go`

Fetches `GET /repos/{owner}/{repo}/compare/{base}...{head}` using an installation token. Returns the diff as a unified-diff string OR as a list of `FileDiff` values ready for `diffscan.ScanDiffs`.

- [ ] **Step 1: Failing tests**

```go
package githubcompare_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/githubcompare"
)

type staticTokener struct{ tok string }

func (s staticTokener) InstallationToken(ctx context.Context) (string, error) {
	return s.tok, nil
}

func TestCompare_ReturnsFileDiffs(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.Contains(t, r.URL.Path, "/repos/alice/bob/compare/main...feature")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"files": []map[string]interface{}{
				{
					"filename": "foo.go",
					"status":   "modified",
					"patch":    "@@ -1,1 +1,1 @@\n-a\n+b\n",
				},
				{
					"filename": "old_test.go",
					"status":   "removed",
					"patch":    "@@ -1,1 +0,0 @@\n-func TestX(t *testing.T) {}\n",
				},
			},
		})
	}))
	defer gh.Close()

	c := githubcompare.New(gh.URL, staticTokener{tok: "test-token"})
	diffs, err := c.Compare(context.Background(), "alice/bob", "main", "feature")
	require.NoError(t, err)
	require.Len(t, diffs, 2)
	require.Equal(t, "foo.go", diffs[0].Path)
	require.True(t, diffs[1].Deleted)
	require.Equal(t, "old_test.go", diffs[1].Path)
}

func TestCompare_HTTPError(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer gh.Close()
	c := githubcompare.New(gh.URL, staticTokener{tok: "t"})
	_, err := c.Compare(context.Background(), "x/y", "main", "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}
```

- [ ] **Step 2: Implement compare.go**

```go
// Package githubcompare fetches diff data from GitHub's compare API and
// maps it to diffscan.FileDiff.
package githubcompare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vaibhav0806/era/internal/diffscan"
)

// Tokener yields an installation token. Implemented by *githubapp.Client.
type Tokener interface {
	InstallationToken(ctx context.Context) (string, error)
}

type Client struct {
	base    string
	tokener Tokener
	http    *http.Client
}

func New(baseURL string, t Tokener) *Client {
	b := strings.TrimRight(baseURL, "/")
	if b == "" {
		b = "https://api.github.com"
	}
	return &Client{base: b, tokener: t, http: &http.Client{Timeout: 20 * 1_000_000_000}}
}

// Compare returns the per-file diffs between base and head on repo (owner/repo).
func (c *Client) Compare(ctx context.Context, repo, base, head string) ([]diffscan.FileDiff, error) {
	tok, err := c.tokener.InstallationToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("installation token: %w", err)
	}
	url := fmt.Sprintf("%s/repos/%s/compare/%s...%s", c.base, repo, base, head)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github compare %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Files []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
			Patch    string `json:"patch"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var out []diffscan.FileDiff
	for _, f := range parsed.Files {
		fd := diffscan.FileDiff{Path: f.Filename, Deleted: f.Status == "removed"}
		// Patch is unified-diff for this one file (without file headers).
		for _, line := range strings.Split(f.Patch, "\n") {
			switch {
			case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "@@"):
				continue
			case strings.HasPrefix(line, "+"):
				fd.Added = append(fd.Added, line[1:])
			case strings.HasPrefix(line, "-"):
				fd.Removed = append(fd.Removed, line[1:])
			}
		}
		out = append(out, fd)
	}
	return out, nil
}
```

- [ ] **Step 3: Tests pass + commit**

```bash
go test -race -v ./internal/githubcompare/...
go test -race ./...
git add internal/githubcompare/
git commit -m "feat(githubcompare): compare-API client returning diffscan.FileDiff"
```

---

### Task M3-4: Migration 0004 — extend tasks.status CHECK

**Files:**
- Create: `migrations/0004_status_review.sql`

SQLite can't ALTER CHECK — recreate the table.

- [ ] **Step 1: Write migration**

```sql
-- migrations/0004_status_review.sql
-- +goose Up
CREATE TABLE tasks_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    description     TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN
        ('queued','running','completed','failed','cancelled','needs_review','approved','rejected')),
    branch_name     TEXT,
    summary         TEXT,
    error           TEXT,
    tokens_used     INTEGER NOT NULL DEFAULT 0,
    cost_cents      INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      DATETIME,
    finished_at     DATETIME
);
INSERT INTO tasks_new SELECT * FROM tasks;
DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_created_at ON tasks(created_at DESC);

-- +goose Down
SELECT 1;
```

- [ ] **Step 2: Dry-run + commit**

```bash
TMP=$(mktemp -t era.XXXXXX.db)
goose -dir migrations sqlite3 "$TMP" up | tail -3
sqlite3 "$TMP" "PRAGMA table_info(tasks);" | grep status
rm -f "$TMP"*
go test -race ./...
git add migrations/0004_status_review.sql
git commit -m "feat(db): migration 0004 — extend tasks.status with needs_review/approved/rejected"
```

---

### Task M3-5: Queue integration — diffscan after RunNext

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `internal/queue/queue_run_test.go`
- Modify: `queries/tasks.sql` — add `SetStatus` query
- Regenerate: `internal/db/tasks.sql.go`
- Modify: `internal/db/repo.go` — wrapper for `SetStatus`
- Modify: `cmd/orchestrator/main.go` — pass `*githubcompare.Client` into Queue

`Queue` grows a new field `compareClient` (a `*githubcompare.Client`). After `CompleteTask` succeeds in `RunNext`:

1. Fetch diff via `compareClient.Compare(ctx, repo, "main", branch)`
2. Run `diffscan.ScanDiffs(diffs)`
3. If `len(findings) > 0`: set status to `needs_review`, append event `kind="diffscan_flagged"` with payload listing findings, then notify as usual (notification copy is extended in Phase R). For M3-5, keep the completion DM behavior unchanged — Phase R changes it.
4. If no findings: unchanged (`status=completed`).

- [ ] **Step 1: Add `SetStatus` SQL query**

In `queries/tasks.sql`:

```sql
-- name: SetTaskStatus :exec
UPDATE tasks SET status = ? WHERE id = ?;
```

Run: `sqlc generate`

- [ ] **Step 2: Add Repo wrapper**

```go
// internal/db/repo.go
func (r *Repo) SetStatus(ctx context.Context, id int64, status string) error {
    return r.q.SetTaskStatus(ctx, SetTaskStatusParams{ID: id, Status: status})
}
```

- [ ] **Step 3: Extend Queue**

```go
// Queue gains:
type DiffSource interface {
    Compare(ctx context.Context, repo, base, head string) ([]diffscan.FileDiff, error)
}

type Queue struct {
    // ... existing fields ...
    compare DiffSource
    repoFQN string // owner/repo for compare lookups
}

// Extend New signature:
func New(repo *db.Repo, runner Runner, tokens TokenSource, compare DiffSource, repoFQN string) *Queue {
    return &Queue{repo: repo, runner: runner, tokens: tokens, compare: compare, repoFQN: repoFQN}
}
```

In `RunNext`, after `CompleteTask`:

```go
// Diff-scan the branch if compare client configured.
if q.compare != nil && branch != "" {
    diffs, err := q.compare.Compare(ctx, q.repoFQN, "main", branch)
    if err != nil {
        // Don't fail the task; log as event.
        _ = q.repo.AppendEvent(ctx, t.ID, "diffscan_error", quoteJSON(err.Error()))
    } else {
        findings := diffscan.ScanDiffs(diffs)
        if len(findings) > 0 {
            payload, _ := json.Marshal(findings)
            _ = q.repo.AppendEvent(ctx, t.ID, "diffscan_flagged", string(payload))
            _ = q.repo.SetStatus(ctx, t.ID, "needs_review")
        }
    }
}
```

- [ ] **Step 4: Write queue tests**

Two new tests:

```go
type fakeCompare struct {
    diffs []diffscan.FileDiff
    err   error
}
func (f *fakeCompare) Compare(ctx context.Context, repo, base, head string) ([]diffscan.FileDiff, error) {
    return f.diffs, f.err
}

func TestQueue_RunNext_CleanDiff_StaysCompleted(t *testing.T) {
    ctx := context.Background()
    fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
    fc := &fakeCompare{diffs: []diffscan.FileDiff{{Path: "foo.go", Added: []string{"a"}}}}
    q, repo := newRunQueueWithDeps(t, fr, nil, fc, "a/b")
    id, _ := q.CreateTask(ctx, "x")
    _, err := q.RunNext(ctx)
    require.NoError(t, err)
    task, _ := repo.GetTask(ctx, id)
    require.Equal(t, "completed", task.Status)
}

func TestQueue_RunNext_FlaggedDiff_SetsNeedsReview(t *testing.T) {
    ctx := context.Background()
    fr := &fakeRunner{branch: "agent/1/x", summary: "s"}
    // Diff removes a test.
    fc := &fakeCompare{diffs: []diffscan.FileDiff{
        {Path: "foo_test.go", Removed: []string{"func TestBar(t *testing.T) {}"}},
    }}
    q, repo := newRunQueueWithDeps(t, fr, nil, fc, "a/b")
    id, _ := q.CreateTask(ctx, "x")
    _, err := q.RunNext(ctx)
    require.NoError(t, err)
    task, _ := repo.GetTask(ctx, id)
    require.Equal(t, "needs_review", task.Status)

    events, _ := repo.ListEvents(ctx, id)
    hasFlag := false
    for _, e := range events {
        if e.Kind == "diffscan_flagged" {
            hasFlag = true
        }
    }
    require.True(t, hasFlag, "expected diffscan_flagged event")
}
```

Add `newRunQueueWithDeps(t, runner, tokens, compare, repoFQN)` helper.

- [ ] **Step 5: Update `cmd/orchestrator/main.go`**

Construct `*githubcompare.Client` (sharing the App client as Tokener) and pass to `queue.New`:

```go
compareClient := githubcompare.New("", appClient)
q := queue.New(repo, runner.QueueAdapter{D: docker}, appClient, compareClient, cfg.GitHubSandboxRepo)
```

- [ ] **Step 6: Tests + commit**

```bash
go test -race ./...
git add queries/tasks.sql internal/db/ internal/queue/ cmd/orchestrator/
git commit -m "feat(queue): diff-scan on task completion; flagged tasks → needs_review"
```

---

### Task M3-6: Phase P Regression Gate

- [ ] Full suite `-race -count=1` green
- [ ] All M0/M1/M2 e2e tests pass
- [ ] All prior phase smoke scripts pass
- [ ] Phase P smoke: `scripts/smoke/phase_p_diffscan.sh` — runs diffscan package tests + asserts migration 0004 applies + recreates test DB with new CHECK constraint values
- [ ] Tag `m3-phase-p-diffscan`

**Ship-here checkpoint.** Flagged tasks sit in `needs_review` status; completion DMs are unchanged (Phase R adds the approval DM). You could stop here with a working reward-hacking detector.

---

## Phase Q — Telegram inline keyboard + callback queries

### Task M3-7: Extend `telegram.Client` to send inline keyboards

**Files:**
- Modify: `internal/telegram/client.go` — `SendMessageWithButtons`; Update type extensions
- Modify: `internal/telegram/client_test.go` — FakeClient updates; compile-time Client checks

Add:

```go
type InlineButton struct {
    Text         string
    CallbackData string
}

type Client interface {
    SendMessage(ctx context.Context, chatID int64, text string) error
    SendMessageWithButtons(ctx context.Context, chatID int64, text string, buttons [][]InlineButton) (messageID int, err error)
    EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error
    AnswerCallback(ctx context.Context, callbackID string, text string) error
    Updates(ctx context.Context) (<-chan Update, error)
}

// Update extended with optional callback:
type Update struct {
    UserID   int64
    ChatID   int64
    Text     string       // set when this is a text message
    Callback *CallbackQuery // set when this is a button tap
}

type CallbackQuery struct {
    ID        string   // Telegram callback query ID; used for AnswerCallback
    MessageID int      // message containing the button
    Data      string   // the button's CallbackData
}
```

Implement on `realClient` via `tgbotapi.InlineKeyboardMarkup` and handle the `update.CallbackQuery` branch in the `Updates` goroutine. Update `FakeClient` similarly for tests.

Full TDD steps omitted here for brevity — pattern follows M2-10 style (failing tests, implement, pass). Key tests:

- `TestClient_SendWithButtonsRoundTrip` — fake client records buttons passed
- `TestClient_CallbackQueryPropagatesThroughUpdates` — fake client `IncomingCallbacks <- CallbackQuery{...}` reaches the Update
- `TestClient_AnswerCallbackRecorded`

### Task M3-8: Handler routes callback queries to `Ops.HandleApproval`

**Files:**
- Modify: `internal/telegram/handler.go` — new Ops method; handler dispatches callbacks
- Modify: `internal/telegram/handler_test.go`

`Ops` interface gains:

```go
type Ops interface {
    // existing:
    CreateTask(ctx context.Context, desc string) (int64, error)
    TaskStatus(ctx context.Context, id int64) (string, error)
    ListRecent(ctx context.Context, limit int) ([]TaskSummary, error)
    // new in M3:
    HandleApproval(ctx context.Context, callbackData string) (replyText string, err error)
}
```

`Handler.Handle` checks if `u.Callback != nil` and routes to `ops.HandleApproval(u.Callback.Data)`; on success it calls `client.AnswerCallback(u.Callback.ID, reply)` (shows toast in user's Telegram UI). The callback data format is up to the sender — Phase R defines it as `approve:<task_id>` / `reject:<task_id>`.

### Task M3-9: `/cancel <id>` + `/retry <id>` handler commands

**Files:**
- Modify: `internal/telegram/handler.go`
- Modify: `internal/telegram/handler_test.go`

New commands + Ops methods:

```go
type Ops interface {
    // ... existing ...
    CancelTask(ctx context.Context, id int64) error
    RetryTask(ctx context.Context, id int64) (newID int64, err error)
}
```

`Handler.Handle` routes `/cancel <id>` and `/retry <id>` to these. Reject with a helpful message if the target task is in a state that can't be cancelled/retried (e.g. cancel on a running task — not allowed in M3; covered in M4 with docker kill).

### Task M3-10: Phase Q Regression Gate

- [ ] Full suite green
- [ ] All prior e2e tests pass
- [ ] Phase Q smoke script — unit-tests-only (live callback testing is Phase R)
- [ ] Tag `m3-phase-q-callbacks`

---

## Phase R — Approval flow end-to-end

### Task M3-11: Notifier gains `NotifyNeedsReview` with inline diff preview

**Files:**
- Modify: `internal/queue/queue.go` — `Notifier.NotifyNeedsReview`
- Modify: `cmd/orchestrator/main.go` — `tgNotifier.NotifyNeedsReview` composes message + buttons + diff preview

The DM:
- Header: `task #N: needs review`
- Findings list (from the diffscan event)
- Truncated diff (first ~2000 chars, joined from all flagged files)
- GitHub compare link: `https://github.com/<repo>/compare/main...<branch>`
- Inline buttons: `[Approve] [Reject]` with callback data `approve:<id>` / `reject:<id>`

Message length: Telegram caps at 4096 chars. Truncate diff accordingly.

### Task M3-12: `Queue.ApproveTask` / `Queue.RejectTask`

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `internal/queue/queue_run_test.go`

Transitions:

- `ApproveTask(id)`: `needs_review` → `completed`. No branch change (branch is already pushed). Appends event `kind="approved"`.
- `RejectTask(id)`: `needs_review` → `rejected`. Deletes the branch via GitHub App token (using the sidecar's `/credentials/git`? Or directly using the App client at orchestrator level? Orchestrator can go direct — easier). Appends event `kind="rejected"`.
- Both idempotent: calling `ApproveTask` on an already-approved task is a no-op (returns nil, doesn't double-emit events).

Provide a small `BranchDeleter` interface (injected through Queue) that `RejectTask` calls; real impl uses the App client to hit `DELETE /repos/{owner}/{repo}/git/refs/heads/<branch>`.

Tests cover approve, reject, double-tap idempotency, reject-on-already-completed returns error.

### Task M3-13: `Queue.HandleApproval` dispatches callback data

**Files:**
- Modify: `internal/queue/queue.go`

```go
// HandleApproval parses callback data of form "approve:<id>" or "reject:<id>"
// and calls ApproveTask/RejectTask. Returns a short reply string suitable
// for Telegram's callback-answer toast.
func (q *Queue) HandleApproval(ctx context.Context, data string) (string, error) {
    parts := strings.SplitN(data, ":", 2)
    if len(parts) != 2 {
        return "", fmt.Errorf("bad callback data")
    }
    id, err := strconv.ParseInt(parts[1], 10, 64)
    if err != nil {
        return "", err
    }
    switch parts[0] {
    case "approve":
        if err := q.ApproveTask(ctx, id); err != nil {
            return "", err
        }
        return fmt.Sprintf("task #%d approved", id), nil
    case "reject":
        if err := q.RejectTask(ctx, id); err != nil {
            return "", err
        }
        return fmt.Sprintf("task #%d rejected", id), nil
    default:
        return "", fmt.Errorf("unknown action %q", parts[0])
    }
}
```

Queue now satisfies the extended `telegram.Ops` interface.

### Task M3-14: `BranchDeleter` implementation in orchestrator

**Files:**
- Create: `internal/githubbranch/delete.go` — `DELETE /repos/{owner}/{repo}/git/refs/heads/<branch>` via App token
- Create: `internal/githubbranch/delete_test.go`
- Modify: `cmd/orchestrator/main.go` — construct + inject into Queue

### Task M3-15: Manual smoke — approve + reject paths

Two Telegram flows:
1. Send a task Pi can handle cleanly (e.g. `add HELLO_M3.md with one line 'approvals'`) → completion DM (unchanged)
2. Send a task that triggers diff-scan: `delete the function TestBar from foo_test.go` (requires a scenario where that function exists — use a throwaway file or seed the sandbox). Expect: approval DM with inline diff + buttons. Tap Reject → branch deleted, DM toast confirms.

Document the manual fixture (what you put in the sandbox repo to make diff-scan fire) in `scripts/smoke/phase_r_approvals.sh` as reference.

### Task M3-16: Phase R Regression Gate

- [ ] Full suite green
- [ ] All prior e2e + new approval e2e test green
- [ ] Manual smoke (approve + reject) completed
- [ ] Tag `m3-phase-r-approvals`

---

## Phase S — EOD digest + /cancel + /retry

### Task M3-17: `internal/digest` package — aggregate + render

**Files:**
- Create: `internal/digest/digest.go`
- Create: `internal/digest/digest_test.go`

Pure function:

```go
func Render(tasks []db.Task, from, to time.Time) string
```

Returns a Telegram message summarizing the window:

```
era digest — 2026-04-23 (last 24h)

8 tasks total:
  6 completed · 1 needs_review · 1 failed

Tokens: 42,180 | Cost: $0.087

Branches pushed:
  #1  completed  add HELLO.md  → agent/1/...
  #2  needs_review (review me!) refactor foo.go → agent/2/...
  #3  failed: cap exceeded  (ran 3m 12s)
  ...

Good morning ☕
```

Deterministic: given fixed input + window, output is byte-identical. Tests lock this via golden-file comparison.

### Task M3-18: Cron scheduler in main.go

**Files:**
- Modify: `internal/config/config.go` — `PI_DIGEST_TIME_UTC` (default `17:30` = 11 PM IST)
- Modify: `cmd/orchestrator/main.go` — goroutine that fires at the configured time each day

Simple implementation: on startup, compute next-fire time; `time.Sleep` until then; send digest; loop. If orchestrator restarts between digest and next fire, the previous day's may be missed. Accept — this is a personal tool.

### Task M3-19: `/cancel <id>` — queued tasks only

**Files:**
- Modify: `internal/queue/queue.go` — `CancelTask(id)`
- Modify: `internal/queue/queue_run_test.go`

Only transitions `queued` → `cancelled`. Running tasks rejected with a clear error. Re-running `/cancel` on an already-cancelled task is a no-op.

### Task M3-20: `/retry <id>` — clone task description

**Files:**
- Modify: `internal/queue/queue.go` — `RetryTask(id)` creates new row with same description
- Modify: `internal/queue/queue_run_test.go`

Returns new task ID. The old task's state is preserved (no change). Telegram DM shows the new task was queued.

### Task M3-21: Phase S Regression Gate

- [ ] Full suite green
- [ ] All prior e2e + new digest/cancel/retry tests
- [ ] Manual smoke — set `PI_DIGEST_TIME_UTC` to 2 minutes from now, observe digest arrives
- [ ] Phase S smoke script
- [ ] Tag `m3-phase-s-digest`

### Task M3-22: M3 release

- [ ] README updated to M3 status
- [ ] Full non-e2e + e2e + every phase smoke green
- [ ] Tag `m3-release`
- [ ] Push origin master + tags

---

## Exit criteria for M3

- [ ] `go test -race -count=1 ./...` PASS
- [ ] All E2E tests pass (M0 + M1 success + M1 cap + M2 full-sandbox + new M3 approval-flow e2e)
- [ ] Diffscan catches a known-bad branch (manual smoke; fixture checked-in to sandbox)
- [ ] Approve button path: flagged branch stays on GitHub; task ends at `approved`
- [ ] Reject button path: flagged branch DELETED from GitHub; task ends at `rejected`
- [ ] EOD digest fires at the configured time with correct task counts
- [ ] `/cancel <id>` on a queued task transitions to `cancelled`
- [ ] `/retry <id>` creates a fresh task with same description
- [ ] README reflects M3 status
- [ ] All 4 phase smoke scripts green

---

## What comes next (not this plan)

- M4 would be an open spec. Possibilities: running-task cancellation (docker kill), VPS deployment with systemd, concurrent task execution, GitHub PR creation (via App), approval-gates BEFORE push (Option A from the design convo), agent-self-reporting mid-task. None currently committed.

---

## Closing reminder

Every commit: `go test -race ./...` green.
Every phase boundary: full smoke checklist for every feature ever built.
Anything breaks — stop, fix, then proceed.

We are cautious. We are serious. We are productive. We do not build blindly.
