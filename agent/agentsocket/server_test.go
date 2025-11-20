package agentsocket_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket"
)

func TestServer(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("agentsocket is not supported on Windows")
	}

	t.Run("StartStop", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server, err := agentsocket.NewServer(socketPath, logger)
		require.NoError(t, err)
		require.NoError(t, server.Close())
	})

	t.Run("AlreadyStarted", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server1, err := agentsocket.NewServer(socketPath, logger)
		require.NoError(t, err)
		defer server1.Close()
		_, err = agentsocket.NewServer(socketPath, logger)
		require.ErrorContains(t, err, "create socket")
	})

	t.Run("AutoSocketPath", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server, err := agentsocket.NewServer(socketPath, logger)
		require.NoError(t, err)
		require.NoError(t, server.Close())
	})
}
