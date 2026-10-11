package coderd_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentHoursUsageFeatureGate(t *testing.T) {
	t.Parallel()

	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{FeatureSet: codersdk.FeatureSetPremium},
	})
	ctx := testutil.Context(t, testutil.WaitLong)

	//nolint:gocritic // The feature gate must reject even the owner.
	_, err := client.AgentHoursUsage(ctx)
	requireAgentHoursStatus(t, err, http.StatusForbidden)
	_, err = client.OrganizationAgentHoursUsage(ctx, owner.OrganizationID)
	requireAgentHoursStatus(t, err, http.StatusForbidden)
	_, err = client.GroupMembersAgentHoursUsage(ctx, owner.OrganizationID, []uuid.UUID{owner.UserID})
	requireAgentHoursStatus(t, err, http.StatusForbidden)
}

func TestAgentHoursUsage(t *testing.T) {
	t.Parallel()

	license := agentHoursLicense(1000)
	// Start the period two days back, off the hour, so buckets can start
	// both before and inside it.
	license.NotBefore = time.Now().Add(-48*time.Hour - 30*time.Minute)
	client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{LicenseOptions: license})
	ctx := testutil.Context(t, testutil.WaitLong)
	org := owner.OrganizationID
	//nolint:gocritic // Setup only.
	defaultOrg, err := client.Organization(ctx, org)
	require.NoError(t, err)

	auditorClient, _ := coderdtest.CreateAnotherUser(t, client, org, rbac.RoleAuditor())
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, org, rbac.ScopedRoleOrgAdmin(org))
	memberClient, member := coderdtest.CreateAnotherUser(t, client, org)
	_, other := coderdtest.CreateAnotherUser(t, client, org)
	otherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	_, outsider := coderdtest.CreateAnotherUser(t, client, otherOrg.ID)
	deletedOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})
	//nolint:gocritic // Setup only.
	require.NoError(t, client.DeleteOrganization(ctx, deletedOrg.ID.String()))

	//nolint:gocritic // Setup only.
	alpha, err := client.CreateGroup(ctx, org, codersdk.CreateGroupRequest{Name: "alpha"})
	require.NoError(t, err)
	_, err = client.PatchGroup(ctx, alpha.ID, codersdk.PatchGroupRequest{AddUsers: []string{member.ID.String()}})
	require.NoError(t, err)
	_, err = client.UpsertAgentHoursGroupAllotment(ctx, alpha.ID, allotmentReq(5000))
	require.NoError(t, err)

	entitlements, err := client.Entitlements(ctx)
	require.NoError(t, err)
	period := *entitlements.Features[codersdk.FeatureAgentRuntimeHours].UsagePeriod
	var (
		// Starts before the period, so it counts toward the previous one.
		before = period.Start.Truncate(time.Hour)
		inside = before.Add(time.Hour)
		// Starts after the period ends.
		after = period.End.Add(time.Hour).Truncate(time.Hour)
	)
	require.True(t, before.Before(period.Start))

	deletedGroupID := uuid.New()
	usagePublisherCtx := dbauthz.AsUsagePublisher(ctx)
	insertBucket := func(bucket time.Time, totalMs int64, rows ...database.GetAgentRuntimeHourlyUsageRow) {
		t.Helper()
		_, err := db.InsertUsageEvent(usagePublisherCtx, database.InsertUsageEventParams{
			ID:        "hb_agent_runtime_v1:" + bucket.Format("2006-01-02_15:04:05"),
			EventType: "hb_agent_runtime_v1",
			EventData: []byte(fmt.Sprintf(`{"runtime_ms": %d}`, totalMs)),
			CreatedAt: bucket,
		})
		require.NoError(t, err)
		params := database.InsertAgentRuntimeHourlyUsageParams{BucketStart: bucket}
		for _, row := range rows {
			params.OrganizationIds = append(params.OrganizationIds, row.OrganizationID)
			params.GroupIds = append(params.GroupIds, row.GroupID)
			params.UserIds = append(params.UserIds, row.UserID)
			params.RuntimeMs = append(params.RuntimeMs, row.RuntimeMs)
		}
		require.NoError(t, db.InsertAgentRuntimeHourlyUsage(usagePublisherCtx, params))
	}
	insertBucket(before, 1000,
		database.GetAgentRuntimeHourlyUsageRow{OrganizationID: org, GroupID: org, UserID: other.ID, RuntimeMs: 1000},
	)
	// One second of the bucket ran in chats deleted before the rollup
	// existed, so no organization holds it.
	insertBucket(inside, 4_901_000,
		database.GetAgentRuntimeHourlyUsageRow{OrganizationID: org, GroupID: alpha.ID, UserID: member.ID, RuntimeMs: 3_600_000},
		database.GetAgentRuntimeHourlyUsageRow{OrganizationID: org, GroupID: org, UserID: other.ID, RuntimeMs: 600_000},
		database.GetAgentRuntimeHourlyUsageRow{OrganizationID: org, GroupID: deletedGroupID, UserID: member.ID, RuntimeMs: 300_000},
		database.GetAgentRuntimeHourlyUsageRow{OrganizationID: otherOrg.ID, GroupID: otherOrg.ID, UserID: outsider.ID, RuntimeMs: 300_000},
		database.GetAgentRuntimeHourlyUsageRow{OrganizationID: deletedOrg.ID, GroupID: deletedOrg.ID, UserID: outsider.ID, RuntimeMs: 100_000},
	)
	insertBucket(after, 2000,
		database.GetAgentRuntimeHourlyUsageRow{OrganizationID: org, GroupID: alpha.ID, UserID: member.ID, RuntimeMs: 2000},
	)

	t.Run("Deployment", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		for _, c := range []*codersdk.Client{client, auditorClient} {
			usage, err := c.AgentHoursUsage(ctx)
			require.NoError(t, err)
			require.True(t, period.Start.Equal(usage.UsagePeriod.Start))
			require.EqualValues(t, 4_901_000, usage.TotalMs)
			require.ElementsMatch(t, []codersdk.AgentHoursOrganizationUsage{
				{OrganizationID: org, OrganizationName: defaultOrg.Name, OrganizationDisplayName: defaultOrg.DisplayName, UsedMs: 4_500_000},
				{OrganizationID: otherOrg.ID, OrganizationName: otherOrg.Name, OrganizationDisplayName: otherOrg.DisplayName, UsedMs: 300_000},
				// Organizations are soft-deleted; a deleted one keeps its
				// usage but loses its name.
				{OrganizationID: deletedOrg.ID, UsedMs: 100_000},
			}, usage.Organizations)
		}

		for _, c := range []*codersdk.Client{orgAdminClient, memberClient} {
			_, err := c.AgentHoursUsage(ctx)
			requireAgentHoursStatus(t, err, http.StatusForbidden)
		}
	})

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		for _, c := range []*codersdk.Client{client, auditorClient, orgAdminClient} {
			usage, err := c.OrganizationAgentHoursUsage(ctx, org)
			require.NoError(t, err)
			require.EqualValues(t, 4_500_000, usage.UsedMs)
			require.ElementsMatch(t, []codersdk.AgentHoursGroupUsage{
				{GroupID: alpha.ID, GroupName: "alpha", GroupDisplayName: "", UsedMs: 3_600_000},
				// The Everyone group holds the hours of members without an
				// allotted group.
				{GroupID: org, GroupName: database.EveryoneGroup, GroupDisplayName: "", UsedMs: 600_000},
				{GroupID: deletedGroupID, UsedMs: 300_000},
			}, usage.Groups)
		}

		_, err := orgAdminClient.OrganizationAgentHoursUsage(ctx, otherOrg.ID)
		requireAgentHoursStatus(t, err, http.StatusNotFound)
	})

	t.Run("GroupMembers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		alphaGroup := codersdk.AgentHoursEffectiveGroup{ID: alpha.ID, Name: "alpha"}
		everyone := codersdk.AgentHoursEffectiveGroup{ID: org, Name: database.EveryoneGroup}

		for _, c := range []*codersdk.Client{client, orgAdminClient} {
			// Non-members are omitted.
			usage, err := c.GroupMembersAgentHoursUsage(ctx, alpha.ID, []uuid.UUID{member.ID, other.ID, outsider.ID})
			require.NoError(t, err)
			require.Equal(t, []codersdk.AgentHoursGroupMemberUsage{
				{UserID: member.ID, UsedMs: 3_600_000, EffectiveGroup: alphaGroup},
			}, usage.Members)

			// Everyone counts every organization member, and reports only
			// the hours that counted toward Everyone.
			usage, err = c.GroupMembersAgentHoursUsage(ctx, org, []uuid.UUID{member.ID, other.ID})
			require.NoError(t, err)
			require.ElementsMatch(t, []codersdk.AgentHoursGroupMemberUsage{
				{UserID: member.ID, UsedMs: 0, EffectiveGroup: alphaGroup},
				{UserID: other.ID, UsedMs: 600_000, EffectiveGroup: everyone},
			}, usage.Members)
		}

		// A member reads only their own row.
		usage, err := memberClient.GroupMembersAgentHoursUsage(ctx, org, []uuid.UUID{member.ID, other.ID})
		require.NoError(t, err)
		require.Equal(t, []codersdk.AgentHoursGroupMemberUsage{
			{UserID: member.ID, UsedMs: 0, EffectiveGroup: alphaGroup},
		}, usage.Members)

		tooMany := make([]uuid.UUID, 101)
		for i := range tooMany {
			tooMany[i] = uuid.New()
		}
		_, err = client.GroupMembersAgentHoursUsage(ctx, alpha.ID, tooMany)
		requireAgentHoursStatus(t, err, http.StatusBadRequest)
	})
}
