package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAllowlist_StaticHostsAllowed(t *testing.T) {
	a := newAllowlist()
	require.True(t, a.allowed("openrouter.ai"))
	require.True(t, a.allowed("github.com"))
	require.True(t, a.allowed("api.github.com"))
	require.True(t, a.allowed("registry.npmjs.org"))
	require.True(t, a.allowed("pypi.org"))
	require.True(t, a.allowed("proxy.golang.org"))
	require.True(t, a.allowed("storage.googleapis.com"))
	require.True(t, a.allowed("crates.io"))
	require.True(t, a.allowed("developer.mozilla.org"))
	require.True(t, a.allowed("docs.python.org"))
	require.True(t, a.allowed("stackoverflow.com"))
}

func TestAllowlist_UnknownHostBlocked(t *testing.T) {
	a := newAllowlist()
	require.False(t, a.allowed("evil.com"))
	require.False(t, a.allowed("attacker.example"))
	require.False(t, a.allowed(""))
}

func TestAllowlist_SuffixMatch(t *testing.T) {
	a := newAllowlist()
	// .npmjs.org is in the static suffix set, so subdomains match.
	require.True(t, a.allowed("registry.npmjs.org"))
	require.True(t, a.allowed("docs.npmjs.org"))
	require.False(t, a.allowed("npmjs.org.evil.com"))
}

func TestAllowlist_StaticHostsAllowed_M6Additions(t *testing.T) {
	a := newAllowlist()
	require.True(t, a.allowed("crates.io"))
	require.True(t, a.allowed("static.crates.io"))
	require.True(t, a.allowed("index.crates.io"))
	require.True(t, a.allowed("registry.yarnpkg.com"))
	require.True(t, a.allowed("cdn.jsdelivr.net"))
	require.True(t, a.allowed("cdnjs.cloudflare.com"))
	require.True(t, a.allowed("unpkg.com"))
	require.True(t, a.allowed("fonts.googleapis.com"))
	require.True(t, a.allowed("fonts.gstatic.com"))
	require.True(t, a.allowed("services.gradle.org"))
}

func TestPIEgressExtra_AppendsHosts(t *testing.T) {
	t.Setenv("PI_EGRESS_EXTRA", "foo.example.com,bar.example.org")
	a := newAllowlist()
	require.True(t, a.allowed("foo.example.com"))
	require.True(t, a.allowed("bar.example.org"))
}

func TestPIEgressExtra_EmptyWhitespaceSkipped(t *testing.T) {
	t.Setenv("PI_EGRESS_EXTRA", "foo.example.com, ,  bar.example.org , ")
	a := newAllowlist()
	require.True(t, a.allowed("foo.example.com"))
	require.True(t, a.allowed("bar.example.org"))
	require.False(t, a.allowed(""))
}

func TestPIEgressExtra_Unset_NoChange(t *testing.T) {
	t.Setenv("PI_EGRESS_EXTRA", "")
	a := newAllowlist()
	require.False(t, a.allowed("notinlist.example.com"))
}

func TestAllowlist_DynamicHostAddedAndExpires(t *testing.T) {
	a := newAllowlist()
	require.False(t, a.allowed("docs.example.com"))
	a.permit("docs.example.com", 100*time.Millisecond)
	require.True(t, a.allowed("docs.example.com"))
	time.Sleep(150 * time.Millisecond)
	require.False(t, a.allowed("docs.example.com"))
}
