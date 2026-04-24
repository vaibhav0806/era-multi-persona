package main

import (
	"net"
	"strings"
	"sync"
	"time"
)

// staticHosts are always permitted. Hostnames are matched exactly OR as a
// suffix-with-leading-dot (so "foo.example.com" matches ".example.com" entry).
var staticHosts = []string{
	// LLM (openrouter.ai hosts the API at the apex domain; api.openrouter.ai is NXDOMAIN)
	"openrouter.ai",
	// GitHub (push, clone, raw)
	"github.com", "api.github.com",
	"raw.githubusercontent.com", "objects.githubusercontent.com",
	"codeload.github.com",
	// Package registries
	"registry.npmjs.org", ".npmjs.org",
	"pypi.org", "files.pythonhosted.org",
	"proxy.golang.org", "sum.golang.org", "storage.googleapis.com",
	"crates.io", "static.crates.io",
	// Doc hosts (low-noise, high-value)
	"developer.mozilla.org", "docs.python.org",
	"pkg.go.dev", "go.dev",
	"docs.rs",
	"nodejs.org",
	"stackoverflow.com", "stackexchange.com",
}

type allowlist struct {
	mu         sync.Mutex
	dynamic    map[string]time.Time // host → expiry
	staticSet  map[string]struct{}
	staticSuff []string // entries beginning with "."
}

func newAllowlist() *allowlist {
	a := &allowlist{
		dynamic:   make(map[string]time.Time),
		staticSet: make(map[string]struct{}),
	}
	for _, h := range staticHosts {
		if strings.HasPrefix(h, ".") {
			a.staticSuff = append(a.staticSuff, h)
		} else {
			a.staticSet[h] = struct{}{}
		}
	}
	return a
}

func (a *allowlist) allowed(hostport string) bool {
	hostport = strings.ToLower(strings.TrimSpace(hostport))
	if hostport == "" {
		return false
	}
	// Strip port to get bare hostname for static checks.
	hostname := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		hostname = h
	}
	if _, ok := a.staticSet[hostname]; ok {
		return true
	}
	for _, suf := range a.staticSuff {
		if strings.HasSuffix(hostname, suf) {
			return true
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	// Check exact match (host:port form as stored by permit).
	if expiry, ok := a.dynamic[hostport]; ok {
		if now.After(expiry) {
			delete(a.dynamic, hostport)
		} else {
			return true
		}
	}
	// Also check hostname-only form (permit may have been called without port).
	if hostname != hostport {
		if expiry, ok := a.dynamic[hostname]; ok {
			if now.After(expiry) {
				delete(a.dynamic, hostname)
			} else {
				return true
			}
		}
	}
	return false
}

// permit dynamically allows host until expiry. Used by /search to allow /fetch
// to retrieve URLs returned in recent search results.
func (a *allowlist) permit(host string, ttl time.Duration) {
	host = strings.ToLower(strings.TrimSpace(host))
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dynamic[host] = time.Now().Add(ttl)
}
