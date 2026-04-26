# M7-C — 0G Compute Sealed Inference Design Spec

> Status: brainstormed and approved 2026-04-26. Implementation plans to follow.
> Companion vision doc: top-level `FEATURE.md`. Parent design: `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md`.

## §1 — Vision & scope

M7-C lands **0G Compute sealed inference** as an `LLMProvider` impl in era-brain. Both planner and reviewer LLM calls go through a fallback wrapper: try `zg_compute` (TEE-attested testnet model `qwen-2.5-7b-instruct`) first, fall back to OpenRouter on any error. Receipts produced by zg_compute set `Sealed=true` whenever the response carried a TEE signature header. Reviewer's prompt extends to surface planner+coder Sealed flags ("lite" cross-check) so the reviewer can be more cautious with unsealed outputs.

Coder remains Pi-in-Docker (unsealed). Bridging Pi → 0G Compute is deferred — out of scope for this milestone.

Same architectural rhythm as M7-B: SDK side ships first (M7-C.1), era integration wires it (M7-C.2).

## §2 — Architecture

Two milestones, three layers.

### Layer 1 — `era-brain` SDK (M7-C.1)

**`era-brain/llm/zg_compute`** — new package wrapping a stateless HTTP client.

- `Config{BearerToken, ProviderEndpoint, DefaultModel, HTTPTimeout}`. Auth is bearer-only (`Authorization: Bearer app-sk-<...>`); no Web3 signing per call.
- `Complete(ctx, req)` POSTs `{ProviderEndpoint}/chat/completions` with OpenAI-shaped request body.
- Response parsing: extract `choices[0].message.content` + model name, plus the TEE signature header. **Use `ZG-Res-Key` as the placeholder header name during C.1.1 implementation; the C.1.0 smoke script confirms the actual name from a real testnet response.** If the actual name differs, update `zg_compute.Provider` before tagging C.1.1. If the header is present and non-empty → `Sealed=true`; else `Sealed=false`.
- We do NOT cryptographically verify the TEE signature — verification needs TS tooling we don't have a Go equivalent for. Sealed=true means "header was present"; honest scope limitation documented in package doc.
- Tests via `httptest.Server` with fake bearer + fake TEE header. Build-tagged `zg_live` test against real testnet.

**`era-brain/llm/fallback`** — new package wrapping two `llm.Provider` impls.

- `New(primary, secondary llm.Provider, onFallback func(err error)) *Provider`.
- `Complete` tries primary; on error → calls `onFallback` → tries secondary. Returns whichever succeeds. Caller can inspect `Response.Sealed` to learn which path ran.
- Tests cover primary-success, primary-fail-secondary-success, both-fail. Pure Go; no external deps.

### Layer 2 — era-brain examples (M7-C.1)

`examples/coding-agent/main.go` extended w/ `--zg-compute` flag. When set, constructs `fallback.New(zgCompute, openrouter)` per persona. `examples/coding-agent/README.md` documents the one-time `0g-compute-cli` setup steps (deposit ZG → transfer to provider → generate bearer).

### Layer 3 — era app integration (M7-C.2)

`cmd/orchestrator/main.go` adds env vars:
- `PI_ZG_COMPUTE_ENDPOINT` — provider URL (resolved during setup via `0g-compute-cli`)
- `PI_ZG_COMPUTE_BEARER` — `app-sk-<secret>` token
- `PI_ZG_COMPUTE_MODEL` — defaults to `qwen-2.5-7b-instruct` (only sealed model on testnet)

When all three are set, orchestrator constructs `zg_compute.New(...)` and wraps existing OpenRouter provider as `fallback.New(zgCompute, openRouter, hook)` for both planner and reviewer LLMs. When missing, behavior reverts to today's M7-B.3 OpenRouter-only baseline.

**Lite reviewer cross-check** (per Q6):
- `swarm.ReviewArgs.PriorPersonaSealed map[string]bool` — populated from prior persona receipts.
- `composeCoderOutput` extended to prefix the diff section with:
  ```
  planner_sealed: <true|false>
  coder_sealed: false
  ```
  Coder is always `false` in M7-C scope (Pi runs on unsealed OpenRouter).
- `ReviewerSystemPrompt` extended w/ a sentence: "Each persona's output is preceded by `<persona>_sealed:` flags. `false` means that persona ran on unsealed inference; treat its output with extra scrutiny."

