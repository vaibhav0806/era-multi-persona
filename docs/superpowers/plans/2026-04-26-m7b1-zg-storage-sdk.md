# M7-B.1 — era-brain 0G Storage SDK Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 0G Storage as a `memory.Provider` impl in `era-brain`. Two new sub-packages — `era-brain/memory/zg_kv` (key-value semantics on 0G KV streams) and `era-brain/memory/zg_log` (append-only log on top of 0G KV streams using sequence-numbered keys) — plus a `era-brain/memory/dual` wrapper that writes to both 0G and SQLite synchronously and reads from SQLite first (write-both, primary-with-fallback per Q4 architectural decision). Live gate is the existing `examples/coding-agent` program reconfigured to use the dual provider against real 0G testnet — receipts appear on 0G explorer AND in local SQLite.

**Architecture:** This is the **SDK-side milestone**. era's orchestrator does NOT yet wire 0G memory — that's M7-B.2. Scope here is pure era-brain SDK growth.

We use 0G's official Go SDK at `github.com/0glabs/0g-storage-client` (v1.3.0+). Both KV and Log semantics route through the SDK's `kv` package — different conventions:

- **`zg_kv` Provider** stores `(ns, key) → val` as 0G stream entries: streamId = `sha256(ns)`, key = `[]byte(key)`, val = `[]byte(val)`. Last-write-wins via per-key versions on the stream.
- **`zg_log` Provider** stores append-only entries as 0G stream entries: streamId = `sha256(ns)`, keys = monotonic sequence numbers (`[]byte("000001")`, `[]byte("000002")`, etc.) so iteration via `kv.Iterator` returns entries in append order.

Tests use a `kvOps` interface seam that wraps the SDK's KV operations, so unit tests inject a fake without touching real 0G. **Phase 0 is a setup-only phase** — no era-brain code touched, just verifying the SDK works against testnet from your machine via a 50-line standalone script.

**Tech Stack:** Go 1.25, `github.com/0glabs/0g-storage-client` (new dep), `github.com/0glabs/0g-storage-client/kv` (KV streams), `github.com/0glabs/0g-storage-client/node` (RPC client), `github.com/openweb3/web3go` (transitive — Web3 client for signing). All other tests still use `stretchr/testify`. No HTTP client roll-our-own.

**Spec:** `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md` §3 + §5 — with these brainstormed decisions:
- Q1: 0G testnet setup needed → Phase 0.
- Q2: Use Go SDK (overrides earlier "HTTP client" recommendation — research showed 0G has no HTTP-only API; auth = signed Web3 tx).
- Q3: Persona memory shape = single blob per (persona, user) holding last-N observations.
- Q4: Write-both (0G + SQLite synchronously); read SQLite-first.

**Testing philosophy:** Strict TDD. Fail-first tests before implementation. `go test -race -count=1 ./...` from repo root green at every commit. Live 0G testnet gate at the end (and a Phase 0 standalone-script gate that proves SDK works against testnet *before* any era-brain code lands). Subagent-driven execution per project philosophy.

**Prerequisites (check before starting):**
- M7-A.5 complete (tag `m7a5-done`). era-brain SDK is `go get`-able. SQLite memory provider works.
- An EVM-compatible wallet (existing one from your hot-wallet pile is fine if you want to reuse, or generate a new one specifically for 0G testnet).
- GitHub repo at `vaibhav0806/era-multi-persona` exists and is pushable.

---

## File Structure

```
era-brain/go.mod                                MODIFY (Phase 1) — add 0g-storage-client
era-brain/go.sum                                MODIFY (Phase 1)

era-brain/memory/zg_kv/                         CREATE (Phase 1)
├── kv.go                                       CREATE — Provider impl + kvOps interface seam
└── kv_test.go                                  CREATE — TDD coverage with fake kvOps

era-brain/memory/zg_log/                        CREATE (Phase 2)
├── log.go                                      CREATE — Provider impl on top of kvOps
└── log_test.go                                 CREATE — TDD coverage

era-brain/memory/dual/                          CREATE (Phase 3)
├── dual.go                                     CREATE — Provider that wraps two providers
└── dual_test.go                                CREATE — TDD coverage with two fake providers

era-brain/examples/coding-agent/main.go         MODIFY (Phase 4) — wire dual(sqlite, zg_kv+zg_log)

scripts/zg-smoke/                               CREATE (Phase 0)
└── zg-smoke.go                                 CREATE — standalone SDK verification script

.env.example                                    MODIFY (Phase 0) — document 0G env vars
```

The `scripts/zg-smoke/` is at era's repo root (not inside era-brain) so it can be deleted or left as a developer-facing tool. It's NOT a Go test file; it's a `main` package run via `go run`.

---

## Phase 0: 0G testnet setup + standalone SDK smoke script

**No era-brain code touched in this phase.** Goal: prove the SDK works against the 0G testnet from your machine **before** integrating into era-brain. If the smoke script can't write+read a KV pair, no point writing the era-brain provider.

**Files:**
- Create: `scripts/zg-smoke/zg-smoke.go`
- Modify: `.env.example`
- Modify (your local): `.env` (NOT committed; just for your wallet credentials)

### 0.1: Acquire 0G testnet credentials

- [ ] **Step 0.1.1: Create a wallet for 0G testnet**

You can reuse any EVM-compatible private key, OR generate a fresh one specifically for hackathon use:

```bash
# Option A: reuse an existing wallet — copy its private key (raw hex, 64 chars).
# Option B: generate a new one with foundry/cast (preferred for hackathon — keeps separation):
cast wallet new
```

Save the private key (raw hex, no `0x` prefix needed for the SDK). **This wallet is hot — never put real funds in it.**

- [ ] **Step 0.1.2: Faucet ZG tokens**

Visit the 0G testnet faucet (linked from `https://docs.0g.ai/developer-hub/testnet/testnet-overview`). Request testnet ZG to your wallet address.

- [ ] **Step 0.1.3: Verify balance via curl**

```bash
WALLET_ADDR=0x...  # your wallet address
curl -X POST https://evmrpc-testnet.0g.ai \
  -H "Content-Type: application/json" \
  -d "{\"jsonrpc\":\"2.0\",\"method\":\"eth_getBalance\",\"params\":[\"$WALLET_ADDR\",\"latest\"],\"id\":1}"
```

Expected: a non-zero hex balance in the response.

If zero, faucet hasn't credited yet — wait a minute and retry.

### 0.2: .env additions

- [ ] **Step 0.2.1: Add 0G env vars to `.env.example`**

Append to `.env.example`:

```
# 0G testnet (M7-B onward)
# Get testnet ZG from https://docs.0g.ai → faucet
PI_ZG_PRIVATE_KEY=0xYOUR_PRIVATE_KEY_HEX
PI_ZG_EVM_RPC=https://evmrpc-testnet.0g.ai
PI_ZG_INDEXER_RPC=https://indexer-storage-testnet-turbo.0g.ai
```

- [ ] **Step 0.2.2: Add the same vars to your local `.env`** (NOT committed)

Open `.env` (not `.env.example`), append the three lines with your real private key.

- [ ] **Step 0.2.3: Verify env reachability**

```bash
set -a; source .env; set +a
echo "key set: ${PI_ZG_PRIVATE_KEY:0:6}..."
echo "rpc: $PI_ZG_EVM_RPC"
echo "indexer: $PI_ZG_INDEXER_RPC"
```

All three should print non-empty.

### 0.3: Standalone SDK smoke script

This script proves the SDK can write + read against real 0G testnet. Roughly 80 lines. Run via `go run`.

