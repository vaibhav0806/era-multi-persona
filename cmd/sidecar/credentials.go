package main

import (
	"fmt"
	"net/http"
)

// credentialsHandler serves git credentials in the standard git-credential-helper
// key=value format. The PAT lives in sidecar env (PI_SIDECAR_GITHUB_PAT) and is
// never exposed to the runner or to Pi.
type credentialsHandler struct {
	pat string
}

func newCredentialsHandler(pat string) http.Handler {
	return &credentialsHandler{pat: pat}
}

func (h *credentialsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.pat == "" {
		http.Error(w, "GitHub PAT not configured (set PI_SIDECAR_GITHUB_PAT)", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Git credential helper format:
	//   key=value\n
	//   ...
	//   \n  (blank line terminator)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "username=x-access-token\npassword=%s\n\n", h.pat)
}
