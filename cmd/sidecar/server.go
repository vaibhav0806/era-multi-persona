package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func newServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	audited := newAuditMiddleware(os.Stderr)(mux)
	return &http.Server{
		Addr:              addr,
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
