# M7-B.3 — Evolving Persona Memory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each persona reads its own prior observations from the dual memory provider (SQLite cache + 0G primary) before the LLM call, prepends them to the user prompt, and writes a fresh observation after the LLM call. Memory accumulates across tasks per `(persona, user)` key. Lands the "evolving persistent memory" criterion verbatim from 0G Track 2.

**Architecture:** Five linear phases. Read+write logic lives inside `era-brain.brain.LLMPersona` (SDK layer); per-persona observation shape lives in `era/internal/swarm` (era app layer). Failure mode = run blind + slog.Warn. Prompt injection = prepend `## Prior observations` block to existing buildUserPrompt output. Single-user — UserID derived from `cfg.TelegramAllowedUserID`.

**Tech Stack:** Go 1.25, era-brain SDK (already wired). No new external dependencies. JSON serialization via stdlib `encoding/json`.

**Spec:** `docs/superpowers/specs/2026-04-26-m7b3-evolving-memory-design.md`. All §-references below point at the spec.

**Testing philosophy:** Strict TDD throughout. Failing test → run → verify FAIL → minimal impl → run → verify PASS → commit. `go test -race -count=1 ./...` from era-brain green at every commit. Live gate at the end (Phase 5) is the integration test for the full milestone.

**Prerequisites (check before starting):**
- M7-B.2 complete (tag `m7b2-done`).
- era-brain SDK builds clean: `cd era-brain && go build ./...` exits 0.
- `.env` has PI_ZG_* vars populated (used in Phase 5 live gate; not needed for Phases 1-4).

---

## File Structure

```
era-brain/brain/memory_shaper.go                CREATE (Phase 1)
era-brain/brain/memory_shaper_test.go           CREATE (Phase 1)
era-brain/brain/persona.go                      MODIFY (Phase 1, 2) — extend LLMPersonaConfig + Run flow
era-brain/brain/persona_test.go                 MODIFY (Phase 1, 2) — read-path + write-path tests

internal/swarm/shapers.go                       CREATE (Phase 3)
internal/swarm/shapers_test.go                  CREATE (Phase 3)
internal/swarm/swarm.go                         MODIFY (Phase 3) — wire shapers into planner/reviewer config

internal/queue/queue.go                         MODIFY (Phase 4) — Queue.userID + SetUserID + RunNext threading
internal/queue/queue_run_test.go                MODIFY (Phase 4) — test UserID propagation; cascade fakes

cmd/orchestrator/main.go                        MODIFY (Phase 4) — derive userID from cfg, q.SetUserID(userID)
```

No changes to `runner/`, `audit/`, `diffscan/`, `digest/`, `githubapp/`, `githubcompare/`, `githubpr/`, `githubbranch/`, `replyprompt/`, `stats/`, `progress/`, `budget/`, `telegram/`. The audit log path stays untouched (decoupled from KV per spec §3 step 4).

---

## Task 1: MemoryShaper type + BareHistoryShaper helper + LLMPersona reads

**Files:**
- Create: `era-brain/brain/memory_shaper.go`
- Create: `era-brain/brain/memory_shaper_test.go`
- Modify: `era-brain/brain/persona.go` — extend LLMPersonaConfig, modify Run to read+prepend observations
- Modify: `era-brain/brain/persona_test.go` — add tests for read path

This phase introduces the type system and the read half. Writes come in Phase 2. After Phase 1, LLMPersona reads (or attempts to read) memory, but never updates it — so the buffer stays empty/cold across runs. That's expected; Phase 2 closes the loop.

### 1A: MemoryShaper type + BareHistoryShaper

- [ ] **Step 1.1: Write the failing test for BareHistoryShaper**

`era-brain/brain/memory_shaper_test.go`:

```go
package brain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

func TestBareHistoryShaper_TruncatesAtMaxChars(t *testing.T) {
	shaper := brain.BareHistoryShaper(50)
	in := brain.Input{TaskID: "t1"}
	out := brain.Output{Text: strings.Repeat("x", 100)}
	got := shaper(in, out)
	require.LessOrEqual(t, len(got), 50, "should truncate")
	require.NotEmpty(t, got)
}

func TestBareHistoryShaper_ReturnsEmptyForEmptyText(t *testing.T) {
	shaper := brain.BareHistoryShaper(100)
	got := shaper(brain.Input{}, brain.Output{Text: ""})
	require.Equal(t, "", got, "empty text → no observation")
}

func TestBareHistoryShaper_PassesShortTextThrough(t *testing.T) {
	shaper := brain.BareHistoryShaper(100)
	got := shaper(brain.Input{}, brain.Output{Text: "hello world"})
	require.Equal(t, "hello world", got)
}
```

- [ ] **Step 1.2: Run, verify FAIL**

```bash
cd era-brain && go test ./brain/ -run BareHistoryShaper
```

Expected: `undefined: brain.BareHistoryShaper`.

- [ ] **Step 1.3: Implement memory_shaper.go**

`era-brain/brain/memory_shaper.go`:

```go
package brain

// MemoryShaper produces a single observation string from one persona run.
// Implementations decide what's worth remembering — the SDK only handles
// rolling-buffer mechanics. Return "" to skip writing this turn (e.g.
// error case, redundant content).
type MemoryShaper func(in Input, out Output) string

// BareHistoryShaper is the default shaper: returns out.Text truncated at
// maxChars. Used by SDK example agents that don't need persona-specific
// observation shape.
func BareHistoryShaper(maxChars int) MemoryShaper {
	return func(_ Input, out Output) string {
		if out.Text == "" {
			return ""
		}
		if len(out.Text) <= maxChars {
			return out.Text
		}
		return out.Text[:maxChars]
	}
}
```

- [ ] **Step 1.4: Run, verify PASS**

