// Package runner spawns Docker containers to execute queued tasks and
// captures their output. The M0 container is a dummy; M1 swaps in Pi.
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/vaibhav0806/era/internal/audit"
)

// ErrNoResult is returned when a container finishes without emitting a
// `RESULT branch=... summary=...` line on stdout.
var ErrNoResult = errors.New("runner produced no RESULT line")

// Docker runs tasks as Docker containers by shelling out to the `docker` CLI.
type Docker struct {
	Image            string
	SandboxRepo      string // "owner/repo"
	GitHubPAT        string
	OpenRouterAPIKey string // forwarded to container as PI_SIDECAR_OPENROUTER_API_KEY
	PiModel          string
	MaxTokens        int
	MaxCostCents     int
	MaxIterations    int
	MaxWallSeconds   int
}

// RunInput carries the per-task inputs to the container.
type RunInput struct {
	TaskID      int64
	Description string
}

// RunOutput is the parsed result of a successful container run.
type RunOutput struct {
	Branch    string
	Summary   string
	Tokens    int64
	CostCents int
	Audits    []audit.Entry // sidecar AUDIT lines parsed from the combined log
	RawLog    string
}

// Run spawns the container, feeds it the task inputs as env vars, waits for
// it to exit, and parses the RESULT line out of its combined stdout+stderr.
func (d *Docker) Run(ctx context.Context, in RunInput) (*RunOutput, error) {
	args := []string{
		"run", "--rm",
		"--cap-add=NET_ADMIN", // for iptables inside container
		"--cap-add=NET_RAW",   // for REJECT --reject-with tcp-reset
		"-e", fmt.Sprintf("ERA_TASK_ID=%d", in.TaskID),
		"-e", fmt.Sprintf("ERA_TASK_DESCRIPTION=%s", in.Description),
		"-e", fmt.Sprintf("ERA_GITHUB_REPO=%s", d.SandboxRepo),
		"-e", fmt.Sprintf("PI_SIDECAR_GITHUB_PAT=%s", d.GitHubPAT),
		"-e", fmt.Sprintf("PI_SIDECAR_OPENROUTER_API_KEY=%s", d.OpenRouterAPIKey),
		"-e", fmt.Sprintf("ERA_PI_MODEL=%s", d.PiModel),
		"-e", fmt.Sprintf("ERA_MAX_TOKENS=%d", d.MaxTokens),
		"-e", fmt.Sprintf("ERA_MAX_COST_CENTS=%d", d.MaxCostCents),
		"-e", fmt.Sprintf("ERA_MAX_ITERATIONS=%d", d.MaxIterations),
		"-e", fmt.Sprintf("ERA_MAX_WALL_SECONDS=%d", d.MaxWallSeconds),
		d.Image,
	}
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
	go streamTo(&mu, stdout, &combined, &wg)
	go streamTo(&mu, stderr, &combined, &wg)
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

func streamTo(mu *sync.Mutex, r io.Reader, w *strings.Builder, wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		mu.Lock()
		w.WriteString(line)
		w.WriteString("\n")
		mu.Unlock()
	}
}

// ParseResult scans a log stream for the first line of the form
// "RESULT key=value key=value..." and returns a RunOutput.
func ParseResult(r io.Reader) (*RunOutput, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "RESULT ") {
			continue
		}
		out := &RunOutput{}
		for _, p := range strings.Fields(strings.TrimPrefix(line, "RESULT ")) {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "branch":
				out.Branch = kv[1]
			case "summary":
				out.Summary = kv[1]
			case "tokens":
				n, _ := strconv.ParseInt(kv[1], 10, 64)
				out.Tokens = n
			case "cost_cents":
				n, _ := strconv.Atoi(kv[1])
				out.CostCents = n
			}
		}
		// no_changes path: branch="" but summary non-empty
		if out.Branch != "" || out.Summary != "" {
			return out, nil
		}
	}
	return nil, ErrNoResult
}
