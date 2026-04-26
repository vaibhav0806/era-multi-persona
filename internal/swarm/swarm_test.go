package swarm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era/internal/swarm"
)

type fakeLLM struct {
	resp string
}

func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	return llm.Response{
		Text:   f.resp + "(sys=" + req.SystemPrompt[:min(len(req.SystemPrompt), 20)] + ")",
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