```bash
go test ./brain/ -run BareHistoryShaper
```

Expected: 3 PASS.

### 1B: Extend LLMPersonaConfig + read path

- [ ] **Step 1.5: Write failing test for LLMPersona read path**

Append to `era-brain/brain/persona_test.go`:

```go
func TestLLMPersona_Run_ReadsMemoryAndPrependsObservationsBlock(t *testing.T) {
	rec := &recordingLLM{resp: "ok"}
	mem := newSpyMem()
	// Pre-seed memory with a blob containing 2 observations.
	priorBlob := []byte(`{"v":1,"observations":["task: prior1 | plan: step a","task: prior2 | plan: step b"]}`)
	require.NoError(t, mem.PutKV(context.Background(), "planner-mem", "user42", priorBlob))

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name:            "planner",
		SystemPrompt:    "you are planner",
		Model:           "test-m",
		LLM:             rec,
		Memory:          mem,
		Now:             time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
	})

	_, err := p.Run(context.Background(), brain.Input{
		TaskID:          "t1",
		UserID:          "user42",
		TaskDescription: "current task",
	})
	require.NoError(t, err)
	require.Contains(t, rec.lastReq.UserPrompt, "## Prior observations")
	require.Contains(t, rec.lastReq.UserPrompt, "task: prior1 | plan: step a")
	require.Contains(t, rec.lastReq.UserPrompt, "task: prior2 | plan: step b")
	require.Contains(t, rec.lastReq.UserPrompt, "current task")
	// Observations block should appear BEFORE the Task: line.
	obsIdx := strings.Index(rec.lastReq.UserPrompt, "## Prior observations")
	taskIdx := strings.Index(rec.lastReq.UserPrompt, "Task: current task")
	require.True(t, obsIdx < taskIdx, "observations should precede task line")
}

func TestLLMPersona_Run_NoShaperMeansNoMemoryRead(t *testing.T) {
	rec := &recordingLLM{resp: "ok"}
	mem := newSpyMem()
	require.NoError(t, mem.PutKV(context.Background(), "planner-mem", "user42", []byte(`{"v":1,"observations":["should not appear"]}`)))

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		// MemoryShaper omitted → no read.
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "user42", TaskDescription: "t"})
	require.NoError(t, err)
	require.NotContains(t, rec.lastReq.UserPrompt, "should not appear")
	require.NotContains(t, rec.lastReq.UserPrompt, "## Prior observations")
}

func TestLLMPersona_Run_NotFoundMeansEmptyObservationsNoBlock(t *testing.T) {
	// First-task-ever case: KV is cold. No observations block in prompt; no warn fired.
	rec := &recordingLLM{resp: "ok"}
	mem := newSpyMem()
	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "newuser", TaskDescription: "first task"})
	require.NoError(t, err)
	require.NotContains(t, rec.lastReq.UserPrompt, "## Prior observations")
	require.Contains(t, rec.lastReq.UserPrompt, "first task")
}

func TestLLMPersona_Run_MalformedBlobRunsBlind(t *testing.T) {
	rec := &recordingLLM{resp: "ok"}
	mem := newSpyMem()
	require.NoError(t, mem.PutKV(context.Background(), "planner-mem", "user42", []byte(`not valid json`)))

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "user42", TaskDescription: "t"})
	require.NoError(t, err, "malformed blob should not fail the task")
	require.NotContains(t, rec.lastReq.UserPrompt, "## Prior observations",
		"malformed → run blind → no block")
}

func TestLLMPersona_Run_NoUserIDMeansNoMemoryRead(t *testing.T) {
	// Defensive: even if shaper is set, no UserID means we have no key to read against.
	rec := &recordingLLM{resp: "ok"}
	mem := newSpyMem()
	require.NoError(t, mem.PutKV(context.Background(), "planner-mem", "", []byte(`{"v":1,"observations":["should not appear"]}`)))

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "", TaskDescription: "t"})
	require.NoError(t, err)
	require.NotContains(t, rec.lastReq.UserPrompt, "should not appear")
}
```

- [ ] **Step 1.6: Run, verify FAIL**

```bash
go test ./brain/ -run LLMPersona_Run_ReadsMemory
```

Expected: undefined fields `MemoryShaper`, `MemoryNamespace` on LLMPersonaConfig.

- [ ] **Step 1.7: Modify LLMPersonaConfig and Run flow**

Open `era-brain/brain/persona.go`. Add fields to `LLMPersonaConfig`:

```go
type LLMPersonaConfig struct {
	Name         string
	SystemPrompt string
	Model        string
	LLM          llm.Provider
	Memory       memory.Provider
	Now          func() time.Time

	// NEW (M7-B.3):
	MemoryShaper    MemoryShaper // optional; nil = no evolving-memory read/write
	MemoryNamespace string       // KV namespace for the persona's blob; required when MemoryShaper set
	MaxObservations int          // rolling buffer cap; defaults to 10
}
```

Add a helper at file scope (above `Run`):

```go
const defaultMaxObservations = 10

// memoryBlob is the JSON shape stored under (MemoryNamespace, UserID).
// v=1 for forward compat (M7-D may add fields).
type memoryBlob struct {
	V            int      `json:"v"`
	Observations []string `json:"observations"`
}

// readPriorObservations fetches and decodes the persona's prior-observations
// blob. Returns the observations slice (possibly empty). Errors are non-fatal:
// ErrNotFound (cold start) is silent; other errors warn and return empty.
func (p *LLMPersona) readPriorObservations(ctx context.Context, userID string) []string {
	if p.cfg.MemoryShaper == nil || p.cfg.Memory == nil || userID == "" || p.cfg.MemoryNamespace == "" {
		return nil
	}
	raw, err := p.cfg.Memory.GetKV(ctx, p.cfg.MemoryNamespace, userID)
	if errors.Is(err, memory.ErrNotFound) {
		return nil
	}
	if err != nil {
		slog.Warn("persona memory read failed",
			"persona", p.cfg.Name, "ns", p.cfg.MemoryNamespace, "err", err)
		return nil
	}
	var blob memoryBlob
	if jerr := json.Unmarshal(raw, &blob); jerr != nil {
		slog.Warn("persona memory blob malformed; running blind",
			"persona", p.cfg.Name, "ns", p.cfg.MemoryNamespace, "err", jerr)
		return nil
	}
	return blob.Observations
}

// renderObservationsBlock formats observations as the prompt-injection block.
// Returns "" if no observations (no block emitted).
func renderObservationsBlock(obs []string) string {
	if len(obs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Prior observations\n")
	for _, o := range obs {
		b.WriteString("- ")
		b.WriteString(o)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}
```

