// Package zg_compute is a thin 0G Compute (OpenAI-compatible) client
// implementing llm.Provider with Sealed=true detection from the TEE
// signature response header.
//
// Auth: stateless bearer (no Web3 signing per call). The bearer is generated
// once via 0g-compute-cli (see scripts/zg-compute-smoke/ for setup steps).
//
// Sealed-flag semantics: the provider's TEE-signed response carries a
// signature header (name discovered in M7-C.1.0 setup). When the header is
// present and non-empty, we set llm.Response.Sealed=true. We do NOT
// cryptographically verify the signature — that requires TS tooling without
// a Go equivalent. Honest hackathon-scope limitation.
package zg_compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

// teeSigHeader is the response header name 0G providers use to attach the
// TEE-signed proof. Confirmed in M7-C.1.0 setup script's response dump.
// If the actual provider uses a different name, update here AND the test
// const at the top of zg_compute_test.go to match.
const teeSigHeader = "ZG-Res-Key"

// Config holds the 0G Compute client setup.
type Config struct {
	BearerToken      string        // app-sk-<...> generated via 0g-compute-cli
	ProviderEndpoint string        // per-provider base URL (no trailing /chat/completions)
	DefaultModel     string        // testnet: qwen-2.5-7b-instruct
	HTTPTimeout      time.Duration // default 60s
}

// Provider is an llm.Provider that talks to a 0G Compute provider.
type Provider struct {
	cfg    Config
	client *http.Client
}

var _ llm.Provider = (*Provider)(nil)

// New constructs a Provider. Defaults HTTPTimeout if zero.
func New(cfg Config) *Provider {
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 60 * time.Second
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: cfg.HTTPTimeout}}
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float32   `json:"temperature,omitempty"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
}

func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.DefaultModel
	}
	body := chatReq{
		Model:       model,
		Messages:    []chatMsg{{Role: "system", Content: req.SystemPrompt}, {Role: "user", Content: req.UserPrompt}},
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal req: %w", err)
	}

	url := strings.TrimRight(p.cfg.ProviderEndpoint, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return llm.Response{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.BearerToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("zg_compute request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("zg_compute read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return llm.Response{}, fmt.Errorf("zg_compute %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return llm.Response{}, fmt.Errorf("parse resp: %w; body=%s", err, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("zg_compute returned 0 choices: %s", string(respBody))
	}

	text := parsed.Choices[0].Message.Content
	usedModel := parsed.Model
	if usedModel == "" {
		usedModel = model
	}

	sealed := resp.Header.Get(teeSigHeader) != ""

	return llm.Response{
		Text:   text,
		Model:  usedModel,
		Sealed: sealed,
	}, nil
}
