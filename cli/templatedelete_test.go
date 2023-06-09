package cli_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestTemplateDelete(t *testing.T) {
	t.Parallel()

	t.Run("Ok", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		inv, root := clitest.New(t, "templates", "delete", template.Name)

		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
		}()

		pty.ExpectMatch(fmt.Sprintf("Delete these templates: %s?", cliui.DefaultStyles.Code.Render(template.Name)))
		pty.WriteLine("yes")

		require.NoError(t, <-execDone)

		_, err := client.Template(context.Background(), template.ID)
		require.Error(t, err, "template should not exist")
	})

	t.Run("Multiple --yes", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		templates := []codersdk.Template{}
		templateNames := []string{}
		for i := 0; i < 3; i++ {
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			templates = append(templates, template)
			templateNames = append(templateNames, template.Name)
		}

		inv, root := clitest.New(t, append([]string{"templates", "delete", "--yes"}, templateNames...)...)
		clitest.SetupConfig(t, client, root)
		require.NoError(t, inv.Run())

		for _, template := range templates {
			_, err := client.Template(context.Background(), template.ID)
			require.Error(t, err, "template should not exist")
		}
	})

	t.Run("Multiple prompted", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		templates := []codersdk.Template{}
		templateNames := []string{}
		for i := 0; i < 3; i++ {
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			templates = append(templates, template)
			templateNames = append(templateNames, template.Name)
		}

		inv, root := clitest.New(t, append([]string{"templates", "delete"}, templateNames...)...)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
		}()

		pty.ExpectMatch(fmt.Sprintf("Delete these templates: %s?", cliui.DefaultStyles.Code.Render(strings.Join(templateNames, ", "))))
		pty.WriteLine("yes")

		require.NoError(t, <-execDone)

		for _, template := range templates {
			_, err := client.Template(context.Background(), template.ID)
			require.Error(t, err, "template should not exist")
		}
	})

	t.Run("Selector", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		inv, root := clitest.New(t, "templates", "delete")
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t).Attach(inv)

		execDone := make(chan error)
		go func() {
			execDone <- inv.Run()
		}()

		pty.WriteLine("yes")
		require.NoError(t, <-execDone)

		_, err := client.Template(context.Background(), template.ID)
		require.Error(t, err, "template should not exist")
	})
}
