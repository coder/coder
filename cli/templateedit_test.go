package cli_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestTemplateEdit(t *testing.T) {
	t.Parallel()

	t.Run("Modified", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Test the cli command.
		name := "new-template-name"
		displayName := "New Display Name 789"
		desc := "lorem ipsum dolor sit amet et cetera"
		icon := "/icons/new-icon.png"
		maxTTL := 12 * time.Hour
		minAutostartInterval := time.Minute
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", name,
			"--display-name", displayName,
			"--description", desc,
			"--icon", icon,
			"--max-ttl", maxTTL.String(),
			"--min-autostart-interval", minAutostartInterval.String(),
		}
		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)

		ctx, _ := testutil.Context(t)
		err := cmd.ExecuteContext(ctx)

		require.NoError(t, err)

		// Assert that the template metadata changed.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, name, updated.Name)
		assert.Equal(t, displayName, updated.DisplayName)
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
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

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

		ctx, _ := testutil.Context(t)
		err := cmd.ExecuteContext(ctx)

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
	t.Run("InvalidDisplayName", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		// Test the cli command.
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", template.Name,
			"--display-name", "a-b-c",
		}
		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)

		ctx, _ := testutil.Context(t)
		err := cmd.ExecuteContext(ctx)

		require.Error(t, err, "client call must fail")
		sdkError, isSdkError := codersdk.AsError(err)
		require.True(t, isSdkError, "sdk error is expected")
		require.Len(t, sdkError.Response.Validations, 1, "field validation error is expected")
		require.Equal(t, sdkError.Response.Validations[0].Detail, `Validation failed for tag "template_display_name" with value: "a-b-c"`)

		// Assert that the template metadata did not change.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, "", template.DisplayName)
	})
}
