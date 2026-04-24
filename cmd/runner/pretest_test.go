package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasMakefileTest_PresentTarget(t *testing.T) {
	require.True(t, HasMakefileTest("testdata/makefile_with_test"))
}

func TestHasMakefileTest_NoTestTarget(t *testing.T) {
	require.False(t, HasMakefileTest("testdata/makefile_no_test"))
}

func TestHasMakefileTest_NoMakefile(t *testing.T) {
	require.False(t, HasMakefileTest("testdata/no_makefile"))
}

func TestHasMakefileTest_TestAllNotMatched(t *testing.T) {
	// Regex `^test\s*:` must NOT match `test-all:` or `test_unit:`.
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(tmp+"/Makefile", []byte("test-all:\n\t@echo x\n"), 0644))
	require.False(t, HasMakefileTest(tmp))
}

func TestHasMakefileTest_CommentedTargetNotMatched(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(tmp+"/Makefile", []byte("# test:\n\t@echo x\n"), 0644))
	require.False(t, HasMakefileTest(tmp))
}
