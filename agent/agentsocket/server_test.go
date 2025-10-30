package agentsocket_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket"
)

func TestServer(t *testing.T) {
	t.Parallel()

	t.Run("StartStop", func(t *testing.T) {
		t.Parallel()

		server := agentsocket.NewServer(agentsocket.Config{
			Path:   filepath.Join(t.TempDir(), "test.sock"),
			Logger: slog.Make().Leveled(slog.LevelDebug),
		})
		require.NoError(t, server.Start())
		require.NoError(t, server.Stop())
	})

	t.Run("AlreadyStarted", func(t *testing.T) {
		t.Parallel()

		server := agentsocket.NewServer(agentsocket.Config{
			Path:   filepath.Join(t.TempDir(), "test.sock"),
			Logger: slog.Make().Leveled(slog.LevelDebug),
		})
		require.NoError(t, server.Start())
		require.ErrorIs(t, server.Start(), agentsocket.ErrServerAlreadyStarted)
	})

	t.Run("AutoSocketPath", func(t *testing.T) {
		t.Parallel()

		server := agentsocket.NewServer(agentsocket.Config{
			Logger: slog.Make().Leveled(slog.LevelDebug),
		})
		// The details of how a socket path is chosen are tested separately.
		// Here, we just want to make sure it doesn't break the server.
		require.NoError(t, server.Start())
		require.NoError(t, server.Stop())
	})
}
