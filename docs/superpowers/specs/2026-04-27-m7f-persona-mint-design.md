# M7-F — `/persona-mint` + Custom Personas Design

**Status:** approved 2026-04-27.
**Parent spec:** `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md` (was deferred from M7-D.2).
**Hackathon impact:** strengthens all three already-claimed prize tracks (0G Track 2 iNFT, 0G Track 1 SDK, ENS Best Integration) with one feature.

## §1 — Goal

Telegram-driven custom persona minting + use:

- `/persona-mint <name> <prompt>` — owner-only command. Validates name → uploads system prompt to 0G Storage → mints a new iNFT token on the existing `EraPersonaINFT` contract → registers `<name>.vaibhav-era.eth` ENS subname with text records → inserts into local SQLite registry → DMs back token ID + chainscan + ENS + 0G storage URI.
- `/task --persona=<name> <desc>` — runs the swarm with the **coder slot** swapped for the named custom persona. Planner + reviewer remain default. The custom coder uses its own memory namespace = `<name>` (each persona has an independent evolving brain).
- `/personas` — lists all known personas (3 defaults + custom mints): name, ENS subname, token ID, description.

Time budget: ~2 days. Five phases (see §7).

## §2 — Architecture

Extends existing layers — no new packages:

- **`era-brain/inft/zg_7857.Provider.Mint`** — replace stub (`ErrNotImplemented`) with real `contract.Mint(...)` call. Returns the new token ID + persona metadata. Parses `Transfer` event from receipt logs to extract the auto-incremented token ID.
- **`era-brain/memory/zg_storage` (or `zg_log` helper)** — new thin helper `UploadPrompt(ctx, content) (uri string, err error)` reusing the M7-B 0G Storage client. Idempotent on content hash.
- **New SQLite migration `0011_personas.sql`** — table `personas (token_id, name, owner_addr, system_prompt_uri, ens_subname, description, created_at)` with `UNIQUE(name)`.
- **`internal/queue.PersonaRegistry`** — new interface (mirrors `INFTProvider` / `ENSResolver` seam): `Lookup`, `List`, `Insert`. Backed by `internal/db/personas.go` SQLite repo.
- **`internal/telegram` handler** — new commands `/persona-mint` and `/personas`. `/task` parses `--persona=<name>` flag (extends existing `--budget=` parser pattern). `Ops` interface gains `MintPersona`, `ListPersonas`, and an extended `CreateTask` with `personaName`.
- **`internal/swarm`** — when queue invokes the swarm, if `personaName != ""`, the coder slot is built from the persona's prompt + memory namespace. Default behavior unchanged when `personaName == ""`.
- **`cmd/orchestrator/main.go`** — boot-time `personasReconcile()` upserts the 3 default personas (token IDs 0/1/2) into SQLite if missing. Custom mints reconcile from on-chain `Transfer` events on next boot if SQLite was wiped (cuts-list candidate; see §10).

### Integration with existing on-chain plumbing

Mint flow chains the existing primitives:
1. **0G Storage upload** (M7-B) → URI
2. **iNFT mint** (M7-D.2 contract, M7-F adds Provider.Mint) → token ID
3. **ENS subname write** (M7-E) → 4 text records

All three are best-effort beyond the first two. Storage + mint required; ENS decorative-but-prized. Failure of ENS half does not undo the mint — recoverable via reconcile.

## §3 — Components (detail)

### `era-brain/inft/zg_7857.Provider.Mint`

```go
// Mint creates a new persona iNFT token, owned by the orchestrator wallet.
// systemPromptURI becomes the contract's tokenURI for the new token.
// Returns the auto-incremented token ID + persona metadata.
func (p *Provider) Mint(ctx context.Context, name, systemPromptURI string) (inft.Persona, error)
```

