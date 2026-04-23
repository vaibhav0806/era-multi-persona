package main

import (
	"errors"
	"os"
)

type sidecarConfig struct {
	ListenAddr       string
	TavilyAPIKey     string // optional; /search returns 503 if empty
	OpenRouterAPIKey string // optional; /llm/* returns 503 if empty
	GitHubPAT        string // M2-16: sidecar holds PAT for /credentials/git
}

func loadSidecarConfig() (*sidecarConfig, error) {
	c := &sidecarConfig{
		ListenAddr:       os.Getenv("PI_SIDECAR_LISTEN_ADDR"),
		TavilyAPIKey:     os.Getenv("PI_SIDECAR_TAVILY_API_KEY"),
		OpenRouterAPIKey: os.Getenv("PI_SIDECAR_OPENROUTER_API_KEY"),
		GitHubPAT:        os.Getenv("PI_SIDECAR_GITHUB_PAT"),
	}
	if c.ListenAddr == "" {
		return nil, errors.New("PI_SIDECAR_LISTEN_ADDR is required")
	}
	return c, nil
}
