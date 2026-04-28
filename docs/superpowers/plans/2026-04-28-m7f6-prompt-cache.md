# M7-F.6 — Persona Prompt SQLite Cache (fix for 0G KV flakiness)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use `- [ ]` syntax for tracking.

**Goal:** Make `/task --persona=<name>` reliable by caching persona prompt text in local SQLite, falling back to 0G KV only when SQLite has no entry. Eliminates the M7-F.5 live-gate failure mode (`zg_storage fetch: memory: not found`) caused by 0G KV testnet flakiness.

**Architecture:** Single migration adds `prompt_text TEXT NOT NULL DEFAULT ''` to the `personas` table. `Queue.MintPersona` writes the prompt to both 0G KV (for the on-chain URI) AND SQLite (for fast/reliable retrieval). A new `Queue.fetchPersonaPrompt(ctx, persona)` helper tries SQLite first, falls back to 0G KV via `q.zgStorage.FetchPrompt`. The 0G URI remains the on-chain artifact (judges click, see the prompt was uploaded); SQLite is the demo-reliable hot path.

**Tech Stack:** SQLite migration (goose), modify `personas.go` repo + `queue.go` orchestration. No new packages, no chain interactions, no new env vars.

**Spec:** none (deferred from M7-F.5 live gate; design is deterministic — see M7-B.3 dual-provider precedent).

**Testing philosophy:** Strict TDD. ~3 phases, ~1 hour total.

**Prerequisites:** M7-F done (tag `m7f-done`).

---

## File Structure

```
migrations/0012_personas_prompt_text.sql                  CREATE — ALTER TABLE personas ADD COLUMN prompt_text TEXT NOT NULL DEFAULT ''

internal/persona/persona.go                               MODIFY — add PromptText string field to Persona struct
internal/db/personas.go                                   MODIFY — InsertPersona writes prompt_text; GetPersonaPrompt new method; ListPersonas + GetPersonaByName SELECT remain prompt-text-free (avoid bloating reads)
internal/db/personas_test.go                              MODIFY — extend tests for PromptText round-trip + GetPersonaPrompt

internal/queue/queue.go                                   MODIFY — MintPersona populates row.PromptText; RunNext persona-resolution path uses new fetchPersonaPrompt helper with SQLite-first / 0G-fallback chain
internal/queue/queue_run_test.go                          MODIFY — TestRunNext_PersonaTask_FetchesPromptFromSQLiteWhenZGFails (new); update existing TestRunNext_PersonaTask_PrependsPrompt to populate PromptText
```

No changes to: `era-brain/`, `cmd/orchestrator/`, `internal/telegram/`. The Telegram-facing `Ops.MintPersona` signature is unchanged. The persona registry interface (`PersonaRegistry`) gains one new method (`GetPersonaPrompt`) — internal-only, no external callers affected.

---

## Phase 1: Migration + persona.PromptText + repo writes/reads

**Files:**
- Create: `migrations/0012_personas_prompt_text.sql`
- Modify: `internal/persona/persona.go`
- Modify: `internal/db/personas.go`
- Modify: `internal/db/personas_test.go`

### Step 1.1: Verify next migration number

```bash
ls /Users/vaibhav/Documents/projects/era-multi-persona/era/migrations/ | tail
```

Expected last 2 lines include `0010_personas.sql` and `0011_tasks_persona_name.sql`. Use `0012` for this phase.

### Step 1.2: Write migration

`migrations/0012_personas_prompt_text.sql`:

```sql
-- +goose Up
ALTER TABLE personas ADD COLUMN prompt_text TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1; -- SQLite ≤ 3.34 cannot DROP COLUMN; no-op for hackathon scope.
```

### Step 1.3: Write failing test for PromptText round-trip

Append to `internal/db/personas_test.go`:

