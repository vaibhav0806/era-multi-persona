# M7-C.1 — era-brain 0G Compute SDK Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 0G Compute sealed inference as an `llm.Provider` impl in era-brain (`llm/zg_compute`), plus a generic `llm/fallback` wrapper. Live gate: `examples/coding-agent --zg-compute` produces planner+coder+reviewer output where the LLMPersona receipts show `Sealed: true` from real testnet sealed-inference calls.

**Architecture:** Four phases. Phase 0 is no-code setup: install `0g-compute-cli` (Node.js), deposit ZG to broker, generate bearer token, identify provider endpoint URL, run smoke script that proves bearer-auth + TEE-signed response works against testnet. Phases 1-3 mirror M7-B.1's TDD cadence: HTTP-client `Provider` with `httptest.Server` unit tests + `zg_live` integration test, then `fallback` wrapper as pure Go logic, then example wiring.

**Tech Stack:** Go 1.25, stdlib `net/http` (no Go SDK exists for 0G Compute), existing era-brain packages. No new external deps.

**Spec:** `docs/superpowers/specs/2026-04-26-m7c-sealed-inference-design.md`. All §-references below point at the spec.

**Testing philosophy:** Strict TDD. Failing test → run → verify FAIL → minimal impl → run → verify PASS → commit. Live testnet gate at the end (Phase 3). `zg_live`-tagged tests skip in CI; only run when env vars present.

**Prerequisites (check before starting):**
- M7-B.3 complete (tag `m7b3-done`).
- 0G testnet wallet has ≥3 ZG (existing wallet from M7-B works; same chain).
- Node.js 18+ available for running `0g-compute-cli`.
- Phase 0 completes successfully — bearer token and provider endpoint identified — before any code lands.

---

## File Structure

```
scripts/zg-compute-smoke/zg-compute-smoke.go    CREATE (Phase 0) — standalone bearer-auth POST verification
.env.example                                     MODIFY (Phase 0) — document PI_ZG_COMPUTE_* vars

era-brain/llm/zg_compute/zg_compute.go          CREATE (Phase 1) — Provider impl
era-brain/llm/zg_compute/zg_compute_test.go     CREATE (Phase 1) — 6 unit tests via httptest.Server
era-brain/llm/zg_compute/zg_compute_live_test.go CREATE (Phase 1) — //go:build zg_live integration test

era-brain/llm/fallback/fallback.go              CREATE (Phase 2) — wraps two llm.Provider
era-brain/llm/fallback/fallback_test.go         CREATE (Phase 2) — 4 unit tests

era-brain/examples/coding-agent/main.go         MODIFY (Phase 3) — --zg-compute flag wires fallback
era-brain/examples/coding-agent/README.md       MODIFY (Phase 3) — document setup steps
```

No changes outside the listed files. era's orchestrator + queue + swarm stay untouched (those are M7-C.2 work).

---

## Phase 0: 0G Compute setup + standalone smoke script

**No era-brain code touched in this phase.** Goal: prove bearer-auth POST works against a real provider before writing the era-brain provider. Same pattern as M7-B.1 Phase 0.

**Files:**
- Create: `scripts/zg-compute-smoke/zg-compute-smoke.go`
- Modify: `.env.example`
- Modify (your local): `.env` — DO NOT commit

### 0.1: Install 0g-compute-cli + acquire credentials

- [ ] **Step 0.1.1: Install the CLI**

```bash
npm install -g @0glabs/0g-serving-broker
# OR if package name has shifted:
# npm install -g 0g-compute-cli
```

If neither works, search 0G docs for the current package name. The CLI is the canonical way to deposit + transfer + generate a bearer.

- [ ] **Step 0.1.2: Deposit 3+ ZG to broker main account**

```bash
0g-compute-cli account deposit --amount 3
```

(Exact CLI flags may differ; check `--help`.)

- [ ] **Step 0.1.3: Transfer ≥1 ZG to a provider sub-account**

```bash
0g-compute-cli inference list-services
# Pick a provider that serves qwen-2.5-7b-instruct on testnet.
0g-compute-cli account transfer --provider <PROVIDER_ADDRESS> --amount 1
```

- [ ] **Step 0.1.4: Generate bearer token**

```bash
0g-compute-cli inference get-secret --provider <PROVIDER_ADDRESS>
# Outputs: app-sk-<HEX>
```