Implementation:
1. Build `auth := *p.auth; auth.Context = ctx` (same shallow-copy pattern as `RecordInvocation`).
2. `tx, err := p.contract.Mint(&auth, p.auth.From, systemPromptURI)` — calls the deployed contract's `mint(address to, string memory uri) external onlyOwner returns (uint256)`. Note: `Provider` has no `signer` field; the wallet address is `p.auth.From`.
3. Wait for receipt: `rc, err := bind.WaitMined(ctx, p.client, tx)` then check `rc.Status == types.ReceiptStatusSuccessful`. (The current `Provider.RecordInvocation` discards the tx; for Mint we need the receipt to extract the token ID, so we always wait — no `dialedClient` gate needed since this code path always sets up a real client via `New`.)
4. Parse `Transfer(from=0x0, to=auth.From, tokenId)` event from receipt logs using the abigen-generated filter helper `p.contract.ParseTransfer(*log)` (defined on `EraPersonaINFTFilterer` at `bindings/era_persona_inft.go`). Iterate `rc.Logs`, call `ParseTransfer`, take the first match where `event.From == 0x0` and `event.To == p.auth.From`. Token ID is `event.TokenId.String()`.
5. Return `inft.Persona{TokenID: tokenID, Name: name, SystemPromptURI: systemPromptURI, Owner: p.auth.From.Hex()}`.

### 0G Storage upload helper

Live at `era-brain/memory/zg_log/upload_prompt.go` or new `era-brain/storage/zg_storage/upload.go`. Implementer picks; the M7-B SDK client is already available. Helper:

```go
// UploadPrompt uploads a system prompt to 0G Storage and returns the canonical
// gateway URI for retrieval. Idempotent on content hash — uploading the same
// content twice returns the same URI.
func UploadPrompt(ctx context.Context, content string) (uri string, err error)
```

Returns a URI shape matching what M7-B already produces (e.g., `https://gateway.0g.ai/blob/<hash>`).

### Migration `0011_personas.sql`

```sql
CREATE TABLE personas (
    token_id          TEXT PRIMARY KEY,            -- decimal string from contract
    name              TEXT NOT NULL UNIQUE,
    owner_addr        TEXT NOT NULL,               -- 0x... (orchestrator wallet)
    system_prompt_uri TEXT NOT NULL,               -- 0G Storage URI (or raw GitHub URL for defaults)
    ens_subname       TEXT,                        -- e.g. "rustacean.vaibhav-era.eth"; NULL when ENS not wired
    description       TEXT,                        -- first 60 chars of prompt for /personas listing
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX personas_name_idx ON personas(name);
```

### `internal/queue.PersonaRegistry`

```go
type PersonaRegistry interface {
    Lookup(ctx context.Context, name string) (Persona, error)  // sql.ErrNoRows -> wrap as ErrPersonaNotFound
    List(ctx context.Context) ([]Persona, error)
    Insert(ctx context.Context, p Persona) error               // unique-violation -> ErrPersonaNameTaken
}

type Persona struct {
    TokenID          string
    Name             string
    OwnerAddr        string
    SystemPromptURI  string
    ENSSubname       string
    Description      string
    CreatedAt        time.Time
}

var (
    ErrPersonaNotFound  = errors.New("persona not found")
    ErrPersonaNameTaken = errors.New("persona name already taken")
)
```

### `internal/db/personas.go`

Repo methods backing the registry: `InsertPersona`, `GetPersonaByName`, `ListPersonas`. Uses standard SQLx pattern from existing repo files.

### Telegram `Ops` interface extension

```go
type Ops interface {
    // ... existing methods ...
    MintPersona(ctx context.Context, name, systemPrompt string) (PersonaMintResult, error)
    ListPersonas(ctx context.Context) ([]Persona, error)
    CreateTask(ctx context.Context, desc, targetRepo, profile, personaName string) (int64, error)  // signature change
}

type PersonaMintResult struct {
    TokenID         string
    MintTxHash      string
    ENSSubname      string  // empty if ENS not wired
    SystemPromptURI string
}
```

### Telegram handler — new commands

`/persona-mint <name> <prompt>` parsing:
- Trim, split on first whitespace → `name`, `prompt`
- Validate name: regex `^[a-z0-9-]{3,32}$`, not in reserved set `{planner, coder, reviewer}`
- Validate prompt: ≥ 20 chars, ≤ 4000 chars
- On success: DM "minting persona…", call `Ops.MintPersona`, edit message with result

`/personas`:
- Calls `Ops.ListPersonas`
- Renders table-style DM with name + ENS subname + token ID + truncated description

`/task --persona=<name> <desc>`:
- Existing `--budget` parser extended with `--persona=` flag
- Empty / unset persona = default behavior (unchanged)
- Resolves at queue-RunNext time (NOT at /task creation) so unknown-persona errors surface as task failure (single failure path)

### Swarm coder swap

