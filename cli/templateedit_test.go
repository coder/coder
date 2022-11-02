package cli_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func TestTemplateEdit(t *testing.T) {
	t.Parallel()

	t.Run("Modified", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.Icon = "/icons/default-icon.png"
			ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})

		// Test the cli command.
		name := "new-template-name"
		desc := "lorem ipsum dolor sit amet et cetera"
		icon := "/icons/new-icon.png"
		defaultTTL := 12 * time.Hour
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", name,
			"--description", desc,
			"--icon", icon,
			"--default-ttl", defaultTTL.String(),
		}
		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()

		require.NoError(t, err)

		// Assert that the template metadata changed.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, name, updated.Name)
		assert.Equal(t, desc, updated.Description)
		assert.Equal(t, icon, updated.Icon)
		assert.Equal(t, defaultTTL.Milliseconds(), updated.DefaultTTLMillis)
	})

	t.Run("NotModified", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.Icon = "/icons/default-icon.png"
			ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})

		// Test the cli command.
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", template.Name,
			"--description", template.Description,
			"--icon", template.Icon,
			"--default-ttl", (time.Duration(template.DefaultTTLMillis) * time.Millisecond).String(),
		}
		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)

		err := cmd.Execute()

		require.ErrorContains(t, err, "not modified")

		// Assert that the template metadata did not change.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.Icon, updated.Icon)
		assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
	})
}
