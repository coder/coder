//go:build !windows

package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/testutil"
)

// setupSocketServer creates an agentsocket server at a temporary path for testing.
// Returns the socket path and a cleanup function. The path should be passed to
// sync commands via the --socket-path flag.
func setupSocketServer(t *testing.T) (path string, cleanup func()) {
	t.Helper()

	// Use a temporary socket path for each test
	socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

	// Create parent directory if needed
	parentDir := filepath.Dir(socketPath)
	err := os.MkdirAll(parentDir, 0o700)
	require.NoError(t, err, "create socket directory")

	server, err := agentsocket.NewServer(
		slog.Make().Leveled(slog.LevelDebug),
		agentsocket.WithPath(socketPath),
	)
	require.NoError(t, err, "create socket server")

	// Return cleanup function
	return socketPath, func() {
		err := server.Close()
		require.NoError(t, err, "close socket server")
		_ = os.Remove(socketPath)
	}
}

func TestSyncCommands_Golden(t *testing.T) {
	t.Parallel()

	t.Run("ping", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "ping", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/ping_success", outBuf.Bytes(), nil)
	})

	t.Run("start_no_dependencies", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "start", "test-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/start_no_dependencies", outBuf.Bytes(), nil)
	})

	t.Run("start_with_dependencies", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Set up dependency: test-unit depends on dep-unit
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)

		// Declare dependency
		err = client.SyncWant(ctx, "test-unit", "dep-unit")
		require.NoError(t, err)
		client.Close()

		// Start a goroutine to complete the dependency after a short delay
		// This simulates the dependency being satisfied while start is waiting
		// The delay ensures the "Waiting..." message appears in the output
		done := make(chan error, 1)
		go func() {
			// Wait a moment to let the start command begin waiting and print the message
			time.Sleep(100 * time.Millisecond)

			compCtx := context.Background()
			compClient, err := agentsocket.NewClient(compCtx, agentsocket.WithPath(path))
			if err != nil {
				done <- err
				return
			}
			defer compClient.Close()

			// Start and complete the dependency unit
			err = compClient.SyncStart(compCtx, "dep-unit")
			if err != nil {
				done <- err
				return
			}
			err = compClient.SyncComplete(compCtx, "dep-unit")
			done <- err
		}()

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "start", "test-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		// Run the start command - it should wait for the dependency
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Ensure the completion goroutine finished
		select {
		case err := <-done:
			require.NoError(t, err, "complete dependency")
		case <-time.After(time.Second):
			// Goroutine should have finished by now
		}

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/start_with_dependencies", outBuf.Bytes(), nil)
	})

	t.Run("want", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "want", "test-unit", "dep-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/want_success", outBuf.Bytes(), nil)
	})

	t.Run("complete", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// First start the unit
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)
		err = client.SyncStart(ctx, "test-unit")
		require.NoError(t, err)
		client.Close()

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "complete", "test-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/complete_success", outBuf.Bytes(), nil)
	})

	t.Run("status_pending", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Set up a unit with unsatisfied dependency
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)
		err = client.SyncWant(ctx, "test-unit", "dep-unit")
		require.NoError(t, err)
		client.Close()

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "status", "test-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/status_pending", outBuf.Bytes(), nil)
	})

	t.Run("status_started", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Start a unit
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)
		err = client.SyncStart(ctx, "test-unit")
		require.NoError(t, err)
		client.Close()

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "status", "test-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/status_started", outBuf.Bytes(), nil)
	})

	t.Run("status_completed", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Start and complete a unit
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)
		err = client.SyncStart(ctx, "test-unit")
		require.NoError(t, err)
		err = client.SyncComplete(ctx, "test-unit")
		require.NoError(t, err)
		client.Close()

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "status", "test-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/status_completed", outBuf.Bytes(), nil)
	})

	t.Run("status_with_dependencies", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Set up a unit with dependencies, some satisfied, some not
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)
		err = client.SyncWant(ctx, "test-unit", "dep-1")
		require.NoError(t, err)
		err = client.SyncWant(ctx, "test-unit", "dep-2")
		require.NoError(t, err)
		// Complete dep-1, leave dep-2 incomplete
		err = client.SyncStart(ctx, "dep-1")
		require.NoError(t, err)
		err = client.SyncComplete(ctx, "dep-1")
		require.NoError(t, err)
		client.Close()

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "status", "test-unit", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/status_with_dependencies", outBuf.Bytes(), nil)
	})

	t.Run("status_json_format", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Set up a unit with dependencies
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)
		err = client.SyncWant(ctx, "test-unit", "dep-unit")
		require.NoError(t, err)
		err = client.SyncStart(ctx, "dep-unit")
		require.NoError(t, err)
		err = client.SyncComplete(ctx, "dep-unit")
		require.NoError(t, err)
		client.Close()

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "status", "test-unit", "--output", "json", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/status_json_format", outBuf.Bytes(), nil)
	})
}
