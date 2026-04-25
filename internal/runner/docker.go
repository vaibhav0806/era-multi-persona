// Package runner spawns Docker containers to execute queued tasks and
// captures their output. The M0 container is a dummy; M1 swaps in Pi.
package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/vaibhav0806/era/internal/audit"
	"github.com/vaibhav0806/era/internal/progress"
)

// ErrNoResult is returned when a container finishes without emitting a
// `RESULT <json>` line on stdout.
var ErrNoResult = errors.New("runner produced no RESULT line")

// ProgressEvent is an alias for progress.Event for convenience in the runner package.
type ProgressEvent = progress.Event

// ProgressCallback is an alias for progress.Callback for convenience in the runner package.
type ProgressCallback = progress.Callback

// Docker runs tasks as Docker containers by shelling out to the `docker` CLI.
// It holds no long-lived credentials; per-task tokens arrive via RunInput.
type Docker struct {
	Image            string
	SandboxRepo      string // "owner/repo"
	OpenRouterAPIKey string // forwarded to container as PI_SIDECAR_OPENROUTER_API_KEY
	PiModel          string
	MaxTokens        int
	MaxCostCents     int
	MaxIterations    int
	MaxWallSeconds   int
}

// RunInput carries the per-task inputs to the container.
type RunInput struct {
	TaskID        int64
	Description   string
	GitHubToken   string // per-task installation token (or classic PAT as fallback)
	Repo          string // per-invocation override; empty falls back to d.SandboxRepo
	ContainerName string // when non-empty, passed as --name to docker run
	MaxIter       int    // M6 AG: per-task override; 0 = use d.MaxIterations
	MaxCents      int    // 0 = use d.MaxCostCents
	MaxWallSec    int    // 0 = use d.MaxWallSeconds
}

// RunOutput is the parsed result of a successful container run.
type RunOutput struct {
	Branch    string        `json:"branch"`
	Summary   string        `json:"summary"`
	Tokens    int64         `json:"tokens"`
	CostCents int           `json:"cost_cents"`
	Audits    []audit.Entry // sidecar AUDIT lines parsed from the combined log
	RawLog    string
}

// BuildDockerArgs builds the argument list for `docker run` from d's config
// and the per-task RunInput. in.Repo must already be resolved (non-empty).
func (d *Docker) BuildDockerArgs(in RunInput) []string {
	args := []string{"run", "--rm"}
	if in.ContainerName != "" {
		args = append(args, "--name", in.ContainerName)
	}
	maxIter := d.MaxIterations
	if in.MaxIter > 0 {
		maxIter = in.MaxIter
	}
	maxCents := d.MaxCostCents
	if in.MaxCents > 0 {
		maxCents = in.MaxCents
	}
	maxWall := d.MaxWallSeconds
	if in.MaxWallSec > 0 {
		maxWall = in.MaxWallSec
	}

	args = append(args,
		"--cap-add=NET_ADMIN", // for iptables inside container
		"--cap-add=NET_RAW",   // for REJECT --reject-with tcp-reset
		"-e", fmt.Sprintf("ERA_TASK_ID=%d", in.TaskID),
		"-e", fmt.Sprintf("ERA_TASK_DESCRIPTION=%s", in.Description),
		"-e", fmt.Sprintf("ERA_GITHUB_REPO=%s", in.Repo),
		"-e", fmt.Sprintf("PI_SIDECAR_GITHUB_PAT=%s", in.GitHubToken),
		"-e", fmt.Sprintf("PI_SIDECAR_OPENROUTER_API_KEY=%s", d.OpenRouterAPIKey),
		"-e", fmt.Sprintf("ERA_PI_MODEL=%s", d.PiModel),
		"-e", fmt.Sprintf("ERA_MAX_TOKENS=%d", d.MaxTokens),
		"-e", fmt.Sprintf("ERA_MAX_COST_CENTS=%d", maxCents),
		"-e", fmt.Sprintf("ERA_MAX_ITERATIONS=%d", maxIter),
		"-e", fmt.Sprintf("ERA_MAX_WALL_SECONDS=%d", maxWall),
		d.Image,
	)
	return args
}

// Run spawns the container, feeds it the task inputs as env vars, waits for
// it to exit, and parses the RESULT line out of its combined stdout+stderr.
// onProgress is called for each valid PROGRESS line emitted on stdout; pass
// nil to ignore progress events.
func (d *Docker) Run(ctx context.Context, in RunInput, onProgress ProgressCallback) (*RunOutput, error) {
	if in.Repo == "" {
		in.Repo = d.SandboxRepo // backward-compat default
	}
	args := d.BuildDockerArgs(in)
	cmd := exec.CommandContext(ctx, "docker", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start docker: %w", err)
	}

	var mu sync.Mutex
	var combined strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)
	go StreamToWithProgress(&mu, stdout, &combined, &wg, onProgress)
	go StreamToWithProgress(&mu, stderr, &combined, &wg, nil)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("docker run: %w; log:\n%s", err, combined.String())
	}

	out, err := ParseResult(strings.NewReader(combined.String()))
	if err != nil {
		return nil, fmt.Errorf("%w; log:\n%s", err, combined.String())
	}
	out.RawLog = combined.String()
	audit.Stream(strings.NewReader(combined.String()), func(e audit.Entry) {
		out.Audits = append(out.Audits, e)
	})
	return out, nil
}

// StreamToWithProgress reads lines from r into combined (under mu) and fires
// onProgress for each valid "PROGRESS <json>" line. wg.Done is called on return.
// Exported for testing; callers within this package use it directly.
func StreamToWithProgress(mu *sync.Mutex, r io.Reader, combined *strings.Builder, wg *sync.WaitGroup, onProgress ProgressCallback) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		mu.Lock()
		combined.WriteString(line)
		combined.WriteString("\n")
		mu.Unlock()
		if onProgress != nil && strings.HasPrefix(line, "PROGRESS ") {
			payload := strings.TrimPrefix(line, "PROGRESS ")
			var ev ProgressEvent
			if err := json.Unmarshal([]byte(payload), &ev); err == nil {
				onProgress(ev)
			}
		}
	}
}

// ParseResult scans a log stream for the first line of the form
// "RESULT <json>" and returns a RunOutput.
func ParseResult(r io.Reader) (*RunOutput, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "RESULT ") {
			continue
		}
		payload := strings.TrimPrefix(line, "RESULT ")
		var out RunOutput
		if err := json.Unmarshal([]byte(payload), &out); err != nil {
			return nil, fmt.Errorf("parse RESULT json: %w; raw=%q", err, payload)
		}
		return &out, nil
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return nil, ErrNoResult
}
