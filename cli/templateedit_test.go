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
		defaultTTL := 12 * time.Hour
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", name,
			"--display-name", displayName,
			"--description", desc,
			"--icon", icon,
			"--default-ttl", defaultTTL.String(),
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
		assert.Equal(t, defaultTTL.Milliseconds(), updated.DefaultTTLMillis)
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
			"--default-ttl", (time.Duration(template.DefaultTTLMillis) * time.Millisecond).String(),
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
		assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
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
			"--display-name", " a-b-c",
		}
		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)

		ctx, _ := testutil.Context(t)
		err := cmd.ExecuteContext(ctx)

		require.Error(t, err, "client call must fail")
		_, isSdkError := codersdk.AsError(err)
		require.True(t, isSdkError, "sdk error is expected")

		// Assert that the template metadata did not change.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, "", template.DisplayName)
	})
}