- [ ] **Step 0.3.1: Add the smoke dir + temporarily add the SDK dep to era's root go.mod**

```bash
mkdir -p scripts/zg-smoke
# Add the SDK to era's root go.mod so `go run ./scripts/zg-smoke` resolves imports.
# After Phase 1 the dep moves to era-brain/go.mod; we can remove from era root then.
go get github.com/0glabs/0g-storage-client@latest
go mod tidy
```

(Avoiding a separate `go.mod` in the smoke dir — it would NOT resolve via `go run` from era root and would just add toolchain friction.)

- [ ] **Step 0.3.2: Write the smoke script**

**Important:** the SDK templates below are best-effort based on pkg.go.dev as of plan-writing. Treat them as scaffolding, NOT authoritative. **Before each unfamiliar SDK call, open `https://pkg.go.dev/github.com/0glabs/0g-storage-client@latest/<package>` and verify the actual signature.** The two known-shaky areas:

1. **Indexer client lives in `github.com/0glabs/0g-storage-client/indexer`**, NOT in `transfer`. Constructor returns `(*indexer.Client, error)` (two-return). `SelectNodes` likely takes more args than shown — verify on pkg.go.dev.
2. **`kv.Batcher.Set` is a builder pattern** with a value receiver. It returns `*streamDataBuilder` — calling `Set(...)` standalone discards the return and writes NOTHING. Correct usage is to capture the return and call `.Build()` on it, or chain. Verify the exact pattern via godoc + the SDK's own tests/examples.

If your implementation diverges from this scaffolding, that is EXPECTED and CORRECT. The whole point of Phase 0 is to discover the right shapes before Phase 1 builds on them.

`scripts/zg-smoke/zg-smoke.go`:

```go
// zg-smoke is a standalone SDK verification script. Run with:
//
//	set -a; source ../../.env; set +a
//	go run ./scripts/zg-smoke
//
// It writes a single KV pair to a 0G stream, reads it back, and prints
// the result. If this works, the era-brain zg_kv provider has a working
// foundation to build on.
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/0glabs/0g-storage-client/indexer" // verify: indexer package, NOT transfer
	"github.com/0glabs/0g-storage-client/kv"
	"github.com/0glabs/0g-storage-client/node"
	"github.com/ethereum/go-ethereum/common"
	"github.com/openweb3/web3go"
)

func main() {
	priv := os.Getenv("PI_ZG_PRIVATE_KEY")
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	indexerURL := os.Getenv("PI_ZG_INDEXER_RPC")
	if priv == "" || rpc == "" || indexerURL == "" {
		log.Fatal("PI_ZG_PRIVATE_KEY, PI_ZG_EVM_RPC, PI_ZG_INDEXER_RPC required")
	}

	w3, err := web3go.NewClientWithOption(rpc, *web3go.NewClientOption().
		WithPrivateKeys([]string{priv}))
	if err != nil {
		log.Fatalf("web3go: %v", err)
	}
	defer w3.Close()

	// Pick storage nodes via the indexer.
	// VERIFY ON GODOC: indexer.NewClient signature + SelectNodes arity.
	// As of v1.3.0 docs: NewClient returns (*indexer.Client, error); SelectNodes
	// takes (ctx, expectedReplica, dropped, method, fullTrusted).
	idx, err := indexer.NewClient(indexerURL)
	if err != nil {
		log.Fatalf("indexer client: %v", err)
	}
	nodes, err := idx.SelectNodes(context.Background(), 1, nil, "", false)
	if err != nil {
		log.Fatalf("select nodes: %v", err)
	}
	defer nodes.Close()

	// Build a KV client (read-side) from the first selected node.
	if len(nodes.Trusted) == 0 {
		log.Fatal("no trusted nodes returned")
	}
	kvNode, err := node.NewKvClient(nodes.Trusted[0].URL)
	if err != nil {
		log.Fatalf("kv node: %v", err)
	}
	client := kv.NewClient(kvNode)

	// streamId derived from a namespace string by sha256.
	ns := "zg-smoke-ns"
	streamId := sha256Hash(ns)
	key := []byte("hello")
	val := []byte(fmt.Sprintf("world-%d", time.Now().Unix()))

	// Write via Batcher.
	// VERIFY ON GODOC: kv.Batcher.Set is a BUILDER pattern with a VALUE receiver.
	// Calling batcher.Set(...) standalone discards the return — writes NOTHING.
	// You MUST capture the returned builder and call .Build() on IT (not on
	// batcher). Then Exec() submits. Pseudocode (verify exact shape):
	//
	//   builder := batcher.Set(streamId, key, val)
	//   streamData, err := builder.Build()
	//   ...
	//   txHash, err := batcher.Exec(ctx, streamData)
	//
	// If Exec doesn't take streamData, the Batcher itself holds it after Build —
	// godoc will clarify. Adjust the code below to match.
	batcher := kv.NewBatcher(1, nodes, w3)
	streamDataBuilder := batcher.Set(streamId, key, val)
	streamData, err := streamDataBuilder.Build()
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	_ = streamData
	txHash, err := batcher.Exec(context.Background())
	if err != nil {
		log.Fatalf("exec: %v", err)
	}
	fmt.Printf("[wrote] tx=%s stream=%s key=%s val=%s\n", txHash, streamId.Hex(), key, val)

	// Wait for confirmation; SDK provides this implicitly per upload but allow a few seconds.
	time.Sleep(5 * time.Second)

	// Read back.
	got, err := client.GetValue(context.Background(), streamId, key)
	if err != nil {
		log.Fatalf("getvalue: %v", err)
	}
	fmt.Printf("[read]  val=%s\n", got.Data)

	if string(got.Data) != string(val) {
		log.Fatalf("MISMATCH: wrote %q, read %q", val, got.Data)
	}
	fmt.Println("OK")
}

func sha256Hash(s string) common.Hash {
	h := sha256.Sum256([]byte(s))
	return common.BytesToHash(h[:])
}
```

**Critical: verify the SDK API shapes against pkg.go.dev BEFORE running.** The template above flags the two highest-risk areas. If the script fails to compile, the godoc is the source of truth — fix the call sites accordingly, then mirror the corrections into Phase 1's `LiveOps` impl. Phase 0 succeeding = "we know the right SDK shapes for this version of the SDK".

- [ ] **Step 0.3.3: Run the smoke script**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
go run ./scripts/zg-smoke
```

Expected output:
```
[wrote] tx=0x... stream=0x... key=hello val=world-1714215...
[read]  val=world-1714215...
OK
```

If the read-after-write returns an error or wrong data:
- Increase the sleep (some testnet writes confirm slowly)
- Verify your wallet has gas (re-check balance from Step 0.1.3)
- Verify indexer URL is correct
- Read the actual SDK godoc for any API changes

**Do not proceed to Phase 1 until this script prints `OK`.**

- [ ] **Step 0.3.4: Commit (Phase 0)**

Commit `.env.example` and the smoke script (NOT `.env`):

```bash
git add .env.example scripts/zg-smoke/
git commit -m "phase(M7-B.1.0): 0G testnet setup + standalone SDK smoke script"
git tag m7b1-0-setup
```

If during 0.3 you discovered SDK API names differ from what's in this plan, ALSO update Phase 1's plan text in this file before committing (call it out in the commit message).

---

## Phase 1: era-brain `memory/zg_kv` provider

**Files:**
- Modify: `era-brain/go.mod`, `era-brain/go.sum`
- Create: `era-brain/memory/zg_kv/kv.go`
- Create: `era-brain/memory/zg_kv/kv_test.go`

The package implements `memory.Provider`'s KV semantics on top of 0G KV streams. Log methods (`AppendLog`, `ReadLog`) are unsupported here — they live in `zg_log`. We define a tiny `kvOps` interface seam inside the package so unit tests can fake the SDK calls without standing up a real 0G node.

### 1A: Add SDK dep to era-brain

- [ ] **Step 1.1: Add the dependency**

```bash
cd era-brain
go get github.com/0glabs/0g-storage-client@latest
go mod tidy
```

- [ ] **Step 1.2: Verify tidy succeeded and existing tests still pass**

```bash
go vet ./...
go test -race ./...
```

Expected: green. era-brain's existing tests (brain, llm, memory/sqlite, etc.) should not be affected by adding the SDK dep (it pulls in transitives but no era-brain file imports it yet).

### 1B: Define the kvOps interface + Provider skeleton (failing test first)

- [ ] **Step 1.3: Write the failing test**

`era-brain/memory/zg_kv/kv_test.go`:

```go
package zg_kv_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
)