**Audit log additions** (optional / cuts-list candidate):
- New event kinds: `inference_sealed`, `inference_fell_back`. **The `onFallback` closure is constructed in `cmd/orchestrator/main.go` (where `fallback.New(...)` is called).** If implementing this audit-log addition, the closure needs access to a callback that knows the current task ID — easiest path: orchestrator passes a small `auditFn` to the closure that calls `q.repo.AppendEvent(ctx, taskID, "inference_fell_back", payload)`. Skip if time-pressed; the `slog.Warn` already provides observability without DB writes.

## §3 — Components (detail)

### `era-brain/llm/zg_compute`

Files:
- `era-brain/llm/zg_compute/zg_compute.go` — Provider impl
- `era-brain/llm/zg_compute/zg_compute_test.go` — 6 unit tests via `httptest.Server`
- `era-brain/llm/zg_compute/zg_compute_live_test.go` — `//go:build zg_live` integration test

Public API:

```go
type Config struct {
    BearerToken      string        // app-sk-<secret>
    ProviderEndpoint string        // per-provider base URL
    DefaultModel     string        // testnet: "qwen-2.5-7b-instruct"
    HTTPTimeout      time.Duration // default 60s
}

type Provider struct { ... }

func New(cfg Config) *Provider

func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error)
```

Compile-time interface check at package scope: `var _ llm.Provider = (*Provider)(nil)`.

### `era-brain/llm/fallback`

Files:
- `era-brain/llm/fallback/fallback.go`
- `era-brain/llm/fallback/fallback_test.go` — 4 unit tests

```go
type FallbackErrorHandler func(primaryErr error)

type Provider struct { ... }

func New(primary, secondary llm.Provider, onFallback FallbackErrorHandler) *Provider

func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error)
```

Sequential semantics: try primary; if non-nil error, call `onFallback(err)` (if provided), then try secondary. If secondary errors, return error wrapping both.

### Setup phase (M7-C.1 Phase 0)

`scripts/zg-compute-smoke/zg-compute-smoke.go` — standalone Go program. Reads `PI_ZG_COMPUTE_ENDPOINT` + `PI_ZG_COMPUTE_BEARER` from env. POSTs a single chat-completion request. Prints response model + content + whether TEE header present.

The bearer + endpoint are obtained via `0g-compute-cli` (Node.js, run once by hand). Setup README documents:
1. `npm install -g @0glabs/0g-compute-cli` (or wherever the CLI lives — confirmed during setup)
2. Use the CLI's deposit + transfer commands to fund a provider sub-account (1 ZG min)
3. Use the CLI's `inference get-secret` to generate `app-sk-<secret>`
4. Resolve provider endpoint URL via the CLI's broker query
5. Populate `.env` w/ the three vars

### `era-brain/examples/coding-agent/main.go` — modified

