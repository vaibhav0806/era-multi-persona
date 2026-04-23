package githubpr

import (
	"context"
	"net/http"
	"time"
)

// TokenSource yields a GitHub App installation token. Satisfied by *githubapp.Client.
type TokenSource interface {
	InstallationToken(ctx context.Context) (string, error)
}

// Client calls the GitHub Pull Requests API.
type Client struct {
	tokens  TokenSource
	http    *http.Client
	baseURL string
}

// New returns a Client. baseURL is the GitHub API base (empty → https://api.github.com).
func New(baseURL string, tokens TokenSource) *Client {
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &Client{
		tokens:  tokens,
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: baseURL,
	}
}