Save this string — it's the `PI_ZG_COMPUTE_BEARER` value.

- [ ] **Step 0.1.5: Identify provider endpoint URL**

```bash
0g-compute-cli inference get-service-metadata --provider <PROVIDER_ADDRESS>
# Outputs: endpoint=https://provider.example.com:port, model=qwen-2.5-7b-instruct
```

Save the endpoint URL — it's `PI_ZG_COMPUTE_ENDPOINT`.

### 0.2: .env additions

- [ ] **Step 0.2.1: Add to `.env.example`**

Append to `.env.example`:

```
# 0G Compute (M7-C onward) — sealed inference via TEE-signed response
# Generated via 0g-compute-cli (see scripts/zg-compute-smoke/README or M7-C.1 plan)
PI_ZG_COMPUTE_ENDPOINT=https://YOUR_PROVIDER_URL
PI_ZG_COMPUTE_BEARER=app-sk-YOUR_HEX_TOKEN
PI_ZG_COMPUTE_MODEL=qwen-2.5-7b-instruct
```

- [ ] **Step 0.2.2: Add real values to your local `.env`** (NOT committed)

- [ ] **Step 0.2.3: Verify env reachability**

```bash
set -a; source .env; set +a
echo "endpoint=$PI_ZG_COMPUTE_ENDPOINT"
echo "bearer=${PI_ZG_COMPUTE_BEARER:0:12}..."
echo "model=$PI_ZG_COMPUTE_MODEL"
```

All three should print non-empty.

### 0.3: Smoke script

- [ ] **Step 0.3.1: Create the script**

`scripts/zg-compute-smoke/zg-compute-smoke.go`:

```go
// zg-compute-smoke is a standalone 0G Compute SDK verification script. Run with:
//
//	set -a; source .env; set +a
//	go run ./scripts/zg-compute-smoke
//
// Sends one bearer-auth POST to the configured 0G Compute provider, prints the
// response model + first 100 chars of content + which response headers are
// present. Use this output to identify the TEE-signature header name (e.g.
// `ZG-Res-Key` or whatever the actual provider uses) so M7-C.1.1's
// zg_compute.Provider can detect Sealed=true correctly.
//
// Phase 0 success = "OK" prints AND a TEE-signature-shaped header is visible
// in the dumped response headers.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model    string    `json:"model"`
	Messages []chatMsg `json:"messages"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
}