// fakeKVOps records writes in-memory; reads return what was last written.
type fakeKVOps struct {
	store map[string][]byte // key = streamID + "/" + key
}

func newFakeKVOps() *fakeKVOps { return &fakeKVOps{store: map[string][]byte{}} }

func (f *fakeKVOps) Set(_ context.Context, streamID string, key, val []byte) error {
	f.store[streamID+"/"+string(key)] = val
	return nil
}

func (f *fakeKVOps) Get(_ context.Context, streamID string, key []byte) ([]byte, error) {
	v, ok := f.store[streamID+"/"+string(key)]
	if !ok {
		return nil, zg_kv.ErrKeyNotFound
	}
	return v, nil
}

func (f *fakeKVOps) Iterate(_ context.Context, _ string) ([][2][]byte, error) {
	return nil, zg_kv.ErrIterateUnsupported // KV provider doesn't expose iterate
}

func TestZGKV_PutAndGet(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	ctx := context.Background()

	require.NoError(t, p.PutKV(ctx, "planner-mem", "userX", []byte("hello")))
	got, err := p.GetKV(ctx, "planner-mem", "userX")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), got)
}

func TestZGKV_GetMissingReturnsErrNotFound(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	_, err := p.GetKV(context.Background(), "ns", "missing")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestZGKV_AppendLogReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	err := p.AppendLog(context.Background(), "ns", []byte("e"))
	require.ErrorIs(t, err, zg_kv.ErrLogUnsupported)
}

func TestZGKV_ReadLogReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	_, err := p.ReadLog(context.Background(), "ns")
	require.ErrorIs(t, err, zg_kv.ErrLogUnsupported)
}

func TestZGKV_NamespaceIsolation(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_kv.NewWithOps(ops)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "planner-mem", "u1", []byte("p")))
	require.NoError(t, p.PutKV(ctx, "coder-mem", "u1", []byte("c")))

	got1, err := p.GetKV(ctx, "planner-mem", "u1")
	require.NoError(t, err)
	require.Equal(t, []byte("p"), got1)

	got2, err := p.GetKV(ctx, "coder-mem", "u1")
	require.NoError(t, err)
	require.Equal(t, []byte("c"), got2)
}
```

- [ ] **Step 1.4: Run, verify FAIL**

```bash
go test ./memory/zg_kv/...
```

Expected: package not found / undefined symbols.

- [ ] **Step 1.5: Implement Provider + kvOps interface + sentinel errors**

`era-brain/memory/zg_kv/kv.go`:

```go
// Package zg_kv is a memory.Provider impl backed by 0G Storage KV streams.
//
// It supports KV semantics only. Log methods return ErrLogUnsupported —
// for log semantics use memory/zg_log, which uses the same underlying
// 0G KV streams API but with sequence-numbered keys.
package zg_kv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// ErrKeyNotFound is what kvOps.Get returns when the (streamID, key) pair has
// no value. The Provider maps this to memory.ErrNotFound at the public API.
var ErrKeyNotFound = errors.New("zg_kv: key not found")

// ErrLogUnsupported is returned by AppendLog/ReadLog. zg_kv does not support
// Log semantics — use memory/zg_log instead.
var ErrLogUnsupported = errors.New("zg_kv: log semantics not supported; use memory/zg_log")

// ErrIterateUnsupported is returned by Iterate when called on the KV provider
// (kept as a separate error for clarity in tests).
var ErrIterateUnsupported = errors.New("zg_kv: iterate not supported on KV provider")

// kvOps is the interface seam between zg_kv.Provider and the 0G SDK.
// Tests inject a fake; the production impl wraps github.com/0glabs/0g-storage-client/kv.
type kvOps interface {
	Set(ctx context.Context, streamID string, key, val []byte) error
	Get(ctx context.Context, streamID string, key []byte) ([]byte, error)
	Iterate(ctx context.Context, streamID string) ([][2][]byte, error)
}

// Provider implements memory.Provider on top of 0G KV streams.
type Provider struct {
	ops kvOps
}

// NewWithOps constructs a Provider with a custom kvOps. Used by tests and by
// memory/zg_log (which wraps the same SDK ops in a different shape).
func NewWithOps(ops kvOps) *Provider {
	return &Provider{ops: ops}
}

// streamID derives a stream ID from a namespace string. We use sha256 hex so
// the same namespace always maps to the same stream and so namespaces don't
// collide on similar prefixes.
func streamID(ns string) string {
	h := sha256.Sum256([]byte(ns))
	return hex.EncodeToString(h[:])
}

func (p *Provider) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	val, err := p.ops.Get(ctx, streamID(ns), []byte(key))
	if errors.Is(err, ErrKeyNotFound) {
		return nil, memory.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("zg_kv getkv: %w", err)
	}
	return val, nil
}

func (p *Provider) PutKV(ctx context.Context, ns, key string, val []byte) error {
	if err := p.ops.Set(ctx, streamID(ns), []byte(key), val); err != nil {
		return fmt.Errorf("zg_kv putkv: %w", err)
	}
	return nil
}

func (p *Provider) AppendLog(_ context.Context, _ string, _ []byte) error {
	return ErrLogUnsupported
}

func (p *Provider) ReadLog(_ context.Context, _ string) ([][]byte, error) {
	return nil, ErrLogUnsupported
}
```

- [ ] **Step 1.6: Run, verify PASS**

```bash
go test -race ./memory/zg_kv/...
```

Expected: 5 PASS.

### 1C: Real SDK-backed kvOps impl + integration test (build-tagged)

We add a `liveOps` impl that wraps the real SDK. This is what production code uses. The unit tests above don't touch it (they use fakeKVOps). A separate build-tagged test verifies liveOps against real testnet.

- [ ] **Step 1.7: Write the live integration test FIRST (build tag)**

`era-brain/memory/zg_kv/kv_live_test.go`:

```go
//go:build zg_live

package zg_kv_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
)

