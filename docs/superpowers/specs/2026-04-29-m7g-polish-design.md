# M7-G — Polish Bucket Design

**Status:** approved 2026-04-29.
**Parent spec:** `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md` (cleanup before M7-H).
**Hackathon impact:** removes demo-day rough edges. No new prize tracks; makes existing tracks more credible.

## §1 — Goal

Three small fixes pre-demo:

1. **Paginate `Provider.ScanNewMints`** so orchestrator boot doesn't warn `personas: chain reconcile failed err="scan: zg_7857 filterTransfer: Block range is too large"` on Galileo's public RPC.
2. **Auto-backfill empty prompts** at boot from 0G storage. Heals the orphaned `rustacean` token #4 (minted in M7-F before SQLite cache existed) AND any future persona imported via `reconcileFromChain` that lacks a local prompt.
3. **`slog.Warn` on `defaultOwnerAddr` parse failures** so a corrupt `PI_ZG_PRIVATE_KEY` surfaces in logs instead of silently producing zero-address persona rows.

Time budget: ~1 hour. Single tagged commit `m7g-done`.

## §2 — Architecture

Three modifications to existing files. No new packages, no new migrations, no new env vars, no new chain interactions:

- **`era-brain/inft/zg_7857/zg_7857.go`** — refactor `ScanNewMints` to loop `FilterTransfer` in 1000-block chunks from block 0 to current head. Aggregate matched events across chunks. Existing call site in `cmd/orchestrator/personas_reconcile.go::reconcileFromChain` unchanged.
- **`cmd/orchestrator/personas_reconcile.go`** — add a 4th idempotent reconcile pass `reconcileBackfillPrompts(ctx, registry, storage)`. Runs after the existing 3 passes (default seed → on-chain Transfer scan → ENS retry). For every persona with `prompt_text = ""`, attempt `storage.FetchPrompt(persona.SystemPromptURI)`; on success, `registry.UpdatePromptText(name, content)`. Best-effort; failures log + skip.
- **`cmd/orchestrator/personas_reconcile.go`** — `defaultOwnerAddr` adds `slog.Warn` when `signerAddress(key)` returns an error, while still returning the zero address. No fail-loud.

