# M7-F — `/persona-mint` + Custom Personas Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `/persona-mint <name> <prompt>`, `/task --persona=<name> <desc>`, and `/personas` Telegram commands. Custom personas mint as iNFT tokens on the existing `EraPersonaINFT` contract, register `<name>.vaibhav-era.eth` ENS subnames, store prompts on 0G Storage, and influence task execution by **prepending the persona's prompt to Pi's task description**.

**Architecture:** Five linear phases. Phase 1 implements `Provider.Mint` (currently `ErrNotImplemented`) + a 0G storage upload helper. Phase 2 adds the `personas` SQLite table + repo CRUD. Phase 3 ships the `/persona-mint` and `/personas` Telegram commands. Phase 4 adds the `--persona=<name>` flag to `/task`, plumbs `personaName` through `CreateTask` → queue → Pi description prefix, refactors the M7-E.2 hardcoded `ensFooter` labels list into a per-task parameter. Phase 5 wires boot reconcile (default seed + on-chain Transfer scan + ENS retry pass) and runs the live Telegram gate.

**Tech Stack:** Go 1.25, go-ethereum's `abigen` + `bind` + receipt log parsing; existing 0G Storage SDK plumbing from M7-B.1; existing iNFT contract at `0x33847c5500C2443E2f3BBf547d9b069B334c3D16` (0G Galileo, chainID 16602); existing ENS NameWrapper + PublicResolver on Sepolia; SQLite with goose migrations.

**Spec:** `docs/superpowers/specs/2026-04-27-m7f-persona-mint-design.md`. All §-references below point at the spec.

**Testing philosophy:** Strict TDD. Failing test first, run, verify FAIL, write minimal Go, run, verify PASS, commit. `go test -race -count=1 ./...` from repo root green at every commit. Live testnet gates per phase where applicable. `m7f_live`-tagged tests skip in CI; only run when env vars present.

**⚠ Architectural clarification vs spec §3.** The spec said custom personas use a memory namespace = persona name. **This plan does NOT implement that.** The coder slot is Pi-in-Docker (not a `brain.LLMPersona`); Pi has no memory-namespace plumbing today. M7-F implements **prompt-prefix behavior only** — the persona's system prompt is prepended to Pi's task `description`. iNFT `recordInvocation` against the persona's tokenID records that "this task ran under persona X" — that is the on-chain provenance. Per-persona evolving memory is deferred to M7-G or later (would require Pi-side or whole-coder-replacement work).

**Prerequisites (check before starting):**
- M7-E done (tag `m7e-done`).
- iNFT contract at `0x33847c5500C2443E2f3BBf547d9b069B334c3D16`; signer is contract owner. Verify: `cast call $PI_ZG_INFT_CONTRACT_ADDRESS 'owner()(address)' --rpc-url $PI_ZG_EVM_RPC` returns signer.
- ENS parent `vaibhav-era.eth` wrapped on Sepolia.
- `.env` populated with `PI_ZG_*`, `PI_ENS_*`, `PI_ZG_INFT_CONTRACT_ADDRESS`. No new env vars needed.
- Existing era + era-brain tests still pass.

---

## File Structure

```
era-brain/inft/zg_7857/zg_7857.go                                   MODIFY (Phase 1) — replace Mint stub with real impl + Transfer event parsing
era-brain/inft/zg_7857/zg_7857_test.go                              MODIFY (Phase 1) — add Mint unit tests
era-brain/inft/zg_7857/zg_7857_live_test.go                         MODIFY (Phase 1) — add live-mint test (zg_live tag)

era-brain/storage/zg_storage/                                       CREATE (Phase 1)
├── upload.go                                                       CREATE — UploadPrompt helper
├── upload_test.go                                                  CREATE — unit + live tests

internal/db/migrations/0011_personas.sql                            CREATE (Phase 2) — personas table
internal/db/migrations/0012_tasks_persona_name.sql                  CREATE (Phase 4) — adds tasks.persona_name
internal/db/personas.go                                             CREATE (Phase 2) — InsertPersona, GetPersonaByName, ListPersonas
internal/db/personas_test.go                                        CREATE (Phase 2)

internal/queue/queue.go                                             MODIFY (Phase 2,3,4) — PersonaRegistry interface, Queue.personas, MintPersona, CompletedArgs.PersonaLabels, CreateTask sig change
internal/queue/queue_run_test.go                                    MODIFY (Phase 4) — stubPersonas + persona-task integration tests

internal/telegram/handler.go                                        MODIFY (Phase 3,4) — Ops.MintPersona, Ops.ListPersonas, parsePersonaFlag, /persona-mint and /personas routing, CreateTask sig change
internal/telegram/handler_test.go                                   MODIFY (Phase 3,4) — handler tests

cmd/orchestrator/main.go                                            MODIFY (Phase 4,5) — ensFooter signature change, personasReconcile() boot helper, default seed + Transfer scan + ENS retry passes
cmd/orchestrator/notifier_ens_test.go                               MODIFY (Phase 4) — pass labels arg to ensFooter; new TestEnsFooter_CustomPersonaLabels test
cmd/orchestrator/personas_reconcile.go                              CREATE (Phase 5) — personasReconcile + sub-helpers (default seed, transfer scan, ens retry)
cmd/orchestrator/personas_reconcile_test.go                         CREATE (Phase 5) — unit tests for the sub-helpers
```

No changes to: `era-brain/identity/ens/`, `internal/swarm/`, `contracts/src/`, `era-brain/brain/`, `era-brain/memory/` (other than reusing existing storage SDK plumbing).

---

## Phase 0: Pre-flight checks (no commit)

- [ ] **Step 0.1: Verify contract ownership**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
cast call $PI_ZG_INFT_CONTRACT_ADDRESS 'owner()(address)' --rpc-url $PI_ZG_EVM_RPC
cast wallet address $PI_ZG_PRIVATE_KEY
```

Expected: both return the same address (`0x6DB1508Deeb45E0194d4716349622806672f6Ac2`). If they differ, STOP — only the owner can mint.

- [ ] **Step 0.2: Verify ParseTransfer helper exists in bindings**

```bash
grep -n "func.*Filterer.*ParseTransfer" era-brain/inft/zg_7857/bindings/era_persona_inft.go | head
```

Expected: hit. abigen always names it `<Type>Filterer.ParseTransfer`. If missing, the bindings need regenerating with `--bin` (already done in M7-D.2; verify).

- [ ] **Step 0.3: Inspect existing 0G storage upload pattern**

```bash
grep -rn "UploadPrompt\|StoreContent\|store.*Append\|zg.*Upload" era-brain/memory/zg_log/ era-brain/memory/zg_kv/ 2>/dev/null | head -20
ls era-brain/memory/zg_log/ era-brain/memory/zg_kv/
```

Note the existing 0G storage client construction (likely in `zg_log` or `zg_kv` — they share the underlying SDK client). Phase 1's `UploadPrompt` reuses this client; do NOT construct a new SDK instance.

---

## Phase 1: `Provider.Mint` impl + 0G storage upload helper

**Files:**
- Modify: `era-brain/inft/zg_7857/zg_7857.go` (replace Mint stub)
- Modify: `era-brain/inft/zg_7857/zg_7857_test.go` (add Mint unit tests)
- Modify: `era-brain/inft/zg_7857/zg_7857_live_test.go` (add live mint test)
- Create: `era-brain/storage/zg_storage/upload.go`
- Create: `era-brain/storage/zg_storage/upload_test.go`

### 1A: Failing unit test for Mint

- [ ] **Step 1.1a: Add `NewWithClient` constructor to `zg_7857.go`**

The existing `zg_7857.go` only has `New(cfg)` (which dials via `ethclient.Dial`). Tests against `simulated.Backend` need to inject a pre-built client. Mirror the M7-E.1 ens.go pattern by adding:

```go
// ContractClient is the subset of *ethclient.Client + simulated.Client we need.
// Both satisfy bind.ContractBackend and bind.DeployBackend; in tests we pass a
// simulated.Client, in prod a *ethclient.Client.
type ContractClient interface {
	bind.ContractBackend
	bind.DeployBackend
}

