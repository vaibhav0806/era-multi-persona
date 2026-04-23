package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg, err := loadSidecarConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sidecar config: %v\n", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("sidecar starting", "addr", cfg.ListenAddr)
	srv := newServer(cfg.ListenAddr)
	if err := runServer(ctx, srv); err != nil {
		fmt.Fprintf(os.Stderr, "sidecar: %v\n", err)
		os.Exit(1)
	}
	slog.Info("sidecar shutdown clean")
}