Modify the existing `Run` method — at the top, BEFORE `user := buildUserPrompt(in)`:

```go
func (p *LLMPersona) Run(ctx context.Context, in Input) (Output, error) {
	// NEW: read prior observations + build prompt prefix.
	prior := p.readPriorObservations(ctx, in.UserID)
	obsBlock := renderObservationsBlock(prior)

	user := obsBlock + buildUserPrompt(in)
	// ... rest of Run unchanged
```

Add the new imports at the top of `persona.go`:

```go
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"        // NEW
	"fmt"
	"log/slog"      // NEW
	"strings"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)
```

- [ ] **Step 1.8: Run, verify PASS**

```bash
go test -race ./brain/ -run LLMPersona_Run
```

Expected: all PASS, including the 5 new tests + existing M7-A.5 LLMPersona tests.

- [ ] **Step 1.9: Run all era-brain tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
```

Expected: green.

- [ ] **Step 1.10: Run era root tests (no regression)**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go vet ./...
go test -race -count=1 ./...
```

Expected: green.

- [ ] **Step 1.11: Commit**

```bash
git add era-brain/brain/
git commit -m "phase(M7-B.3.1): MemoryShaper type + BareHistoryShaper + LLMPersona reads prior observations"
git tag m7b3-1-read
```

---

## Task 2: LLMPersona writes updated blob after run

**Files:**
- Modify: `era-brain/brain/persona.go` — extend Run with shaper invocation + write-back
- Modify: `era-brain/brain/persona_test.go` — add tests for write path

After this phase, the read path from Phase 1 sees real data accumulating across runs.

### 2A: Append-and-trim semantics

- [ ] **Step 2.1: Write failing test for write path**

Append to `era-brain/brain/persona_test.go`:

```go
func TestLLMPersona_Run_WritesObservationAfterLLMCall(t *testing.T) {
	rec := &recordingLLM{resp: "the response text"}
	mem := newSpyMem()

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
	})
	_, err := p.Run(context.Background(), brain.Input{
		TaskID: "t1", UserID: "user42", TaskDescription: "task A",
	})
	require.NoError(t, err)

	// Blob should be written under (planner-mem, user42).
	got, ok := mem.puts["planner-mem/user42"]
	require.True(t, ok, "PutKV should be called for (planner-mem, user42)")

	var blob struct {
		V            int      `json:"v"`
		Observations []string `json:"observations"`
	}
	require.NoError(t, json.Unmarshal(got, &blob))
	require.Equal(t, 1, blob.V)
	require.Len(t, blob.Observations, 1)
	require.Equal(t, "the response text", blob.Observations[0])
}

func TestLLMPersona_Run_AppendsToExistingBuffer(t *testing.T) {
	rec := &recordingLLM{resp: "second"}
	mem := newSpyMem()
	require.NoError(t, mem.PutKV(context.Background(), "planner-mem", "user42",
		[]byte(`{"v":1,"observations":["first"]}`)))

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
	})
	_, err := p.Run(context.Background(), brain.Input{
		TaskID: "t1", UserID: "user42", TaskDescription: "t",
	})
	require.NoError(t, err)

	got := mem.puts["planner-mem/user42"]
	var blob struct {
		Observations []string `json:"observations"`
	}
	require.NoError(t, json.Unmarshal(got, &blob))
	require.Equal(t, []string{"first", "second"}, blob.Observations)
}

func TestLLMPersona_Run_TrimsToMaxObservations(t *testing.T) {
	rec := &recordingLLM{resp: "new entry"}
	mem := newSpyMem()
	prior := `{"v":1,"observations":["o1","o2","o3","o4","o5"]}`
	require.NoError(t, mem.PutKV(context.Background(), "planner-mem", "user42", []byte(prior)))

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
		MaxObservations: 3,
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "user42", TaskDescription: "t"})
	require.NoError(t, err)

	got := mem.puts["planner-mem/user42"]
	var blob struct {
		Observations []string `json:"observations"`
	}
	require.NoError(t, json.Unmarshal(got, &blob))
	require.Equal(t, []string{"o4", "o5", "new entry"}, blob.Observations,
		"buffer should keep last 3, dropping oldest")
}

func TestLLMPersona_Run_EmptyShaperOutputSkipsWrite(t *testing.T) {
	// Shaper returns "" → no PutKV call.
	rec := &recordingLLM{resp: ""}
	mem := newSpyMem()
	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "user42", TaskDescription: "t"})
	require.NoError(t, err)
	_, exists := mem.puts["planner-mem/user42"]
	require.False(t, exists, "empty shaper output → no PutKV")
}

func TestLLMPersona_Run_NoShaperMeansNoMemoryWrite(t *testing.T) {
	rec := &recordingLLM{resp: "ok"}
	mem := newSpyMem()
	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		// MemoryShaper omitted
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "user42", TaskDescription: "t"})
	require.NoError(t, err)
	_, exists := mem.puts["planner-mem/user42"]
	require.False(t, exists)
}

func TestLLMPersona_Run_DefaultsMaxObservationsTo10(t *testing.T) {
	rec := &recordingLLM{resp: "11th"}
	mem := newSpyMem()
	// Pre-seed with 10 entries; MaxObservations not set in config → default 10.
	prior := `{"v":1,"observations":["o1","o2","o3","o4","o5","o6","o7","o8","o9","o10"]}`
	require.NoError(t, mem.PutKV(context.Background(), "planner-mem", "user42", []byte(prior)))

	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name: "planner", SystemPrompt: "x", Model: "m", LLM: rec, Memory: mem, Now: time.Now,
		MemoryShaper:    brain.BareHistoryShaper(200),
		MemoryNamespace: "planner-mem",
		// MaxObservations omitted → default 10
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", UserID: "user42", TaskDescription: "t"})
	require.NoError(t, err)

	got := mem.puts["planner-mem/user42"]
	var blob struct {
		Observations []string `json:"observations"`
	}
	require.NoError(t, json.Unmarshal(got, &blob))
	require.Len(t, blob.Observations, 10, "default cap should be 10")
	require.Equal(t, "11th", blob.Observations[9])
	require.Equal(t, "o2", blob.Observations[0], "oldest should be evicted")
}
```

