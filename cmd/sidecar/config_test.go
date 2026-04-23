package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSidecarConfig_Defaults(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "127.0.0.1:8080")
	c, err := loadSidecarConfig()
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8080", c.ListenAddr)
}

func TestSidecarConfig_MissingListenAddr(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "")
	_, err := loadSidecarConfig()
	require.ErrorContains(t, err, "PI_SIDECAR_LISTEN_ADDR")
}

func TestSidecarConfig_TavilyOptional(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "127.0.0.1:8080")
	t.Setenv("PI_SIDECAR_TAVILY_API_KEY", "tvly-abc")
	c, err := loadSidecarConfig()
	require.NoError(t, err)
	require.Equal(t, "tvly-abc", c.TavilyAPIKey)
}

func TestSidecarConfig_TavilyMissingOK(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "127.0.0.1:8080")
	t.Setenv("PI_SIDECAR_TAVILY_API_KEY", "")
	c, err := loadSidecarConfig()
	require.NoError(t, err) // not required
	require.Equal(t, "", c.TavilyAPIKey)
}

func TestSidecarConfig_OpenRouterOptional(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "127.0.0.1:8080")
	t.Setenv("PI_SIDECAR_OPENROUTER_API_KEY", "sk-or-v1-abc")
	c, err := loadSidecarConfig()
	require.NoError(t, err)
	require.Equal(t, "sk-or-v1-abc", c.OpenRouterAPIKey)
}

func TestSidecarConfig_GitHubPATOptional(t *testing.T) {
	t.Setenv("PI_SIDECAR_LISTEN_ADDR", "127.0.0.1:8080")
	t.Setenv("PI_SIDECAR_GITHUB_PAT", "ghp_abc")
	c, err := loadSidecarConfig()
	require.NoError(t, err)
	require.Equal(t, "ghp_abc", c.GitHubPAT)
}
