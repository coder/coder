package budget_test

import (
	"bytes"
	"context"
	"testing"

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
		setup   func(t *testing.T, ctx context.Context, db database.Store) (userID uuid.UUID, want budget.EffectiveBudget, wantOK bool)
		wantErr string
	}{
		{
			name:   "OverrideWins",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				// A higher group budget that the override must still beat.
				budgetedGroup(t, ctx, db, org.ID, user.ID, "rich-group", 9_000_000)
				// The override names its own group; the user must be a member.
				og := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "override-group"})
				dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: user.ID, GroupID: og.ID})
				_, err := db.UpsertUserAIBudgetOverride(ctx, database.UpsertUserAIBudgetOverrideParams{
					UserID:           user.ID,
					GroupID:          og.ID,
					SpendLimitMicros: 1_000_000,
				})
				require.NoError(t, err)
				return user.ID, budget.EffectiveBudget{GroupID: og.ID, SpendLimitMicros: 1_000_000, Source: codersdk.AIBudgetLimitSourceUserOverride}, true
			},
		},
		{
			name:   "SingleGroupBudget",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				gid := budgetedGroup(t, ctx, db, org.ID, user.ID, "only", 8_000_000)
				return user.ID, budget.EffectiveBudget{GroupID: gid, SpendLimitMicros: 8_000_000, Source: codersdk.AIBudgetLimitSourceGroup}, true
			},
		},
		{
			name:   "HighestGroupWins",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				budgetedGroup(t, ctx, db, org.ID, user.ID, "low", 5_000_000)
				budgetedGroup(t, ctx, db, org.ID, user.ID, "mid", 20_000_000)
				high := budgetedGroup(t, ctx, db, org.ID, user.ID, "high", 50_000_000)
				return user.ID, budget.EffectiveBudget{GroupID: high, SpendLimitMicros: 50_000_000, Source: codersdk.AIBudgetLimitSourceGroup}, true
			},
		},
		{
			name:   "TieBrokenByName",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				// Equal limits; "alpha" must win over "beta" by name ascending.
				alpha := budgetedGroup(t, ctx, db, org.ID, user.ID, "alpha", 10_000_000)
				budgetedGroup(t, ctx, db, org.ID, user.ID, "beta", 10_000_000)
				return user.ID, budget.EffectiveBudget{GroupID: alpha, SpendLimitMicros: 10_000_000, Source: codersdk.AIBudgetLimitSourceGroup}, true
			},
		},
		{
			name:   "TieBrokenByGroupID",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				user := dbgen.User(t, db, database.User{})
				// Two groups in different orgs share both name and limit.
				// Group id breaks the tie, so resolution is deterministic.
				org1 := dbgen.Organization(t, db, database.Organization{})
				org2 := dbgen.Organization(t, db, database.Organization{})
				g1 := budgetedGroup(t, ctx, db, org1.ID, user.ID, "dup", 10_000_000)
				g2 := budgetedGroup(t, ctx, db, org2.ID, user.ID, "dup", 10_000_000)
				winner := g1
				if bytes.Compare(g2[:], g1[:]) < 0 {
					winner = g2
				}
				return user.ID, budget.EffectiveBudget{GroupID: winner, SpendLimitMicros: 10_000_000, Source: codersdk.AIBudgetLimitSourceGroup}, true
			},
		},
		{
			name:   "GroupsButNoneBudgeted",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				g := dbgen.Group(t, db, database.Group{OrganizationID: org.ID, Name: "unbudgeted"})
				dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: user.ID, GroupID: g.ID})
				return user.ID, budget.EffectiveBudget{}, false
			},
		},
		{
			name:   "EveryoneGroupBudget",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				// Membership is via organization_members only (no group_members row),
				// exercising the org-members half of group_members_expanded.
				everyoneID := budgetedEveryoneGroup(t, ctx, db, org.ID, user.ID, 7_000_000)
				return user.ID, budget.EffectiveBudget{GroupID: everyoneID, SpendLimitMicros: 7_000_000, Source: codersdk.AIBudgetLimitSourceGroup}, true
			},
		},
		{
			name:   "OverrideBeatsEveryoneBudget",
			policy: codersdk.AIBudgetPolicyHighest,
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				org := dbgen.Organization(t, db, database.Organization{})
				user := dbgen.User(t, db, database.User{})
				everyoneID := budgetedEveryoneGroup(t, ctx, db, org.ID, user.ID, 7_000_000)
				// Override attributed to the Everyone group; the user is a member
				// via organization_members, satisfying the membership trigger.
				_, err := db.UpsertUserAIBudgetOverride(ctx, database.UpsertUserAIBudgetOverrideParams{
					UserID:           user.ID,
					GroupID:          everyoneID,
					SpendLimitMicros: 2_000_000,
				})
				require.NoError(t, err)
				return user.ID, budget.EffectiveBudget{GroupID: everyoneID, SpendLimitMicros: 2_000_000, Source: codersdk.AIBudgetLimitSourceUserOverride}, true
			},
		},
		{
			name:   "UnsupportedPolicy",
			policy: codersdk.AIBudgetPolicy("unsupported"),
			setup: func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, budget.EffectiveBudget, bool) {
				// No override, so resolution reaches the policy switch and errors.
				user := dbgen.User(t, db, database.User{})
				return user.ID, budget.EffectiveBudget{}, false
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
			require.Equal(t, want.SpendLimitMicros, got.SpendLimitMicros)
			require.Equal(t, want.Source, got.Source)
		})
	}
}
