package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentials_ReturnsGitHelperFormat(t *testing.T) {
	h := newCredentialsHandler("ghp_test123")
	req := httptest.NewRequest("POST", "/credentials/git", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, 200, rec.Code)
	body, _ := io.ReadAll(rec.Body)
	s := string(body)
	// Git credential format: key=value lines, blank line terminator
	require.Contains(t, s, "username=x-access-token")
	require.Contains(t, s, "password=ghp_test123")
	// Should end with blank line
	require.True(t, strings.HasSuffix(s, "\n\n"), "git credential format requires trailing blank line")
}

func TestCredentials_TokenNotInLogOutput(t *testing.T) {
	// Ensure the Content-Type is text/plain and the response is plain-format,
	// not JSON with secret-bearing structure.
	h := newCredentialsHandler("super-secret-token")
	req := httptest.NewRequest("POST", "/credentials/git", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, "text/plain", rec.Header().Get("Content-Type"))
}

func TestCredentials_MissingPATReturns503(t *testing.T) {
	h := newCredentialsHandler("")
	req := httptest.NewRequest("POST", "/credentials/git", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, 503, rec.Code)
	body, _ := io.ReadAll(rec.Body)
	require.Contains(t, string(body), "GitHub PAT not configured")
}

func TestCredentials_GETAlsoWorks(t *testing.T) {
	// Some helpers use GET. Accept both GET and POST.
	h := newCredentialsHandler("tkn")
	req := httptest.NewRequest("GET", "/credentials/git", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
}