func main() {
	endpoint := os.Getenv("PI_ZG_COMPUTE_ENDPOINT")
	bearer := os.Getenv("PI_ZG_COMPUTE_BEARER")
	model := os.Getenv("PI_ZG_COMPUTE_MODEL")
	if endpoint == "" || bearer == "" || model == "" {
		log.Fatal("PI_ZG_COMPUTE_ENDPOINT, PI_ZG_COMPUTE_BEARER, PI_ZG_COMPUTE_MODEL required")
	}

	body := chatReq{
		Model: model,
		Messages: []chatMsg{
			{Role: "system", Content: "You are a one-line answerer."},
			{Role: "user", Content: "What is 2 + 2?"},
		},
	}
	buf, _ := json.Marshal(body)

	url := strings.TrimRight(endpoint, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		log.Fatalf("build req: %v", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("[status] %d\n", resp.StatusCode)
	fmt.Println("[response headers]")
	for k, v := range resp.Header {
		fmt.Printf("  %s: %s\n", k, strings.Join(v, "; "))
	}

	if resp.StatusCode >= 400 {
		log.Fatalf("provider error: %s", string(respBody))
	}

	var parsed chatResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		log.Fatalf("parse: %v; body=%s", err, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		log.Fatalf("no choices: %s", string(respBody))
	}

	text := parsed.Choices[0].Message.Content
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	fmt.Printf("\n[model]   %s\n[content] %s\n", parsed.Model, text)
	fmt.Println("\nOK")
	fmt.Println("\nNext steps:")
	fmt.Println("- Identify the TEE-signature response header from the dump above")
	fmt.Println("  (likely 'ZG-Res-Key', 'ZG-Signature', or similar)")
	fmt.Println("- Use that header name in M7-C.1.1's zg_compute.Provider.Complete")
}
```

- [ ] **Step 0.3.2: Run the smoke script**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
go run ./scripts/zg-compute-smoke
```

**Acceptance criteria:**
- Process exits 0.
- `[status] 200` appears.
- Response headers section dumped — look for one named like `ZG-Res-Key`, `ZG-Signature`, `ZG-Tee-Sig`, etc. **Note the EXACT name** — Phase 1 needs it.
- `[content]` shows a sane LLM response (something about "4" answering "what is 2+2").
- Final line prints `OK`.

If FAIL:
- 401 → bearer expired/wrong → re-run `0g-compute-cli inference get-secret`.
- 402/403 → provider sub-account out of ZG → re-deposit + transfer.
- timeout → wrong endpoint URL → verify with `inference get-service-metadata`.

**Do not proceed to Phase 1 until smoke prints `OK`.**

- [ ] **Step 0.3.3: Commit Phase 0 artifacts**

```bash
git add scripts/zg-compute-smoke/zg-compute-smoke.go .env.example
git commit -m "phase(M7-C.1.0): 0G Compute setup + smoke script verifies bearer-auth POST against provider"
git tag m7c1-0-setup
```

**Record the TEE-signature header name** (e.g. `ZG-Res-Key`) somewhere visible — Phase 1 uses it. Either:
- Update the spec doc's §3 reference to use the actual name, OR
- Note it as a comment in the Phase 1 commit message.

---

## Phase 1: era-brain `llm/zg_compute` provider

**Files:**
- Create: `era-brain/llm/zg_compute/zg_compute.go`
- Create: `era-brain/llm/zg_compute/zg_compute_test.go`
- Create: `era-brain/llm/zg_compute/zg_compute_live_test.go` (build-tagged)

The package mirrors `era-brain/llm/openrouter` in shape: `Config` → `Provider` → `Complete(ctx, req) (resp, error)`. Only material difference: response parsing also reads the TEE-signature header → `Sealed=true` when present.

### 1A: Failing unit test

- [ ] **Step 1.1: Write the failing test**

`era-brain/llm/zg_compute/zg_compute_test.go`:

```go
package zg_compute_test

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
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/zg_compute"
)

// TEE signature header name — REPLACE with the actual name discovered in
// Phase 0 if it differs from "ZG-Res-Key".
const teeSigHeader = "ZG-Res-Key"

func TestZGCompute_HappyPath_SealedTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer app-sk-test", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"role":"system"`)
		require.Contains(t, string(body), `"role":"user"`)
		require.Contains(t, string(body), `"model":"qwen-2.5-7b-instruct"`)

		w.Header().Set(teeSigHeader, "0xabc123signature")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "the response"}},
			},
			"model": "qwen-2.5-7b-instruct",
		})
	}))
	defer srv.Close()

	p := zg_compute.New(zg_compute.Config{
		BearerToken:      "app-sk-test",
		ProviderEndpoint: srv.URL,
		DefaultModel:     "qwen-2.5-7b-instruct",
	})

	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "sys",
		UserPrompt:   "user",
	})
	require.NoError(t, err)
	require.Equal(t, "the response", resp.Text)
	require.Equal(t, "qwen-2.5-7b-instruct", resp.Model)
	require.True(t, resp.Sealed, "TEE-signature header present → Sealed=true")
}

func TestZGCompute_NoTEEHeader_SealedFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No TEE header set.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "x"}}},
			"model":   "qwen-2.5-7b-instruct",
		})
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "qwen-2.5-7b-instruct",
	})
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "u"})
	require.NoError(t, err)
	require.False(t, resp.Sealed, "no TEE header → Sealed=false")
}

func TestZGCompute_PerRequestModelOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"model":"custom-model"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "x"}}},
			"model":   "custom-model",
		})
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "default-model",
	})
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x", Model: "custom-model"})
	require.NoError(t, err)
	require.Equal(t, "custom-model", resp.Model)
}

func TestZGCompute_HTTPErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "m",
	})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "unauthorized"))
}

func TestZGCompute_EmptyChoicesIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}, "model": "m"})
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "m",
	})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
}

