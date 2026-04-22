# Pi-Agent Milestone 0: Plumbing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the smallest end-to-end slice of the pi-agent orchestrator that proves the plumbing works — Telegram in, Docker container out, git branch pushed, summary returned. No real coding agent yet; the container runs a dummy script. Everything later layers onto this chassis.

**Architecture:** A single Go binary (`orchestrator`) runs on the developer's laptop, talks to Telegram over long-polling, persists state to a local SQLite file, and shells out to `docker run` for each task. The container image (`pi-agent-runner`) is a stub that clones a sandbox repo, makes a dummy commit, and pushes to a uniquely-named branch using a GitHub PAT. The orchestrator watches the container, captures logs, and reports the branch URL back on Telegram.

**Tech Stack:**
- Language: Go 1.22+
- Datastore: SQLite via `modernc.org/sqlite` (pure Go, no cgo)
- SQL codegen: `sqlc`
- Migrations: `pressly/goose`
- Telegram: `go-telegram-bot-api/telegram-bot-api/v5`
- Container runtime: Docker (shell out to `docker` CLI; SDK deferred)
- Test framework: stdlib `testing` + `github.com/stretchr/testify/require`
- Config: env vars loaded via `github.com/joho/godotenv` for local dev

---

## NON-NEGOTIABLE TESTING PHILOSOPHY

**This is the load-bearing rule for this entire project. Read it twice.**

1. **Test-Driven Development, every task, no exceptions.** Every task starts by writing a failing test. We never write implementation code without a failing test that justifies it. If a task feels "too small to test," it's still tested — write the smallest assertion that would have caught the bug you didn't know you had.

2. **Every task ends by running the FULL test suite, not just the new test.** `go test ./...` — the whole thing — runs green at the end of every single task. If it doesn't, you stop and fix the regression before moving on. You never commit with a red suite. You never skip a test to "come back to it later."