- [ ] **Step 2.2: Run, verify FAIL**

```bash
cd era-brain && go test ./brain/ -run LLMPersona_Run_Writes -count=1
```

Expected: tests reference `mem.puts["planner-mem/user42"]` which is empty (no write happens yet).

- [ ] **Step 2.3: Implement write path**

In `era-brain/brain/persona.go`, add a helper above `Run`:

```go
// writeUpdatedObservations appends a new observation and trims the buffer
// to MaxObservations (default 10). All errors are non-fatal — log and return.
func (p *LLMPersona) writeUpdatedObservations(ctx context.Context, in Input, out Output, prior []string) {
	if p.cfg.MemoryShaper == nil || p.cfg.Memory == nil || in.UserID == "" || p.cfg.MemoryNamespace == "" {
		return
	}
	obs := p.cfg.MemoryShaper(in, out)
	if obs == "" {
		return
	}
	maxObs := p.cfg.MaxObservations
	if maxObs <= 0 {
		maxObs = defaultMaxObservations
	}
	updated := append(prior, obs)
	if len(updated) > maxObs {
		updated = updated[len(updated)-maxObs:]
	}
	blob := memoryBlob{V: 1, Observations: updated}
	raw, err := json.Marshal(blob)
	if err != nil {
		slog.Warn("persona memory marshal failed",
			"persona", p.cfg.Name, "err", err)
		return
	}
	if err := p.cfg.Memory.PutKV(ctx, p.cfg.MemoryNamespace, in.UserID, raw); err != nil {
		slog.Warn("persona memory write failed",
			"persona", p.cfg.Name, "ns", p.cfg.MemoryNamespace, "err", err)
	}
}
```

Modify `Run` to call this AFTER the audit log write, BEFORE the return:

```go
func (p *LLMPersona) Run(ctx context.Context, in Input) (Output, error) {
	prior := p.readPriorObservations(ctx, in.UserID)
	obsBlock := renderObservationsBlock(prior)

	user := obsBlock + buildUserPrompt(in)
	resp, err := p.cfg.LLM.Complete(ctx, llm.Request{
		SystemPrompt: p.cfg.SystemPrompt,
		UserPrompt:   user,
		Model:        p.cfg.Model,
	})
	if err != nil {
		return Output{}, fmt.Errorf("llm complete: %w", err)
	}

	// ... existing receipt + audit log block unchanged ...

	out := Output{PersonaName: p.cfg.Name, Text: resp.Text, Receipt: r}

	// NEW: write updated observations.
	p.writeUpdatedObservations(ctx, in, out, prior)

	return out, nil
}
```

(The exact placement is: keep the existing `if p.cfg.Memory != nil && in.TaskID != ""` audit-log block, then construct `out`, then call `writeUpdatedObservations`, then return.)

- [ ] **Step 2.4: Run, verify PASS**

```bash
go test -race ./brain/ -run LLMPersona_Run -count=1
```

Expected: all 11+ LLMPersona tests pass (existing + 6 new write tests + 5 read tests).

- [ ] **Step 2.5: Run all era-brain + era root tests + vet**

```bash
cd era-brain && go vet ./... && go test -race -count=1 ./...
cd .. && go vet ./... && go test -race -count=1 ./...
```

Expected: green everywhere. `internal/swarm` tests still pass — they use shaper=nil so memory is unaffected.

- [ ] **Step 2.6: Commit**

```bash
git add era-brain/brain/
git commit -m "phase(M7-B.3.2): LLMPersona writes shaped observation after LLM call; trims to MaxObservations"
git tag m7b3-2-write
```

---

## Task 3: era swarm shapers

**Files:**
- Create: `internal/swarm/shapers.go`
- Create: `internal/swarm/shapers_test.go`
- Modify: `internal/swarm/swarm.go` — wire shapers into planner/reviewer LLMPersona configs

### 3A: plannerShaper + reviewerShaper

- [ ] **Step 3.1: Write failing test**

`internal/swarm/shapers_test.go`:

