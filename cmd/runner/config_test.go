package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunnerConfig_AllPresent(t *testing.T) {
	t.Setenv("ERA_TASK_ID", "7")
	t.Setenv("ERA_TASK_DESCRIPTION", "fix the thing")
	t.Setenv("ERA_GITHUB_REPO", "a/b")
	t.Setenv("ERA_PI_MODEL", "m")
	t.Setenv("ERA_MAX_TOKENS", "100")
	t.Setenv("ERA_MAX_COST_CENTS", "5")
	t.Setenv("ERA_MAX_ITERATIONS", "3")
	t.Setenv("ERA_MAX_WALL_SECONDS", "60")

	c, err := loadRunnerConfig()
	require.NoError(t, err)
	require.Equal(t, int64(7), c.TaskID)
	require.Equal(t, "fix the thing", c.TaskDescription)
	require.Equal(t, "a/b", c.GitHubRepo)
	require.Equal(t, "m", c.PiModel)
	require.Equal(t, 100, c.MaxTokens)
	require.Equal(t, 5, c.MaxCostCents)
	require.Equal(t, 3, c.MaxIterations)
	require.Equal(t, 60, c.MaxWallSeconds)
}

func TestRunnerConfig_MissingRequired(t *testing.T) {
	t.Setenv("ERA_TASK_ID", "")
	_, err := loadRunnerConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ERA_TASK_ID")
}
