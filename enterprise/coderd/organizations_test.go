package coderd_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestMultiOrgFetch(t *testing.T) {
	t.Parallel()
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentMultiOrganization)}
	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})

	ctx := testutil.Context(t, testutil.WaitLong)

	makeOrgs := []string{"foo", "bar", "baz"}
	for _, name := range makeOrgs {
		_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        name,
			DisplayName: name,
		})
		require.NoError(t, err)
	}

	//nolint:gocritic // using the owner intentionally since only they can make orgs
	myOrgs, err := client.OrganizationsByUser(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NotNil(t, myOrgs)
	require.Len(t, myOrgs, len(makeOrgs)+1)

	orgs, err := client.Organizations(ctx)
	require.NoError(t, err)
	require.NotNil(t, orgs)
	require.ElementsMatch(t, myOrgs, orgs)
}

func TestOrganizationsByUser(t *testing.T) {
	t.Parallel()
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentMultiOrganization)}
	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})

	ctx := testutil.Context(t, testutil.WaitLong)

	orgs, err := client.OrganizationsByUser(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NotNil(t, orgs)
	require.Len(t, orgs, 1)
	require.True(t, orgs[0].IsDefault, "first org is always default")

	// Make an extra org, and it should not be defaulted.
	notDefault, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name:        "another",
		DisplayName: "Another",
	})
	require.NoError(t, err)
	require.False(t, notDefault.IsDefault, "only 1 default org allowed")
}

func TestAddOrganizationMembers(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentMultiOrganization)}
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		_, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitMedium)
		//nolint:gocritic // must be an owner, only owners can create orgs
		otherOrg, err := ownerClient.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
			Name:        "Other",
			DisplayName: "",
			Description: "",
			Icon:        "",
		})
		require.NoError(t, err, "create another organization")

		inv, root := clitest.New(t, "organization", "members", "add", "-O", otherOrg.ID.String(), user.Username)
		//nolint:gocritic // must be an owner
		clitest.SetupConfig(t, ownerClient, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		//nolint:gocritic // must be an owner
		members, err := ownerClient.OrganizationMembers(ctx, otherOrg.ID)
		require.NoError(t, err)

		require.Len(t, members, 2)
	})
}
