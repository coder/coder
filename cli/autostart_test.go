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

func TestAutostart(t *testing.T) {
	t.Parallel()

	t.Run("EnableDisableOK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			api       = coderdtest.New(t, nil)
			_         = coderdtest.NewProvisionerDaemon(t, api.Client)
			user      = coderdtest.CreateFirstUser(t, api.Client)
			version   = coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
			project   = coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, project.ID)
			tz        = "Europe/Dublin"
			cmdArgs   = []string{"autostart", "enable", workspace.Name, "--minute", "30", "--hour", "9", "--days", "1-5", "--tz", tz}
			sched     = "CRON_TZ=Europe/Dublin 30 9 * * 1-5"
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, api.Client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will automatically start at", "unexpected output")

		// Ensure autostart schedule updated
		updated, err := api.Client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, sched, updated.AutostartSchedule, "expected autostart schedule to be set")

		// Disable schedule
		cmd, root = clitest.New(t, "autostart", "disable", workspace.Name)
		clitest.SetupConfig(t, api.Client, root)
		cmd.SetOut(stdoutBuf)

		err = cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will no longer automatically start", "unexpected output")

		// Ensure autostart schedule updated
		updated, err = api.Client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Empty(t, updated.AutostartSchedule, "expected autostart schedule to not be set")
	})

	t.Run("Enable_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			api     = coderdtest.New(t, nil)
			_       = coderdtest.NewProvisionerDaemon(t, api.Client)
			user    = coderdtest.CreateFirstUser(t, api.Client)
			version = coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		)

		cmd, root := clitest.New(t, "autostart", "enable", "doesnotexist")
		clitest.SetupConfig(t, api.Client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 404: no workspace found by name", "unexpected error")
	})

	t.Run("Disable_NotFound", func(t *testing.T) {
		t.Parallel()

		var (
			api     = coderdtest.New(t, nil)
			_       = coderdtest.NewProvisionerDaemon(t, api.Client)
			user    = coderdtest.CreateFirstUser(t, api.Client)
			version = coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, nil)
			_       = coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
		)

		cmd, root := clitest.New(t, "autostart", "disable", "doesnotexist")
		clitest.SetupConfig(t, api.Client, root)

		err := cmd.Execute()
		require.ErrorContains(t, err, "status code 404: no workspace found by name", "unexpected error")
	})

	t.Run("Enable_DefaultSchedule", func(t *testing.T) {
		t.Parallel()

		var (
			ctx       = context.Background()
			api       = coderdtest.New(t, nil)
			_         = coderdtest.NewProvisionerDaemon(t, api.Client)
			user      = coderdtest.CreateFirstUser(t, api.Client)
			version   = coderdtest.CreateTemplateVersion(t, api.Client, user.OrganizationID, nil)
			_         = coderdtest.AwaitTemplateVersionJob(t, api.Client, version.ID)
			project   = coderdtest.CreateTemplate(t, api.Client, user.OrganizationID, version.ID)
			workspace = coderdtest.CreateWorkspace(t, api.Client, user.OrganizationID, project.ID)
		)

		// check current TZ env var
		currTz := os.Getenv("TZ")
		if currTz == "" {
			currTz = "UTC"
		}
		expectedSchedule := fmt.Sprintf("CRON_TZ=%s 0 9 * * 1-5", currTz)
		cmd, root := clitest.New(t, "autostart", "enable", workspace.Name)
		clitest.SetupConfig(t, api.Client, root)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")

		// Ensure nothing happened
		updated, err := api.Client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, expectedSchedule, updated.AutostartSchedule, "expected default autostart schedule")
	})
}