// NewWithClient is a test entry point: skip dial, use the provided client.
// Production callers use New.
func NewWithClient(cfg Config, client ContractClient) (*Provider, error) {
	return newWithBackend(cfg, client)
}
```

Refactor `New` to delegate to a private `newWithBackend(cfg, client)` (same shape as ens.go's `newWithBackend`). The `Provider` struct's existing `client *ethclient.Client` field needs to become `ContractClient`-typed (or split into `client ContractClient` + `dialedClient *ethclient.Client` like ens.go) so simulated clients work. Read `era-brain/identity/ens/ens.go:55-78` for the exact shape.

Run `go build ./inft/zg_7857/...` to confirm refactor compiles before continuing. NO commit yet — this is part of Step 1.1.

- [ ] **Step 1.1b: Append failing tests to `zg_7857_test.go`**

`deployContractOnSim` already returns `*ecdsa.PrivateKey` as the 4th value — verified at `era-brain/inft/zg_7857/zg_7857_test.go:20`. Use it directly. No `privKeyOf` helper needed.

```go
func TestProvider_Mint_HappyPath(t *testing.T) {
	backend, contract, auth, key, addr := deployContractOnSim(t)
	_ = contract

	keyHex := common.Bytes2Hex(crypto.FromECDSA(key))
	p, err := zg_7857.NewWithClient(zg_7857.Config{
		ContractAddress: addr.Hex(),
		PrivateKey:      keyHex,
		ChainID:         1337,
	}, backend.Client())
	require.NoError(t, err)
	t.Cleanup(p.Close)

	persona, err := p.Mint(context.Background(), "rustacean", "ipfs://prompt-blob")
	require.NoError(t, err)
	backend.Commit() // safety: ensure mint receipt is in a sealed block
	require.NotEmpty(t, persona.TokenID, "token ID should be populated from Transfer event")
	require.Equal(t, "rustacean", persona.Name)
	require.Equal(t, "ipfs://prompt-blob", persona.SystemPromptURI)
	require.Equal(t, auth.From.Hex(), persona.Owner)
	require.NotEmpty(t, persona.MintTxHash, "tx hash should be populated for DM rendering")

	tokenID, ok := new(big.Int).SetString(persona.TokenID, 10)
	require.True(t, ok)
	owner, err := contract.OwnerOf(&bind.CallOpts{}, tokenID)
	require.NoError(t, err)
	require.Equal(t, auth.From, owner)

	uri, err := contract.TokenURI(&bind.CallOpts{}, tokenID)
	require.NoError(t, err)
	require.Equal(t, "ipfs://prompt-blob", uri)
}
```

(Note: existing `deployContractOnSim` already mints token #0 to deployer in its body. The first user-driven Mint() in this test produces token #1.)

- [ ] **Step 1.2: Run, verify FAIL**

```bash
cd era-brain
go test ./inft/zg_7857/ -run TestProvider_Mint -v 2>&1 | head -30
```

Expected: FAIL with assertions about `persona.TokenID` being empty (Mint still returns `inft.Persona{}, ErrNotImplemented`).

### 1B: Implement Mint

- [ ] **Step 1.3: Replace Mint stub in `zg_7857.go`**

Find the existing `Mint` method (returns `ErrNotImplemented`) and replace with:

```go
// Mint creates a new persona iNFT token, owned by the orchestrator wallet.
// systemPromptURI becomes the contract's tokenURI for the new token.
// Returns the auto-incremented token ID + persona metadata.
//
// Calls EraPersonaINFT.mint(address to, string memory uri) — onlyOwner.
// Parses the Transfer(from=0x0, to=signer, tokenId) event from the receipt
// to extract the auto-incremented token ID.
func (p *Provider) Mint(ctx context.Context, name, systemPromptURI string) (inft.Persona, error) {
	auth := *p.auth
	auth.Context = ctx

	tx, err := p.contract.Mint(&auth, p.auth.From, systemPromptURI)
	if err != nil {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint tx: %w", err)
	}

	rc, err := bind.WaitMined(ctx, p.client, tx)
	if err != nil {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint waitmined: %w", err)
	}
	if rc.Status != types.ReceiptStatusSuccessful {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint reverted: txHash=%s", tx.Hash().Hex())
	}

	// Find Transfer(from=0x0, to=signer) in receipt logs.
	var tokenID *big.Int
	for _, log := range rc.Logs {
		ev, perr := p.contract.ParseTransfer(*log)
		if perr != nil {
			continue // not a Transfer event
		}
		zero := common.Address{}
		if ev.From == zero && ev.To == p.auth.From {
			tokenID = ev.TokenId
			break
		}
	}
	if tokenID == nil {
		return inft.Persona{}, fmt.Errorf("zg_7857 mint: no matching Transfer event in receipt; txHash=%s", tx.Hash().Hex())
	}

	return inft.Persona{
		TokenID:         tokenID.String(),
		Name:            name,
		SystemPromptURI: systemPromptURI,
		Owner:           p.auth.From.Hex(),
		MintTxHash:      tx.Hash().Hex(),
	}, nil
}
```

Add the `types` import at the top of `zg_7857.go` (`"github.com/ethereum/go-ethereum/core/types"`) if not already present.

**Required struct extension.** Read existing `inft.Persona`:
```bash
grep -n "type Persona struct" -A 10 era-brain/inft/provider.go
```

The existing struct (M7-A.2) has `Name`, `TokenID`, `SystemPromptURI`. Phase 1 must extend it with `Owner string` and `MintTxHash string`. Edit `era-brain/inft/provider.go`:

```go
type Persona struct {
	TokenID         string
	Name            string
	SystemPromptURI string
	Owner           string  // NEW (M7-F.1) — 0x... wallet that owns the token
	MintTxHash      string  // NEW (M7-F.1) — for DM-rendering chainscan link
}
```

If existing fields differ, use the existing names — but the two new fields MUST be added since Phase 3's `Queue.MintPersona` and the Telegram DM both consume them.

- [ ] **Step 1.4: Run unit tests, verify PASS**

```bash
cd era-brain
go test -race ./inft/zg_7857/...
```

Expected: 4 prior tests still pass + 2 new Mint tests pass. If Mint test fails:
- Empty TokenID → ParseTransfer didn't match; check the contract's constructor (does it pre-mint?), check the Transfer event signature, check `From == 0x0` predicate.
- "no matching Transfer event" → walk through `rc.Logs` manually to debug.

### 1C: Live mint integration test

- [ ] **Step 1.5: Append live test to `zg_7857_live_test.go`**

⚠ **Cost note:** This test mints a real iNFT token on the testnet contract. Token cannot be deleted. To avoid cluttering the contract during repeated CI runs, use a unique URI marker per run (timestamp). Skip the test by default — only run during this phase's gate.

```go
//go:build zg_live

func TestProvider_LiveMint(t *testing.T) {
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	contractAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
	privKey := os.Getenv("PI_ZG_PRIVATE_KEY")
	if rpc == "" || contractAddr == "" || privKey == "" {
		t.Skip("0G envs required")
	}

	p, err := zg_7857.New(zg_7857.Config{
		ContractAddress: contractAddr,
		EVMRPCURL:       rpc,
		PrivateKey:      privKey,
		ChainID:         16602,
	})
	require.NoError(t, err)
	t.Cleanup(p.Close)

	uri := "ipfs://m7f-live-test-" + strconv.FormatInt(time.Now().Unix(), 10)
	persona, err := p.Mint(context.Background(), "live-test", uri)
	require.NoError(t, err)
	require.NotEmpty(t, persona.TokenID)
	require.Equal(t, uri, persona.SystemPromptURI)

	t.Logf("minted live token #%s — uri=%s", persona.TokenID, uri)
}
```

- [ ] **Step 1.6: Run live mint test (USES REAL ZG ~0.001)**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era/era-brain
set -a; source ../.env; set +a
go test -tags zg_live -v -run TestProvider_LiveMint ./inft/zg_7857/... 2>&1 | tee /tmp/m7f-mint-live.log
grep -E "^--- PASS: TestProvider_LiveMint" /tmp/m7f-mint-live.log
```

Expected: PASS line printed; final grep exits 0. Logs the new token ID.

If FAIL:
- "execution reverted" + "Ownable: caller is not the owner" → wallet is not contract owner. Re-run pre-flight Step 0.1.
- "insufficient funds" → faucet up.
- "no matching Transfer event" → live contract emits a different event shape than mock. Inspect `rc.Logs` manually.

### 1D: 0G storage `UploadPrompt` helper

- [ ] **Step 1.7: Read existing 0G storage SDK usage**

```bash
grep -rn "client\.\|storage\.\|kv.\|Append\|StoreContent" era-brain/memory/zg_log/*.go era-brain/memory/zg_kv/*.go | head -30
```

Identify the SDK client construction + any existing upload-blob method. The new helper reuses or thinly wraps it.

- [ ] **Step 1.8: Write failing test for `UploadPrompt`**

`era-brain/storage/zg_storage/upload_test.go`:

```go
package zg_storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/storage/zg_storage"
)

func TestUploadPrompt_ReturnsURI(t *testing.T) {
	cfg := zg_storage.Config{
		// fill in test or mock config — see helper used by zg_log tests
	}
	c, err := zg_storage.New(cfg)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	uri, err := c.UploadPrompt(context.Background(), "You only write idiomatic Rust.")
	require.NoError(t, err)
	require.NotEmpty(t, uri)
	require.Contains(t, uri, "0g") // shape check; adjust to actual URI shape
}

func TestUploadPrompt_Idempotent(t *testing.T) {
	// Same content uploaded twice should return the same URI.
	cfg := zg_storage.Config{ /* ... */ }
	c, err := zg_storage.New(cfg)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	uri1, err := c.UploadPrompt(context.Background(), "deterministic content")
	require.NoError(t, err)
	uri2, err := c.UploadPrompt(context.Background(), "deterministic content")
	require.NoError(t, err)
	require.Equal(t, uri1, uri2, "idempotent on content hash")
}
```

⚠ **Implementation reality check:** The existing 0G storage SDK may not expose a "blob upload" primitive — the M7-B work used `zg_log` (append-only log) and `zg_kv` (key-value). For `UploadPrompt`, the simplest path is:
1. **Reuse `zg_kv`**: store under key = `sha256(content)`, value = content. URI = `zg-kv://<keyhex>`. Idempotent by construction.
2. **Reuse `zg_log`**: append entry, return the entry's tx hash as URI. NOT idempotent — duplicates content per call.

**Default to option 1 (zg_kv).** If `zg_kv.Provider` already exists with a `Set(key, value)` method, the helper is ~5 lines.

- [ ] **Step 1.9: Run, verify FAIL**

```bash
cd era-brain
go test ./storage/zg_storage/... -v 2>&1 | head -30
```

Expected: build failure (`no such package`). Exit non-zero.

- [ ] **Step 1.10: Implement `UploadPrompt`**

`era-brain/storage/zg_storage/upload.go`:

```go
// Package zg_storage provides simple blob upload + retrieval for persona
// system prompts on 0G Storage. Wraps the existing zg_kv plumbing from M7-B.
//
// Idempotent: uploading the same content twice returns the same URI.
package zg_storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
)

type Config struct {
	// Embed or mirror the zg_kv config; reuse the SDK client there.
	KVConfig zg_kv.Config
}

type Client struct {
	kv *zg_kv.Provider
}

func New(cfg Config) (*Client, error) {
	kv, err := zg_kv.New(cfg.KVConfig)
	if err != nil {
		return nil, fmt.Errorf("zg_storage: %w", err)
	}
	return &Client{kv: kv}, nil
}

func (c *Client) Close() {
	if c.kv != nil {
		c.kv.Close()
	}
}

// UploadPrompt stores content under key = sha256(content) and returns a URI
// of shape "zg://<keyhex>" pointing at it. Idempotent — same content → same URI.
func (c *Client) UploadPrompt(ctx context.Context, content string) (string, error) {
	hash := sha256.Sum256([]byte(content))
	key := hex.EncodeToString(hash[:])
	if err := c.kv.Set(ctx, "personas/prompts", key, []byte(content)); err != nil {
		return "", fmt.Errorf("zg_storage upload: %w", err)
	}
	return "zg://" + key, nil
}

// FetchPrompt retrieves a previously uploaded prompt by URI.
func (c *Client) FetchPrompt(ctx context.Context, uri string) (string, error) {
	const prefix = "zg://"
	if len(uri) <= len(prefix) || uri[:len(prefix)] != prefix {
		return "", fmt.Errorf("zg_storage: invalid URI %q", uri)
	}
	key := uri[len(prefix):]
	v, err := c.kv.Get(ctx, "personas/prompts", key)
	if err != nil {
		return "", fmt.Errorf("zg_storage fetch: %w", err)
	}
	return string(v), nil
}
```

⚠ **Method signatures depend on `zg_kv.Provider` actual API.** Read it first:
```bash
grep -n "func.*Provider.*Set\|func.*Provider.*Get" era-brain/memory/zg_kv/*.go | head
```

Adapt the namespace argument and method names to match. If `zg_kv.Set` takes different args (e.g., no namespace), drop accordingly.

- [ ] **Step 1.11: Run unit tests, verify PASS**

```bash
cd era-brain
go test -race ./storage/zg_storage/...
```

Expected: 2 tests pass.

- [ ] **Step 1.12: Run all era + era-brain tests + vet (no regression)**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go vet ./...
go test -race -count=1 ./...
cd era-brain
go vet ./...
go test -race -count=1 ./...
```

Both green.

- [ ] **Step 1.13: Commit**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
git add era-brain/inft/zg_7857/ era-brain/storage/
git commit -m "phase(M7-F.1): zg_7857.Mint impl + zg_storage.UploadPrompt — Transfer event parsing for token ID; idempotent prompt blob storage on 0G"
git tag m7f-1-mint-and-upload
```