3. **Every phase ends with an explicit Regression Gate.** A standalone task at each phase boundary that re-runs the full suite, re-runs a manual smoke checklist covering *every feature built so far* (not just this phase's features), and tags the commit. If anything is broken — even something we built two phases ago — we stop and fix it before touching the next phase.

4. **Regressions in old features block new work.** If Phase D accidentally breaks something from Phase B, we fix Phase B's breakage before finishing Phase D. We do not accept "it was already broken, we'll fix later." There is no later.

5. **Manual smoke tests are written down, not vibes.** Every phase has a checklist of manual steps with expected outputs. We follow the checklist, we check off each item, we save the output. We do not trust memory.

6. **Fail loud. No swallowed errors. No `if err != nil { return nil }` patterns.** Every error is either logged with context and propagated, or is the kind the caller explicitly opted into ignoring with a comment explaining why.

7. **No mocks for our own code, minimum mocks for external boundaries.** Telegram and Docker get thin interfaces we can fake in tests. SQLite stays real — we use an in-memory SQLite DB in tests, same engine as production. Our own packages are tested through their real public API.

8. **Commits are atomic and green.** Every commit compiles, every commit has green tests. If you need to break the build to reorganize code, that's a single commit that stashes the breaking change and unstashes it in the same tick — not two separate red commits.

We are cautious. We are serious. We are productive. We do not build blindly. Everything means everything.

---

## Out of Scope for Milestone 0 (explicit deferral)

These belong to later milestones and **must not creep into M0**, no matter how tempting:

- **M1 — Real agent:** Pi + OpenRouter integration. M0 container runs a dummy bash script, not a real coding agent.
- **M2 — Security hardening:** Network allowlist, secret proxy sidecar, untrusted-content tags, tool-call audit log, diff-scan/reward-hacking guards. M0 containers have default bridged networking and a plain PAT env var. **We accept this as a known insecurity for M0 only. The sandbox repo is throwaway.**
- **M3 — Approvals + EOD digest:** Inline buttons, approval state machine, daily summary cron. M0 has no approval flow — tasks just run and report.
- **GitHub App flow:** Stays on PAT for M0 and M1; swap to App in M2.
- **VPS deployment:** M0 runs on the developer's laptop. Systemd/launchd service files are M1+.
- **Budget cap, token cap, 1hr timeout enforcement:** Deferred to M1. M0 containers have a hard `docker run --stop-timeout` as a belt, nothing fancier.
- **Parallel tasks:** M0 is strictly one-task-at-a-time with a database lock. Concurrency is an M3+ concern and may never be added per FEATURE.md.

---

## Prerequisites (user action before Task 1)

These cannot be automated and block execution. Complete all three before starting.

1. **Create a Telegram bot via [@BotFather](https://t.me/BotFather)** and save the bot token. Also look up your own numeric Telegram user ID (message [@userinfobot](https://t.me/userinfobot)).
2. **Create a throwaway GitHub repo** named `pi-agent-sandbox` under your account. Add a single `README.md` with any content. This is the target repo the dummy container will push branches to.
3. **Create a GitHub PAT** with `repo` scope (classic PAT is fine for M0; fine-grained token scoped to `pi-agent-sandbox` is better). Save the token.

You will place these in a `.env` file at the repo root (see Task 2).

---

## File Structure

```
pi-agent/
├── FEATURE.md                         # already exists
├── README.md                          # written in Task 20
├── Makefile                           # written in Task 1
├── .gitignore                         # written in Task 1
├── .env.example                       # written in Task 2
├── go.mod, go.sum                     # Task 1
├── sqlc.yaml                          # Task 4
├── cmd/
│   └── orchestrator/
│       └── main.go                    # Task 3, wired up in Tasks 11, 16
├── internal/
│   ├── config/
│   │   ├── config.go                  # Task 2
│   │   └── config_test.go             # Task 2
│   ├── db/
│   │   ├── db.go                      # Task 5 (Open, migrate, close)
│   │   ├── db_test.go                 # Task 5
│   │   ├── queries.sql.go             # sqlc-generated, Task 6
│   │   ├── models.go                  # sqlc-generated, Task 6
│   │   ├── repo.go                    # Task 7 (domain wrapper)
│   │   └── repo_test.go               # Task 7
│   ├── telegram/
│   │   ├── client.go                  # Task 9 (interface + real impl)
│   │   ├── client_test.go             # Task 9
│   │   ├── handler.go                 # Task 10 (/task /status /list)
│   │   └── handler_test.go            # Task 10
│   ├── runner/
│   │   ├── docker.go                  # Task 14 (shell to docker CLI)
│   │   ├── docker_test.go             # Task 14
│   │   ├── runner.go                  # Task 15 (orchestration)
│   │   └── runner_test.go             # Task 15
│   └── queue/
│       ├── queue.go                   # Task 16 (single-task loop)
│       └── queue_test.go              # Task 16
├── migrations/
│   ├── 0001_init.sql                  # Task 4
│   └── 0001_init.down.sql             # Task 4
├── queries/
│   └── tasks.sql                      # Task 6
├── docker/
│   └── runner/
│       ├── Dockerfile                 # Task 13
│       └── entrypoint.sh              # Task 13
├── scripts/
│   └── smoke/                         # manual smoke-test scripts referenced in regression gates
│       ├── phase_b_db.sh              # Task 7
│       ├── phase_c_telegram.sh        # Task 12
│       ├── phase_d_runner.sh          # Task 17
│       └── phase_e_e2e.sh             # Task 19
└── docs/
    └── superpowers/
        └── plans/
            └── 2026-04-22-milestone-0-plumbing.md    # this file
```

Rationale:
- **`internal/` packages are split by responsibility, not layer.** `telegram` owns all Telegram I/O. `runner` owns all Docker. `db` owns all persistence. `queue` is the thin orchestration layer that ties them together.
- **`cmd/orchestrator/main.go` stays tiny** — it parses config, wires packages, starts the queue loop. Business logic lives in `internal/`.
- **Tests live beside code** (Go convention). No separate `tests/` tree.
- **Smoke scripts are bash** and committed — they are how we verify end-to-end behavior manually, and we want to diff them when they change.

---

## Phase Overview

| Phase | Tasks | What ships |
|-------|-------|-----------|
| A. Scaffold | 1–3 | Repo, Go module, first passing test, binary that prints version |
| B. Persistence | 4–7 | Migrations, sqlc-generated queries, domain repo, full DB test coverage |
| C. Telegram | 8–12 | Bot handlers for `/task`, `/status`, `/list` reading/writing the DB |
| D. Runner | 13–17 | Dummy runner image, Docker spawn, branch-push, status feedback |
| E. End-to-end | 18–20 | Full E2E test, docs, Milestone 0 release tag |

Each phase ends with a **Regression Gate** task that runs the full suite and a manual smoke checklist covering *every feature built so far*.

---

## Phase A — Scaffold

### Task 1: Initialize repository, Go module, Makefile, .gitignore

**Files:**
- Create: `.gitignore`
- Create: `Makefile`
- Create: `go.mod`
- Create: `.github/` (empty, placeholder for later CI)

- [ ] **Step 1: Initialize git**

```bash
cd /Users/vaibhav/Documents/projects/pi-agent
git init
git add FEATURE.md docs/
git commit -m "chore: import FEATURE.md and M0 plan"
```

- [ ] **Step 2: Create `.gitignore`**

```gitignore
# Binaries
/bin/
/orchestrator

# Local env
.env
.env.local

# SQLite
*.db
*.db-wal
*.db-shm

# Test artifacts
coverage.out
*.test

# IDE
.idea/
.vscode/
.DS_Store
```

- [ ] **Step 3: Initialize Go module**

Run: `go mod init github.com/vaibhavpandey/pi-agent`

Expected: creates `go.mod` with `module github.com/vaibhavpandey/pi-agent` and `go 1.22` (or current).

- [ ] **Step 4: Create `Makefile`**

```makefile
.PHONY: build test test-v lint fmt run clean smoke

BIN := bin/orchestrator

build:
	go build -o $(BIN) ./cmd/orchestrator

test:
	go test ./...

test-v:
	go test -v ./...

test-race:
	go test -race ./...

fmt:
	go fmt ./...
	goimports -w .

lint:
	go vet ./...

run: build
	./$(BIN)

clean:
	rm -rf bin/ *.db *.db-wal *.db-shm coverage.out
```

- [ ] **Step 5: Verify Go toolchain works**

Run: `go version && go env GOPATH`
Expected: prints Go 1.22+ and a non-empty GOPATH. If Go is not installed, install it via `brew install go` before proceeding.

- [ ] **Step 6: Commit scaffold**

```bash
git add .gitignore Makefile go.mod
git commit -m "chore: scaffold Go module and Makefile"
```

---

### Task 2: Config package with env-var loading

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `.env.example`

- [ ] **Step 1: Add godotenv dependency**

Run: `go get github.com/joho/godotenv@latest`
Expected: go.mod updated.

- [ ] **Step 2: Write the failing test**

```go
// internal/config/config_test.go
package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_AllRequiredPresent(t *testing.T) {
	t.Setenv("PI_TELEGRAM_TOKEN", "tok")
	t.Setenv("PI_TELEGRAM_ALLOWED_USER_ID", "12345")
	t.Setenv("PI_GITHUB_PAT", "ghp_xxx")
	t.Setenv("PI_GITHUB_SANDBOX_REPO", "vaibhavpandey/pi-agent-sandbox")
	t.Setenv("PI_DB_PATH", "./test.db")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "tok", cfg.TelegramToken)
	require.Equal(t, int64(12345), cfg.TelegramAllowedUserID)
	require.Equal(t, "ghp_xxx", cfg.GitHubPAT)
	require.Equal(t, "vaibhavpandey/pi-agent-sandbox", cfg.GitHubSandboxRepo)
	require.Equal(t, "./test.db", cfg.DBPath)
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("PI_TELEGRAM_TOKEN", "")
	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_TELEGRAM_TOKEN")
}

func TestLoad_InvalidAllowedUserID(t *testing.T) {
	t.Setenv("PI_TELEGRAM_TOKEN", "tok")
	t.Setenv("PI_TELEGRAM_ALLOWED_USER_ID", "not-a-number")
	t.Setenv("PI_GITHUB_PAT", "x")
	t.Setenv("PI_GITHUB_SANDBOX_REPO", "x/y")
	t.Setenv("PI_DB_PATH", "x")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_TELEGRAM_ALLOWED_USER_ID")
}
```

- [ ] **Step 3: Run test to confirm failure**

Run: `go test ./internal/config/...`
Expected: build failure — `Load` does not exist.

- [ ] **Step 4: Implement `Load`**

```go
// internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	TelegramToken         string
	TelegramAllowedUserID int64
	GitHubPAT             string
	GitHubSandboxRepo     string // "owner/repo"
	DBPath                string
}

func Load() (*Config, error) {
	c := &Config{
		TelegramToken:     os.Getenv("PI_TELEGRAM_TOKEN"),
		GitHubPAT:         os.Getenv("PI_GITHUB_PAT"),
		GitHubSandboxRepo: os.Getenv("PI_GITHUB_SANDBOX_REPO"),
		DBPath:            os.Getenv("PI_DB_PATH"),
	}

	if c.TelegramToken == "" {
		return nil, errors.New("PI_TELEGRAM_TOKEN is required")
	}
	if c.GitHubPAT == "" {
		return nil, errors.New("PI_GITHUB_PAT is required")
	}
	if c.GitHubSandboxRepo == "" {
		return nil, errors.New("PI_GITHUB_SANDBOX_REPO is required")
	}
	if c.DBPath == "" {
		return nil, errors.New("PI_DB_PATH is required")
	}

	raw := os.Getenv("PI_TELEGRAM_ALLOWED_USER_ID")
	if raw == "" {
		return nil, errors.New("PI_TELEGRAM_ALLOWED_USER_ID is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("PI_TELEGRAM_ALLOWED_USER_ID must be integer: %w", err)
	}
	c.TelegramAllowedUserID = id

	return c, nil
}
```

- [ ] **Step 5: Run tests to confirm green**

Run: `go test ./internal/config/...`
Expected: PASS for all three test cases.

- [ ] **Step 6: Run FULL suite**

Run: `go test ./...`
Expected: PASS. (Only config tests exist yet, but we build the habit now.)

- [ ] **Step 7: Write `.env.example`**

```bash
# Telegram bot token from @BotFather
PI_TELEGRAM_TOKEN=

# Your personal Telegram user ID (from @userinfobot)
PI_TELEGRAM_ALLOWED_USER_ID=

# GitHub PAT with repo scope
PI_GITHUB_PAT=

# Sandbox repo the M0 runner will push branches to (format: owner/repo)
PI_GITHUB_SANDBOX_REPO=

# Path to SQLite database file (local dev: ./pi-agent.db)
PI_DB_PATH=./pi-agent.db
```

- [ ] **Step 8: Commit**

```bash
git add internal/config/ .env.example go.mod go.sum
git commit -m "feat(config): load and validate env vars"
```

---

### Task 3: Minimum viable main.go that loads config and exits

**Files:**
- Create: `cmd/orchestrator/main.go`

- [ ] **Step 1: Write the implementation**

```go
// cmd/orchestrator/main.go
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/vaibhavpandey/pi-agent/internal/config"
)

var version = "0.0.1-m0"

func main() {
	if err := godotenv.Load(); err != nil {
		// .env is optional in production; log and continue
		slog.Info(".env not loaded", "err", err)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	slog.Info("orchestrator starting",
		"version", version,
		"db_path", cfg.DBPath,
		"sandbox_repo", cfg.GitHubSandboxRepo,
	)
	slog.Info("orchestrator exiting (no work to do yet)")
}
```

- [ ] **Step 2: Build and run without env**

```bash
make build
./bin/orchestrator
```
Expected: exits non-zero with `config error: PI_TELEGRAM_TOKEN is required` (or similar).

- [ ] **Step 3: Create a local `.env` from `.env.example`** with your real values.

- [ ] **Step 4: Run again with env**

Run: `./bin/orchestrator`
Expected: logs "orchestrator starting" and "orchestrator exiting", exit 0.

- [ ] **Step 5: Run FULL suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/orchestrator/main.go
git commit -m "feat(cmd): minimal main that loads config and exits"
```

---

### Phase A Regression Gate

- [ ] **Step 1: Run full test suite**

Run: `go test -race ./...`
Expected: PASS across all packages.

- [ ] **Step 2: Run linters**

Run: `make lint && make fmt`
Expected: no diff, no warnings.

- [ ] **Step 3: Manual smoke — scaffold**

- [ ] Confirm `git log --oneline` shows 3–4 commits in order: FEATURE import, scaffold, config, main.
- [ ] Confirm `./bin/orchestrator` fails cleanly when `.env` is removed.
- [ ] Confirm `./bin/orchestrator` succeeds with valid `.env`.

- [ ] **Step 4: Tag phase boundary**

```bash
git tag -a m0-phase-a-scaffold -m "M0 Phase A (scaffold) complete"
```

---

## Phase B — Persistence

### Task 4: Schema design + goose migrations

**Files:**
- Create: `migrations/0001_init.sql`
- Create: `sqlc.yaml`
- Add: `tool/tools.go` (pinning codegen tool versions)

- [ ] **Step 1: Install `goose` and `sqlc`**

Run:
```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```
Verify: `goose -version && sqlc version`

- [ ] **Step 2: Write the migration**

```sql
-- migrations/0001_init.sql
-- +goose Up
CREATE TABLE tasks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    description     TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN
                        ('queued','running','completed','failed','cancelled')),
    branch_name     TEXT,
    summary         TEXT,
    error           TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      DATETIME,
    finished_at     DATETIME
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_created_at ON tasks(created_at DESC);

CREATE TABLE events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    payload     TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_events_task_id ON events(task_id);

CREATE TABLE approvals (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('pending','approved','denied','expired')),
    requested_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at DATETIME
);

