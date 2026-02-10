package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestExpTaskPause(t *testing.T) {
	t.Parallel()

	// setup creates an AI task with a completed workspace build, ready
	// to be paused. Follows the pattern from TestPauseTask in
	// coderd/aitasks_test.go.
	setup := func(t *testing.T) (*codersdk.Client, codersdk.Task) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: []*proto.Response{
				{Type: &proto.Response_Graph{Graph: &proto.GraphComplete{
					HasAiTasks: true,
				}}},
			},
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		tpl := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		task, err := userClient.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
			TemplateVersionID: tpl.ActiveVersionID,
			Input:             "test task for pause",
		})
		require.NoError(t, err)
		require.True(t, task.WorkspaceID.Valid)

		ws, err := userClient.Workspace(ctx, task.WorkspaceID.UUID)
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, ws.LatestBuild.ID)

		return userClient, task
	}

	t.Run("WithYesFlag", func(t *testing.T) {
		t.Parallel()

		client, task := setup(t)

		var stdout strings.Builder
		inv, root := clitest.New(t, "task", "pause", task.Name, "--yes")
		inv.Stdout = &stdout
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, stdout.String(), "has been paused")
	})

	t.Run("PromptConfirm", func(t *testing.T) {
		t.Parallel()

		client, task := setup(t)

		inv, root := clitest.New(t, "task", "pause", task.Name)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Pause task")
		pty.WriteLine("yes")
		pty.ExpectMatchContext(ctx, "has been paused")
		require.NoError(t, w.Wait())
	})

	t.Run("PromptDecline", func(t *testing.T) {
		t.Parallel()

		client, task := setup(t)

		inv, root := clitest.New(t, "task", "pause", task.Name)
		clitest.SetupConfig(t, client, root)

		ctx := testutil.Context(t, testutil.WaitLong)
		inv = inv.WithContext(ctx)
		pty := ptytest.New(t).Attach(inv)
		w := clitest.StartWithWaiter(t, inv)
		pty.ExpectMatchContext(ctx, "Pause task")
		pty.WriteLine("no")
		require.Error(t, w.Wait())
	})
}
