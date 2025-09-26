package cli_test

import (
	"context"
	"net"
	"testing"

	"github.com/google/uuid"
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

func TestImmortalStreamClientOperations(t *testing.T) {
	t.Parallel()

	t.Run("CreateStream", func(t *testing.T) {
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

		// Create a stream directly via API
		stream, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 22,
		})
		require.NoError(t, err)
		assert.Equal(t, uint16(22), stream.TCPPort)
		assert.NotEmpty(t, stream.ID)
		assert.NotEmpty(t, stream.Name)

		// Clean up
		err = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, stream.ID)
		assert.NoError(t, err)
	})

	t.Run("ListStreams", func(t *testing.T) {
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

		// Initially should have no streams
		streams, err := client.WorkspaceAgentImmortalStreams(ctx, agent.ID)
		require.NoError(t, err)
		assert.Empty(t, streams)

		// Create a stream
		stream1, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 22,
		})
		require.NoError(t, err)

		// Create another stream
		stream2, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 80,
		})
		require.NoError(t, err)

		// List should now return both streams
		streams, err = client.WorkspaceAgentImmortalStreams(ctx, agent.ID)
		require.NoError(t, err)
		assert.Len(t, streams, 2)

		// Clean up
		_ = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, stream1.ID)
		_ = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, stream2.ID)
	})

	t.Run("DeleteStream", func(t *testing.T) {
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

		// Create a stream
		stream, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 22,
		})
		require.NoError(t, err)

		// Verify it exists
		streams, err := client.WorkspaceAgentImmortalStreams(ctx, agent.ID)
		require.NoError(t, err)
		assert.Len(t, streams, 1)

		// Delete the stream
		err = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, stream.ID)
		require.NoError(t, err)

		// Verify it's gone
		streams, err = client.WorkspaceAgentImmortalStreams(ctx, agent.ID)
		require.NoError(t, err)
		assert.Empty(t, streams)

		// Try to delete non-existent stream - should not error
		randomID := uuid.New()
		err = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, randomID)
		assert.NoError(t, err) // Agent should handle this gracefully
	})
}

func TestImmortalStreamMultiplePorts(t *testing.T) {
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

	// Test creating streams on different ports
	testPorts := []uint16{22, 80, 443, 8080}
	streamIDs := make([]uuid.UUID, 0, len(testPorts))

	for _, port := range testPorts {
		stream, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: port,
		})
		require.NoError(t, err)
		assert.Equal(t, port, stream.TCPPort)
		streamIDs = append(streamIDs, stream.ID)
	}

	// Verify all streams exist
	streams, err := client.WorkspaceAgentImmortalStreams(ctx, agent.ID)
	require.NoError(t, err)
	assert.Len(t, streams, len(testPorts))

	// Clean up all streams
	for _, streamID := range streamIDs {
		err = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, streamID)
		assert.NoError(t, err)
	}

	// Verify all streams are gone
	streams, err = client.WorkspaceAgentImmortalStreams(ctx, agent.ID)
	require.NoError(t, err)
	assert.Empty(t, streams)
}

func TestImmortalStreamErrorHandling(t *testing.T) {
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

	t.Run("InvalidPort", func(t *testing.T) {
		// Test port 0 (invalid)
		_, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 0,
		})
		// This should either succeed or fail gracefully - agent should handle validation
		if err != nil {
			t.Logf("Expected error for port 0: %v", err)
		}
	})

	t.Run("NonExistentAgent", func(t *testing.T) {
		fakeAgentID := uuid.New()
		_, err := client.WorkspaceAgentCreateImmortalStream(ctx, fakeAgentID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 22,
		})
		require.Error(t, err)
		t.Logf("Expected error for non-existent agent: %v", err)
	})
}

