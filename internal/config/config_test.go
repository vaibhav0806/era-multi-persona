package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_AllRequiredPresent(t *testing.T) {
	t.Setenv("PI_TELEGRAM_TOKEN", "tok")
	t.Setenv("PI_TELEGRAM_ALLOWED_USER_ID", "12345")
	t.Setenv("PI_GITHUB_PAT", "ghp_xxx")
	t.Setenv("PI_GITHUB_SANDBOX_REPO", "vaibhavpandey/pi-agent-sandbox")
	t.Setenv("PI_DB_PATH", "./test.db")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "tok", cfg.TelegramToken)
	require.Equal(t, int64(12345), cfg.TelegramAllowedUserID)
	require.Equal(t, "ghp_xxx", cfg.GitHubPAT)
	require.Equal(t, "vaibhavpandey/pi-agent-sandbox", cfg.GitHubSandboxRepo)
	require.Equal(t, "./test.db", cfg.DBPath)
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("PI_TELEGRAM_TOKEN", "")
	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_TELEGRAM_TOKEN")
}

func TestLoad_InvalidAllowedUserID(t *testing.T) {
	t.Setenv("PI_TELEGRAM_TOKEN", "tok")
	t.Setenv("PI_TELEGRAM_ALLOWED_USER_ID", "not-a-number")
	t.Setenv("PI_GITHUB_PAT", "x")
	t.Setenv("PI_GITHUB_SANDBOX_REPO", "x/y")
	t.Setenv("PI_DB_PATH", "x")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_TELEGRAM_ALLOWED_USER_ID")
}
