# M7-A — era-brain SDK Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a `go get`-able `era-brain` Go module (in-monorepo) with the six core abstractions (5 interfaces — `Persona`, `MemoryProvider`, `LLMProvider`, `INFTRegistry`, `IdentityResolver` — plus the concrete `Brain` orchestrator struct), SQLite + OpenRouter reference impls, a `Brain.Run(personas...)` chain runner, and a runnable `examples/coding-agent/` that demonstrates a 3-persona (planner → coder → reviewer) flow against a synthetic task — all in-process, no Docker, no real GitHub.

**Architecture:** Six linear phases. era-brain lives at `era-brain/` as its own Go module (`module github.com/vaibhav0806/era-multi-persona/era-brain`) inside the existing repo — separate `go.mod` so the SDK is independently `go get`-able and judges see a clean public API boundary. iNFT and Identity interfaces are defined in M7-A but implemented in M7-D / M7-E. Memory is `sqlite` only (no 0G yet). LLM is `openrouter` only (no 0G Compute). The era orchestrator does **not** import era-brain in M7-A — that work is deferred to M7-A.5. M7-A's live gate is `go run ./examples/coding-agent` producing realistic planner/coder/reviewer output against a real OpenRouter call.

**Tech Stack:** Go 1.25, modernc.org/sqlite (matches era's existing dep), net/http for OpenRouter client, stretchr/testify for asserts, Go's standard `httptest` for fakes. No new external dependencies.

**Spec:** `docs/superpowers/specs/2026-04-26-era-multi-persona-design.md` §1-§6 + the M7-A scope tightening confirmed by the user (SDK-only live gate; runner integration deferred to M7-A.5; monorepo subdir instead of separate repo).

**Testing philosophy:** Strict TDD. Failing test first, run it, see it fail with the expected error, write minimal code to pass, run it, see it pass, commit. `go test -race -count=1 ./...` green before every commit. No "I'll add the test after." No "trivial enough to skip the test." Subagent-driven execution per project philosophy.

**Prerequisites (check before starting):**
- Go 1.25.6 (`go version`).
- An OpenRouter API key with a few cents of credit available (set `OPENROUTER_API_KEY` for the live gate).
- This repo at clean tree state on `master` (no untracked, no unstaged).
- Existing era M6 tests still green — M7-A must not regress them: `go test -race ./...` from repo root.

---

## File Structure

```
era-brain/                                              CREATE (Phase 1)
├── go.mod                                              CREATE — module github.com/vaibhav0806/era-multi-persona/era-brain
├── go.sum                                              CREATE (auto)
├── README.md                                           CREATE — what era-brain is, how to install, link to top-level FEATURE.md
│
├── brain/
│   ├── brain.go                                        CREATE (Phase 5) — Brain struct + Run([]Persona)
│   ├── brain_test.go                                   CREATE (Phase 5)
│   ├── persona.go                                      CREATE (Phase 2) — Persona interface + types; rewritten in Phase 5 to add concrete LLMPersona
│   ├── persona_test.go                                 CREATE (Phase 5) — LLMPersona tests; Phase 2 has no persona test (interface-only)
│   ├── receipt.go                                      CREATE (Phase 2) — Receipt type, ReceiptHash helper
│   └── receipt_test.go                                 CREATE (Phase 2)
│
├── memory/
│   ├── provider.go                                     CREATE (Phase 2) — MemoryProvider interface + types
│   ├── provider_test.go                                CREATE (Phase 2) — interface contract test against fake
│   └── sqlite/
│       ├── sqlite.go                                   CREATE (Phase 3) — SQLite-backed impl
│       └── sqlite_test.go                              CREATE (Phase 3)
│
├── llm/
│   ├── provider.go                                     CREATE (Phase 2) — LLMProvider interface + Request/Response
│   ├── provider_test.go                                CREATE (Phase 2) — interface contract test against fake
│   └── openrouter/
│       ├── openrouter.go                               CREATE (Phase 4) — HTTP client to api.openrouter.ai
│       └── openrouter_test.go                          CREATE (Phase 4) — uses httptest.Server
│
├── inft/
│   └── provider.go                                     CREATE (Phase 2) — INFTRegistry interface (no impl in M7-A)
│
├── identity/
│   └── provider.go                                     CREATE (Phase 2) — IdentityResolver interface (no impl in M7-A)
│
└── examples/
    └── coding-agent/
        ├── main.go                                     CREATE (Phase 6) — CLI program demoing 3-persona flow
        ├── prompts.go                                  CREATE (Phase 6) — planner/coder/reviewer system prompts
        └── README.md                                   CREATE (Phase 6) — how to run the example
```

**No changes to existing era code in M7-A.** The orchestrator does not yet import era-brain — that's M7-A.5.

---

## Task 1: Bootstrap era-brain module

**Files:**
- Create: `era-brain/go.mod`
- Create: `era-brain/README.md`
- Create: `era-brain/.gitignore`

- [ ] **Step 1.1: Create the directory and initialize the module**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
mkdir -p era-brain
cd era-brain
go mod init github.com/vaibhav0806/era-multi-persona/era-brain
```

- [ ] **Step 1.2: Verify the module compiles (no code yet → trivially passes)**

Run from `era-brain/`:
```bash
go build ./...
```

Expected: no output (no packages yet). Exit 0.

- [ ] **Step 1.3: Write the README skeleton**

`era-brain/README.md`:
```markdown
# era-brain

> Modular agent brain SDK: swappable memory, LLM, iNFT, and identity providers.

`era-brain` is the framework powering [era-multi-persona](../). It defines six interfaces — `Persona`, `Brain`, `MemoryProvider`, `LLMProvider`, `INFTRegistry`, `IdentityResolver` — and ships reference implementations for SQLite, OpenRouter, 0G Storage (KV + Log), 0G Compute (sealed inference), ERC-7857 (forked), and ENS.

See [`examples/coding-agent`](./examples/coding-agent) for a working 3-persona pipeline.

## Install

```bash
go get github.com/vaibhav0806/era-multi-persona/era-brain
```

## Status

M7-A: skeleton + SQLite + OpenRouter impls. iNFT, ENS, and 0G providers are interface-only and arrive in M7-B through M7-E.
```

- [ ] **Step 1.4: Add a minimal .gitignore for the module**

`era-brain/.gitignore`:
```
*.test
*.out
coverage.txt
```

- [ ] **Step 1.5: Add stretchr/testify dependency (used by Task 2 onward)**

```bash
cd era-brain
go get github.com/stretchr/testify@v1.11.1
```

This populates `era-brain/go.sum` and adds the `require` to `era-brain/go.mod`. Without this, the `go test` runs in Task 2 will fail with "missing go.sum entry" rather than the expected "undefined symbol" error, breaking the TDD red signal.

- [ ] **Step 1.6: Commit**

From repo root:
```bash
git add era-brain/go.mod era-brain/go.sum era-brain/README.md era-brain/.gitignore
git commit -m "phase(M7-A.1): bootstrap era-brain Go module"
```

Tag for replay: `git tag m7a-1-bootstrap`

---

## Task 2: Define core interfaces (no impls)

**Files:**
- Create: `era-brain/brain/persona.go`
- Create: `era-brain/brain/persona_test.go`
- Create: `era-brain/brain/receipt.go`
- Create: `era-brain/brain/receipt_test.go`
- Create: `era-brain/memory/provider.go`
- Create: `era-brain/memory/provider_test.go`
- Create: `era-brain/llm/provider.go`
- Create: `era-brain/llm/provider_test.go`
- Create: `era-brain/inft/provider.go`
- Create: `era-brain/identity/provider.go`

This task defines all six interfaces from spec §3. iNFT and Identity get only interfaces (no impls, no tests in M7-A — impls land in M7-D/E). Memory and LLM get interfaces + contract tests (against fake impls written inside the test files). Persona/Receipt get the type definitions only; the concrete `LLMPersona` impl is built in Phase 5.

**Why one task for all six:** The interfaces are tightly coupled (Brain takes Personas, Personas take MemoryProvider + LLMProvider, all return Receipts). Defining them in one task lets the implementer hold the whole shape in head and avoid churn from mid-task signature changes.

### 2A: Receipt type + helper

- [ ] **Step 2.1: Write the failing test for Receipt + ReceiptHash**

`era-brain/brain/receipt_test.go`:
```go
package brain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReceiptHash_DeterministicForSameInputs(t *testing.T) {
	r1 := Receipt{Persona: "planner", Model: "qwen3.6-plus", InputHash: "abc", OutputHash: "def", Sealed: true, TimestampUnix: 1700000000}
	r2 := Receipt{Persona: "planner", Model: "qwen3.6-plus", InputHash: "abc", OutputHash: "def", Sealed: true, TimestampUnix: 1700000000}
	require.Equal(t, ReceiptHash(r1), ReceiptHash(r2))
}

func TestReceiptHash_DiffersWhenSealedFlagFlips(t *testing.T) {
	r1 := Receipt{Persona: "coder", Model: "x", InputHash: "i", OutputHash: "o", Sealed: true, TimestampUnix: 1}
	r2 := r1
	r2.Sealed = false
	require.NotEqual(t, ReceiptHash(r1), ReceiptHash(r2))
}

func TestReceiptHash_HexString64Chars(t *testing.T) {
	h := ReceiptHash(Receipt{Persona: "p", Model: "m", InputHash: "i", OutputHash: "o", Sealed: false, TimestampUnix: 1})
	require.Len(t, h, 64)
	require.NotContains(t, h, " ")
	require.True(t, strings.ContainsAny(h, "0123456789abcdef"))
}
```

- [ ] **Step 2.2: Run the test, verify FAIL**

```bash
cd era-brain && go test ./brain/...
```

Expected: compile error — undefined: `Receipt`, `ReceiptHash`.

- [ ] **Step 2.3: Implement Receipt + ReceiptHash minimally**

`era-brain/brain/receipt.go`:
```go
// Package brain defines the core interfaces and orchestration of era-brain.
package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Receipt records one persona invocation. M7-A produces unsealed receipts only;
// M7-C extends this with sealed-inference attestations from 0G Compute.
type Receipt struct {
	Persona       string // "planner", "coder", "reviewer", or custom
	Model         string // e.g. "qwen3.6-plus" or OpenRouter model id
	InputHash     string // sha256 of input prompt
	OutputHash    string // sha256 of output text
	Sealed        bool   // true only when the LLMProvider produced an attested receipt
	TimestampUnix int64  // unix seconds at completion
}

// ReceiptHash returns a deterministic 64-char hex digest over the receipt's fields.
// Used as the on-chain attestation key in M7-D's recordInvocation.
func ReceiptHash(r Receipt) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%t|%d", r.Persona, r.Model, r.InputHash, r.OutputHash, r.Sealed, r.TimestampUnix)
	return hex.EncodeToString(h.Sum(nil))
}
```

The package doc-comment lives here because `receipt.go` is the alphabetically-first file in `brain/`. Don't move it.

- [ ] **Step 2.4: Run the test, verify PASS**

```bash
go test ./brain/...
```

Expected: `ok  github.com/vaibhav0806/era-multi-persona/era-brain/brain  0.0Ns`.

### 2B: MemoryProvider interface + contract test

- [ ] **Step 2.5: Write the failing test using a fake impl**

`era-brain/memory/provider_test.go`:
```go
package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// fakeProvider is the in-memory reference impl used by the contract test.
// Real impls (sqlite, zg_kv, zg_log) must satisfy the same contract.
type fakeProvider struct {
	kv  map[string][]byte
	log map[string][][]byte
}

