package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAutostart(t *testing.T) {
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
			workspace = coderdtest.CreateWorkspace(t, client, codersdk.Me, project.ID)
			sched     = "CRON_TZ=Europe/Dublin 30 9 1-5"
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, "workspaces", "autostart", "enable", workspace.Name, sched)
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
		cmd, root = clitest.New(t, "workspaces", "autostart", "disable", workspace.Name)
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
			client  = coderdtest.New(t, nil)
			_       = coderdtest.NewProvisionerDaemon(t, client)
			user    = coderdtest.CreateFirstUser(t, client)
			version = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			sched   = "CRON_TZ=Europe/Dublin 30 9 1-5"
		)

		cmd, root := clitest.New(t, "workspaces", "autostart", "enable", "doesnotexist", sched)
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

		cmd, root := clitest.New(t, "workspaces", "autostart", "disable", "doesnotexist")
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 404: no workspace found by name", "unexpected error")
	})

	t.Run("Enable_InvalidSchedule", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, nil)
			_         = coderdtest.NewProvisionerDaemon(t, client)
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, codersdk.Me, project.ID)
			sched     = "sdfasdfasdf asdf asdf"
		)

		cmd, root := clitest.New(t, "workspaces", "autostart", "enable", workspace.Name, sched)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "failed to parse int from sdfasdfasdf: strconv.Atoi:", "unexpected error")

		// Ensure nothing happened
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Empty(t, updated.AutostartSchedule, "expected autostart schedule to be empty")
	})

	t.Run("Enable_NoSchedule", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			client    = coderdtest.New(t, nil)
			_         = coderdtest.NewProvisionerDaemon(t, client)
			user      = coderdtest.CreateFirstUser(t, client)
			version   = coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			project   = coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, client, codersdk.Me, project.ID)
		)

		cmd, root := clitest.New(t, "workspaces", "autostart", "enable", workspace.Name)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "accepts 2 arg(s), received 1", "unexpected error")

		// Ensure nothing happened
		updated, err := client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Empty(t, updated.AutostartSchedule, "expected autostart schedule to be empty")
	})
}
