package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditMiddleware_LogsRequestLine(t *testing.T) {
	var buf bytes.Buffer
	mw := newAuditMiddleware(&buf)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))

	req := httptest.NewRequest("GET", "/health?x=1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	line := buf.String()
	require.Contains(t, line, `"method":"GET"`)
	require.Contains(t, line, `"path":"/health"`)
	require.Contains(t, line, `"status":204`)
	require.True(t, strings.HasPrefix(line, "AUDIT "), "audit lines should be prefixed AUDIT for grep-ability")
}
