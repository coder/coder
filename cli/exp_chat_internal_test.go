package cli

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestParseChatID(t *testing.T) {
	t.Parallel()

	t.Run("EmptyIsNil", func(t *testing.T) {
		t.Parallel()
		got, err := parseChatID("")
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, got)
	})

	t.Run("ValidUUID", func(t *testing.T) {
		t.Parallel()
		want := uuid.MustParse("11111111-1111-4111-8111-111111111111")
		got, err := parseChatID(want.String())
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("InvalidErrors", func(t *testing.T) {
		t.Parallel()
		_, err := parseChatID("not-a-uuid")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid chat ID")
	})
}

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
