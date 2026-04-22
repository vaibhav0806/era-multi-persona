package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// errCapExceeded is returned by Caps.onEvent when a per-task cap is hit.
// The driver aborts the Pi subprocess and surfaces the error to the RESULT.
var errCapExceeded = errors.New("cap exceeded")

type caps struct {
	cfg      runnerConfig
	start    time.Time
	tokens   int64
	costUSD  float64
	iters    int
	deadline time.Time
}

func newCaps(ctx context.Context, cfg runnerConfig) *caps {
	now := time.Now()
	return &caps{
		cfg:      cfg,
		start:    now,
		deadline: now.Add(time.Duration(cfg.MaxWallSeconds) * time.Second),
	}
}

func (c *caps) onEvent(e *piEvent) error {
	if time.Now().After(c.deadline) {
		return fmt.Errorf("%w: wall-clock %ds exceeded", errCapExceeded, c.cfg.MaxWallSeconds)
	}
	switch e.Type {
	case "message_end":
		c.tokens += e.Message.Usage.TotalTokens
		c.costUSD += e.Message.Usage.Cost.Total
		if c.tokens > int64(c.cfg.MaxTokens) {
			return fmt.Errorf("%w: tokens %d > %d", errCapExceeded, c.tokens, c.cfg.MaxTokens)
		}
		if int(c.costUSD*100) > c.cfg.MaxCostCents {
			return fmt.Errorf("%w: cost %.4f USD > %d cents", errCapExceeded, c.costUSD, c.cfg.MaxCostCents)
		}
	case "tool_use_end":
		c.iters++
		if c.iters > c.cfg.MaxIterations {
			return fmt.Errorf("%w: iterations %d > %d", errCapExceeded, c.iters, c.cfg.MaxIterations)
		}
	}
	return nil
}

func (c *caps) Totals() (tokens int64, costUSD float64, iters int) {
	return c.tokens, c.costUSD, c.iters
}
