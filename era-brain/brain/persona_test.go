package brain_test

import (
	"context"
	"encoding/json"
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
	return llm.Response{Text: r.resp, Model: "test-m", Sealed: false}, nil
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

func TestLLMPersona_Run_AppendedReceiptHasCorrectFields(t *testing.T) {
	rec := &recordingLLM{resp: "RESPONSE_TEXT"}
	mem := newSpyMem()
	p := brain.NewLLMPersona(brain.LLMPersonaConfig{
		Name:         "planner",
		SystemPrompt: "sys",
		Model:        "test-m",
		LLM:          rec,
		Memory:       mem,
		Now:          func() time.Time { return time.Unix(1700000000, 0) },
	})
	_, err := p.Run(context.Background(), brain.Input{TaskID: "t1", TaskDescription: "do thing"})
	require.NoError(t, err)
	require.Len(t, mem.logs["audit/t1"], 1)

	var got brain.Receipt
	require.NoError(t, json.Unmarshal(mem.logs["audit/t1"][0], &got))
	require.Equal(t, "planner", got.Persona)
	require.Equal(t, "test-m", got.Model)
	require.False(t, got.Sealed)
	require.Equal(t, int64(1700000000), got.TimestampUnix)
	require.Len(t, got.InputHash, 64, "InputHash should be sha256 hex (64 chars)")
	require.Len(t, got.OutputHash, 64, "OutputHash should be sha256 hex (64 chars)")
	require.Regexp(t, "^[0-9a-f]{64}$", got.InputHash)
	require.Regexp(t, "^[0-9a-f]{64}$", got.OutputHash)
}

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
