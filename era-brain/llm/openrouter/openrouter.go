// Package openrouter is a thin OpenRouter (OpenAI-compatible) client implementing llm.Provider.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
)

// Config holds the OpenRouter client setup.
type Config struct {
	APIKey       string        // required
	BaseURL      string        // default: https://openrouter.ai
	DefaultModel string        // required; per-request Model overrides
	HTTPTimeout  time.Duration // default: 60s
}

// Provider is an llm.Provider that talks to OpenRouter.
type Provider struct {
	cfg    Config
	client *http.Client
}

var _ llm.Provider = (*Provider)(nil)

// New constructs a Provider. Defaults BaseURL and HTTPTimeout if empty.
func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://openrouter.ai"
	}
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/api/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return llm.Response{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openrouter read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return llm.Response{}, fmt.Errorf("openrouter %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return llm.Response{}, fmt.Errorf("parse resp: %w; body=%s", err, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("openrouter returned 0 choices: %s", string(respBody))
	}

	text := parsed.Choices[0].Message.Content
	usedModel := parsed.Model
	if usedModel == "" {
		usedModel = model
	}

	return llm.Response{
		Text:   text,
		Model:  usedModel,
		Sealed: false,
	}, nil
}
