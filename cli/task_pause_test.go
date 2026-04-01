package cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpTaskPause(t *testing.T) {
	t.Parallel()

	t.Run("WithYesFlag", func(t *testing.T) {
		t.Parallel()

		// Given: A running task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)

		// When: We attempt to pause the task
		inv, root := clitest.New(t, "task", "pause", setup.task.Name, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, setup.userClient, root)

		// Then: Expect the task to be paused
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been paused")

		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	// OtherUserTask verifies that an admin can pause a task owned by
	// another user using the "owner/name" identifier format.
	t.Run("OtherUserTask", func(t *testing.T) {
		t.Parallel()

		// Given: A different user's running task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)

		// When: We attempt to pause their task
		identifier := fmt.Sprintf("%s/%s", setup.task.OwnerName, setup.task.Name)
		inv, root := clitest.New(t, "task", "pause", identifier, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, setup.ownerClient, root)

		// Then: We expect the task to be paused
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been paused")

		updated, err := setup.ownerClient.TaskByIdentifier(ctx, identifier)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	t.Run("PromptConfirm", func(t *testing.T) {
		t.Parallel()

		// Given: A running task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)

		// When: We attempt to pause the task
		inv, root := clitest.New(t, "task", "pause", setup.task.Name)
		clitest.SetupConfig(t, setup.userClient, root)

		// And: We confirm we want to pause the task
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Pause task")
		pty.WriteLine("yes")

		// Then: We expect the task to be paused
		pty.ExpectMatchContext(ctx, "has been paused")
		require.NoError(t, w.Wait())

		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	t.Run("PromptDecline", func(t *testing.T) {
		t.Parallel()

		// Given: A running task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)

		// When: We attempt to pause the task
		inv, root := clitest.New(t, "task", "pause", setup.task.Name)
		clitest.SetupConfig(t, setup.userClient, root)

		// But: We say no at the confirmation screen
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Pause task")
		pty.WriteLine("no")
		require.Error(t, w.Wait())

		// Then: We expect the task to not be paused
		updated, err := setup.userClient.TaskByIdentifier(ctx, setup.task.Name)
		require.NoError(t, err)
		require.NotEqual(t, codersdk.TaskStatusPaused, updated.Status)
	})

	t.Run("TaskAlreadyPaused", func(t *testing.T) {
		t.Parallel()

		// Given: A running task
		setupCtx := testutil.Context(t, testutil.WaitLong)
		setup := setupCLITaskTest(setupCtx, t, nil)

		// And: We paused the running task
		pauseTask(setupCtx, t, setup.userClient, setup.task)

		// When: We attempt to pause the task again
		inv, root := clitest.New(t, "task", "pause", setup.task.Name, "--yes")
		clitest.SetupConfig(t, setup.userClient, root)

		// Then: We expect to get an error that the task is already paused
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "is already paused")
	})
}
