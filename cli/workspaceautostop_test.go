package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
)

func TestWorkspaceAutostop(t *testing.T) {
	t.Parallel()

	t.Run("EnableDisableOK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, nil)
			_         = coderdtest.NewProvisionerDaemon(t, client)
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"workspaces", "autostop", "enable", workspace.Name, "--minute", "30", "--hour", "17", "--days", "1-5", "--tz", "Europe/Dublin"}
			sched     = "CRON_TZ=Europe/Dublin 30 17 * * 1-5"
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will automatically stop at", "unexpected output")

		// Ensure autostop schedule updated
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, sched, updated.AutostopSchedule, "expected autostop schedule to be set")

		// Disable schedule
		cmd, root = clitest.New(t, "workspaces", "autostop", "disable", workspace.Name)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err = cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will no longer automatically stop", "unexpected output")

		// Ensure autostop schedule updated
		updated, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Empty(t, updated.AutostopSchedule, "expected autostop schedule to not be set")
	})

	t.Run("Enable_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			client  = coderdtest.New(t, nil)
			_       = coderdtest.NewProvisionerDaemon(t, client)
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		)

		cmd, root := clitest.New(t, "workspaces", "autostop", "enable", "doesnotexist")
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 404: no workspace found by name", "unexpected error")
	})

	t.Run("Disable_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			client  = coderdtest.New(t, nil)
			_       = coderdtest.NewProvisionerDaemon(t, client)
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		)

		cmd, root := clitest.New(t, "workspaces", "autostop", "disable", "doesnotexist")
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 404: no workspace found by name", "unexpected error")
	})

	t.Run("Enable_DefaultSchedule", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, nil)
			_         = coderdtest.NewProvisionerDaemon(t, client)
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
		)

		// check current TZ env var
		currTz := os.Getenv("TZ")
		if currTz == "" {
			currTz = "UTC"
		}
		expectedSchedule := fmt.Sprintf("CRON_TZ=%s 0 18 * * 1-5", currTz)

		cmd, root := clitest.New(t, "workspaces", "autostop", "enable", workspace.Name)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")

		// Ensure nothing happened
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, expectedSchedule, updated.AutostopSchedule, "expected default autostop schedule")
	})
}
