package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestServer_Health(t *testing.T) {
	addr := freePort(t)
	srv := newServer(addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runServer(ctx, srv)
	time.Sleep(50 * time.Millisecond) // let it bind

	resp, err := http.Get("http://" + addr + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, "ok\n", string(body))
}