func newFake() *fakeProvider { return &fakeProvider{kv: map[string][]byte{}, log: map[string][][]byte{}} }

func (f *fakeProvider) GetKV(_ context.Context, ns, key string) ([]byte, error) {
	v, ok := f.kv[ns+"/"+key]
	if !ok {
		return nil, memory.ErrNotFound
	}
	return v, nil
}
func (f *fakeProvider) PutKV(_ context.Context, ns, key string, val []byte) error {
	f.kv[ns+"/"+key] = val
	return nil
}
func (f *fakeProvider) AppendLog(_ context.Context, ns string, entry []byte) error {
	f.log[ns] = append(f.log[ns], entry)
	return nil
}
func (f *fakeProvider) ReadLog(_ context.Context, ns string) ([][]byte, error) {
	return f.log[ns], nil
}

func TestMemoryProviderContract_KV_PutThenGetRoundtrips(t *testing.T) {
	var p memory.Provider = newFake()
	require.NoError(t, p.PutKV(context.Background(), "planner-mem", "userX", []byte(`{"prior_plans":[]}`)))
	got, err := p.GetKV(context.Background(), "planner-mem", "userX")
	require.NoError(t, err)
	require.Equal(t, []byte(`{"prior_plans":[]}`), got)
}

func TestMemoryProviderContract_KV_GetMissingReturnsErrNotFound(t *testing.T) {
	var p memory.Provider = newFake()
	_, err := p.GetKV(context.Background(), "planner-mem", "missing")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestMemoryProviderContract_Log_AppendThenReadInOrder(t *testing.T) {
	var p memory.Provider = newFake()
	ctx := context.Background()
	require.NoError(t, p.AppendLog(ctx, "audit/task42", []byte("a")))
	require.NoError(t, p.AppendLog(ctx, "audit/task42", []byte("b")))
	require.NoError(t, p.AppendLog(ctx, "audit/task42", []byte("c")))
	entries, err := p.ReadLog(ctx, "audit/task42")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a"), []byte("b"), []byte("c")}, entries)
}
```

- [ ] **Step 2.6: Run the test, verify FAIL**

```bash
go test ./memory/...
```

Expected: `undefined: memory.Provider`, `undefined: memory.ErrNotFound`.

- [ ] **Step 2.7: Define the interface + sentinel error**

`era-brain/memory/provider.go`:
```go
// Package memory defines the MemoryProvider interface and ships reference impls.
//
// A Provider exposes both KV (mutable) and Log (append-only) semantics so a single
// dependency injection point covers persona memory (KV) and audit history (Log).
// Real impls live in subpackages: memory/sqlite (M7-A), memory/zg_kv + memory/zg_log (M7-B).
package memory

import (
	"context"
	"errors"
)

// ErrNotFound is returned by GetKV when the (namespace, key) pair has no value.
var ErrNotFound = errors.New("memory: not found")

// Provider is the unified KV + Log interface. Impls must be safe for concurrent use.
//
// KV semantics: last-write-wins on (ns, key). Get of missing key returns ErrNotFound.
// Log semantics: append-only, ordered. ReadLog returns entries in insertion order.
type Provider interface {
	GetKV(ctx context.Context, ns, key string) ([]byte, error)
	PutKV(ctx context.Context, ns, key string, val []byte) error
	AppendLog(ctx context.Context, ns string, entry []byte) error
	ReadLog(ctx context.Context, ns string) ([][]byte, error)
}
```

- [ ] **Step 2.8: Run the test, verify PASS**

```bash
go test ./memory/...
```

Expected: 3 PASS.

### 2C: LLMProvider interface + contract test

- [ ] **Step 2.9: Write the failing test using a fake impl**

`era-brain/llm/provider_test.go`:
```go
package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

type fakeLLM struct {
	respText string
	model    string
	sealed   bool
}

func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	return llm.Response{
		Text:   f.respText + " (echo: " + strings.TrimSpace(req.UserPrompt) + ")",
		Model:  f.model,
		Sealed: f.sealed,
	}, nil
}