```go
func TestPersonas_InsertWithPromptText(t *testing.T) {
	repo := openTest(t)

	p := persona.Persona{
		TokenID:         "5",
		Name:            "with-prompt",
		OwnerAddr:       "0x6DB1508Deeb45E0194d4716349622806672f6Ac2",
		SystemPromptURI: "zg://abc",
		Description:     "test",
		PromptText:      "You only write idiomatic Rust code.",
	}
	require.NoError(t, repo.InsertPersona(context.Background(), p))

	got, err := repo.GetPersonaPrompt(context.Background(), "with-prompt")
	require.NoError(t, err)
	require.Equal(t, "You only write idiomatic Rust code.", got)
}

func TestPersonas_GetPrompt_NotFound(t *testing.T) {
	repo := openTest(t)
	_, err := repo.GetPersonaPrompt(context.Background(), "nope")
	require.ErrorIs(t, err, persona.ErrPersonaNotFound)
}

func TestPersonas_GetPrompt_EmptyPromptIsValid(t *testing.T) {
	// Personas imported via reconcileFromChain have no local prompt cached
	// (only the 0G URI). GetPersonaPrompt returns "" without error in that case.
	repo := openTest(t)
	require.NoError(t, repo.InsertPersona(context.Background(), persona.Persona{
		TokenID:         "6",
		Name:            "imported",
		OwnerAddr:       "0x...",
		SystemPromptURI: "zg://def",
		// PromptText omitted — DEFAULT '' applies
	}))

	got, err := repo.GetPersonaPrompt(context.Background(), "imported")
	require.NoError(t, err)
	require.Equal(t, "", got)
}
```

Add the `persona` import if not already present.

