# M7-B.2 — Orchestrator 0G Storage Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire era's orchestrator to use `dual.New(sqlite_cache, zg_composite_primary)` instead of bare SQLite when 0G testnet env vars are present. Real `/task` writes its 2 audit-log receipts (planner + reviewer) to BOTH 0G testnet AND local SQLite. Falls back gracefully to sqlite-only when 0G env vars are missing — M7-A.5 behavior is preserved as the default.

**Architecture:** Single-file change. `cmd/orchestrator/main.go` already constructs a sqlite memory provider for the swarm; we wrap it with `dual` when env vars are present. Zero changes to `internal/queue`, `internal/swarm`, runner, GitHub App, or anything else. The composite + dual + LiveOps pieces are already shipped from M7-B.1.

**What's NOT in scope (deferred to M7-B.3):**
- **Persona KV reads.** `LLMPersona.Run` only writes audit log; reading prior persona memory before the LLM call is a real feature gap that M7-B.3 closes. Without it, the audit-log story works but the "evolving memory" criterion doesn't fully land. M7-B.3 brainstorm + plan happens after M7-B.2 ships.

**Tech Stack:** Go 1.25. era-brain SDK (already wired). 0G testnet (already proven via M7-B.1 live gates). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md` §3 — orchestrator side of 0G Storage integration.

**Testing philosophy:** Strict TDD where applicable. The wiring change in main.go is mostly config glue (no easily unit-testable surface area at the cmd level — main.go has no go test coverage today). The real gate is the **live Telegram /task** at the end. We rely on the unit-tested components (sqlite, zg_kv, zg_log, dual, composite) to behave correctly in production.

**Prerequisites (check before starting):**
- M7-B.1 complete (tag `m7b1-done`).
- `.env` populated with `PI_ZG_PRIVATE_KEY`, `PI_ZG_EVM_RPC`, `PI_ZG_INDEXER_RPC`. `PI_ZG_KV_NODE` is optional.
- Wallet has testnet ZG (faucet covers per-task gas — about 2 transactions per /task).
- Hetzner VPS M6 era is currently running and serving the bot. We'll stop it briefly for the live gate.

---

## File Structure

```
cmd/orchestrator/main.go        MODIFY (Phase 1) — env-var loading, conditional dual construction
```

Single file. No new files. No package-level changes.

---

## Phase 1: Wire dual provider in main.go

**Files:**
- Modify: `cmd/orchestrator/main.go` — around lines 107-122 (the existing era-brain swarm wiring block)

The existing block (M7-A.5 / M7-B.1.4 baseline):
```go
plannerModel := envOrDefault("PI_BRAIN_PLANNER_MODEL", "openai/gpt-4o-mini")
reviewerModel := envOrDefault("PI_BRAIN_REVIEWER_MODEL", "openai/gpt-4o-mini")
plannerLLM := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: plannerModel})
reviewerLLM := openrouter.New(openrouter.Config{APIKey: cfg.OpenRouterAPIKey, DefaultModel: reviewerModel})
brainDBPath := filepath.Join(filepath.Dir(cfg.DBPath), "era-brain.db")
brainMem, err := brainsqlite.Open(brainDBPath)
if err != nil {
    fail(fmt.Errorf("era-brain sqlite: %w", err))
}
defer brainMem.Close()
sw := swarm.New(swarm.Config{
    PlannerLLM:  plannerLLM,
    ReviewerLLM: reviewerLLM,
    Memory:      brainMem,
})
```

Becomes (after this phase):
```go
// ... openrouter setup unchanged ...
// ... brainMem (sqlite) setup unchanged — sqlite stays as the cache layer ...