func TestLLMProviderContract_BasicComplete(t *testing.T) {
	var p llm.Provider = &fakeLLM{respText: "ok", model: "test-model", sealed: false}
	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "you are a helper",
		UserPrompt:   "hello",
	})
	require.NoError(t, err)
	require.Contains(t, resp.Text, "echo: hello")
	require.Equal(t, "test-model", resp.Model)
	require.False(t, resp.Sealed)
}

func TestLLMProviderContract_SealedFlagPropagates(t *testing.T) {
	var p llm.Provider = &fakeLLM{respText: "ok", model: "qwen3.6-plus", sealed: true}
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.True(t, resp.Sealed)
}
```

- [ ] **Step 2.10: Run, verify FAIL**

```bash
go test ./llm/...
```

Expected: `undefined: llm.Provider`, `undefined: llm.Request`.

- [ ] **Step 2.11: Define the interface + Request/Response**

`era-brain/llm/provider.go`:
```go
// Package llm defines the LLMProvider interface and ships reference impls.
// Real impls live in subpackages: llm/openrouter (M7-A), llm/zg_compute (M7-C).
package llm

import "context"

// Request is the minimal completion shape era-brain depends on.
// Tool-use, streaming, and function-calling are out of scope for M7-A and live as
// extensions on the impl side; brain orchestration only needs prompt → text.
type Request struct {
	SystemPrompt string
	UserPrompt   string
	Model        string  // optional override; empty = use Provider's default
	MaxTokens    int     // 0 = provider default
	Temperature  float32 // 0 = provider default
}

// Response is what a completion returns. Sealed=true only when the impl produced
// an attested receipt (M7-C 0G Compute path); openrouter impl always returns false.
type Response struct {
	Text       string
	Model      string // model the provider actually used (may differ from Request.Model)
	Sealed     bool
	InputHash  string // sha256 of (SystemPrompt+UserPrompt+Model); set by impls for receipt building
	OutputHash string // sha256 of Text; set by impls
}

// Provider is the LLM completion interface. Impls must be safe for concurrent use.
type Provider interface {
	Complete(ctx context.Context, req Request) (Response, error)
}
```

- [ ] **Step 2.12: Run, verify PASS**

```bash
go test ./llm/...
```

Expected: 2 PASS.

### 2D: Persona interface (type only)

- [ ] **Step 2.13: Define the Persona interface (no impl yet — concrete LLMPersona arrives in Phase 5)**

`era-brain/brain/persona.go`:
```go
package brain

import "context"

// Persona is one stage in a Brain run. It receives the threaded conversation
// state, produces output, and writes a Receipt. Impls choose how to use the
// underlying LLMProvider and MemoryProvider; brain only orchestrates the chain.
type Persona interface {
	Name() string
	Run(ctx context.Context, in Input) (Output, error)
}

// Input threads task context through the persona chain. Each successive persona
// sees prior personas' outputs in PriorOutputs (in order).
type Input struct {
	TaskID        string
	UserID        string
	TaskDescription string
	PriorOutputs  []Output // populated by Brain; planner sees [], coder sees [planner.Output], reviewer sees [planner.Output, coder.Output]
}

// Output is what a persona emits. Brain accumulates Outputs and threads them.
type Output struct {
	PersonaName string
	Text        string
	Receipt     Receipt
}
```

- [ ] **Step 2.14: Run all brain tests (should still pass)**

```bash
go test ./brain/...
```

Expected: receipt tests still PASS, persona has no tests of its own yet.

### 2E: INFTRegistry + IdentityResolver interfaces (no impls)

- [ ] **Step 2.15: Define INFTRegistry interface**

`era-brain/inft/provider.go`:
```go
// Package inft defines the iNFT (ERC-7857) registry interface.
// Reference impl in inft/zg_7857 lands in M7-D.
package inft

import "context"

// Persona binds a persona to an iNFT. Returned by Lookup, used by callers to know
// which token to recordInvocation against after a run.
type Persona struct {
	Name           string
	TokenID        string
	ContractAddr   string
	OwnerAddr      string
	SystemPromptURI string // 0G Storage URI to the persona's system prompt blob (M7-B)
}

// Registry exposes mint + lookup + invocation-recording. M7-A defines the
// interface; M7-D ships zg_7857 impl backed by a forked ERC-7857 contract.
type Registry interface {
	Mint(ctx context.Context, name, systemPromptURI string) (Persona, error)
	Lookup(ctx context.Context, ownerAddr, name string) (Persona, error)
	RecordInvocation(ctx context.Context, tokenID, receiptHash string) error
}
```

- [ ] **Step 2.16: Define IdentityResolver interface**

`era-brain/identity/provider.go`:
```go
// Package identity defines the IdentityResolver interface for persona name → metadata lookup.
// Reference impl in identity/ens lands in M7-E.
package identity

