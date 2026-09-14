package agentcontextconfig_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
)

// platformAbsPath constructs an absolute path that is valid
// on the current platform. On Windows paths must include a
// drive letter to be considered absolute.
func platformAbsPath(parts ...string) string {
	if runtime.GOOS == "windows" {
		return `C:\` + filepath.Join(parts...)
	}
	return "/" + filepath.Join(parts...)
}

func TestResolvePath(t *testing.T) { //nolint:tparallel // subtests using t.Setenv cannot be parallel
	t.Run("EmptyInput", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", agentcontextconfig.ResolvePath("", platformAbsPath("base")))
	})

	t.Run("WhitespaceOnly", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", agentcontextconfig.ResolvePath("   ", platformAbsPath("base")))
	})

	// Tests that use t.Setenv cannot be parallel.
	t.Run("TildeAlone", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		got := agentcontextconfig.ResolvePath("~", platformAbsPath("base"))
		require.Equal(t, fakeHome, got)
	})

	t.Run("TildeSlashPath", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		got := agentcontextconfig.ResolvePath("~/docs/readme", platformAbsPath("base"))
		require.Equal(t, filepath.Join(fakeHome, "docs", "readme"), got)
	})

	t.Run("AbsolutePath", func(t *testing.T) {
		t.Parallel()
		p := platformAbsPath("etc", "coder")
		got := agentcontextconfig.ResolvePath(p, platformAbsPath("base"))
		require.Equal(t, p, got)
	})

	t.Run("RelativePath", func(t *testing.T) {
		t.Parallel()
		base := platformAbsPath("work")
		got := agentcontextconfig.ResolvePath("foo/bar", base)
		require.Equal(t, filepath.Join(base, "foo", "bar"), got)
	})

	t.Run("RelativePathWithWhitespace", func(t *testing.T) {
		t.Parallel()
		base := platformAbsPath("work")
		got := agentcontextconfig.ResolvePath("  foo/bar  ", base)
		require.Equal(t, filepath.Join(base, "foo", "bar"), got)
	})

	t.Run("RelativePathWithEmptyBaseDir", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePath(".agents/skills", "")
		require.Equal(t, "", got)
	})
}

func TestResolvePath_HomeUnset(t *testing.T) {
	// Cannot be parallel — modifies HOME env var.
	t.Setenv("HOME", "")
	// Also clear USERPROFILE for Windows compatibility.
	t.Setenv("USERPROFILE", "")

	require.Equal(t, "", agentcontextconfig.ResolvePath("~", platformAbsPath("base")))
	require.Equal(t, "", agentcontextconfig.ResolvePath("~/docs", platformAbsPath("base")))
}

func TestResolvePaths(t *testing.T) { //nolint:tparallel // subtests using t.Setenv cannot be parallel
	t.Run("EmptyString", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, agentcontextconfig.ResolvePaths("", platformAbsPath("base")))
	})

	t.Run("WhitespaceOnly", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, agentcontextconfig.ResolvePaths("   ", platformAbsPath("base")))
	})

	t.Run("SingleEntry", func(t *testing.T) {
		t.Parallel()
		p := platformAbsPath("abs", "path")
		got := agentcontextconfig.ResolvePaths(p, platformAbsPath("base"))
		require.Equal(t, []string{p}, got)
	})

	// Tests that use t.Setenv cannot be parallel.
	t.Run("MultipleEntries", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		b := platformAbsPath("b")
		base := platformAbsPath("base")
		got := agentcontextconfig.ResolvePaths("~/a,"+b+",rel", base)
		require.Equal(t, []string{
			filepath.Join(fakeHome, "a"),
			b,
			filepath.Join(base, "rel"),
		}, got)
	})

	t.Run("TrimsWhitespace", func(t *testing.T) {
		t.Parallel()
		a := platformAbsPath("a")
		b := platformAbsPath("b")
		got := agentcontextconfig.ResolvePaths("  "+a+" , "+b+"  ", platformAbsPath("base"))
		require.Equal(t, []string{a, b}, got)
	})

	t.Run("SkipsEmptyEntries", func(t *testing.T) {
		t.Parallel()
		a := platformAbsPath("a")
		b := platformAbsPath("b")
		got := agentcontextconfig.ResolvePaths(a+",,"+b+",", platformAbsPath("base"))
		require.Equal(t, []string{a, b}, got)
	})

	t.Run("TrailingComma", func(t *testing.T) {
		t.Parallel()
		p := platformAbsPath("only")
		got := agentcontextconfig.ResolvePaths(p+",", platformAbsPath("base"))
		require.Equal(t, []string{p}, got)
	})

	t.Run("RelativePathSkippedWhenBaseDirEmpty", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		t.Setenv("USERPROFILE", fakeHome)
		got := agentcontextconfig.ResolvePaths("~/.coder,.agents/skills", "")
		require.Equal(t, []string{filepath.Join(fakeHome, ".coder")}, got)
	})
}
