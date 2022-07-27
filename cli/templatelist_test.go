package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
)

func TestTemplateList(t *testing.T) {
	t.Parallel()
	t.Run("ListTemplates", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		firstVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, firstVersion.ID)
		firstTemplate := coderdtest.CreateTemplate(t, client, user.OrganizationID, firstVersion.ID)

		secondVersion := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, secondVersion.ID)
		secondTemplate := coderdtest.CreateTemplate(t, client, user.OrganizationID, secondVersion.ID)

		cmd, root := clitest.New(t, "templates", "list")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()

		require.NoError(t, <-errC)

		pty.ExpectMatch(firstTemplate.Name)
		pty.ExpectMatch(secondTemplate.Name)
	})
	t.Run("NoTemplates", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		coderdtest.CreateFirstUser(t, client)

		cmd, root := clitest.New(t, "templates", "list")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()

		require.NoError(t, <-errC)

		pty.ExpectMatch("No templates found in testuser! Create one:")
	})
}