import "context"

// Resolution carries the result of a name → identity lookup.
type Resolution struct {
	Name             string // e.g. "coder.vaibhav-era.eth"
	INFTContractAddr string // text record from the resolver
	INFTTokenID      string
	MemoryURI        string // 0G Storage URI for the persona's memory blob (M7-B)
}

// Resolver does name → metadata lookups and (for owners) subname registration.
type Resolver interface {
	Resolve(ctx context.Context, name string) (Resolution, error)
	RegisterSubname(ctx context.Context, parent, label string, res Resolution) error
}
```

- [ ] **Step 2.17: Run all era-brain tests + go vet**

```bash
cd era-brain && go vet ./... && go test -race ./...
```

Expected: all PASS, no vet warnings.

- [ ] **Step 2.18: Commit**

From repo root:
```bash
git add era-brain/
git commit -m "phase(M7-A.2): define era-brain core interfaces (Persona, MemoryProvider, LLMProvider, Receipt, INFTRegistry, IdentityResolver)"
git tag m7a-2-interfaces
```

---

## Task 3: SQLite MemoryProvider impl

**Files:**
- Create: `era-brain/memory/sqlite/sqlite.go`
- Create: `era-brain/memory/sqlite/sqlite_test.go`

The SQLite impl is a single-table schema (`namespace TEXT, key TEXT, val BLOB, seq INTEGER PRIMARY KEY AUTOINCREMENT`). KV ops use `(ns, key)` with `INSERT OR REPLACE`. Log ops append a row with empty `key` and use `seq` for ordering. One table covers both shapes — keeps the impl tiny and the code path identical to what the 0G Storage impls (M7-B) will need.

- [ ] **Step 3.1: Write the failing test**

`era-brain/memory/sqlite/sqlite_test.go`:
```go
package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
)

func newProvider(t *testing.T) memory.Provider {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mem.db")
	p, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestSQLite_KV_PutAndGet(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "planner-mem", "u1", []byte("hello")))
	got, err := p.GetKV(ctx, "planner-mem", "u1")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), got)
}

func TestSQLite_KV_Overwrite(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "ns", "k", []byte("v1")))
	require.NoError(t, p.PutKV(ctx, "ns", "k", []byte("v2")))
	got, err := p.GetKV(ctx, "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("v2"), got)
}

func TestSQLite_KV_GetMissingErrNotFound(t *testing.T) {
	p := newProvider(t)
	_, err := p.GetKV(context.Background(), "ns", "nope")
	require.ErrorIs(t, err, memory.ErrNotFound)
}

func TestSQLite_Log_AppendAndRead(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	for _, e := range [][]byte{[]byte("a"), []byte("b"), []byte("c")} {
		require.NoError(t, p.AppendLog(ctx, "audit/t1", e))
	}
	entries, err := p.ReadLog(ctx, "audit/t1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a"), []byte("b"), []byte("c")}, entries)
}

func TestSQLite_Log_NamespaceIsolation(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.AppendLog(ctx, "ns1", []byte("a")))
	require.NoError(t, p.AppendLog(ctx, "ns2", []byte("b")))
	got, err := p.ReadLog(ctx, "ns1")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("a")}, got)
}