NO `Co-Authored-By` trailer per `~/.claude/CLAUDE.md`. NO `--author` flag.

---

## Phase 2: `personas` SQLite migration + repo CRUD

**Files:**
- Create: `internal/db/migrations/0011_personas.sql`
- Create: `internal/db/personas.go`
- Create: `internal/db/personas_test.go`
- Modify: `internal/queue/queue.go` — add `PersonaRegistry` interface + sentinel errors

### 2A: Failing test for the repo

- [ ] **Step 2.1: Read existing migration filename pattern**

```bash
ls internal/db/migrations/ | tail
```

Confirm migrations are numbered `NNNN_name.sql` (goose). Latest should be `0009_*.sql` or `0010_*.sql`. Use the next number for personas (`0011`).

- [ ] **Step 2.2: Write the migration file**

`internal/db/migrations/0011_personas.sql`:

```sql
-- +goose Up
CREATE TABLE personas (
    token_id          TEXT PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    owner_addr        TEXT NOT NULL,
    system_prompt_uri TEXT NOT NULL,
    ens_subname       TEXT,
    description       TEXT,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX personas_name_idx ON personas(name);

-- +goose Down
DROP INDEX IF EXISTS personas_name_idx;
DROP TABLE IF EXISTS personas;
```

(Verify the file format matches existing migrations — `+goose Up`/`+goose Down` markers, no leading whitespace.)

- [ ] **Step 2.3: Write failing test**

`internal/db/personas_test.go`:

```go
package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/db"
	"github.com/vaibhav0806/era/internal/queue"
)

func TestPersonas_InsertAndGet(t *testing.T) {
	repo := newTestRepo(t) // helper from existing test files

	p := queue.Persona{
		TokenID:         "3",
		Name:            "rustacean",
		OwnerAddr:       "0x6DB1508Deeb45E0194d4716349622806672f6Ac2",
		SystemPromptURI: "zg://abc",
		ENSSubname:      "rustacean.vaibhav-era.eth",
		Description:     "You only write idiomatic Rust",
	}
	require.NoError(t, repo.InsertPersona(context.Background(), p))

	got, err := repo.GetPersonaByName(context.Background(), "rustacean")
	require.NoError(t, err)
	require.Equal(t, p.TokenID, got.TokenID)
	require.Equal(t, p.Name, got.Name)
	require.Equal(t, p.SystemPromptURI, got.SystemPromptURI)
	require.NotZero(t, got.CreatedAt)
}

func TestPersonas_DuplicateName(t *testing.T) {
	repo := newTestRepo(t)
	p := queue.Persona{TokenID: "3", Name: "rustacean", OwnerAddr: "0x...", SystemPromptURI: "zg://x"}
	require.NoError(t, repo.InsertPersona(context.Background(), p))

	p2 := queue.Persona{TokenID: "4", Name: "rustacean", OwnerAddr: "0x...", SystemPromptURI: "zg://y"}
	err := repo.InsertPersona(context.Background(), p2)
	require.ErrorIs(t, err, queue.ErrPersonaNameTaken)
}

func TestPersonas_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.GetPersonaByName(context.Background(), "nope")
	require.ErrorIs(t, err, queue.ErrPersonaNotFound)
}

func TestPersonas_List(t *testing.T) {
	repo := newTestRepo(t)
	_ = repo.InsertPersona(context.Background(), queue.Persona{TokenID: "0", Name: "planner", OwnerAddr: "0x", SystemPromptURI: "u0"})
	_ = repo.InsertPersona(context.Background(), queue.Persona{TokenID: "3", Name: "rustacean", OwnerAddr: "0x", SystemPromptURI: "u3"})

	list, err := repo.ListPersonas(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)
}
```

The test references `newTestRepo(t)` — find the existing helper:
```bash
grep -n "func newTestRepo\|func setupTest" internal/db/*_test.go | head
```
Reuse it. If it doesn't exist or has a different name, adapt.

- [ ] **Step 2.4: Run, verify FAIL**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go test ./internal/db/ -run TestPersonas -count=1 -v 2>&1 | head -30
```

Expected: build failure mentioning `undefined: queue.Persona`, `undefined: db.InsertPersona`, etc. Exit non-zero.

### 2B: Implement Persona type + repo methods

- [ ] **Step 2.5: Add Persona type + sentinel errors to `internal/queue/queue.go`**

Near the existing `INFTProvider` / `ENSResolver` interfaces in `queue.go`:

```go
// Persona represents a persona iNFT registry row. Used for /persona-mint, /personas,
// and /task --persona=<name> routing.
type Persona struct {
	TokenID         string
	Name            string
	OwnerAddr       string
	SystemPromptURI string
	ENSSubname      string
	Description     string
	CreatedAt       time.Time
}

// PersonaRegistry is the queue's view of the personas table.
type PersonaRegistry interface {
	Lookup(ctx context.Context, name string) (Persona, error)  // returns ErrPersonaNotFound
	List(ctx context.Context) ([]Persona, error)
	Insert(ctx context.Context, p Persona) error              // returns ErrPersonaNameTaken on duplicate
	UpdateENSSubname(ctx context.Context, name, subname string) error // for Phase 5 ENS reconcile
}

var (
	ErrPersonaNotFound  = errors.New("persona not found")
	ErrPersonaNameTaken = errors.New("persona name already taken")
)
```

If `time` isn't imported in queue.go, add it.

- [ ] **Step 2.6: Implement repo methods in `internal/db/personas.go`**

```go
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/vaibhav0806/era/internal/queue"
)

func (r *Repo) InsertPersona(ctx context.Context, p queue.Persona) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO personas (token_id, name, owner_addr, system_prompt_uri, ens_subname, description)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.TokenID, p.Name, p.OwnerAddr, p.SystemPromptURI,
		nullableString(p.ENSSubname), nullableString(p.Description))
	if err != nil {
		if isUniqueViolation(err, "personas.name") {
			return queue.ErrPersonaNameTaken
		}
		return fmt.Errorf("insert persona: %w", err)
	}
	return nil
}

func (r *Repo) GetPersonaByName(ctx context.Context, name string) (queue.Persona, error) {
	var p queue.Persona
	var ensSubname, description sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT token_id, name, owner_addr, system_prompt_uri, ens_subname, description, created_at
		FROM personas WHERE name = ?`, name).
		Scan(&p.TokenID, &p.Name, &p.OwnerAddr, &p.SystemPromptURI, &ensSubname, &description, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return queue.Persona{}, queue.ErrPersonaNotFound
	}
	if err != nil {
		return queue.Persona{}, fmt.Errorf("get persona: %w", err)
	}
	p.ENSSubname = ensSubname.String
	p.Description = description.String
	return p, nil
}

func (r *Repo) UpdatePersonaENSSubname(ctx context.Context, name, subname string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE personas SET ens_subname = ? WHERE name = ?`, subname, name)
	if err != nil {
		return fmt.Errorf("update persona ens_subname: %w", err)
	}
	return nil
}

