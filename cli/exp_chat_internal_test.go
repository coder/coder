package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveContextSourcePath(t *testing.T) {
	t.Parallel()

	t.Run("EmptyErrors", func(t *testing.T) {
		t.Parallel()
		_, err := resolveContextSourcePath("   ")
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty")
	})

	t.Run("PreservesTilde", func(t *testing.T) {
		t.Parallel()
		// A leading ~ is left for the agent to expand against its own home.
		got, err := resolveContextSourcePath("~")
		require.NoError(t, err)
		require.Equal(t, "~", got)

		got, err = resolveContextSourcePath("  ~/skills/deploy  ")
		require.NoError(t, err)
		require.Equal(t, "~/skills/deploy", got)
	})

	t.Run("KeepsAbsolute", func(t *testing.T) {
		t.Parallel()
		got, err := resolveContextSourcePath("/home/coder/AGENTS.md")
		require.NoError(t, err)
		require.Equal(t, "/home/coder/AGENTS.md", got)
	})

	t.Run("MakesRelativeAbsolute", func(t *testing.T) {
		t.Parallel()
		// "./" was the reported failure: a relative path must be resolved to an
		// absolute one before it reaches the agent.
		got, err := resolveContextSourcePath("./")
		require.NoError(t, err)
		require.True(t, filepath.IsAbs(got), "want absolute, got %q", got)
		want, err := filepath.Abs("./")
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}