func TestSQLite_KVAndLog_DontInterfere(t *testing.T) {
	p := newProvider(t)
	ctx := context.Background()
	require.NoError(t, p.PutKV(ctx, "ns", "k", []byte("kv-val")))
	require.NoError(t, p.AppendLog(ctx, "ns", []byte("log-val")))

	v, err := p.GetKV(ctx, "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("kv-val"), v)

	entries, err := p.ReadLog(ctx, "ns")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("log-val")}, entries)
}
```

- [ ] **Step 3.2: Add the sqlite dep to era-brain go.mod**

```bash
cd era-brain
go get modernc.org/sqlite@v1.49.1
```

(stretchr/testify was already added in Step 1.5.)

- [ ] **Step 3.3: Run the test, verify FAIL**

```bash
go test ./memory/sqlite/...
```

Expected: `package github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite is not in std`.

- [ ] **Step 3.4: Implement minimally**

`era-brain/memory/sqlite/sqlite.go`:
```go
// Package sqlite is a SQLite-backed reference impl of memory.Provider.
// Used as the default in M7-A; supplanted (but never removed) by 0G impls in M7-B.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS entries (
  seq       INTEGER PRIMARY KEY AUTOINCREMENT,
  namespace TEXT NOT NULL,
  key       TEXT NOT NULL,
  val       BLOB NOT NULL,
  is_kv     INTEGER NOT NULL CHECK (is_kv IN (0,1))
);
CREATE INDEX IF NOT EXISTS idx_entries_kv ON entries(namespace, key) WHERE is_kv = 1;
CREATE INDEX IF NOT EXISTS idx_entries_log ON entries(namespace, seq) WHERE is_kv = 0;
`

// Provider is a memory.Provider backed by SQLite.
type Provider struct {
	db *sql.DB
}

// Open creates or opens a SQLite database at path. Caller must Close.
func Open(path string) (*Provider, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Provider{db: db}, nil
}

func (p *Provider) Close() error { return p.db.Close() }

func (p *Provider) GetKV(ctx context.Context, ns, key string) ([]byte, error) {
	var val []byte
	err := p.db.QueryRowContext(ctx,
		`SELECT val FROM entries WHERE namespace = ? AND key = ? AND is_kv = 1
		 ORDER BY seq DESC LIMIT 1`, ns, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, memory.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getkv: %w", err)
	}
	return val, nil
}

func (p *Provider) PutKV(ctx context.Context, ns, key string, val []byte) error {
	if _, err := p.db.ExecContext(ctx,
		`INSERT INTO entries(namespace, key, val, is_kv) VALUES(?,?,?,1)`,
		ns, key, val); err != nil {
		return fmt.Errorf("putkv: %w", err)
	}
	return nil
}

func (p *Provider) AppendLog(ctx context.Context, ns string, entry []byte) error {
	if _, err := p.db.ExecContext(ctx,
		`INSERT INTO entries(namespace, key, val, is_kv) VALUES(?,'',?,0)`,
		ns, entry); err != nil {
		return fmt.Errorf("appendlog: %w", err)
	}
	return nil
}

func (p *Provider) ReadLog(ctx context.Context, ns string) ([][]byte, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT val FROM entries WHERE namespace = ? AND is_kv = 0 ORDER BY seq ASC`, ns)
	if err != nil {
		return nil, fmt.Errorf("readlog: %w", err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var v []byte
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
```

`PutKV` uses INSERT-only (not REPLACE) and `GetKV` reads the most-recent row by `seq`. This is intentional: the same table behaves as Log (`AppendLog` reads all) and KV (`GetKV` reads latest). Saves a table; no real downside at era-brain scales.

- [ ] **Step 3.5: Run test, verify PASS**

```bash
go test -race ./memory/sqlite/...
```

Expected: 6 PASS.

- [ ] **Step 3.6: Run all era-brain tests + vet**

```bash
go vet ./... && go test -race ./...
```

Expected: all PASS, no warnings.

- [ ] **Step 3.7: Commit**

```bash
git add era-brain/memory/sqlite/ era-brain/go.mod era-brain/go.sum
git commit -m "phase(M7-A.3): SQLite memory.Provider impl with KV + Log semantics"
git tag m7a-3-sqlite
```

---

## Task 4: OpenRouter LLMProvider impl

**Files:**
- Create: `era-brain/llm/openrouter/openrouter.go`
- Create: `era-brain/llm/openrouter/openrouter_test.go`

OpenRouter's chat/completions API is OpenAI-compatible. We do not pull in the openai SDK — a 60-line `net/http` client is plenty and avoids a heavy dep. Tests use `httptest.Server` to fake the API; live calls only happen in Phase 6's example program.

- [ ] **Step 4.1: Write the failing test**

`era-brain/llm/openrouter/openrouter_test.go`:
```go
package openrouter_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/openrouter"
)

func TestOpenRouter_Complete_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"role":"system"`)
		require.Contains(t, string(body), `"role":"user"`)
		require.Contains(t, string(body), `"content":"sys-prompt"`)
		require.Contains(t, string(body), `"content":"user-prompt"`)
		require.Contains(t, string(body), `"model":"openai/gpt-4o-mini"`)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "the response"}},
			},
			"model": "openai/gpt-4o-mini",
		})
	}))
	defer srv.Close()

	p := openrouter.New(openrouter.Config{
		APIKey:       "test-key",
		BaseURL:      srv.URL,
		DefaultModel: "openai/gpt-4o-mini",
	})

	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "sys-prompt",
		UserPrompt:   "user-prompt",
	})
	require.NoError(t, err)
	require.Equal(t, "the response", resp.Text)
	require.Equal(t, "openai/gpt-4o-mini", resp.Model)
	require.False(t, resp.Sealed)
	require.NotEmpty(t, resp.InputHash)
	require.NotEmpty(t, resp.OutputHash)
}

func TestOpenRouter_Complete_PerRequestModelOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"model":"qwen/qwen3-30b"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "x"}}},
			"model":   "qwen/qwen3-30b",
		})
	}))
	defer srv.Close()
	p := openrouter.New(openrouter.Config{APIKey: "k", BaseURL: srv.URL, DefaultModel: "default-model"})
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x", Model: "qwen/qwen3-30b"})
	require.NoError(t, err)
	require.Equal(t, "qwen/qwen3-30b", resp.Model)
}

func TestOpenRouter_Complete_HTTPErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := openrouter.New(openrouter.Config{APIKey: "k", BaseURL: srv.URL, DefaultModel: "m"})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate limited"))
}

func TestOpenRouter_Complete_EmptyChoicesIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}, "model": "m"})
	}))
	defer srv.Close()
	p := openrouter.New(openrouter.Config{APIKey: "k", BaseURL: srv.URL, DefaultModel: "m"})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
}
```

- [ ] **Step 4.2: Run, verify FAIL**

```bash
go test ./llm/openrouter/...
```

Expected: package not found.

- [ ] **Step 4.3: Implement minimally**

`era-brain/llm/openrouter/openrouter.go`:
```go
// Package openrouter is a thin OpenRouter (OpenAI-compatible) client implementing llm.Provider.
package openrouter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

// Config holds the OpenRouter client setup.
type Config struct {
	APIKey       string        // required
	BaseURL      string        // default: https://openrouter.ai
	DefaultModel string        // required; per-request Model overrides
	HTTPTimeout  time.Duration // default: 60s
}

// Provider is an llm.Provider that talks to OpenRouter.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New constructs a Provider. Defaults BaseURL and HTTPTimeout if empty.
func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://openrouter.ai"
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 60 * time.Second
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: cfg.HTTPTimeout}}
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float32   `json:"temperature,omitempty"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
}