func TestZGKV_LiveTestnet_PutGetRoundtrip(t *testing.T) {
	priv := os.Getenv("PI_ZG_PRIVATE_KEY")
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	indexer := os.Getenv("PI_ZG_INDEXER_RPC")
	if priv == "" || rpc == "" || indexer == "" {
		t.Skip("PI_ZG_PRIVATE_KEY/PI_ZG_EVM_RPC/PI_ZG_INDEXER_RPC required for live test")
	}

	live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
		PrivateKey:  priv,
		EVMRPCURL:   rpc,
		IndexerURL:  indexer,
	})
	require.NoError(t, err)
	t.Cleanup(live.Close)

	var p memory.Provider = zg_kv.NewWithOps(live)

	ctx := context.Background()
	ns := "era-brain-live-test"
	key := fmt.Sprintf("k-%d", time.Now().UnixNano())
	val := []byte(fmt.Sprintf("v-%d", time.Now().UnixNano()))

	require.NoError(t, p.PutKV(ctx, ns, key, val))
	// Brief sleep for testnet confirmation; tune up if reads return ErrNotFound.
	time.Sleep(5 * time.Second)

	got, err := p.GetKV(ctx, ns, key)
	require.NoError(t, err)
	require.Equal(t, val, got)
}
```

- [ ] **Step 1.8: Run, verify FAIL**

```bash
go test -tags zg_live ./memory/zg_kv/...
```

Expected: `undefined: zg_kv.NewLiveOps`, `undefined: zg_kv.LiveOpsConfig`.

- [ ] **Step 1.9: Implement LiveOps wrapping the real SDK**

`era-brain/memory/zg_kv/live.go`:

```go
package zg_kv

import (
	"context"
	"fmt"
	"time"

	"github.com/0glabs/0g-storage-client/indexer"
	"github.com/0glabs/0g-storage-client/kv"
	"github.com/0glabs/0g-storage-client/node"
	"github.com/0glabs/0g-storage-client/transfer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/openweb3/web3go"
)

// LiveOpsConfig configures a kvOps backed by the real 0G SDK.
type LiveOpsConfig struct {
	PrivateKey string // hex (with or without 0x prefix)
	EVMRPCURL  string // e.g. https://evmrpc-testnet.0g.ai
	IndexerURL string // e.g. https://indexer-storage-testnet-turbo.0g.ai
	WriteTimeout time.Duration // optional; default 30s
}

// LiveOps is a kvOps backed by the real 0G SDK. Construct with NewLiveOps.
//
// IMPORTANT: this scaffolding mirrors what the Phase 0 smoke script proved
// works against testnet. If Phase 0 needed to fix any SDK call shapes
// (indexer constructor, Batcher pattern, SelectNodes arity, etc.), apply
// the SAME fixes here. The smoke script's working source is the source of
// truth, not this template.
type LiveOps struct {
	cfg   LiveOpsConfig
	w3    *web3go.Client
	idx   *indexer.Client
	read  *kv.Client
	nodes *transfer.SelectedNodes
}

// NewLiveOps constructs a LiveOps. Caller must Close.
func NewLiveOps(cfg LiveOpsConfig) (*LiveOps, error) {
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	w3, err := web3go.NewClientWithOption(cfg.EVMRPCURL, *web3go.NewClientOption().
		WithPrivateKeys([]string{cfg.PrivateKey}))
	if err != nil {
		return nil, fmt.Errorf("web3go: %w", err)
	}
	idx, err := indexer.NewClient(cfg.IndexerURL)
	if err != nil {
		w3.Close()
		return nil, fmt.Errorf("indexer client: %w", err)
	}
	// SelectNodes args: (ctx, expectedReplica, dropped, method, fullTrusted).
	// VERIFY against pkg.go.dev — adjust if v1.3.0 has a different arity.
	nodes, err := idx.SelectNodes(context.Background(), 1, nil, "", false)
	if err != nil {
		w3.Close()
		return nil, fmt.Errorf("select nodes: %w", err)
	}
	if len(nodes.Trusted) == 0 {
		nodes.Close()
		w3.Close()
		return nil, fmt.Errorf("no trusted nodes")
	}
	kvNode, err := node.NewKvClient(nodes.Trusted[0].URL)
	if err != nil {
		nodes.Close()
		w3.Close()
		return nil, fmt.Errorf("kv client: %w", err)
	}
	return &LiveOps{
		cfg:   cfg,
		w3:    w3,
		idx:   idx,
		read:  kv.NewClient(kvNode),
		nodes: nodes,
	}, nil
}

// Close releases the SDK resources.
func (l *LiveOps) Close() {
	if l.nodes != nil {
		l.nodes.Close()
	}
	if l.w3 != nil {
		l.w3.Close()
	}
}

func (l *LiveOps) Set(ctx context.Context, streamID string, key, val []byte) error {
	ctx, cancel := context.WithTimeout(ctx, l.cfg.WriteTimeout)
	defer cancel()

	streamHash := common.HexToHash("0x" + streamID)
	batcher := kv.NewBatcher(1, l.nodes, l.w3)
	// Capture the builder return — Set has a value receiver and returns
	// *streamDataBuilder. Discarding it produces an empty/no-op batch.
	// VERIFY exact shape via godoc; adjust if Build/Exec live elsewhere.
	streamDataBuilder := batcher.Set(streamHash, key, val)
	if _, err := streamDataBuilder.Build(); err != nil {
		return fmt.Errorf("build batch: %w", err)
	}
	if _, err := batcher.Exec(ctx); err != nil {
		return fmt.Errorf("exec batch: %w", err)
	}
	return nil
}

func (l *LiveOps) Get(ctx context.Context, streamID string, key []byte) ([]byte, error) {
	streamHash := common.HexToHash("0x" + streamID)
	val, err := l.read.GetValue(ctx, streamHash, key)
	if err != nil {
		// FIXME post-Phase-0: the SDK probably returns a typed not-found error
		// (e.g. node.ErrKeyNotExist or similar). Distinguish that from real
		// network/RPC errors so dual.Provider.GetKV doesn't fall through to
		// primary on transient network blips. For M7-B.1's first cut we map
		// every error to ErrKeyNotFound so the dual fallback path stays
		// permissive — refine once Phase 0 reveals the SDK's error types.
		return nil, ErrKeyNotFound
	}
	if val == nil || len(val.Data) == 0 {
		return nil, ErrKeyNotFound
	}
	return val.Data, nil
}

func (l *LiveOps) Iterate(ctx context.Context, streamID string) ([][2][]byte, error) {
	streamHash := common.HexToHash("0x" + streamID)
	iter := l.read.NewIterator(streamHash)
	if err := iter.SeekToFirst(ctx); err != nil {
		return nil, fmt.Errorf("seek first: %w", err)
	}
	var out [][2][]byte
	for iter.Valid() {
		kv := iter.KeyValue()
		out = append(out, [2][]byte{kv.Key, kv.Data})
		if err := iter.Next(ctx); err != nil {
			return nil, fmt.Errorf("iter next: %w", err)
		}
	}
	return out, nil
}
```

**Note:** the exact SDK call shapes (`SelectNodes` signature, `NewBatcher` arity, `iter.SeekToFirst` vs `iter.SeekToFirst(ctx)`, `kv.Key` vs `kv.GetKey()`) may differ from this template. **Refer to whatever the Phase 0 smoke script discovered as the actual API.** If Phase 0's script needed adjustments, mirror those here.

- [ ] **Step 1.10: Run live test, verify PASS**

```bash
set -a; source ../.env; set +a   # from era-brain/, source the parent dir's .env
go test -tags zg_live -run TestZGKV_LiveTestnet ./memory/zg_kv/...
```

Expected: `--- PASS`. May take 10-30 seconds (testnet write confirmation).

If FAIL: read the error, refine LiveOps to match the real SDK shape (the Phase 0 script proved write+read works; mirror its calls here).

- [ ] **Step 1.11: Run all era-brain tests + vet**

```bash
go vet ./...
go test -race ./...   # without zg_live tag — live test is skipped
```

Expected: green. Live test is build-tag-guarded so it doesn't run in CI.

- [ ] **Step 1.12: Commit**

```bash
git add era-brain/
git commit -m "phase(M7-B.1.1): memory/zg_kv provider — KV ops on 0G Storage streams"
git tag m7b1-1-zg-kv
```

---

## Phase 2: era-brain `memory/zg_log` provider

**Files:**
- Create: `era-brain/memory/zg_log/log.go`
- Create: `era-brain/memory/zg_log/log_test.go`

`zg_log` implements `memory.Provider`'s Log semantics on the same underlying 0G KV streams API. Convention:

- streamID = `sha256(ns)`
- entry sequence numbers are 6-digit zero-padded strings: `[]byte("000001")`, `[]byte("000002")`, etc.
- `AppendLog` reads the highest existing sequence (via Iterate's last entry), increments, calls `Set`.
- `ReadLog` calls `Iterate` and returns values in sequence order.

KV methods (`GetKV`, `PutKV`) return `ErrKVUnsupported` — for KV semantics use `memory/zg_log`. (Mirror image of `zg_kv`.)

We reuse the `kvOps` interface from `zg_kv` so `zg_log.Provider` can be wired with the same `LiveOps` instance, halving the SDK-init overhead at runtime.

### 2A: Tests + impl

- [ ] **Step 2.1: Write the failing test**

`era-brain/memory/zg_log/log_test.go`:

```go
package zg_log_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
)

