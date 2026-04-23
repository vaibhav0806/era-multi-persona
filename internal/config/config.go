package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	TelegramToken         string
	TelegramAllowedUserID int64
	// GitHubPAT: REMOVED (M2-26). GitHub App replaces classic PAT.
	GitHubSandboxRepo string // "owner/repo"
	DBPath            string

	// M1 additions
	OpenRouterAPIKey     string
	PiModel              string // OpenRouter model id (e.g. "moonshotai/kimi-k2.6")
	MaxTokensPerTask     int
	MaxCostCentsPerTask  int
	MaxIterationsPerTask int
	MaxWallClockSeconds  int

	// M2-25/26: GitHub App now REQUIRED.
	GitHubAppID               int64
	GitHubAppInstallationID   int64
	GitHubAppPrivateKeyBase64 string
}

const (
	defaultPiModel              = "moonshotai/kimi-k2.6"
	defaultMaxTokensPerTask     = 500_000
	defaultMaxCostCentsPerTask  = 50 // $0.50 USD
	defaultMaxIterationsPerTask = 30
	defaultMaxWallClockSeconds  = 900 // 15 min
)

func Load() (*Config, error) {
	c := &Config{
		TelegramToken:     os.Getenv("PI_TELEGRAM_TOKEN"),
		GitHubSandboxRepo: os.Getenv("PI_GITHUB_SANDBOX_REPO"),
		DBPath:            os.Getenv("PI_DB_PATH"),
		OpenRouterAPIKey:  os.Getenv("PI_OPENROUTER_API_KEY"),
	}

	if c.TelegramToken == "" {
		return nil, errors.New("PI_TELEGRAM_TOKEN is required")
	}
	if c.GitHubSandboxRepo == "" {
		return nil, errors.New("PI_GITHUB_SANDBOX_REPO is required")
	}
	if c.DBPath == "" {
		return nil, errors.New("PI_DB_PATH is required")
	}
	if c.OpenRouterAPIKey == "" {
		return nil, errors.New("PI_OPENROUTER_API_KEY is required")
	}

	raw := os.Getenv("PI_TELEGRAM_ALLOWED_USER_ID")
	if raw == "" {
		return nil, errors.New("PI_TELEGRAM_ALLOWED_USER_ID is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("PI_TELEGRAM_ALLOWED_USER_ID must be integer: %w", err)
	}
	c.TelegramAllowedUserID = id

	c.PiModel = defOrEnv("PI_MODEL", defaultPiModel)
	if c.MaxTokensPerTask, err = intEnv("PI_MAX_TOKENS_PER_TASK", defaultMaxTokensPerTask); err != nil {
		return nil, err
	}
	if c.MaxCostCentsPerTask, err = intEnv("PI_MAX_COST_CENTS_PER_TASK", defaultMaxCostCentsPerTask); err != nil {
		return nil, err
	}
	if c.MaxIterationsPerTask, err = intEnv("PI_MAX_ITERATIONS_PER_TASK", defaultMaxIterationsPerTask); err != nil {
		return nil, err
	}
	if c.MaxWallClockSeconds, err = intEnv("PI_MAX_WALL_CLOCK_SECONDS", defaultMaxWallClockSeconds); err != nil {
		return nil, err
	}

	// GitHub App fields — REQUIRED in M2-26.
	if c.GitHubAppID, err = int64Env("PI_GITHUB_APP_ID", 0); err != nil {
		return nil, err
	}
	if c.GitHubAppInstallationID, err = int64Env("PI_GITHUB_APP_INSTALLATION_ID", 0); err != nil {
		return nil, err
	}
	c.GitHubAppPrivateKeyBase64 = os.Getenv("PI_GITHUB_APP_PRIVATE_KEY")

	if c.GitHubAppID == 0 {
		return nil, errors.New("PI_GITHUB_APP_ID is required")
	}
	if c.GitHubAppInstallationID == 0 {
		return nil, errors.New("PI_GITHUB_APP_INSTALLATION_ID is required")
	}
	if c.GitHubAppPrivateKeyBase64 == "" {
		return nil, errors.New("PI_GITHUB_APP_PRIVATE_KEY is required")
	}

	return c, nil
}

func defOrEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be integer: %w", key, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %d", key, v)
	}
	return v, nil
}

func int64Env(key string, def int64) (int64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be integer: %w", key, err)
	}
	return v, nil
}
