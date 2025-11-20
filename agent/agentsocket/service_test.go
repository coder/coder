package agentsocket_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
)

// tempDirUnixSocket returns a temporary directory that can safely hold unix
// sockets (probably).
//
// During tests on darwin we hit the max path length limit for unix sockets
// pretty easily in the default location, so this function uses /tmp instead to
// get shorter paths. To keep paths short, we use a hash of the test name
// instead of the full test name.
func tempDirUnixSocket(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		// Use a short hash of the test name to keep the path under 104 chars
		hash := sha256.Sum256([]byte(t.Name()))
		hashStr := hex.EncodeToString(hash[:])[:8] // Use first 8 chars of hash
		dir, err := os.MkdirTemp("/tmp", fmt.Sprintf("c-%s-", hashStr))
		require.NoError(t, err, "create temp dir for unix socket test")
		t.Cleanup(func() {
			err := os.RemoveAll(dir)
			assert.NoError(t, err, "remove temp dir", dir)
		})
		return dir
	}
	return t.TempDir()
}

// newSocketClient creates a DRPC client connected to the Unix socket at the given path.
func newSocketClient(t *testing.T, socketPath string) proto.DRPCAgentSocketClient {
	t.Helper()

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)

	config := yamux.DefaultConfig()
	config.Logger = nil
	session, err := yamux.Client(conn, config)
	require.NoError(t, err)

	client := proto.NewDRPCAgentSocketClient(drpcsdk.MultiplexedConn(session))

	t.Cleanup(func() {
		_ = session.Close()
		_ = conn.Close()
	})
	return client
}

func TestDRPCAgentSocketService(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("agentsocket is not supported on Windows")
	}

	t.Run("Ping", func(t *testing.T) {
		t.Parallel()

		socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

		server, err := agentsocket.NewServer(
			socketPath,
			slog.Make().Leveled(slog.LevelDebug),
		)
		require.NoError(t, err)
		defer server.Close()

		client := newSocketClient(t, socketPath)

		_, err = client.Ping(context.Background(), &proto.PingRequest{})
		require.NoError(t, err)
	})

	t.Run("SyncStart", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnit", func(t *testing.T) {
			t.Parallel()
			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)
		})

		t.Run("UnitAlreadyStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			// First Start
			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			status, err := client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Second Start
			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.ErrorContains(t, err, unit.ErrSameStatusAlreadySet.Error())

			status, err = client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)
		})

		t.Run("UnitAlreadyCompleted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			// First start
			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Complete the unit
			_, err = client.SyncComplete(context.Background(), &proto.SyncCompleteRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			status, err = client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "completed", status.Status)

			// Second start
			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			status, err = client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)
		})

		t.Run("UnitNotReady", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			_, err = client.SyncWant(context.Background(), &proto.SyncWantRequest{
				Unit:      "test-unit",
				DependsOn: "dependency-unit",
			})
			require.NoError(t, err)

			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.ErrorContains(t, err, "unit not ready")

			status, err := client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, string(unit.StatusPending), status.Status)
			require.False(t, status.IsReady)
		})
	})

	t.Run("SyncWant", func(t *testing.T) {
		t.Parallel()

		t.Run("NewUnits", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			// If dependency units are not registered, they are registered automatically
			_, err = client.SyncWant(context.Background(), &proto.SyncWantRequest{
				Unit:      "test-unit",
				DependsOn: "dependency-unit",
			})
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Len(t, status.Dependencies, 1)
			require.Equal(t, "dependency-unit", status.Dependencies[0].DependsOn)
			require.Equal(t, "completed", status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAlreadyRegistered", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			// Start the dependency unit
			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "dependency-unit",
			})
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "dependency-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Add the dependency after the dependency unit has already started
			_, err = client.SyncWant(context.Background(), &proto.SyncWantRequest{
				Unit:      "test-unit",
				DependsOn: "dependency-unit",
			})

			// Dependencies can be added even if the dependency unit has already started
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "dependency-unit", status.Dependencies[0].DependsOn)
			require.Equal(t, "completed", status.Dependencies[0].RequiredStatus)
		})

		t.Run("DependencyAddedAfterDependentStarted", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			// Start the dependent unit
			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			status, err := client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "started", status.Status)

			// Add the dependency after the dependency unit has already started
			_, err = client.SyncWant(context.Background(), &proto.SyncWantRequest{
				Unit:      "test-unit",
				DependsOn: "dependency-unit",
			})

			// Dependencies can be added even if the dependent unit has already started.
			// The dependency applies the next time a unit is started. The current status is not updated.
			// This is to allow flexible dependency management. It does mean that users of this API should
			// take care to add dependencies before they start their dependent units.
			require.NoError(t, err)

			// The dependency is now reflected in the test unit's status
			status, err = client.SyncStatus(context.Background(), &proto.SyncStatusRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.Equal(t, "dependency-unit", status.Dependencies[0].DependsOn)
			require.Equal(t, "completed", status.Dependencies[0].RequiredStatus)
		})
	})

	t.Run("SyncReady", func(t *testing.T) {
		t.Parallel()

		t.Run("UnregisteredUnit", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			response, err := client.SyncReady(context.Background(), &proto.SyncReadyRequest{
				Unit: "unregistered-unit",
			})
			require.NoError(t, err)
			require.False(t, response.Ready)
		})

		t.Run("UnitNotReady", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			// Register a unit with an unsatisfied dependency
			_, err = client.SyncWant(context.Background(), &proto.SyncWantRequest{
				Unit:      "test-unit",
				DependsOn: "dependency-unit",
			})
			require.NoError(t, err)

			// Check readiness - should be false because dependency is not satisfied
			response, err := client.SyncReady(context.Background(), &proto.SyncReadyRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)
			require.False(t, response.Ready)
		})

		t.Run("UnitReady", func(t *testing.T) {
			t.Parallel()

			socketPath := filepath.Join(tempDirUnixSocket(t), "test.sock")

			server, err := agentsocket.NewServer(
				socketPath,
				slog.Make().Leveled(slog.LevelDebug),
			)
			require.NoError(t, err)
			defer server.Close()

			client := newSocketClient(t, socketPath)

			// Register a unit with no dependencies - should be ready immediately
			_, err = client.SyncStart(context.Background(), &proto.SyncStartRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			// Check readiness - should be true
			_, err = client.SyncReady(context.Background(), &proto.SyncReadyRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			// Also test a unit with satisfied dependencies
			_, err = client.SyncWant(context.Background(), &proto.SyncWantRequest{
				Unit:      "dependent-unit",
				DependsOn: "test-unit",
			})
			require.NoError(t, err)

			// Complete the dependency
			_, err = client.SyncComplete(context.Background(), &proto.SyncCompleteRequest{
				Unit: "test-unit",
			})
			require.NoError(t, err)

			// Now dependent-unit should be ready
			_, err = client.SyncReady(context.Background(), &proto.SyncReadyRequest{
				Unit: "dependent-unit",
			})
			require.NoError(t, err)
		})
	})
}
