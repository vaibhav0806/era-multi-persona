package main

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// llmHandler proxies /llm/* requests to OpenRouter with auth injection.
// The real API key lives in the sidecar (from PI_SIDECAR_OPENROUTER_API_KEY);
// clients never see it. The client's own Authorization header (if any) is
// stripped and replaced.
type llmHandler struct {
	upstreamBase string // e.g. "https://openrouter.ai" (so /llm/v1/x → <base>/api/v1/x)
	apiKey       string
	client       *http.Client
}

func newLLMHandler(upstreamBase, apiKey string) http.Handler {
	// Default to OpenRouter if no override supplied.
	if upstreamBase == "" {
		upstreamBase = "https://openrouter.ai"
	}
	return &llmHandler{
		upstreamBase: strings.TrimRight(upstreamBase, "/"),
		apiKey:       apiKey,
		client:       &http.Client{Timeout: 5 * time.Minute}, // LLM calls can be slow
	}
}

func (h *llmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/llm/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if h.apiKey == "" {
		http.Error(w, "OpenRouter API key not configured (set PI_SIDECAR_OPENROUTER_API_KEY)", http.StatusServiceUnavailable)
		return
	}

	// /llm/v1/chat/completions → /api/v1/chat/completions
	tail := strings.TrimPrefix(r.URL.Path, "/llm/")
	upstreamURL := h.upstreamBase + "/api/" + tail
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Copy headers, but strip Authorization (client may have sent a dummy)
	// and a few hop-by-hop headers.
	for k, vv := range r.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "proxy-connection" || lk == "connection" {
			continue
		}
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Header.Set("Authorization", "Bearer "+h.apiKey)

	resp, err := h.client.Do(outReq)
	if err != nil {
		http.Error(w, "llm upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers + status + body verbatim.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
