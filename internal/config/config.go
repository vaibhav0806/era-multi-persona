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
	GitHubPAT             string
	GitHubSandboxRepo     string // "owner/repo"
	DBPath                string
}

func Load() (*Config, error) {
	c := &Config{
		TelegramToken:     os.Getenv("PI_TELEGRAM_TOKEN"),
		GitHubPAT:         os.Getenv("PI_GITHUB_PAT"),
		GitHubSandboxRepo: os.Getenv("PI_GITHUB_SANDBOX_REPO"),
		DBPath:            os.Getenv("PI_DB_PATH"),
	}

	if c.TelegramToken == "" {
		return nil, errors.New("PI_TELEGRAM_TOKEN is required")
	}
	if c.GitHubPAT == "" {
		return nil, errors.New("PI_GITHUB_PAT is required")
	}
	if c.GitHubSandboxRepo == "" {
		return nil, errors.New("PI_GITHUB_SANDBOX_REPO is required")
	}
	if c.DBPath == "" {
		return nil, errors.New("PI_DB_PATH is required")
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

	return c, nil
}
