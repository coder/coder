package cli_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateCreate(t *testing.T) {
	t.Parallel()

	t.Run("RequireActiveVersion", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAccessControl: 1,
				},
			},
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
		})
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		inv, conf := newCLI(t, "templates",
			"create", "new",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--require-active-version",
			"-y",
		)

		clitest.SetupConfig(t, templateAdmin, conf)

		err := inv.Run()
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		template, err := templateAdmin.TemplateByName(ctx, user.OrganizationID, "new")
		require.NoError(t, err)
		require.True(t, template.RequireActiveVersion)
	})

	t.Run("WorkspaceCleanup", func(t *testing.T) {
		t.Parallel()

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
		})
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleTemplateAdmin())

		source := clitest.CreateTemplateVersionSource(t, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionApply: echo.ApplyComplete,
		})

		const (
			expectedFailureTTL           = time.Hour * 3
			expectedDormancyThreshold    = time.Hour * 4
			expectedDormancyAutoDeletion = time.Minute * 10
		)

		inv, conf := newCLI(t, "templates",
			"create", "new",
			"--directory", source,
			"--test.provisioner", string(database.ProvisionerTypeEcho),
			"--failure-ttl="+expectedFailureTTL.String(),
			"--dormancy-threshold="+expectedDormancyThreshold.String(),
			"--dormancy-auto-deletion="+expectedDormancyAutoDeletion.String(),
			"-y",
			"--",
		)

		clitest.SetupConfig(t, templateAdmin, conf)

		err := inv.Run()
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		template, err := templateAdmin.TemplateByName(ctx, user.OrganizationID, "new")
		require.NoError(t, err)
		require.Equal(t, expectedFailureTTL.Milliseconds(), template.FailureTTLMillis)
		require.Equal(t, expectedDormancyThreshold.Milliseconds(), template.TimeTilDormantMillis)
		require.Equal(t, expectedDormancyAutoDeletion.Milliseconds(), template.TimeTilDormantAutoDeleteMillis)
	})

	t.Run("NotEntitled", func(t *testing.T) {
		t.Parallel()

		client, admin := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
		})
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())

		inv, conf := newCLI(t, "templates",
			"create", "new",
			"--require-active-version",
			"-y",
		)

		clitest.SetupConfig(t, templateAdmin, conf)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "your license is not entitled to use enterprise access control, so you cannot set --require-active-version")
	})
}