### Step 1.4: Run, verify FAIL

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go test ./internal/db/ -run "TestPersonas_InsertWithPromptText|TestPersonas_GetPrompt" -count=1 -v 2>&1 | head -30
```

Expected: build failure on `undefined: persona.Persona.PromptText`, `undefined: db.Repo.GetPersonaPrompt`. Exit non-zero.

### Step 1.5: Add PromptText field to Persona struct

In `internal/persona/persona.go`, add field to struct:

```go
type Persona struct {
	TokenID         string
	Name            string
	OwnerAddr       string
	SystemPromptURI string
	ENSSubname      string
	Description     string
	PromptText      string    // NEW (M7-F.6) — local cache of the prompt for fast/reliable fetch; fallback chain uses this before 0G KV
	CreatedAt       time.Time
}
```

### Step 1.6: Update Repo.InsertPersona to write prompt_text

In `internal/db/personas.go`, modify `InsertPersona`:

```go
func (r *Repo) InsertPersona(ctx context.Context, p persona.Persona) error {
	_, err := r.q.db.ExecContext(ctx, `
		INSERT INTO personas (token_id, name, owner_addr, system_prompt_uri, ens_subname, description, prompt_text)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.TokenID, p.Name, p.OwnerAddr, p.SystemPromptURI,
		nullableString(p.ENSSubname), nullableString(p.Description),
		p.PromptText)
	if err != nil {
		if isUniqueViolation(err, "personas.name") {
			return persona.ErrPersonaNameTaken
		}
		return fmt.Errorf("insert persona: %w", err)
	}
	return nil
}
```

Add `GetPersonaPrompt` method:

```go
// GetPersonaPrompt returns the locally-cached prompt text for the named
// persona. Returns "" with nil error when the row exists but no prompt was
// cached (e.g., personas imported via reconcileFromChain). Returns
// persona.ErrPersonaNotFound when the row doesn't exist.
func (r *Repo) GetPersonaPrompt(ctx context.Context, name string) (string, error) {
	var prompt string
	err := r.q.db.QueryRowContext(ctx,
		`SELECT prompt_text FROM personas WHERE name = ?`, name).Scan(&prompt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", persona.ErrPersonaNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get persona prompt: %w", err)
	}
	return prompt, nil
}
```

`ListPersonas` and `GetPersonaByName` — leave their SELECT column lists untouched. The new `prompt_text` column is NOT included in those reads (avoids bloating each /personas listing with 4KB×N of unused prompt content).

### Step 1.7: Run, verify PASS

```bash
go test ./internal/db/ -run "TestPersonas" -count=1 -v
```

Expected: 7 tests pass (4 original + 3 new).

### Step 1.8: Run full regression

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

Both green.

### Step 1.9: Commit

```bash
git add migrations/0012_personas_prompt_text.sql internal/persona/persona.go internal/db/personas.go internal/db/personas_test.go
git commit -m "phase(M7-F.6.1): personas.prompt_text column + Persona.PromptText field + Repo.GetPersonaPrompt method"
git tag m7f6-1-prompt-cache-schema
```

NO Co-Authored-By, NO `--author`.

---

## Phase 2: Queue write+read with SQLite-first fallback

**Files:**
- Modify: `internal/queue/queue.go` — `MintPersona` populates `row.PromptText`; new `fetchPersonaPrompt` helper; RunNext persona-resolution uses helper
- Modify: `internal/queue/queue.go` — extend `PersonaRegistry` interface with `GetPersonaPrompt`
- Modify: `internal/queue/queue_run_test.go` — extend `stubPersonas` with `prompts map[string]string`; new failover test

### Step 2.1: Failing test — fetchPersonaPrompt prefers SQLite when 0G fails

Append to `internal/queue/queue_run_test.go`:

```go
func TestRunNext_PersonaTask_FetchesPromptFromSQLiteWhenZGFails(t *testing.T) {
	// Mints a persona with PromptText cached in stubPersonas.
	// stubPromptStorage's FetchPrompt is configured to fail (simulating 0G KV
	// flakiness). Task should still complete because the queue falls back to
	// SQLite via the persona registry's GetPersonaPrompt method.

	fr := &fakeRunner{branch: "agent/1/ok", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	stub := &stubSwarm{planText: "1.", plannerSealed: true, reviewDecision: swarm.DecisionApprove}
	q.SetSwarm(stub)
	q.SetUserID("u")

	personas := &stubPersonas{
		personas: map[string]queue.Persona{
			"rustacean": {
				TokenID:         "4",
				Name:            "rustacean",
				SystemPromptURI: "zg://broken",
				PromptText:      "RUSTACEAN-LOCAL-CACHED-PROMPT",
			},
		},
		prompts: map[string]string{"rustacean": "RUSTACEAN-LOCAL-CACHED-PROMPT"},
	}
	storage := &stubPromptStorage{
		fetchErr: errors.New("zg_storage fetch: memory: not found"), // simulate 0G KV miss
	}
	q.SetPersonas(personas)
	q.SetPromptStorage(storage)

	q.SetINFT(&stubINFT{})

	_, err := repo.CreateTask(context.Background(), "fix auth", "owner/repo", "default", "rustacean")
	require.NoError(t, err)
	processed, err := q.RunNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed)

	// Pi (fakeRunner) should have seen the SQLite-cached prompt, NOT errored on the 0G failure.
	require.Contains(t, fr.lastDesc, "RUSTACEAN-LOCAL-CACHED-PROMPT")
}
```

Update `stubPersonas` (in same file or wherever it's declared) to add a `prompts map[string]string` field + a `GetPersonaPrompt` method:

```go
type stubPersonas struct {
	personas map[string]queue.Persona
	prompts  map[string]string  // NEW (M7-F.6) — SQLite cache equivalent
}

func (s *stubPersonas) GetPersonaPrompt(_ context.Context, name string) (string, error) {
	if v, ok := s.prompts[name]; ok {
		return v, nil
	}
	if _, ok := s.personas[name]; ok {
		return "", nil // row exists, no prompt cached
	}
	return "", queue.ErrPersonaNotFound
}
```

Also add `fetchErr` field to `stubPromptStorage`:
```go
type stubPromptStorage struct {
	prompts  map[string]string
	fetchErr error  // NEW — when set, FetchPrompt returns this error instead
}

func (s *stubPromptStorage) FetchPrompt(_ context.Context, uri string) (string, error) {
	if s.fetchErr != nil {
		return "", s.fetchErr
	}
	if v, ok := s.prompts[uri]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found: %s", uri)
}
```

### Step 2.2: Run, verify FAIL

```bash
go test ./internal/queue/ -run "TestRunNext_PersonaTask_FetchesPromptFromSQLiteWhenZGFails" -count=1 -v 2>&1 | head -30
```

Expected: build failure on `undefined: stubPersonas.GetPersonaPrompt` (since the interface doesn't have it yet) OR test runs and Pi sees no prompt because RunNext uses 0G-only fetch. Exit non-zero.

### Step 2.3: Extend `PersonaRegistry` interface

In `internal/queue/queue.go`:

```go
type PersonaRegistry interface {
	Lookup(ctx context.Context, name string) (Persona, error)
	List(ctx context.Context) ([]Persona, error)
	Insert(ctx context.Context, p Persona) error
	UpdateENSSubname(ctx context.Context, name, subname string) error
	GetPersonaPrompt(ctx context.Context, name string) (string, error)  // NEW (M7-F.6)
}
```

The compile-time assertion `var _ PersonaRegistry = (*db.Repo)(nil)` will catch any impl gap. Add adapter on `Repo` if not already present:

```go
// internal/db/personas.go
func (r *Repo) GetPersonaPrompt(ctx context.Context, name string) (string, error) {
	// already added in Phase 1 — this is the adapter for queue.PersonaRegistry
	// (lowercase signature matches; just confirm the existing method satisfies the interface)
}
```

(The Phase 1 method already matches the interface signature — no extra adapter needed.)

### Step 2.4: Add `fetchPersonaPrompt` helper + use in RunNext

In `internal/queue/queue.go`, add a private helper:

```go
// fetchPersonaPrompt returns the prompt text for a persona. SQLite-first
// (fast, reliable), 0G KV fallback (canonical, sometimes flaky on testnet).
// Returns the first non-empty result; returns the 0G error only if both fail.
func (q *Queue) fetchPersonaPrompt(ctx context.Context, p Persona) (string, error) {
	// Try SQLite first
	if q.personas != nil {
		if cached, err := q.personas.GetPersonaPrompt(ctx, p.Name); err == nil && cached != "" {
			return cached, nil
		}
	}
	// Fall back to 0G KV
	if q.zgStorage == nil {
		return "", fmt.Errorf("no SQLite-cached prompt and 0G storage not wired")
	}
	prompt, err := q.zgStorage.FetchPrompt(ctx, p.SystemPromptURI)
	if err != nil {
		return "", fmt.Errorf("sqlite cache miss + 0G fetch failed: %w", err)
	}
	return prompt, nil
}
```

In RunNext's persona-resolution block, replace the existing `q.zgStorage.FetchPrompt(...)` call with `q.fetchPersonaPrompt(ctx, persona)`:

```go
// Before (M7-F.4):
prompt, ferr := q.zgStorage.FetchPrompt(ctx, persona.SystemPromptURI)

// After (M7-F.6):
prompt, ferr := q.fetchPersonaPrompt(ctx, persona)
```

### Step 2.5: Update `Queue.MintPersona` to populate `row.PromptText`

In `internal/queue/queue.go` `MintPersona`, find the `row := Persona{...}` literal (after the iNFT mint, before `Insert`) and add the new field:

```go
row := Persona{
	TokenID:         persona.TokenID,
	Name:            name,
	OwnerAddr:       persona.OwnerAddr,
	SystemPromptURI: uri,
	Description:     desc,
	PromptText:      prompt,  // NEW (M7-F.6) — SQLite cache for fast/reliable fetch
}
```

### Step 2.6: Run, verify PASS

```bash
go test ./internal/queue/ -run "TestRunNext_PersonaTask" -count=1 -v
```

Expected: all 4 persona tests pass (3 original + 1 new failover test).

### Step 2.7: Update existing test to populate PromptText

Find `TestRunNext_PersonaTask_PrependsPromptAndUsesCustomTokenID` and `TestRunNext_CustomPersona_FreshNamespace_NoError`. They currently set `SystemPromptURI` and rely on `stubPromptStorage.prompts[uri]`. To make them compatible with the SQLite-first fallback (which will now hit `q.personas.GetPersonaPrompt` first), populate the `prompts` map on `stubPersonas` AND keep the `stubPromptStorage.prompts` map for completeness:

```go
personas := &stubPersonas{
	personas: map[string]queue.Persona{
		"rustacean": {TokenID: "3", Name: "rustacean", SystemPromptURI: "stub://x", PromptText: "RUSTACEAN-PROMPT"},
	},
	prompts: map[string]string{"rustacean": "RUSTACEAN-PROMPT"},
}
```

If the test previously asserted that `stubPromptStorage` was queried, that assertion no longer holds (SQLite path wins). Re-run the test and adjust assertions if any break.

### Step 2.8: Full regression

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

Both green.

### Step 2.9: Commit

```bash
git add internal/queue/
git commit -m "phase(M7-F.6.2): SQLite-first prompt fetch — Queue.fetchPersonaPrompt with 0G fallback; MintPersona caches PromptText"
git tag m7f6-2-fallback-fetch
```

---

## Phase 3: Live Telegram gate

**Files:** none modified. Verification only.

The hard part: `rustacean.vaibhav-era.eth` already exists on-chain from M7-F live gate (token #4) but its prompt isn't in SQLite. We need a fresh persona OR a way to backfill.

### Step 3.1: Mint a fresh persona

Don't reuse `rustacean` (already taken in SQLite). Pick a new name.

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go build -o bin/orchestrator ./cmd/orchestrator
ssh era@178.105.44.3 sudo systemctl stop era
ssh era@178.105.44.3 systemctl is-active era   # expect "inactive"
set -a; source .env; set +a
./bin/orchestrator
```

Send via Telegram:
```
/persona-mint pythonic You write clean Pythonic code. Prefer comprehensions over loops, type hints everywhere, dataclasses over dicts, pathlib over os.path. Always include docstrings.
```

Expect successful mint with token #5, ENS subname `pythonic.vaibhav-era.eth`.

### Step 3.2: Use the new persona

```
/task --persona=pythonic add a /healthz endpoint that returns 200 OK
```

Expect:
- Task NOT failed (the SQLite cache hit means the prompt is found regardless of 0G KV state)
- DM `personas:` footer shows `pythonic.vaibhav-era.eth → token #5`
- iNFT contract events on chainscan-galileo show recordInvocation against token #5
- PR opens

### Step 3.3: Restart-resilience test

Stop the orchestrator (Ctrl-C). Restart it (`./bin/orchestrator`). Send another `/task --persona=pythonic ...`. Should still succeed — SQLite survives the restart.

### Step 3.4: Restart VPS

```bash
ssh era@178.105.44.3 sudo systemctl start era
```

Stop local (Ctrl-C). Push tags + commits to origin.

### Step 3.5: Tag M7-F.6 done

```bash
git tag m7f6-done
git push origin master
git push --tags
```

---

## Acceptance criteria (M7-F.6 done)

1. `go build ./...` green; `go test -race -count=1 ./...` green for both modules.
2. Real `/persona-mint` of a fresh name → /task --persona=<name> succeeds even after stopping+restarting orchestrator between mint and use.
3. SQLite `personas.prompt_text` column contains the full prompt for newly-minted personas.
4. /personas listing remains fast — does NOT include prompt_text in the SELECT (avoids per-listing bloat).
5. M7-F's existing functionality unchanged: ENS subnames register, iNFT mints, listings work.

---

## Risks + cuts list

1. **Existing `rustacean` token #4 stays orphaned.** No prompt in SQLite (M7-F.6 only writes for new mints). Acceptable — just don't use it in the demo. M7-F.6 doesn't include backfill from 0G; would require a one-shot CLI tool.
2. **Migration on existing DB** — `ALTER TABLE ... ADD COLUMN` with `DEFAULT ''` is a fast metadata-only operation in SQLite. No risk.
3. **`stubPersonas` test doubles need updating in 2-3 places.** Phase 2.7 catches this; if a test still references the old shape, the build will fail loudly.

---

## Notes for implementer

- The full architectural rationale is in the M7-F live-gate report: 0G KV testnet returned `memory: not found` after a server restart, blocking `/task --persona=`. The fix is to add SQLite as the primary read path, keeping 0G as the canonical (and demonstrable on-chain) artifact.
- Phase 1 + Phase 2 each commit independently; Phase 3 is verification-only.
- Don't widen `Ops.MintPersona` or `/personas` Telegram listing — they don't need the prompt text.
- Don't change `cmd/orchestrator/main.go` — the queue's existing wiring (`q.SetPersonas(repo)` + `q.SetPromptStorage(storageClient)`) already provides what `fetchPersonaPrompt` needs.
- All commits exclude `Co-Authored-By` and `--author` per `~/.claude/CLAUDE.md`.