func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}
	body := chatReq{
		Model:       model,
		Messages:    []chatMsg{{Role: "system", Content: req.SystemPrompt}, {Role: "user", Content: req.UserPrompt}},
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal req: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/api/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return llm.Response{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return llm.Response{}, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return llm.Response{}, fmt.Errorf("parse resp: %w; body=%s", err, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("openrouter returned 0 choices: %s", string(respBody))
	}

	text := parsed.Choices[0].Message.Content
	usedModel := parsed.Model
	if usedModel == "" {
		usedModel = model
	}

	// inH hashes the *requested* model (which may have been the default) for
	// determinism — keeps the receipt stable even if the upstream API substitutes.
	// M7-C's sealed impl follows the same convention.
	inH := sha256Hex(req.SystemPrompt + "\x00" + req.UserPrompt + "\x00" + model)
	outH := sha256Hex(text)

	return llm.Response{
		Text:       text,
		Model:      usedModel,
		Sealed:     false,
		InputHash:  inH,
		OutputHash: outH,
	}, nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 4.4: Run, verify PASS**

```bash
go test -race ./llm/openrouter/...
```

Expected: 4 PASS.

- [ ] **Step 4.5: Run all era-brain tests**

```bash
go vet ./... && go test -race ./...
```

Expected: all PASS.

- [ ] **Step 4.6: Commit**

```bash
git add era-brain/llm/openrouter/
git commit -m "phase(M7-A.4): OpenRouter llm.Provider impl"
git tag m7a-4-openrouter
```

---

## Task 5: Brain orchestration + concrete LLMPersona

**Files:**
- Create: `era-brain/brain/brain.go`
- Create: `era-brain/brain/brain_test.go`
- Modify: `era-brain/brain/persona.go` — add concrete `LLMPersona` impl
- Modify: `era-brain/brain/persona_test.go` — add LLMPersona tests

`Brain.Run` is just a `for _, p := range personas { p.Run(...) }` loop with output threading + receipt accumulation. The interesting impl is `LLMPersona`: it composes a system prompt from (config) + (memory snapshot), submits to LLMProvider, builds a Receipt, optionally writes back to memory.

- [ ] **Step 5.1: Write the failing Brain test**

`era-brain/brain/brain_test.go`:
```go
package brain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

// stubPersona returns a fixed Output. Used to test Brain orchestration logic
// without spinning up real LLMs/memory.
type stubPersona struct {
	name string
	text string
}

func (s *stubPersona) Name() string { return s.name }
func (s *stubPersona) Run(_ context.Context, in brain.Input) (brain.Output, error) {
	return brain.Output{
		PersonaName: s.name,
		Text:        s.text + "(saw " + boolToStr(len(in.PriorOutputs) > 0) + " prior)",
		Receipt: brain.Receipt{
			Persona:       s.name,
			Model:         "stub",
			InputHash:     "i",
			OutputHash:    "o",
			Sealed:        false,
			TimestampUnix: 1,
		},
	}, nil
}

func boolToStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func TestBrain_Run_ChainsPersonasInOrderAndThreadsPriorOutputs(t *testing.T) {
	b := brain.New()
	personas := []brain.Persona{
		&stubPersona{name: "planner", text: "plan-out"},
		&stubPersona{name: "coder", text: "code-out"},
		&stubPersona{name: "reviewer", text: "review-out"},
	}

	res, err := b.Run(context.Background(), brain.Input{
		TaskID:          "t1",
		UserID:          "u1",
		TaskDescription: "do the thing",
	}, personas)
	require.NoError(t, err)

	require.Len(t, res.Outputs, 3)
	require.Equal(t, "planner", res.Outputs[0].PersonaName)
	require.Contains(t, res.Outputs[0].Text, "saw no prior")
	require.Equal(t, "coder", res.Outputs[1].PersonaName)
	require.Contains(t, res.Outputs[1].Text, "saw yes prior")
	require.Equal(t, "reviewer", res.Outputs[2].PersonaName)
	require.Contains(t, res.Outputs[2].Text, "saw yes prior")

	require.Len(t, res.Receipts, 3)
	require.Equal(t, "planner", res.Receipts[0].Persona)
}

type errorPersona struct{}

func (errorPersona) Name() string { return "boom" }
func (errorPersona) Run(_ context.Context, _ brain.Input) (brain.Output, error) {
	return brain.Output{}, context.DeadlineExceeded
}

func TestBrain_Run_StopsOnFirstPersonaError(t *testing.T) {
	b := brain.New()
	personas := []brain.Persona{
		&stubPersona{name: "planner", text: "ok"},
		errorPersona{},
		&stubPersona{name: "should-not-run", text: "never"},
	}
	res, err := b.Run(context.Background(), brain.Input{TaskID: "t1"}, personas)
	require.Error(t, err)
	require.Len(t, res.Outputs, 1) // planner ran successfully; reviewer never started
}

func TestBrain_Run_EmptyPersonaListError(t *testing.T) {
	b := brain.New()
	_, err := b.Run(context.Background(), brain.Input{}, nil)
	require.Error(t, err)
}
```

- [ ] **Step 5.2: Run, verify FAIL**

```bash
go test ./brain/...
```

Expected: `undefined: brain.New`, `undefined: brain.Result`.

- [ ] **Step 5.3: Implement Brain**

`era-brain/brain/brain.go`:
```go
package brain

import (
	"context"
	"errors"
	"fmt"
)

// Brain orchestrates a sequential chain of Personas. Each Persona sees prior
// Personas' Outputs in the order they ran. Brain accumulates Receipts and stops
// at the first error (subsequent Personas don't run).
type Brain struct{}

// New returns a Brain. It's stateless; the type exists for forward-compatibility
// (M7-B may add Brain-level memory hooks).
func New() *Brain { return &Brain{} }

// Result is what Brain.Run returns: the per-persona outputs (in run order) and
// flattened receipts (in run order).
type Result struct {
	Outputs  []Output
	Receipts []Receipt
}

// Run executes the persona chain. Returns whatever outputs/receipts completed
// successfully even on partial failure — caller can inspect Result.Outputs to
// see how far the chain progressed before erroring.
func (b *Brain) Run(ctx context.Context, in Input, personas []Persona) (Result, error) {
	if len(personas) == 0 {
		return Result{}, errors.New("brain: empty persona list")
	}
	var res Result
	for _, p := range personas {
		stepIn := in
		stepIn.PriorOutputs = append([]Output(nil), res.Outputs...)
		out, err := p.Run(ctx, stepIn)
		if err != nil {
			return res, fmt.Errorf("persona %q failed: %w", p.Name(), err)
		}
		res.Outputs = append(res.Outputs, out)
		res.Receipts = append(res.Receipts, out.Receipt)
	}
	return res, nil
}
```

- [ ] **Step 5.4: Run, verify PASS**

```bash
go test -race ./brain/...
```

Expected: 3 PASS for Brain tests + receipt tests still PASS.

- [ ] **Step 5.5: Write the failing LLMPersona test**

Create `era-brain/brain/persona_test.go` (the file did not exist in Phase 2 — only the interface lived in `persona.go`):
```go
package brain_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

type recordingLLM struct {
	lastReq llm.Request
	resp    string
}

func (r *recordingLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	r.lastReq = req
	return llm.Response{Text: r.resp, Model: "test-m", InputHash: "ih", OutputHash: "oh", Sealed: false}, nil
}

type spyMem struct {
	puts map[string][]byte
	logs map[string][][]byte
}

func newSpyMem() *spyMem { return &spyMem{puts: map[string][]byte{}, logs: map[string][][]byte{}} }
func (s *spyMem) GetKV(_ context.Context, ns, key string) ([]byte, error) {
	v, ok := s.puts[ns+"/"+key]
	if !ok {
		return nil, memory.ErrNotFound
	}
	return v, nil
}
func (s *spyMem) PutKV(_ context.Context, ns, key string, val []byte) error {
	s.puts[ns+"/"+key] = val
	return nil
}
func (s *spyMem) AppendLog(_ context.Context, ns string, e []byte) error {
	s.logs[ns] = append(s.logs[ns], e)
	return nil
}
func (s *spyMem) ReadLog(_ context.Context, ns string) ([][]byte, error) { return s.logs[ns], nil }

func TestLLMPersona_Run_ComposesPromptFromConfigAndPriorOutputs(t *testing.T) {
	rec := &recordingLLM{resp: "PLAN_OUTPUT"}
	mem := newSpyMem()
	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name:         "planner",
		SystemPrompt: "you are the planner",
		Model:        "test-m",
		LLM:          rec,
		Memory:       mem,
		Now:          func() time.Time { return time.Unix(1700000000, 0) },
	})

	out, err := p.Run(context.Background(), brain.Input{
		TaskID:          "t1",
		TaskDescription: "add JWT auth",
	})
	require.NoError(t, err)

	require.Equal(t, "planner", out.PersonaName)
	require.Equal(t, "PLAN_OUTPUT", out.Text)
	require.Contains(t, rec.lastReq.SystemPrompt, "you are the planner")
	require.Contains(t, rec.lastReq.UserPrompt, "add JWT auth")
	require.Equal(t, int64(1700000000), out.Receipt.TimestampUnix)
	require.Equal(t, "planner", out.Receipt.Persona)
}

func TestLLMPersona_Run_IncludesPriorOutputsInPrompt(t *testing.T) {
	rec := &recordingLLM{resp: "REVIEW"}
	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name:         "reviewer",
		SystemPrompt: "you review",
		Model:        "test-m",
		LLM:          rec,
		Memory:       newSpyMem(),
		Now:          time.Now,
	})
	_, err := p.Run(context.Background(), brain.Input{
		TaskID: "t1",
		PriorOutputs: []brain.Output{
			{PersonaName: "planner", Text: "PLAN_TEXT"},
			{PersonaName: "coder", Text: "CODE_TEXT"},
		},
	})
	require.NoError(t, err)
	require.True(t, strings.Contains(rec.lastReq.UserPrompt, "PLAN_TEXT"),
		"reviewer prompt should include planner's output")
	require.True(t, strings.Contains(rec.lastReq.UserPrompt, "CODE_TEXT"),
		"reviewer prompt should include coder's output")
}

func TestLLMPersona_Run_AppendsReceiptToLog(t *testing.T) {
	rec := &recordingLLM{resp: "x"}
	mem := newSpyMem()
	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name:         "planner",
		SystemPrompt: "x",
		Model:        "m",
		LLM:          rec,
		Memory:       mem,
		Now:          time.Now,
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1"})
	require.NoError(t, err)
	require.Len(t, mem.logs["audit/t1"], 1, "one receipt log entry per persona run")
}
```

- [ ] **Step 5.6: Run, verify FAIL**

```bash
go test ./brain/...
```

Expected: `undefined: brain.NewLLMPersona`.

- [ ] **Step 5.7: Implement LLMPersona**

Replace `era-brain/brain/persona.go` with:
```go
package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// Persona is one stage in a Brain run. It receives the threaded conversation
// state, produces output, and writes a Receipt. Impls choose how to use the
// underlying LLMProvider and MemoryProvider; brain only orchestrates the chain.
type Persona interface {
	Name() string
	Run(ctx context.Context, in Input) (Output, error)
}

// Input threads task context through the persona chain.
type Input struct {
	TaskID          string
	UserID          string
	TaskDescription string
	PriorOutputs    []Output
}

// Output is what a persona emits.
type Output struct {
	PersonaName string
	Text        string
	Receipt     Receipt
}

// LLMPersonaConfig configures a concrete LLM-backed Persona.
type LLMPersonaConfig struct {
	Name         string
	SystemPrompt string
	Model        string // passed as Request.Model; empty = LLM provider default
	LLM          llm.Provider
	Memory       memory.Provider // used for audit-log writes; KV reads land in M7-B
	Now          func() time.Time
}

// LLMPersona is the standard Persona impl: builds a prompt from config +
// PriorOutputs, calls the LLM, writes a receipt to the audit log.
type LLMPersona struct {
	cfg LLMPersonaConfig
}

// NewLLMPersona constructs an LLMPersona. Now defaults to time.Now.
func NewLLMPersona(cfg LLMPersonaConfig) *LLMPersona {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &LLMPersona{cfg: cfg}
}

func (p *LLMPersona) Name() string { return p.cfg.Name }

func (p *LLMPersona) Run(ctx context.Context, in Input) (Output, error) {
	user := buildUserPrompt(in)
	resp, err := p.cfg.LLM.Complete(ctx, llm.Request{
		SystemPrompt: p.cfg.SystemPrompt,
		UserPrompt:   user,
		Model:        p.cfg.Model,
	})
	if err != nil {
		return Output{}, fmt.Errorf("llm complete: %w", err)
	}
	r := Receipt{
		Persona:       p.cfg.Name,
		Model:         resp.Model,
		InputHash:     resp.InputHash,
		OutputHash:    resp.OutputHash,
		Sealed:        resp.Sealed,
		TimestampUnix: p.cfg.Now().Unix(),
	}
	if p.cfg.Memory != nil && in.TaskID != "" {
		entry, _ := json.Marshal(r)
		_ = p.cfg.Memory.AppendLog(ctx, "audit/"+in.TaskID, entry)
	}
	return Output{PersonaName: p.cfg.Name, Text: resp.Text, Receipt: r}, nil
}

func buildUserPrompt(in Input) string {
	var b strings.Builder
	if in.TaskDescription != "" {
		b.WriteString("Task: ")
		b.WriteString(in.TaskDescription)
		b.WriteString("\n\n")
	}
	for _, o := range in.PriorOutputs {
		b.WriteString("--- ")
		b.WriteString(o.PersonaName)
		b.WriteString(" output ---\n")
		b.WriteString(o.Text)
		b.WriteString("\n\n")
	}
	return b.String()
}
```

- [ ] **Step 5.8: Run, verify PASS**

```bash
go test -race ./brain/...
```

Expected: all PASS (Brain + LLMPersona + Receipt).

- [ ] **Step 5.9: Run all era-brain tests + vet**

```bash
go vet ./... && go test -race ./...
```

Expected: all PASS.

- [ ] **Step 5.10: Commit**

```bash
git add era-brain/brain/
git commit -m "phase(M7-A.5): Brain.Run orchestration + LLMPersona impl"
git tag m7a-5-brain
```

---

## Task 6: coding-agent example + live gate

**Files:**
- Create: `era-brain/examples/coding-agent/main.go`
- Create: `era-brain/examples/coding-agent/prompts.go`
- Create: `era-brain/examples/coding-agent/README.md`

The example program:
1. Reads `OPENROUTER_API_KEY` from env.
2. Reads task description from `--task` flag.
3. Builds three LLMPersonas (planner / coder / reviewer) with hardcoded prompts.
4. Runs `Brain.Run` against them.
5. Prints each persona's output + receipt to stdout.

Coder doesn't actually edit files in M7-A — it produces a *proposed* unified diff as text in its output. Reviewer reviews that diff text. Real file ops return in M7-A.5 when the runner integration lands.

**Why no test file:** The example is glued together at the cmd level and depends on a real OpenRouter key. The components it composes (Brain, LLMPersona, OpenRouter, SQLite) are already covered by tests in Tasks 2-5. The live gate is the test for this task.

- [ ] **Step 6.1: Write the prompts file**

`era-brain/examples/coding-agent/prompts.go`:
```go
package main

const plannerSystemPrompt = `You are the PLANNER persona. Given a coding task and a target repo (which you will not see), produce a numbered step list (3-7 steps) describing what code changes are needed. Be specific about files and behaviors. Do not write code yet. Output ONLY the numbered list.`

const coderSystemPrompt = `You are the CODER persona. You will see the planner's step list. Produce a unified diff (in git diff format, with --- and +++ headers and @@ hunks) that implements the plan against a hypothetical existing codebase. Invent file paths and surrounding context as needed. Do not include explanations outside the diff. Output ONLY the diff.`

const reviewerSystemPrompt = `You are the REVIEWER persona. You will see the planner's plan and the coder's proposed diff. Critique the diff: flag (a) any test removals or skips, (b) any weakened assertions, (c) any deviations from the plan, and (d) anything that looks like it would not compile or run. End your output with a single line of either "DECISION: approve" or "DECISION: flag" based on whether the diff is safe to land. If you find no issues, write "no issues found" before the decision line.`
```

- [ ] **Step 6.2: Write the main file**

`era-brain/examples/coding-agent/main.go`:
```go
// coding-agent demonstrates the era-brain 3-persona flow against a synthetic task.
//
// Usage:
//
//	OPENROUTER_API_KEY=sk-... go run ./examples/coding-agent --task="add JWT auth to /login endpoint"
//
// Output: planner plan, coder diff, reviewer critique + decision, plus per-persona receipts.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/openrouter"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory/sqlite"
)

func main() {
	task := flag.String("task", "", "task description (required)")
	model := flag.String("model", "openai/gpt-4o-mini", "OpenRouter model id")
	flag.Parse()
	if *task == "" {
		log.Fatal("--task is required")
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY is required")
	}

	dbPath := filepath.Join(os.TempDir(), "era-brain-example.db")
	mem, err := sqlite.Open(dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer mem.Close()
	fmt.Fprintf(os.Stderr, "[memory db: %s]\n", dbPath)

	llmProv := openrouter.New(openrouter.Config{
		APIKey:       apiKey,
		DefaultModel: *model,
	})

	personas := []brain.Persona{
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "planner",
			SystemPrompt: plannerSystemPrompt,
			LLM:          llmProv,
			Memory:       mem,
			Now:          time.Now,
		}),
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "coder",
			SystemPrompt: coderSystemPrompt,
			LLM:          llmProv,
			Memory:       mem,
			Now:          time.Now,
		}),
		brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "reviewer",
			SystemPrompt: reviewerSystemPrompt,
			LLM:          llmProv,
			Memory:       mem,
			Now:          time.Now,
		}),
	}

	b := brain.New()
	taskID := fmt.Sprintf("example-%d", time.Now().Unix())
	res, err := b.Run(context.Background(), brain.Input{
		TaskID:          taskID,
		UserID:          "local",
		TaskDescription: *task,
	}, personas)
	if err != nil {
		log.Fatalf("brain run: %v", err)
	}

	for _, o := range res.Outputs {
		fmt.Printf("\n========== %s ==========\n", o.PersonaName)
		fmt.Println(o.Text)
		fmt.Printf("\n[receipt: model=%s sealed=%t hash=%s]\n",
			o.Receipt.Model, o.Receipt.Sealed, brain.ReceiptHash(o.Receipt))
	}
}
```

- [ ] **Step 6.3: Verify it compiles**

```bash
cd era-brain && go build ./examples/coding-agent/
```

Expected: `era-brain/examples/coding-agent/coding-agent` binary; exit 0.

- [ ] **Step 6.4: Write the example README**

`era-brain/examples/coding-agent/README.md`:
```markdown
# coding-agent example

