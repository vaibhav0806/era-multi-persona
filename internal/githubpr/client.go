package githubpr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// DefaultBranch returns the default branch name for repo (owner/repo).
func (c *Client) DefaultBranch(ctx context.Context, repo string) (string, error) {
	req, err := c.newReq(ctx, "GET", "/repos/"+repo, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("get repo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get repo %s: %d %s", repo, resp.StatusCode, string(body))
	}
	var body struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	return body.DefaultBranch, nil
}

// CreateArgs holds the parameters for creating a pull request.
type CreateArgs struct {
	Repo  string
	Head  string
	Base  string
	Title string
	Body  string
}

// PR represents a GitHub pull request response.
type PR struct {
	Number  int    `json:"number"`
	URL     string `json:"url"`
	HTMLURL string `json:"html_url"`
}

// Create opens a new pull request and returns the result.
func (c *Client) Create(ctx context.Context, args CreateArgs) (*PR, error) {
	payload, err := json.Marshal(map[string]string{
		"head":  args.Head,
		"base":  args.Base,
		"title": args.Title,
		"body":  args.Body,
	})
	if err != nil {
		return nil, err
	}
	req, err := c.newReq(ctx, "POST", "/repos/"+args.Repo+"/pulls", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create pr: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create pr %s head=%s base=%s: %d %s",
			args.Repo, args.Head, args.Base, resp.StatusCode, string(body))
	}
	var pr PR
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode pr: %w", err)
	}
	return &pr, nil
}

// Close sets the state of pull request number on repo (owner/repo) to closed.
func (c *Client) Close(ctx context.Context, repo string, number int) error {
	payload, _ := json.Marshal(map[string]string{"state": "closed"})
	req, err := c.newReq(ctx, "PATCH", fmt.Sprintf("/repos/%s/pulls/%d", repo, number), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("close pr: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("close pr %s#%d: %d %s", repo, number, resp.StatusCode, string(body))
	}
	return nil
}

// ApprovePR submits an APPROVED review on the given PR. Body is optional prose.
func (c *Client) ApprovePR(ctx context.Context, repo string, number int, body string) error {
	payload, _ := json.Marshal(map[string]string{
		"event": "APPROVE",
		"body":  body,
	})
	req, err := c.newReq(ctx, "POST", fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, number), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("approve pr: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("approve pr %s#%d: %d %s", repo, number, resp.StatusCode, string(rb))
	}
	return nil
}

func (c *Client) newReq(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	tok, err := c.tokens.InstallationToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("mint token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
