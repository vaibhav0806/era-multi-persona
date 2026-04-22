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
	"strings"
	"sync"
)

// ErrNoResult is returned when a container finishes without emitting a
// `RESULT branch=... summary=...` line on stdout.
var ErrNoResult = errors.New("runner produced no RESULT line")

// Docker runs tasks as Docker containers by shelling out to the `docker` CLI.
// We shell out (rather than use the Docker SDK) because the CLI is simpler to
// set up, easier to audit, and good enough for M0.
type Docker struct {
	Image       string // e.g. "pi-agent-runner:m0"
	SandboxRepo string // "owner/repo"
	GitHubPAT   string // scoped PAT for pushes
}

// RunInput carries the per-task inputs to the container.
type RunInput struct {
	TaskID      int64
	Description string
}

// RunOutput is the parsed result of a successful container run.
type RunOutput struct {
	Branch  string
	Summary string
	RawLog  string
}

// Run spawns the container, feeds it the task inputs as env vars, waits for
// it to exit, and parses the RESULT line out of its combined stdout+stderr.
// If the container exits non-zero or produces no RESULT line, Run returns an
// error that includes the combined log for debugging.
func (d *Docker) Run(ctx context.Context, in RunInput) (*RunOutput, error) {
	args := []string{
		"run", "--rm",
		"-e", fmt.Sprintf("PI_TASK_ID=%d", in.TaskID),
		"-e", fmt.Sprintf("PI_TASK_DESCRIPTION=%s", in.Description),
		"-e", fmt.Sprintf("PI_GITHUB_PAT=%s", d.GitHubPAT),
		"-e", fmt.Sprintf("PI_GITHUB_REPO=%s", d.SandboxRepo),
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

	// Fan-in both streams into a single buffer so we have the full log even
	// if the RESULT line lands on stderr (entrypoint emits it to stdout, but
	// we don't want a test regression to hide it).
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

	branch, summary, err := ParseResult(strings.NewReader(combined.String()))
	if err != nil {
		return nil, fmt.Errorf("%w; log:\n%s", err, combined.String())
	}
	return &RunOutput{Branch: branch, Summary: summary, RawLog: combined.String()}, nil
}

func streamTo(mu *sync.Mutex, r io.Reader, w *strings.Builder, wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	// Allow larger-than-default lines for logs with long paths.
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
// "RESULT key=value key=value..." and returns the branch and summary fields.
// Returns ErrNoResult if no such line exists.
func ParseResult(r io.Reader) (branch, summary string, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "RESULT ") {
			continue
		}
		for _, p := range strings.Fields(strings.TrimPrefix(line, "RESULT ")) {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "branch":
				branch = kv[1]
			case "summary":
				summary = kv[1]
			}
		}
		if branch != "" {
			return branch, summary, nil
		}
	}
	return "", "", ErrNoResult
}
