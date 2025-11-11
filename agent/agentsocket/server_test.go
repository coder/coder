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

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server, err := agentsocket.NewServer(socketPath, logger)
		require.NoError(t, err)
		require.NoError(t, server.Start())
		require.NoError(t, server.Stop())
	})

	t.Run("AlreadyStarted", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server, err := agentsocket.NewServer(socketPath, logger)
		require.NoError(t, err)
		require.NoError(t, server.Start())
		require.ErrorIs(t, server.Start(), agentsocket.ErrServerAlreadyStarted)
	})

	t.Run("AutoSocketPath", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server, err := agentsocket.NewServer(socketPath, logger)
		require.NoError(t, err)
		require.NoError(t, server.Start())
		require.NoError(t, server.Stop())
	})
}
