package cli_test

import (
	"context"
	"strconv"
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

	t.Run("FirstEmptyThenModified", func(t *testing.T) {
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
		allowUserCancelWorkspaceJobs := false

		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--name", name,
			"--display-name", displayName,
			"--description", desc,
			"--icon", icon,
			"--default-ttl", defaultTTL.String(),
			"--allow-user-cancel-workspace-jobs=" + strconv.FormatBool(allowUserCancelWorkspaceJobs),
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
		assert.Equal(t, allowUserCancelWorkspaceJobs, updated.AllowUserCancelWorkspaceJobs)
	})
	t.Run("FirstEmptyThenNotModified", func(t *testing.T) {
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
			"--allow-user-cancel-workspace-jobs=" + strconv.FormatBool(template.AllowUserCancelWorkspaceJobs),
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
		assert.Equal(t, template.AllowUserCancelWorkspaceJobs, updated.AllowUserCancelWorkspaceJobs)
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
	t.Run("WithPropertiesThenModified", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		initialDisplayName := "This is a template"
		initialDescription := "This is description"
		initialIcon := "/img/icon.png"

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DisplayName = initialDisplayName
			ctr.Description = initialDescription
			ctr.Icon = initialIcon
		})

		// Test created template
		created, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, initialDisplayName, created.DisplayName)
		assert.Equal(t, initialDescription, created.Description)
		assert.Equal(t, initialIcon, created.Icon)

		// Test the cli command.
		displayName := "New Display Name 789"
		icon := "/icons/new-icon.png"
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
			"--display-name", displayName,
			"--icon", icon,
		}
		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)

		ctx, _ := testutil.Context(t)
		err = cmd.ExecuteContext(ctx)

		require.NoError(t, err)

		// Assert that the template metadata changed.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, template.Name, updated.Name)             // doesn't change
		assert.Equal(t, initialDescription, updated.Description) // doesn't change
		assert.Equal(t, displayName, updated.DisplayName)
		assert.Equal(t, icon, updated.Icon)
	})
	t.Run("WithPropertiesThenEmptyEdit", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		initialDisplayName := "This is a template"
		initialDescription := "This is description"
		initialIcon := "/img/icon.png"

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DisplayName = initialDisplayName
			ctr.Description = initialDescription
			ctr.Icon = initialIcon
		})

		// Test created template
		created, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		assert.Equal(t, initialDisplayName, created.DisplayName)
		assert.Equal(t, initialDescription, created.Description)
		assert.Equal(t, initialIcon, created.Icon)

		// Test the cli command.
		cmdArgs := []string{
			"templates",
			"edit",
			template.Name,
		}
		cmd, root := clitest.New(t, cmdArgs...)
		clitest.SetupConfig(t, client, root)

		ctx, _ := testutil.Context(t)
		err = cmd.ExecuteContext(ctx)

		require.NoError(t, err)

		// Assert that the template metadata changed.
		updated, err := client.Template(context.Background(), template.ID)
		require.NoError(t, err)
		// Properties don't change
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, template.Description, updated.Description)
		// These properties are removed, as the API considers it as "delete" request
		// See: https://github.com/coder/coder/issues/5066
		assert.Equal(t, "", updated.Icon)
		assert.Equal(t, "", updated.DisplayName)
	})
}
