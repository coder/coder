package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestTemplateVersions(t *testing.T) {
	t.Parallel()
	t.Run("ListVersions", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		admin := coderdtest.CreateFirstUser(t, client)
		member, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, admin.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, admin.OrganizationID, version.ID)

		inv, root := clitest.New(t, "templates", "versions", "list", template.Name)
		clitest.SetupConfig(t, member, root)

		pty := ptytest.New(t).Attach(inv)

		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()

		require.NoError(t, <-errC)

		pty.ExpectMatch(version.Name)
		pty.ExpectMatch(version.CreatedBy.Username)
		pty.ExpectMatch("Active")
	})
}