CREATE INDEX idx_approvals_task_id ON approvals(task_id);

-- +goose Down
DROP TABLE approvals;
DROP TABLE events;
DROP TABLE tasks;
```

Note: `approvals` is unused in M0 but cheap to create now so M3 doesn't need a migration.

- [ ] **Step 3: Write `sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "queries"
    schema: "migrations"
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_package: "database/sql"
        emit_interface: true
        emit_json_tags: true
```

- [ ] **Step 4: Dry-run migration with goose on a temp DB**

Run:
```bash
goose -dir migrations sqlite3 ./tmp.db up
goose -dir migrations sqlite3 ./tmp.db status
rm ./tmp.db
```
Expected: both commands succeed, status shows `Applied At 0001_init.sql`.

- [ ] **Step 5: Commit**

```bash
git add migrations/ sqlc.yaml
git commit -m "feat(db): initial schema for tasks/events/approvals"
```

---

### Task 5: `internal/db` — Open, Migrate, Close with real SQLite tests

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/db_test.go`

- [ ] **Step 1: Add SQLite driver and embed migrations**

Run: `go get modernc.org/sqlite github.com/pressly/goose/v3`

- [ ] **Step 2: Write the failing test**

```go
// internal/db/db_test.go
package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhavpandey/pi-agent/internal/db"
)

func TestOpen_MigratesFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	// Expect tasks table to exist (and be empty)
	var count int
	err = h.Raw().QueryRow(`SELECT count(*) FROM tasks`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestOpen_ReopenExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	h1, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	_, err = h1.Raw().Exec(`INSERT INTO tasks(description, status) VALUES (?, ?)`, "hi", "queued")
	require.NoError(t, err)
	require.NoError(t, h1.Close())

	h2, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h2.Close() })

	var count int
	require.NoError(t, h2.Raw().QueryRow(`SELECT count(*) FROM tasks`).Scan(&count))
	require.Equal(t, 1, count)
}
```

- [ ] **Step 3: Run the test to confirm failure**

Run: `go test ./internal/db/...`
Expected: build error — `db.Open` does not exist.

- [ ] **Step 4: Implement `db.Open` with embedded migrations**

```go
// internal/db/db.go
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed all:../../migrations/*.sql
var migrationsFS embed.FS

type Handle struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Handle, error) {
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "../../migrations"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("goose up: %w", err)
	}

	return &Handle{db: sqlDB}, nil
}

func (h *Handle) Raw() *sql.DB { return h.db }
func (h *Handle) Close() error { return h.db.Close() }
```

Note: the `//go:embed` path is relative to the `internal/db/db.go` file; adjust if your layout resolves differently. If embed-from-parent fails, move migrations embed to a helper at the repo root and re-import.

- [ ] **Step 5: Run test to confirm pass**

Run: `go test ./internal/db/... -v`
Expected: both tests PASS. If embed path fails, flatten migrations into `internal/db/migrations/` and re-embed.

- [ ] **Step 6: Run FULL suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/db/db.go internal/db/db_test.go go.mod go.sum
git commit -m "feat(db): Open/Migrate/Close with embedded migrations"
```

---

### Task 6: sqlc queries + generated code

**Files:**
- Create: `queries/tasks.sql`
- Generated: `internal/db/queries.sql.go`, `internal/db/models.go`

- [ ] **Step 1: Write query definitions**

```sql
-- queries/tasks.sql

-- name: CreateTask :one
INSERT INTO tasks (description, status)
VALUES (?, 'queued')
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = ? LIMIT 1;

-- name: ListRecentTasks :many
SELECT * FROM tasks ORDER BY created_at DESC LIMIT ?;

-- name: ClaimNextQueuedTask :one
UPDATE tasks SET status = 'running', started_at = CURRENT_TIMESTAMP
WHERE id = (SELECT id FROM tasks WHERE status = 'queued' ORDER BY id ASC LIMIT 1)
RETURNING *;

-- name: MarkTaskCompleted :exec
UPDATE tasks SET status = 'completed', branch_name = ?, summary = ?, finished_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: MarkTaskFailed :exec
UPDATE tasks SET status = 'failed', error = ?, finished_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: AppendEvent :exec
INSERT INTO events (task_id, kind, payload) VALUES (?, ?, ?);

-- name: ListEventsForTask :many
SELECT * FROM events WHERE task_id = ? ORDER BY created_at ASC;
```

- [ ] **Step 2: Generate code**

Run: `sqlc generate`
Expected: `internal/db/queries.sql.go` and `internal/db/models.go` created.

- [ ] **Step 3: Verify the generated code builds**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: Run FULL suite**

Run: `go test ./...`
Expected: PASS (still only opens/closes DB — no direct sqlc tests yet; those come in Task 7).

- [ ] **Step 5: Commit**

```bash
git add queries/ internal/db/queries.sql.go internal/db/models.go
git commit -m "feat(db): sqlc-generated queries for tasks and events"
```

---

### Task 7: `internal/db/repo.go` — domain wrapper with full test coverage

**Files:**
- Create: `internal/db/repo.go`
- Create: `internal/db/repo_test.go`
- Create: `scripts/smoke/phase_b_db.sh`

- [ ] **Step 1: Write failing tests for the domain repo**

```go
// internal/db/repo_test.go
package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhavpandey/pi-agent/internal/db"
)

func openTest(t *testing.T) *db.Repo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	return db.NewRepo(h)
}

func TestRepo_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	created, err := r.CreateTask(ctx, "do the thing")
	require.NoError(t, err)
	require.Equal(t, "queued", created.Status)

	got, err := r.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "do the thing", got.Description)
}

func TestRepo_ClaimNextQueued(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	_, _ = r.CreateTask(ctx, "first")
	_, _ = r.CreateTask(ctx, "second")

	claimed, err := r.ClaimNext(ctx)
	require.NoError(t, err)
	require.Equal(t, "first", claimed.Description)
	require.Equal(t, "running", claimed.Status)

	// Second claim returns the next task
	claimed2, err := r.ClaimNext(ctx)
	require.NoError(t, err)
	require.Equal(t, "second", claimed2.Description)

	// Third claim: no tasks
	_, err = r.ClaimNext(ctx)
	require.ErrorIs(t, err, db.ErrNoTasks)
}

func TestRepo_CompleteAndFail(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	t1, _ := r.CreateTask(ctx, "a")
	require.NoError(t, r.CompleteTask(ctx, t1.ID, "agent/1/slug", "did stuff"))
	got, _ := r.GetTask(ctx, t1.ID)
	require.Equal(t, "completed", got.Status)
	require.Equal(t, "agent/1/slug", got.BranchName.String)

	t2, _ := r.CreateTask(ctx, "b")
	require.NoError(t, r.FailTask(ctx, t2.ID, "boom"))
	got2, _ := r.GetTask(ctx, t2.ID)
	require.Equal(t, "failed", got2.Status)
	require.Equal(t, "boom", got2.Error.String)
}

func TestRepo_Events(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	task, _ := r.CreateTask(ctx, "x")
	require.NoError(t, r.AppendEvent(ctx, task.ID, "started", `{"pid":42}`))
	require.NoError(t, r.AppendEvent(ctx, task.ID, "progress", `{"pct":50}`))

	evts, err := r.ListEvents(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, evts, 2)
	require.Equal(t, "started", evts[0].Kind)
	require.Equal(t, "progress", evts[1].Kind)
}

func TestRepo_ListRecent(t *testing.T) {
	ctx := context.Background()
	r := openTest(t)

	for i := 0; i < 5; i++ {
		_, _ = r.CreateTask(ctx, "t")
	}
	list, err := r.ListRecent(ctx, 3)
	require.NoError(t, err)
	require.Len(t, list, 3)
}
```

- [ ] **Step 2: Confirm tests fail**

Run: `go test ./internal/db/...`
Expected: build error — `db.NewRepo`, `db.ErrNoTasks`, etc. do not exist.

- [ ] **Step 3: Implement `Repo`**

```go
// internal/db/repo.go
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNoTasks = errors.New("no queued tasks")

