package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestRestart(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		ctx := testutil.Context(t, testutil.WaitLong)

		inv, root := clitest.New(t, "restart", workspace.Name, "--yes")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t).Attach(inv)

		done := make(chan error, 1)
		go func() {
			done <- inv.WithContext(ctx).Run()
		}()
		pty.ExpectMatch("Stopping workspace")
		pty.ExpectMatch("Starting workspace")
		pty.ExpectMatch("workspace has been restarted")

		err := <-done
		require.NoError(t, err, "execute failed")
	})
}
