package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLLM_ForwardsWithInjectedAuth(t *testing.T) {
	var (
		recvdAuth string
		recvdPath string
		recvdBody map[string]interface{}
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recvdAuth = r.Header.Get("Authorization")
		recvdPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&recvdBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-x","choices":[{"message":{"content":"hi"}}]}`))
	}))
	defer upstream.Close()

	h := newLLMHandler(upstream.URL, "sk-or-real-key")

	req := httptest.NewRequest("POST", "/llm/v1/chat/completions",
		strings.NewReader(`{"model":"moonshotai/kimi-k2.6","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer pi-dummy") // should be replaced
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, 200, rec.Code)
	require.Equal(t, "Bearer sk-or-real-key", recvdAuth, "sidecar must inject real key, not pi's dummy")
	require.Equal(t, "/api/v1/chat/completions", recvdPath, "path /llm/v1/chat/completions → /api/v1/chat/completions")
	require.Equal(t, "moonshotai/kimi-k2.6", recvdBody["model"])

	body, _ := io.ReadAll(rec.Body)
	require.Contains(t, string(body), "chatcmpl-x")
}

func TestLLM_StripsClientAuth(t *testing.T) {
	var recvdAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recvdAuth = r.Header.Get("Authorization")
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	h := newLLMHandler(upstream.URL, "real-key")
	req := httptest.NewRequest("POST", "/llm/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sensitive-value-that-should-not-leak")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, "Bearer real-key", recvdAuth, "client's Authorization must be stripped before forwarding")
}

func TestLLM_MissingKeyReturns503(t *testing.T) {
	h := newLLMHandler("http://unused", "") // empty key
	req := httptest.NewRequest("POST", "/llm/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, 503, rec.Code)
	body, _ := io.ReadAll(rec.Body)
	require.Contains(t, string(body), "OpenRouter API key not configured")
}

func TestLLM_NonLLMPathReturns404(t *testing.T) {
	h := newLLMHandler("http://unused", "k")
	req := httptest.NewRequest("GET", "/not-llm", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, 404, rec.Code)
}

func TestLLM_PreservesStatusAndBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer upstream.Close()

	h := newLLMHandler(upstream.URL, "k")
	req := httptest.NewRequest("POST", "/llm/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, 429, rec.Code)
	body, _ := io.ReadAll(rec.Body)
	require.Contains(t, string(body), "rate limited")
}
