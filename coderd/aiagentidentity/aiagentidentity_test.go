package aiagentidentity_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
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
	agentUser, agent, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     database.AIAgentOriginChat,
		OriginID:       originID,
	})
	require.NoError(t, err)
	require.Equal(t, database.UserKindAIAgent, agentUser.Kind)
	require.Equal(t, database.LoginTypeNone, agentUser.LoginType)
	require.Empty(t, agentUser.Email)
	require.False(t, agentUser.IsServiceAccount)
	require.Equal(t, database.UserStatusActive, agentUser.Status)
	require.Equal(t, owner.ID, agent.OwnerUserID)
	require.Equal(t, originID, agent.OriginID)

	memberships, err := db.GetOrganizationIDsByMemberIDs(ctx, []uuid.UUID{agentUser.ID})
	require.NoError(t, err)
	require.Empty(t, memberships)

	profile := aiagentidentity.ChatAgentProfile(originID)
	key, token, err := aiagentidentity.MintKey(ctx, db, agentUser.ID, profile)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, agentUser.ID, key.UserID)
	require.Equal(t, database.LoginTypeToken, key.LoginType)
	require.NotEmpty(t, key.Scopes)
	require.NotEmpty(t, key.AllowList)
	require.False(t, key.Scopes.Has(database.ApiKeyScopeCoderApikeysmanageSelf))
	require.False(t, key.Scopes.Has(database.ApiKeyScopeCoderTemplatesauthor))

	resolved, err := aiagentidentity.Resolve(ctx, db, agentUser.ID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, resolved.Actor.OwnerUserID)
	require.Equal(t, agentUser.ID, resolved.Actor.AgentUserID)

	_, _, err = aiagentidentity.MintKey(ctx, db, agentUser.ID, aiagentidentity.Profile{})
	require.ErrorContains(t, err, "at least one scope")

	_, _, err = aiagentidentity.MintKey(ctx, db, agentUser.ID, aiagentidentity.Profile{
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
		_, _, err = aiagentidentity.MintKey(ctx, db, agentUser.ID, aiagentidentity.Profile{
			Scopes:    database.APIKeyScopes{forbidden},
			AllowList: database.AllowList{rbac.AllowListAll()},
		})
		require.ErrorContains(t, err, "forbidden", "scope %q must be forbidden", forbidden)
	}
	okKey, _, err := aiagentidentity.MintKey(ctx, db, agentUser.ID, aiagentidentity.Profile{
		Scopes:    database.APIKeyScopes{database.ApiKeyScopeUserRead},
		AllowList: database.AllowList{rbac.AllowListAll()},
	})
	require.NoError(t, err)
	require.True(t, okKey.Scopes.Has(database.ApiKeyScopeUserRead))
}

func TestCreateRequiresOwnerOrganizationMembership(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})

	_, _, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     database.AIAgentOriginWorkspace,
		OriginID:       uuid.New(),
	})
	require.ErrorContains(t, err, "not a member")
}