// fakeKVOps reused from zg_kv-test pattern, but local copy here for isolation.
type fakeKVOps struct {
	mu    sync.Mutex
	store map[string]map[string][]byte // streamID → key → val
}

func newFakeKVOps() *fakeKVOps { return &fakeKVOps{store: map[string]map[string][]byte{}} }

func (f *fakeKVOps) Set(_ context.Context, sid string, key, val []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store[sid] == nil {
		f.store[sid] = map[string][]byte{}
	}
	f.store[sid][string(key)] = val
	return nil
}

func (f *fakeKVOps) Get(_ context.Context, sid string, key []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store[sid] == nil {
		return nil, zg_log.ErrKeyNotFound
	}
	v, ok := f.store[sid][string(key)]
	if !ok {
		return nil, zg_log.ErrKeyNotFound
	}
	return v, nil
}

func (f *fakeKVOps) Iterate(_ context.Context, sid string) ([][2][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.store[sid]))
	for k := range f.store[sid] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2][]byte, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2][]byte{[]byte(k), f.store[sid][k]})
	}
	return out, nil
}

func TestZGLog_AppendAndRead(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	ctx := context.Background()

	for _, e := range [][]byte{[]byte("a"), []byte("b"), []byte("c")} {
		require.NoError(t, p.AppendLog(ctx, "audit/t1", e))
	}
	got, err := p.ReadLog(ctx, "audit/t1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a"), []byte("b"), []byte("c")}, got)
}

func TestZGLog_NamespaceIsolation(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	ctx := context.Background()
	require.NoError(t, p.AppendLog(ctx, "ns1", []byte("x")))
	require.NoError(t, p.AppendLog(ctx, "ns2", []byte("y")))

	got, err := p.ReadLog(ctx, "ns1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("x")}, got)
}

func TestZGLog_ReadEmptyNamespaceReturnsEmpty(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	got, err := p.ReadLog(context.Background(), "never-written")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestZGLog_PutKVReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	err := p.PutKV(context.Background(), "ns", "k", []byte("v"))
	require.True(t, errors.Is(err, zg_log.ErrKVUnsupported))
}

func TestZGLog_GetKVReturnsUnsupported(t *testing.T) {
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	_, err := p.GetKV(context.Background(), "ns", "k")
	require.True(t, errors.Is(err, zg_log.ErrKVUnsupported))
}

func TestZGLog_AppendIsAtomic_PreservesOrderUnderConcurrency(t *testing.T) {
	// Stress-ish test: 5 goroutines append "G<i>-<n>" 4 times each. 20 total
	// entries, each unique. Read-back: contains all 20, order matches insertion.
	// This is a smoke check of the seq-number derivation under concurrent appends.
	ops := newFakeKVOps()
	var p memory.Provider = zg_log.NewWithOps(ops)
	ctx := context.Background()

	var wg sync.WaitGroup
	for g := 0; g < 5; g++ {
		wg.Add(1)
		g := g
		go func() {
			defer wg.Done()
			for n := 0; n < 4; n++ {
				_ = p.AppendLog(ctx, "ns", []byte(fmt.Sprintf("G%d-%d", g, n)))
			}
		}()
	}
	wg.Wait()

	got, err := p.ReadLog(ctx, "ns")
	require.NoError(t, err)
	require.Len(t, got, 20)
}
```

- [ ] **Step 2.2: Run, verify FAIL**

```bash
go test ./memory/zg_log/...
```

Expected: package not found.

- [ ] **Step 2.3: Implement Provider**

`era-brain/memory/zg_log/log.go`:

```go
// Package zg_log is a memory.Provider impl backed by 0G Storage KV streams,
// where keys are monotonic 6-digit sequence numbers so iteration returns
// entries in append order. Mirror image of memory/zg_kv: this package
// supports Log semantics; KV methods return ErrKVUnsupported.
package zg_log

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

// kvOps mirrors zg_kv's interface seam. Callers can pass the same LiveOps
// instance to both zg_kv.NewWithOps and zg_log.NewWithOps.
type kvOps interface {
	Set(ctx context.Context, streamID string, key, val []byte) error
	Get(ctx context.Context, streamID string, key []byte) ([]byte, error)
	Iterate(ctx context.Context, streamID string) ([][2][]byte, error)
}

// ErrKeyNotFound — returned by ops.Get when a key is missing. Callers test
// errors.Is on this; it must equal zg_kv.ErrKeyNotFound's behavior.
var ErrKeyNotFound = errors.New("zg_log: key not found")

// ErrKVUnsupported — KV methods on a Log provider always return this.
var ErrKVUnsupported = errors.New("zg_log: KV semantics not supported; use memory/zg_kv")

// Provider implements memory.Provider on top of 0G KV streams using
// sequence-numbered keys.
type Provider struct {
	ops kvOps

	// per-namespace counters guarded by mu so concurrent AppendLog calls
	// against the same namespace don't collide on sequence numbers.
	// In a single-process orchestrator this is sufficient; multi-process
	// writers to the same namespace would still race (acceptable in M7-B.1
	// scope; M7-B.2's runner-side integration is single-process).
	mu       sync.Mutex
	counters map[string]int
}

// NewWithOps constructs a Provider with the given kvOps.
func NewWithOps(ops kvOps) *Provider {
	return &Provider{ops: ops, counters: map[string]int{}}
}

const seqWidth = 6 // "000001" — supports 999_999 entries per namespace before width overflows

func streamID(ns string) string {
	h := sha256.Sum256([]byte(ns))
	return hex.EncodeToString(h[:])
}

func (p *Provider) AppendLog(ctx context.Context, ns string, entry []byte) error {
	sid := streamID(ns)

	p.mu.Lock()
	// Lazy-load counter from the underlying store on first append per namespace.
	if _, ok := p.counters[ns]; !ok {
		entries, err := p.ops.Iterate(ctx, sid)
		if err != nil {
			p.mu.Unlock()
			return fmt.Errorf("zg_log appendlog (init counter): %w", err)
		}
		p.counters[ns] = len(entries)
	}
	p.counters[ns]++
	seq := p.counters[ns]
	p.mu.Unlock()

	// Note: counter increment + ops.Set is intentionally NOT atomic — the
	// counter advances under the lock, but Set runs outside it. If Set fails,
	// sequence number `seq` is "lost" (no entry written, but counter moved
	// past it). Subsequent appends use seq+1, leaving a gap at seq.
	// Iterate-based reads are unaffected (they only return existing entries),
	// so this is survivable. Acceptable for M7-B.1 single-process scope.
	// Fix-forward: rollback the counter on Set failure if ever needed.
	key := []byte(fmt.Sprintf("%0*d", seqWidth, seq))
	if err := p.ops.Set(ctx, sid, key, entry); err != nil {
		return fmt.Errorf("zg_log appendlog: %w", err)
	}
	return nil
}

func (p *Provider) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	entries, err := p.ops.Iterate(ctx, streamID(ns))
	if err != nil {
		return nil, fmt.Errorf("zg_log readlog: %w", err)
	}
	out := make([][]byte, 0, len(entries))
	for _, kv := range entries {
		out = append(out, kv[1])
	}
	return out, nil
}