`internal/queue/queue.go` `RunNext`: when task has `persona_name != ""`:
1. `persona := q.personas.Lookup(ctx, taskRow.PersonaName)` → fail task on `ErrPersonaNotFound`
2. `promptText := q.zgStorage.FetchPrompt(persona.SystemPromptURI)` (fetch + cache in-process; cuts-list candidate to add LRU)
3. Build coder slot with prompt + memory namespace = `persona.Name`
4. iNFT recordInvocation post-coder uses `persona.TokenID` instead of hardcoded `coderTokenID`

### What stays untouched

- Default persona pipeline when `--persona=` is unset
- Existing iNFT contract — `mint(address, string)` already exists from M7-D.1
- ENS Provider — `EnsureSubname` + `SetTextRecord` already work for arbitrary labels
- 0G Storage SDK plumbing
- `tgNotifier.ensFooter` — automatically reflects whichever subnames are used in the task because it reads from the queue's ENSResolver, but for custom personas the footer needs to read the actual labels used (see §10 risk #5)

## §4 — Data flows

### `/persona-mint rustacean You only write idiomatic Rust...`

```
handler validates name (lowercase alnum + dashes, ≥3 chars, ≠ reserved)
  → handler validates prompt (20-4000 chars)
  → Ops.MintPersona(ctx, "rustacean", "You only write...")
    → personas.Lookup("rustacean") → ErrPersonaNotFound (else hard-reject)
    → zgStorage.UploadPrompt(prompt) → "https://gateway.0g.ai/blob/<hash>"
    → zg_7857.Mint(ctx, "rustacean", uri) → tokenID="3", txHash="0xabc..."
    → ens.EnsureSubname(ctx, "rustacean")            // if ens != nil
    → ens.SetTextRecord(ctx, "rustacean", "inft_addr",      "0x33847c...")
    → ens.SetTextRecord(ctx, "rustacean", "inft_token_id",  "3")
    → ens.SetTextRecord(ctx, "rustacean", "zg_storage_uri", uri)
    → ens.SetTextRecord(ctx, "rustacean", "description",    first 60 chars)
    → personas.Insert(Persona{TokenID:"3", Name:"rustacean", ENSSubname:"rustacean.vaibhav-era.eth", ...})
  → DM (edited from "minting…"):
      ✓ persona "rustacean" minted as token #3
        chainscan: https://chainscan-galileo.0g.ai/tx/0xabc...
        ens:       https://sepolia.app.ens.domains/rustacean.vaibhav-era.eth
        prompt:    https://gateway.0g.ai/blob/<hash>
```

Total cost: 1 0G Storage upload + 1 0G iNFT mint tx + ≤ 5 Sepolia txs (1 register + 4 setText) ≈ 0.005 ZG + 0.0015 ETH ≈ **$0.005 per /persona-mint**. Within faucet budget.

### `/task --persona=rustacean fix the auth bug`

```
handler parses --persona=rustacean → Ops.CreateTask(ctx, desc, repo, profile, personaName="rustacean")
  task row stored with persona_name column  (migration adds the column)
  RunNext:
    if task.PersonaName != "":
      persona = q.personas.Lookup(ctx, task.PersonaName)
        → ErrPersonaNotFound: fail task with "unknown persona <name>; run /personas"
      promptText = q.zgStorage.FetchPrompt(persona.SystemPromptURI)
        → on error: fail task ("can't fetch persona prompt")
      coderSlot = customCoder(promptText, namespace=persona.Name, tokenID=persona.TokenID)
    else:
      coderSlot = defaultCoder
    brain.Run([planner_default, coderSlot, reviewer_default])
    → 3 sealed inference receipts (rustacean's receipt → memory namespace "rustacean")
    → 3 iNFT recordInvocation calls: planner=0, RUSTACEAN=3 (not 1), reviewer=2
    → DM with personas: footer showing rustacean.vaibhav-era.eth → token #3 (instead of coder.…)
```

The DM `personas:` footer (added in M7-E.2) needs to take the per-task list of `(label, tokenID)` pairs from the queue rather than reading `{planner, coder, reviewer}` from a hardcoded list. See §10 risk #5.

### `/personas`

```
Ops.ListPersonas(ctx) → SELECT * FROM personas ORDER BY CAST(token_id AS INTEGER)
DM:
  era personas
  ────────────
  #0  planner.vaibhav-era.eth   default planner — break tasks down
  #1  coder.vaibhav-era.eth     default coder — write changes
  #2  reviewer.vaibhav-era.eth  default reviewer — flag deviations
  #3  rustacean.vaibhav-era.eth You only write idiomatic Rust…
```