func (r *Repo) ListPersonas(ctx context.Context) ([]queue.Persona, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT token_id, name, owner_addr, system_prompt_uri, ens_subname, description, created_at
		FROM personas ORDER BY CAST(token_id AS INTEGER)`)
	if err != nil {
		return nil, fmt.Errorf("list personas: %w", err)
	}
	defer rows.Close()
	var out []queue.Persona
	for rows.Next() {
		var p queue.Persona
		var ensSubname, description sql.NullString
		if err := rows.Scan(&p.TokenID, &p.Name, &p.OwnerAddr, &p.SystemPromptURI, &ensSubname, &description, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.ENSSubname = ensSubname.String
		p.Description = description.String
		out = append(out, p)
	}
	return out, rows.Err()
}

// nullableString returns sql.NullString for the given s — empty string maps to NULL.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// isUniqueViolation returns true when err is a SQLite UNIQUE constraint failure
// on the named index/column. Pattern: "UNIQUE constraint failed: personas.name".
func isUniqueViolation(err error, columnHint string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		strings.Contains(msg, columnHint)
}
```

If `Repo.db` field name differs (e.g., `r.conn`), adapt by reading existing repo files.

If `isUniqueViolation` already exists in `internal/db/` (e.g., for tasks), reuse it.

Wire the repo into the `queue.PersonaRegistry` interface — the queue's repo wrapper or a thin adapter (depends on existing pattern). One clean way: `Repo` implements `queue.PersonaRegistry` directly via `Lookup = GetPersonaByName`, `Insert = InsertPersona`, `List = ListPersonas`. Add small adapter functions:

```go
func (r *Repo) Lookup(ctx context.Context, name string) (queue.Persona, error) {
	return r.GetPersonaByName(ctx, name)
}
func (r *Repo) Insert(ctx context.Context, p queue.Persona) error {
	return r.InsertPersona(ctx, p)
}
func (r *Repo) List(ctx context.Context) ([]queue.Persona, error) {
	return r.ListPersonas(ctx)
}
func (r *Repo) UpdateENSSubname(ctx context.Context, name, subname string) error {
	return r.UpdatePersonaENSSubname(ctx, name, subname)
}
```

Verify the assignment compiles by adding to queue.go:
```go
var _ PersonaRegistry = (*db.Repo)(nil) // compile-time check
```

(This requires queue.go to import db, which it does already — confirm.)

- [ ] **Step 2.7: Run unit tests, verify PASS**

```bash
go test -race ./internal/db/ -run TestPersonas
```

Expected: 4 tests pass.

- [ ] **Step 2.8: Run all tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

Both green.

- [ ] **Step 2.9: Commit**

```bash
git add internal/db/ internal/queue/queue.go
git commit -m "phase(M7-F.2): personas SQLite migration + repo CRUD — InsertPersona, GetPersonaByName, ListPersonas; queue.PersonaRegistry interface + sentinel errors"
git tag m7f-2-registry
```

---

## Phase 3: Telegram `/persona-mint` + `/personas` commands

**Files:**
- Modify: `internal/queue/queue.go` — `Queue.MintPersona`, `Queue.ListPersonas`
- Modify: `internal/telegram/handler.go` — Ops extension, command routing
- Modify: `internal/telegram/handler_test.go`

### 3A: Failing handler tests

- [ ] **Step 3.1: Read existing Ops interface + handler test pattern**

```bash
grep -n "type Ops interface\|MintPersona\|ListPersonas\|HandleApproval" internal/telegram/handler.go | head
```

Note current `Ops` methods. Phase 3 adds two: `MintPersona`, `ListPersonas`.

- [ ] **Step 3.2: Write failing handler tests**

Append to `internal/telegram/handler_test.go`:

```go
type stubOps struct {
	// extend the existing test stub — add fields below if not present
	mintCalled       bool
	mintName         string
	mintPrompt       string
	mintErr          error
	mintResult       PersonaMintResult
	listResult       []queue.Persona
	listErr          error
	// ... existing stub fields
}

func (s *stubOps) MintPersona(ctx context.Context, name, prompt string) (PersonaMintResult, error) {
	s.mintCalled = true
	s.mintName = name
	s.mintPrompt = prompt
	return s.mintResult, s.mintErr
}
func (s *stubOps) ListPersonas(ctx context.Context) ([]queue.Persona, error) {
	return s.listResult, s.listErr
}

func TestHandle_PersonaMint_Success(t *testing.T) {
	ops := &stubOps{
		mintResult: PersonaMintResult{
			TokenID:         "3",
			MintTxHash:      "0xabc",
			ENSSubname:      "rustacean.vaibhav-era.eth",
			SystemPromptURI: "zg://hash",
		},
	}
	client, h := newTestHandler(t, ops)

	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1, Text: "/persona-mint rustacean You only write idiomatic Rust code. Never compromise on memory safety or borrow-checker correctness."}))

	require.True(t, ops.mintCalled)
	require.Equal(t, "rustacean", ops.mintName)
	require.Contains(t, ops.mintPrompt, "idiomatic Rust")
	// DM should mention token #3 + chainscan + ENS link
	last := client.LastMessage()
	require.Contains(t, last, "token #3")
	require.Contains(t, last, "rustacean.vaibhav-era.eth")
}

func TestHandle_PersonaMint_InvalidName_NoChainCalls(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"uppercase",       "/persona-mint RustLover prompt text long enough"},
		{"too_short",       "/persona-mint xy prompt text long enough"},
		{"reserved_planner","/persona-mint planner prompt text long enough"},
		{"reserved_coder",  "/persona-mint coder prompt text long enough"},
		{"reserved_reviewer","/persona-mint reviewer prompt text long enough"},
		{"empty_prompt",    "/persona-mint rustacean"},
		{"prompt_too_short","/persona-mint rustacean tiny"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ops := &stubOps{}
			_, h := newTestHandler(t, ops)
			require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: c.text}))
			require.False(t, ops.mintCalled, "should not call MintPersona on invalid input")
		})
	}
}

func TestHandle_PersonaMint_DuplicateName(t *testing.T) {
	ops := &stubOps{mintErr: queue.ErrPersonaNameTaken}
	client, h := newTestHandler(t, ops)
	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1, Text: "/persona-mint rustacean You only write idiomatic Rust code, no exceptions whatsoever."}))
	require.Contains(t, client.LastMessage(), "already taken")
}

func TestHandle_Personas_Lists(t *testing.T) {
	ops := &stubOps{
		listResult: []queue.Persona{
			{TokenID: "0", Name: "planner", ENSSubname: "planner.vaibhav-era.eth", Description: "default planner"},
			{TokenID: "3", Name: "rustacean", ENSSubname: "rustacean.vaibhav-era.eth", Description: "Rust-only persona"},
		},
	}
	client, h := newTestHandler(t, ops)
	require.NoError(t, h.Handle(context.Background(), Update{ChatID: 1, Text: "/personas"}))
	body := client.LastMessage()
	require.Contains(t, body, "planner.vaibhav-era.eth")
	require.Contains(t, body, "rustacean.vaibhav-era.eth")
	require.Contains(t, body, "#0")
	require.Contains(t, body, "#3")
}
```

The stub names + helpers (`newTestHandler`, `client.LastMessage`) must match the existing pattern. Read `handler_test.go` first; adapt struct + test names accordingly.

- [ ] **Step 3.3: Run, verify FAIL**

```bash
go test ./internal/telegram/ -run "TestHandle_Persona" -count=1 -v 2>&1 | head -30
```

Expected: build failure (`undefined: PersonaMintResult`, `undefined: stubOps.MintPersona`, etc.). Exit non-zero.

### 3B: Wire queue dependencies BEFORE the orchestration step

- [ ] **Step 3.4a: Extend `INFTProvider` interface with `Mint`**

In `internal/queue/queue.go`, modify the existing M7-D.2 interface:

```go
// INFTProvider is the queue's view of the iNFT registry.
type INFTProvider interface {
	RecordInvocation(ctx context.Context, tokenID, receiptHashHex string) error
	Mint(ctx context.Context, name, systemPromptURI string) (inft.Persona, error)  // NEW (M7-F.3)
}
```

Update existing `stubINFT` in `internal/queue/queue_run_test.go` (around line 888-905) to also implement `Mint`:

```go
func (s *stubINFT) Mint(_ context.Context, _, _ string) (inft.Persona, error) {
	// queue_run_test doesn't exercise mint; existing tests don't care.
	return inft.Persona{}, nil
}
```

Add the `inft` import to the test file if not present.

`*zg_7857.Provider` already implements both methods after Phase 1, so production wiring `q.SetINFT(prov)` keeps working.

Run `go vet ./...` + `go test ./internal/queue/... -count=1` to confirm the existing M7-D.2 tests still pass with the extended interface.

- [ ] **Step 3.4b: Add `ENSResolver` interface + `Queue.ens` field + setter**

`Queue.MintPersona` (next step) needs to call ENS write operations. The notifier already has its own `ENSResolver` interface (in `cmd/orchestrator/main.go`) but that one is read-only. Define a queue-side write interface:

```go
// ENSWriter is the queue's view of the ENS provider — adds + reads subnames.
// Implemented by *ens.Provider after M7-E.1.
type ENSWriter interface {
	EnsureSubname(ctx context.Context, label string) error
	SetTextRecord(ctx context.Context, label, key, value string) error
	ParentName() string
}
```

Add field + setter on `Queue`:

```go
type Queue struct {
	// ... existing fields ...
	ensWriter ENSWriter // may be nil
}

func (q *Queue) SetENSWriter(w ENSWriter) { q.ensWriter = w }
```

Production wiring (Phase 5) calls `q.SetENSWriter(ensProv)` — `*ens.Provider` already implements all three methods after M7-E.1, so it satisfies `ENSWriter` implicitly.

Don't conflate this with the notifier's `ENSResolver` (read-only, used by ensFooter). Both interfaces co-exist; the same `*ens.Provider` instance satisfies both.

- [ ] **Step 3.4c: Extract `syncPersonaENSRecords` helper**

Create `internal/queue/persona_ens.go`:

```go
package queue

import (
	"context"
	"fmt"
)

// SyncPersonaENSRecords runs EnsureSubname + 4 setText for a single persona.
// Used by both Queue.MintPersona and the boot-time ENS reconcile pass (M7-F.5).
// Errors are returned to the caller; logging is the caller's responsibility.
//
// Exported (capital S) so the orchestrator's reconcile pass can call it
// without copy-pasting the body.
func SyncPersonaENSRecords(ctx context.Context, ens ENSWriter, p Persona, inftAddr string) error {
	if ens == nil {
		return nil // ENS not wired — no-op
	}
	if err := ens.EnsureSubname(ctx, p.Name); err != nil {
		return fmt.Errorf("ensureSubname: %w", err)
	}
	desc := p.Description
	if len(desc) > 60 {
		desc = desc[:60]
	}
	for k, v := range map[string]string{
		"inft_addr":      inftAddr,
		"inft_token_id":  p.TokenID,
		"zg_storage_uri": p.SystemPromptURI,
		"description":    desc,
	} {
		if err := ens.SetTextRecord(ctx, p.Name, k, v); err != nil {
			return fmt.Errorf("setText %s: %w", k, err)
		}
	}
	return nil
}
```

This helper is shared between `Queue.MintPersona` (Phase 3) and `reconcileENS` (Phase 5).

### 3C: Implement Ops extension + handler routing

- [ ] **Step 3.4: Add `PersonaMintResult` + extend `Ops` interface**

In `internal/telegram/handler.go`, near the existing `Ops` interface:

```go
// PersonaMintResult bundles the persona-mint output for the Telegram DM.
type PersonaMintResult struct {
	TokenID         string
	MintTxHash      string
	ENSSubname      string  // empty if ENS not wired
	SystemPromptURI string
}

// Ops extension for M7-F.
//   MintPersona writes the iNFT + ENS + SQLite registry; returns mint result.
//   ListPersonas reads SQLite.
type Ops interface {
	// ... existing methods ...
	MintPersona(ctx context.Context, name, systemPrompt string) (PersonaMintResult, error)
	ListPersonas(ctx context.Context) ([]queue.Persona, error)
}
```

Add the `queue` import if not present.

- [ ] **Step 3.5: Add command routing + validation**

In the existing `Handle` switch in `handler.go`, add cases for `/persona-mint` and `/personas`:

```go
case strings.HasPrefix(text, "/persona-mint "):
	args := strings.TrimSpace(strings.TrimPrefix(text, "/persona-mint "))
	name, prompt, err := parsePersonaMintArgs(args)
	if err != nil {
		_, sErr := h.client.SendMessage(ctx, u.ChatID, "usage: /persona-mint <name> <prompt>\n  name: 3-32 chars, lowercase letters/digits/dashes, not 'planner'/'coder'/'reviewer'\n  prompt: 20-4000 chars\n  error: "+err.Error())
		return sErr
	}
	res, err := h.ops.MintPersona(ctx, name, prompt)
	if errors.Is(err, queue.ErrPersonaNameTaken) {
		_, sErr := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("name '%s' already taken — pick another", name))
		return sErr
	}
	if err != nil {
		_, sErr := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("mint failed: %v", err))
		return sErr
	}
	body := fmt.Sprintf("✓ persona %q minted as token #%s", name, res.TokenID)
	if res.MintTxHash != "" {
		body += fmt.Sprintf("\n  chainscan: https://chainscan-galileo.0g.ai/tx/%s", res.MintTxHash)
	}
	if res.ENSSubname != "" {
		body += fmt.Sprintf("\n  ens: https://sepolia.app.ens.domains/%s", res.ENSSubname)
	}
	if res.SystemPromptURI != "" {
		body += fmt.Sprintf("\n  prompt: %s", res.SystemPromptURI)
	}
	_, err = h.client.SendMessage(ctx, u.ChatID, body)
	return err

case text == "/persona-mint":
	_, err := h.client.SendMessage(ctx, u.ChatID, "usage: /persona-mint <name> <prompt>")
	return err

case text == "/personas":
	list, err := h.ops.ListPersonas(ctx)
	if err != nil {
		_, sErr := h.client.SendMessage(ctx, u.ChatID, fmt.Sprintf("error: %v", err))
		return sErr
	}
	_, err = h.client.SendMessage(ctx, u.ChatID, formatPersonasDM(list))
	return err
```

Add the parser + formatter helpers at the bottom of `handler.go`:

```go
var personaNameRE = regexp.MustCompile(`^[a-z0-9-]{3,32}$`)
var reservedPersonaNames = map[string]bool{"planner": true, "coder": true, "reviewer": true}

// parsePersonaMintArgs splits "<name> <prompt>" and validates. Returns
// (name, prompt, nil) on success. On any validation failure returns a
// human-readable error string in err — meant to surface to the user.
func parsePersonaMintArgs(s string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(s), " ", 2)
	if len(parts) < 2 {
		return "", "", errors.New("missing prompt")
	}
	name := parts[0]
	prompt := strings.TrimSpace(parts[1])

	if !personaNameRE.MatchString(name) {
		return "", "", errors.New("invalid name (must be 3-32 lowercase alphanumerics + dashes)")
	}
	if reservedPersonaNames[name] {
		return "", "", fmt.Errorf("name '%s' is reserved", name)
	}
	if len(prompt) < 20 {
		return "", "", errors.New("prompt too short (min 20 chars)")
	}
	if len(prompt) > 4000 {
		return "", "", errors.New("prompt too long (max 4000 chars)")
	}
	return name, prompt, nil
}

// formatPersonasDM renders the /personas listing.
func formatPersonasDM(personas []queue.Persona) string {
	if len(personas) == 0 {
		return "no personas yet — try /persona-mint <name> <prompt>"
	}
	var b strings.Builder
	b.WriteString("era personas\n────────────\n")
	for _, p := range personas {
		desc := p.Description
		if len(desc) > 50 {
			desc = desc[:50] + "…"
		}
		ens := p.ENSSubname
		if ens == "" {
			ens = "(no ens)"
		}
		fmt.Fprintf(&b, "#%s  %s · %s\n", p.TokenID, ens, desc)
	}
	return b.String()
}
```

- [ ] **Step 3.6: Implement `Queue.MintPersona` + `Queue.ListPersonas`**

In `internal/queue/queue.go`, add the new fields + methods:

```go
type Queue struct {
	// ... existing fields ...
	personas    PersonaRegistry  // may be nil before SetPersonas
	zgStorage   PromptStorage    // for upload + fetch (interface defined below)
}

// PromptStorage is the queue's view of the 0G prompt blob storage.
// Implemented by zg_storage.Client.
type PromptStorage interface {
	UploadPrompt(ctx context.Context, content string) (uri string, err error)
	FetchPrompt(ctx context.Context, uri string) (string, error)
}

func (q *Queue) SetPersonas(p PersonaRegistry)         { q.personas = p }
func (q *Queue) SetPromptStorage(s PromptStorage)      { q.zgStorage = s }

func (q *Queue) MintPersona(ctx context.Context, name, prompt string) (telegram.PersonaMintResult, error) {
	// 0. Pre-check duplicate via SQLite
	if _, err := q.personas.Lookup(ctx, name); err == nil {
		return telegram.PersonaMintResult{}, ErrPersonaNameTaken
	} else if !errors.Is(err, ErrPersonaNotFound) {
		return telegram.PersonaMintResult{}, fmt.Errorf("lookup: %w", err)
	}

	// 1. Upload prompt to 0G
	uri, err := q.zgStorage.UploadPrompt(ctx, prompt)
	if err != nil {
		return telegram.PersonaMintResult{}, fmt.Errorf("upload prompt: %w", err)
	}

	// 2. Mint iNFT (q.inft now has Mint per Step 3.4a)
	if q.inft == nil {
		return telegram.PersonaMintResult{}, errors.New("iNFT not wired (PI_ZG_INFT_CONTRACT_ADDRESS missing)")
	}
	persona, err := q.inft.Mint(ctx, name, uri)
	if err != nil {
		return telegram.PersonaMintResult{}, fmt.Errorf("mint: %w", err)
	}

	// Build the persona row early so syncPersonaENSRecords + Insert share fields.
	desc := prompt
	if len(desc) > 60 {
		desc = desc[:60]
	}
	row := Persona{
		TokenID:         persona.TokenID,
		Name:            name,
		OwnerAddr:       persona.Owner,
		SystemPromptURI: uri,
		Description:     desc,
	}

	// 3. ENS subname (best-effort if q.ensWriter != nil — see Step 3.4b/c)
	if q.ensWriter != nil {
		inftAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
		if err := SyncPersonaENSRecords(ctx, q.ensWriter, row, inftAddr); err != nil {
			slog.Warn("persona-mint ens sync", "name", name, "err", err)
			// don't set row.ENSSubname — leave empty so Phase 5 reconcile retries
		} else {
			row.ENSSubname = name + "." + q.ensWriter.ParentName()
		}
	}

	// 4. Insert into SQLite
	if err := q.personas.Insert(ctx, row); err != nil {
		return telegram.PersonaMintResult{}, fmt.Errorf("registry insert: %w", err)
	}

	return telegram.PersonaMintResult{
		TokenID:         persona.TokenID,
		MintTxHash:      persona.MintTxHash, // populated by Phase 1's Mint impl (Step 1.3)
		ENSSubname:      row.ENSSubname,
		SystemPromptURI: uri,
	}, nil
}

func (q *Queue) ListPersonas(ctx context.Context) ([]Persona, error) {
	return q.personas.List(ctx)
}
```

(`INFTProvider.Mint` extension + `ENSWriter` interface + `syncPersonaENSRecords` helper were all introduced in Steps 3.4a–c above. Production wiring of `q.SetENSWriter(ensProv)` + `q.SetPersonas(repo)` + `q.SetPromptStorage(storageClient)` lands in Phase 5.)

- [ ] **Step 3.7: Run, verify PASS**

```bash
go test ./internal/telegram/ -run "TestHandle_Persona" -count=1 -v
```

Expected: 4 test cases pass.

- [ ] **Step 3.8: Run all tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

Both green. Existing handler tests (M0-M6) MUST still pass — the new Ops methods break the existing `stubOps` if not stubbed.

- [ ] **Step 3.9: Commit**

```bash
git add internal/telegram/ internal/queue/queue.go internal/queue/queue_run_test.go
git commit -m "phase(M7-F.3): /persona-mint + /personas Telegram commands; Queue.MintPersona orchestrates upload+mint+ENS+insert; PersonaMintResult shape; handler validation"
git tag m7f-3-telegram-mint
```

---

## Phase 4: `/task --persona=<name>` plumbing + ensFooter refactor

**Files:**
- Create: `internal/db/migrations/0012_tasks_persona_name.sql`
- Modify: `internal/db/repo.go` (or wherever tasks CRUD lives) — extend `CreateTask` sig, propagate persona_name through to RunNext
- Modify: `internal/queue/queue.go` — `CreateTask` sig change, `CompletedArgs`/`NeedsReviewArgs` `PersonaLabels []string`, RunNext fetches prompt + injects into Pi description + uses persona's tokenID for recordInvocation
- Modify: `internal/telegram/handler.go` — `parsePersonaFlag`, `/task --persona=<name>` parsing, `Ops.CreateTask` sig change
- Modify: `cmd/orchestrator/main.go` — `ensFooter` signature change, callers pass `a.PersonaLabels`
- Modify: `cmd/orchestrator/notifier_ens_test.go` — pass labels arg, add `TestEnsFooter_CustomPersonaLabels`
- Modify: `internal/queue/queue_run_test.go` — stubPersonas, persona-task tests

### 4A: Failing tests

- [ ] **Step 4.1: Migration `0012_tasks_persona_name.sql`**

```sql
-- +goose Up
ALTER TABLE tasks ADD COLUMN persona_name TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite ≤ 3.34 cannot DROP COLUMN; rebuild via temp table.
-- For era's hackathon scope, leaving this as a no-op is acceptable.
SELECT 1;
```

- [ ] **Step 4.2: Write failing handler test for --persona=**

Append to `internal/telegram/handler_test.go`:

```go
func TestHandle_Task_WithPersonaFlag(t *testing.T) {
	ops := &stubOps{}
	_, h := newTestHandler(t, ops)
	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1, Text: "/task --persona=rustacean fix the auth bug"}))
	require.Equal(t, "rustacean", ops.lastPersonaName, "persona flag should propagate to Ops.CreateTask")
	require.Equal(t, "fix the auth bug", ops.lastDesc)
}

func TestHandle_Task_NoPersonaFlag_DefaultsToEmpty(t *testing.T) {
	ops := &stubOps{}
	_, h := newTestHandler(t, ops)
	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1, Text: "/task fix the auth bug"}))
	require.Equal(t, "", ops.lastPersonaName)
}

func TestHandle_Task_PersonaFlagBeforeRepo(t *testing.T) {
	ops := &stubOps{}
	_, h := newTestHandler(t, ops)
	require.NoError(t, h.Handle(context.Background(), Update{
		ChatID: 1, Text: "/task --persona=rustacean foo/bar fix the auth bug"}))
	require.Equal(t, "rustacean", ops.lastPersonaName)
	require.Equal(t, "foo/bar", ops.lastRepo)
	require.Equal(t, "fix the auth bug", ops.lastDesc)
}
```

The stub needs new fields `lastPersonaName`, `lastRepo`, `lastDesc` capturing the args. Read `handler_test.go` for the existing stub shape and add fields.

- [ ] **Step 4.3: Write failing queue test for persona task**

Append to `internal/queue/queue_run_test.go`:

```go
type stubPersonas struct {
	personas map[string]queue.Persona
}

func (s *stubPersonas) Lookup(_ context.Context, name string) (queue.Persona, error) {
	if p, ok := s.personas[name]; ok {
		return p, nil
	}
	return queue.Persona{}, queue.ErrPersonaNotFound
}
func (s *stubPersonas) List(context.Context) ([]queue.Persona, error) { return nil, nil }
func (s *stubPersonas) Insert(_ context.Context, p queue.Persona) error {
	if s.personas == nil { s.personas = map[string]queue.Persona{} }
	if _, ok := s.personas[p.Name]; ok { return queue.ErrPersonaNameTaken }
	s.personas[p.Name] = p
	return nil
}

type stubPromptStorage struct {
	prompts map[string]string // uri → content
}

func (s *stubPromptStorage) UploadPrompt(_ context.Context, content string) (string, error) {
	uri := "stub://" + sha256short(content)
	if s.prompts == nil { s.prompts = map[string]string{} }
	s.prompts[uri] = content
	return uri, nil
}
func (s *stubPromptStorage) FetchPrompt(_ context.Context, uri string) (string, error) {
	if v, ok := s.prompts[uri]; ok { return v, nil }
	return "", fmt.Errorf("not found: %s", uri)
}

func sha256short(s string) string { /* helper */ }

func TestRunNext_PersonaTask_PrependsPromptAndUsesCustomTokenID(t *testing.T) {
	fr := &fakeRunner{branch: "agent/1/ok", summary: "ok"}
	q, repo := newRunQueue(t, fr)

	stub := &stubSwarm{planText: "1. step", plannerSealed: true, reviewDecision: swarm.DecisionApprove}
	q.SetSwarm(stub)
	q.SetUserID("u")

	personas := &stubPersonas{personas: map[string]queue.Persona{
		"rustacean": {TokenID: "3", Name: "rustacean", SystemPromptURI: "stub://x"},
	}}
	storage := &stubPromptStorage{prompts: map[string]string{"stub://x": "RUSTACEAN-PROMPT"}}
	q.SetPersonas(personas)
	q.SetPromptStorage(storage)

	inftStub := &stubINFT{}
	q.SetINFT(inftStub)

	id, err := repo.CreateTask(context.Background(), "fix auth", "owner/repo", "default", "rustacean")
	require.NoError(t, err)
	processed, err := q.RunNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed)

	// Pi (fakeRunner) saw the prepended prompt
	require.Contains(t, fr.lastDesc, "RUSTACEAN-PROMPT")
	require.Contains(t, fr.lastDesc, "fix auth")

	// iNFT recordInvocation was called for token #3 (rustacean), not #1 (default coder)
	tokenIDs := []string{}
	for _, c := range inftStub.calls {
		tokenIDs = append(tokenIDs, c.tokenID)
	}
	require.Contains(t, tokenIDs, "3", "expected recordInvocation against rustacean tokenID 3")
	require.NotContains(t, tokenIDs, "1", "should NOT use default coder tokenID 1")

	// Task #ID
	_ = id
}

func TestRunNext_PersonaTask_UnknownPersona_FailsTask(t *testing.T) {
	fr := &fakeRunner{}
	q, repo := newRunQueue(t, fr)
	q.SetSwarm(&stubSwarm{}) // not consulted
	q.SetUserID("u")

	q.SetPersonas(&stubPersonas{}) // empty registry
	q.SetPromptStorage(&stubPromptStorage{})

	_, err := repo.CreateTask(context.Background(), "fix auth", "owner/repo", "default", "ghost")
	require.NoError(t, err)
	processed, err := q.RunNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed) // RunNext processed it even though task failed

	// Verify the task is in failed state with the right reason in DB.
	// (use existing repo helper — adapt)
}

func TestRunNext_CustomPersona_FreshNamespace_NoError(t *testing.T) {
	// Per spec §10 risk #4 — exercise a never-before-seen persona.
	// Today, since custom personas don't get their own memory namespace
	// (Pi has none), this test asserts that the task completes without
	// error even with a fresh persona — i.e., the persona system doesn't
	// add a new failure mode.
	fr := &fakeRunner{branch: "agent/1/ok", summary: "ok"}
	q, repo := newRunQueue(t, fr)
	q.SetSwarm(&stubSwarm{planText: "1.", plannerSealed: true, reviewDecision: swarm.DecisionApprove})
	q.SetUserID("u")

	personas := &stubPersonas{personas: map[string]queue.Persona{
		"never-seen": {TokenID: "999", Name: "never-seen", SystemPromptURI: "stub://fresh"},
	}}
	storage := &stubPromptStorage{prompts: map[string]string{"stub://fresh": "fresh prompt content"}}
	q.SetPersonas(personas)
	q.SetPromptStorage(storage)

	_, err := repo.CreateTask(context.Background(), "test fresh", "owner/repo", "default", "never-seen")
	require.NoError(t, err)
	processed, err := q.RunNext(context.Background())
	require.NoError(t, err)
	require.True(t, processed)
}
```

- [ ] **Step 4.4: Write failing ensFooter test for custom labels**

In `cmd/orchestrator/notifier_ens_test.go`, add:

```go
func TestEnsFooter_CustomPersonaLabels(t *testing.T) {
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr":      "0x33847c5500…",
			"planner:inft_token_id":  "0",
			"rustacean:inft_addr":    "0x33847c5500…",
			"rustacean:inft_token_id":"3",
			"reviewer:inft_addr":     "0x33847c5500…",
			"reviewer:inft_token_id": "2",
		},
	}
	footer := ensFooter(context.Background(), stub, []string{"planner", "rustacean", "reviewer"})
	require.Contains(t, footer, "rustacean.vaibhav-era.eth")
	require.Contains(t, footer, "token #3")
	require.NotContains(t, footer, "coder.vaibhav-era.eth", "default coder label should NOT appear when custom persona is used")
}

func TestEnsFooter_NilLabelsFallsBackToDefaults(t *testing.T) {
	// Backwards compat: nil/empty labels means use the legacy {planner, coder, reviewer}.
	stub := &stubENS{
		parent: "vaibhav-era.eth",
		values: map[string]string{
			"planner:inft_addr": "x", "planner:inft_token_id": "0",
			"coder:inft_addr": "x", "coder:inft_token_id": "1",
			"reviewer:inft_addr": "x", "reviewer:inft_token_id": "2",
		},
	}
	footer := ensFooter(context.Background(), stub, nil)
	require.Contains(t, footer, "coder.vaibhav-era.eth")
}
```

Update existing `TestEnsFooter_*` tests to pass `nil` for the new `labels` argument (backwards-compat fallback).

- [ ] **Step 4.5: Run all 4A failing tests, verify FAIL**

```bash
go test ./internal/telegram/ ./internal/queue/ ./cmd/orchestrator/ -run "TestHandle_Task_WithPersonaFlag|TestHandle_Task_NoPersonaFlag|TestHandle_Task_PersonaFlagBeforeRepo|TestRunNext_PersonaTask|TestRunNext_CustomPersona_FreshNamespace|TestEnsFooter_CustomPersonaLabels|TestEnsFooter_NilLabelsFallsBackToDefaults" -count=1 -v 2>&1 | head -60
```

Expected: build failures everywhere — `Ops.CreateTask` signature mismatch, `ensFooter` signature mismatch, `Queue.SetPersonas` undefined, etc.

### 4B: Implement

- [ ] **Step 4.6: Extend `Ops.CreateTask` signature**

In `internal/telegram/handler.go`:

```go
type Ops interface {
	CreateTask(ctx context.Context, desc, targetRepo, profile, personaName string) (int64, error)
	// ... rest unchanged ...
}
```

Update the existing `/task` handler to parse `--persona=<name>`:

```go
// Add parsePersonaFlag near parseTaskArgs:
var personaFlagRE = regexp.MustCompile(`^--persona=([a-z0-9-]+)\s*`)

// extractAndStripPersonaFlag parses "--persona=<name>" from the head of the
// /task body and returns (name, remaining). If absent, returns ("", body).
func extractAndStripPersonaFlag(body string) (string, string) {
	m := personaFlagRE.FindStringSubmatch(body)
	if m == nil {
		return "", body
	}
	return m[1], strings.TrimSpace(body[len(m[0]):])
}
```

In the existing `/task` case in `Handle`:

```go
case strings.HasPrefix(text, "/task"):
	body := strings.TrimSpace(strings.TrimPrefix(text, "/task"))
	if body == "" { /* existing usage message */ }
	profile, body := budget.ParseBudgetFlag(body)
	personaName, body := extractAndStripPersonaFlag(body)  // NEW
	repo, desc := parseTaskArgs(body)
	if desc == "" { /* existing usage */ }
	id, err := h.ops.CreateTask(ctx, desc, repo, profile, personaName)  // updated sig
	// ... rest unchanged, just consider mentioning persona in the queued DM
```

(Order of flag extraction: budget → persona → repo → desc. This works because each flag has a distinct prefix.)

- [ ] **Step 4.7: Implement `Queue.CreateTask` sig change + persona resolution in `RunNext`**

In `internal/queue/queue.go`, change `CreateTask` to accept `personaName` and store it. Persist via the migration's new column. The repo's `CreateTask` (likely in `internal/db/repo.go`) needs the same sig change.

In `RunNext`, after the task is loaded:

```go
// Resolve persona before kicking off the swarm.
var coderTokenIDForRecord = coderTokenID  // existing default
var customPrompt string
if t.PersonaName != "" {
	failReason := ""
	switch {
	case q.personas == nil:
		failReason = "persona registry not wired"
	default:
		persona, err := q.personas.Lookup(ctx, t.PersonaName)
		switch {
		case errors.Is(err, ErrPersonaNotFound):
			failReason = fmt.Sprintf("unknown persona '%s'; run /personas", t.PersonaName)
		case err != nil:
			failReason = fmt.Sprintf("persona lookup failed: %v", err)
		case q.zgStorage == nil:
			failReason = "0G prompt storage not wired"
		default:
			prompt, ferr := q.zgStorage.FetchPrompt(ctx, persona.SystemPromptURI)
			if ferr != nil {
				failReason = fmt.Sprintf("persona prompt fetch failed: %v", ferr)
			} else {
				customPrompt = prompt
				coderTokenIDForRecord = persona.TokenID
			}
		}
	}
	if failReason != "" {
		// q.repo.FailTask is the existing helper at internal/db/repo.go:54
		if err := q.repo.FailTask(ctx, t.ID, failReason); err != nil {
			slog.Error("FailTask", "task_id", t.ID, "err", err)
		}
		if q.notifier != nil {
			q.notifier.NotifyFailed(ctx, t.ID, failReason)
		}
		return true, nil
	}
}

// When constructing the Pi runner's description, prepend customPrompt if non-empty.
piDesc := effectiveDesc  // the existing variable
if customPrompt != "" {
	piDesc = "PERSONA SYSTEM PROMPT:\n" + customPrompt + "\n\nTASK:\n" + effectiveDesc
}
// pass piDesc to q.runner.Run instead of effectiveDesc
```

Then at the existing iNFT recordInvocation call site for the coder:

```go
// (existing) — record planner invocation against plannerTokenID
// (existing) — record reviewer invocation against reviewerTokenID
// NEW: record coder invocation against coderTokenIDForRecord (custom or default)
if rerr == nil && q.inft != nil {
	hash := brain.ReceiptHash(coderReceiptStandIn) // see note below
	if recErr := q.inft.RecordInvocation(ctx, coderTokenIDForRecord, hash); recErr != nil {
		slog.Warn("inft recordInvocation failed (coder)", "task_id", t.ID, "tokenID", coderTokenIDForRecord, "err", recErr)
	}
}
```

⚠ **Coder receipt hash.** Currently M7-D.2 only records planner + reviewer. Pi has no `brain.Receipt`. To honor "task ran under persona X" semantics, compute a deterministic stand-in hash: `sha256(taskID + personaName + branch + summary)`. Or skip the coder record for now — this is a design call. For demo: **add the coder record using a stand-in hash** so `recordInvocation` events appear for custom-persona tokens. Note it as a stand-in in the commit message + a code comment.

Add the queue fields `q.personas`, `q.zgStorage` and their `SetPersonas` / `SetPromptStorage` setters (mirrors the `SetINFT` / `SetSwarm` pattern from earlier milestones).

Build the per-task `personaLabels` and pass it through:

```go
// At the existing q.notifier.NotifyCompleted call site:
labels := []string{"planner", "coder", "reviewer"}
if t.PersonaName != "" {
	labels = []string{"planner", t.PersonaName, "reviewer"}
}
q.notifier.NotifyCompleted(ctx, CompletedArgs{
	// ... existing fields ...
	PersonaLabels: labels,
})
// Same for NotifyNeedsReview.
```

Add the `PersonaLabels []string` field to both `CompletedArgs` and `NeedsReviewArgs`.

- [ ] **Step 4.8: Refactor `ensFooter` to take `labels` argument**

In `cmd/orchestrator/main.go`:

```go
func ensFooter(ctx context.Context, ens ENSResolver, labels []string) string {
	if ens == nil {
		return ""
	}
	if len(labels) == 0 {
		labels = []string{"planner", "coder", "reviewer"}
	}
	type row struct{ label, addr, tokenID string }
	rows := make([]row, 0, len(labels))
	for _, label := range labels {
		// ... existing logic, parameterized by `label`
	}
	// ... rest unchanged
}
```

Update the two call sites:
- `NotifyCompleted` (line ~329): `body += ensFooter(ctx, n.ens, a.PersonaLabels)`
- `NotifyNeedsReview` (line ~379): `body := formatNeedsReviewMessage(a) + ensFooter(ctx, n.ens, a.PersonaLabels)`

Update existing 4 `TestEnsFooter_*` tests to pass `nil` for the new arg.

- [ ] **Step 4.9: Run, verify all PASS**

```bash
go test ./internal/telegram/ ./internal/queue/ ./cmd/orchestrator/ -count=1 -v 2>&1 | tail -50
```

Expected: all new tests pass + existing tests still green.

- [ ] **Step 4.10: Run full regression**

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

Both green.

- [ ] **Step 4.11: Commit**

```bash
git add internal/db/migrations/ internal/db/ internal/queue/ internal/telegram/ cmd/orchestrator/
git commit -m "phase(M7-F.4): /task --persona=<name> plumbing — flag parse, prompt prefix to Pi description, custom tokenID for recordInvocation, ensFooter refactored to take labels per-task

The custom 'coder slot' is implemented as a prompt prefix to Pi's task
description (Pi-in-Docker has no system-prompt parameter). iNFT
recordInvocation against the persona's tokenID records that the task ran
under that persona's prompt — receipt hash is a deterministic stand-in
since Pi doesn't generate a brain.Receipt."
git tag m7f-4-task-persona
```

---

## Phase 5: Boot reconcile + live Telegram gate

**Files:**
- Create: `cmd/orchestrator/personas_reconcile.go`
- Create: `cmd/orchestrator/personas_reconcile_test.go`
- Modify: `cmd/orchestrator/main.go` — add `personasReconcile()` call after notifier setup
- Modify: `internal/queue/queue.go` — wire `q.SetPersonas` + `q.SetPromptStorage` from main.go

### 5A: Failing test for `personasReconcile`

- [ ] **Step 5.1: Write failing tests for the three reconcile passes**

`cmd/orchestrator/personas_reconcile_test.go`:

```go
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/queue"
)

func TestReconcile_DefaultSeed_InsertsBuiltins(t *testing.T) {
	registry := newInMemoryRegistry(t)
	require.NoError(t, reconcileDefaults(context.Background(), registry))
	list, err := registry.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 3)
	names := []string{}
	for _, p := range list { names = append(names, p.Name) }
	require.ElementsMatch(t, []string{"planner", "coder", "reviewer"}, names)
}

func TestReconcile_DefaultSeed_Idempotent(t *testing.T) {
	registry := newInMemoryRegistry(t)
	require.NoError(t, reconcileDefaults(context.Background(), registry))
	require.NoError(t, reconcileDefaults(context.Background(), registry)) // 2nd call no-ops
	list, _ := registry.List(context.Background())
	require.Len(t, list, 3)
}

func TestReconcile_ENSRetry_SkipsNonEmpty(t *testing.T) {
	registry := newInMemoryRegistry(t)
	registry.Insert(context.Background(), queue.Persona{TokenID: "0", Name: "planner", ENSSubname: "planner.foo.eth"})
	registry.Insert(context.Background(), queue.Persona{TokenID: "3", Name: "rust", ENSSubname: ""})

	ens := &stubENS{ /* tracks calls */ }
	require.NoError(t, reconcileENS(context.Background(), registry, ens))
	require.NotContains(t, ens.ensuredLabels, "planner") // already has subname
	require.Contains(t, ens.ensuredLabels, "rust")        // missing — retry
}

func TestReconcile_TransferScan_ImportsNewMints(t *testing.T) {
	registry := newInMemoryRegistry(t)
	registry.Insert(context.Background(), queue.Persona{TokenID: "0", Name: "planner"})
	registry.Insert(context.Background(), queue.Persona{TokenID: "1", Name: "coder"})
	registry.Insert(context.Background(), queue.Persona{TokenID: "2", Name: "reviewer"})

	// Stub iNFT scanner returns one new event for tokenID 3
	scanner := &stubTransferScanner{
		events: []transferEvent{{TokenID: "3", URI: "stub://prompt-3"}},
	}
	storage := &stubPromptStorage{prompts: map[string]string{"stub://prompt-3": "Test prompt for token 3"}}
	require.NoError(t, reconcileFromChain(context.Background(), registry, scanner, storage))

	got, err := registry.List(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 4)
	tokenIDs := []string{}
	for _, p := range got { tokenIDs = append(tokenIDs, p.TokenID) }
	require.Contains(t, tokenIDs, "3")
}
```

The test references helpers `newInMemoryRegistry`, `reconcileDefaults`, `reconcileENS`, `reconcileFromChain`, `stubTransferScanner`, `transferEvent`. Define minimal stubs in the test file.

- [ ] **Step 5.2: Run, verify FAIL**

```bash
go test ./cmd/orchestrator/ -run TestReconcile -count=1 -v 2>&1 | head -30
```

Expected: build failure on `undefined: reconcileDefaults`, etc.

### 5B: Implement reconcile

- [ ] **Step 5.3: Write `personas_reconcile.go`**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/vaibhav0806/era-multi-persona/era-brain/identity/ens"
	"github.com/vaibhav0806/era-multi-persona/era-brain/inft/zg_7857"
	"github.com/vaibhav0806/era/internal/queue"
)