func TestZGCompute_MalformedJSONIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "m",
	})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
}
```

- [ ] **Step 1.2: Run, verify FAIL**

```bash
cd era-brain && go test ./llm/zg_compute/...
```

Expected: package not found.

### 1B: Implement Provider

- [ ] **Step 1.3: Write zg_compute.go**

`era-brain/llm/zg_compute/zg_compute.go`:

```go
// Package zg_compute is a thin 0G Compute (OpenAI-compatible) client
// implementing llm.Provider with Sealed=true detection from the TEE
// signature response header.
//
// Auth: stateless bearer (no Web3 signing per call). The bearer is generated
// once via 0g-compute-cli (see scripts/zg-compute-smoke/ for setup steps).
//
// Sealed-flag semantics: the provider's TEE-signed response carries a
// signature header (name discovered in M7-C.1.0 setup). When the header is
// present and non-empty, we set llm.Response.Sealed=true. We do NOT
// cryptographically verify the signature — that requires TS tooling without
// a Go equivalent. Honest hackathon-scope limitation.
package zg_compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

// teeSigHeader is the response header name 0G providers use to attach the
// TEE-signed proof. Confirmed in M7-C.1.0 setup script's response dump.
// If the actual provider uses a different name, update here AND the test
// const at the top of zg_compute_test.go to match.
const teeSigHeader = "ZG-Res-Key"

// Config holds the 0G Compute client setup.
type Config struct {
	BearerToken      string        // app-sk-<...> generated via 0g-compute-cli
	ProviderEndpoint string        // per-provider base URL (no trailing /chat/completions)
	DefaultModel     string        // testnet: qwen-2.5-7b-instruct
	HTTPTimeout      time.Duration // default 60s
}

// Provider is an llm.Provider that talks to a 0G Compute provider.
type Provider struct {
	cfg    Config
	client *http.Client
}

var _ llm.Provider = (*Provider)(nil)

// New constructs a Provider. Defaults HTTPTimeout if zero.
func New(cfg Config) *Provider {
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

	url := strings.TrimRight(p.cfg.ProviderEndpoint, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return llm.Response{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("zg_compute request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("zg_compute read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return llm.Response{}, fmt.Errorf("zg_compute %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return llm.Response{}, fmt.Errorf("parse resp: %w; body=%s", err, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("zg_compute returned 0 choices: %s", string(respBody))
	}

	text := parsed.Choices[0].Message.Content
	usedModel := parsed.Model
	if usedModel == "" {
		usedModel = model
	}

	sealed := resp.Header.Get(teeSigHeader) != ""

	return llm.Response{
		Text:   text,
		Model:  usedModel,
		Sealed: sealed,
	}, nil
}
```

- [ ] **Step 1.4: Run unit tests, verify PASS**

```bash
cd era-brain && go test -race ./llm/zg_compute/...
```

Expected: 6 PASS.

### 1C: Live integration test

- [ ] **Step 1.5: Write build-tagged live test**

`era-brain/llm/zg_compute/zg_compute_live_test.go`:

```go
//go:build zg_live

package zg_compute_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/zg_compute"
)

func TestZGCompute_LiveTestnet_SealedRoundtrip(t *testing.T) {
	endpoint := os.Getenv("PI_ZG_COMPUTE_ENDPOINT")
	bearer := os.Getenv("PI_ZG_COMPUTE_BEARER")
	model := os.Getenv("PI_ZG_COMPUTE_MODEL")
	if endpoint == "" || bearer == "" || model == "" {
		t.Skip("PI_ZG_COMPUTE_ENDPOINT/BEARER/MODEL required")
	}

	p := zg_compute.New(zg_compute.Config{
		BearerToken:      bearer,
		ProviderEndpoint: endpoint,
		DefaultModel:     model,
	})

	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "You answer with exactly one digit.",
		UserPrompt:   "What is 2+2? Answer with just the digit.",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Text)
	require.True(t, resp.Sealed, "TEE-signature header should be present on real testnet response")
}
```

- [ ] **Step 1.6: Run live test**

```bash
cd era-brain
set -a; source ../.env; set +a
go test -tags zg_live -run TestZGCompute_LiveTestnet ./llm/zg_compute/...
```

Expected: PASS in 5-30 seconds.

If FAIL:
- `Sealed=false` → TEE header name in `zg_compute.go` doesn't match the real one. Use Phase 0 smoke output to find the actual name; update `teeSigHeader` constant.
- Other errors → re-check bearer + endpoint + provider funding.

### 1D: Sanity sweep

- [ ] **Step 1.7: Run all era-brain tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...   # without zg_live tag — live test skipped
```

Expected: green.

- [ ] **Step 1.8: Run all era root tests (no regression)**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go vet ./...
go test -race -count=1 ./...
```

Expected: green.

- [ ] **Step 1.9: Commit**

```bash
git add era-brain/llm/zg_compute/
git commit -m "phase(M7-C.1.1): llm/zg_compute provider — bearer-auth + TEE-signature Sealed detection"
git tag m7c1-1-zg-compute
```

---

## Phase 2: era-brain `llm/fallback` wrapper

**Files:**
- Create: `era-brain/llm/fallback/fallback.go`
- Create: `era-brain/llm/fallback/fallback_test.go`

Pure Go logic; no external dependencies. Wraps two `llm.Provider` impls.

### 2A: Failing test

- [ ] **Step 2.1: Write the failing test**

`era-brain/llm/fallback/fallback_test.go`:

```go
package fallback_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/fallback"
)