```go
package swarm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

func TestPlannerShaper_FormatsObservation(t *testing.T) {
	in := brain.Input{TaskDescription: "add /healthz endpoint to the existing HTTP server"}
	out := brain.Output{Text: "1. Find the router file\n2. Add a new GET handler\n3. Add a passing test"}
	got := plannerShaper(in, out)
	require.Contains(t, got, "task: ")
	require.Contains(t, got, "plan: ")
	require.Contains(t, got, "Find the router file")
	require.LessOrEqual(t, len(got), 250, "observation should stay bounded")
}

func TestPlannerShaper_TruncatesLongTask(t *testing.T) {
	longTask := strings.Repeat("x", 200)
	in := brain.Input{TaskDescription: longTask}
	out := brain.Output{Text: "1. step"}
	got := plannerShaper(in, out)
	require.LessOrEqual(t, len(got), 250)
}

func TestReviewerShaper_FormatsObservation(t *testing.T) {
	in := brain.Input{TaskDescription: "add /healthz"}
	out := brain.Output{Text: "no issues found\nDECISION: approve"}
	got := reviewerShaper(in, out)
	require.Contains(t, got, "task: ")
	require.Contains(t, got, "decision: approve")
	require.LessOrEqual(t, len(got), 250)
}

func TestReviewerShaper_FlagDecision(t *testing.T) {
	in := brain.Input{TaskDescription: "remove cache layer"}
	out := brain.Output{Text: "(a) Deviations from plan: tests removed\nDECISION: flag"}
	got := reviewerShaper(in, out)
	require.Contains(t, got, "decision: flag")
}
```

(Test file uses `package swarm` not `swarm_test` so it can access unexported `plannerShaper` and `reviewerShaper` directly. Existing `swarm_test.go` uses `package swarm_test` — both are valid; new file is internal-only by design since shapers are package-private.)

- [ ] **Step 3.2: Run, verify FAIL**

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
go test ./internal/swarm/ -run Shaper -count=1
```

Expected: `undefined: plannerShaper`, `undefined: reviewerShaper`.

- [ ] **Step 3.3: Implement shapers**

`internal/swarm/shapers.go`:

```go
package swarm

import (
	"fmt"
	"strings"

	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
)

// plannerShaper records what the planner persona produced for a task.
// Format: `task: "<desc>" | plan: <first 3 plan lines>`.
// Outcome (approve/flag) isn't known at planner-write-time (reviewer hasn't
// run yet); the reviewer's own memory closes the loop on its side.
func plannerShaper(in brain.Input, out brain.Output) string {
	desc := truncateUTF8(in.TaskDescription, 80)
	plan := firstNLines(out.Text, 3)
	plan = truncateUTF8(plan, 100)
	return fmt.Sprintf("task: %q | plan: %s", desc, plan)
}

// reviewerShaper records reviewer's task + decision + critique snippet.
func reviewerShaper(in brain.Input, out brain.Output) string {
	desc := truncateUTF8(in.TaskDescription, 80)
	decision := string(parseDecision(out.Text)) // existing helper in swarm.go
	headline := firstNLines(out.Text, 1)
	headline = truncateUTF8(headline, 100)
	return fmt.Sprintf("task: %q | decision: %s | %s", desc, decision, headline)
}

// firstNLines returns the first n newline-delimited lines of s, joined back
// with newlines. If s has fewer lines, returns s unchanged.
func firstNLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " ")
}

// truncateUTF8 returns the first up-to-n bytes of s. If s is shorter, returns
// s unchanged. Conservative ASCII-byte cap; rune-safe truncation is overkill
// for shaper output (our prompts are ASCII-dominant).
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
```

- [ ] **Step 3.4: Run, verify PASS**

```bash
go test ./internal/swarm/ -run Shaper -count=1
```

Expected: 4 PASS.

### 3B: Wire shapers into Swarm.New

- [ ] **Step 3.5: Write failing test for swarm.New wiring**

Append to `internal/swarm/swarm_test.go`:

```go
func TestSwarm_New_WiresShapersIntoLLMPersonaConfig(t *testing.T) {
	// Smoke-test: Plan must run end-to-end with a memory provider, and the
	// planner's LLMPersonaConfig must have MemoryNamespace="planner-mem"
	// (verifiable by writing a fake observation through the shaper and
	// inspecting the puts on the spy memory).
	plannerLLM := &fakeLLM{resp: "1. step a\n2. step b"}
	reviewerLLM := &fakeLLM{resp: "ok\nDECISION: approve"}
	mem := newSpyMem()
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM, Memory: mem})

	_, err := s.Plan(context.Background(), swarm.PlanArgs{
		TaskID: "t1", UserID: "user42", TaskDescription: "test task",
	})
	require.NoError(t, err)

	// After Plan, the planner shaper should have written under planner-mem/user42.
	got, ok := mem.puts["planner-mem/user42"]
	require.True(t, ok, "swarm.New should wire planner shaper + namespace")

	var blob struct {
		Observations []string `json:"observations"`
	}
	require.NoError(t, json.Unmarshal(got, &blob))
	require.Len(t, blob.Observations, 1)
	require.Contains(t, blob.Observations[0], "test task")
}

