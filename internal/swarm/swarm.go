// Package swarm wraps era-brain.LLMPersona with era-specific glue:
// planner runs before Pi, reviewer runs after Pi sees the diff. Pi itself
// is the coder persona's tool-loop engine and is not part of this package.
package swarm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/brain"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/memory"
)

// Config configures a Swarm.
type Config struct {
	PlannerLLM  llm.Provider
	ReviewerLLM llm.Provider
	Memory      memory.Provider // optional; when set, receipts append to audit log
	Now         func() time.Time
}

// Swarm orchestrates planner + reviewer LLM calls. Coder is Pi-in-Docker,
// invoked by the queue between Plan and Review.
type Swarm struct {
	planner  *brain.LLMPersona
	reviewer *brain.LLMPersona
}

// New constructs a Swarm with the planner and reviewer personas wired up.
func New(cfg Config) *Swarm {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Swarm{
		planner: brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "planner",
			SystemPrompt: PlannerSystemPrompt,
			LLM:          cfg.PlannerLLM,
			Memory:       cfg.Memory,
			Now:          cfg.Now,
		}),
		reviewer: brain.NewLLMPersona(brain.LLMPersonaConfig{
			Name:         "reviewer",
			SystemPrompt: ReviewerSystemPrompt,
			LLM:          cfg.ReviewerLLM,
			Memory:       cfg.Memory,
			Now:          cfg.Now,
		}),
	}
}

// PlanArgs is the input to Plan.
type PlanArgs struct {
	TaskID          string
	UserID          string
	TaskDescription string
}

// PlanResult is what Plan returns.
type PlanResult struct {
	PlanText string
	Receipt  brain.Receipt
}

// Plan runs the planner persona and returns the plan text plus receipt.
func (s *Swarm) Plan(ctx context.Context, args PlanArgs) (PlanResult, error) {
	out, err := s.planner.Run(ctx, brain.Input{
		TaskID:          args.TaskID,
		UserID:          args.UserID,
		TaskDescription: args.TaskDescription,
	})
	if err != nil {
		return PlanResult{}, fmt.Errorf("swarm.Plan: %w", err)
	}
	return PlanResult{PlanText: out.Text, Receipt: out.Receipt}, nil
}

// Decision is the reviewer persona's verdict on the diff.
type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionFlag    Decision = "flag"
)

// ReviewArgs is the input to Review.
type ReviewArgs struct {
	TaskID           string
	UserID           string
	TaskDescription  string
	PlanText         string
	DiffText         string
	DiffScanFindings []string // human-readable rule names; e.g. ["removed_test (foo_test.go)"]
}

// ReviewResult is what Review returns.
type ReviewResult struct {
	CritiqueText string
	Decision     Decision
	Receipt      brain.Receipt
}

// Review runs the reviewer persona on the coder's diff. Returns the critique,
// decision (approve | flag — flag is the safe default), and receipt.
func (s *Swarm) Review(ctx context.Context, args ReviewArgs) (ReviewResult, error) {
	out, err := s.reviewer.Run(ctx, brain.Input{
		TaskID:          args.TaskID,
		UserID:          args.UserID,
		TaskDescription: args.TaskDescription,
		PriorOutputs: []brain.Output{
			{PersonaName: "planner", Text: args.PlanText},
			{PersonaName: "coder", Text: composeCoderOutput(args.DiffText, args.DiffScanFindings)},
		},
	})
	if err != nil {
		return ReviewResult{}, fmt.Errorf("swarm.Review: %w", err)
	}
	return ReviewResult{
		CritiqueText: out.Text,
		Decision:     parseDecision(out.Text),
		Receipt:      out.Receipt,
	}, nil
}

func composeCoderOutput(diff string, findings []string) string {
	out := "Diff:\n" + diff
	if len(findings) > 0 {
		out += "\n\nDiff-scan findings:\n"
		for _, f := range findings {
			out += "- " + f + "\n"
		}
	}
	return out
}

func parseDecision(text string) Decision {
	// Look for a line starting with "decision: approve". Anything else (including
	// "decision: flag" or no decision line at all) maps to flag — safe default.
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(strings.ToLower(line))
		if strings.HasPrefix(l, "decision: approve") {
			return DecisionApprove
		}
	}
	return DecisionFlag
}
