package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type runnerConfig struct {
	TaskID          int64
	TaskDescription string
	GitHubRepo      string // "owner/repo"

	PiModel string

	MaxTokens      int
	MaxCostCents   int
	MaxIterations  int
	MaxWallSeconds int
}

func loadRunnerConfig() (*runnerConfig, error) {
	c := &runnerConfig{
		TaskDescription: os.Getenv("ERA_TASK_DESCRIPTION"),
		GitHubRepo:      os.Getenv("ERA_GITHUB_REPO"),
		PiModel:         os.Getenv("ERA_PI_MODEL"),
	}

	idRaw := os.Getenv("ERA_TASK_ID")
	if idRaw == "" {
		return nil, errors.New("ERA_TASK_ID is required")
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("ERA_TASK_ID: %w", err)
	}
	c.TaskID = id

	for _, f := range []struct {
		name string
		v    string
	}{
		{"ERA_TASK_DESCRIPTION", c.TaskDescription},
		{"ERA_GITHUB_REPO", c.GitHubRepo},
		{"ERA_PI_MODEL", c.PiModel},
	} {
		if f.v == "" {
			return nil, fmt.Errorf("%s is required", f.name)
		}
	}

	if c.MaxTokens, err = posIntEnv("ERA_MAX_TOKENS"); err != nil {
		return nil, err
	}
	if c.MaxCostCents, err = posIntEnv("ERA_MAX_COST_CENTS"); err != nil {
		return nil, err
	}
	if c.MaxIterations, err = posIntEnv("ERA_MAX_ITERATIONS"); err != nil {
		return nil, err
	}
	if c.MaxWallSeconds, err = posIntEnv("ERA_MAX_WALL_SECONDS"); err != nil {
		return nil, err
	}
	return c, nil
}

func posIntEnv(name string) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %d", name, v)
	}
	return v, nil
}
