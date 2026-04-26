# M7-B.3 — Evolving Persona Memory Design Spec

> Status: brainstormed and approved 2026-04-26. Implementation plan to follow.
> Companion vision doc: top-level `FEATURE.md`. Parent design: `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md`.

## §1 — Vision & scope

M7-B.3 lands the **"evolving persistent memory"** criterion verbatim from 0G Track 2. Each persona reads its own prior observations from the dual memory provider (SQLite cache + 0G primary) **before** the LLM call, prepends them to the user prompt, then writes a fresh observation **after** the LLM call. Memory accumulates across tasks per `(persona, user)` key.

This is the substantive feature work that closes a Track 2 prize anchor. Without it, era's submission has the audit-log story but not the memory story.

Scope splits between:
- **`era-brain` SDK** — read+write mechanics inside `LLMPersona.Run`, default `BareHistoryShaper`, JSON blob format.
- **`era` app** — per-persona shaper functions (`plannerShaper`, `reviewerShaper`) tailored to coding-agent semantics.

Failure mode: **run blind + warn** (Q3). Prompt injection: **prepend to user prompt** (Q4). Per-persona content: **shaper-driven** (Q2 option C).

## §2 — Architecture

Three layers:

**Layer 1 — `era-brain.brain.LLMPersona` (SDK).**

New `LLMPersonaConfig` fields:
- `MemoryShaper MemoryShaper` — optional func `(in Input, out Output) string` returning a single observation. Nil = no memory accumulation; LLMPersona behaves as M7-A.5.
- `MemoryNamespace string` — KV namespace for the persona's blob. Required when MemoryShaper set.
- `MaxObservations int` — rolling buffer cap. Defaults to 10.

Modified `LLMPersona.Run` flow:
1. If `MemoryShaper != nil && Memory != nil && in.UserID != ""`: read prior blob via `Memory.GetKV(ctx, MemoryNamespace, in.UserID)`. Tolerate `ErrNotFound` (cold start) silently; log non-NotFound errors via `slog.Warn` and run blind.
2. Render observations as `## Prior observations\n- entry1\n- entry2\n\n` and prepend to `buildUserPrompt(in)` output.
3. Call LLM (existing).
4. Build receipt + append to audit log (existing).
5. If shaper set + LLM call succeeded: `obs := MemoryShaper(in, out)`; if non-empty, decode prior blob, append, trim to `MaxObservations`, JSON-encode, `Memory.PutKV(...)`. Errors warn-only.

New default helper `BareHistoryShaper(maxChars int) MemoryShaper`:
- Returns `out.Text` truncated to `maxChars` (default 200) on each turn.
- Used by SDK example agents (M7-F's audit-agent / chat-agent) so they get memory for free.

**Layer 2 — `era/internal/swarm` (era app).**

New `internal/swarm/shapers.go` with two functions:

- **`plannerShaper`** — records `task: "<desc>" | plan: "<first 3 plan lines>"`. Outcome (approve/flag) isn't known at planner-write-time (reviewer hasn't run yet); planner records what it produced.
- **`reviewerShaper`** — records `task: "<desc>" | decision: "<approve|flag>" | <first critique line>`. Reviewer sees full context post-Pi-run.
- **Coder shaper deferred** — coder is Pi-in-Docker, not LLMPersona. Bridging Pi → era-brain memory is M7-B.3.5+ work; explicit out-of-scope here.

`swarm.New` wires shapers + namespaces into the planner and reviewer LLMPersonaConfig:
- planner: `MemoryNamespace="planner-mem"`, `MemoryShaper=plannerShaper`
- reviewer: `MemoryNamespace="reviewer-mem"`, `MemoryShaper=reviewerShaper`

**Layer 3 — `era/cmd/orchestrator/main.go` + `internal/queue`.**

`queue.Queue` gains a `userID string` field with a `SetUserID(string)` setter. `RunNext` passes `userID` as `swarm.PlanArgs.UserID` and `swarm.ReviewArgs.UserID`. main.go derives `userID` from `strconv.FormatInt(cfg.TelegramAllowedUserID, 10)` (single-user; PI_TELEGRAM_ALLOWED_USER_ID is the only authoritative ID era already trusts). Calls `q.SetUserID(userID)` at startup before the run loop begins.

