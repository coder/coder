package cli_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateEdit(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentTemplateUpdatePolicies),
		}

		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAccessControl: 1,
				},
			},
			Options: &coderdtest.Options{
				DeploymentValues:         dv,
				IncludeProvisionerDaemon: true,
			},
		})

		templateAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, templateAdmin, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, version.ID)
		template := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, version.ID)
		require.False(t, template.RequireActiveVersion)

		inv, conf := newCLI(t, "templates",
			"edit", template.Name,
			"--require-active-version",
			"-y",
		)

		clitest.SetupConfig(t, templateAdmin, conf)

		err := inv.Run()
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		template, err = templateAdmin.Template(ctx, template.ID)
		require.NoError(t, err)
		require.True(t, template.RequireActiveVersion)
	})

	t.Run("NotEntitled", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentTemplateUpdatePolicies),
		}

		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
			Options: &coderdtest.Options{
				DeploymentValues:         dv,
				IncludeProvisionerDaemon: true,
			},
		})
		templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)
		require.False(t, template.RequireActiveVersion)

		inv, conf := newCLI(t, "templates",
			"edit", template.Name,
			"--require-active-version",
			"-y",
		)

		clitest.SetupConfig(t, templateAdmin, conf)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "your license is not entitled to use enterprise access control, so you cannot set --require-active-version")
	})

	t.Run("WorkspaceCleanup", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{
			string(codersdk.ExperimentWorkspaceActions),
		}

		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
			Options: &coderdtest.Options{
				DeploymentValues:         dv,
				IncludeProvisionerDaemon: true,
			},
		})

		templateAdmin, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, templateAdmin, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdmin, version.ID)
		template := coderdtest.CreateTemplate(t, templateAdmin, owner.OrganizationID, version.ID)
		require.False(t, template.RequireActiveVersion)

		const (
			expectedFailureTTL           = time.Hour * 3
			expectedDormancyThreshold    = time.Hour * 4
			expectedDormancyAutoDeletion = time.Minute * 10
		)
		inv, conf := newCLI(t, "templates",
			"edit", template.Name,
			"--failure-ttl="+expectedFailureTTL.String(),
			"--dormancy-threshold="+expectedDormancyThreshold.String(),
			"--dormancy-auto-deletion="+expectedDormancyAutoDeletion.String(),
			"-y",
		)

		clitest.SetupConfig(t, templateAdmin, conf)

		err := inv.Run()
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		template, err = templateAdmin.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, expectedFailureTTL.Milliseconds(), template.FailureTTLMillis)
		require.Equal(t, expectedDormancyThreshold.Milliseconds(), template.TimeTilDormantMillis)
		require.Equal(t, expectedDormancyAutoDeletion.Milliseconds(), template.TimeTilDormantAutoDeleteMillis)

		inv, conf = newCLI(t, "templates",
			"edit", template.Name,
			"--display-name=idc",
			"-y",
		)

		clitest.SetupConfig(t, templateAdmin, conf)

		err = inv.Run()
		require.NoError(t, err)

		// Refetch the template to assert we haven't inadvertently updated
		// the values to their default values.
		template, err = templateAdmin.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, expectedFailureTTL.Milliseconds(), template.FailureTTLMillis)
		require.Equal(t, expectedDormancyThreshold.Milliseconds(), template.TimeTilDormantMillis)
		require.Equal(t, expectedDormancyAutoDeletion.Milliseconds(), template.TimeTilDormantAutoDeleteMillis)
	})
}
