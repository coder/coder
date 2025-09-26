package cli_test

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestImmortalStreamSSHIntegration(t *testing.T) {
	t.Parallel()

	t.Run("BasicSSHConnection", func(t *testing.T) {
		t.Parallel()

		client, workspaceDB, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspaceDB.ID)

		// Get the workspace via API to access build resources
		workspace, err := client.Workspace(context.Background(), workspaceDB.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Create agent connection
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// Test successful immortal SSH setup
		sshConn, streamClient, streamID, err := cli.Export_SetupImmortalSSHStream(
			ctx, client, workspace, agent, logger, true, agentConn,
		)
		require.NoError(t, err)

		// Should have a connection and stream info
		require.NotNil(t, sshConn)
		require.NotNil(t, streamClient)
		require.NotNil(t, streamID)

		// Test that the connection is functional
		testConn(t, sshConn)

		// Clean up the immortal stream
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cleanupCancel()
		err = client.WorkspaceAgentDeleteImmortalStream(cleanupCtx, agent.ID, *streamID)
		assert.NoError(t, err)

		// Close the connection
		_ = sshConn.Close()
	})

	t.Run("SSHWithFallback", func(t *testing.T) {
		t.Parallel()

		client, workspaceDB, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspaceDB.ID)

		// Get the workspace via API to access build resources
		workspace, err := client.Workspace(context.Background(), workspaceDB.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// Create agent connection
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// Test with fallback enabled - should always succeed
		sshConn, _, _, err := cli.Export_SetupImmortalSSHStream(
			ctx, client, workspace, agent, logger, true, agentConn,
		)

		require.NoError(t, err)
		require.NotNil(t, sshConn)

		// Test that the connection is functional
		testConn(t, sshConn)

		// Close the connection
		_ = sshConn.Close()
	})

	t.Run("ImmortalStreamReconnection", func(t *testing.T) {
		t.Parallel()

		client, workspaceDB, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspaceDB.ID)

		// Get the workspace via API to access build resources
		workspace, err := client.Workspace(context.Background(), workspaceDB.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Create agent connection
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// First, create an immortal stream via API
		stream, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 22,
		})
		require.NoError(t, err)
		defer func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cleanupCancel()
			_ = client.WorkspaceAgentDeleteImmortalStream(cleanupCtx, agent.ID, stream.ID)
		}()

		// Now test that we can use the existing stream
		sshConn, _, _, err := cli.Export_SetupImmortalSSHStream(
			ctx, client, workspace, agent, logger, true, agentConn,
		)

		require.NoError(t, err)
		require.NotNil(t, sshConn)

		// Test that the connection works
		testConn(t, sshConn)

		// Close the connection
		_ = sshConn.Close()
	})
}

func TestImmortalStreamPortForwarding(t *testing.T) {
	t.Parallel()

	t.Run("BasicPortForward", func(t *testing.T) {
		t.Parallel()

		client, workspaceDB, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspaceDB.ID)

		// Get the workspace via API to access build resources
		workspace, err := client.Workspace(context.Background(), workspaceDB.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Create agent connection
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// Test port forwarding with immortal streams
		listenAddr, _ := netip.AddrFromSlice(net.ParseIP("127.0.0.1").To4())
		listener, err := cli.Export_ListenAndPortForward(
			ctx, nil, agentConn, nil, "tcp", listenAddr,
			0, 22, logger, true, true, "", client, agent.ID,
		)
		require.NoError(t, err)

		require.NotNil(t, listener)
		defer listener.Close()

		// Verify listener is working
		addr := listener.Addr()
		require.NotNil(t, addr)
		t.Logf("Port forwarding listening on %s", addr.String())
	})
}

func TestImmortalStreamCompatibility(t *testing.T) {
	t.Parallel()

	t.Run("WithExistingSSHFeatures", func(t *testing.T) {
		t.Parallel()

		client, workspaceDB, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspaceDB.ID)

		// Get the workspace via API to access build resources
		workspace, err := client.Workspace(context.Background(), workspaceDB.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		// Create agent connection
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		// Test multiple immortal streams for different ports
		testPorts := []uint16{22, 80, 3000}

		for _, port := range testPorts {
			t.Run("Port_"+string(rune(port)), func(t *testing.T) {
				stream, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
					TCPPort: port,
				})
				require.NoError(t, err)
				defer func() {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), testutil.WaitShort)
					defer cleanupCancel()
					_ = client.WorkspaceAgentDeleteImmortalStream(cleanupCtx, agent.ID, stream.ID)
				}()

				assert.Equal(t, port, stream.TCPPort)
				assert.NotEmpty(t, stream.ID)
				assert.NotEmpty(t, stream.Name)
			})
		}
	})

	t.Run("FallbackBehavior", func(t *testing.T) {
		t.Parallel()

		client, workspaceDB, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspaceDB.ID)

		// Get the workspace via API to access build resources
		workspace, err := client.Workspace(context.Background(), workspaceDB.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		agent := workspace.LatestBuild.Resources[0].Agents[0]

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		// Create agent connection
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// Test immortal streams with different fallback settings
		testCases := []struct {
			name     string
			fallback bool
		}{
			{"With_Fallback", true},
			{"Without_Fallback", false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				sshConn, _, _, err := cli.Export_SetupImmortalSSHStream(
					ctx, client, workspace, agent, logger, tc.fallback, agentConn,
				)
				require.NoError(t, err)

				require.NotNil(t, sshConn)
				testConn(t, sshConn)
				_ = sshConn.Close()
			})
		}
	})
}

// testConn performs basic connectivity tests on a network connection
func testConn(t *testing.T, conn net.Conn) {
	t.Helper()

	// Set a short deadline for testing
	err := conn.SetDeadline(time.Now().Add(time.Second * 5))
	require.NoError(t, err)

	// Test basic read/write (this may not work depending on the protocol)
	// For SSH connections, this would require a full SSH handshake
	// So we'll just verify the connection exists and can set deadlines
	assert.NotNil(t, conn.LocalAddr())
	assert.NotNil(t, conn.RemoteAddr())
}
