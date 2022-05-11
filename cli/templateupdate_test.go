package cli_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/pty/ptytest"
)

func TestTemplateUpdate(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	_ = coderdtest.NewProvisionerDaemon(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
	_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	// Test the cli command.
	source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
		Parse:     echo.ParseComplete,
		Provision: echo.ProvisionComplete,
	})
	cmd, root := clitest.New(t, "templates", "update", template.Name, "--directory", source, "--test.provisioner", string(database.ProvisionerTypeEcho))
	clitest.SetupConfig(t, client, root)
	pty := ptytest.New(t)
	cmd.SetIn(pty.Input())
	cmd.SetOut(pty.Output())
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := cmd.Execute()
		require.NoError(t, err)
	}()
	matches := []string{
		"Upload", "yes",
	}
	for i := 0; i < len(matches); i += 2 {
		match := matches[i]
		value := matches[i+1]
		pty.ExpectMatch(match)
		pty.WriteLine(value)
	}
	<-done

	// Assert that the template version changed.
	templateVersions, err := client.TemplateVersionsByTemplate(context.Background(), codersdk.TemplateVersionsByTemplateRequest{
		TemplateID: template.ID,
	})
	require.NoError(t, err)
	assert.Len(t, templateVersions, 2)
	assert.NotEqual(t, template.ActiveVersionID, templateVersions[1].ID)
}