// personasReconcile runs three idempotent passes at boot to ensure SQLite is
// consistent with on-chain state. Each pass logs failures + continues.
func personasReconcile(
	ctx context.Context,
	registry queue.PersonaRegistry,
	inftProv *zg_7857.Provider,    // may be nil
	ensProv ens.Resolver,          // type alias for the read interface; may be nil
	storage queue.PromptStorage,   // may be nil
) {
	if err := reconcileDefaults(ctx, registry); err != nil {
		slog.Warn("personas: default seed failed", "err", err)
	}
	if inftProv != nil && storage != nil {
		if err := reconcileFromChain(ctx, registry, inftProv, storage); err != nil {
			slog.Warn("personas: chain reconcile failed", "err", err)
		}
	}
	if ensProv != nil {
		if err := reconcileENS(ctx, registry, ensProv); err != nil {
			slog.Warn("personas: ens reconcile failed", "err", err)
		}
	}
}

// reconcileDefaults INSERT-OR-IGNOREs the 3 builtin personas.
func reconcileDefaults(ctx context.Context, registry queue.PersonaRegistry) error {
	defaults := []queue.Persona{
		{
			TokenID:         "0",
			Name:            "planner",
			OwnerAddr:       os.Getenv("PI_ZG_INFT_OWNER_OR_DEPLOYER"), // adapt: derive from PI_ZG_PRIVATE_KEY
			SystemPromptURI: plannerZGURI, // hardcoded const from main.go (M7-E.3)
			ENSSubname:      "planner." + os.Getenv("PI_ENS_PARENT_NAME"),
			Description:     "default planner — break tasks down",
		},
		// coder, reviewer ...
	}
	for _, p := range defaults {
		if err := registry.Insert(ctx, p); err != nil {
			if errors.Is(err, queue.ErrPersonaNameTaken) {
				continue // already present, idempotent
			}
			return fmt.Errorf("insert default %s: %w", p.Name, err)
		}
	}
	return nil
}