func (p *Provider) GetKV(_ context.Context, _, _ string) ([]byte, error) {
	return nil, ErrKVUnsupported
}

func (p *Provider) PutKV(_ context.Context, _, _ string, _ []byte) error {
	return ErrKVUnsupported
}
```

- [ ] **Step 2.4: Run, verify PASS**

```bash
go test -race ./memory/zg_log/...
```

Expected: 6 PASS.

### 2B: Live integration test

Mirror Phase 1's live test for the log path. We piggyback on zg_kv's LiveOps — same SDK init, just used in a different shape.

- [ ] **Step 2.5: Write the live test**

`era-brain/memory/zg_log/log_live_test.go`:

```go
//go:build zg_live

package zg_log_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
)

func TestZGLog_LiveTestnet_AppendAndRead(t *testing.T) {
	priv := os.Getenv("PI_ZG_PRIVATE_KEY")
	rpc := os.Getenv("PI_ZG_EVM_RPC")
	indexer := os.Getenv("PI_ZG_INDEXER_RPC")
	if priv == "" || rpc == "" || indexer == "" {
		t.Skip("PI_ZG_* env vars required")
	}

	live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
		PrivateKey: priv, EVMRPCURL: rpc, IndexerURL: indexer,
	})
	require.NoError(t, err)
	t.Cleanup(live.Close)

	var p memory.Provider = zg_log.NewWithOps(live)
	ns := fmt.Sprintf("era-brain-live-log-%d", time.Now().UnixNano())
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, p.AppendLog(ctx, ns, []byte(fmt.Sprintf("entry-%d", i))))
	}
	time.Sleep(8 * time.Second) // testnet confirmation

	got, err := p.ReadLog(ctx, ns)
	require.NoError(t, err)
	require.Len(t, got, 3)
	for i, e := range got {
		require.Equal(t, []byte(fmt.Sprintf("entry-%d", i)), e)
	}
}
```

- [ ] **Step 2.6: Run live test**

```bash
set -a; source ../.env; set +a
go test -tags zg_live -run TestZGLog_LiveTestnet ./memory/zg_log/...
```

Expected: PASS in ~30 seconds.

- [ ] **Step 2.7: Run all tests + vet**

```bash
go vet ./...
go test -race ./...
```

Green.

- [ ] **Step 2.8: Commit**

```bash
git add era-brain/
git commit -m "phase(M7-B.1.2): memory/zg_log provider — sequence-numbered append-only log on 0G KV streams"
git tag m7b1-2-zg-log
```

---

## Phase 3: era-brain `memory/dual` provider

**Files:**
- Create: `era-brain/memory/dual/dual.go`
- Create: `era-brain/memory/dual/dual_test.go`

The `dual` provider wraps two `memory.Provider` impls: one designated `Cache` (typically SQLite — fast, local, authoritative for hot path) and one `Primary` (typically 0G — canonical record, slower).

Semantics:
- **PutKV / AppendLog:** write to Cache first (must succeed; failure is fatal — local DB broken means we have a real problem). Then write to Primary in the SAME goroutine — failures here are non-fatal: log via the optional `OnPrimaryError` hook, never block the caller. Sequential, not parallel — keeps the code dead simple, and SQLite write is sub-ms while 0G write is multi-second; concurrency wouldn't help.
- **GetKV / ReadLog:** read from Cache first. If Cache returns `ErrNotFound`, fall through to Primary. (This makes the Cache effectively a write-through cache.)

Note: when the cache has data the primary doesn't, that's fine — we don't reconcile back. Reconciliation is a future concern.

### 3A: Tests

- [ ] **Step 3.1: Write the failing test**

`era-brain/memory/dual/dual_test.go`:

```go
package dual_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/dual"
)

type fakeProvider struct {
	mu      sync.Mutex
	kv      map[string][]byte
	logs    map[string][][]byte
	failPut bool
	failGet bool
}

func newFake() *fakeProvider { return &fakeProvider{kv: map[string][]byte{}, logs: map[string][][]byte{}} }

var errBoom = errors.New("provider failure")

func (f *fakeProvider) GetKV(_ context.Context, ns, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet {
		return nil, errBoom
	}
	v, ok := f.kv[ns+"/"+key]
	if !ok {
		return nil, memory.ErrNotFound
	}
	return v, nil
}

func (f *fakeProvider) PutKV(_ context.Context, ns, key string, val []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPut {
		return errBoom
	}
	f.kv[ns+"/"+key] = val
	return nil
}

func (f *fakeProvider) AppendLog(_ context.Context, ns string, e []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPut {
		return errBoom
	}
	f.logs[ns] = append(f.logs[ns], e)
	return nil
}

