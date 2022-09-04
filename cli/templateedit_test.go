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
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})

		// Test the cli command.
		name := "new-template-name"
		desc := "lorem ipsum dolor sit amet et cetera"
		icon := "/icons/new-icon.png"
		maxTTL := 12 * time.Hour
		minAutostartInterval := time.Minute
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", name,
			"--description", desc,
			"--icon", icon,
			"--max-ttl", maxTTL.String(),
			"--min-autostart-interval", minAutostartInterval.String(),
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
		assert.Equal(t, maxTTL.Milliseconds(), updated.MaxTTLMillis)
		assert.Equal(t, minAutostartInterval.Milliseconds(), updated.MinAutostartIntervalMillis)
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
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})

		// Test the cli command.
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", template.Name,
			"--description", template.Description,
			"--icon", template.Icon,
			"--max-ttl", (time.Duration(template.MaxTTLMillis) * time.Millisecond).String(),
			"--min-autostart-interval", (time.Duration(template.MinAutostartIntervalMillis) * time.Millisecond).String(),
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
		assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, template.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
	})
}
