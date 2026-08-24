package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type aiAgentAuthFixture struct {
	db        database.Store
	owner     database.User
	agentUser database.User
	token     string
}

func newAIAgentAuthFixture(t *testing.T, ownerRoles []string) aiAgentAuthFixture {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{RBACRoles: ownerRoles})
	organization := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: organization.ID,
		UserID:         owner.ID,
	})
	originID := uuid.New()
	agentUser, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     entity.CreationSiteTypeChat,
		OriginID:       originID,
	})
	require.NoError(t, err)
	_, token, err := aiagentidentity.MintKey(ctx, db, agentUser.ID, aiagentidentity.ChatAgentProfile(originID))
	require.NoError(t, err)
	return aiAgentAuthFixture{
		db:        db,
		owner:     owner,
		agentUser: agentUser,
		token:     token,
	}
}

func serveAIAgentKey(t *testing.T, db database.Store, token string, handler http.Handler) int {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(codersdk.SessionTokenHeader, token)
	rw := httptest.NewRecorder()
	httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB: db,
	})(handler).ServeHTTP(rw, r)
	return rw.Code
}

func TestAIAgentPermissionCeiling(t *testing.T) {
	t.Parallel()

	fixture := newAIAgentAuthFixture(t, []string{rbac.RoleOwner().String()})
	ctx := testutil.Context(t, testutil.WaitShort)

	var firstSubject rbac.Subject
	status := serveAIAgentKey(t, fixture.db, fixture.token, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		firstSubject = httpmw.UserAuthorization(r.Context())
		actor, ok := aiagentidentity.ActorFromContext(r.Context())
		require.True(t, ok)
		require.Equal(t, fixture.agentUser.ID, actor.AgentUserID)
		require.Equal(t, fixture.owner.ID, actor.OwnerUserID)
		rw.WriteHeader(http.StatusNoContent)
	}))
	require.Equal(t, http.StatusNoContent, status)
	require.Equal(t, fixture.owner.ID.String(), firstSubject.ID)
	require.Equal(t, rbac.SubjectTypeAIAgent, firstSubject.Type)
	require.Equal(t, fixture.agentUser.Username, firstSubject.FriendlyName)

	authorizer := rbac.NewAuthorizer(prometheus.NewRegistry())
	template := rbac.ResourceTemplate.WithID(uuid.New()).InOrg(uuid.New())
	ownerSubject, _, err := httpmw.UserRBACSubject(ctx, fixture.db, fixture.owner.ID, rbac.ScopeAll)
	require.NoError(t, err)
	require.NoError(t, authorizer.Authorize(ctx, ownerSubject, policy.ActionDelete, template))
	require.Error(t, authorizer.Authorize(ctx, firstSubject, policy.ActionDelete, template))

	apiKeyObject := rbac.ResourceApiKey.WithOwner(fixture.owner.ID.String())
	require.Error(t, authorizer.Authorize(ctx, firstSubject, policy.ActionCreate, apiKeyObject))

	_, err = fixture.db.UpdateUserRoles(ctx, database.UpdateUserRolesParams{
		ID:           fixture.owner.ID,
		GrantedRoles: []string{},
	})
	require.NoError(t, err)

	var secondSubject rbac.Subject
	status = serveAIAgentKey(t, fixture.db, fixture.token, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		secondSubject = httpmw.UserAuthorization(r.Context())
		rw.WriteHeader(http.StatusNoContent)
	}))
	require.Equal(t, http.StatusNoContent, status)
	roles, err := secondSubject.Roles.Expand()
	require.NoError(t, err)
	require.NotContains(t, roleNames(roles), rbac.RoleOwner().String())
}

func TestAIAgentOwnerAndIdentityLiveness(t *testing.T) {
	t.Parallel()

	t.Run("OwnerSuspended", func(t *testing.T) {
		t.Parallel()
		fixture := newAIAgentAuthFixture(t, nil)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := fixture.db.UpdateUserStatus(ctx, database.UpdateUserStatusParams{
			ID:         fixture.owner.ID,
			Status:     database.UserStatusSuspended,
			UpdatedAt:  dbtime.Now(),
			UserIsSeen: false,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, serveAIAgentKey(t, fixture.db, fixture.token, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("suspended owner request reached handler")
		})))
	})

	t.Run("OwnerDeleted", func(t *testing.T) {
		t.Parallel()
		fixture := newAIAgentAuthFixture(t, nil)
		ctx := testutil.Context(t, testutil.WaitShort)
		require.NoError(t, fixture.db.UpdateUserDeletedByID(ctx, fixture.owner.ID))
		require.Equal(t, http.StatusUnauthorized, serveAIAgentKey(t, fixture.db, fixture.token, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("deleted owner request reached handler")
		})))
	})

	t.Run("IdentityDeleted", func(t *testing.T) {
		t.Parallel()
		fixture := newAIAgentAuthFixture(t, nil)
		ctx := testutil.Context(t, testutil.WaitShort)
		// Revocation retires the agent in the ledger, which is what
		// resolution reads.
		require.NoError(t, entity.RetireAIAgent(ctx, fixture.db, fixture.agentUser.ID,
			entity.EventAIAgentKill, entity.Ref{Type: entity.TypeUser, ID: fixture.owner.ID},
			dbtime.Now()))
		require.Equal(t, http.StatusUnauthorized, serveAIAgentKey(t, fixture.db, fixture.token, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("deleted identity request reached handler")
		})))
	})
}

func TestAIAgentMissingMetadataFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	agentUser, err := db.InsertAIAgentUser(ctx, database.InsertAIAgentUserParams{
		ID:        uuid.New(),
		Username:  "ai-chat-" + uuid.NewString()[:8],
		CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)

	keyParams, token, err := apikey.Generate(apikey.CreateParams{
		UserID:          agentUser.ID,
		LoginType:       database.LoginTypeToken,
		DefaultLifetime: time.Hour,
		Scopes:          database.APIKeyScopes{database.ApiKeyScopeChatRead},
		AllowList: database.AllowList{
			{Type: rbac.ResourceChat.Type, ID: uuid.NewString()},
		},
	})
	require.NoError(t, err)
	_, err = db.InsertAPIKey(ctx, keyParams)
	require.NoError(t, err)

	require.Equal(t, http.StatusUnauthorized, serveAIAgentKey(t, db, token, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("AI agent without metadata reached handler")
	})))
}

func roleNames(roles []rbac.Role) []string {
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role.Identifier.String())
	}
	return names
}
