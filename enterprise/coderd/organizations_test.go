package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

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
