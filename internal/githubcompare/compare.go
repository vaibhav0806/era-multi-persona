// Package githubcompare fetches diff data from GitHub's compare API and
// maps it to diffscan.FileDiff values.
package githubcompare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vaibhav0806/era/internal/diffscan"
)

// Tokener yields an installation token. Implemented by *githubapp.Client.
type Tokener interface {
	InstallationToken(ctx context.Context) (string, error)
}

// Client hits GitHub's compare API and converts the response into FileDiff.
type Client struct {
	base    string
	tokener Tokener
	http    *http.Client
}

func New(baseURL string, t Tokener) *Client {
	b := strings.TrimRight(baseURL, "/")
	if b == "" {
		b = "https://api.github.com"
	}
	return &Client{
		base:    b,
		tokener: t,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Compare returns the per-file diffs between base and head on repo (owner/repo).
func (c *Client) Compare(ctx context.Context, repo, base, head string) ([]diffscan.FileDiff, error) {
	tok, err := c.tokener.InstallationToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("installation token: %w", err)
	}
	url := fmt.Sprintf("%s/repos/%s/compare/%s...%s", c.base, repo, base, head)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github compare %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Files []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
			Patch    string `json:"patch"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	out := make([]diffscan.FileDiff, 0, len(parsed.Files))
	for _, f := range parsed.Files {
		fd := diffscan.FileDiff{
			Path:    f.Filename,
			Deleted: f.Status == "removed",
		}
		// Patch is unified-diff for this one file (without file headers).
		for _, line := range strings.Split(f.Patch, "\n") {
			switch {
			case strings.HasPrefix(line, "+++"),
				strings.HasPrefix(line, "---"),
				strings.HasPrefix(line, "@@"):
				continue
			case strings.HasPrefix(line, "+"):
				fd.Added = append(fd.Added, line[1:])
			case strings.HasPrefix(line, "-"):
				fd.Removed = append(fd.Removed, line[1:])
			}
		}
		out = append(out, fd)
	}
	return out, nil
}
