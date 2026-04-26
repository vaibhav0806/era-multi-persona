package swarm_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
	"github.com/vaibhav0806/era/internal/swarm"
)

type spyMem struct {
	puts map[string][]byte
	logs map[string][][]byte
}

func newSpyMem() *spyMem {
	return &spyMem{puts: map[string][]byte{}, logs: map[string][][]byte{}}
}

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

func (s *spyMem) ReadLog(_ context.Context, ns string) ([][]byte, error) {
	return s.logs[ns], nil
}

type fakeLLM struct {
	resp    string
	lastReq llm.Request
}

func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	f.lastReq = req
	return llm.Response{
		Text:   f.resp,
		Model:  "test-m",
		Sealed: false,
	}, nil
}

func TestSwarm_Plan_ProducesPlanWithReceipt(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "1. step one\n2. step two\n3. step three"}
	reviewerLLM := &fakeLLM{resp: "no issues found\nDECISION: approve"}
	s := swarm.New(swarm.Config{
		PlannerLLM:  plannerLLM,
		ReviewerLLM: reviewerLLM,
	})

	res, err := s.Plan(context.Background(), swarm.PlanArgs{
		TaskID:          "t1",
		TaskDescription: "add JWT auth",
	})
	require.NoError(t, err)
	require.Contains(t, res.PlanText, "step one")
	require.Equal(t, "planner", res.Receipt.Persona)
	require.Equal(t, "test-m", res.Receipt.Model)
	require.False(t, res.Receipt.Sealed)
	require.NotEmpty(t, res.Receipt.InputHash)
	_ = brain.Receipt{} // keeps brain import used in this file even before later tests reference it
}

func TestSwarm_Review_DecisionApprove(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "no issues found\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	res, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:          "t1",
		TaskDescription: "task",
		PlanText:        "1. step",
		DiffText:        "diff --git a/x b/x\n+hello",
	})
	require.NoError(t, err)
	require.Equal(t, swarm.DecisionApprove, res.Decision)
	require.Contains(t, strings.ToLower(res.CritiqueText), "no issues")
	require.Equal(t, "reviewer", res.Receipt.Persona)
}

func TestSwarm_Review_DecisionFlag(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "the diff removes a test\nDECISION: flag"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	res, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:   "t1",
		PlanText: "p",
		DiffText: "d",
	})
	require.NoError(t, err)
	require.Equal(t, swarm.DecisionFlag, res.Decision)
}

func TestSwarm_Review_NoExplicitDecisionDefaultsToFlag(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "looks fine I guess"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	res, err := s.Review(context.Background(), swarm.ReviewArgs{TaskID: "t1", PlanText: "p", DiffText: "d"})
	require.NoError(t, err)
	require.Equal(t, swarm.DecisionFlag, res.Decision)
}

func TestSwarm_InjectPlan_PrependsPlanToTaskDescription(t *testing.T) {
	out := swarm.InjectPlan("add JWT auth", "1. add middleware\n2. add login")
	require.Contains(t, out, "add JWT auth")
	require.Contains(t, out, "1. add middleware")
	taskIdx := strings.Index(out, "add JWT auth")
	planIdx := strings.Index(out, "1. add middleware")
	require.True(t, taskIdx < planIdx, "task should appear before plan")
}

func TestSwarm_InjectPlan_NoPlanReturnsTaskAsIs(t *testing.T) {
	out := swarm.InjectPlan("just the task", "")
	require.Equal(t, "just the task", out)
}

func TestSwarm_Review_TruncatesLargeDiff(t *testing.T) {
	// A 200k-char diff (much larger than the 30k cap) must not produce a
	// reviewer prompt that blows past the 128k-token model context window.
	// The reviewer's fakeLLM captures lastReq.UserPrompt; assert it stays
	// bounded and that a "diff truncated" marker is present.
	plannerLLM := &fakeLLM{resp: "plan"}
	bigDiff := strings.Repeat("x", 200_000)
	rec := &fakeLLM{resp: "no issues found\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: rec})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:   "t1",
		PlanText: "plan",
		DiffText: bigDiff,
	})
	require.NoError(t, err)
	require.Less(t, len(rec.lastReq.UserPrompt), 50_000,
		"reviewer prompt should be bounded; got %d chars", len(rec.lastReq.UserPrompt))
	require.Contains(t, rec.lastReq.UserPrompt, "diff truncated")
}

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

func TestSwarm_Review_PromptIncludesSealedFlags(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "ok\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:          "t1",
		TaskDescription: "task",
		PlanText:        "plan",
		DiffText:        "diff",
		PriorPersonaSealed: map[string]bool{
			"planner": true,
		},
	})
	require.NoError(t, err)
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "planner_sealed: true")
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "coder_sealed: false")
}

func TestSwarm_Review_PlannerUnsealedPropagates(t *testing.T) {
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "ok\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID:          "t1",
		TaskDescription: "task",
		PlanText:        "plan",
		DiffText:        "diff",
		PriorPersonaSealed: map[string]bool{
			"planner": false, // fallback fired
		},
	})
	require.NoError(t, err)
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "planner_sealed: false")
}

func TestSwarm_Review_DefaultsBothSealedFalseWhenMapNil(t *testing.T) {
	// Backward-compat: if PriorPersonaSealed is nil (pre-M7-C.2 callers),
	// emit both flags as false. Reviewer treats unknown sealed status as
	// the safe default.
	plannerLLM := &fakeLLM{resp: "plan"}
	reviewerLLM := &fakeLLM{resp: "ok\nDECISION: approve"}
	s := swarm.New(swarm.Config{PlannerLLM: plannerLLM, ReviewerLLM: reviewerLLM})

	_, err := s.Review(context.Background(), swarm.ReviewArgs{
		TaskID: "t1", TaskDescription: "task", PlanText: "plan", DiffText: "diff",
		// PriorPersonaSealed: nil
	})
	require.NoError(t, err)
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "planner_sealed: false")
	require.Contains(t, reviewerLLM.lastReq.UserPrompt, "coder_sealed: false")
}
