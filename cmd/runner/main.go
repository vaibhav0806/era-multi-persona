package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	cfg, err := loadRunnerConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "runner config: %v\n", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(cfg.MaxWallSeconds+10)*time.Second) // +10s grace after caps fire
	defer cancel()

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "runner failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *runnerConfig) error {
	workspace := filepath.Join("/workspace", "repo")

	branch := fmt.Sprintf("agent/%d/%s", cfg.TaskID, slugify(cfg.TaskDescription))
	remote := fmt.Sprintf("https://github.com/%s.git", cfg.GitHubRepo)

	g := &gitDriver{
		RemoteURL:   remote,
		BranchName:  branch,
		CommitMsg:   fmt.Sprintf("task #%d: %s", cfg.TaskID, cfg.TaskDescription),
		AuthorName:  "era",
		AuthorEmail: "era@local",
	}

	slog.Info("cloning", "repo", cfg.GitHubRepo, "into", workspace)
	if err := g.Clone(ctx, workspace); err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	// Wall-clock cap starts NOW — after clone, before Pi spawns. This means
	// the cap measures Pi-time, not clone-time. (M0 clone takes <5s for the
	// sandbox; not a concern for M1.)
	c := newCaps(ctx, *cfg)

	prompt := composePrompt(cfg.TaskDescription)
	p, err := newRealPi(ctx, cfg.PiModel, workspace, prompt)
	if err != nil {
		return fmt.Errorf("pi spawn: %w", err)
	}

	slog.Info("running pi", "model", cfg.PiModel)
	summary, piErr := runPi(ctx, p, c)

	// Even on error, record what we spent and try to push whatever Pi wrote
	// so the branch is inspectable next morning.
	tokens, costUSD, iters := c.Totals()
	_ = iters
	slog.Info("pi done",
		"tokens", tokens, "cost_usd", costUSD,
		"iter", iters, "err", piErr)

	// If Pi was aborted by a cap, don't commit or push — the task is failed
	// and we don't want partial work landing on the sandbox.
	if errors.Is(piErr, errCapExceeded) {
		return piErr
	}

	commitErr := g.CommitAndPush(ctx, workspace)
	switch {
	case commitErr == nil:
		writeResult(os.Stdout, runResult{
			Branch:    branch,
			Summary:   piSummary(summary, piErr),
			Tokens:    tokens,
			CostCents: int(math.Round(costUSD * 100)),
		})
		return nil
	case commitErr == errNoChanges:
		// Pi ran but made no edits. Surface this distinctly.
		writeResult(os.Stdout, runResult{
			Branch:    "",
			Summary:   "no_changes",
			Tokens:    tokens,
			CostCents: int(math.Round(costUSD * 100)),
		})
		if piErr != nil {
			return piErr
		}
		return nil
	default:
		return fmt.Errorf("commit/push: %w (pi err: %v)", commitErr, piErr)
	}
}

func piSummary(s *runSummary, err error) string {
	if err != nil {
		return "aborted_" + sanitize(err.Error())
	}
	return fmt.Sprintf("ok_tokens=%d_cost=%.4f", s.TotalTokens, s.TotalCostUSD)
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' {
			return '_'
		}
		return r
	}, s)
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

func slugify(s string) string {
	s = strings.ToLower(s)
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ', r == '-', r == '_':
			out = append(out, '-')
		}
	}
	slug := string(out)
	if len(slug) > 40 {
		slug = slug[:40]
	}
	if slug == "" {
		slug = fmt.Sprintf("task-%d", time.Now().Unix())
	}
	return slug
}