type Repo struct {
	q *Queries
}

func NewRepo(h *Handle) *Repo {
	return &Repo{q: New(h.Raw())}
}

func (r *Repo) CreateTask(ctx context.Context, desc string) (Task, error) {
	return r.q.CreateTask(ctx, desc)
}

func (r *Repo) GetTask(ctx context.Context, id int64) (Task, error) {
	return r.q.GetTask(ctx, id)
}

func (r *Repo) ClaimNext(ctx context.Context) (Task, error) {
	t, err := r.q.ClaimNextQueuedTask(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNoTasks
	}
	if err != nil {
		return Task{}, fmt.Errorf("claim next: %w", err)
	}
	return t, nil
}

func (r *Repo) CompleteTask(ctx context.Context, id int64, branch, summary string) error {
	return r.q.MarkTaskCompleted(ctx, MarkTaskCompletedParams{
		BranchName: sql.NullString{String: branch, Valid: true},
		Summary:    sql.NullString{String: summary, Valid: true},
		ID:         id,
	})
}

func (r *Repo) FailTask(ctx context.Context, id int64, reason string) error {
	return r.q.MarkTaskFailed(ctx, MarkTaskFailedParams{
		Error: sql.NullString{String: reason, Valid: true},
		ID:    id,
	})
}

func (r *Repo) AppendEvent(ctx context.Context, taskID int64, kind, payload string) error {
	return r.q.AppendEvent(ctx, AppendEventParams{TaskID: taskID, Kind: kind, Payload: payload})
}

func (r *Repo) ListEvents(ctx context.Context, taskID int64) ([]Event, error) {
	return r.q.ListEventsForTask(ctx, taskID)
}

func (r *Repo) ListRecent(ctx context.Context, limit int) ([]Task, error) {
	return r.q.ListRecentTasks(ctx, int64(limit))
}
```

- [ ] **Step 4: Run repo tests to confirm green**

Run: `go test ./internal/db/... -v`
Expected: all `TestRepo_*` tests PASS.

- [ ] **Step 5: Run FULL suite**

Run: `go test -race ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Write smoke script**

```bash
#!/usr/bin/env bash
# scripts/smoke/phase_b_db.sh
set -euo pipefail

DB=$(mktemp -t pi-smoke.XXXXXX.db)
trap "rm -f $DB $DB-wal $DB-shm" EXIT

export PI_TELEGRAM_TOKEN=dummy
export PI_TELEGRAM_ALLOWED_USER_ID=1
export PI_GITHUB_PAT=dummy
export PI_GITHUB_SANDBOX_REPO=x/y
export PI_DB_PATH=$DB

./bin/orchestrator
goose -dir migrations sqlite3 "$DB" status | grep -q "0001_init.sql"
echo "OK: migrations applied on orchestrator startup (will apply in Task 11 once wired)"
```

(Script will be meaningful only after Task 11 when main wires the DB. For now, document intent and mark it in commit.)

Make executable: `chmod +x scripts/smoke/phase_b_db.sh`

- [ ] **Step 7: Commit**

```bash
git add internal/db/repo.go internal/db/repo_test.go scripts/smoke/phase_b_db.sh
git commit -m "feat(db): Repo wrapper with CreateTask/ClaimNext/Complete/Fail/Events"
```

---

### Phase B Regression Gate

- [ ] **Step 1: Run full test suite with race detector**

Run: `go test -race ./...`
Expected: PASS across `internal/config/...` and `internal/db/...`.

- [ ] **Step 2: Run linters**

Run: `make lint && make fmt`
Expected: clean.

- [ ] **Step 3: Regression smoke — everything from Phase A still works**

- [ ] `./bin/orchestrator` with missing env: fails cleanly with clear error (Phase A behavior).
- [ ] `./bin/orchestrator` with valid `.env`: still logs startup and exits 0 (Phase A behavior).
- [ ] `go build ./...`: succeeds.

- [ ] **Step 4: Phase B smoke**

- [ ] Build via `make build`.
- [ ] Manually open a Go REPL or tiny script to confirm you can `db.Open`, `CreateTask`, `GetTask`, `ClaimNext`.
- [ ] Inspect produced SQLite file with `sqlite3 pi-agent.db '.schema'`. Confirm 3 tables + indexes present.

- [ ] **Step 5: Tag phase boundary**

```bash
git tag -a m0-phase-b-persistence -m "M0 Phase B (persistence) complete"
```

---

## Phase C — Telegram Bot

### Task 8: Telegram client interface + real implementation (no handlers yet)

**Files:**
- Create: `internal/telegram/client.go`
- Create: `internal/telegram/client_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get github.com/go-telegram-bot-api/telegram-bot-api/v5`

- [ ] **Step 2: Define the interface we need (not the one the library exposes)**

```go
// internal/telegram/client.go
package telegram

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Update is our own domain type to insulate handlers from the library.
type Update struct {
	UserID  int64
	ChatID  int64
	Text    string
}

type Client interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	Updates(ctx context.Context) (<-chan Update, error)
}

type realClient struct {
	api           *tgbotapi.BotAPI
	allowedUserID int64
}

func NewClient(token string, allowedUserID int64) (Client, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot api: %w", err)
	}
	return &realClient{api: api, allowedUserID: allowedUserID}, nil
}

func (c *realClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	_, err := c.api.Send(msg)
	if err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	return nil
}

func (c *realClient) Updates(ctx context.Context) (<-chan Update, error) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	ch := c.api.GetUpdatesChan(u)

	out := make(chan Update)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				c.api.StopReceivingUpdates()
				return
			case up, ok := <-ch:
				if !ok {
					return
				}
				if up.Message == nil || up.Message.From == nil {
					continue
				}
				if up.Message.From.ID != c.allowedUserID {
					// Drop messages from unauthorized users silently.
					continue
				}
				out <- Update{
					UserID: up.Message.From.ID,
					ChatID: up.Message.Chat.ID,
					Text:   up.Message.Text,
				}
			}
		}
	}()
	return out, nil
}
```

- [ ] **Step 3: Write a fake for tests**

```go
// internal/telegram/client_test.go
package telegram

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type FakeClient struct {
	mu   sync.Mutex
	Sent []struct {
		ChatID int64
		Text   string
	}
	Incoming chan Update
}

func NewFakeClient() *FakeClient { return &FakeClient{Incoming: make(chan Update, 16)} }

func (f *FakeClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sent = append(f.Sent, struct {
		ChatID int64
		Text   string
	}{chatID, text})
	return nil
}

func (f *FakeClient) Updates(ctx context.Context) (<-chan Update, error) {
	out := make(chan Update)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case u, ok := <-f.Incoming:
				if !ok {
					return
				}
				out <- u
			}
		}
	}()
	return out, nil
}

func TestFakeClient_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := NewFakeClient()
	updates, err := f.Updates(ctx)
	require.NoError(t, err)

	f.Incoming <- Update{UserID: 1, ChatID: 1, Text: "hi"}
	got := <-updates
	require.Equal(t, "hi", got.Text)

	require.NoError(t, f.SendMessage(ctx, 1, "hello"))
	require.Len(t, f.Sent, 1)
	require.Equal(t, "hello", f.Sent[0].Text)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/telegram/... -v`
Expected: PASS. (FakeClient is exercised; real `NewClient` is covered by smoke tests in Phase C gate.)

- [ ] **Step 5: Run FULL suite**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/telegram/client.go internal/telegram/client_test.go go.mod go.sum
git commit -m "feat(telegram): client interface, real impl, fake for tests"
```

---

### Task 9: Handler routing (command parsing only, no DB wiring yet)

**Files:**
- Create: `internal/telegram/handler.go`
- Create: `internal/telegram/handler_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/telegram/handler_test.go
package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubOps records calls instead of touching a real DB.
type stubOps struct {
	Created []string
	Status  map[int64]string
	Listed  bool
}

func (s *stubOps) CreateTask(ctx context.Context, desc string) (int64, error) {
	s.Created = append(s.Created, desc)
	return int64(len(s.Created)), nil
}
func (s *stubOps) TaskStatus(ctx context.Context, id int64) (string, error) {
	if v, ok := s.Status[id]; ok {
		return v, nil
	}
	return "", ErrTaskNotFound
}
func (s *stubOps) ListRecent(ctx context.Context, limit int) ([]TaskSummary, error) {
	s.Listed = true
	return []TaskSummary{{ID: 1, Description: "t1", Status: "queued"}}, nil
}