// reconcileFromChain scans iNFT Transfer events for tokens > maxKnownTokenID
// and imports them into the registry by fetching tokenURI + prompt content.
func reconcileFromChain(
	ctx context.Context,
	registry queue.PersonaRegistry,
	scanner TransferScanner, // small interface; *zg_7857.Provider implements it
	storage queue.PromptStorage,
) error {
	known, err := registry.List(ctx)
	if err != nil { return err }
	maxKnown := int64(0)
	for _, p := range known {
		n, _ := strconv.ParseInt(p.TokenID, 10, 64)
		if n > maxKnown { maxKnown = n }
	}

	events, err := scanner.ScanNewMints(ctx, maxKnown)
	if err != nil { return fmt.Errorf("scan: %w", err) }

	for _, ev := range events {
		prompt, err := storage.FetchPrompt(ctx, ev.URI)
		if err != nil {
			slog.Warn("reconcile: fetch prompt failed", "tokenID", ev.TokenID, "err", err)
			continue
		}
		desc := prompt
		if len(desc) > 60 { desc = desc[:60] }
		row := queue.Persona{
			TokenID:         ev.TokenID,
			Name:            "imported-" + ev.TokenID, // we don't know the name; user can rename later (out of scope for M7-F)
			OwnerAddr:       ev.Owner,
			SystemPromptURI: ev.URI,
			ENSSubname:      "",
			Description:     desc,
		}
		if err := registry.Insert(ctx, row); err != nil {
			slog.Warn("reconcile: insert imported persona failed", "tokenID", ev.TokenID, "err", err)
		}
	}
	return nil
}

