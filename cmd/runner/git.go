package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// errNoChanges means Pi ran but produced no diff. The driver surfaces this
// as a non-fatal outcome (task "completed with no changes"), not a push.
var errNoChanges = errors.New("no changes to commit")

type gitDriver struct {
	RemoteURL   string // includes PAT for real use; local file:// for tests
	BranchName  string
	CommitMsg   string
	AuthorName  string
	AuthorEmail string
}

func (g *gitDriver) Clone(ctx context.Context, dest string) error {
	return g.runAt(ctx, "", "git", "clone", "--depth", "1", g.RemoteURL, dest)
}

func (g *gitDriver) CommitAndPush(ctx context.Context, workDir string) error {
	steps := [][]string{
		{"git", "config", "user.email", g.AuthorEmail},
		{"git", "config", "user.name", g.AuthorName},
		{"git", "checkout", "-b", g.BranchName},
	}
	for _, s := range steps {
		if err := g.runAt(ctx, workDir, s[0], s[1:]...); err != nil {
			return err
		}
	}

	if err := g.runAt(ctx, workDir, "git", "add", "-A"); err != nil {
		return err
	}

	// Is there anything to commit?
	out, err := g.outputAt(ctx, workDir, "git", "diff", "--cached", "--stat")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return errNoChanges
	}

	if err := g.runAt(ctx, workDir, "git", "commit", "-m", g.CommitMsg); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if err := g.runAt(ctx, workDir, "git", "push", "origin", g.BranchName); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

func (g *gitDriver) runAt(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, errBuf.String())
	}
	return nil
}

func (g *gitDriver) outputAt(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %v: %w\n%s", name, args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}