Add `--zg-compute` boolean flag. When true:
- Read same env vars as orchestrator.
- Construct `zg_compute.New(...)` and wrap existing planner+reviewer LLMs via `fallback.New(...)`.
- Defer-close any resources (HTTP client doesn't need explicit close).

When false: behavior unchanged from M7-B.1.4.

### `era/cmd/orchestrator/main.go` — modified

After existing OpenRouter construction (around the era-brain swarm wiring block):

```go
plannerProv := plannerLLM // existing openrouter
reviewerProv := reviewerLLM // existing openrouter

zgEndpoint := os.Getenv("PI_ZG_COMPUTE_ENDPOINT")
zgBearer   := os.Getenv("PI_ZG_COMPUTE_BEARER")
zgModel    := envOrDefault("PI_ZG_COMPUTE_MODEL", "qwen-2.5-7b-instruct")

if zgEndpoint != "" && zgBearer != "" {
    zgComp := zg_compute.New(zg_compute.Config{
        BearerToken:      zgBearer,
        ProviderEndpoint: zgEndpoint,
        DefaultModel:     zgModel,
    })
    plannerProv = fallback.New(zgComp, plannerLLM, func(err error) {
        slog.Warn("planner sealed inference fell back to openrouter", "err", err)
    })
    reviewerProv = fallback.New(zgComp, reviewerLLM, func(err error) {
        slog.Warn("reviewer sealed inference fell back to openrouter", "err", err)
    })
    slog.Info("0G Compute sealed inference wired", "model", zgModel)
}

// pass plannerProv/reviewerProv to swarm.New (replaces direct plannerLLM/reviewerLLM)
```

Single shared zg_compute provider across both personas (testnet has one endpoint per provider, one model).

### `era/internal/swarm` — modified

Three changes:

1. **`ReviewArgs`** gains `PriorPersonaSealed map[string]bool` field.
2. **`composeCoderOutput`** signature extended:
   ```go
   func composeCoderOutput(diff string, findings []string, plannerSealed, coderSealed bool) string {
       header := fmt.Sprintf("planner_sealed: %t\ncoder_sealed: %t\n\n", plannerSealed, coderSealed)
       return header + existingComposeBody(diff, findings)
   }
   ```
   The single caller is `Swarm.Review()` in `swarm.go`. Update the call site to read `args.PriorPersonaSealed["planner"]` and pass it (plus `coderSealed=false` always — Pi is unsealed in M7-C scope) into the new params.
3. **`ReviewerSystemPrompt`** appended w/ sealed-flag explanation paragraph.

### `era/internal/queue/queue.go` — modified

In `RunNext`'s reviewer-call block, populate `PriorPersonaSealed` from the planner receipt:

```go
priorSealed := map[string]bool{
    "planner": plannerReceipt.Sealed,
    "coder":   false, // Pi is unsealed; M7-C scope
}
rr, rerr := q.swarm.Review(ctx, swarm.ReviewArgs{
    // ... existing fields
    PriorPersonaSealed: priorSealed,
})
```

## §4 — Error handling, testing, security

### Error handling

- zg_compute HTTP error (4xx/5xx, timeout, dial failure) → error returned to fallback wrapper.
- fallback catches error → calls `onFallback(err)` → tries secondary (OpenRouter). If OpenRouter also errors → return error wrapping both.
- TEE signature header missing on a 200 OK response → `Sealed=false` returned. NOT an error. Caller (reviewer prompt logic) decides what to do with the flag.
- Malformed JSON in zg_compute response → return error → fallback fires.
- 0g-compute-cli generates a bearer that's already expired → first call returns 401 → fallback fires immediately. User must regenerate bearer offline. Document in setup README.
- Provider sub-account out of ZG → 402/403 from provider → fallback fires. Document the deposit-flow in setup README.

### Testing

**Unit (zg_compute):**
- Happy path: bearer header sent correctly, JSON body shape matches OpenAI chat.completions, TEE-signature header → `Sealed=true`.
- TEE-signature header absent → `Sealed=false` (still success).
- 401 → error returned.
- 5xx → error returned.
- Malformed JSON → error returned.
- Per-request model override (`Request.Model` overrides `DefaultModel`).
- Compile-time `var _ llm.Provider = (*Provider)(nil)`.

**Unit (fallback):**
- Primary success → primary's response returned; `onFallback` not called.
- Primary fail, secondary success → secondary's response; hook called once with primary's error.
- Both fail → error wraps both; hook still called for primary fail.
- Sealed flag passes through whichever provider answered.

**Live (`zg_live` build tag):**
- `zg_compute.Provider.Complete` against real testnet endpoint, returns valid response w/ Sealed=true. Skip if env vars missing.

**era integration tests (M7-C.2):**
- swarm test asserts that when `PriorPersonaSealed` is set, planner+reviewer prompts contain the sealed-flag block. Use a stub LLM that records `lastReq` and verify substring presence.

**Live gate (M7-C.2 final):**
- Real `/task` via Telegram. Stdout shows `0G Compute sealed inference wired` boot line + (during task) sealed-inference HTTP calls. Reviewer's critique text contains `planner_sealed: true` (or `false` if fell back). Verify era-brain.db audit log entries have `Sealed=true` on planner + reviewer receipt JSON.

### Security

- **Bearer token in env var.** Same surface as `PI_OPENROUTER_API_KEY`. Lives only in `.env` and orchestrator process memory. Never in audit log, never in DM. Document key-rotation steps in setup README.
- **TEE signature trust boundary.** We record the signature presence as `Sealed=true` but don't cryptographically verify. Honest hackathon-scope limitation: judges can independently verify signatures via 0G's tools if they care. Same disclaimer as parent spec §4 ("we don't invent forgery defense beyond what 0G provides").
- **Receipt forgery surface.** A compromised orchestrator could fake `Sealed=true` in the audit log without a real TEE signature. Mitigation: M7-D records receipts on-chain via iNFT contract; judges audit on-chain receipts independently of orchestrator-reported state.
- **Provider impersonation.** If `PI_ZG_COMPUTE_ENDPOINT` points at a malicious server, we'd happily call it w/ our bearer + record `Sealed=true` from any header value. Mitigation: setup README warns to use only provider URLs queried from official broker. Production would pin endpoints; hackathon trusts the deployment script.

## §5 — Milestones (phases)

Each milestone splits into ~5 phases w/ TDD cycle + commit + tag.

### M7-C.1 — era-brain SDK

| Phase | Tag | What |
|---|---|---|
| C.1.0 | `m7c1-0-setup` | 0G Compute setup: install `0g-compute-cli`, deposit + transfer ZG, generate bearer, identify provider endpoint URL. NO code. Smoke script (`scripts/zg-compute-smoke/`) makes one bearer-auth POST. Live gate = smoke prints `OK` + TEE header presence. Commit `.env.example` + smoke. |
| C.1.1 | `m7c1-1-zg-compute` | `era-brain/llm/zg_compute` package — `Provider` impl + 6 unit tests via `httptest.Server` + build-tagged live test. |
| C.1.2 | `m7c1-2-fallback` | `era-brain/llm/fallback` package — wraps two providers + 4 unit tests. Pure Go logic; no SDK calls. |
| C.1.3 | `m7c1-done` | Extend `examples/coding-agent/main.go` w/ `--zg-compute` flag. Live gate: real OpenRouter + real 0G Compute call; planner+reviewer receipts show `Sealed: true`. |

### M7-C.2 — era integration

| Phase | Tag | What |
|---|---|---|
| C.2.1 | `m7c2-1-wired` | `cmd/orchestrator/main.go` constructs zg_compute + wraps planner+reviewer LLMs via fallback when env vars present. Falls back to OpenRouter-only when missing. Boot log says `0G Compute sealed inference wired`. |
| C.2.2 | `m7c2-2-cross-check` | swarm-side: `composeCoderOutput` adds `planner_sealed:` / `coder_sealed:` lines. `ReviewArgs.PriorPersonaSealed` field. Reviewer system prompt updated. queue.RunNext populates the field from planner's receipt. Tests cover both sealed paths. |
| C.2.3 | `m7c2-done` | Live gate: real Telegram `/task`. Verify boot log + reviewer DM contains evidence of sealed inference. era-brain.db audit log shows `Sealed=true` on planner + reviewer receipt JSON. |

**Total estimated effort:** 2 days (M7-C.1) + 1 day (M7-C.2) = ~3 days subagent work. C.1.0 setup is the wildcard.

## §6 — Decisions log (Q&A from brainstorming)

| Q | Choice | Rationale |
|---|---|---|
| Q1 — Milestone split | A: two milestones (C.1 SDK + C.2 era integration) | Same proven pattern as M7-B → predictable cadence + clean phase gates. |
| Q2 — Compute access setup | B: never used Compute → Phase 0 setup | Bearer + endpoint + provider funding all need verification before code lands. |
| Q3 — SDK choice | A: roll our own HTTP client | No Go SDK exists. Bearer-auth is OpenRouter-shaped; ~150 lines. |
| Q4 — Fallback architecture | A: new `era-brain/llm/fallback` package | Same pattern as `memory/dual`. Reusable for any LLMProvider pair. Track 1 framework story. |
| Q5 — Sealed personas | A: both planner + reviewer | Strongest swarm-uses-sealed-inference demo. Coder remains Pi (unsealed) — deferred. |
| Q6 — Reviewer cross-check | A: lite (Sealed flag visibility) | Honest about what we can verify (Sealed flag) vs what we can't (TEE signature crypto). Real signal when fallback fires. |

## §7 — Out of scope (deferred)

- **Coder persona via 0G Compute.** Pi is in-Docker; bridging Pi → 0G Compute is a separate effort.
- **Cryptographic TEE signature verification.** Requires TS tooling that has no Go equivalent. Defer to post-hackathon.
- **On-chain receipt recording for sealed inference.** M7-D's iNFT `recordInvocation` will hash the sealed receipt and put it on-chain.
- **Per-provider failover.** Today fallback is just primary→secondary. Real production might want primary→backup-zg-provider→openrouter. Out of scope.
- **Mainnet model migration.** When mainnet ships GLM-5-FP8 / qwen3.6-plus, swap models via env vars without code change.

## §8 — Cuts list (in order if slipping)

1. Live integration test in C.1.1 (`zg_live` tag) — defer to C.1.3's live gate.
2. Reviewer cross-check (C.2.2) — defer; ship just sealed flag in receipts (C.2.1) without surfacing to reviewer LLM. Loses some demo bait but keeps M7-C testable.
3. Audit-log event kinds (`inference_sealed`/`inference_fell_back`) — minor logging, defer.
4. Per-persona model env vars (was in earlier draft as `PI_BRAIN_PLANNER_ZG_MODEL` etc.) — testnet has one model; one env var (`PI_ZG_COMPUTE_MODEL`) suffices. Mainnet expansion comes when models do.

---