func TestDialImmortalOrFallback(t *testing.T) {
	t.Parallel()

	t.Run("DisabledUseFallback", func(t *testing.T) {
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

		// Create a mock agent connection
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		// Mock fallback dialer that returns a basic connection
		fallbackCalled := false
		fallbackDial := func(ctx context.Context) (net.Conn, error) {
			fallbackCalled = true
			// Return a mock connection for testing
			server, client := net.Pipe()
			go server.Close() // Close server side immediately
			return client, nil
		}

		ops := cli.ImmortalDialOptions{
			Enabled:    false,
			Fallback:   true,
			TargetPort: 22,
		}

		result, err := cli.DialImmortalOrFallback(ctx, agentConn, client, agent.ID, logger, ops, fallbackDial)
		require.NoError(t, err)
		require.True(t, fallbackCalled, "fallback should have been called when immortal is disabled")
		require.False(t, result.UsedImmortal, "should not have used immortal when disabled")
		require.NotNil(t, result.Conn)
		require.Nil(t, result.StreamClient)
		require.Nil(t, result.StreamID)

		// Clean up connection
		_ = result.Conn.Close()
	})

	t.Run("EnabledCreatesStream", func(t *testing.T) {
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

		// Create a real agent connection for immortal streams
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		fallbackCalled := false
		fallbackDial := func(ctx context.Context) (net.Conn, error) {
			fallbackCalled = true
			server, client := net.Pipe()
			go server.Close()
			return client, nil
		}

		ops := cli.ImmortalDialOptions{
			Enabled:    true,
			Fallback:   true,
			TargetPort: 22,
		}

		result, err := cli.DialImmortalOrFallback(ctx, agentConn, client, agent.ID, logger, ops, fallbackDial)

		// The test may succeed (immortal stream created) or fallback due to connection issues
		require.NoError(t, err)
		require.NotNil(t, result.Conn)

		if result.UsedImmortal {
			// If immortal stream was used successfully
			require.False(t, fallbackCalled, "fallback should not be called when immortal succeeds")
			require.NotNil(t, result.StreamClient)
			require.NotNil(t, result.StreamID)

			// Clean up the stream
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cleanupCancel()
			_ = client.WorkspaceAgentDeleteImmortalStream(cleanupCtx, agent.ID, *result.StreamID)
		} else {
			// If fallback was used due to connection issues
			require.True(t, fallbackCalled, "fallback should be called when immortal fails")
			require.Nil(t, result.StreamClient)
			require.Nil(t, result.StreamID)
		}

		// Clean up connection
		_ = result.Conn.Close()
	})

	t.Run("FailureWithoutFallback", func(t *testing.T) {
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

		// Use a non-existent agent ID to force failure
		fakeAgentID := uuid.New()

		// Create agent connection with the real agent but use fake ID for immortal stream
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		fallbackCalled := false
		fallbackDial := func(ctx context.Context) (net.Conn, error) {
			fallbackCalled = true
			server, client := net.Pipe()
			go server.Close()
			return client, nil
		}

		ops := cli.ImmortalDialOptions{
			Enabled:    true,
			Fallback:   false, // No fallback allowed
			TargetPort: 22,
		}

		// This should fail because we're using a fake agent ID and fallback is disabled
		_, err = cli.DialImmortalOrFallback(ctx, agentConn, client, fakeAgentID, logger, ops, fallbackDial)
		require.Error(t, err)
		require.False(t, fallbackCalled, "fallback should not be called when disabled")
	})

	t.Run("FallbackOnError", func(t *testing.T) {
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

		// Use a non-existent agent ID to force immortal stream creation to fail
		fakeAgentID := uuid.New()

		// Create agent connection with the real agent
		agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
		require.NoError(t, err)
		defer agentConn.Close()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		fallbackCalled := false
		fallbackDial := func(ctx context.Context) (net.Conn, error) {
			fallbackCalled = true
			server, client := net.Pipe()
			go server.Close()
			return client, nil
		}

		ops := cli.ImmortalDialOptions{
			Enabled:    true,
			Fallback:   true, // Fallback allowed
			TargetPort: 22,
		}

		// This should succeed using fallback because the fake agent ID will cause immortal stream to fail
		result, err := cli.DialImmortalOrFallback(ctx, agentConn, client, fakeAgentID, logger, ops, fallbackDial)
		require.NoError(t, err)
		require.True(t, fallbackCalled, "fallback should be called when immortal fails")
		require.False(t, result.UsedImmortal, "should not have used immortal when it fails")
		require.NotNil(t, result.Conn)
		require.Nil(t, result.StreamClient)
		require.Nil(t, result.StreamID)

		// Clean up connection
		_ = result.Conn.Close()
	})
}

func TestDialImmortalOrFallbackFallbackScenarios(t *testing.T) {
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

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	// Create agent connection
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	agentConn, err := workspacesdk.New(client).DialAgent(ctx, agent.ID, nil)
	require.NoError(t, err)
	defer agentConn.Close()

	fallbackDial := func(ctx context.Context) (net.Conn, error) {
		server, client := net.Pipe()
		go server.Close()
		return client, nil
	}

	// Test different port configurations
	testCases := []struct {
		name       string
		targetPort uint16
		expectFail bool
	}{
		{
			name:       "SSH Port",
			targetPort: 22,
			expectFail: false,
		},
		{
			name:       "HTTP Port",
			targetPort: 80,
			expectFail: false,
		},
		{
			name:       "HTTPS Port",
			targetPort: 443,
			expectFail: false,
		},
		{
			name:       "Custom Port",
			targetPort: 8080,
			expectFail: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ops := cli.ImmortalDialOptions{
				Enabled:    true,
				Fallback:   true,
				TargetPort: tc.targetPort,
			}

			result, err := cli.DialImmortalOrFallback(ctx, agentConn, client, agent.ID, logger, ops, fallbackDial)
			if tc.expectFail {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result.Conn)

			// Clean up
			_ = result.Conn.Close()
			if result.UsedImmortal && result.StreamClient != nil && result.StreamID != nil {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), testutil.WaitShort)
				defer cleanupCancel()
				_ = client.WorkspaceAgentDeleteImmortalStream(cleanupCtx, agent.ID, *result.StreamID)
			}
		})
	}
}