func TestSwarm_New_ReviewerHasReviewerMemNamespace(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "p"}
	reviewerLLM := &fakeLLM{resp: "no issues\nDECISION: approve"}
	mem := newSpyMem()
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM, Memory: mem})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID: "t1", UserID: "user42", TaskDescription: "task X",
		PlanText: "plan", DiffText: "diff",
	})
	require.NoError(t, err)

	got, ok := mem.puts["reviewer-mem/user42"]
	require.True(t, ok, "swarm.New should wire reviewer shaper + namespace")

	var blob struct {
		Observations []string `json:"observations"`
	}
	require.NoError(t, json.Unmarshal(got, &blob))
	require.Len(t, blob.Observations, 1)
	require.Contains(t, blob.Observations[0], "task X")
	require.Contains(t, blob.Observations[0], "decision: approve")
}
```

**Confirmed setup steps for swarm_test.go before running this test:**

1. `fakeLLM` already exists in `internal/swarm/swarm_test.go` from M7-A.5. Reuse it.
2. `spyMem` does NOT exist in `swarm_test.go`. Copy the type + `newSpyMem` constructor from `era-brain/brain/persona_test.go` (lines around the existing `spyMem` definition there). The shape:

   ```go
   type spyMem struct {
       puts map[string][]byte
       logs map[string][][]byte
   }
   func newSpyMem() *spyMem { return &spyMem{puts: map[string][]byte{}, logs: map[string][][]byte{}} }
   func (s *spyMem) GetKV(_ context.Context, ns, key string) ([]byte, error) {
       v, ok := s.puts[ns+"/"+key]
       if !ok { return nil, memory.ErrNotFound }
       return v, nil
   }
   func (s *spyMem) PutKV(_ context.Context, ns, key string, val []byte) error {
       s.puts[ns+"/"+key] = val; return nil
   }
   func (s *spyMem) AppendLog(_ context.Context, ns string, e []byte) error {
       s.logs[ns] = append(s.logs[ns], e); return nil
   }
   func (s *spyMem) ReadLog(_ context.Context, ns string) ([][]byte, error) { return s.logs[ns], nil }
   ```

3. **Add these imports to `internal/swarm/swarm_test.go`** if not already present (verify the existing import block; add only what's missing):
   - `"encoding/json"` — needed by `json.Unmarshal` in the new test
   - `"github.com/vaibhav0806/era-multi-persona/era-brain/memory"` — needed by `memory.ErrNotFound` in `spyMem.GetKV`
   - `"context"` — likely already there for existing tests; verify

4. Pass `Memory: mem` in the existing `swarm.Config` literal AND set the spy fields per the test bodies below.

- [ ] **Step 3.6: Run, verify FAIL**

```bash
go test ./internal/swarm/ -run Swarm_New_Wires -count=1
```

Expected: `mem.puts["planner-mem/user42"]` empty (shapers not wired).

- [ ] **Step 3.7: Modify swarm.New**

Open `internal/swarm/swarm.go`. Find the existing `func New(cfg Config) *Swarm` and modify the LLMPersonaConfig construction for both planner and reviewer:

```go
func New(cfg Config) *Swarm {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Swarm{
		planner: brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:            "planner",
			SystemPrompt:    PlannerSystemPrompt,
			LLM:             cfg.PlannerLLM,
			Memory:          cfg.Memory,
			Now:             cfg.Now,
			MemoryShaper:    plannerShaper,    // NEW
			MemoryNamespace: "planner-mem",    // NEW
		}),
		reviewer: brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:            "reviewer",
			SystemPrompt:    ReviewerSystemPrompt,
			LLM:             cfg.ReviewerLLM,
			Memory:          cfg.Memory,
			Now:             cfg.Now,
			MemoryShaper:    reviewerShaper,   // NEW
			MemoryNamespace: "reviewer-mem",   // NEW
		}),
	}
}
```

- [ ] **Step 3.8: Run, verify PASS**

```bash
go test -race ./internal/swarm/ -count=1
```

Expected: all swarm tests pass (existing + 4 shaper unit + 2 New-wiring).

- [ ] **Step 3.9: Run all era tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
```

Expected: green. NOTE: existing `internal/queue` tests may still pass without UserID flowing — that's fine; queue's PlanArgs.UserID stays empty until Phase 4. Memory writes will be skipped at LLMPersona level (UserID=="" guard) for those tests, which is the correct M7-A.5-baseline behavior.

- [ ] **Step 3.10: Commit**

```bash
git add internal/swarm/
git commit -m "phase(M7-B.3.3): plannerShaper + reviewerShaper; swarm.New wires per-persona memory namespaces"
git tag m7b3-3-shapers
```

---

## Task 4: UserID plumbing

**Files:**
- Modify: `internal/queue/queue.go` — Queue.userID field + SetUserID; thread into PlanArgs/ReviewArgs.UserID
- Modify: `internal/queue/queue_run_test.go` — assert UserID propagation; cascade fakeSwarm
- Modify: `cmd/orchestrator/main.go` — derive userID from cfg, q.SetUserID at startup

### 4A: Queue side

- [ ] **Step 4.1: Read existing queue.RunNext to find PlanArgs/ReviewArgs construction**

```bash
grep -n "swarm.PlanArgs\|swarm.ReviewArgs\|UserID:" /Users/vaibhav/Documents/projects/era-multi-persona/era/internal/queue/queue.go | head
```

