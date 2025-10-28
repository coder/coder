package agentsdk

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocketClient_Integration(t *testing.T) {
	t.Parallel()

	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Set environment variable for socket discovery
	oldPath := os.Getenv("CODER_AGENT_SOCKET_PATH")
	defer os.Setenv("CODER_AGENT_SOCKET_PATH", oldPath)
	os.Setenv("CODER_AGENT_SOCKET_PATH", socketPath)

	// Start a mock server
	server := startMockServer(t, socketPath)
	defer server.Stop()

	// Create client
	client, err := NewSocketClient(SocketConfig{})
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
	t.Parallel()

	// Test with explicit path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	server := startMockServer(t, socketPath)
	defer server.Stop()

	client, err := NewSocketClient(SocketConfig{Path: socketPath})
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	_, err = client.Ping(ctx)
	require.NoError(t, err)
}

func TestSocketClient_ErrorHandling(t *testing.T) {
	t.Parallel()

	// Test with non-existent socket
	client, err := NewSocketClient(SocketConfig{Path: "/nonexistent/socket"})
	assert.Error(t, err)
	assert.Nil(t, client)
}

// Mock server for testing
type mockServer struct {
	path string
	// Add server implementation here if needed
}

func startMockServer(t *testing.T, path string) *mockServer {
	// This is a simplified mock - in a real test you'd start an actual server
	// For now, we'll just return a mock that can be stopped
	return &mockServer{path: path}
}

func (s *mockServer) Stop() {
	// Clean up mock server
	os.Remove(s.path)
}