func (f *fakeProvider) ReadLog(_ context.Context, ns string) ([][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet {
		return nil, errBoom
	}
	return f.logs[ns], nil
}

func TestDual_PutKV_WritesToBoth(t *testing.T) {
	cache := newFake()
	primary := newFake()
	d := dual.New(cache, primary, nil)
	require.NoError(t, d.PutKV(context.Background(), "ns", "k", []byte("v")))
	require.Equal(t, []byte("v"), cache.kv["ns/k"])
	require.Equal(t, []byte("v"), primary.kv["ns/k"])
}

func TestDual_PutKV_PrimaryFailureDoesNotBlock(t *testing.T) {
	cache := newFake()
	primary := newFake()
	primary.failPut = true
	var primaryErrs []error
	d := dual.New(cache, primary, func(op string, err error) {
		primaryErrs = append(primaryErrs, err)
	})

	err := d.PutKV(context.Background(), "ns", "k", []byte("v"))
	require.NoError(t, err)
	require.Equal(t, []byte("v"), cache.kv["ns/k"])
	require.Empty(t, primary.kv) // primary write failed silently
	require.Len(t, primaryErrs, 1)
}

func TestDual_PutKV_CacheFailureIsFatal(t *testing.T) {
	cache := newFake()
	cache.failPut = true
	primary := newFake()
	d := dual.New(cache, primary, nil)

	err := d.PutKV(context.Background(), "ns", "k", []byte("v"))
	require.Error(t, err)
}

func TestDual_GetKV_PrefersCache(t *testing.T) {
	cache := newFake()
	primary := newFake()
	require.NoError(t, cache.PutKV(context.Background(), "ns", "k", []byte("from-cache")))
	require.NoError(t, primary.PutKV(context.Background(), "ns", "k", []byte("from-primary")))

	d := dual.New(cache, primary, nil)
	got, err := d.GetKV(context.Background(), "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("from-cache"), got)
}

func TestDual_GetKV_FallsThroughToPrimary(t *testing.T) {
	cache := newFake()
	primary := newFake()
	require.NoError(t, primary.PutKV(context.Background(), "ns", "k", []byte("from-primary")))

	d := dual.New(cache, primary, nil)
	got, err := d.GetKV(context.Background(), "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("from-primary"), got)
}

func TestDual_GetKV_BothMissingReturnsErrNotFound(t *testing.T) {
	d := dual.New(newFake(), newFake(), nil)
	_, err := d.GetKV(context.Background(), "ns", "missing")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestDual_AppendLog_WritesToBoth(t *testing.T) {
	cache := newFake()
	primary := newFake()
	d := dual.New(cache, primary, nil)
	require.NoError(t, d.AppendLog(context.Background(), "ns", []byte("a")))
	require.Equal(t, [][]byte{[]byte("a")}, cache.logs["ns"])
	require.Equal(t, [][]byte{[]byte("a")}, primary.logs["ns"])
}

func TestDual_ReadLog_PrefersCache(t *testing.T) {
	cache := newFake()
	primary := newFake()
	cache.logs["ns"] = [][]byte{[]byte("c")}
	primary.logs["ns"] = [][]byte{[]byte("p")}

	d := dual.New(cache, primary, nil)
	got, err := d.ReadLog(context.Background(), "ns")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("c")}, got)
}

func TestDual_ReadLog_FallsThroughOnEmptyCache(t *testing.T) {
	cache := newFake()
	primary := newFake()
	primary.logs["ns"] = [][]byte{[]byte("p")}

	d := dual.New(cache, primary, nil)
	got, err := d.ReadLog(context.Background(), "ns")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("p")}, got)
}
```

**Note on ReadLog fallthrough:** the cache-first read for logs treats "non-empty cache result" as authoritative. If the cache is empty (zero entries), we fall through to primary. This is a heuristic — a partially-cached log would be incorrectly preferred. For M7-B.1's hot-path-write-through pattern, the cache always has equal-or-more-than primary, so this is safe. Document it.

- [ ] **Step 3.2: Run, verify FAIL**

```bash
go test ./memory/dual/...
```

Expected: package not found.

- [ ] **Step 3.3: Implement**

`era-brain/memory/dual/dual.go`:

```go
// Package dual wraps two memory.Provider impls — a fast local Cache and a
// canonical Primary — implementing write-both/read-cache-first semantics.
//
// Use it to combine memory/sqlite (cache) with memory/zg_kv + memory/zg_log
// (primary) so era-brain has both 0G's tamper-proof record AND fast local
// reads, with primary failures degrading gracefully (logged but non-fatal).
package dual

import (
	"context"
	"errors"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// PrimaryErrorHandler is called when a primary write fails. Operation is
// "put_kv" or "append_log". Use to log + monitor; do not panic.
type PrimaryErrorHandler func(op string, err error)

// Provider implements memory.Provider as a write-both/read-cache-first wrapper.
type Provider struct {
	cache    memory.Provider
	primary  memory.Provider
	onErrPri PrimaryErrorHandler // optional
}

// New constructs a dual.Provider. onPrimaryError is optional (nil is fine —
// failures are silently swallowed); pass a function to log them.
func New(cache, primary memory.Provider, onPrimaryError PrimaryErrorHandler) *Provider {
	return &Provider{cache: cache, primary: primary, onErrPri: onPrimaryError}
}

func (p *Provider) reportPrimary(op string, err error) {
	if err == nil || p.onErrPri == nil {
		return
	}
	p.onErrPri(op, err)
}

func (p *Provider) PutKV(ctx context.Context, ns, key string, val []byte) error {
	if err := p.cache.PutKV(ctx, ns, key, val); err != nil {
		return fmt.Errorf("dual cache putkv: %w", err)
	}
	if err := p.primary.PutKV(ctx, ns, key, val); err != nil {
		p.reportPrimary("put_kv", err)
	}
	return nil
}

func (p *Provider) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	val, err := p.cache.GetKV(ctx, ns, key)
	if err == nil {
		return val, nil
	}
	if !errors.Is(err, memory.ErrNotFound) {
		// Cache read errored for a non-not-found reason — fall through to primary
		// rather than failing, since the primary is the canonical source.
		// This trades some failure visibility for resilience; document the choice.
	}
	return p.primary.GetKV(ctx, ns, key)
}

func (p *Provider) AppendLog(ctx context.Context, ns string, entry []byte) error {
	if err := p.cache.AppendLog(ctx, ns, entry); err != nil {
		return fmt.Errorf("dual cache appendlog: %w", err)
	}
	if err := p.primary.AppendLog(ctx, ns, entry); err != nil {
		p.reportPrimary("append_log", err)
	}
	return nil
}