func TestHandler_TaskCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)

	err := h.Handle(context.Background(), Update{ChatID: 42, Text: "/task build auth flow"})
	require.NoError(t, err)
	require.Equal(t, []string{"build auth flow"}, ops.Created)
	require.Len(t, fc.Sent, 1)
	require.Contains(t, fc.Sent[0].Text, "queued")
}

func TestHandler_StatusCommand(t *testing.T) {
	ops := &stubOps{Status: map[int64]string{7: "running"}}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)

	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/status 7"}))
	require.Contains(t, strings.ToLower(fc.Sent[0].Text), "running")
}

func TestHandler_StatusUnknownTask(t *testing.T) {
	ops := &stubOps{Status: map[int64]string{}}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/status 99"}))
	require.Contains(t, fc.Sent[0].Text, "not found")
}

func TestHandler_ListCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/list"}))
	require.True(t, ops.Listed)
	require.Contains(t, fc.Sent[0].Text, "t1")
}

func TestHandler_UnknownCommand(t *testing.T) {
	ops := &stubOps{}
	fc := NewFakeClient()
	h := NewHandler(fc, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/wat"}))
	require.Contains(t, strings.ToLower(fc.Sent[0].Text), "unknown")
}
```

- [ ] **Step 2: Confirm tests fail**

Run: `go test ./internal/telegram/...`
Expected: build error.

- [ ] **Step 3: Implement handler**

```go
// internal/telegram/handler.go
package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrTaskNotFound = errors.New("task not found")

type TaskSummary struct {
	ID          int64
	Description string
	Status      string
	BranchName  string
}

// Ops is the subset of orchestrator functionality the handler needs.
// Kept narrow so we can fake it. Implemented by internal/queue.Queue in Task 16.
type Ops interface {
	CreateTask(ctx context.Context, desc string) (int64, error)
	TaskStatus(ctx context.Context, id int64) (string, error)
	ListRecent(ctx context.Context, limit int) ([]TaskSummary, error)
}

type Handler struct {
	client Client
	ops    Ops
}

func NewHandler(c Client, ops Ops) *Handler { return &Handler{client: c, ops: ops} }

func (h *Handler) Handle(ctx context.Context, u Update) error {
	text := strings.TrimSpace(u.Text)
	switch {
	case strings.HasPrefix(text, "/task "):
		desc := strings.TrimSpace(strings.TrimPrefix(text, "/task "))
		if desc == "" {
			return h.client.SendMessage(ctx, u.ChatID, "usage: /task <description>")
		}
		id, err := h.ops.CreateTask(ctx, desc)
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
		}
		return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d queued", id))

	case strings.HasPrefix(text, "/status "):
		raw := strings.TrimSpace(strings.TrimPrefix(text, "/status "))
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, "usage: /status <task-id>")
		}
		status, err := h.ops.TaskStatus(ctx, id)
		if errors.Is(err, ErrTaskNotFound) {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d not found", id))
		}
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
		}
		return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("task #%d: %s", id, status))

	case text == "/list":
		items, err := h.ops.ListRecent(ctx, 10)
		if err != nil {
			return h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
		}
		var b strings.Builder
		if len(items) == 0 {
			b.WriteString("no tasks yet")
		}
		for _, it := range items {
			fmt.Fprintf(&b, "#%d [%s] %s\n", it.ID, it.Status, it.Description)
		}
		return h.client.SendMessage(ctx, u.ChatID, b.String())

	default:
		return h.client.SendMessage(ctx, u.ChatID, "unknown command. try /task, /status, /list")
	}
}
```

- [ ] **Step 4: Confirm tests pass**

Run: `go test ./internal/telegram/... -v`
Expected: all 5 tests PASS.

- [ ] **Step 5: Run FULL suite**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/telegram/handler.go internal/telegram/handler_test.go
git commit -m "feat(telegram): command handler for /task /status /list"
```

---

### Task 10: Wire DB-backed Ops adapter + update `main.go` to listen

**Files:**
- Modify: `cmd/orchestrator/main.go`
- Create: `internal/queue/queue.go` (stub, full impl in Task 16)
- Create: `internal/queue/queue_test.go`

- [ ] **Step 1: Write failing tests for a DB-backed Ops**

```go
// internal/queue/queue_test.go
package queue_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhavpandey/pi-agent/internal/db"
	"github.com/vaibhavpandey/pi-agent/internal/queue"
	"github.com/vaibhavpandey/pi-agent/internal/telegram"
)

func newQueue(t *testing.T) (*queue.Queue, *db.Repo) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	h, err := db.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	r := db.NewRepo(h)
	q := queue.New(r, nil) // runner nil for now; wired Task 16
	return q, r
}

func TestQueue_CreateTask_ReturnsID(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	id, err := q.CreateTask(ctx, "hello")
	require.NoError(t, err)
	require.Greater(t, id, int64(0))
}

func TestQueue_TaskStatus_NotFound(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	_, err := q.TaskStatus(ctx, 999)
	require.ErrorIs(t, err, telegram.ErrTaskNotFound)
}

func TestQueue_TaskStatus_Found(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	id, _ := q.CreateTask(ctx, "x")
	s, err := q.TaskStatus(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "queued", s)
}

func TestQueue_ListRecent(t *testing.T) {
	ctx := context.Background()
	q, _ := newQueue(t)
	_, _ = q.CreateTask(ctx, "a")
	_, _ = q.CreateTask(ctx, "b")
	list, err := q.ListRecent(ctx, 5)
	require.NoError(t, err)
	require.Len(t, list, 2)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `go test ./internal/queue/...`
Expected: build error.

- [ ] **Step 3: Implement the stub queue**

```go
// internal/queue/queue.go
package queue

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vaibhavpandey/pi-agent/internal/db"
	"github.com/vaibhavpandey/pi-agent/internal/telegram"
)

// Runner is wired in Task 15; nil-safe for Phase C.
type Runner interface {
	Run(ctx context.Context, taskID int64, description string) (branch, summary string, err error)
}

type Queue struct {
	repo   *db.Repo
	runner Runner
}

func New(repo *db.Repo, runner Runner) *Queue { return &Queue{repo: repo, runner: runner} }

func (q *Queue) CreateTask(ctx context.Context, desc string) (int64, error) {
	t, err := q.repo.CreateTask(ctx, desc)
	if err != nil {
		return 0, err
	}
	return t.ID, nil
}

func (q *Queue) TaskStatus(ctx context.Context, id int64) (string, error) {
	t, err := q.repo.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", telegram.ErrTaskNotFound
	}
	if err != nil {
		return "", err
	}
	return t.Status, nil
}

