package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func msgEnd(tok int64, cost float64) *piEvent {
	e := &piEvent{Type: "message_end"}
	e.Message.Usage.TotalTokens = tok
	e.Message.Usage.Cost.Total = cost
	return e
}

func TestCaps_TokenLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newCaps(ctx, runnerConfig{MaxTokens: 100, MaxCostCents: 10000, MaxIterations: 1000, MaxWallSeconds: 60})

	require.NoError(t, c.onEvent(msgEnd(50, 0.001)))
	require.NoError(t, c.onEvent(msgEnd(40, 0.001)))
	err := c.onEvent(msgEnd(20, 0.001)) // cumulative 110 > 100
	require.Error(t, err)
	require.ErrorIs(t, err, errCapExceeded)
	require.Contains(t, err.Error(), "tokens")
}

func TestCaps_CostLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newCaps(ctx, runnerConfig{MaxTokens: 1_000_000_000, MaxCostCents: 5, MaxIterations: 1000, MaxWallSeconds: 60})

	require.NoError(t, c.onEvent(msgEnd(1, 0.03))) // 3¢ cumulative
	err := c.onEvent(msgEnd(1, 0.04))              // 7¢ > 5¢ cap
	require.ErrorIs(t, err, errCapExceeded)
	require.Contains(t, err.Error(), "cost")
}

func TestCaps_IterationLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newCaps(ctx, runnerConfig{MaxTokens: 1_000_000_000, MaxCostCents: 10000, MaxIterations: 2, MaxWallSeconds: 60})

	require.NoError(t, c.onEvent(&piEvent{Type: "tool_use_end"}))
	require.NoError(t, c.onEvent(&piEvent{Type: "tool_use_end"}))
	err := c.onEvent(&piEvent{Type: "tool_use_end"})
	require.ErrorIs(t, err, errCapExceeded)
	require.Contains(t, err.Error(), "iterations")
}

func TestCaps_WallClock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newCaps(ctx, runnerConfig{MaxTokens: 1_000_000_000, MaxCostCents: 1_000_000_000, MaxIterations: 1_000_000_000, MaxWallSeconds: 1})

	// Wait past the wall-clock deadline.
	time.Sleep(1100 * time.Millisecond)
	err := c.onEvent(msgEnd(1, 0.001))
	require.ErrorIs(t, err, errCapExceeded)
	require.Contains(t, err.Error(), "wall-clock")
}

func TestCaps_ExposesRunningTotals(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newCaps(ctx, runnerConfig{MaxTokens: 1_000_000_000, MaxCostCents: 1_000_000_000, MaxIterations: 1_000_000_000, MaxWallSeconds: 60})

	_ = c.onEvent(msgEnd(100, 0.05))
	_ = c.onEvent(&piEvent{Type: "tool_use_end"})
	tok, co, it := c.Totals()
	require.Equal(t, int64(100), tok)
	require.InDelta(t, 0.05, co, 1e-9)
	require.Equal(t, 1, it)
}

// NOTE: do NOT declare errCapExceeded here. The real declaration lives in
// caps.go (Step 3). The tests reference it via `errors.Is(err, errCapExceeded)`
// which works because caps_test.go is in the same `package main`.