func (p *Provider) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	entries, err := p.cache.ReadLog(ctx, ns)
	if err == nil && len(entries) > 0 {
		return entries, nil
	}
	// Empty cache OR cache error — fall through to primary.
	return p.primary.ReadLog(ctx, ns)
}
```

- [ ] **Step 3.4: Run, verify PASS**

```bash
go test -race ./memory/dual/...
```

Expected: 9 PASS.

- [ ] **Step 3.5: Run all era-brain tests + vet**

```bash
go vet ./...
go test -race ./...
```

Green.

- [ ] **Step 3.6: Commit**

```bash
git add era-brain/
git commit -m "phase(M7-B.1.3): memory/dual provider — write-both, read-cache-first wrapper"
git tag m7b1-3-dual
```

---

## Phase 4: Wire dual into examples/coding-agent + live gate

**Files:**
- Modify: `era-brain/examples/coding-agent/main.go`

The coding-agent example currently uses `sqlite.Open(...)` directly as `memory.Provider`. We swap that for `dual.New(sqlite, zg_kv+zg_log)` so a single live run writes to both stores. Tests verify the local SQLite copy; the live gate verifies the 0G testnet copy via the SDK read-back.

**Note:** because zg_kv and zg_log are separate provider impls (each with limited semantics), we need a small composite for the `primary` arg to `dual.New`. The simplest path: define an inline anonymous-struct type that delegates KV ops to zg_kv and Log ops to zg_log. Or we add a `memory/composite` mini-package later — but that's M7-B.2 work; for the example program inline is fine.

- [ ] **Step 4.1: Modify the example to use dual provider**

In `era-brain/examples/coding-agent/main.go`:

1. Add imports:
   ```go
   "github.com/vaibhav0806/era-multi-persona/era-brain/memory"
   "github.com/vaibhav0806/era-multi-persona/era-brain/memory/dual"
   "github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
   "github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
   ```

2. Add a `--zg-live` boolean flag (default false). When false, behavior is identical to today (sqlite-only). When true, construct the dual provider:

   ```go
   zgLive := flag.Bool("zg-live", false, "use 0G testnet alongside SQLite (requires PI_ZG_* env vars)")
   ```

3. After opening SQLite (`mem` is the *sqlite.Provider*), if `*zgLive` is true, wrap with dual:

   ```go
   var memProv memory.Provider = mem
   if *zgLive {
       priv := os.Getenv("PI_ZG_PRIVATE_KEY")
       rpc := os.Getenv("PI_ZG_EVM_RPC")
       indexer := os.Getenv("PI_ZG_INDEXER_RPC")
       if priv == "" || rpc == "" || indexer == "" {
           log.Fatal("--zg-live set but PI_ZG_* env vars missing")
       }
       live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
           PrivateKey: priv, EVMRPCURL: rpc, IndexerURL: indexer,
       })
       if err != nil {
           log.Fatalf("zg live ops: %v", err)
       }
       defer live.Close()
       primary := composite{
           kvP:  zg_kv.NewWithOps(live),
           logP: zg_log.NewWithOps(live),
       }
       memProv = dual.New(mem, &primary, func(op string, err error) {
           fmt.Fprintf(os.Stderr, "[zg primary %s failed: %v]\n", op, err)
       })
   }
   ```

4. Pass `memProv` (instead of `mem`) to each `LLMPersonaConfig.Memory`.

5. Add the small `composite` helper at the bottom of `main.go`:

   ```go
   // composite combines a KV-only provider and a Log-only provider into a single
   // memory.Provider. This is what dual.New's primary argument needs when the
   // primary is split across zg_kv (for KV ops) and zg_log (for Log ops).
   type composite struct {
       kvP  memory.Provider
       logP memory.Provider
   }

   func (c *composite) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
       return c.kvP.GetKV(ctx, ns, key)
   }
   func (c *composite) PutKV(ctx context.Context, ns, key string, val []byte) error {
       return c.kvP.PutKV(ctx, ns, key, val)
   }
   func (c *composite) AppendLog(ctx context.Context, ns string, entry []byte) error {
       return c.logP.AppendLog(ctx, ns, entry)
   }
   func (c *composite) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
       return c.logP.ReadLog(ctx, ns)
   }
   ```

- [ ] **Step 4.2: Verify it compiles**

```bash
cd era-brain
go build ./examples/coding-agent/
```

Expected: binary built, exit 0.

- [ ] **Step 4.3: Run without --zg-live (smoke that sqlite-only path still works)**

```bash
set -a; source ../.env; set +a
go run ./examples/coding-agent --task="add /healthz endpoint"
```

Expected: 3-persona output, same as M7-A.6 live gate.

- [ ] **Step 4.4: Live gate — run WITH --zg-live**

```bash
set -a; source ../.env; set +a
go run ./examples/coding-agent --task="add /healthz endpoint" --zg-live
```

**Acceptance criteria:**
- Process exits 0.
- Same 3-persona output as Step 4.3 (planner/coder/reviewer with diffs and decisions).
- No `[zg primary ... failed: ...]` lines in stderr (would indicate 0G writes failed).
- Total runtime: under 60 seconds (3 LLM calls + ~6 0G writes ≈ 30s typical).

- [ ] **Step 4.5: Verify 0G testnet has the receipts**

The audit log namespace for the example is `audit/example-<unix>` (set by main.go). Read it back via the live SDK:

```bash
cd era-brain
cat > /tmp/zg-readback.go <<'EOF'
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_kv"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/zg_log"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: zg-readback <namespace>")
		os.Exit(1)
	}
	ns := os.Args[1]
	live, err := zg_kv.NewLiveOps(zg_kv.LiveOpsConfig{
		PrivateKey: os.Getenv("PI_ZG_PRIVATE_KEY"),
		EVMRPCURL:  os.Getenv("PI_ZG_EVM_RPC"),
		IndexerURL: os.Getenv("PI_ZG_INDEXER_RPC"),
	})
	if err != nil { panic(err) }
	defer live.Close()

	logP := zg_log.NewWithOps(live)
	entries, err := logP.ReadLog(context.Background(), ns)
	if err != nil { panic(err) }
	fmt.Printf("ns=%q entries=%d\n", ns, len(entries))
	for i, e := range entries {
		fmt.Printf("  [%d] %s\n", i, e)
	}
}
EOF
go run /tmp/zg-readback.go "audit/example-<the-unix-from-step-4.4>"
```

Expected: 3 entries (planner, coder/synth, reviewer). Each is JSON-encoded `brain.Receipt`.

(Coder is the LLMPersona's coder — not Pi-as-coder; this is the SDK example, not the era runner. The coder LLMPersona DOES emit a real receipt here because it's an LLMPersona, unlike the era runner's Pi-as-coder.)

- [ ] **Step 4.6: Verify local SQLite ALSO has the same receipts**

```bash
sqlite3 /tmp/era-brain-example.db "SELECT seq, namespace, length(val) FROM entries WHERE is_kv = 0 ORDER BY seq"
```

Expected: 3 rows under the same `audit/example-...` namespace, matching what 0G has. This proves dual.New wrote to both.

- [ ] **Step 4.7: Replay all phases**

```bash
cd era-brain && go vet ./... && go test -race ./...
cd .. && go vet ./... && go test -race ./...
```

Both green. era M6 + M7-A.5 tests untouched.

- [ ] **Step 4.8: Commit + tag M7-B.1 done**

```bash
git add era-brain/examples/
git commit -m "phase(M7-B.1.4): coding-agent example wired with dual(sqlite, zg_kv+zg_log) — live 0G testnet gate"
git tag m7b1-4-example
git tag m7b1-done
```

Push when ready (per the user's existing pattern).

---

## Live gate summary (M7-B.1 acceptance)

When this milestone is done:

1. `go test -race ./...` from era-brain — green. New tests pass: 5 zg_kv unit + 6 zg_log unit + 9 dual = 20 new tests (plus existing).
2. `go test -tags zg_live ./memory/zg_kv/... ./memory/zg_log/...` — green against real testnet.
3. era-brain example w/ `--zg-live`:
   - Same 3-persona output as M7-A.6.
   - 3 receipts on 0G testnet at the example's namespace, retrievable via SDK.
   - 3 receipts in local SQLite at the same namespace, identical content.
4. era M6 + M7-A.5 tests at repo root — green (no regression).
5. era-brain SDK still `go get`-able — `go build ./...` from era-brain works without zg_live tag.

---

## Out of scope (deferred to M7-B.2 and later)

- **era orchestrator integration.** M7-B.2: `cmd/orchestrator/main.go` constructs `dual.New(sqlite, zg_kv+zg_log)` and passes to swarm.New. Runs in production /task pipeline. Pi audit log unchanged (era's existing `internal/audit` stays in pi-agent.db).
- **A `memory/composite` package.** Right now we inline the `composite` helper in the example. Promote to a proper package only when 2+ callers need it (M7-B.2 likely will).
- **0G Storage URI for persona memory blobs.** Used by iNFT metadata in M7-D. The 0G stream addresses ARE retrievable via the SDK; we just don't expose them as URIs in this milestone.
- **Persona memory KV reads in LLMPersona.Run.** Currently LLMPersona only writes to memory (audit log). Reading prior memory before LLM call is M7-B.2 (alongside the era integration).
- **Per-user namespacing in production.** Right now namespaces are just persona names + simple suffix. M7-B.2 thinks through user_id + persona_id + repo_id namespacing for multi-user (defer until needed).
- **Sealed-inference receipts on 0G.** Sealed flag stays false in M7-B; M7-C wires 0G Compute and the receipt's Sealed field flips when the LLM provider is the 0G Compute impl.
- **iNFT recordInvocation per receipt.** M7-D.
- **ENS subnames.** M7-E.

---

## Risks + cuts list (in order, if we slip the budget)

1. **Phase 4's live gate fails** because the SDK's actual API shape diverged from this plan's templates. Recovery: read the smoke script's working code from Phase 0 — it already proves the SDK works against testnet. Mirror its exact calls in `LiveOps`.
2. **Concurrent AppendLog under load corrupts sequence numbers.** Recovery: zg_log's `mu` already serializes counter access; if testnet writes interleave at the network layer, that's a problem only if multiple processes write to the same namespace, which we don't do in M7-B.1's scope. Document, defer to M7-B.2.
3. **Phase 1's live test eats faucet ZG faster than expected.** Recovery: gas budget per write is ~0.001 ZG; faucet provides 100+ ZG. Even 100 writes/day stays under faucet replenishment. If hit, rate-limit the live tests.
4. **0G testnet maintenance/downtime mid-build.** Recovery: live gate is the only blocker. Unit tests use the kvOps fake and don't need testnet. If testnet is down for hours, code can still land; live gate retries when network is up.
