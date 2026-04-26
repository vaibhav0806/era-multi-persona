package openrouter_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/openrouter"
)

func TestOpenRouter_Complete_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"role":"system"`)
		assert.Contains(t, string(body), `"role":"user"`)
		assert.Contains(t, string(body), `"content":"sys-prompt"`)
		assert.Contains(t, string(body), `"content":"user-prompt"`)
		assert.Contains(t, string(body), `"model":"openai/gpt-4o-mini"`)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "the response"}},
			},
			"model": "openai/gpt-4o-mini",
		})
	}))
	defer srv.Close()

	p := openrouter.New(openrouter.Config{
		APIKey:       "test-key",
		BaseURL:      srv.URL,
		DefaultModel: "openai/gpt-4o-mini",
	})

	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "sys-prompt",
		UserPrompt:   "user-prompt",
	})
	require.NoError(t, err)
	require.Equal(t, "the response", resp.Text)
	require.Equal(t, "openai/gpt-4o-mini", resp.Model)
	require.False(t, resp.Sealed)
}

func TestOpenRouter_Complete_PerRequestModelOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"model":"qwen/qwen3-30b"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "x"}}},
			"model":   "qwen/qwen3-30b",
		})
	}))
	defer srv.Close()
	p := openrouter.New(openrouter.Config{APIKey: "k", BaseURL: srv.URL, DefaultModel: "default-model"})
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x", Model: "qwen/qwen3-30b"})
	require.NoError(t, err)
	require.Equal(t, "qwen/qwen3-30b", resp.Model)
}

func TestOpenRouter_Complete_HTTPErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := openrouter.New(openrouter.Config{APIKey: "k", BaseURL: srv.URL, DefaultModel: "m"})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate limited"))
}

func TestOpenRouter_Complete_EmptyChoicesIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}, "model": "m"})
	}))
	defer srv.Close()
	p := openrouter.New(openrouter.Config{APIKey: "k", BaseURL: srv.URL, DefaultModel: "m"})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
}
