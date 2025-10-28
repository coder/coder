package agentsdk_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestSocketClient_Integration(t *testing.T) {
	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Set environment variable for socket discovery
	t.Setenv("CODER_AGENT_SOCKET_PATH", socketPath)

	// Start a real socket server
	server := startSocketServer(t, socketPath)
	defer server.Stop()

	// Create client
	client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{})
	require.NoError(t, err)
	defer client.Close()

	// Test ping
	ctx := context.Background()
	pingResp, err := client.Ping(ctx)
	require.NoError(t, err)
	assert.Equal(t, "pong", pingResp.Message)
	assert.False(t, pingResp.Timestamp.IsZero())

	// Test health
	healthResp, err := client.Health(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ready", healthResp.Status)
	assert.NotEmpty(t, healthResp.Uptime)

	// Test agent info
	agentInfo, err := client.AgentInfo(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-agent", agentInfo.ID)
	assert.Equal(t, "1.0.0", agentInfo.Version)
	assert.Equal(t, "ready", agentInfo.Status)

	// Test list methods
	methods, err := client.ListMethods(ctx)
	require.NoError(t, err)
	assert.Contains(t, methods, "ping")
	assert.Contains(t, methods, "health")
	assert.Contains(t, methods, "agent.info")
}

func TestSocketClient_Discovery(t *testing.T) {
	// Test with explicit path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	server := startSocketServer(t, socketPath)
	defer server.Stop()

	client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{Path: socketPath})
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	_, err = client.Ping(ctx)
	require.NoError(t, err)
}

func TestSocketClient_ErrorHandling(t *testing.T) {
	// Test with non-existent socket
	client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{Path: "/nonexistent/socket"})
	assert.Error(t, err)
	assert.Nil(t, client)
}

// startSocketServer starts a real socket server for testing
func startSocketServer(t *testing.T, path string) *agentsocket.Server {
	// Create server
	server := agentsocket.NewServer(agentsocket.Config{
		Path:   path,
		Logger: slog.Make().Leveled(slog.LevelDebug),
	})

	// Register default handlers with test data
	handlerCtx := agentsocket.CreateHandlerContext(
		"test-agent",
		"1.0.0",
		"ready",
		time.Now().Add(-time.Hour),
		slog.Make(),
	)
	agentsocket.RegisterDefaultHandlers(server, handlerCtx)

	// Start server
	err := server.Start()
	require.NoError(t, err)

	return server
}