// reconcileENS retries ENS subname registration for personas with empty ens_subname.
// Uses the same `syncPersonaENSRecords` helper as Queue.MintPersona (defined in
// internal/queue/persona_ens.go in Phase 3 step 3.4c). This ensures the records
// written at boot are byte-identical to the records written at /persona-mint time.
func reconcileENS(ctx context.Context, registry queue.PersonaRegistry, ens queue.ENSWriter) error {
	all, err := registry.List(ctx)
	if err != nil { return err }
	inftAddr := os.Getenv("PI_ZG_INFT_CONTRACT_ADDRESS")
	for _, p := range all {
		if p.ENSSubname != "" { continue }
		// syncPersonaENSRecords is exported from internal/queue/persona_ens.go.
		// If it's lowercase (file-internal), export it as `SyncPersonaENSRecords`
		// or move it to a separate exported helper.
		if err := queue.SyncPersonaENSRecords(ctx, ens, p, inftAddr); err != nil {
			slog.Warn("reconcile: ens sync failed", "name", p.Name, "err", err)
			continue
		}
		// Update SQLite row with the new ens_subname so future boots skip it.
		// (Add a small repo helper UpdatePersonaENSSubname(ctx, name, subname) — 5 lines.)
		full := p.Name + "." + ens.ParentName()
		if err := registry.UpdateENSSubname(ctx, p.Name, full); err != nil {
			slog.Warn("reconcile: persisting ens_subname failed", "name", p.Name, "err", err)
		}
	}
	return nil
}

