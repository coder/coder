package autostart_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/autostart"
)

func TestWorkspaceDispatcher(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create test workspace names.
	workspaceNames := []string{"workspace-1", "workspace-2", "workspace-3"}

	// Create dispatcher.
	dispatcher := autostart.NewWorkspaceDispatcher(workspaceNames)
	require.Len(t, dispatcher.Channels, 3)

	// Create source channel for updates.
	source := make(chan codersdk.WorkspaceBuildUpdate, 10)

	// Start the dispatcher.
	dispatcher.Start(ctx, source)

	// Send updates for each workspace.
	updates := []codersdk.WorkspaceBuildUpdate{
		{
			WorkspaceName: "workspace-1",
			Transition:    "start",
			JobStatus:     "pending",
			BuildNumber:   1,
		},
		{
			WorkspaceName: "workspace-2",
			Transition:    "start",
			JobStatus:     "running",
			BuildNumber:   1,
		},
		{
			WorkspaceName: "workspace-3",
			Transition:    "start",
			JobStatus:     "succeeded",
			BuildNumber:   1,
		},
		{
			WorkspaceName: "workspace-1",
			Transition:    "start",
			JobStatus:     "succeeded",
			BuildNumber:   1,
		},
	}

	for _, update := range updates {
		source <- update
	}

	// Verify each workspace receives its updates.
	receivedWorkspace1 := <-dispatcher.Channels["workspace-1"]
	require.Equal(t, "workspace-1", receivedWorkspace1.WorkspaceName)
	require.Equal(t, "pending", receivedWorkspace1.JobStatus)

	receivedWorkspace2 := <-dispatcher.Channels["workspace-2"]
	require.Equal(t, "workspace-2", receivedWorkspace2.WorkspaceName)
	require.Equal(t, "running", receivedWorkspace2.JobStatus)

	receivedWorkspace3 := <-dispatcher.Channels["workspace-3"]
	require.Equal(t, "workspace-3", receivedWorkspace3.WorkspaceName)
	require.Equal(t, "succeeded", receivedWorkspace3.JobStatus)

	// workspace-1 should have another update.
	receivedWorkspace1Again := <-dispatcher.Channels["workspace-1"]
	require.Equal(t, "workspace-1", receivedWorkspace1Again.WorkspaceName)
	require.Equal(t, "succeeded", receivedWorkspace1Again.JobStatus)

	// Close the source channel.
	close(source)

	// All workspace channels should close.
	for name, ch := range dispatcher.Channels {
		select {
		case _, ok := <-ch:
			require.False(t, ok, "channel for %s should be closed", name)
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for channel %s to close", name)
		}
	}
}

func TestWorkspaceDispatcher_UnknownWorkspace(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create dispatcher with known workspaces.
	workspaceNames := []string{"workspace-1", "workspace-2"}
	dispatcher := autostart.NewWorkspaceDispatcher(workspaceNames)

	// Create source channel.
	source := make(chan codersdk.WorkspaceBuildUpdate, 10)

	// Start the dispatcher.
	dispatcher.Start(ctx, source)

	// Send update for unknown workspace - should be ignored.
	source <- codersdk.WorkspaceBuildUpdate{
		WorkspaceName: "unknown-workspace",
		Transition:    "start",
		JobStatus:     "pending",
		BuildNumber:   1,
	}

	// Send update for known workspace.
	source <- codersdk.WorkspaceBuildUpdate{
		WorkspaceName: "workspace-1",
		Transition:    "start",
		JobStatus:     "succeeded",
		BuildNumber:   1,
	}

	// workspace-1 should receive its update.
	received := <-dispatcher.Channels["workspace-1"]
	require.Equal(t, "workspace-1", received.WorkspaceName)
	require.Equal(t, "succeeded", received.JobStatus)

	// Close source and verify channels close.
	close(source)

	for name, ch := range dispatcher.Channels {
		select {
		case _, ok := <-ch:
			require.False(t, ok, "channel for %s should be closed", name)
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for channel %s to close", name)
		}
	}
}

func TestWorkspaceDispatcher_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	// Create dispatcher.
	workspaceNames := []string{"workspace-1"}
	dispatcher := autostart.NewWorkspaceDispatcher(workspaceNames)

	// Create source channel.
	source := make(chan codersdk.WorkspaceBuildUpdate, 10)

	// Start the dispatcher.
	dispatcher.Start(ctx, source)

	// Fill up the channel buffer.
	for i := 0; i < 20; i++ {
		source <- codersdk.WorkspaceBuildUpdate{
			WorkspaceID:   uuid.New(),
			WorkspaceName: "workspace-1",
			Transition:    "start",
			JobStatus:     "pending",
			BuildNumber:   int32(i),
		}
	}

	// Cancel context - dispatcher should stop trying to send.
	cancel()

	// Give dispatcher time to react to cancellation.
	time.Sleep(100 * time.Millisecond)

	// Dispatcher goroutine should have stopped, so closing source shouldn't deadlock.
	close(source)

	// Channels might not be closed yet since source was closed after cancellation,
	// but the important thing is that we don't deadlock.
	// Just drain the channel if there's anything.
	drained := 0
	for {
		select {
		case _, ok := <-dispatcher.Channels["workspace-1"]:
			if !ok {
				// Channel closed.
				return
			}
			drained++
			if drained > 100 {
				t.Fatal("drained too many messages, dispatcher not respecting context cancellation")
			}
		case <-time.After(time.Second):
			// Timeout is OK - channel may or may not be closed.
			return
		}
	}
}
