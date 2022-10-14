package cli_test

import (
	"sort"
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
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
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

		// expect that templates are listed alphebetically
		var templatesList = []string{firstTemplate.Name, secondTemplate.Name}
		sort.Strings(templatesList)

		require.NoError(t, <-errC)

		for _, name := range templatesList {
			pty.ExpectMatch(name)
		}
	})
	t.Run("NoTemplates", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{})
		coderdtest.CreateFirstUser(t, client)

		cmd, root := clitest.New(t, "templates", "list")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetErr(pty.Output())

		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()

		require.NoError(t, <-errC)

		pty.ExpectMatch("No templates found in")
		pty.ExpectMatch(coderdtest.FirstUserParams.Username)
		pty.ExpectMatch("Create one:")
	})
}