// TransferScanner is implemented by *zg_7857.Provider after Phase 5.
type TransferScanner interface {
	ScanNewMints(ctx context.Context, sinceTokenID int64) ([]TransferEvent, error)
}

type TransferEvent struct {
	TokenID string
	Owner   string
	URI     string
}
```

⚠ **`TransferScanner.ScanNewMints` impl** lives in `era-brain/inft/zg_7857/zg_7857.go`. Add:

```go
func (p *Provider) ScanNewMints(ctx context.Context, sinceTokenID int64) ([]TransferEvent, error) {
	// FilterTransfer(opts, []common.Address{0x0}, []common.Address{p.auth.From}, nil)
	// from block 0 to latest. Iterate, filter event.TokenId > sinceTokenID.
	zero := common.Address{}
	iter, err := p.contract.FilterTransfer(&bind.FilterOpts{Context: ctx}, []common.Address{zero}, []common.Address{p.auth.From}, nil)
	if err != nil { return nil, err }
	defer iter.Close()
	var out []TransferEvent
	for iter.Next() {
		ev := iter.Event
		if ev.TokenId.Int64() <= sinceTokenID { continue }
		uri, err := p.contract.TokenURI(&bind.CallOpts{Context: ctx}, ev.TokenId)
		if err != nil { continue }
		out = append(out, TransferEvent{
			TokenID: ev.TokenId.String(),
			Owner:   ev.To.Hex(),
			URI:     uri,
		})
	}
	return out, iter.Error()
}
```

Also need `TransferEvent` exported from `zg_7857` package (or use the orchestrator-side type and adapt). Cleanest: define `TransferEvent` in `zg_7857/types.go` and the orchestrator imports it.

- [ ] **Step 5.4: Wire `personasReconcile()` into `main.go`**

In `cmd/orchestrator/main.go`, after `notifier := &tgNotifier{...}` and after the ENS wiring block (M7-E.3), add:

```go
// M7-F: persona registry reconcile (defaults + on-chain Transfer scan + ENS retry)
{
	var inftForReconcile *zg_7857.Provider
	if zgINFTEnabled() {
		// reuse the inftProv from M7-D.2 wiring — capture it earlier in scope
		inftForReconcile = inftProv
	}
	var storage queue.PromptStorage
	if zgEnabled() {
		// construct zg_storage.Client once and reuse
		storage = newZGStorage()  // helper that wraps zg_kv config
	}
	personasReconcile(ctx, repo /* implements PersonaRegistry */, inftForReconcile, ensProv, storage)
	q.SetPersonas(repo)
	q.SetPromptStorage(storage)
}
```

Adapt scope: `inftProv`, `ensProv` may be defined inside their `if zgEnabled() { … }` blocks; lift to outer scope or pass through.

- [ ] **Step 5.5: Run, verify PASS**

```bash
go test ./cmd/orchestrator/ -run TestReconcile -count=1 -v
```

Expected: 4 reconcile tests pass.

- [ ] **Step 5.6: Run full regression + vet**

```bash
go vet ./...
go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./... && cd ..
```

Both green.

- [ ] **Step 5.7: Commit (code only — live gate is separate)**

```bash
git add cmd/orchestrator/ internal/queue/ era-brain/inft/zg_7857/
git commit -m "phase(M7-F.5): personas boot reconcile — default seed + on-chain Transfer scan + ENS retry pass; ScanNewMints helper"
git tag m7f-5-reconcile
```

### 5C: Live Telegram gate (no commit; verification only)

- [ ] **Step 5.8: Pre-flight checks**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
cast call $PI_ZG_INFT_CONTRACT_ADDRESS 'owner()(address)' --rpc-url $PI_ZG_EVM_RPC
cast wallet address $PI_ZG_PRIVATE_KEY
cast balance $(cast wallet address $PI_ZG_PRIVATE_KEY) --rpc-url $PI_ENS_RPC --ether
```

Owner == signer. Balance ≥ 0.005 Sepolia ETH. If not, faucet up.

- [ ] **Step 5.9: Build orchestrator + stop VPS**

```bash
go build -o bin/orchestrator ./cmd/orchestrator
ssh era@178.105.44.3 sudo systemctl stop era
ssh era@178.105.44.3 systemctl is-active era
```

Confirm `inactive`.

- [ ] **Step 5.10: Start local orchestrator**

```bash
./bin/orchestrator
```

Expected boot lines (additive to M7-E):
- `INFO 0G iNFT registry wired ...`
- `INFO ENS resolver wired ...`
- (NEW) `INFO personas reconciled defaults=3 imported=N ens_retried=M` — or per-pass log lines

If reconcile fails, boot continues with warnings. Verify by checking the logs.

- [ ] **Step 5.11: Send `/persona-mint` via Telegram**

```
/persona-mint rustacean You only write idiomatic, production-quality Rust code with rigorous error handling, comprehensive tests, and clear lifetimes. Never compromise borrow-checker correctness for cleverness.
```

Expected DM:
```
✓ persona "rustacean" minted as token #3
  chainscan: https://chainscan-galileo.0g.ai/tx/0x...
  ens: https://sepolia.app.ens.domains/rustacean.vaibhav-era.eth
  prompt: zg://<hash>
```

Verify on-chain:
- chainscan link shows the mint tx
- sepolia.app.ens.domains/rustacean.vaibhav-era.eth shows 4 text records
- SQLite: `sqlite3 pi-agent.db "SELECT * FROM personas WHERE name='rustacean'"`

- [ ] **Step 5.12: Send `/personas`**

Expected DM lists 4 personas including `rustacean`.

- [ ] **Step 5.13: Send `/task --persona=rustacean ...`**

```
/task --persona=rustacean add a /healthz endpoint that returns 200 OK
```

Watch:
- Pi runs as usual (with prompt prefix)
- Telegram review DM contains `personas:` footer with `rustacean.vaibhav-era.eth → token #3` (NOT `coder.…`)
- iNFT contract events show recordInvocation against token #3
- PR opens normally

- [ ] **Step 5.14: Verify on chainscan**

Open https://chainscan-galileo.0g.ai/address/0x33847c5500C2443E2f3BBf547d9b069B334c3D16#events

Expect new `Invocation` events with `tokenId = 3`.

- [ ] **Step 5.15: Restart VPS + stop local**

```bash
ssh era@178.105.44.3 sudo systemctl start era
# Ctrl-C local orchestrator
```

- [ ] **Step 5.16: Replay tests**

```bash
go vet ./... && go test -race -count=1 ./...
cd era-brain && go vet ./... && go test -race -count=1 ./...
```

Both green.

- [ ] **Step 5.17: Tag M7-F done**

```bash
git tag m7f-done
git push origin master
git push --tags
```

---

## Live gate summary (M7-F acceptance)

When this milestone is done:

1. `go build ./...` from repo root succeeds.
2. `go test -race -count=1 ./...` from repo root green; no regression.
3. Real `/persona-mint <name> <prompt>`:
   - DM contains: token #N, chainscan link, sepolia ENS link, 0G storage URI
   - Token visible on chainscan-galileo
   - Subname resolvable on sepolia.app.ens.domains with 4 text records
   - SQLite row exists
4. Real `/task --persona=<name> <desc>`:
   - PR opens; reviewer flow works
   - DM `personas:` footer reflects custom subname (NOT `coder.…`)
   - iNFT events show recordInvocation against custom token ID
5. `/personas` lists defaults + custom mints.
6. Hard rejects: duplicate name, invalid name format, unknown persona at /task time.
7. Without ENS env vars: mint succeeds (no ENS); /personas works; /task --persona= still works.
8. SQLite-wipe + restart: 3 defaults seed; on-chain custom mints reimport via Transfer scan.
9. VPS M6 era is restarted after the live gate.

---

## Out of scope (deferred)

- **Per-slot persona flags** (`--planner=`, `--reviewer=`). Same plumbing as `--persona=`, just more flag variants.
- **Persona memory namespace per persona.** Spec mentioned this; deferred (Pi has no memory namespace; would need swarm/coder refactor to a brain.LLMPersona-based coder).
- **Real coder receipt hash for custom personas.** M7-F uses a stand-in deterministic hash for `recordInvocation` from the coder slot. A future milestone can compute a proper sealed-inference receipt if/when a brain.LLMPersona-based coder ships.
- **Imported persona naming.** When `reconcileFromChain` discovers a previously-minted token, it imports as `imported-<tokenID>` (no original name available without an off-chain registry). User can rename via SQLite manually for now.
- **Persona deletion / re-mint.** Hard reject on duplicate name; pick a different name.

---

## Risks + cuts list (in order if slipping)

1. **`zg_kv` API mismatch.** Phase 1's `UploadPrompt` assumes `zg_kv.Provider.Set(namespace, key, value)`. If the actual API differs, adapt the helper. Worst case: ~1 hour to wrap differently.
2. **Pi's task description prefix breaks Pi parsing.** Pi was designed for plain task descriptions. Prefixing with "PERSONA SYSTEM PROMPT:" headers may confuse Pi's tool loop. Recovery: Pi will still try to do the task; if the prompt prefix breaks Pi, switch to environment variable injection (Pi reads `ERA_PERSONA_PROMPT` and uses as system-prompt prefix internally). Adds ~1hr.
3. **`reconcileFromChain` Transfer scan from block 0 is heavy.** With only ~6 mints to date (3 defaults + 3 from this milestone's testing) it's fine. If it becomes slow, persist `last_scanned_block` in a small KV table. Not needed for hackathon scope.
4. **Migration `0012_tasks_persona_name.sql` Down** — SQLite ≤ 3.34 can't `DROP COLUMN`. Hackathon scope: leave as no-op. Production fix is a temp-table rebuild; out of scope here.
5. **Coder `recordInvocation` stand-in hash** is not a sealed-inference receipt — it's a marker. Spec calls this out; commit message must too.
6. **`Provider.Mint` retries / nonce drift** — go-ethereum's signer auto-fetches nonce from latest block. If two `/persona-mint` calls fire concurrently (single-user bot, unlikely) the second might fail with "nonce too low." Acceptable.

---

## Notes for implementer

- Phase 1's `Provider.Mint` is the highest-risk piece — Transfer event parsing is fragile if abigen output changes. Lock to the existing v1.17.2 abigen.
- Phase 4's prompt-prefix-to-Pi is the design choice that defines the feature. Don't try to plumb prompt as a separate param into Pi — Pi's existing surface area is the description string.
- All migrations use `+goose Up`/`+goose Down` markers — match the existing pattern in `internal/db/migrations/`.
- Phase 5 is the only phase with a live gate that costs real funds (~$0.005); all other phases use simulated.Backend.
- Each phase's tagged commit ends with green tests in BOTH modules. Don't push tag until both are verified.
- All commits exclude `Co-Authored-By` and `--author` per `~/.claude/CLAUDE.md`.