## §5 — Error handling

| Failure | Behavior |
|---|---|
| Name validation fails (uppercase, special chars, < 3, > 32, reserved name) | DM error message; no chain calls |
| Name already in SQLite | DM "name '<name>' already taken"; no chain calls |
| Prompt validation fails (empty, < 20, > 4000) | DM error; no chain calls |
| 0G Storage upload fails | DM error; no mint, no ENS, no SQLite write |
| iNFT mint reverts (gas, ACL, nonce) | DM error; prompt blob orphaned in 0G (acceptable — tiny, no rollback) |
| Mint succeeded; ENS write reverts | DM warn "minted as token #N; ENS not set (will retry on next boot)"; SQLite written WITHOUT ens_subname; **boot reconcile re-runs `EnsureSubname` + setText for personas with empty ens_subname** (see §7.5) |
| Mint succeeded; SQLite insert fails | DM error "minted token #N at <chainscan>; local registry write failed; will recover on next boot"; **boot reconcile scans on-chain `Transfer` events from highest known token ID and upserts** (see §7.5) |
| `/task --persona=<name>`: name unknown | task fails with "unknown persona <name>; run /personas"; NO chain calls made |
| Persona prompt fetch from 0G fails at /task time | fail task ("can't fetch persona prompt"); receipts not generated |
| `/persona-mint` invoked with iNFT env vars missing | hard reject ("iNFT env vars required") — mint depends on it |
| `/persona-mint` invoked with ENS env vars missing | mint succeeds; SQLite has empty ens_subname; reconcile next boot if ENS env vars get set |

**Critical invariant:** /persona-mint is best-effort across the 4-7 chain interactions (storage → mint → ENS register → ENS setText × 4). Storage + mint are required; ENS is decorative-but-prized. Failure of ENS half does not undo the mint.

## §6 — Security

Single-user bot (allowed user ID gate from M0). All `/persona-mint` calls authenticate via existing Telegram allow-list. Hot wallet exposure unchanged from M7-D/M7-E:
- Loss of `PI_ZG_PRIVATE_KEY` = loss of mint capability + ENS write capability (same blast radius as before)
- Spam mints would drain the faucet; mitigated by the single-user gate

Reserved names list (`planner`, `coder`, `reviewer`) prevents user from minting a token that overrides defaults' SQLite rows. The default 3 are seeded by `personasReconcile()` at boot using their hardcoded values; user mints get token IDs ≥ 3.

## §7 — Implementation phases (~2 days)

This becomes **M7-F**. Five phases following the M7-D.2 / M7-E pattern. Each phase ends with `go test -race ./...` green for both modules and a tagged commit.

### M7-F.1 — `zg_7857.Mint` impl + 0G prompt upload helper (~0.5 day)

- Phase 1a: replace `ErrNotImplemented` in `Mint` with real `contract.Mint(...)` call. Parse Transfer event for token ID. Unit tests via simulated.Backend (deploy contract, call Mint, assert tokenID + ownerOf + tokenURI). Live `ens_live`-style build-tagged test mints + reads tokenURI on testnet (cleanup: token cannot be deleted; live test mints a "test-token-{timestamp}" that is acceptable artifact).
- Phase 1b: `UploadPrompt` helper in 0G storage package. TDD with mock storage client. Live test uploads + retrieves. Tag `m7f-1-mint-and-upload`.

### M7-F.2 — `personas` SQLite migration + repo CRUD (~0.5 day)

- Migration `0011_personas.sql`. Repo functions `InsertPersona`, `GetPersonaByName`, `ListPersonas`. `internal/db/personas.go`. TDD with in-memory SQLite + uniqueness assertion. Tag `m7f-2-registry`.

### M7-F.3 — Telegram `/persona-mint` + `/personas` commands (~0.5 day)

- Extend `Ops` interface with `MintPersona` + `ListPersonas`. Handler routes `/persona-mint` (with name + prompt validation, hard reject on collision) and `/personas`. TDD via `handler_test.go` patterns. Tag `m7f-3-telegram-mint`.

### M7-F.4 — `/task --persona=<name>` plumbing + ensFooter refactor (~0.5 day)

