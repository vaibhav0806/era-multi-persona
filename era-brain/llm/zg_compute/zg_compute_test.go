package zg_compute_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm"
	"github.com/vaibhav0806/era-multi-persona/era-brain/llm/zg_compute"
)

// TEE signature header name — REPLACE with the actual name discovered in
// Phase 0 if it differs from "ZG-Res-Key".
const teeSigHeader = "ZG-Res-Key"

func TestZGCompute_HappyPath_SealedTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer app-sk-test", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"role":"system"`)
		require.Contains(t, string(body), `"role":"user"`)
		require.Contains(t, string(body), `"model":"qwen-2.5-7b-instruct"`)

		w.Header().Set(teeSigHeader, "0xabc123signature")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "the response"}},
			},
			"model": "qwen-2.5-7b-instruct",
		})
	}))
	defer srv.Close()

	p := zg_compute.New(zg_compute.Config{
		BearerToken:      "app-sk-test",
		ProviderEndpoint: srv.URL,
		DefaultModel:     "qwen-2.5-7b-instruct",
	})

	resp, err := p.Complete(context.Background(), llm.Request{
		SystemPrompt: "sys",
		UserPrompt:   "user",
	})
	require.NoError(t, err)
	require.Equal(t, "the response", resp.Text)
	require.Equal(t, "qwen-2.5-7b-instruct", resp.Model)
	require.True(t, resp.Sealed, "TEE-signature header present → Sealed=true")
}

func TestZGCompute_NoTEEHeader_SealedFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No TEE header set.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "x"}}},
			"model":   "qwen-2.5-7b-instruct",
		})
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "qwen-2.5-7b-instruct",
	})
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "u"})
	require.NoError(t, err)
	require.False(t, resp.Sealed, "no TEE header → Sealed=false")
}

func TestZGCompute_PerRequestModelOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"model":"custom-model"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "x"}}},
			"model":   "custom-model",
		})
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "default-model",
	})
	resp, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x", Model: "custom-model"})
	require.NoError(t, err)
	require.Equal(t, "custom-model", resp.Model)
}

func TestZGCompute_HTTPErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "m",
	})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "unauthorized"))
}

func TestZGCompute_EmptyChoicesIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}, "model": "m"})
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "m",
	})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
}

func TestZGCompute_MalformedJSONIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer srv.Close()
	p := zg_compute.New(zg_compute.Config{
		BearerToken: "k", ProviderEndpoint: srv.URL, DefaultModel: "m",
	})
	_, err := p.Complete(context.Background(), llm.Request{UserPrompt: "x"})
	require.Error(t, err)
}
