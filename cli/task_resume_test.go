package cli_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpTaskResume(t *testing.T) {
	t.Parallel()

	// pauseTask is a helper that pauses a task and waits for the stop
	// build to complete.
	pauseTask := func(ctx context.Context, t *testing.T, client *codersdk.Client, task codersdk.Task) {
		t.Helper()

		pauseResp, err := client.PauseTask(ctx, task.OwnerName, task.ID)
		require.NoError(t, err)
		require.NotNil(t, pauseResp.WorkspaceBuild)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, pauseResp.WorkspaceBuild.ID)
	}

	t.Run("WithYesFlag", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, userClient, task := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, userClient, task)

		// When: We attempt to resume the task
		inv, root := clitest.New(t, "task", "resume", task.Name, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		// Then: We expect the task to be resumed
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been resumed")

		updated, err := userClient.TaskByIdentifier(ctx, task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	// OtherUserTask verifies that an admin can resume a task owned by
	// another user using the "owner/name" identifier format.
	t.Run("OtherUserTask", func(t *testing.T) {
		t.Parallel()

		// Given: A different user's paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		adminClient, userClient, task := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, userClient, task)

		// When: We attempt to resume their task
		identifier := fmt.Sprintf("%s/%s", task.OwnerName, task.Name)
		inv, root := clitest.New(t, "task", "resume", identifier, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, adminClient, root)

		// Then: We expect the task to be resumed
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been resumed")

		updated, err := adminClient.TaskByIdentifier(ctx, identifier)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	t.Run("NoWait", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, userClient, task := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, userClient, task)

		// When: We attempt to resume the task (and specify no wait)
		inv, root := clitest.New(t, "task", "resume", task.Name, "--yes", "--no-wait")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, userClient, root)

		// Then: We expect to have "no-wait" mode returned
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "no-wait mode")

		// And: The task to eventually be resumed
		require.True(t, task.WorkspaceID.Valid, "task should have a workspace ID")
		ws := coderdtest.MustWorkspace(t, userClient, task.WorkspaceID.UUID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, userClient, ws.LatestBuild.ID)

		updated, err := userClient.TaskByIdentifier(ctx, task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	t.Run("PromptConfirm", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, userClient, task := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, userClient, task)

		// When: We attempt to resume the task
		inv, root := clitest.New(t, "task", "resume", task.Name)
		clitest.SetupConfig(t, userClient, root)

		// And: We confirm we want to resume the task
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Resume task")
		pty.WriteLine("yes")

		// Then: We expect the task to be resumed
		pty.ExpectMatchContext(ctx, "has been resumed")
		require.NoError(t, w.Wait())

		updated, err := userClient.TaskByIdentifier(ctx, task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	t.Run("PromptDecline", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, userClient, task := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, userClient, task)

		// When: We attempt to resume the task
		inv, root := clitest.New(t, "task", "resume", task.Name)
		clitest.SetupConfig(t, userClient, root)

		// But: Say no at the confirmation screen
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Resume task")
		pty.WriteLine("no")
		require.Error(t, w.Wait())

		// Then: We expect the task to still be paused
		updated, err := userClient.TaskByIdentifier(ctx, task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	t.Run("TaskNotPaused", func(t *testing.T) {
		t.Parallel()

		// Given: A running task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, userClient, task := setupCLITaskTest(setupCtx, t, nil)

		// When: We attempt to resume the task that is not paused
		inv, root := clitest.New(t, "task", "resume", task.Name, "--yes")
		clitest.SetupConfig(t, userClient, root)

		// Then: We expect to get an error that the task is not paused
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "is not paused")
	})
}
