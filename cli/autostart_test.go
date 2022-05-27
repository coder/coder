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
	"github.com/coder/coder/codersdk"
)

func TestAutostart(t *testing.T) {
	t.Parallel()

	t.Run("ShowOK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			cmdArgs   = []string{"autostart", "show", workspace.Name}
			sched     = "CRON_TZ=Europe/Dublin 30 17 * * 1-5"
			stdoutBuf = &bytes.Buffer{}
		)

		err := client.UpdateWorkspaceAutostart(ctx, workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
			Schedule: sched,
		})
		require.NoError(t, err)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err = cmd.Execute()
		require.NoError(t, err, "unexpected error")
		// CRON_TZ gets stripped
		require.Contains(t, stdoutBuf.String(), "schedule: 30 17 * * 1-5")
	})

	t.Run("EnableDisableOK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, user.OrganizationID, project.ID)
			tz        = "Europe/Dublin"
			cmdArgs   = []string{"autostart", "enable", workspace.Name, "--minute", "30", "--hour", "9", "--days", "1-5", "--tz", tz}
			sched     = "CRON_TZ=Europe/Dublin 30 9 * * 1-5"
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will automatically start at", "unexpected output")

		// Ensure autostart schedule updated
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, sched, updated.AutostartSchedule, "expected autostart schedule to be set")

		// Disable schedule
		cmd, root = clitest.New(t, "autostart", "disable", workspace.Name)
		clitest.SetupConfig(t, client, root)
		cmd.SetOut(stdoutBuf)

		err = cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will no longer automatically start", "unexpected output")

		// Ensure autostart schedule updated
		updated, err = client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Empty(t, updated.AutostartSchedule, "expected autostart schedule to not be set")
	})

	t.Run("Enable_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			client  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		)

		cmd, root := clitest.New(t, "autostart", "enable", "doesnotexist")
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 403: forbidden", "unexpected error")
	})

	t.Run("Disable_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			client  = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		)

		cmd, root := clitest.New(t, "autostart", "disable", "doesnotexist")
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 403: forbidden", "unexpected error")
	})

	t.Run("Enable_DefaultSchedule", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
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
		expectedSchedule := fmt.Sprintf("CRON_TZ=%s 0 9 * * 1-5", currTz)
		cmd, root := clitest.New(t, "autostart", "enable", workspace.Name)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")

		// Ensure nothing happened
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, expectedSchedule, updated.AutostartSchedule, "expected default autostart schedule")
	})
}
