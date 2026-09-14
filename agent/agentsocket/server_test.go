package agentsocket_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/testutil"
)

func TestServer(t *testing.T) {
	t.Parallel()

	t.Run("StartStop", func(t *testing.T) {
		t.Parallel()

		socketPath := testutil.AgentSocketPath(t)
		logger := slog.Make().Leveled(slog.LevelDebug)
		server, err := agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.NoError(t, err)
		require.NoError(t, server.Close())
	})

	t.Run("AlreadyStarted", func(t *testing.T) {
		t.Parallel()

		socketPath := testutil.AgentSocketPath(t)
		logger := slog.Make().Leveled(slog.LevelDebug)
		server1, err := agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.NoError(t, err)
		defer server1.Close()
		_, err = agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.ErrorContains(t, err, "create socket")
	})
}