type fakeLLM struct {
	resp llm.Response
	err  error
}

func (f *fakeLLM) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	return f.resp, f.err
}

func TestFallback_PrimarySuccess_NoFallbackHook(t *testing.T) {
	primary := &fakeLLM{resp: llm.Response{Text: "primary", Model: "p", Sealed: true}}
	secondary := &fakeLLM{resp: llm.Response{Text: "secondary", Model: "s", Sealed: false}}
	hookCalls := 0
	p := fallback.New(primary, secondary, func(err error) { hookCalls++ })

	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.Equal(t, "primary", resp.Text)
	require.True(t, resp.Sealed)
	require.Equal(t, 0, hookCalls, "primary success → no hook")
}

func TestFallback_PrimaryFail_SecondarySuccess_HookCalled(t *testing.T) {
	primaryErr := errors.New("primary down")
	primary := &fakeLLM{err: primaryErr}
	secondary := &fakeLLM{resp: llm.Response{Text: "secondary", Model: "s", Sealed: false}}
	var hookErr error
	p := fallback.New(primary, secondary, func(err error) { hookErr = err })

	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.Equal(t, "secondary", resp.Text)
	require.False(t, resp.Sealed, "secondary's Sealed flag passes through (false for openrouter)")
	require.ErrorIs(t, hookErr, primaryErr)
}

func TestFallback_BothFail_ErrorWrapsBoth(t *testing.T) {
	primaryErr := errors.New("primary down")
	secondaryErr := errors.New("secondary down")
	primary := &fakeLLM{err: primaryErr}
	secondary := &fakeLLM{err: secondaryErr}
	hookCalls := 0
	p := fallback.New(primary, secondary, func(err error) { hookCalls++ })

	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "primary down")
	require.Contains(t, err.Error(), "secondary down")
	require.Equal(t, 1, hookCalls, "hook called once for primary failure even when secondary also fails")
}

func TestFallback_NilHookOK(t *testing.T) {
	primary := &fakeLLM{err: errors.New("primary down")}
	secondary := &fakeLLM{resp: llm.Response{Text: "ok"}}
	p := fallback.New(primary, secondary, nil) // nil hook = noop

	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Text)
}
```

- [ ] **Step 2.2: Run, verify FAIL**

```bash
cd era-brain && go test ./llm/fallback/...
```

Expected: package not found.

### 2B: Implement

- [ ] **Step 2.3: Write fallback.go**

`era-brain/llm/fallback/fallback.go`:

```go
// Package fallback wraps two llm.Provider impls — a Primary and a Secondary —
// implementing try-primary-then-secondary semantics with an optional error hook.
//
// Use it to combine llm/zg_compute (sealed inference) with llm/openrouter
// (unsealed) so era-brain personas keep working when 0G Compute is flaky.
// Receipts inherit whichever provider succeeded — the caller can inspect
// llm.Response.Sealed to learn which path ran.
package fallback

