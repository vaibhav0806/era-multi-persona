package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func newServer(cfg *sidecarConfig) *http.Server {
	allow := newAllowlist()

	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	var testPermitHandler http.HandlerFunc
	if os.Getenv("PI_SIDECAR_TEST_HOOKS") == "1" {
		testPermitHandler = newTestPermitHandler(allow)
	}
	searchHandler := newSearchHandler("", cfg.TavilyAPIKey, allow)
	fetchHandler := newFetchHandler(allow)
	llmHandler := newLLMHandler("", cfg.OpenRouterAPIKey)
	credentialsHandler := newCredentialsHandler(cfg.GitHubPAT)
	proxy := newProxyHandler(allow)

	// Route manually instead of using http.ServeMux. Go 1.22+ ServeMux
	// normalises paths and issues 301 redirects for CONNECT requests that
	// arrive with an empty path, breaking the forward-proxy tunnel. A plain
	// HandlerFunc avoids that redirect entirely.
	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			healthHandler.ServeHTTP(w, r)
		case "/_test/permit":
			if testPermitHandler != nil {
				testPermitHandler.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
		case "/search":
			searchHandler.ServeHTTP(w, r)
		case "/fetch":
			fetchHandler.ServeHTTP(w, r)
		case "/credentials/git":
			credentialsHandler.ServeHTTP(w, r)
		default:
			if strings.HasPrefix(r.URL.Path, "/llm/") {
				llmHandler.ServeHTTP(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		}
	})

	audited := newAuditMiddleware(os.Stderr)(root)
	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           audited,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func runServer(ctx context.Context, srv *http.Server) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