// Build the memory provider passed to swarm. Default = sqlite alone (M7-A.5 behavior).
// If 0G testnet env vars are present, wrap sqlite with the dual provider so audit
// log writes land on BOTH 0G AND SQLite.
var memProv memory.Provider = brainMem
if zgEnabled() {
    live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
        PrivateKey: os.Getenv("PI_ZG_PRIVATE_KEY"),
        EVMRPCURL:  os.Getenv("PI_ZG_EVM_RPC"),
        IndexerURL: os.Getenv("PI_ZG_INDEXER_RPC"),
        KVNodeURL:  os.Getenv("PI_ZG_KV_NODE"), // optional
    })
    if err != nil {
        fail(fmt.Errorf("0G live ops: %w", err))
    }
    defer live.Close()
    primary := &zgComposite{
        kvP:  zg_kv.NewWithOps(live),
        logP: zg_log.NewWithOps(live),
    }
    memProv = dual.New(brainMem, primary, func(op string, err error) {
        slog.Warn("0G primary write failed", "op", op, "err", err)
    })
    slog.Info("0G storage wired", "indexer", os.Getenv("PI_ZG_INDEXER_RPC"),
        "kv_node_set", os.Getenv("PI_ZG_KV_NODE") != "")
}

sw := swarm.New(swarm.Config{
    PlannerLLM:  plannerLLM,
    ReviewerLLM: reviewerLLM,
    Memory:      memProv, // was: brainMem
})
```

Plus a small `zgEnabled()` helper and `zgComposite` type at the bottom of main.go.

### Step 1.1: Read main.go to identify exact insertion points

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
grep -n "brainMem\|swarm.New\|brainDBPath" cmd/orchestrator/main.go
```

Expected: hits at the era-brain swarm wiring block (around line 107-122).

### Step 1.2: Add the new imports

In `cmd/orchestrator/main.go`'s import block, add:

```go
"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
"github.com/vaibhav0806/era-multi-persona/era-brain/memory/dual"
"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
```

`os` is likely already imported. `slog` is too (era uses structured logging).

### Step 1.3: Add the `zgEnabled` helper + `zgComposite` type at the bottom of main.go

Pick a location after the existing `tgNotifier` impl and `envOrDefault`. Match the file's existing style.

```go
// zgEnabled returns true when all required 0G testnet env vars are present.
// PI_ZG_KV_NODE is optional — its absence just means reads return ErrNotFound,
// which dual.Provider correctly falls through to the SQLite cache.
func zgEnabled() bool {
    return os.Getenv("PI_ZG_PRIVATE_KEY") != "" &&
        os.Getenv("PI_ZG_EVM_RPC") != "" &&
        os.Getenv("PI_ZG_INDEXER_RPC") != ""
}

// zgComposite combines zg_kv (KV ops) and zg_log (Log ops) into a single
// memory.Provider, used as the Primary in the dual provider. Both sub-providers
// share the same underlying *zg_kv.LiveOps so we open the SDK clients once.
type zgComposite struct {
    kvP  memory.Provider
    logP memory.Provider
}

func (c *zgComposite) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
    return c.kvP.GetKV(ctx, ns, key)
}
func (c *zgComposite) PutKV(ctx context.Context, ns, key string, val []byte) error {
    return c.kvP.PutKV(ctx, ns, key, val)
}
func (c *zgComposite) AppendLog(ctx context.Context, ns string, entry []byte) error {
    return c.logP.AppendLog(ctx, ns, entry)
}
func (c *zgComposite) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
    return c.logP.ReadLog(ctx, ns)
}
```

### Step 1.4: Modify the era-brain swarm wiring block

Replace the existing block (around lines 113-122) per the "Becomes" template above. Specifically:

1. Keep the existing `brainMem, err := brainsqlite.Open(brainDBPath)` and its `defer brainMem.Close()`.
2. After `defer brainMem.Close()`, add the `var memProv memory.Provider = brainMem` declaration and the `if zgEnabled() { ... }` block.
3. Change the `Memory: brainMem` line in the `swarm.New(swarm.Config{...})` literal to `Memory: memProv`.

### Step 1.5: Build, verify compile

```bash
go build ./...
```

Expected: exit 0.

