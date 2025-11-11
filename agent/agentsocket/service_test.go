package agentsocket_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestDRPCAgentSocketService(t *testing.T) {
	t.Parallel()

	t.Run("Ping", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(t.TempDir(), "test.sock")

		server, err := agentsocket.NewServer(
			socketPath,
			slog.Make().Leveled(slog.LevelDebug),
		)
		require.NoError(t, err)

		err = server.Start()
		require.NoError(t, err)
		defer server.Stop()

		client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
			Path: socketPath,
		})
		require.NoError(t, err)
		defer client.Close()

		response, err := client.Ping(context.Background())
		require.NoError(t, err)
		require.Equal(t, "pong", response.Message)
	})

	t.Run("SyncStart", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnit", func(t *testing.T) {
			t.Parallel()
			socketPath := filepath.Join(t.TempDir(), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			err = server.Start()
			require.NoError(t, err)
			defer server.Stop()

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: socketPath,
			})
			require.NoError(t, err)
			defer client.Close()

			err = client.SyncStart(context.Background(), "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)
		})

		t.Run("UnitAlreadyStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(t.TempDir(), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			err = server.Start()
			require.NoError(t, err)
			defer server.Stop()

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: socketPath,
			})
			require.NoError(t, err)
			defer client.Close()

			err = client.SyncStart(context.Background(), "test-unit")
			require.NoError(t, err)

			// First Start
			status, err := client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			status, err = client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Second Start
			err = client.SyncStart(context.Background(), "test-unit")
			require.ErrorContains(t, err, unit.ErrSameStatusAlreadySet.Error())

			status, err = client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)
		})

		t.Run("UnitAlreadyCompleted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(t.TempDir(), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			err = server.Start()
			require.NoError(t, err)
			defer server.Stop()

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: socketPath,
			})
			require.NoError(t, err)
			defer client.Close()

			// First start
			err = client.SyncStart(context.Background(), "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Complete the unit
			err = client.SyncComplete(context.Background(), "test-unit")
			require.NoError(t, err)

			status, err = client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "completed", status.Status)

			// Second start
			err = client.SyncStart(context.Background(), "test-unit")
			require.NoError(t, err)

			status, err = client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)
		})

		t.Run("UnitNotReady", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(t.TempDir(), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			err = server.Start()
			require.NoError(t, err)
			defer server.Stop()

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: socketPath,
			})
			require.NoError(t, err)
			defer client.Close()

			client.SyncWant(context.Background(), "test-unit", "dependency-unit")
			require.NoError(t, err)

			err = client.SyncStart(context.Background(), "test-unit")
			require.ErrorContains(t, err, "Unit is not ready")

			status, err := client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "", status.Status)
		})
	})

	t.Run("SyncWant", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnits", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(t.TempDir(), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			err = server.Start()
			require.NoError(t, err)
			defer server.Stop()

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: socketPath,
			})
			require.NoError(t, err)
			defer client.Close()

			// If units are not registered, they are registered automatically
			err = client.SyncWant(context.Background(), "test-unit", "dependency-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "dependency-unit", status.Dependencies[0].DependsOn)
			require.Equal(t, "completed", status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAlreadyRegistered", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(t.TempDir(), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			err = server.Start()
			require.NoError(t, err)
			defer server.Stop()

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: socketPath,
			})
			require.NoError(t, err)
			defer client.Close()

			// Start the dependency unit
			err = client.SyncStart(context.Background(), "dependency-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), "dependency-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Add the dependency after the dependency unit has already started
			err = client.SyncWant(context.Background(), "test-unit", "dependency-unit")

			// Dependencies can be added even if the dependency unit has already started
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "dependency-unit", status.Dependencies[0].DependsOn)
			require.Equal(t, "completed", status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAddedAfterDependentStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(t.TempDir(), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			err = server.Start()
			require.NoError(t, err)
			defer server.Stop()

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: socketPath,
			})
			require.NoError(t, err)
			defer client.Close()

			// Start the dependent unit
			err = client.SyncStart(context.Background(), "test-unit")
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Add the dependency after the dependency unit has already started
			err = client.SyncWant(context.Background(), "test-unit", "dependency-unit")

			// Dependencies can be added even if the dependent unit has already started.
			// The dependency applies the next time a unit is started. The current status is not updated.
			// This is to allow flexible dependency management. It does mean that users of this API should
			// take care to add dependencies before they start their dependent units.
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(context.Background(), "test-unit", false)
			require.NoError(t, err)
			require.Equal(t, "dependency-unit", status.Dependencies[0].DependsOn)
			require.Equal(t, "completed", status.Dependencies[0].RequiredStatus)
		})
	})
}
