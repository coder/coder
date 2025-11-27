package agentsocket_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
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
		server, err := agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.NoError(t, err)
		require.NoError(t, server.Close())
	})

	t.Run("AlreadyStarted", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server1, err := agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.NoError(t, err)
		defer server1.Close()
		_, err = agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.ErrorContains(t, err, "create socket")
	})

	t.Run("AutoSocketPath", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		server, err := agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.NoError(t, err)
		require.NoError(t, server.Close())
	})
}

func TestServerWindowsNotSupported(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("this test only runs on Windows")
	}

	t.Run("NewServer", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")
		logger := slog.Make().Leveled(slog.LevelDebug)
		_, err := agentsocket.NewServer(logger, agentsocket.WithPath(socketPath))
		require.ErrorContains(t, err, "agentsocket is not supported on Windows")
	})

	t.Run("NewClient", func(t *testing.T) {
		t.Parallel()

		_, err := agentsocket.NewClient(context.Background(), agentsocket.WithPath("test.sock"))
		require.ErrorContains(t, err, "agentsocket is not supported on Windows")
	})
}

func TestAgentInitializesOnWindowsWithoutSocketServer(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("this test only runs on Windows")
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t).Named("agent")

	derpMap, _ := tailnettest.RunDERPAndSTUN(t)

	coordinator := tailnet.NewCoordinator(logger)
	t.Cleanup(func() {
		_ = coordinator.Close()
	})

	statsCh := make(chan *agentproto.Stats, 50)
	agentID := uuid.New()
	manifest := agentsdk.Manifest{
		AgentID:       agentID,
		AgentName:     "test-agent",
		WorkspaceName: "test-workspace",
		OwnerName:     "test-user",
		WorkspaceID:   uuid.New(),
		DERPMap:       derpMap,
	}

	client := agenttest.NewClient(t, logger.Named("agenttest"), agentID, manifest, statsCh, coordinator)
	t.Cleanup(client.Close)

	options := agent.Options{
		Client:                 client,
		Filesystem:             afero.NewMemMapFs(),
		Logger:                 logger.Named("agent"),
		ReconnectingPTYTimeout: testutil.WaitShort,
		EnvironmentVariables:   map[string]string{},
		SocketPath:             "",
	}

	agnt := agent.New(options)
	t.Cleanup(func() {
		_ = agnt.Close()
	})

	startup := testutil.TryReceive(ctx, t, client.GetStartup())
	require.NotNil(t, startup, "agent should send startup message")

	err := agnt.Close()
	require.NoError(t, err, "agent should close cleanly")
}
