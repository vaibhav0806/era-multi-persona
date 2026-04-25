package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// piProcess abstracts the `pi --mode json` child process so we can fake it in
// unit tests. The real implementation is realPi.
//
// Pi --mode json takes the prompt as a positional CLI argument (set in
// newRealPi), not via stdin. There is no Stdin() method.
type piProcess interface {
	Stdout() (io.Reader, error)
	Stderr() (io.Reader, error)
	Start() error
	Wait() error
	Abort() error
}

type runSummary struct {
	TotalTokens  int64
	TotalCostUSD float64
	ToolUseCount int
	LastText     string
}

// eventObserver watches the live event stream (used by caps enforcer).
type eventObserver interface {
	onEvent(e *piEvent) error
}

type progressFunc func(iter int, action string, tokens int64, costUSD float64)

func runPi(ctx context.Context, p piProcess, obs eventObserver, onProgress progressFunc) (*runSummary, error) {
	stdout, err := p.Stdout()
	if err != nil {
		return nil, fmt.Errorf("stdout: %w", err)
	}
	if err := p.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	// Watchdog: if the parent ctx is canceled (wall-clock cap or external
	// shutdown), kill Pi so the scanner unblocks and we can return promptly.
	ctxDone := make(chan struct{})
	defer close(ctxDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = p.Abort()
		case <-ctxDone:
		}
	}()

	// Stream events live — one per line — so the observer can abort mid-stream.
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	summary := &runSummary{}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		e, err := parseEvent([]byte(line))
		if err != nil {
			// Don't abort on a single malformed line — Pi might emit
			// something unexpected; keep reading, surface the error only if
			// nothing useful arrives after.
			continue
		}
		// Accumulate BEFORE calling observer so caps.Totals() reflects
		// running totals when onEvent is called.
		switch e.Type {
		case "message_end":
			summary.TotalTokens += e.Message.Usage.TotalTokens
			summary.TotalCostUSD += e.Message.Usage.Cost.Total
			if e.Message.Role == "assistant" {
				var b strings.Builder
				for _, c := range e.Message.Content {
					if c.Type == "text" {
						b.WriteString(c.Text)
					}
				}
				if txt := b.String(); txt != "" {
					summary.LastText = txt
				}
			}
		case "tool_execution_end":
			summary.ToolUseCount++
			if onProgress != nil {
				onProgress(summary.ToolUseCount, e.Tool, summary.TotalTokens, summary.TotalCostUSD)
			}
		case "error":
			_ = p.Abort()
			return summary, fmt.Errorf("pi error: %s", e.Error)
		}
		if obsErr := obs.onEvent(e); obsErr != nil {
			_ = p.Abort()
			return summary, obsErr
		}
		if e.Type == "agent_end" {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return summary, fmt.Errorf("stream: %w", err)
	}
	if err := p.Wait(); err != nil {
		if ctx.Err() != nil {
			return summary, fmt.Errorf("wall-clock cap fired during pi run: %w", ctx.Err())
		}
		return summary, fmt.Errorf("pi wait: %w", err)
	}
	return summary, nil
}

// realPi is a thin exec.Cmd wrapper that implements piProcess.
type realPi struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func newRealPi(ctx context.Context, model, workdir, prompt string) (*realPi, error) {
	cmd := exec.CommandContext(ctx, "pi",
		"--mode", "json",
		"--provider", "openrouter",
		"--model", model,
		"--tools", "read,write,edit,grep,find,ls,bash",
		prompt,
	)
	cmd.Dir = workdir // Pi uses process CWD as session CWD; pass explicitly.
	// OPENROUTER_API_KEY is a dummy value; the sidecar (started by entrypoint)
	// holds the real key and injects it when proxying requests to openrouter.ai.
	// The sidecar's baseUrl is written into /tmp/pi-state/models.json by the
	// entrypoint so Pi's openrouter provider routes through the sidecar.
	cmd.Env = append(cmd.Environ(),
		"OPENROUTER_API_KEY=dummy-sidecar-injects-real",
		"PI_CODING_AGENT_DIR=/tmp/pi-state",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	return &realPi{cmd: cmd, stdout: stdout, stderr: stderr}, nil
}

func (r *realPi) Stdout() (io.Reader, error) { return r.stdout, nil }
func (r *realPi) Stderr() (io.Reader, error) { return r.stderr, nil }
func (r *realPi) Start() error               { return r.cmd.Start() }
func (r *realPi) Wait() error                { return r.cmd.Wait() }
func (r *realPi) Abort() error {
	if r.cmd.Process == nil {
		return nil
	}
	return r.cmd.Process.Kill()
}