The new reconcile pass requires:
- New repo method `Repo.UpdatePromptText(ctx, name, content) error`
- New `PersonaRegistry.UpdatePromptText` interface method
- Updates to all `PersonaRegistry` test stubs (stubPersonas, inMemoryRegistry — same pattern as M7-F.6.2's interface extension)

## §3 — Components (detail)

### `era-brain/inft/zg_7857/zg_7857.go::ScanNewMints` (paginate)

Current shape:
```go
iter, err := p.contract.FilterTransfer(&bind.FilterOpts{Context: ctx},
    []common.Address{zero}, []common.Address{p.auth.From}, nil)
```

New shape:
```go
const scanChunkSize = uint64(1000)

func (p *Provider) ScanNewMints(ctx context.Context, sinceTokenID int64) ([]TransferEvent, error) {
    // 90s upper bound on the whole pagination loop. Galileo public RPC has
    // ~30M blocks; even at 5ms/empty-chunk that's ~2.5min worst case — boot
    // mustn't hang on it. Caller can pass a longer ctx if needed.
    scanCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
    defer cancel()

    head, err := p.client.BlockNumber(scanCtx)
    if err != nil {
        return nil, fmt.Errorf("zg_7857 head block: %w", err)
    }

    var out []TransferEvent
    zero := common.Address{}
    for start := uint64(0); start <= head; start += scanChunkSize {
        end := start + scanChunkSize - 1
        if end > head { end = head }

        iter, err := p.contract.FilterTransfer(
            &bind.FilterOpts{Context: scanCtx, Start: start, End: &end},
            []common.Address{zero},
            []common.Address{p.auth.From},
            nil,
        )
        if err != nil {
            return nil, fmt.Errorf("zg_7857 filterTransfer chunk %d-%d: %w", start, end, err)
        }
        for iter.Next() {
            ev := iter.Event
            if ev.TokenId.Int64() <= sinceTokenID { continue }
            uri, urierr := p.contract.TokenURI(&bind.CallOpts{Context: scanCtx}, ev.TokenId)
            if urierr != nil { continue }
            out = append(out, TransferEvent{
                TokenID: ev.TokenId.String(),
                Owner:   ev.To.Hex(),
                URI:     uri,
            })
        }
        // Preserve iter.Error() check from the pre-M7-G implementation —
        // catches RPC errors during iteration, not just at the FilterTransfer call.
        if iterErr := iter.Error(); iterErr != nil {
            iter.Close()
            return nil, fmt.Errorf("zg_7857 filterTransfer chunk %d-%d iter: %w", start, end, iterErr)
        }
        iter.Close()
    }
    return out, nil
}
```

`p.client` must support `BlockNumber(ctx) (uint64, error)`. The `ContractClient` interface (M7-E.1 pattern) needs to expose this. Both `*ethclient.Client` and `simulated.Client` already have it. Add to interface if not present:

```go
type ContractClient interface {
    bind.ContractBackend
    bind.DeployBackend
    BlockNumber(ctx context.Context) (uint64, error)
}
```

### `cmd/orchestrator/personas_reconcile.go::reconcileBackfillPrompts`

```go
// reconcileBackfillPrompts fills empty prompt_text fields from 0G storage.
// Heals personas imported via reconcileFromChain (which has no local prompt)
// and personas minted before M7-F.6's SQLite cache existed (e.g., M7-F's
// rustacean token #4). Best-effort: 0G failures log + skip; next boot retries.
func reconcileBackfillPrompts(ctx context.Context, registry queue.PersonaRegistry, storage queue.PromptStorage) error {
    if storage == nil {
        return nil // can't backfill without 0G storage wired
    }
    all, err := registry.List(ctx)
    if err != nil { return fmt.Errorf("list: %w", err) }
    for _, p := range all {
        // Need to read prompt_text via the new GetPersonaPrompt; List() doesn't return it.
        cached, err := registry.GetPersonaPrompt(ctx, p.Name)
        if err != nil {
            slog.Warn("backfill: get cached prompt failed", "name", p.Name, "err", err)
            continue
        }
        if cached != "" { continue } // already cached
        if p.SystemPromptURI == "" { continue } // no URI to fetch from

        content, err := storage.FetchPrompt(ctx, p.SystemPromptURI)
        if err != nil {
            slog.Warn("backfill: fetch prompt from 0G failed", "name", p.Name, "uri", p.SystemPromptURI, "err", err)
            continue
        }
        if err := registry.UpdatePromptText(ctx, p.Name, content); err != nil {
            slog.Warn("backfill: update prompt_text failed", "name", p.Name, "err", err)
            continue
        }
        slog.Info("backfilled persona prompt from 0G", "name", p.Name, "bytes", len(content))
    }
    return nil
}
```

Wire as 4th pass in `personasReconcile`:
```go
if storage != nil {
    if err := reconcileBackfillPrompts(ctx, registry, storage); err != nil {
        slog.Warn("personas: backfill failed", "err", err)
    }
}
```

### `Repo.UpdatePromptText` + adapter

In `internal/db/personas.go`:
```go
func (r *Repo) UpdatePromptText(ctx context.Context, name, content string) error {
    _, err := r.q.db.ExecContext(ctx,
        `UPDATE personas SET prompt_text = ? WHERE name = ?`, content, name)
    if err != nil {
        return fmt.Errorf("update prompt_text: %w", err)
    }
    return nil
}
```

The existing adapter pattern (`Lookup`, `Insert`, `List`, `UpdateENSSubname`, `GetPersonaPrompt` already in place) gains one more passthrough — `UpdatePromptText` is the queue-interface name and `Repo.UpdatePromptText` is the impl, so they match directly with no extra adapter method needed.

### `PersonaRegistry` extension

In `internal/queue/queue.go`:
```go
type PersonaRegistry interface {
    Lookup(...)
    List(...)
    Insert(...)
    UpdateENSSubname(...)
    GetPersonaPrompt(...)
    UpdatePromptText(ctx context.Context, name, content string) error  // NEW (M7-G)
}
```

All test stubs (stubPersonas, inMemoryRegistry) gain a no-op or in-memory-map UpdatePromptText.

### `defaultOwnerAddr` slog.Warn

Current:
```go
func defaultOwnerAddr() string {
    key := os.Getenv("PI_ZG_PRIVATE_KEY")
    if key == "" { return zeroAddr }
    addr, err := signerAddress(key)
    if err != nil { return zeroAddr }
    return addr
}
```

New:
```go
func defaultOwnerAddr() string {
    key := os.Getenv("PI_ZG_PRIVATE_KEY")
    if key == "" { return zeroAddr }
    addr, err := signerAddress(key)
    if err != nil {
        slog.Warn("personas: defaultOwnerAddr parse failed; using zero address",
            "err", err)
        return zeroAddr
    }
    return addr
}
```

## §4 — Testing

Strict TDD. 3 new failing tests across 3 files:

1. **`era-brain/inft/zg_7857/zg_7857_test.go::TestProvider_ScanNewMints_Paginated`**
   - Setup: simulated.Backend, deploy contract, mint 2 tokens with `backend.AdjustTime` or repeated `Commit()` to span enough blocks. Configure scan chunk to 100 blocks (test override constant or pass param). Assert: `ScanNewMints(ctx, -1)` returns both events even when they're in different chunks.
   - Easier alternative: use a stub `ContractClient` whose `BlockNumber` returns a large value (e.g., 5000) and whose `FilterLogs` is called multiple times. Assert: caller invokes `FilterTransfer` more than once.

2. **`internal/db/personas_test.go::TestPersonas_UpdatePromptText`**
   - Insert persona with empty prompt_text → `UpdatePromptText(name, "RUSTACEAN-PROMPT")` → `GetPersonaPrompt(name)` returns "RUSTACEAN-PROMPT".

3. **`cmd/orchestrator/personas_reconcile_test.go::TestReconcile_BackfillPrompts_FetchesAndUpdates`**
   - inMemoryRegistry has 2 personas: one with prompt_text already set, one empty. Stub storage has the URI mapped for the empty one. After `reconcileBackfillPrompts`, the empty one is filled; the populated one is unchanged.
   - Plus `TestReconcile_BackfillPrompts_StorageFailureLogsAndContinues` — storage returns error; backfill doesn't crash; persona stays empty.

No live test for M7-G — every fix is purely in-memory/SQLite.

## §5 — Phases

Single phase, one commit:

### M7-G.1 — Paginate scan + auto-backfill + log defaultOwnerAddr (~1 hour)

- Step 1.1: Add `BlockNumber` to `ContractClient` interface; verify `simulated.Client` + `*ethclient.Client` satisfy it.
- Step 1.2: Failing test `TestProvider_ScanNewMints_Paginated`. Verify FAIL.
- Step 1.3: Refactor `Provider.ScanNewMints` to paginate. Verify PASS.
- Step 1.4: Failing test `TestPersonas_UpdatePromptText`. Verify FAIL.
- Step 1.5: Add `Repo.UpdatePromptText`. Verify PASS.
- Step 1.6: Extend `queue.PersonaRegistry` interface with `UpdatePromptText`. Update stubPersonas + inMemoryRegistry to satisfy it. Run `go build ./...` — confirm no other implementers break.
- Step 1.7: Failing test `TestReconcile_BackfillPrompts_FetchesAndUpdates` + `_StorageFailureLogsAndContinues`. Verify FAIL.
- Step 1.8: Add `reconcileBackfillPrompts` + wire as 4th pass in `personasReconcile`. Verify PASS.
- Step 1.9: Add `slog.Warn` to `defaultOwnerAddr`. Run existing reconcile tests — green.
- Step 1.10: Full regression both modules. Commit + tag `m7g-done`.

## §6 — Out of scope

- Persistence of last-scanned-block (M7-G keeps it stateless; future M7-H+ if scan time becomes a problem).
- Backfill from a non-0G source (e.g., GitHub raw URL for legacy iNFT URIs from M7-D.1 that point at GitHub). Acceptable: those personas are defaults seeded with prompts already in SQLite.
- Re-fetch on stale prompts (we don't detect drift; one-shot fill).
- Burn / re-mint of orphaned tokens.

## §7 — Risks

1. **`BlockNumber` not on existing `ContractClient` interface** — adding it to the interface breaks any custom test fake. Phase 1.1 catches this; the fix is one line per fake.
2. **Pagination cost** — at ~30M blocks Galileo and 1000-block chunks, that's ~30k chunks. With `iter.Next()` short-circuiting on no events, ~5ms each → ~2.5min worst case on cold cache. In practice public RPCs cache hot, and our wallet's mint history concentrates in recent blocks — actual scan likely <30s. If too slow on demo day, ship M7-G+ with last-scanned-block persistence.
3. **Auto-backfill 0G dependency** — if 0G KV is fully down at boot, all imported personas stay empty. Same M7-F.6 fallback story still applies (production /task fetch falls back to 0G), but if 0G stays down personas with no SQLite prompt fail. Acceptable — this is the SAME failure mode as M7-F.6 §risks #1, just bounded to first-task-after-import.
4. **Test stub propagation** — every PersonaRegistry implementer in the codebase needs the new method. Phase 1.6's `go build ./...` step catches all gaps.

## §8 — Acceptance criteria

1. `go build ./...` green; `go test -race -count=1 ./...` green for both modules.
2. Booting orchestrator with current production state shows NO "Block range is too large" warning.
3. After one boot with M7-G code AND 0G KV reachable, `sqlite3 pi-agent.db "SELECT name, length(prompt_text) FROM personas WHERE name = 'rustacean'"` returns a non-zero length. If 0G KV is down at boot, the criterion is deferred to the next successful boot (best-effort per §7 risk #3).
4. `/task --persona=rustacean ...` works end-to-end (sanity verify post-boot).
5. With `PI_ZG_PRIVATE_KEY` set to a malformed value, boot logs include `personas: defaultOwnerAddr parse failed`.