If you see "imported and not used" errors, the new imports might be incorrect — check that `memory`, `dual`, `zg_kv`, `zg_log`, `context`, `os`, `slog` are all imported (and only what's needed).

### Step 1.6: Run all tests

```bash
go vet ./...
go test -race -count=1 ./...
```

Expected: green. No new tests added in this phase (cmd/orchestrator has no test surface for this wiring).

### Step 1.7: Smoke test — sqlite-only path (env vars NOT set)

Without sourcing `.env`, run the orchestrator briefly:

```bash
unset PI_ZG_PRIVATE_KEY PI_ZG_EVM_RPC PI_ZG_INDEXER_RPC PI_ZG_KV_NODE
# Source the rest of .env manually OR use a separate .env.test for this:
export PI_TELEGRAM_BOT_TOKEN=...  # whatever existing era M6 needed
# (only do this step if you have a way to run without all env vars; otherwise skip
# the smoke and rely on Phase 2's live gate.)
```

If the orchestrator starts without 0G wiring messages in stdout, the fallback path works. If it crashes on missing PI_ZG_*, that's a regression — fix the `zgEnabled` guard.

This step is OPTIONAL — if you can't easily run a partial-env orchestrator, skip and rely on Phase 2's live gate (which exercises both paths).

### Step 1.8: Commit

```bash
git add cmd/orchestrator/main.go
git commit -m "phase(M7-B.2.1): orchestrator wires dual(sqlite, 0G) memory provider when PI_ZG_* env vars are set"
git tag m7b2-1-wired
```

---

## Phase 2: Live gate — real /task with 0G writes

**Files:** none modified. This is the integration test for the milestone.

### Step 2.1: Build the orchestrator binary

```bash
go build -o bin/orchestrator ./cmd/orchestrator
```

Expected: 15-20MB binary at `bin/orchestrator`.

### Step 2.2: Stop the VPS-hosted M6 era to free the Telegram bot lock

In another terminal (or via the VPS):

```bash
ssh era@178.105.44.3 sudo systemctl stop era
```

Wait ~3 seconds. The remote M6 instance stops polling the bot.

### Step 2.3: Start local orchestrator with full env

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
./bin/orchestrator
```

**Expected boot log lines:**
- migrations OK (existing era migrations only)
- `INFO github app token source configured ...`
- `INFO orchestrator ready ...`
- (NEW) `INFO 0G storage wired indexer=https://indexer-storage-testnet-turbo.0g.ai kv_node_set=true|false`
  - The `kv_node_set=true|false` reflects whether you have PI_ZG_KV_NODE in env.
- (NEW) `INFO Selecting nodes ...` from the SDK during LiveOps construction.
- `INFO digest scheduled ...`

If you don't see "0G storage wired", the env-var detection failed — `echo $PI_ZG_PRIVATE_KEY` to verify .env sourced correctly.

### Step 2.4: Send a /task via Telegram

From your phone:

```
/task add a /healthz endpoint that returns 200 OK with body "ok"
```

(or your usual M7-A.5 form on the sandbox repo).

### Step 2.5: Watch the orchestrator stdout

Expected event sequence:
1. Task queued + claimed
2. **`INFO Set tx params ...`** + **`INFO Transaction receipt ...`** (NEW — planner audit log write to 0G)
3. `planner_ok` event (existing M7-A.5 event)
4. Pi container spawns + tool-loop progress (existing)
5. `pr_opened` event (existing)
6. **`INFO Set tx params ...`** + **`INFO Transaction receipt ...`** (NEW — reviewer audit log write to 0G)
7. `reviewer_ok` event (existing)
8. `completed` event
9. Telegram completion DM (or needs-review DM if reviewer flagged)

**Capture the 2 tx hashes** from steps 2 and 6 — these are the proof points for the milestone.

### Step 2.6: Verify era-brain.db has the same 2 receipts (cache mirror)

```bash
sqlite3 ./era-brain.db "SELECT seq, namespace, length(val) FROM entries WHERE is_kv = 0 ORDER BY seq DESC LIMIT 4"
```

Expected: 2 rows under the same `audit/<task_id>` namespace, written in step 2 and step 6's flow.

(Why 2 not 3: only planner + reviewer go through `LLMPersona.Run` which writes the audit log. The synthesized coder receipt in `synthCoderReceipt()` is constructed in queue.go for DM purposes only — it doesn't traverse a memory provider.)

### Step 2.7: Verify the Telegram DM looks the same as M7-A.5

The DM shape should be unchanged from M7-A.5 — branch link, PR URL, summary, planner one-line, reviewer decision. M7-B.2 didn't touch DM rendering. If the DM shows persona breakdown, the existing M7-A.5 path still works.

### Step 2.8: Restart VPS M6 to bring it back online

```bash
ssh era@178.105.44.3 sudo systemctl start era
```

**This is important.** The local orchestrator is using the same bot token as VPS-era M6. Leaving local running indefinitely + VPS off = production bot offline. After verifying the live gate, ALWAYS restart the VPS service.

(After M7-F we'll have a separate hackathon-bot deployment story; for now M6-VPS is your day-to-day bot.)

### Step 2.9: Stop the local orchestrator

Ctrl-C the local orchestrator after VPS comes back up.

### Step 2.10: Replay all era + era-brain tests one more time

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race ./...
```

Both green.

### Step 2.11: Tag M7-B.2 done

```bash
git tag m7b2-done
```

(no commit needed — Phase 2 is verification only)

---

## Live gate summary (M7-B.2 acceptance)

When this milestone is done:

1. `go build ./...` from repo root succeeds.
2. `go test -race ./...` from repo root green; no regression to era M6 / M7-A.5 / M7-B.1.
3. Real `/task` on a real repo:
   - Orchestrator startup logs show `0G storage wired ...`.
   - Two 0G testnet transactions visible in stdout during the task — one for planner audit log, one for reviewer audit log. Both produce real tx hashes.
   - `era-brain.db` has 2 rows in `audit/<task_id>` namespace (cache mirror confirmed).
   - Telegram DM unchanged from M7-A.5.
4. Without `PI_ZG_*` env vars, the orchestrator falls back to sqlite-only with no errors and no 0G log lines (M7-A.5 baseline preserved).
5. VPS M6 era is restarted after the live gate so the production bot stays online.

---

## Out of scope (deferred to M7-B.3 and beyond)

- **Persona KV reads.** `LLMPersona.Run` only writes the audit log right now — it does not read prior persona memory before the LLM call. M7-B.3 brainstorm + plan addresses this: defines the per-persona observation shape, wires the read step into LLMPersona, and exposes the "evolving memory" feature that 0G Track 2's prize criteria asks for verbatim.
- **`/restore` command.** Re-hydrate `era-brain.db` from 0G after a redeploy / DB loss. Requires a working KV node URL; defer until 0G publishes a stable one.
- **Per-task gas budget caps.** Right now every task burns whatever gas 0G charges (~0.001 ZG × 2 writes). At faucet replenish rates this is fine for hackathon use. Production-grade rate limiting deferred.
- **VPS deployment of M7-B.2.** CI deploy to Hetzner is blocked on missing `DEPLOY_SSH_KEY` secret on the new repo (flagged at M7-A.5 push). M7-B.2 lives on local + master; production hackathon demo deployment is a separate decision in M7-F.
- **Sealed inference receipts on 0G Compute.** Current LLM calls go through OpenRouter; receipts have `Sealed=false`. M7-C wires 0G Compute and flips the flag.
- **iNFT recordInvocation per receipt.** M7-D.

---

## Risks + cuts list

1. **Live gate fails because 0G testnet has a hiccup mid-task.** Recovery: the dual provider's primary error handler logs but doesn't fail the task — Pi still commits + opens PR. Re-run the task once testnet recovers; the cache mirror is already in SQLite.
2. **Faucet runs dry mid-development.** Recovery: 0G faucet replenishes daily; testnet ZG is plentiful. If hit, stop running live tasks until refilled.
3. **VPS M6 stays stopped if you forget Step 2.8.** Recovery: re-run `ssh era@178.105.44.3 sudo systemctl start era`. Production bot is back within 5 seconds.
4. **The "0G storage wired" log line obscures a bad path.** If LiveOps construction succeeds but the very first `Set` call fails (e.g. faucet drained, indexer transient down), the orchestrator runs but every task hits `[zg primary append_log failed: ...]` warnings. Recovery: stop, fix env or wait, restart.