Demonstrates a 3-persona flow (planner → coder → reviewer) using era-brain.

## Run

```bash
export OPENROUTER_API_KEY=sk-or-v1-...
go run ./examples/coding-agent --task="add a /healthz endpoint that returns 200 OK"
```

## What you'll see

- **Planner** lists the steps to implement the task.
- **Coder** produces a unified diff implementing those steps.
- **Reviewer** critiques the diff and ends with `DECISION: approve` or `DECISION: flag`.
- Each persona's receipt prints below its output (model, sealed flag, hash).

## What this is NOT

This example does **not** edit real files or open PRs — that integration lives in the [era orchestrator](../../..) and arrives in M7-A.5. The point of this example is to validate the era-brain abstraction in-process.
```

- [ ] **Step 6.5: Live gate — run the example against real OpenRouter**

```bash
export OPENROUTER_API_KEY=sk-or-v1-... # your real key
cd era-brain
go run ./examples/coding-agent --task="add a /healthz endpoint that returns 200 OK in a Go HTTP server"
```

**Acceptance criteria:**
- Process exits 0.
- Three sections print: `========== planner ==========`, `========== coder ==========`, `========== reviewer ==========`.
- Planner output is a numbered list of 3-7 steps.
- Coder output contains `---`, `+++`, and `@@` lines (unified diff format).
- Reviewer output ends with either `DECISION: approve` or `DECISION: flag`.
- Each section prints a `[receipt: model=… sealed=false hash=…]` footer with a 64-char hex hash.
- Total LLM cost: under $0.05 (3 short calls to gpt-4o-mini ≈ ~$0.001).

**If any criterion fails:** check OpenRouter key validity, model id, network. Don't paper over with mocks — fix the underlying issue.

- [ ] **Step 6.6: Replay all prior phases**

```bash
cd era-brain && go vet ./... && go test -race ./...
cd .. && go vet ./... && go test -race ./...
```

Both must be green. era's existing test suite must not have regressed.

- [ ] **Step 6.7: Commit + tag M7-A done**

```bash
git add era-brain/examples/
git commit -m "phase(M7-A.6): coding-agent example demonstrating 3-persona flow against OpenRouter"
git tag m7a-6-example
git tag m7a-done
```

---

## Live gate summary (M7-A acceptance)

When this milestone is done:

1. `cd era-brain && go test -race ./...` — green.
2. `go test -race ./...` from repo root — green (era's existing suite unchanged).
3. `OPENROUTER_API_KEY=… go run ./era-brain/examples/coding-agent --task="..."` — produces coherent planner/coder/reviewer output with three `Sealed: false` receipts.
4. `era-brain/` is its own Go module — `go build ./era-brain/...` works from the era-brain directory without needing era's go.mod.
5. The era orchestrator binary (`bin/orchestrator`) builds cleanly and existing /task flow still works end-to-end on the live VPS — **no regression** on M6 product.

---

## Out of scope (deferred to M7-A.5 or later)

- Wiring `era-brain` into `internal/runner/` so real /task uses it instead of monolithic Pi. (M7-A.5)
- Persona memory reads in `LLMPersona.Run` — KV-fetch before LLM call. M7-A only writes the audit log; M7-B introduces real persona memory carried across tasks.
- Per-persona model overrides at the orchestrator level (CLI/env config). M7-C wires this once 0G Compute is online.
- iNFT and Identity impls. Interfaces only in M7-A.
- A `chat-agent` or `audit-agent` example. M7-F.
- Receipt verification (currently `Sealed=false` always with M7-A's openrouter impl).