Expected: hits at the existing PlanArgs and ReviewArgs literal blocks. Both currently OMIT UserID (so it's "").

- [ ] **Step 4.2: Write failing test for UserID propagation**

Append to `internal/queue/queue_run_test.go`. Mirror the existing `TestRunNext_PlannerInjectedIntoRunnerDescription` test scaffolding — find that test, copy its setup, and add the UserID assertion.

The test asserts:
- `q.SetUserID("user42")` is called in setup.
- After RunNext processes a task, the stub swarm's `lastPlanArgs.UserID == "user42"`.
- After RunNext finishes the review path, `lastReviewArgs.UserID == "user42"`.

Concretely (assuming existing `stubSwarm` has fields `plannedDesc string; planText string; reviewedDiff string; reviewDecision swarm.Decision`):

1. Add `lastPlanArgs swarm.PlanArgs` and `lastReviewArgs swarm.ReviewArgs` fields to `stubSwarm`.
2. In `stubSwarm.Plan`, set `s.lastPlanArgs = args` before returning.
3. In `stubSwarm.Review`, set `s.lastReviewArgs = args`.
4. New test:

```go
func TestRunNext_ThreadsUserIDIntoSwarm(t *testing.T) {
	// Mirror TestRunNext_PlannerInjectedIntoRunnerDescription's scaffolding.
	// ... (copy boilerplate that creates Queue, stubSwarm, fakeRunner, fakeNotifier, etc.)

	stub := &stubSwarm{planText: "p", reviewDecision: swarm.DecisionApprove}
	q.SetSwarm(stub)
	q.SetUserID("user42")

	// run a task end-to-end via RunNext
	// ...

	require.Equal(t, "user42", stub.lastPlanArgs.UserID)
	require.Equal(t, "user42", stub.lastReviewArgs.UserID)
}
```

(The full test scaffolding for `TestRunNext_*` is already in `queue_run_test.go`. Copy it; just add the UserID assertion.)

- [ ] **Step 4.3: Run, verify FAIL**

```bash
go test ./internal/queue/ -run ThreadsUserID -count=1
```

Expected: `q.SetUserID undefined`.

- [ ] **Step 4.4: Add SetUserID + thread through**

In `internal/queue/queue.go`:

1. Add field to `Queue` struct:

```go
type Queue struct {
	// ... existing fields ...
	swarm  Swarm
	userID string // M7-B.3 — passed to swarm.{Plan,Review}Args.UserID
}
```

2. Add setter near `SetSwarm`:

```go
// SetUserID sets the user ID threaded into swarm.PlanArgs.UserID and
// swarm.ReviewArgs.UserID. era is single-user; this comes from
// cfg.TelegramAllowedUserID at orchestrator startup.
func (q *Queue) SetUserID(id string) { q.userID = id }
```

3. In `RunNext`, find the existing `swarm.PlanArgs{...}` literal and add `UserID: q.userID,`:

```go
pr, perr := q.swarm.Plan(ctx, swarm.PlanArgs{
	TaskID:          fmt.Sprintf("%d", t.ID),
	UserID:          q.userID, // NEW
	TaskDescription: t.Description,
})
```

4. Similarly for the existing `swarm.ReviewArgs{...}` literal:

```go
rr, rerr := q.swarm.Review(ctx, swarm.ReviewArgs{
	TaskID:          fmt.Sprintf("%d", t.ID),
	UserID:          q.userID, // NEW
	TaskDescription: t.Description,
	PlanText:        planText,
	DiffText:        diffText,
})
```

- [ ] **Step 4.5: Run, verify PASS**

```bash
go test -race ./internal/queue/ -count=1
```

Expected: all tests including new `TestRunNext_ThreadsUserIDIntoSwarm` pass.

### 4B: Orchestrator side

- [ ] **Step 4.6: Read main.go to find queue construction**

```bash
grep -n "queue.New\|q.SetSwarm\|TelegramAllowedUserID" /Users/vaibhav/Documents/projects/era-multi-persona/era/cmd/orchestrator/main.go | head
```

Expected: `queue.New(...)` and `q.SetSwarm(sw)` lines. We add `q.SetUserID(...)` immediately after `q.SetSwarm`.

- [ ] **Step 4.7: Modify main.go**

After `q.SetSwarm(sw)`:

```go
q.SetUserID(strconv.FormatInt(cfg.TelegramAllowedUserID, 10))
```

Add `"strconv"` to the imports if not already present.

- [ ] **Step 4.8: Build whole repo**

```bash
go build ./...
```

Expected: exit 0.

- [ ] **Step 4.9: Run all tests + vet**

```bash
go vet ./...
go test -race -count=1 ./...
```

Expected: green.

- [ ] **Step 4.10: Commit**

```bash
git add internal/queue/ cmd/orchestrator/main.go
git commit -m "phase(M7-B.3.4): Queue.userID + SetUserID; RunNext threads into swarm.{Plan,Review}Args"
git tag m7b3-4-userid
```

---

## Task 5: Live gate — real /task A then /task B

**Files:** none modified. Verification only.

### Step 5.1: Build orchestrator binary

```bash
go build -o bin/orchestrator ./cmd/orchestrator
```

Exit 0; binary at `bin/orchestrator`.

### Step 5.2: Stop VPS M6 era

```bash
ssh era@178.105.44.3 sudo systemctl stop era
```

Wait ~3 sec.

### Step 5.3: Start local orchestrator

```bash
cd /Users/vaibhav/Documents/projects/era-multi-persona/era
set -a; source .env; set +a
./bin/orchestrator
```

Expected boot lines (same as M7-B.2 + nothing new — the M7-B.3 changes don't surface in startup logs):
- migrations OK
- `INFO github app token source configured ...`
- `INFO Selecting nodes ...` (0G SDK)
- `INFO 0G storage wired indexer=... kv_node_set=true|false`
- `INFO orchestrator ready ...`

### Step 5.4: Send task A

Via Telegram:

```
/task add a /healthz endpoint that returns 200 OK with body "ok"
```

Watch stdout. Expected:
- planner runs → audit-log write (1 0G tx, planner side)
- planner ALSO writes its observation to `planner-mem/<userID>` (1 more 0G tx)
- Pi runs (no 0G writes)
- reviewer runs → audit-log write (1 0G tx)
- reviewer ALSO writes its observation to `reviewer-mem/<userID>` (1 more 0G tx)
- Total: **4 0G txs per task** (was 2 in M7-B.2 — doubled because of the new memory writes).

Cost note: 4 testnet txs ≈ 0.004 ZG. Faucet covers easily.

Some 0G writes may revert (per M7-B.2's observed flakiness) — that's fine; cache catches up. Watch for `0G primary write failed` slog.Warn lines and document them. Task should still complete.

### Step 5.5: Verify era-brain.db has memory blobs

After task A finishes:

```bash
sqlite3 ./era-brain.db "SELECT seq, namespace, length(val) FROM entries WHERE is_kv = 1 ORDER BY seq DESC LIMIT 5"
```

Expected: at least 2 KV entries:
- One under `planner-mem` keyed by your user ID
- One under `reviewer-mem` keyed by your user ID

Inspect the planner blob:

```bash
sqlite3 ./era-brain.db "SELECT val FROM entries WHERE is_kv = 1 AND namespace = 'planner-mem' ORDER BY seq DESC LIMIT 1"
```

Should be JSON `{"v":1,"observations":["task: \"...\" | plan: ..."]}`.

### Step 5.6: Send task B

```
/task add a /version endpoint that returns the current commit SHA
```

The KEY assertion of M7-B.3: task B's planner LLM call should see task A's observation in its prompt.

This is hard to observe live without a debug log. Two ways to verify:

**A) Inspect post-task blobs.** After task B finishes, the planner-mem blob should contain BOTH task A and task B observations:

```bash
sqlite3 ./era-brain.db "SELECT val FROM entries WHERE is_kv = 1 AND namespace = 'planner-mem' ORDER BY seq DESC LIMIT 1"
```

Expected: `{"v":1,"observations":["task: \"add a /healthz...\" | plan: ...","task: \"add a /version...\" | plan: ..."]}`. Two entries, in append order.

**B) Add temp debug logging.** Optional — temporarily add `slog.Debug("persona prompt", "user_prompt", user)` inside `LLMPersona.Run` after building the prompt, run with `LOG_LEVEL=debug`, observe task B's planner prompt contains `## Prior observations\n- task: "add a /healthz...`. Revert before commit.

Path A is sufficient — accumulated blob is direct evidence the read+write loop worked.

### Step 5.7: Verify reviewer-mem accumulated similarly

```bash
sqlite3 ./era-brain.db "SELECT val FROM entries WHERE is_kv = 1 AND namespace = 'reviewer-mem' ORDER BY seq DESC LIMIT 1"
```

Expected: `{"v":1,"observations":[...task A reviewer obs..., ...task B reviewer obs...]}`.

### Step 5.8: Check for 0G primary write warnings

```bash
# in the orchestrator stdout, search for:
# "0G primary write failed"
# "persona memory write failed"  ← should not appear if 0G is healthy
```

If present in significant numbers, document but proceed — dual provider's resilience covers task completion.

### Step 5.9: Restart VPS M6

```bash
ssh era@178.105.44.3 sudo systemctl start era
```

**Don't skip.** Production bot stays offline until you do.

### Step 5.10: Stop local orchestrator (Ctrl-C).

### Step 5.11: Replay all era + era-brain tests

```bash
cd era-brain && go vet ./... && go test -race -count=1 ./...
cd .. && go vet ./... && go test -race -count=1 ./...
```

Both green.

### Step 5.12: Tag M7-B.3 done

```bash
git tag m7b3-done
```

(No commit — Phase 5 is verification only.)

---

## Live gate summary (M7-B.3 acceptance)

When this milestone is done:

1. `go test -race -count=1 ./...` from era-brain green; `go test -race -count=1 ./...` from era root green.
2. `go vet ./...` clean from both modules.
3. Real task A on a real repo:
   - 4 0G txs per task (2 audit-log + 2 memory KV). Some may revert (acceptable per M7-B.2 observation); dual handles gracefully.
   - era-brain.db has KV entries under `planner-mem/<userID>` and `reviewer-mem/<userID>`, each with one observation.
4. Real task B:
   - Same 4-tx pattern.
   - `planner-mem/<userID>` blob now has 2 observations (task A's + task B's), in append order.
   - `reviewer-mem/<userID>` blob has 2 observations.
5. Existing M7-B.2 baseline (audit-log writes to dual provider) still works — this milestone is purely additive at the persona-memory layer.

---

## Out of scope (deferred)

Per spec §7. Notable highlights:

- **Coder persona memory.** Pi is not LLMPersona; bridging Pi RESULT → memory needs a separate effort.
- **Cross-persona observation linking.** Planner observations don't reference reviewer's decision (would require deferred-write or two-pass writes; complexity not justified for hackathon).
- **Memory inspection via Telegram.** No `/memory <persona>` command. Inspect via sqlite directly.
- **Time-decay or recency-weighted eviction.** Hard cap by count (`MaxObservations`); last-N-wins.
- **Untrusted-tag wrapping** of observations to defend against prompt-injection-via-prior-output. Real risk; defer to M7-F polish.

---

## Risks + cuts list (in order if slipping)

1. **Live gate: testnet flake doubles 0G tx cost.** Mitigation: each /task now does 4 0G writes (was 2). If faucet drains, top up. Observed M7-B.2 reverts will continue to occur; cache mirror covers.
2. **swarm_test.go's spyMem doesn't exist** (it's in era-brain's brain_test, not swarm_test). If swarm_test.go uses a different fake, copy the spyMem pattern from era-brain/brain/persona_test.go into a swarm_test helper.
3. **Test cascade on existing queue_run_test.go fakeSwarm.** The new `lastPlanArgs` / `lastReviewArgs` fields on stubSwarm may need to be added in Phase 4 if not already there. Mirror the pattern from existing assertions.
4. **memoryBlob backward compat.** v=1 reserved. If a future M7-D adds fields, extend `memoryBlob` struct — JSON tags handle missing fields gracefully on decode.

---

## Notes for implementer

- The "## Prior observations" string is part of the LLM-prompt protocol. Do NOT change the literal without updating tests AND the spec.
- spyMem's `puts` map in era-brain/brain/persona_test.go uses key `ns+"/"+key` (e.g. `"planner-mem/user42"`). Match that convention if creating new fakes.
- LLMPersona.Run's audit log write block is unchanged in this milestone. Don't refactor it.
- `slog.Warn` + structured fields (`"persona", p.cfg.Name, "ns", ..., "err", err`) — match era's existing slog usage.
- The B.3.1 read-path test pre-seeds the memory provider via `mem.PutKV(...)`. This is NOT a real round-trip test (no actual write happens during read-path test setup). The full round-trip is exercised in B.3.2 (write side) + B.3.5 (live gate).