import (
	"context"
	"fmt"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

// FallbackErrorHandler is called when the primary provider returns an error
// (before the secondary is tried). Use to log + monitor; do not panic.
type FallbackErrorHandler func(primaryErr error)

// Provider implements llm.Provider with primary-first / secondary-fallback
// semantics.
type Provider struct {
	primary    llm.Provider
	secondary  llm.Provider
	onFallback FallbackErrorHandler
}

var _ llm.Provider = (*Provider)(nil)

// New constructs a fallback.Provider. onFallback may be nil.
func New(primary, secondary llm.Provider, onFallback FallbackErrorHandler) *Provider {
	return &Provider{primary: primary, secondary: secondary, onFallback: onFallback}
}

func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	resp, err := p.primary.Complete(ctx, req)
	if err == nil {
		return resp, nil
	}
	if p.onFallback != nil {
		p.onFallback(err)
	}
	resp2, err2 := p.secondary.Complete(ctx, req)
	if err2 != nil {
		return llm.Response{}, fmt.Errorf("primary: %w; secondary: %w", err, err2)
	}
	return resp2, nil
}
```

- [ ] **Step 2.4: Run, verify PASS**

```bash
go test -race ./llm/fallback/...
```

Expected: 4 PASS.

- [ ] **Step 2.5: Run all era-brain tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
```

Expected: green.

- [ ] **Step 2.6: Commit**

```bash
git add era-brain/llm/fallback/
git commit -m "phase(M7-C.1.2): llm/fallback wrapper — primary-first, secondary-fallback with error hook"
git tag m7c1-2-fallback
```

---

## Phase 3: examples/coding-agent --zg-compute live gate

**Files:**
- Modify: `era-brain/examples/coding-agent/main.go` — add `--zg-compute` flag
- Modify: `era-brain/examples/coding-agent/README.md` — document setup

### 3A: Wire --zg-compute flag

- [ ] **Step 3.1: Read current main.go to find insertion points**

```bash
grep -n "openrouter.New\|llmProv\|flag.Bool\|flag.Parse" /Users/vaibhav/Documents/projects/era-multi-persona/era/era-brain/examples/coding-agent/main.go | head
```

Expected: hits at the existing flag-parse block + the openrouter client construction.

- [ ] **Step 3.2: Add the flag**

In `main()`, near other `flag.Bool`/`flag.String` calls:

```go
zgCompute := flag.Bool("zg-compute", false, "use 0G Compute sealed inference w/ OpenRouter fallback (requires PI_ZG_COMPUTE_* env vars)")
```

- [ ] **Step 3.3: Build the LLM provider conditionally**

**Note on coder-persona scope:** in this SDK example, all three personas (planner, coder, reviewer) are `LLMPersona` instances sharing one `llmProv`. So wrapping `llmProv` with the fallback applies sealed inference to all three — intentional for the SDK demo. The spec's "coder stays unsealed" applies only to the era-orchestrator path (M7-C.2), where coder is Pi-in-Docker, NOT an LLMPersona. Don't try to split `llmProv` per-persona here.

Find the existing line that constructs `openrouter.New(...)`:

```go
llmProv := openrouter.New(openrouter.Config{
    APIKey:       apiKey,
    DefaultModel: *model,
})
```

Replace with:

```go
var llmProv llm.Provider = openrouter.New(openrouter.Config{
    APIKey:       apiKey,
    DefaultModel: *model,
})

if *zgCompute {
    zgEndpoint := os.Getenv("PI_ZG_COMPUTE_ENDPOINT")
    zgBearer := os.Getenv("PI_ZG_COMPUTE_BEARER")
    zgModel := os.Getenv("PI_ZG_COMPUTE_MODEL")
    if zgEndpoint == "" || zgBearer == "" || zgModel == "" {
        log.Fatal("--zg-compute set but PI_ZG_COMPUTE_ENDPOINT/BEARER/MODEL missing")
    }
    zgComp := zg_compute.New(zg_compute.Config{
        BearerToken:      zgBearer,
        ProviderEndpoint: zgEndpoint,
        DefaultModel:     zgModel,
    })
    llmProv = fallback.New(zgComp, llmProv, func(err error) {
        fmt.Fprintf(os.Stderr, "[zg_compute fell back to openrouter: %v]\n", err)
    })
}
```

Add imports:

```go
"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
"github.com/vaibhav0806/era-multi-persona/era-brain/llm/fallback"
"github.com/vaibhav0806/era-multi-persona/era-brain/llm/zg_compute"
```

- [ ] **Step 3.4: Verify it compiles**

```bash
cd era-brain && go build ./examples/coding-agent/
```

Expected: exit 0.