## §3 — Components (detail)

### era-brain.brain.LLMPersona — modified

Files modified:
- `era-brain/brain/persona.go` — extend LLMPersonaConfig, modify Run flow.
- `era-brain/brain/persona_test.go` — new tests for read+write paths.

File created:
- `era-brain/brain/memory_shaper.go` — MemoryShaper type + BareHistoryShaper helper.

Memory blob JSON format (versioned for forward compat):

```json
{
  "v": 1,
  "observations": [
    "task: \"add /healthz endpoint\" | decision: approve | the diff cleanly adds the route and a passing test",
    "task: \"add JWT auth\" | decision: flag | the diff removed an existing test for token refresh"
  ]
}
```

`v: 1` for forward compat (M7-D might extend with on-chain references). Implementations ignore unknown fields.

Hard limits:
- Per-observation char cap: 200 (enforced by individual shapers; SDK doesn't truncate; shaper returns the final string).
- `MaxObservations`: default 10.
- Total memory injection budget: ~2500 chars (10 × 250 with format overhead) ≈ 625 tokens. Fits comfortably in any 128k-token model.

### era/internal/swarm — modified

Files modified:
- `internal/swarm/swarm.go` — `swarm.New` wires shapers + namespaces.
- `internal/swarm/swarm_test.go` — add coverage for shaper invocation through Plan/Review.

File created:
- `internal/swarm/shapers.go` — plannerShaper + reviewerShaper + small helpers (truncate, firstNLines).

### era/internal/queue + cmd/orchestrator — modified

Files modified:
- `internal/queue/queue.go` — Queue.userID field + SetUserID setter; RunNext threads UserID into PlanArgs/ReviewArgs.
- `internal/queue/queue_run_test.go` — fakeSwarm tests assert UserID propagation.
- `cmd/orchestrator/main.go` — derive userID from cfg, call `q.SetUserID(userID)` before the run loop.

## §4 — Error handling, testing, security

### Error handling

- **KV read fails (non-NotFound)** → `slog.Warn("persona memory read failed", err=...)` → run blind (empty observations block). Receipt still written.
- **KV read returns ErrNotFound** → cold start; no warn, no observations block.
- **KV read returns malformed JSON** → warn, run blind, blob overwritten on next successful write (self-healing).
- **KV write fails** → warn, task continues. Memory just doesn't accumulate this turn. dual.Provider's cache mirror catches up next read.
- **MemoryShaper returns ""** → skip write. Useful for shapers that detect "nothing worth remembering" (e.g. error-output runs).
- **LLM call fails** → existing early-return; no shaper invocation; memory unchanged.

### Testing

**Unit (era-brain):**
- `LLMPersona.Run` with MemoryShaper + spy memory provider. Assert sequence: GetKV → prompt contains observations block → LLM called with correct UserPrompt → PutKV called with updated blob.
- NotFound on first run → empty observations block in prompt, no warn fired.
- Real error on read → warn fired, run blind.
- Malformed JSON in stored blob → warn fired, run blind, blob overwritten on write.
- `BareHistoryShaper(200)` returns truncated `out.Text`. Returns "" when `out.Text` empty.
- Trim to MaxObservations cap when buffer would exceed.
- MemoryShaper=nil → no GetKV/PutKV calls (M7-A.5 behavior preserved).

**Unit (era):**
- `plannerShaper` produces expected `task: "..." | plan: "..."` strings on representative inputs.
- `reviewerShaper` produces expected `task: "..." | decision: "..." | <first line>` strings.
- swarm.Plan/Review pass UserID through to underlying LLMPersona.Run.

**Integration (era-brain):** build-tagged `zg_live` test exercises full read-write-read-back-confirm cycle against testnet.

**Live gate:** real Telegram `/task A` then `/task B`. Inspect `era-brain.db`: planner-mem/<userID> and reviewer-mem/<userID> blobs accumulate. Inspect orchestrator stdout: 2nd task's planner prompt contains observation from task A.

### Security

- **Memory blob content** is task descriptions + LLM-shaped observations. Same risk surface as the existing audit log. No new secrets exposed.
- **DoS via unbounded growth** prevented by `MaxObservations × per-obs-char-cap` hard limits.
- **Cross-persona memory leakage** prevented by sha256-hashed namespaces — `planner-mem` and `reviewer-mem` map to non-overlapping streams.
- **Prompt injection via observations** is a real risk — prior LLM outputs could contain malicious prompt fragments that contaminate future runs. Single-user blast radius (era's Telegram allow-list); not blocking M7-B.3. Mitigation candidate for M7-F polish: wrap observations in `<untrusted>...</untrusted>` tags following M2 sidecar's existing pattern.

## §5 — Milestones (phases)

Per project philosophy: ~5 phases, each with TDD cycle + commit + tag.

| Phase | Tag | What |
|---|---|---|
| B.3.1 | `m7b3-1-read` | New `memory_shaper.go`. Extend LLMPersonaConfig (MemoryShaper, MemoryNamespace, MaxObservations). Modify Run to read+prepend observations block. NotFound + error paths. Failing test → impl → green. |
| B.3.2 | `m7b3-2-write` | Run writes updated blob after LLM call. Append-and-trim. Skip-on-empty-shaper-output. JSON encode/decode helpers. |
| B.3.3 | `m7b3-3-shapers` | `internal/swarm/shapers.go`: plannerShaper + reviewerShaper. swarm.New wires them. Unit tests on shaper outputs. |
| B.3.4 | `m7b3-4-userid` | Queue.userID + SetUserID. RunNext threads through PlanArgs/ReviewArgs.UserID. main.go derives from cfg. Cascade on existing queue tests. |
| B.3.5 | `m7b3-done` | Live gate: 2 sequential `/task` calls. Verify blob accumulation in era-brain.db. Verify 2nd planner prompt sees task A's observation. |

**Estimated effort:** ~3-4 hours subagent-driven work. Smaller than M7-B.2 — no new SDK package, just extending LLMPersona + adding shapers.

## §6 — Decisions log (Q&A from brainstorming)

| Q | Choice | Rationale |
|---|---|---|
| Q1 — Where do read+write live | A: both inside LLMPersona | Track 1 evaluation looks at SDK directly; memory is a SDK feature, not era-app-specific. |
| Q2 — Per-persona observation content | C: SDK ships default `BareHistoryShaper`; era passes per-persona shapers | Generic SDK story (audit-agent/chat-agent get memory free) + tailored era observations. |
| Q3 — KV read failure | B: run blind + slog.Warn | Matches dual.Provider's existing fault model; M7-A.5 ran without memory just fine. |
| Q4 — Prompt injection placement | B: prepend to user prompt | `buildUserPrompt` already prepends structured sections; system prompt stays stable; XML tags overkill. |

## §7 — Out of scope (deferred)

- **Coder persona memory.** Coder is Pi-in-Docker, not LLMPersona. Bridging Pi RESULT json → era-brain memory is a separate effort (M7-B.3.5+).
- **Cross-persona observation linking.** No "outcome" field on planner observations referencing reviewer's decision (would require deferred-write or two-pass writes; complexity not justified).
- **Memory inspection commands.** No `/memory <persona>` Telegram command. Inspect via sqlite directly for now.
- **Per-task memory pruning / time decay.** Hard cap by count (`MaxObservations`); no recency-based eviction. Last-N-wins is the only policy.
- **Untrusted-tag wrapping.** Prompt-injection-via-observation defense deferred to M7-F polish.
- **Multi-user namespacing.** era is single-user. SDK's `Input.UserID` field already supports multi-tenant; era passes the allowlisted user ID.

## §8 — Cuts list (in order if slipping)

1. Live integration test against testnet in B.3.1 (write+read round-trip via SDK) — defer to B.3.5's live gate only.
2. Snapshot tests of shaper output format — replace with inline string-equals assertions.
3. Reviewer shaper — ship only planner shaper if time-pressed (planner observation is the more visible feature).

---
