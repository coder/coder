package agentcontextconfig_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
)

func TestResolvePath(t *testing.T) {

	t.Run("EmptyInput", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", agentcontextconfig.ResolvePath("", "/base"))
	})

	t.Run("WhitespaceOnly", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", agentcontextconfig.ResolvePath("   ", "/base"))
	})

	// Tests that use t.Setenv cannot be parallel.
	t.Run("TildeAlone", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		got := agentcontextconfig.ResolvePath("~", "/base")
		require.Equal(t, fakeHome, got)
	})

	t.Run("TildeSlashPath", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		got := agentcontextconfig.ResolvePath("~/docs/readme", "/base")
		require.Equal(t, filepath.Join(fakeHome, "docs/readme"), got)
	})

	t.Run("AbsolutePath", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePath("/etc/coder", "/base")
		require.Equal(t, "/etc/coder", got)
	})

	t.Run("RelativePath", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePath("foo/bar", "/work")
		require.Equal(t, "/work/foo/bar", got)
	})

	t.Run("RelativePathWithWhitespace", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePath("  foo/bar  ", "/work")
		require.Equal(t, "/work/foo/bar", got)
	})
}

func TestResolvePaths(t *testing.T) {

	t.Run("EmptyString", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, agentcontextconfig.ResolvePaths("", "/base"))
	})

	t.Run("WhitespaceOnly", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, agentcontextconfig.ResolvePaths("   ", "/base"))
	})

	t.Run("SingleEntry", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePaths("/abs/path", "/base")
		require.Equal(t, []string{"/abs/path"}, got)
	})

	// Tests that use t.Setenv cannot be parallel.
	t.Run("MultipleEntries", func(t *testing.T) {
		fakeHome := t.TempDir()
		t.Setenv("HOME", fakeHome)
		got := agentcontextconfig.ResolvePaths("~/a,/b,rel", "/base")
		require.Equal(t, []string{
			filepath.Join(fakeHome, "a"),
			"/b",
			"/base/rel",
		}, got)
	})

	t.Run("TrimsWhitespace", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePaths("  /a , /b  ", "/base")
		require.Equal(t, []string{"/a", "/b"}, got)
	})

	t.Run("SkipsEmptyEntries", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePaths("/a,,/b,", "/base")
		require.Equal(t, []string{"/a", "/b"}, got)
	})

	t.Run("TrailingComma", func(t *testing.T) {
		t.Parallel()
		got := agentcontextconfig.ResolvePaths("/only,", "/base")
		require.Equal(t, []string{"/only"}, got)
	})
}
