package diffscan_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/diffscan"
)

func TestScan_UnifiedDiff_Clean(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
-func Hello() { return "hi" }
+func Hello() { return "hello" }
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	require.Empty(t, f)
}

func TestScan_UnifiedDiff_RemovedTest(t *testing.T) {
	diff := `--- a/foo_test.go
+++ b/foo_test.go
@@ -1,5 +1,1 @@
-func TestBar(t *testing.T) {
-	require.Equal(t, 1, 1)
-}
-
 func TestBaz(t *testing.T) {}
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	require.NotEmpty(t, f)
	// Find a removed_test finding
	hasRemoved := false
	for _, fn := range f {
		if fn.Rule == "removed_test" {
			hasRemoved = true
		}
	}
	require.True(t, hasRemoved)
}

func TestScan_UnifiedDiff_DeletedFile(t *testing.T) {
	diff := `--- a/bar_test.go
+++ /dev/null
@@ -1,5 +0,0 @@
-func TestBar(t *testing.T) {
-	require.Equal(t, 1, 1)
-}
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	hasDeleted := false
	hasRemoved := false
	for _, fn := range f {
		if fn.Rule == "deleted_test_file" {
			hasDeleted = true
			require.Equal(t, "bar_test.go", fn.Path)
		}
		if fn.Rule == "removed_test" {
			hasRemoved = true
		}
	}
	require.True(t, hasDeleted, "deleted_test_file must fire")
	require.True(t, hasRemoved, "removed_test must fire for test funcs inside a deleted file")
}

func TestScan_Multifile(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,1 @@
-a
+b
--- a/bar_test.go
+++ b/bar_test.go
@@ -1,0 +1,1 @@
+t.Skip("flaky")
`
	f, err := diffscan.Scan(diff)
	require.NoError(t, err)
	require.Len(t, f, 1)
	require.Equal(t, "skip_directive", f[0].Rule)
	require.Equal(t, "bar_test.go", f[0].Path)
}

func TestScanDiffs_Structured(t *testing.T) {
	fds := []diffscan.FileDiff{
		{Path: "foo_test.go", Added: []string{"\tt.Skip(\"x\")"}},
		{Path: "bar.go", Removed: []string{"func Hello() {}"}},
	}
	findings := diffscan.ScanDiffs(fds)
	require.Len(t, findings, 1)
	require.Equal(t, "skip_directive", findings[0].Rule)
}

func TestScan_EmptyInput(t *testing.T) {
	f, err := diffscan.Scan("")
	require.NoError(t, err)
	require.Empty(t, f)
}
