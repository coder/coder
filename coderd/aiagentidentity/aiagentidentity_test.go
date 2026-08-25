package aiagentidentity_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateAndMintKey(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: organization.ID,
		UserID:         owner.ID,
	})

	originID := uuid.New()
	agent, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     entity.CreationSiteTypeChat,
		OriginID:       originID,
	})
	require.NoError(t, err)
	// **An AI agent has no users row.** These assertions used to describe one,
	// checking the kind, login type and status the mirror was written with.
	// Nothing writes it now, so what is worth asserting is its absence.
	_, err = db.GetUserByID(ctx, agent.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "an AI agent is not a user")

	ledger, err := db.GetAIAgentLedgerRowByID(ctx, agent.ID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, ledger.OwnerID)
	require.Equal(t, originID, ledger.CreationSiteID)

	memberships, err := db.GetOrganizationIDsByMemberIDs(ctx, []uuid.UUID{agent.ID})
	require.NoError(t, err)
	require.Empty(t, memberships)

	profile := aiagentidentity.ChatAgentProfile(originID)
	key, token, err := aiagentidentity.MintKey(ctx, db, agent.ID, profile)
	require.NoError(t, err)
	require.True(t, aiagentidentity.APIKeyMatchesBuiltInProfile(key))
	require.NotEmpty(t, token)
	require.Equal(t, agent.ID, key.HolderID.AsUserIDUnchecked())
	require.Equal(t, database.LoginTypeToken, key.LoginType)
	require.NotEmpty(t, key.Scopes)
	require.NotEmpty(t, key.AllowList)
	require.False(t, key.Scopes.Has(database.ApiKeyScopeCoderApikeysmanageSelf))
	require.False(t, key.Scopes.Has(database.ApiKeyScopeCoderTemplatesauthor))

	resolved, err := aiagentidentity.Resolve(ctx, db, agent.ID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, resolved.Actor.OwnerUserID)
	require.Equal(t, agent.ID, resolved.Actor.AgentUserID)

	profileTests := []struct {
		name         string
		profile      aiagentidentity.Profile
		wantErr      string
		matchesShape bool
	}{
		{
			name:         "chat profile",
			profile:      aiagentidentity.ChatAgentProfile(uuid.New()),
			matchesShape: true,
		},
		{
			name:         "workspace profile",
			profile:      aiagentidentity.WorkspaceAgentIdentityProfile(uuid.New()),
			matchesShape: true,
		},
		{
			name:         "sandbox profile",
			profile:      aiagentidentity.SandboxIdentityProfile(uuid.New(), uuid.New()),
			matchesShape: true,
		},
		{
			name: "user read",
			profile: aiagentidentity.Profile{
				Scopes: database.APIKeyScopes{database.ApiKeyScopeUserRead},
				AllowList: database.AllowList{
					{Type: rbac.ResourceUser.Type, ID: policy.WildcardSymbol},
				},
			},
		},
		{
			name: "coder all",
			profile: aiagentidentity.Profile{
				Scopes: database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				AllowList: database.AllowList{
					{Type: rbac.ResourceWorkspace.Type, ID: originID.String()},
				},
			},
			wantErr: "forbidden",
		},
		{
			name: "application connect",
			profile: aiagentidentity.Profile{
				Scopes: database.APIKeyScopes{database.ApiKeyScopeCoderApplicationConnect},
				AllowList: database.AllowList{
					{Type: rbac.ResourceWorkspace.Type, ID: originID.String()},
				},
			},
			wantErr: "forbidden",
		},
		{
			name: "global allow list",
			profile: aiagentidentity.Profile{
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeWorkspaceRead},
				AllowList: database.AllowList{rbac.AllowListAll()},
			},
			wantErr: "every resource",
		},
	}
	for _, tt := range profileTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			key, _, err := aiagentidentity.MintKey(ctx, db, agent.ID, tt.profile)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.matchesShape, aiagentidentity.APIKeyMatchesBuiltInProfile(key))
			require.NotEmpty(t, key.Scopes)
			require.NotEmpty(t, key.AllowList)
		})
	}

	require.False(t, aiagentidentity.APIKeyMatchesBuiltInProfile(database.APIKey{
		Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
		AllowList: database.AllowList{rbac.AllowListAll()},
	}))

	_, _, err = aiagentidentity.MintKey(ctx, db, agent.ID, aiagentidentity.Profile{})
	require.ErrorContains(t, err, "at least one scope")

	_, _, err = aiagentidentity.MintKey(ctx, db, agent.ID, aiagentidentity.Profile{
		Scopes: database.APIKeyScopes{database.ApiKeyScopeApiKeyCreate},
		AllowList: database.AllowList{
			rbac.AllowListAll(),
		},
	})
	require.ErrorContains(t, err, "forbidden")

	// user:read is permitted (agents resolve their owner to act on the
	// owner's behalf), but personal/PII reads and user mutations are not.
	for _, forbidden := range []database.APIKeyScope{
		database.ApiKeyScopeUserReadPersonal,
		database.ApiKeyScopeUserUpdate,
		database.ApiKeyScopeUserSecretRead,
		database.ApiKeyScopeUserSkillRead,
	} {
		t.Run(string(forbidden), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			_, _, err := aiagentidentity.MintKey(ctx, db, agent.ID, aiagentidentity.Profile{
				Scopes:    database.APIKeyScopes{forbidden},
				AllowList: database.AllowList{rbac.AllowListAll()},
			})
			require.ErrorContains(t, err, "forbidden", "scope %q must be forbidden", forbidden)
		})
	}
}

func TestCreateRequiresOwnerOrganizationMembership(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})

	_, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     entity.CreationSiteTypeWorkspace,
		OriginID:       uuid.New(),
	})
	require.ErrorContains(t, err, "not a member")
}
