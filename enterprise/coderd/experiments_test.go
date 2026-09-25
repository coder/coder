package coderd_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/experiments"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// TestExperimentRuleConditions checks that conditions over group and
// organization membership decide /api/v2/experiments per user.
func TestExperimentRuleConditions(t *testing.T) {
	t.Parallel()

	ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC:          1,
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	ctx := testutil.Context(t, testutil.WaitLong)

	defaultOrg, err := ownerClient.Organization(ctx, owner.OrganizationID)
	require.NoError(t, err)
	otherOrg := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{})

	groupedClient, grouped := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	otherOrgClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, otherOrg.ID)
	plainClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	group, err := ownerClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "rollout"})
	require.NoError(t, err)
	_, err = ownerClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
		AddUsers: []string{grouped.ID.String()},
	})
	require.NoError(t, err)

	//nolint:gocritic // Tests seed rules directly; there is no rules API yet.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	for ex, condition := range map[codersdk.Experiment]string{
		codersdk.ExperimentExample:       fmt.Sprintf("%q in user.groups", defaultOrg.Name+"/"+group.Name),
		codersdk.ExperimentMCPToolSearch: fmt.Sprintf("%q in user.organizations", otherOrg.Name),
	} {
		_, _, changed, err := experiments.WriteRule(systemCtx, db, owner.UserID, ex, experiments.Rule{
			Mode:      experiments.ModeCondition,
			Condition: condition,
		}, 0)
		require.NoError(t, err)
		require.True(t, changed)
	}

	for _, tc := range []struct {
		name   string
		client *codersdk.Client
		want   codersdk.Experiments
	}{
		{name: "GroupMember", client: groupedClient, want: codersdk.Experiments{codersdk.ExperimentExample}},
		{name: "OrganizationMember", client: otherOrgClient, want: codersdk.Experiments{codersdk.ExperimentMCPToolSearch}},
		{name: "Neither", client: plainClient, want: codersdk.Experiments{}},
	} {
		got, err := tc.client.Experiments(ctx)
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.want, got, tc.name)
	}
}