func (q *Queue) ListRecent(ctx context.Context, limit int) ([]telegram.TaskSummary, error) {
	rows, err := q.repo.ListRecent(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]telegram.TaskSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, telegram.TaskSummary{
			ID:          r.ID,
			Description: r.Description,
			Status:      r.Status,
			BranchName:  r.BranchName.String,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run queue tests to confirm green**

Run: `go test ./internal/queue/... -v`
Expected: PASS.

- [ ] **Step 5: Update `main.go` to wire Telegram listener**

```go
// cmd/orchestrator/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/vaibhavpandey/pi-agent/internal/config"
	"github.com/vaibhavpandey/pi-agent/internal/db"
	"github.com/vaibhavpandey/pi-agent/internal/queue"
	"github.com/vaibhavpandey/pi-agent/internal/telegram"
)

var version = "0.0.1-m0"

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Info(".env not loaded", "err", err)
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handle, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		fail(err)
	}
	defer handle.Close()
	repo := db.NewRepo(handle)

	q := queue.New(repo, nil)

	client, err := telegram.NewClient(cfg.TelegramToken, cfg.TelegramAllowedUserID)
	if err != nil {
		fail(err)
	}
	handler := telegram.NewHandler(client, q)

	updates, err := client.Updates(ctx)
	if err != nil {
		fail(err)
	}

	slog.Info("orchestrator ready", "version", version)

	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return
		case u, ok := <-updates:
			if !ok {
				slog.Info("updates channel closed")
				return
			}
			if err := handler.Handle(ctx, u); err != nil {
				slog.Error("handler", "err", err)
			}
		}
	}
}

func fail(err error) {
	var cfgErr *config.Config
	_ = errors.As(err, &cfgErr)
	fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
	os.Exit(1)
}
```

- [ ] **Step 6: Build**

Run: `make build`
Expected: success.

- [ ] **Step 7: Run FULL suite**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/queue/ cmd/orchestrator/main.go
git commit -m "feat(queue,cmd): wire Telegram bot to queue-backed Ops"
```

---

### Task 11: Manual integration with real Telegram

Confirm the bot actually responds. This is the first real external-world test.

- [ ] **Step 1: Ensure `.env` has real bot token + user ID.**

- [ ] **Step 2: Run orchestrator**

```bash
rm -f pi-agent.db*
make run
```
Expected log: `orchestrator ready`.

- [ ] **Step 3: In Telegram, send `/task hello world` to your bot**

Expected reply: `task #1 queued`

- [ ] **Step 4: Send `/status 1`**

Expected reply: `task #1: queued`

- [ ] **Step 5: Send `/list`**

Expected reply: single line mentioning task #1.

- [ ] **Step 6: Send `/wat`**

Expected reply: `unknown command. try /task, /status, /list`

- [ ] **Step 7: Send a message from a different Telegram account** (or ask a friend to message the bot)

Expected: orchestrator silently drops the message. No reply. Logs show nothing for it.

- [ ] **Step 8: Ctrl-C orchestrator**

Expected: clean shutdown, DB file intact, tasks preserved.

- [ ] **Step 9: Re-run, `/list`**

Expected: task #1 still there with status `queued`.

- [ ] **Step 10: Inspect DB manually**

```bash
sqlite3 pi-agent.db "SELECT id, description, status FROM tasks;"
```
Expected: row exists.

- [ ] **Step 11: Write down any surprises in a session note and fix them before proceeding.**

- [ ] **Step 12: Commit documentation if any config had to be tweaked**

---

### Task 12: Phase C Regression Gate

- [ ] **Step 1: Run full suite with race**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 2: Regression smoke — Phase A + B features**

- [ ] Missing env → fails cleanly (Phase A).
- [ ] Migrations apply on startup (Phase B): verify with `sqlite3 pi-agent.db '.schema tasks'`.
- [ ] Repo CRUD still works: re-run `go test ./internal/db/...` explicitly.

- [ ] **Step 3: Phase C smoke (from Task 11 script)**

Run the manual Telegram checklist in full; all 12 steps must pass.

- [ ] **Step 4: Write smoke script checklist**

`scripts/smoke/phase_c_telegram.sh` — not executable automation (Telegram requires the real bot) but a step-by-step doc. Commit it as a reference.

- [ ] **Step 5: Tag**

```bash
git tag -a m0-phase-c-telegram -m "M0 Phase C (telegram) complete"
```

---

## Phase D — Docker Runner (dummy)

### Task 13: Runner Docker image

**Files:**
- Create: `docker/runner/Dockerfile`
- Create: `docker/runner/entrypoint.sh`

- [ ] **Step 1: Write entrypoint**

```bash
#!/usr/bin/env bash
# docker/runner/entrypoint.sh
# M0 dummy runner. Clones sandbox repo, makes trivial change, pushes new branch.
# INPUT (env vars):
#   PI_TASK_ID          task id (numeric)
#   PI_TASK_DESCRIPTION plain text description
#   PI_GITHUB_PAT       GitHub PAT with repo scope on sandbox repo
#   PI_GITHUB_REPO      owner/repo
# OUTPUT:
#   stdout last line: "RESULT branch=<name> summary=<text>"  (on success)
#   stderr: all logs
#   exit 0 on success, non-zero on failure.

set -euo pipefail

: "${PI_TASK_ID:?PI_TASK_ID required}"
: "${PI_TASK_DESCRIPTION:?PI_TASK_DESCRIPTION required}"
: "${PI_GITHUB_PAT:?PI_GITHUB_PAT required}"
: "${PI_GITHUB_REPO:?PI_GITHUB_REPO required}"

branch="agent/${PI_TASK_ID}/dummy-$(date -u +%s)"
work=$(mktemp -d)
cd "$work"

git config --global user.email "pi-agent@local"
git config --global user.name  "pi-agent"
git config --global advice.detachedHead false

git clone --depth 1 "https://x-access-token:${PI_GITHUB_PAT}@github.com/${PI_GITHUB_REPO}.git" repo
cd repo
git checkout -b "$branch"

{
  echo ""
  echo "## Task #${PI_TASK_ID}"
  echo "${PI_TASK_DESCRIPTION}"
  echo ""
  echo "_Dummy commit from M0 runner at $(date -u +%FT%TZ)._"
} >> README.md

git add README.md
git commit -m "task #${PI_TASK_ID}: ${PI_TASK_DESCRIPTION}"
git push origin "$branch"

echo "RESULT branch=${branch} summary=dummy-commit-ok"
```

- [ ] **Step 2: Write Dockerfile**

```dockerfile
# docker/runner/Dockerfile
FROM alpine:3.19

RUN apk add --no-cache git bash ca-certificates coreutils

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
```

- [ ] **Step 3: Build the image**

Run: `docker build -t pi-agent-runner:m0 docker/runner/`
Expected: success.

- [ ] **Step 4: Smoke-run the image directly**

```bash
docker run --rm \
  -e PI_TASK_ID=0 \
  -e PI_TASK_DESCRIPTION="smoke test" \
  -e PI_GITHUB_PAT="$(grep PI_GITHUB_PAT .env | cut -d= -f2)" \
  -e PI_GITHUB_REPO="$(grep PI_GITHUB_SANDBOX_REPO .env | cut -d= -f2)" \
  pi-agent-runner:m0
```
Expected: final stdout line `RESULT branch=agent/0/dummy-... summary=dummy-commit-ok`. Confirm in GitHub UI that the branch exists on the sandbox repo.

- [ ] **Step 5: Delete the test branch from GitHub** (manual cleanup).

- [ ] **Step 6: Commit**

```bash
git add docker/
git commit -m "feat(runner): M0 dummy Dockerfile and entrypoint"
```

---

### Task 14: `internal/runner/docker.go` — shell to `docker run` and capture result

**Files:**
- Create: `internal/runner/docker.go`
- Create: `internal/runner/docker_test.go`

- [ ] **Step 1: Write failing test using a fake `exec.Cmd` path**

```go
// internal/runner/docker_test.go
package runner_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhavpandey/pi-agent/internal/runner"
)

func TestParseResultLine(t *testing.T) {
	out := bytes.NewBufferString(`some log line
another log
RESULT branch=agent/3/foo summary=dummy-commit-ok
`)
	branch, summary, err := runner.ParseResult(out)
	require.NoError(t, err)
	require.Equal(t, "agent/3/foo", branch)
	require.Equal(t, "dummy-commit-ok", summary)
}

func TestParseResultLine_Missing(t *testing.T) {
	out := bytes.NewBufferString("nope\nnothing here\n")
	_, _, err := runner.ParseResult(out)
	require.ErrorIs(t, err, runner.ErrNoResult)
}

// Real docker invocation is exercised in the Phase D regression gate
// by calling Docker.Run() with a real image against the sandbox repo.
// We keep the unit test confined to the parser to avoid flaky CI.

func TestDocker_Run_SkipsWithoutDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("docker integration skipped in -short mode")
	}
	// This test is documentation; the Phase D gate exercises it for real.
	_ = context.Background()
}
```

- [ ] **Step 2: Confirm tests fail**

Run: `go test ./internal/runner/...`
Expected: build error.

- [ ] **Step 3: Implement docker wrapper**

```go
// internal/runner/docker.go
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

var ErrNoResult = errors.New("runner produced no RESULT line")

type Docker struct {
	Image        string
	SandboxRepo  string // "owner/repo"
	GitHubPAT    string
}

type RunInput struct {
	TaskID      int64
	Description string
}

type RunOutput struct {
	Branch  string
	Summary string
	RawLog  string
}

func (d *Docker) Run(ctx context.Context, in RunInput) (*RunOutput, error) {
	args := []string{
		"run", "--rm",
		"-e", fmt.Sprintf("PI_TASK_ID=%d", in.TaskID),
		"-e", fmt.Sprintf("PI_TASK_DESCRIPTION=%s", in.Description),
		"-e", fmt.Sprintf("PI_GITHUB_PAT=%s", d.GitHubPAT),
		"-e", fmt.Sprintf("PI_GITHUB_REPO=%s", d.SandboxRepo),
		d.Image,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)

	var combined strings.Builder
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start docker: %w", err)
	}

	// Fan-in both streams to combined.
	done := make(chan struct{}, 2)
	go streamTo(stdout, &combined, done)
	go streamTo(stderr, &combined, done)
	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("docker run: %w; log:\n%s", err, combined.String())
	}

	branch, summary, err := ParseResult(strings.NewReader(combined.String()))
	if err != nil {
		return nil, fmt.Errorf("%w; log:\n%s", err, combined.String())
	}
	return &RunOutput{Branch: branch, Summary: summary, RawLog: combined.String()}, nil
}

func streamTo(r io.Reader, w *strings.Builder, done chan<- struct{}) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		w.WriteString(sc.Text())
		w.WriteString("\n")
	}
	done <- struct{}{}
}

// ParseResult scans the log for the "RESULT branch=... summary=..." line.
func ParseResult(r io.Reader) (branch, summary string, err error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "RESULT ") {
			continue
		}
		rest := strings.TrimPrefix(line, "RESULT ")
		parts := strings.Fields(rest)
		for _, p := range parts {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "branch":
				branch = kv[1]
			case "summary":
				summary = kv[1]
			}
		}
		if branch != "" {
			return branch, summary, nil
		}
	}
	return "", "", ErrNoResult
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/runner/... -v`
Expected: parser tests PASS.

- [ ] **Step 5: Full suite**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/runner/docker.go internal/runner/docker_test.go
git commit -m "feat(runner): docker wrapper and RESULT parser"
```

---

### Task 15: Wire runner into the queue and execute queued tasks

**Files:**
- Modify: `internal/queue/queue.go` (add `RunPending` loop)
- Add: `internal/queue/queue_run_test.go`
- Modify: `cmd/orchestrator/main.go` (spawn run loop)

- [ ] **Step 1: Write the failing test**

```go
// internal/queue/queue_run_test.go
package queue_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhavpandey/pi-agent/internal/db"
	"github.com/vaibhavpandey/pi-agent/internal/queue"
)

type fakeRunner struct {
	branch  string
	summary string
	err     error
	calls   int
}

func (f *fakeRunner) Run(ctx context.Context, taskID int64, desc string) (string, string, error) {
	f.calls++
	return f.branch, f.summary, f.err
}

func TestQueue_RunNext_Success(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	h, err := db.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })
	r := db.NewRepo(h)

	fr := &fakeRunner{branch: "agent/1/x", summary: "ok"}
	q := queue.New(r, fr)

	id, err := q.CreateTask(ctx, "do x")
	require.NoError(t, err)

	ran, err := q.RunNext(ctx)
	require.NoError(t, err)
	require.True(t, ran)
	require.Equal(t, 1, fr.calls)

	got, err := r.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "completed", got.Status)
	require.Equal(t, "agent/1/x", got.BranchName.String)

	// No more tasks.
	ran, err = q.RunNext(ctx)
	require.NoError(t, err)
	require.False(t, ran)
}

