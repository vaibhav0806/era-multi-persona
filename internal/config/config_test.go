package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// setRequiredEnv sets all required env vars (including GitHub App, required as of M2-26).
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PI_TELEGRAM_TOKEN", "tok")
	t.Setenv("PI_TELEGRAM_ALLOWED_USER_ID", "12345")
	t.Setenv("PI_GITHUB_SANDBOX_REPO", "vaibhavpandey/pi-agent-sandbox")
	t.Setenv("PI_DB_PATH", "./test.db")
	t.Setenv("PI_OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("PI_GITHUB_APP_ID", "3475140")
	t.Setenv("PI_GITHUB_APP_INSTALLATION_ID", "126365088")
	t.Setenv("PI_GITHUB_APP_PRIVATE_KEY", "LS0tLS1CRUdJTiBSU0Eg...")
}

func TestLoad_AllPresent(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "tok", cfg.TelegramToken)
	require.Equal(t, int64(12345), cfg.TelegramAllowedUserID)
	require.Equal(t, "vaibhavpandey/pi-agent-sandbox", cfg.GitHubSandboxRepo)
	require.Equal(t, "./test.db", cfg.DBPath)
	require.Equal(t, int64(3475140), cfg.GitHubAppID)
	require.Equal(t, int64(126365088), cfg.GitHubAppInstallationID)
	require.Equal(t, "LS0tLS1CRUdJTiBSU0Eg...", cfg.GitHubAppPrivateKeyBase64)
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
	t.Setenv("PI_GITHUB_SANDBOX_REPO", "x/y")
	t.Setenv("PI_DB_PATH", "x")
	t.Setenv("PI_OPENROUTER_API_KEY", "k")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_TELEGRAM_ALLOWED_USER_ID")
}

func TestLoad_WithOpenRouterAndCaps(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PI_MODEL", "moonshotai/kimi-k2.5")
	t.Setenv("PI_MAX_TOKENS_PER_TASK", "100000")
	t.Setenv("PI_MAX_COST_CENTS_PER_TASK", "25")
	t.Setenv("PI_MAX_ITERATIONS_PER_TASK", "20")
	t.Setenv("PI_MAX_WALL_CLOCK_SECONDS", "600")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "sk-or-test", cfg.OpenRouterAPIKey)
	require.Equal(t, "moonshotai/kimi-k2.5", cfg.PiModel)
	require.Equal(t, 100000, cfg.MaxTokensPerTask)
	require.Equal(t, 25, cfg.MaxCostCentsPerTask)
	require.Equal(t, 20, cfg.MaxIterationsPerTask)
	require.Equal(t, 600, cfg.MaxWallClockSeconds)
}

func TestLoad_DefaultsForOptional(t *testing.T) {
	setRequiredEnv(t)
	// All PI_MAX_* and PI_MODEL unset — expect defaults.
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "moonshotai/kimi-k2.6", cfg.PiModel)
	require.Equal(t, 500000, cfg.MaxTokensPerTask)
	require.Equal(t, 50, cfg.MaxCostCentsPerTask)
	require.Equal(t, 30, cfg.MaxIterationsPerTask)
	require.Equal(t, 900, cfg.MaxWallClockSeconds)
}

func TestLoad_MissingOpenRouterKey(t *testing.T) {
	t.Setenv("PI_TELEGRAM_TOKEN", "tok")
	t.Setenv("PI_TELEGRAM_ALLOWED_USER_ID", "1")
	t.Setenv("PI_GITHUB_SANDBOX_REPO", "a/b")
	t.Setenv("PI_DB_PATH", "./x.db")
	t.Setenv("PI_OPENROUTER_API_KEY", "")
	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_OPENROUTER_API_KEY")
}

func TestLoad_MissingAppID(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PI_GITHUB_APP_ID", "")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_GITHUB_APP_ID")
}

func TestLoad_MissingInstallationID(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PI_GITHUB_APP_INSTALLATION_ID", "")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_GITHUB_APP_INSTALLATION_ID")
}

func TestLoad_MissingPrivateKey(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PI_GITHUB_APP_PRIVATE_KEY", "")

	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_GITHUB_APP_PRIVATE_KEY")
}

func TestLoad_DigestTimeDefault(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "17:30", cfg.DigestTimeUTC)
}

func TestLoad_DigestTimeCustom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PI_DIGEST_TIME_UTC", "08:15")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "08:15", cfg.DigestTimeUTC)
}

func TestLoad_DigestTimeMalformed(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PI_DIGEST_TIME_UTC", "not-a-time")
	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "PI_DIGEST_TIME_UTC")
}

func TestParseDigestTime(t *testing.T) {
	h, m, err := ParseDigestTime("17:30")
	require.NoError(t, err)
	require.Equal(t, 17, h)
	require.Equal(t, 30, m)
}

func TestParseDigestTime_Bad(t *testing.T) {
	_, _, err := ParseDigestTime("25:00")
	require.Error(t, err)
	_, _, err = ParseDigestTime("12")
	require.Error(t, err)
	_, _, err = ParseDigestTime("12:60")
	require.Error(t, err)
}