- Flag parse in handler (extend `parseBudgetFlag` pattern). `CreateTask` signature gains `personaName`. DB column added (migration `0012_tasks_persona_name.sql`). Queue `RunNext`: `Lookup(persona) → fetchPrompt → custom coder slot → recordInvocation tokenID = persona.TokenID`.
- **ensFooter refactor** (per §10 risk #5): add `PersonaLabels []string` to `CompletedArgs` + `NeedsReviewArgs`; queue populates per task; `ensFooter` signature gains `labels` param.
- TDD via `queue_run_test.go` with stubPersonas. **Required new test** `TestRunNext_CustomPersona_FreshNamespace_NoError` — exercises a never-before-seen memory namespace string to verify M7-B.3 evolving memory handles it (don't assume; assert).
- Tag `m7f-4-task-persona`.

### M7-F.5 — orchestrator boot reconcile + live Telegram gate (~0.7 day)

`personasReconcile()` in `main.go` runs three reconcile passes at boot, each idempotent:

1. **Default seed.** If `personas` table is missing rows for token IDs 0/1/2, INSERT OR IGNORE them with hardcoded values (matches the iNFT mint metadata committed at M7-D.1: planner, coder, reviewer + their raw-GitHub URIs).
2. **On-chain Transfer scan.** Compute `maxKnownTokenID := SELECT MAX(CAST(token_id AS INTEGER))`. Use `p.contract.FilterTransfer(opts, []common.Address{zeroAddr}, []common.Address{p.auth.From}, nil)` from block 0 (or last known boot block, stored in a small key-value table to avoid scanning history each boot). For each event with `tokenID > maxKnownTokenID`, fetch `tokenURI(tokenID)` via the contract, fetch the prompt blob from 0G storage, upsert into SQLite with `ens_subname = ''`. Recovers from "mint succeeded but SQLite insert failed" — closes §5 row 2.
3. **ENS retry pass.** `SELECT * FROM personas WHERE ens_subname = ''` → for each, run `EnsureSubname(label) + 4 × SetTextRecord(...)` (same as `syncPersonaENS` from M7-E.3 but driven from SQLite rows instead of hardcoded list). Closes §5 row 1.

All three passes are non-fatal individually — failures log + continue (mirrors M7-E.3 pattern). The whole reconcile runs after `q.SetNotifier(notifier)` and before goroutines start consuming tasks.

Real Telegram live gate: `/persona-mint rustacean ...` → DM links → `/task --persona=rustacean ...` → PR + DM with `rustacean.vaibhav-era.eth` in personas footer + iNFT contract event for token #3. Tag `m7f-done`.

## §8 — Out of scope (deferred / cuts)

- **Per-slot persona flags** (`--planner=`, `--reviewer=`). Trivial to add later (same plumbing as `--persona=` but more flag variants).
- **Persona deletion / token burn.** No use case for hackathon.
- **Re-mint to update prompt.** Hard reject on name collision per §5; user picks a new name.
- **Multi-user persona ownership.** Single-user bot; mints always go to orchestrator wallet.
- **Marketplace / royalty splits.** Out of M7-F entirely (cuts-list since M7-D).
- **`/persona-edit`, `/persona-show <name>`.** Stretch only.
- **Avatar / extended ENS records** (avatar URL, github_url, etc.). Add 1-2 in M7-F.3 if cheap, otherwise defer.
- **(MOVED IN-SCOPE)** ~~Boot reconcile from on-chain Transfer events.~~ Promoted to M7-F.5 because it's the recovery path for two §5 failure modes (mint+SQLite-fail, mint+ENS-fail). Cost: ~1 hour of implementation; halves the demo-fragility blast radius.
- **LRU prompt cache.** First-task latency for a persona = 1 0G fetch (~500ms-2s); per-process map cache after that. No eviction needed at demo scale.

## §9 — Acceptance criteria (M7-F done)

1. `go build ./...` green; `go test -race -count=1 ./...` green for both modules.
2. Real `/persona-mint <name> <prompt>` via Telegram:
   - DM contains: token #N, chainscan link, sepolia ENS link, 0G storage URI
   - On-chain: new iNFT token visible on chainscan-galileo at the contract address
   - On-chain: `<name>.vaibhav-era.eth` resolvable on sepolia.app.ens.domains with all 4 text records (`inft_addr`, `inft_token_id`, `zg_storage_uri`, `description`)
   - SQLite `personas` row exists
3. Real `/task --persona=<name> <desc>`:
   - PR opens as before; reviewer flow works
   - Telegram DM `personas:` footer shows the **custom subname** (e.g. `rustacean.vaibhav-era.eth → token #3`), not `coder.…`
   - iNFT contract events show `recordInvocation` against token #N (not token #1 / coder)
   - Reviewer DM is unchanged in shape
4. `/personas` lists all known personas (3 defaults + custom mints).
5. Hard rejects: duplicate name, invalid name format, unknown persona at /task time.
6. Without ENS env vars: mint succeeds (SQLite written, no ENS records); /personas works; /task --persona= still works.
7. SQLite-wipe + restart: 3 defaults reconcile automatically AND custom personas re-import from on-chain Transfer events — `/personas` shows the full pre-wipe list after a single boot. ENS reconcile pass also re-fills any rows with empty `ens_subname`.

## §10 — Risks + cuts list (in order if slipping)

1. **Contract `mint` is `external onlyOwner`** — verified, signer == owner. Pre-flight: `cast call $PI_ZG_INFT_CONTRACT_ADDRESS 'owner()(address)' --rpc-url $PI_ZG_EVM_RPC` returns signer.
2. **Transfer event parsing fragility.** ERC-721 standard event is `Transfer(address indexed from, address indexed to, uint256 indexed tokenId)`. Implementer must use abigen-generated event filter, not raw log decoding. Recovery: copy the planner/reviewer minted-token-ID parsing pattern from the M7-D.1 deploy script if it exists.
3. **0G Storage upload latency** could make `/persona-mint` feel slow (~5-10s). Acceptable. Mitigation: edit "minting…" DM with a progress phase ("✓ uploaded prompt; minting…"). Cuts-list.
4. **Memory namespace per persona, fresh on first task.** Existing M7-B.3 evolving memory handles namespace-not-yet-existing (returns empty memory, writes seed). Verification not assumed: M7-F.4 includes `TestRunNext_CustomPersona_FreshNamespace_NoError` which exercises a fresh namespace and asserts task completes successfully.
5. **`personas:` footer in DM — refactor required.** M7-E.2 hardcoded `[]string{"planner", "coder", "reviewer"}` inside `ensFooter` (`cmd/orchestrator/main.go:471`). With custom personas the footer must reflect the actual labels used per task. Refactor (option A from prior review):
   - **New signature:** `ensFooter(ctx context.Context, ens ENSResolver, labels []string) string`. Caller passes the labels for THIS task.
   - **Caller plumbing:** `queue.CompletedArgs` + `queue.NeedsReviewArgs` (in `internal/queue/queue.go:72-101`) get a new field `PersonaLabels []string`. Queue's `RunNext` populates it: for default tasks `["planner", "coder", "reviewer"]`; for `/task --persona=rustacean` `["planner", "rustacean", "reviewer"]`. tgNotifier's `NotifyCompleted` (main.go:292) and `NotifyNeedsReview` (main.go:366) read `a.PersonaLabels` and pass to `ensFooter`.
   - **Empty/nil labels fallback:** If `len(labels) == 0`, treat as legacy default (use the 3 builtins) — keeps backwards-compat with any code path that hasn't been updated.
   - **Test update:** existing 4 `TestEnsFooter_*` tests in `notifier_ens_test.go` get a `labels` arg added; new test `TestEnsFooter_CustomPersonaLabels` exercises non-default labels.
   This refactor lands in M7-F.4 alongside the `/task --persona=` plumbing.
6. **Reserved-name list collision.** Hard reject on `{planner, coder, reviewer}`. Test case in handler_test.
7. **ENS subname creation race** — same risk as M7-E (parent must be wrapped). Already wrapped, no risk.
8. **Default-persona reconcile collision.** Boot reconcile inserts defaults if missing. If a previous reconcile partially completed (e.g., 2 of 3 defaults), unique constraint must allow upsert. Use `INSERT OR IGNORE` for defaults, plain `INSERT` for user mints.
9. **Task migration `0012_tasks_persona_name.sql`** introduces a new column on the existing `tasks` table. Use `ALTER TABLE tasks ADD COLUMN persona_name TEXT NOT NULL DEFAULT ''` so existing rows are valid.
10. **Token ID 3 vs 4 vs 5 ambiguity.** Contract auto-increments. We trust `Transfer` event for the canonical ID. Don't try to predict.