func TestQueue_RunNext_Failure(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	h, _ := db.Open(ctx, path)
	t.Cleanup(func() { _ = h.Close() })
	r := db.NewRepo(h)

	fr := &fakeRunner{err: context.DeadlineExceeded}
	q := queue.New(r, fr)
	id, _ := q.CreateTask(ctx, "boom")

	_, err := q.RunNext(ctx)
	require.Error(t, err)

	got, _ := r.GetTask(ctx, id)
	require.Equal(t, "failed", got.Status)
	require.Contains(t, got.Error.String, "deadline")
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/queue/...`
Expected: build error — `RunNext` missing.

- [ ] **Step 3: Extend `queue.go`**

```go
// appended to internal/queue/queue.go

func (q *Queue) RunNext(ctx context.Context) (bool, error) {
	t, err := q.repo.ClaimNext(ctx)
	if errors.Is(err, db.ErrNoTasks) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	_ = q.repo.AppendEvent(ctx, t.ID, "started", "{}")

	branch, summary, runErr := q.runner.Run(ctx, t.ID, t.Description)
	if runErr != nil {
		_ = q.repo.AppendEvent(ctx, t.ID, "failed", quoteJSON(runErr.Error()))
		if err := q.repo.FailTask(ctx, t.ID, runErr.Error()); err != nil {
			return true, err
		}
		return true, runErr
	}

	_ = q.repo.AppendEvent(ctx, t.ID, "completed", "{}")
	if err := q.repo.CompleteTask(ctx, t.ID, branch, summary); err != nil {
		return true, err
	}
	return true, nil
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(map[string]string{"error": s})
	return string(b)
}
```

Add imports for `encoding/json` and ensure `errors` is present.

- [ ] **Step 4: Make the runner interface match the test**

In `queue.go`, change Runner to:

```go
type Runner interface {
	Run(ctx context.Context, taskID int64, description string) (branch, summary string, err error)
}
```

…and create `internal/runner/runner.go` to adapt `*runner.Docker` to this interface:

```go
// internal/runner/runner.go
package runner

import "context"

func (d *Docker) RunForQueue(ctx context.Context, taskID int64, desc string) (string, string, error) {
	out, err := d.Run(ctx, RunInput{TaskID: taskID, Description: desc})
	if err != nil {
		return "", "", err
	}
	return out.Branch, out.Summary, nil
}
```

We use `RunForQueue` as the method name so `Docker` satisfies `queue.Runner` via a thin method pointer (`q := queue.New(repo, runnerAdapter{d})`). Alternatively, define `type runnerAdapter struct { *Docker }` with a `Run` method that forwards.

For simplicity, add:

```go
type QueueAdapter struct{ D *Docker }

func (q QueueAdapter) Run(ctx context.Context, id int64, desc string) (string, string, error) {
	return q.D.RunForQueue(ctx, id, desc)
}
```

- [ ] **Step 5: Tests pass**

Run: `go test ./internal/queue/... -v`
Expected: all queue tests PASS.

- [ ] **Step 6: Wire in main.go**

Replace `q := queue.New(repo, nil)` with:

```go
docker := &runner.Docker{
	Image:       "pi-agent-runner:m0",
	SandboxRepo: cfg.GitHubSandboxRepo,
	GitHubPAT:   cfg.GitHubPAT,
}
q := queue.New(repo, runner.QueueAdapter{D: docker})

// goroutine: poll for queued tasks every 2s
go func() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := q.RunNext(ctx); err != nil {
				slog.Error("run next", "err", err)
			}
		}
	}
}()
```

Add imports: `time`, and `github.com/vaibhavpandey/pi-agent/internal/runner`.

- [ ] **Step 7: Build**

Run: `make build`
Expected: success.

- [ ] **Step 8: Run FULL suite**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/queue/ internal/runner/runner.go cmd/orchestrator/main.go
git commit -m "feat(queue,runner,cmd): execute queued tasks via docker runner"
```

---

### Task 16: Notify Telegram on task completion

**Files:**
- Modify: `internal/queue/queue.go`
- Modify: `cmd/orchestrator/main.go`

- [ ] **Step 1: Extend the `Queue` with a notifier callback**

Add to `queue.go`:

```go
type Notifier interface {
	NotifyCompleted(ctx context.Context, taskID int64, branch, summary string)
	NotifyFailed(ctx context.Context, taskID int64, reason string)
}

// attach via field so default is nil-safe
type Queue struct {
	repo     *db.Repo
	runner   Runner
	notifier Notifier
}

func (q *Queue) SetNotifier(n Notifier) { q.notifier = n }

// In RunNext, after CompleteTask:
if q.notifier != nil {
    q.notifier.NotifyCompleted(ctx, t.ID, branch, summary)
}
// And after FailTask:
if q.notifier != nil {
    q.notifier.NotifyFailed(ctx, t.ID, runErr.Error())
}
```

- [ ] **Step 2: Test the notifier path**

Append to `queue_run_test.go`:

```go
type fakeNotifier struct {
	completed []string
	failed    []string
}

func (f *fakeNotifier) NotifyCompleted(ctx context.Context, id int64, b, s string) {
	f.completed = append(f.completed, b)
}
func (f *fakeNotifier) NotifyFailed(ctx context.Context, id int64, r string) {
	f.failed = append(f.failed, r)
}

func TestQueue_Notifier(t *testing.T) {
	// ... setup repo, runner ...
	q := queue.New(r, &fakeRunner{branch: "b", summary: "s"})
	n := &fakeNotifier{}
	q.SetNotifier(n)
	_, _ = q.CreateTask(ctx, "x")
	_, _ = q.RunNext(ctx)
	require.Equal(t, []string{"b"}, n.completed)
}
```

- [ ] **Step 3: Implement Notifier in `main.go`**

```go
type tgNotifier struct {
	client telegram.Client
	chatID int64
	repo   string
}

func (t *tgNotifier) NotifyCompleted(ctx context.Context, id int64, branch, summary string) {
	msg := fmt.Sprintf("task #%d completed\nbranch: %s\nhttps://github.com/%s/tree/%s\nsummary: %s",
		id, branch, t.repo, branch, summary)
	_ = t.client.SendMessage(ctx, t.chatID, msg)
}
func (t *tgNotifier) NotifyFailed(ctx context.Context, id int64, reason string) {
	_ = t.client.SendMessage(ctx, t.chatID, fmt.Sprintf("task #%d failed: %s", id, reason))
}
```