### 3B: Smoke test sqlite-only path (no --zg-compute)

- [ ] **Step 3.5: Confirm baseline still works**

```bash
set -a; source ../.env; set +a
go run ./examples/coding-agent --task="add a /healthz endpoint that returns 200 OK"
```

Expected: same 3-persona output as M7-B.1.4. No regressions.

### 3C: Live gate WITH --zg-compute

- [ ] **Step 3.6: Run with --zg-compute**

```bash
set -a; source ../.env; set +a
go run ./examples/coding-agent --task="add a /healthz endpoint that returns 200 OK" --zg-compute
```

**Acceptance criteria:**
- Process exits 0.
- Same 3-persona output as Step 3.5.
- Each persona's `[receipt: ...]` line shows `sealed=true` for all three (planner, coder, reviewer — coder here is an LLMPersona in the SDK example, NOT Pi).
- No `[zg_compute fell back to openrouter: ...]` lines in stderr (would indicate sealed inference failed).

If `sealed=true` appears for all 3 → Phase 3 ✅.

If `sealed=false` appears for any → either:
- TEE header name in `zg_compute.go` is wrong (Phase 0 smoke output is the source of truth).
- Provider returned 200 OK without a TEE header (network blip; retry).

If fallback fired → primary call failed. Check the stderr message for the underlying error (auth, funding, network).

### 3D: Document setup in README

- [ ] **Step 3.7: Update examples/coding-agent/README.md**

Append a "## Run with 0G Compute sealed inference" section with the setup steps from Phase 0 (CLI install, deposit, transfer, get-secret, get-service-metadata) + the `--zg-compute` flag usage.

### 3E: Replay all phases

- [ ] **Step 3.8: Verify everything still green**

```bash
cd era-brain && go vet ./... && go test -race -count=1 ./...
cd .. && go vet ./... && go test -race -count=1 ./...
```

Both green.

- [ ] **Step 3.9: Commit + tag M7-C.1 done**

```bash
git add era-brain/examples/coding-agent/
git commit -m "phase(M7-C.1.3): coding-agent --zg-compute flag wires fallback(zg_compute, openrouter); live gate verifies Sealed=true"
git tag m7c1-3-example
git tag m7c1-done
```

---

## Live gate summary (M7-C.1 acceptance)

When this milestone is done:

1. `cd era-brain && go test -race -count=1 ./...` — green. New tests: 6 zg_compute + 4 fallback = 10 new unit tests.
2. `cd era-brain && go test -tags zg_live ./llm/zg_compute/...` — green against real testnet.
3. era-brain example w/ `--zg-compute`:
   - 3-persona output as before.
   - All 3 persona receipts show `sealed=true`.
   - No fallback warnings in stderr.
4. era root tests still pass — no regressions.
5. era-brain SDK still `go get`-able — `go build ./...` from era-brain works without zg_live tag.

---

## Out of scope (deferred to M7-C.2)

- **era orchestrator integration.** M7-C.2: `cmd/orchestrator/main.go` constructs zg_compute + wraps planner+reviewer LLMs via fallback. swarm-side reviewer cross-check (Sealed flag visibility in prompt).
- **Reviewer cross-check.** M7-C.2: `composeCoderOutput` extends with `planner_sealed:` / `coder_sealed:` lines. Reviewer system prompt updated.
- **Audit log new event kinds** (`inference_sealed`, `inference_fell_back`). M7-C.2 cuts-list candidate.
- **Cryptographic TEE signature verification.** Out of scope hackathon-wide. Sealed=true means "header present"; honest scope limit.

---

## Risks + cuts list (in order if slipping)

1. **Phase 0 setup blocks indefinitely** because 0g-compute-cli has package-name drift or undocumented setup steps. Recovery: search 0G Discord/docs for current package name; if truly unavailable, skip Phase 0 and write zg_compute provider against a mocked endpoint, defer live testnet verification to M7-C.2's live gate.
2. **TEE header name differs from `ZG-Res-Key` placeholder.** Recovery: Phase 0's smoke script DUMPS all response headers; just pick the right name and update the constant. ~5 min fix.
3. **Faucet drains during testing.** Recovery: rate-limit live tests; testnet ZG replenishes daily.
4. **Provider sub-account drained mid-development.** Recovery: re-deposit + transfer via 0g-compute-cli.
