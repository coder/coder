package agentsocket_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/testutil"
)

// newSocketClient creates a DRPC client connected to the Unix socket at the given path.
func newSocketClient(ctx context.Context, t *testing.T, socketPath string) *agentsocket.Client {
	t.Helper()

	client, err := agentsocket.NewClient(ctx, agentsocket.WithPath(socketPath))
	t.Cleanup(func() {
		_ = client.Close()
	})
	require.NoError(t, err)

	return client
}

func TestDRPCAgentSocketService(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("agentsocket is not supported on Windows")
	}

	t.Run("Ping", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
		ctx := testutil.Context(t, testutil.WaitShort)
		server, err := agentsocket.NewServer(
			slog.Make().Leveled(slog.LevelDebug),
			agentsocket.WithPath(socketPath),
		)
		require.NoError(t, err)
		defer server.Close()

		client := newSocketClient(ctx, t, socketPath)

		err = client.Ping(ctx)
		require.NoError(t, err)
	})

	t.Run("SyncStart", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnit", func(t *testing.T) {
			t.Parallel()
			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)
		})

		t.Run("UnitAlreadyStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// First Start
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)
			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Second Start
			err = client.SyncStart(ctx, "test-unit")
			require.ErrorContains(t, err, unit.ErrSameStatusAlreadySet.Error())

			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)
		})

		t.Run("UnitAlreadyCompleted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// First start
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Complete the unit
			err = client.SyncComplete(ctx, "test-unit")
			require.NoError(t, err)

			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusComplete, status.Status)

			// Second start
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)
		})

		t.Run("UnitNotReady", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			err = client.SyncWant(ctx, "test-unit", "dependency-unit")
			require.NoError(t, err)

			err = client.SyncStart(ctx, "test-unit")
			require.ErrorContains(t, err, "unit not ready")

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusPending, status.Status)
			require.False(t, status.IsReady)
		})
	})

	t.Run("SyncWant", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnits", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// If dependency units are not registered, they are registered automatically
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Len(t, status.Dependencies, 1)
			require.Equal(t, unit.ID("dependency-unit"), status.Dependencies[0].DependsOn)
			require.Equal(t, unit.StatusComplete, status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAlreadyRegistered", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Start the dependency unit
			err = client.SyncStart(ctx, "dependency-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "dependency-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Add the dependency after the dependency unit has already started
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")

			// Dependencies can be added even if the dependency unit has already started
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.ID("dependency-unit"), status.Dependencies[0].DependsOn)
			require.Equal(t, unit.StatusComplete, status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAddedAfterDependentStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Start the dependent unit
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.StatusStarted, status.Status)

			// Add the dependency after the dependency unit has already started
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")

			// Dependencies can be added even if the dependent unit has already started.
			// The dependency applies the next time a unit is started. The current status is not updated.
			// This is to allow flexible dependency management. It does mean that users of this API should
			// take care to add dependencies before they start their dependent units.
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(ctx, "test-unit")
			require.NoError(t, err)
			require.Equal(t, unit.ID("dependency-unit"), status.Dependencies[0].DependsOn)
			require.Equal(t, unit.StatusComplete, status.Dependencies[0].RequiredStatus)
		})
	})

	t.Run("SyncReady", func(t *testing.T) {
		t.Parallel()

		t.Run("UnregisteredUnit", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			ready, err := client.SyncReady(ctx, "unregistered-unit")
			require.NoError(t, err)
			require.True(t, ready)
		})

		t.Run("UnitNotReady", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Register a unit with an unsatisfied dependency
			err = client.SyncWant(ctx, "test-unit", "dependency-unit")
			require.NoError(t, err)

			// Check readiness - should be false because dependency is not satisfied
			ready, err := client.SyncReady(ctx, "test-unit")
			require.NoError(t, err)
			require.False(t, ready)
		})

		t.Run("UnitReady", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(testutil.TempDirUnixSocket(t), "test.sock")
			ctx := testutil.Context(t, testutil.WaitShort)
			server, err := agentsocket.NewServer(
				slog.Make().Leveled(slog.LevelDebug),
				agentsocket.WithPath(socketPath),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(ctx, t, socketPath)

			// Register a unit with no dependencies - should be ready immediately
			err = client.SyncStart(ctx, "test-unit")
			require.NoError(t, err)

			// Check readiness - should be true
			ready, err := client.SyncReady(ctx, "test-unit")
			require.NoError(t, err)
			require.True(t, ready)

			// Also test a unit with satisfied dependencies
			err = client.SyncWant(ctx, "dependent-unit", "test-unit")
			require.NoError(t, err)

			// Complete the dependency
			err = client.SyncComplete(ctx, "test-unit")
			require.NoError(t, err)

			// Now dependent-unit should be ready
			ready, err = client.SyncReady(ctx, "dependent-unit")
			require.NoError(t, err)
			require.True(t, ready)
		})
	})
}
