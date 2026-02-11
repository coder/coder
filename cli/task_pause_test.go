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

		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, client, task := setupCLITaskTest(setupCtx, t, nil)

		inv, root := clitest.New(t, "task", "pause", task.Name, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been paused")

		// Verify the task is actually paused on the server.
		updated, err := client.TaskByIdentifier(ctx, task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	// OtherUserTask verifies that an admin can pause a task owned by
	// another user using the "owner/name" identifier format.
	t.Run("OtherUserTask", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		adminClient, _, task := setupCLITaskTest(setupCtx, t, nil)

		identifier := fmt.Sprintf("%s/%s", task.OwnerName, task.Name)

		inv, root := clitest.New(t, "task", "pause", identifier, "--yes")
		output := clitest.Capture(inv)
		clitest.SetupConfig(t, adminClient, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, output.Stdout(), "has been paused")

		// Verify the task is actually paused on the server.
		updated, err := adminClient.TaskByIdentifier(ctx, identifier)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	t.Run("PromptConfirm", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, userClient, task := setupCLITaskTest(setupCtx, t, nil)

		inv, root := clitest.New(t, "task", "pause", task.Name)
		clitest.SetupConfig(t, userClient, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Pause task")
		pty.WriteLine("yes")
		pty.ExpectMatchContext(ctx, "has been paused")
		require.NoError(t, w.Wait())

		// Verify the task is actually paused on the server.
		updated, err := userClient.TaskByIdentifier(ctx, task.Name)
		require.NoError(t, err)
		require.Equal(t, codersdk.TaskStatusPaused, updated.Status)
	})

	t.Run("PromptDecline", func(t *testing.T) {
		t.Parallel()

		setupCtx := testutil.Context(t, testutil.WaitLong)
		_, userClient, task := setupCLITaskTest(setupCtx, t, nil)

		inv, root := clitest.New(t, "task", "pause", task.Name)
		clitest.SetupConfig(t, userClient, root)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Pause task")
		pty.WriteLine("no")
		require.Error(t, w.Wait())

		// Verify the task was not paused.
		updated, err := userClient.TaskByIdentifier(ctx, task.Name)
		require.NoError(t, err)
		require.NotEqual(t, codersdk.TaskStatusPaused, updated.Status)
	})
}
