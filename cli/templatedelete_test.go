package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestTemplateDelete(t *testing.T) {
	t.Parallel()

	t.Run("Ok", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		cmd, root := clitest.New(t, "templates", "delete", template.Name)
		clitest.SetupConfig(t, client, root)
		require.NoError(t, cmd.Execute())

		_, err := client.Template(context.Background(), template.ID)
		require.Error(t, err, "template should not exist")
	})

	t.Run("Multiple", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		templates := []codersdk.Template{
			coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID),
			coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID),
			coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID),
		}
		templateNames := []string{}
		for _, template := range templates {
			templateNames = append(templateNames, template.Name)
		}

		cmd, root := clitest.New(t, append([]string{"templates", "delete"}, templateNames...)...)
		clitest.SetupConfig(t, client, root)
		require.NoError(t, cmd.Execute())

		for _, template := range templates {
			_, err := client.Template(context.Background(), template.ID)
			require.Error(t, err, "template should not exist")
		}
	})

	t.Run("Selector", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		cmd, root := clitest.New(t, "templates", "delete")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		execDone := make(chan error)
		go func() {
			execDone <- cmd.Execute()
		}()

		pty.WriteLine("docker-local")
		require.NoError(t, <-execDone)

		_, err := client.Template(context.Background(), template.ID)
		require.Error(t, err, "template should not exist")
	})
}
