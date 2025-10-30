package agentsdk_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestSocketClient_SyncWait(t *testing.T) {
	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Set environment variable for socket discovery
	t.Setenv("CODER_AGENT_SOCKET_PATH", socketPath)

	// Start a real socket server with dependency tracker
	server := startSocketServerWithDependencyTracker(t, socketPath)
	t.Cleanup(func() { server.Stop() })

	// Create client
	client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()

	t.Run("SyncWait_UnitNotRegistered", func(t *testing.T) {
		// Test sync ready on unregistered unit - should return error
		err := client.SyncReady(ctx, "nonexistent-unit")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sync ready failed: Failed to check readiness")
	})

	t.Run("SyncWait_UnitWithUnsatisfiedDependencies", func(t *testing.T) {
		// Register a unit with dependencies
		err := client.SyncStart(ctx, "test-unit")
		require.NoError(t, err)

		// Add a dependency that's not satisfied
		err = client.SyncWant(ctx, "test-unit", "dependency-unit")
		require.NoError(t, err)

		// Sync ready should return error because dependency is not satisfied
		err = client.SyncReady(ctx, "test-unit")
		require.Error(t, err)
		assert.ErrorIs(t, err, unit.ErrDependenciesNotSatisfied)
	})

	t.Run("SyncWait_UnitWithSatisfiedDependencies", func(t *testing.T) {
		// Register dependency unit and mark it as complete
		err := client.SyncStart(ctx, "dependency-unit")
		require.NoError(t, err)
		err = client.SyncComplete(ctx, "dependency-unit")
		require.NoError(t, err)

		// Now sync wait should succeed
		err = client.SyncReady(ctx, "test-unit")
		require.NoError(t, err)
	})
}

func TestSocketClient_SyncStart(t *testing.T) {
	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Set environment variable for socket discovery
	t.Setenv("CODER_AGENT_SOCKET_PATH", socketPath)

	// Start a real socket server with dependency tracker
	server := startSocketServerWithDependencyTracker(t, socketPath)
	t.Cleanup(func() { server.Stop() })

	// Create client
	client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()

	t.Run("SyncStart_UnitWithUnsatisfiedDependencies", func(t *testing.T) {
		// Register a unit with dependencies
		err := client.SyncStart(ctx, "test-unit-start")
		require.NoError(t, err)

		// Add a dependency that's not satisfied
		err = client.SyncWant(ctx, "test-unit-start", "dependency-unit-start")
		require.NoError(t, err)

		// Sync start should succeed (it registers the unit)
		err = client.SyncStart(ctx, "test-unit-start")
		require.NoError(t, err)
	})

	t.Run("SyncStart_UnitWithSatisfiedDependencies", func(t *testing.T) {
		// Register dependency unit and mark it as complete
		err := client.SyncStart(ctx, "dependency-unit-start")
		require.NoError(t, err)
		err = client.SyncComplete(ctx, "dependency-unit-start")
		require.NoError(t, err)

		// Now sync start should succeed
		err = client.SyncStart(ctx, "test-unit-start")
		require.NoError(t, err)
	})

	t.Run("SyncStart_InfiniteLoopScenario", func(t *testing.T) {
		// This test simulates the scenario where sync start gets stuck in a loop
		// First, register a unit with dependencies
		err := client.SyncStart(ctx, "infinite-test-unit")
		require.NoError(t, err)

		// Add a dependency
		err = client.SyncWant(ctx, "infinite-test-unit", "infinite-dependency-unit")
		require.NoError(t, err)

		// Register and complete the dependency
		err = client.SyncStart(ctx, "infinite-dependency-unit")
		require.NoError(t, err)
		err = client.SyncComplete(ctx, "infinite-dependency-unit")
		require.NoError(t, err)

		// Now sync start should succeed without infinite loop
		err = client.SyncStart(ctx, "infinite-test-unit")
		require.NoError(t, err)

		// Verify the unit is marked as started
		status, err := client.SyncStatus(ctx, "infinite-test-unit", false)
		require.NoError(t, err)
		assert.Equal(t, "started", status.Status)
	})
}

func TestSocketClient_Discovery(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	// Test with non-existent socket
	client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{Path: "/nonexistent/socket"})
	assert.Error(t, err)
	assert.Nil(t, client)
}

// startSocketServer starts a real socket server for testing
func startSocketServer(t *testing.T, path string) *agentsocket.Server {
	t.Helper()

	// Create server
	server, err := agentsocket.NewServer(path, slog.Make().Leveled(slog.LevelDebug))
	require.NoError(t, err)

	// Start server
	err = server.Start()
	require.NoError(t, err)

	return server
}

// startSocketServerWithDependencyTracker starts a socket server with sync handlers for testing
func startSocketServerWithDependencyTracker(t *testing.T, path string) *agentsocket.Server {
	t.Helper()

	// Create server
	server, err := agentsocket.NewServer(path, slog.Make().Leveled(slog.LevelDebug))
	require.NoError(t, err)

	// Start server
	err = server.Start()
	require.NoError(t, err)

	return server
}
