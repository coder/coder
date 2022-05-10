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

func TestAutostop(t *testing.T) {
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
			cmdArgs   = []string{"autostop", "enable", workspace.Name, "--minute", "30", "--hour", "17", "--days", "1-5", "--tz", "Europe/Dublin"}
			sched     = "CRON_TZ=Europe/Dublin 30 17 * * 1-5"
			stdoutBuf = &bytes.Buffer{}
		)

		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, api.Client, root)
		cmd.SetOut(stdoutBuf)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will automatically stop at", "unexpected output")

		// Ensure autostop schedule updated
		updated, err := api.Client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, sched, updated.AutostopSchedule, "expected autostop schedule to be set")

		// Disable schedule
		cmd, root = clitest.New(t, "autostop", "disable", workspace.Name)
		clitest.SetupConfig(t, api.Client, root)
		cmd.SetOut(stdoutBuf)

		err = cmd.Execute()
		require.NoError(t, err, "unexpected error")
		require.Contains(t, stdoutBuf.String(), "will no longer automatically stop", "unexpected output")

		// Ensure autostop schedule updated
		updated, err = api.Client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Empty(t, updated.AutostopSchedule, "expected autostop schedule to not be set")
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

		cmd, root := clitest.New(t, "autostop", "enable", "doesnotexist")
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

		cmd, root := clitest.New(t, "autostop", "disable", "doesnotexist")
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
		expectedSchedule := fmt.Sprintf("CRON_TZ=%s 0 18 * * 1-5", currTz)

		cmd, root := clitest.New(t, "autostop", "enable", workspace.Name)
		clitest.SetupConfig(t, api.Client, root)

		err := cmd.Execute()
		require.NoError(t, err, "unexpected error")

		// Ensure nothing happened
		updated, err := api.Client.Workspace(ctx, workspace.ID)
		require.NoError(t, err, "fetch updated workspace")
		require.Equal(t, expectedSchedule, updated.AutostopSchedule, "expected default autostop schedule")
	})
}
