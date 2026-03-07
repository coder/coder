package cli_test

import (
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

	t.Run("WithYesFlag", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to resume the task
		inv, root := clitest.New(t, "task", "resume", setup.task.Name, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, setup.userClient, root)

		// Then: We expect the task to be resumed
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been resumed")

		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	// OtherUserTask verifies that an admin can resume a task owned by
	// another user using the "owner/name" identifier format.
	t.Run("OtherUserTask", func(t *testing.T) {
		t.Parallel()

		// Given: A different user's paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to resume their task
		identifier := fmt.Sprintf("%s/%s", setup.task.OwnerName, setup.task.Name)
		inv, root := clitest.New(t, "task", "resume", identifier, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, setup.ownerClient, root)

		// Then: We expect the task to be resumed
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been resumed")

		updated, err := setup.ownerClient.TaskByIdentifier(ctx, identifier)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	t.Run("NoWait", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to resume the task (and specify no wait)
		inv, root := clitest.New(t, "task", "resume", setup.task.Name, "--yes", "--no-wait")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, setup.userClient, root)

		// Then: We expect the task to be resumed in the background
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "in the background")

		// And: The task to eventually be resumed
		require.True(t, setup.task.WorkspaceID.Valid, "task should have a workspace ID")
		ws := coderdtest.MustWorkspace(t, setup.userClient, setup.task.WorkspaceID.UUID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.userClient, ws.LatestBuild.ID)

		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	t.Run("PromptConfirm", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to resume the task
		inv, root := clitest.New(t, "task", "resume", setup.task.Name)
		clitest.SetupConfig(t, setup.userClient, root)

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

		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusInitializing, updated.Status)
	})

	t.Run("PromptDecline", func(t *testing.T) {
		t.Parallel()

		// Given: A paused task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)
		pauseTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to resume the task
		inv, root := clitest.New(t, "task", "resume", setup.task.Name)
		clitest.SetupConfig(t, setup.userClient, root)

		// But: Say no at the confirmation screen
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Resume task")
		pty.WriteLine("no")
		require.Error(t, w.Wait())

		// Then: We expect the task to still be paused
		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	t.Run("TaskNotPaused", func(t *testing.T) {
		t.Parallel()

		// Given: A running task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)

		// When: We attempt to resume the task that is not paused
		inv, root := clitest.New(t, "task", "resume", setup.task.Name, "--yes")
		clitest.SetupConfig(t, setup.userClient, root)

		// Then: We expect to get an error that the task is not paused
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "cannot be resumed")
	})
}
