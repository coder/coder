package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestImmortalStreamCLICommands(t *testing.T) {
	t.Parallel()

	t.Run("List_NoStreams", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		inv, root := clitest.New(t, "exp", "immortal-stream", "list", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		done := make(chan error)
		go func() {
			done <- inv.WithContext(ctx).Run()
		}()

		pty.ExpectMatch("No active immortal streams found.")

		err := <-done
		assert.NoError(t, err)
	})

	t.Run("List_WithStreams", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Get the workspace via API to access build resources
		workspaceAPI, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspaceAPI.LatestBuild.Resources, 1)
		require.Len(t, workspaceAPI.LatestBuild.Resources[0].Agents, 1)
		agent := workspaceAPI.LatestBuild.Resources[0].Agents[0]

		// Create some streams via API first
		ctx := context.Background()
		stream1, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 22,
		})
		require.NoError(t, err)
		defer func() {
			_ = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, stream1.ID)
		}()

		stream2, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 80,
		})
		require.NoError(t, err)
		defer func() {
			_ = client.WorkspaceAgentDeleteImmortalStream(ctx, agent.ID, stream2.ID)
		}()

		// Now test the CLI list command
		inv, root := clitest.New(t, "exp", "immortal-stream", "list", workspace.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctxCLI, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		done := make(chan error)
		go func() {
			done <- inv.WithContext(ctxCLI).Run()
		}()

		pty.ExpectMatch("Active Immortal Streams:")
		pty.ExpectMatch("NAME")
		pty.ExpectMatch("PORT")
		pty.ExpectMatch("CREATED")
		pty.ExpectMatch("LAST CONNECTED")

		err = <-done
		assert.NoError(t, err)
	})

	t.Run("Delete_ExistingStream", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Get the workspace via API to access build resources
		workspaceAPI, err := client.Workspace(context.Background(), workspace.ID)
		require.NoError(t, err)

		// Get the workspace agent from the build resources
		require.Len(t, workspaceAPI.LatestBuild.Resources, 1)
		require.Len(t, workspaceAPI.LatestBuild.Resources[0].Agents, 1)
		agent := workspaceAPI.LatestBuild.Resources[0].Agents[0]

		// Create a stream via API first
		ctx := context.Background()
		stream, err := client.WorkspaceAgentCreateImmortalStream(ctx, agent.ID, codersdk.CreateImmortalStreamRequest{
			TCPPort: 22,
		})
		require.NoError(t, err)

		// Test the CLI delete command
		inv, root := clitest.New(t, "exp", "immortal-stream", "delete", workspace.Name, stream.Name)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		ctxCLI, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		done := make(chan error)
		go func() {
			done <- inv.WithContext(ctxCLI).Run()
		}()

		pty.ExpectMatch("Deleted immortal stream")
		pty.ExpectMatch(stream.Name)

		err = <-done
		assert.NoError(t, err)

		// Verify the stream was deleted
		streams, err := client.WorkspaceAgentImmortalStreams(ctx, agent.ID)
		require.NoError(t, err)
		assert.Empty(t, streams)
	})

	t.Run("Delete_NonExistentStream", func(t *testing.T) {
		t.Parallel()

		client, workspace, agentToken := setupWorkspaceForAgent(t)

		// Start the agent
		_ = agenttest.New(t, client.URL, agentToken)
		coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

		// Test the CLI delete command with non-existent stream
		inv, root := clitest.New(t, "exp", "immortal-stream", "delete", workspace.Name, "non-existent-stream")
		clitest.SetupConfig(t, client, root)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestImmortalStreamCLIWorkspaceValidation(t *testing.T) {
	t.Parallel()

	t.Run("WorkspaceNotRunning", func(t *testing.T) {
		t.Parallel()

		client, workspace, _ := setupWorkspaceForAgent(t)
		// Don't start the agent so workspace is not running

		// Stop the workspace
		ctx := context.Background()
		stopReq := codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionStop,
		}
		_, err := client.CreateWorkspaceBuild(ctx, workspace.ID, stopReq)
		require.NoError(t, err)

		// Test list command on stopped workspace
		inv, root := clitest.New(t, "exp", "immortal-stream", "list", workspace.Name)
		clitest.SetupConfig(t, client, root)

		err = inv.WithContext(ctx).Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace must be running")
	})
}

func TestImmortalStreamCLIHelp(t *testing.T) {
	t.Parallel()

	t.Run("MainCommand", func(t *testing.T) {
		inv, _ := clitest.New(t, "exp", "immortal-stream", "--help")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("ListCommand", func(t *testing.T) {
		inv, _ := clitest.New(t, "exp", "immortal-stream", "list", "--help")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})

	t.Run("DeleteCommand", func(t *testing.T) {
		inv, _ := clitest.New(t, "exp", "immortal-stream", "delete", "--help")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
	})
}
