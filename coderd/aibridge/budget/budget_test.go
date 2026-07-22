package budget_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveUserAIBudget(t *testing.T) {
	t.Parallel()

	// budgetedGroup creates a regular group in the org, adds the user to it, and
	// sets a group AI budget. Returns the group ID.
	budgetedGroup := func(t *testing.T, ctx context.Context, db database.Store, orgID, userID uuid.UUID, groupName string, spendLimit int64) uuid.UUID {
		t.Helper()
		g := dbgen.Group(t, db, database.Group{OrganizationID: orgID, Name: groupName})
		dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: userID, GroupID: g.ID})
		_, err := db.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
			GroupID:          g.ID,
			SpendLimitMicros: spendLimit,
		})
		require.NoError(t, err)
		return g.ID
	}

	// budgetedEveryoneGroup creates the org's "Everyone" group (id == org id),
	// which is not auto-created for orgs built via dbgen, makes the user an org
	// member so membership flows through organization_members, and sets a group
	// AI budget. Returns the group ID.
	budgetedEveryoneGroup := func(t *testing.T, ctx context.Context, db database.Store, orgID, userID uuid.UUID, spendLimit int64) uuid.UUID {
		t.Helper()
		g := dbgen.Group(t, db, database.Group{ID: orgID, OrganizationID: orgID, Name: "Everyone"})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: orgID, UserID: userID})
		_, err := db.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
			GroupID:          g.ID,
			SpendLimitMicros: spendLimit,
		})
		require.NoError(t, err)
		return g.ID
	}

	tests := []struct {
		name    string
		policy  codersdk.AIBudgetPolicy
		setup   func(t *testing.T, ctx context.Context, db database.Store) (userID uuid.UUID, want budget.EffectiveGroup, wantOK bool)
		wantErr string
	}{
		{
			name:   "OverrideWins",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				// A higher group budget that the override must still beat.
				budgetedGroup(t, ctx, db, org.ID, user.ID, "rich-group", 9_000_000)
				// The override names a group the user must be a member of.
				og := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "override-group"})
				dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: user.ID, GroupID: og.ID})
				_, err := db.UpsertUserAIBudgetOverride(ctx, database.UpsertUserAIBudgetOverrideParams{
					UserID:           user.ID,
					GroupID:          og.ID,
					SpendLimitMicros: 1_000_000,
				})
				require.NoError(t, err)
				return user.ID, budget.EffectiveGroup{GroupID: og.ID, Limit: &budget.Limit{SpendLimitMicros: 1_000_000, Source: codersdk.AIBudgetLimitSourceUserOverride}}, true
			},
		},
		{
			name:   "SingleGroupBudget",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
				gid := budgetedGroup(t, ctx, db, org.ID, user.ID, "only", 8_000_000)
				return user.ID, budget.EffectiveGroup{GroupID: gid, Limit: &budget.Limit{SpendLimitMicros: 8_000_000, Source: codersdk.AIBudgetLimitSourceGroup}}, true
			},
		},
		{
			name:   "HighestGroupWins",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
				budgetedGroup(t, ctx, db, org.ID, user.ID, "low", 5_000_000)
				budgetedGroup(t, ctx, db, org.ID, user.ID, "mid", 20_000_000)
				high := budgetedGroup(t, ctx, db, org.ID, user.ID, "high", 50_000_000)
				return user.ID, budget.EffectiveGroup{GroupID: high, Limit: &budget.Limit{SpendLimitMicros: 50_000_000, Source: codersdk.AIBudgetLimitSourceGroup}}, true
			},
		},
		{
			name:   "TieBrokenByEarliestOrgMembership",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				user := dbgen.User(t, db, database.User{})
				// Two groups in different orgs share the same limit. The earlier
				// organization membership breaks the tie.
				earlyOrg := dbgen.Organization(t, db, database.Organization{})
				lateOrg := dbgen.Organization(t, db, database.Organization{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: earlyOrg.ID, UserID: user.ID, CreatedAt: time.Now().Add(-time.Hour)})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: lateOrg.ID, UserID: user.ID})
				winner := budgetedGroup(t, ctx, db, earlyOrg.ID, user.ID, "dup", 10_000_000)
				budgetedGroup(t, ctx, db, lateOrg.ID, user.ID, "dup", 10_000_000)
				return user.ID, budget.EffectiveGroup{GroupID: winner, Limit: &budget.Limit{SpendLimitMicros: 10_000_000, Source: codersdk.AIBudgetLimitSourceGroup}}, true
			},
		},
		{
			name:   "TieBrokenByGroupID",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
				// Both groups are in the same org, so both resolve to the same
				// organization membership and the tie falls to the lowest group ID.
				groupA := budgetedGroup(t, ctx, db, org.ID, user.ID, "alpha", 10_000_000)
				groupB := budgetedGroup(t, ctx, db, org.ID, user.ID, "beta", 10_000_000)
				winner := groupA
				if bytes.Compare(groupB[:], groupA[:]) < 0 {
					winner = groupB
				}
				return user.ID, budget.EffectiveGroup{GroupID: winner, Limit: &budget.Limit{SpendLimitMicros: 10_000_000, Source: codersdk.AIBudgetLimitSourceGroup}}, true
			},
		},
		{
			name:   "GroupsButNoneBudgeted",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				g := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "unbudgeted"})
				dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: user.ID, GroupID: g.ID})
				return user.ID, budget.EffectiveGroup{}, false
			},
		},
		{
			name:   "EveryoneGroupBudget",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				// Membership is via organization_members only (no group_members row),
				// exercising the org-members half of group_members_expanded.
				everyoneID := budgetedEveryoneGroup(t, ctx, db, org.ID, user.ID, 7_000_000)
				return user.ID, budget.EffectiveGroup{GroupID: everyoneID, Limit: &budget.Limit{SpendLimitMicros: 7_000_000, Source: codersdk.AIBudgetLimitSourceGroup}}, true
			},
		},
		{
			name:   "OverrideBeatsEveryoneBudget",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				everyoneID := budgetedEveryoneGroup(t, ctx, db, org.ID, user.ID, 7_000_000)
				// Override attributed to the Everyone group. The user is a member
				// via organization_members, satisfying the membership trigger.
				_, err := db.UpsertUserAIBudgetOverride(ctx, database.UpsertUserAIBudgetOverrideParams{
					UserID:           user.ID,
					GroupID:          everyoneID,
					SpendLimitMicros: 2_000_000,
				})
				require.NoError(t, err)
				return user.ID, budget.EffectiveGroup{GroupID: everyoneID, Limit: &budget.Limit{SpendLimitMicros: 2_000_000, Source: codersdk.AIBudgetLimitSourceUserOverride}}, true
			},
		},
		{
			name:   "UnsupportedPolicy",
			policy: codersdk.AIBudgetPolicy("unsupported"),
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				// No override, so resolution reaches the policy switch and errors.
				user := dbgen.User(t, db, database.User{})
				return user.ID, budget.EffectiveGroup{}, false
			},
			wantErr: "unsupported AI budget policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			userID, want, wantOK := tt.setup(t, ctx, db)
			got, ok, err := budget.ResolveUserAIBudget(ctx, db, userID, tt.policy)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, wantOK, ok)
			if !wantOK {
				return
			}
			require.Equal(t, want.GroupID, got.GroupID)
			require.Equal(t, want.Limit, got.Limit)
		})
	}
}

func TestResolveUserEffectiveGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		policy  codersdk.AIBudgetPolicy
		setup   func(t *testing.T, ctx context.Context, db database.Store) (userID uuid.UUID, want budget.EffectiveGroup, wantOK bool)
		wantErr string
	}{
		{
			// The Everyone group has a budget, so it resolves via the budget
			// path rather than the fallback.
			name:   "EveryoneGroupWithBudget",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				// The Everyone group's id equals the org id.
				group := dbgen.Group(t, db, database.Group{ID: org.ID, OrganizationID: org.ID, Name: "Everyone"})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
				_, err := db.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
					GroupID:          group.ID,
					SpendLimitMicros: 7_000_000,
				})
				require.NoError(t, err)
				return user.ID, budget.EffectiveGroup{GroupID: group.ID, Limit: &budget.Limit{SpendLimitMicros: 7_000_000, Source: codersdk.AIBudgetLimitSourceGroup}}, true
			},
		},
		{
			// With a single org and no budget, attribution falls back to that
			// org's Everyone group with no limit.
			name:   "FallbackToEveryoneUnlimited",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
				return user.ID, budget.EffectiveGroup{GroupID: org.ID}, true
			},
		},
		{
			// The fallback prefers the default org even over an org joined
			// earlier.
			name:   "FallbackPrefersDefaultOrg",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				defaultOrg, err := db.GetDefaultOrganization(ctx)
				require.NoError(t, err)
				otherOrg := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: otherOrg.ID, UserID: user.ID, CreatedAt: time.Now().Add(-time.Hour)})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: defaultOrg.ID, UserID: user.ID})
				return user.ID, budget.EffectiveGroup{GroupID: defaultOrg.ID}, true
			},
		},
		{
			// Among non-default orgs, the fallback breaks ties by the earliest
			// organization membership.
			name:   "FallbackTieByEarliestOrgMembership",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				user := dbgen.User(t, db, database.User{})
				earlyOrg := dbgen.Organization(t, db, database.Organization{})
				lateOrg := dbgen.Organization(t, db, database.Organization{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: earlyOrg.ID, UserID: user.ID, CreatedAt: time.Now().Add(-time.Hour)})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: lateOrg.ID, UserID: user.ID})
				return user.ID, budget.EffectiveGroup{GroupID: earlyOrg.ID}, true
			},
		},
		{
			// A user with no org membership has no effective group.
			name:   "NoOrgMembership",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				user := dbgen.User(t, db, database.User{})
				return user.ID, budget.EffectiveGroup{}, false
			},
		},
		{
			// An unsupported policy surfaces the error from ResolveUserAIBudget.
			name:   "UnsupportedPolicy",
			policy: codersdk.AIBudgetPolicy("unsupported"),
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveGroup, bool) {
				user := dbgen.User(t, db, database.User{})
				return user.ID, budget.EffectiveGroup{}, false
			},
			wantErr: "unsupported AI budget policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			userID, want, wantOK := tt.setup(t, ctx, db)
			got, ok, err := budget.ResolveUserEffectiveGroup(ctx, db, userID, tt.policy)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, wantOK, ok)
			if !wantOK {
				return
			}

			require.Equal(t, want.GroupID, got.GroupID)
			require.Equal(t, want.Limit, got.Limit)
		})
	}
}