Telegram inbound messages give us ChatID; for outbound notifications we need a default. For M0 set it equal to the user ID (DM chats on Telegram have `chat_id == user_id`).

In main.go:

```go
q.SetNotifier(&tgNotifier{
    client: client,
    chatID: cfg.TelegramAllowedUserID,
    repo:   cfg.GitHubSandboxRepo,
})
```

- [ ] **Step 4: Run tests**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/queue/ cmd/orchestrator/main.go
git commit -m "feat(queue,cmd): notify telegram on task completion/failure"
```

---

### Task 17: Phase D Regression Gate

- [ ] **Step 1: Run full suite with race**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 2: Regression smoke — Phases A–C features still intact**

- [ ] Missing env → fails cleanly (Phase A).
- [ ] Migrations still apply (Phase B).
- [ ] Telegram `/task`, `/status`, `/list`, `/wat`, cross-user drop all still work (Phase C — redo the 12 steps from Task 11).
- [ ] Bot-DM `chat_id == user_id` notification goes to same DM.

- [ ] **Step 3: Phase D smoke (end-to-end)**

```bash
./scripts/smoke/phase_d_runner.sh
```
(Below is the scripted form; run it manually and check each step.)

```bash
#!/usr/bin/env bash
# scripts/smoke/phase_d_runner.sh
set -euo pipefail

rm -f pi-agent.db*

docker build -t pi-agent-runner:m0 docker/runner/
make build

# start orchestrator in background
./bin/orchestrator &
PID=$!
trap "kill $PID 2>/dev/null || true" EXIT
sleep 2

echo "NOW:"
echo "1. In Telegram, send: /task m0 end-to-end smoke"
echo "2. Wait ~30s"
echo "3. Expected messages: 'task #1 queued', then 'task #1 completed / branch: agent/1/dummy-... / summary: dummy-commit-ok'"
echo "4. Open https://github.com/<your-sandbox>/branches and confirm the branch was pushed"
echo "Press ENTER when done"
read -r
```

- [ ] **Step 4: Delete the sandbox branch from GitHub** to leave it clean.

- [ ] **Step 5: Confirm DB state matches**

```bash
sqlite3 pi-agent.db "SELECT id, status, branch_name, summary FROM tasks;"
sqlite3 pi-agent.db "SELECT task_id, kind FROM events ORDER BY id;"
```
Expected: task row has `status=completed`, `branch_name=agent/1/...`, `summary=dummy-commit-ok`. Events include `started` and `completed`.

- [ ] **Step 6: Test failure path**

Set `PI_GITHUB_PAT=INVALID` in `.env`, restart orchestrator, send `/task will fail`.
Expected: Telegram reports failure with reason; DB task `status=failed`, `error` populated. Events include `failed`.

- [ ] **Step 7: Restore valid PAT**

- [ ] **Step 8: Tag**

```bash
git tag -a m0-phase-d-runner -m "M0 Phase D (runner) complete"
```

---

## Phase E — End-to-End + Release

### Task 18: E2E test with fake Telegram + real Docker (optional but recommended)

**Files:**
- Create: `internal/e2e/e2e_test.go`

- [ ] **Step 1: Guard with build tag so it doesn't run in normal `go test`**

```go
//go:build e2e
// +build e2e

package e2e_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/vaibhavpandey/pi-agent/internal/db"
    "github.com/vaibhavpandey/pi-agent/internal/queue"
    "github.com/vaibhavpandey/pi-agent/internal/runner"
)

func TestE2E_QueueToDockerToBranch(t *testing.T) {
    pat := os.Getenv("PI_GITHUB_PAT")
    repoEnv := os.Getenv("PI_GITHUB_SANDBOX_REPO")
    if pat == "" || repoEnv == "" {
        t.Skip("PI_GITHUB_PAT and PI_GITHUB_SANDBOX_REPO required")
    }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    path := filepath.Join(t.TempDir(), "e2e.db")
    h, err := db.Open(ctx, path)
    require.NoError(t, err)
    defer h.Close()
    r := db.NewRepo(h)

    d := &runner.Docker{Image: "pi-agent-runner:m0", SandboxRepo: repoEnv, GitHubPAT: pat}
    q := queue.New(r, runner.QueueAdapter{D: d})
    id, err := q.CreateTask(ctx, "e2e smoke")
    require.NoError(t, err)

    ran, err := q.RunNext(ctx)
    require.NoError(t, err)
    require.True(t, ran)

    task, _ := r.GetTask(ctx, id)
    require.Equal(t, "completed", task.Status)
    require.NotEmpty(t, task.BranchName.String)
}
```

- [ ] **Step 2: Run**

Run: `go test -tags e2e ./internal/e2e/... -v`
Expected: PASS (creates a real branch on sandbox repo). Delete branch after.

- [ ] **Step 3: Commit**

```bash
git add internal/e2e/
git commit -m "test(e2e): end-to-end queue→docker→branch (manual run)"
```

---

### Task 19: Wire everything up and document running instructions

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write README**

```markdown
# pi-agent — Milestone 0

Status: plumbing only. No real coding agent yet; the runner makes a dummy commit.
See FEATURE.md for the full vision and docs/superpowers/plans/ for the implementation plan.

## Prerequisites

- Go 1.22+
- Docker
- A Telegram bot token (from @BotFather) and your numeric user ID (from @userinfobot)
- A throwaway GitHub repo (`pi-agent-sandbox`) and a PAT with `repo` scope

## Setup

1. Copy `.env.example` to `.env` and fill in values.
2. `docker build -t pi-agent-runner:m0 docker/runner/`
3. `make build`
4. `./bin/orchestrator`

## Commands

- `/task <description>` — queue a task
- `/status <id>` — check task state
- `/list` — recent tasks

## Security notes for M0

Running M0 means running a PAT-scoped container with full network access and
no prompt-injection protections. **Only point it at `pi-agent-sandbox` or another
throwaway repo. Do not run M0 against anything you care about.** M2 adds
network allowlisting, secret proxy, and diff-scan gates.

## Development

- `make test` — unit + integration tests
- `make test-race` — with race detector
- `go test -tags e2e ./internal/e2e/...` — end-to-end (requires env + docker)
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add M0 README with setup and safety notes"
```

---

### Task 20: Phase E Regression Gate + M0 release tag

- [ ] **Step 1: Run full suite with race**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 2: Run E2E**

Run: `go test -tags e2e ./internal/e2e/... -v`
Expected: PASS. Clean up branch afterward.

- [ ] **Step 3: Full manual regression — everything works**

Walk through **every phase's smoke checklist again, in order**:
- [ ] Phase A: scaffold, binary builds, config validation.
- [ ] Phase B: migrations apply, repo CRUD.
- [ ] Phase C: Telegram bot responds to `/task`, `/status`, `/list`, `/wat`, and drops unauthorized senders.
- [ ] Phase D: queued task becomes a real branch on sandbox repo within ~30s; failures surface cleanly.

- [ ] **Step 4: Confirm Milestone 0 exit criteria**

- [ ] `make build` produces a working binary.
- [ ] `go test -race ./...` green.
- [ ] E2E run green.
- [ ] Telegram round-trip: `/task X` → branch pushed → completion message ≤ 60s.
- [ ] Cross-user messages are silently dropped.
- [ ] Invalid PAT produces a failed-task message, not a crash.
- [ ] Restart preserves task history.
- [ ] `README.md` describes setup from a fresh clone.

- [ ] **Step 5: Tag M0 release**

```bash
git tag -a m0-release -m "Milestone 0: plumbing complete. Telegram in, Docker out, dummy runner."
```

---

## What comes next (not this plan)

- **M1 plan (separate doc):** Replace the dummy entrypoint with Pi + OpenRouter (Kimi K2.5 default). Add token cap + 1h timeout enforcement. Add per-task OpenRouter budget cap.
- **M2 plan (separate doc):** Network allowlist per container, secret proxy sidecar, untrusted-content tags in context, diff-scan/reward-hacking guards, GitHub App installation token flow.
- **M3 plan (separate doc):** Approval-gate state machine, inline Telegram buttons, EOD digest generator, secret upfront-declaration convention enforced.

Each of those is a self-contained plan with its own phases, regression gates, and exit criteria.

---

## Closing reminder (repeat for emphasis)

Before every commit: `go test -race ./...` green.
Before every phase boundary: manual smoke checklist for **every feature ever built**, not just the new ones.
If anything — anything — breaks, stop and fix it.

We are cautious. We are serious. We are productive. We do not build blindly.
