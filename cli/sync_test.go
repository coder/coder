package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
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
	socketPath := testutil.AgentSocketPath(t)

	// Create parent directory if needed. Not necessary on Windows because named pipes live in an abstract namespace
	// not tied to any real files.
	if runtime.GOOS != "windows" {
		parentDir := filepath.Dir(socketPath)
		err := os.MkdirAll(parentDir, 0o700)
		require.NoError(t, err, "create socket directory")
	}

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

	// Each case seeds the socket server through a client, runs one sync
	// command against it, and compares the output with a golden file.
	cases := []struct {
		name   string
		seed   func(t *testing.T, ctx context.Context, client *agentsocket.Client)
		args   []string
		golden string
	}{
		{
			name:   "ping",
			args:   []string{"ping"},
			golden: "ping_success",
		},
		{
			name:   "start_no_dependencies",
			args:   []string{"start", "test-unit"},
			golden: "start_no_dependencies",
		},
		{
			// test-unit depends on dep-unit and dep-unit-2, both already complete.
			name: "start_with_satisfied_dependencies",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncWant(ctx, "test-unit", "dep-unit"))
				require.NoError(t, client.SyncWant(ctx, "test-unit", "dep-unit-2"))
				require.NoError(t, client.SyncStart(ctx, "dep-unit"))
				require.NoError(t, client.SyncComplete(ctx, "dep-unit"))
				require.NoError(t, client.SyncStart(ctx, "dep-unit-2"))
				require.NoError(t, client.SyncComplete(ctx, "dep-unit-2"))
			},
			args:   []string{"start", "test-unit"},
			golden: "start_with_satisfied_dependencies",
		},
		{
			name:   "want",
			args:   []string{"want", "test-unit", "dep-unit"},
			golden: "want_success",
		},
		{
			name: "complete",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncStart(ctx, "test-unit"))
			},
			args:   []string{"complete", "test-unit"},
			golden: "complete_success",
		},
		{
			// A unit with an unsatisfied dependency.
			name: "status_pending",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncWant(ctx, "test-unit", "dep-unit"))
			},
			args:   []string{"status", "test-unit"},
			golden: "status_pending",
		},
		{
			name: "status_started",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncStart(ctx, "test-unit"))
			},
			args:   []string{"status", "test-unit"},
			golden: "status_started",
		},
		{
			name: "status_completed",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncStart(ctx, "test-unit"))
				require.NoError(t, client.SyncComplete(ctx, "test-unit"))
			},
			args:   []string{"status", "test-unit"},
			golden: "status_completed",
		},
		{
			// dep-1 is complete, dep-2 is not.
			name: "status_with_dependencies",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncWant(ctx, "test-unit", "dep-1"))
				require.NoError(t, client.SyncWant(ctx, "test-unit", "dep-2"))
				require.NoError(t, client.SyncStart(ctx, "dep-1"))
				require.NoError(t, client.SyncComplete(ctx, "dep-1"))
			},
			args:   []string{"status", "test-unit"},
			golden: "status_with_dependencies",
		},
		{
			name: "status_json_format",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncWant(ctx, "test-unit", "dep-unit"))
				require.NoError(t, client.SyncStart(ctx, "dep-unit"))
				require.NoError(t, client.SyncComplete(ctx, "dep-unit"))
			},
			args:   []string{"status", "test-unit", "--output", "json"},
			golden: "status_json_format",
		},
		{
			name:   "list_no_units",
			args:   []string{"list"},
			golden: "list_no_units",
		},
		{
			// unit-a started, unit-b completed, unit-c pending on unit-a.
			name: "list_with_units",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncStart(ctx, "unit-a"))
				require.NoError(t, client.SyncStart(ctx, "unit-b"))
				require.NoError(t, client.SyncComplete(ctx, "unit-b"))
				require.NoError(t, client.SyncWant(ctx, "unit-c", "unit-a"))
			},
			args:   []string{"list"},
			golden: "list_with_units",
		},
		{
			name: "list_json_format",
			seed: func(t *testing.T, ctx context.Context, client *agentsocket.Client) {
				require.NoError(t, client.SyncStart(ctx, "my-unit"))
			},
			args:   []string{"list", "--output", "json"},
			golden: "list_json_format",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, cleanup := setupSocketServer(t)
			defer cleanup()

			ctx := testutil.Context(t, testutil.WaitShort)

			if tc.seed != nil {
				client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
				require.NoError(t, err)
				tc.seed(t, ctx, client)
				client.Close()
			}

			var outBuf bytes.Buffer
			args := append([]string{"exp", "sync"}, tc.args...)
			inv, _ := clitest.New(t, append(args, "--socket-path", path)...)
			inv.Stdout = &outBuf
			inv.Stderr = &outBuf

			err := inv.WithContext(ctx).Run()
			require.NoError(t, err)

			clitest.TestGoldenFile(t, "TestSyncCommands_Golden/"+tc.golden, outBuf.Bytes(), nil)
		})
	}

	t.Run("start_with_dependencies", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		// Set up dependencies: test-unit depends on dep-unit and dep-unit-2.
		client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(path))
		require.NoError(t, err)

		err = client.SyncWant(ctx, "test-unit", "dep-unit")
		require.NoError(t, err)
		err = client.SyncWant(ctx, "test-unit", "dep-unit-2")
		require.NoError(t, err)
		client.Close()

		outBuf := testutil.NewWaitBuffer()
		done := make(chan error, 1)
		go func() {
			if err := outBuf.WaitFor(ctx, "is waiting for dependencies"); err != nil {
				done <- err
				return
			}

			compCtx := context.Background()
			compClient, err := agentsocket.NewClient(compCtx, agentsocket.WithPath(path))
			if err != nil {
				done <- err
				return
			}
			defer compClient.Close()

			// Start and complete both dependency units.
			err = compClient.SyncStart(compCtx, "dep-unit")
			if err != nil {
				done <- err
				return
			}
			err = compClient.SyncComplete(compCtx, "dep-unit")
			if err != nil {
				done <- err
				return
			}
			err = compClient.SyncStart(compCtx, "dep-unit-2")
			if err != nil {
				done <- err
				return
			}
			err = compClient.SyncComplete(compCtx, "dep-unit-2")
			done <- err
		}()

		inv, _ := clitest.New(t, "exp", "sync", "start", "test-unit", "--socket-path", path)
		inv.Stdout = outBuf
		inv.Stderr = outBuf

		// Run the start command. It should wait for the dependencies.
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Ensure the completion goroutine finished.
		select {
		case err := <-done:
			require.NoError(t, err, "complete dependency")
		case <-ctx.Done():
			t.Fatal("timed out waiting for dependency completion goroutine")
		}

		clitest.TestGoldenFile(t, "TestSyncCommands_Golden/start_with_dependencies", outBuf.Bytes(), nil)
	})

	t.Run("want_multiple_deps", func(t *testing.T) {
		t.Parallel()
		path, cleanup := setupSocketServer(t)
		defer cleanup()

		ctx := testutil.Context(t, testutil.WaitShort)

		var outBuf bytes.Buffer
		inv, _ := clitest.New(t, "exp", "sync", "want", "test-unit", "dep-1", "dep-2", "dep-3", "--socket-path", path)
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, outBuf.String(), "Unit \"test-unit\" declared dependencies: [dep-1, dep-2, dep-3]")
		require.Contains(t, outBuf.String(), "dep-1")
		require.Contains(t, outBuf.String(), "dep-2")
		require.Contains(t, outBuf.String(), "dep-3")

		// Verify all dependencies were registered by checking status.
		outBuf.Reset()
		inv, _ = clitest.New(t, "exp", "sync", "status", "test-unit", "--socket-path", path, "--output", "json")
		inv.Stdout = &outBuf
		inv.Stderr = &outBuf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// The output should mention all three dependencies.
		output := outBuf.String()
		require.Contains(t, output, "dep-1")
		require.Contains(t, output, "dep-2")
		require.Contains(t, output, "dep-3")
	})
}
