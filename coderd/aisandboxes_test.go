package coderd_test

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

type aiSandboxLifecycleFixture struct {
	client      *codersdk.Client
	db          database.Store
	owner       codersdk.CreateFirstUserResponse
	workspace   dbfake.WorkspaceResponse
	agentClient *agentsdk.Client
}

func newAISandboxLifecycleFixture(t *testing.T, options *coderdtest.Options) aiSandboxLifecycleFixture {
	t.Helper()

	client, db := coderdtest.NewWithDatabase(t, options)
	owner := coderdtest.CreateFirstUser(t, client)
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: owner.OrganizationID,
		OwnerID:        owner.UserID,
	}).WithAgent().Do()
	return aiSandboxLifecycleFixture{
		client:      client,
		db:          db,
		owner:       owner,
		workspace:   workspace,
		agentClient: agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken)),
	}
}

func bindAISandboxLifecycleParent(t *testing.T, fixture aiSandboxLifecycleFixture) database.AIAgent {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	agentUser, agent, err := aiagentidentity.Create(ctx, fixture.db, aiagentidentity.CreateParams{
		OwnerID:        fixture.owner.UserID,
		OrganizationID: fixture.owner.OrganizationID,
		OriginType:     database.AIAgentOriginChat,
		OriginID:       uuid.New(),
	})
	require.NoError(t, err)
	_, err = fixture.db.UpdateWorkspaceAgentAIAgentID(dbauthz.AsSystemRestricted(ctx), database.UpdateWorkspaceAgentAIAgentIDParams{
		ID:        fixture.workspace.Agents[0].ID,
		AIAgentID: uuid.NullUUID{UUID: agentUser.ID, Valid: true},
	})
	require.NoError(t, err)
	return agent
}

func requireAISandboxLifecycleStatus(t *testing.T, err error, status int) *codersdk.Error {
	t.Helper()

	require.Error(t, err)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, status, sdkErr.StatusCode())
	return sdkErr
}

func aiSandboxSessionTokenStatus(t *testing.T, db database.Store, token string) int {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(codersdk.SessionTokenHeader, token)
	rw := httptest.NewRecorder()
	httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB: db,
	})(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rw, r)
	return rw.Code
}

func TestAISandboxLifecycleCreateUnboundParent(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxLifecycleFixture(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	parent := fixture.workspace.Agents[0]

	created, err := fixture.agentClient.CreateAISandbox(ctx, agentsdk.CreateAISandboxRequest{
		Name:              "unbound-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.ID)
	require.NotEqual(t, uuid.Nil, created.ChildAgentID)
	require.NotEmpty(t, created.AgentToken)
	require.NotEmpty(t, created.SessionToken)
	require.False(t, created.Reconciled)

	workspaceIdentity, err := fixture.db.GetAIAgentByOrigin(dbauthz.AsSystemRestricted(ctx), database.GetAIAgentByOriginParams{
		OriginType: database.AIAgentOriginWorkspace,
		OriginID:   fixture.workspace.Workspace.ID,
	})
	require.NoError(t, err)
	require.Equal(t, workspaceIdentity.UserID, created.AIAgentID)

	child, err := fixture.db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), created.ChildAgentID)
	require.NoError(t, err)
	require.Equal(t, uuid.NullUUID{UUID: parent.ID, Valid: true}, child.ParentID)
	require.Equal(t, uuid.NullUUID{UUID: workspaceIdentity.UserID, Valid: true}, child.AIAgentID)

	childClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(created.AgentToken))
	_, err = childClient.AIEgressPolicy(ctx)
	require.NoError(t, err)
}

func TestAISandboxLifecycleCreateBoundParent(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxLifecycleFixture(t, nil)
	parentIdentity := bindAISandboxLifecycleParent(t, fixture)
	ctx := testutil.Context(t, testutil.WaitLong)

	created, err := fixture.agentClient.CreateAISandbox(ctx, agentsdk.CreateAISandboxRequest{
		Name:              "bound-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementAdvisory,
	})
	require.NoError(t, err)
	require.Equal(t, parentIdentity.UserID, created.AIAgentID)

	child, err := fixture.db.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), created.ChildAgentID)
	require.NoError(t, err)
	require.Equal(t, uuid.NullUUID{UUID: parentIdentity.UserID, Valid: true}, child.AIAgentID)

	_, err = fixture.db.GetAIAgentByOrigin(dbauthz.AsSystemRestricted(ctx), database.GetAIAgentByOriginParams{
		OriginType: database.AIAgentOriginWorkspace,
		OriginID:   fixture.workspace.Workspace.ID,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestAISandboxLifecycleReconcile(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxLifecycleFixture(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	request := agentsdk.CreateAISandboxRequest{
		Name:              "reconcile-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
	}

	first, err := fixture.agentClient.CreateAISandbox(ctx, request)
	require.NoError(t, err)
	profile := aiagentidentity.SandboxIdentityProfile(fixture.workspace.Workspace.ID, first.ID)
	firstKey, err := fixture.db.GetAPIKeyByName(dbauthz.AsSystemRestricted(ctx), database.GetAPIKeyByNameParams{
		UserID:    first.AIAgentID,
		TokenName: profile.TokenName,
	})
	require.NoError(t, err)

	second, err := fixture.agentClient.CreateAISandbox(ctx, request)
	require.NoError(t, err)
	require.True(t, second.Reconciled)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.ChildAgentID, second.ChildAgentID)
	require.Equal(t, first.AgentToken, second.AgentToken)
	require.NotEqual(t, first.SessionToken, second.SessionToken)

	sandboxes, err := fixture.db.GetAISandboxesByParentAgentID(dbauthz.AsSystemRestricted(ctx), fixture.workspace.Agents[0].ID)
	require.NoError(t, err)
	require.Len(t, sandboxes, 1)

	_, err = fixture.db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), firstKey.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	secondKey, err := fixture.db.GetAPIKeyByName(dbauthz.AsSystemRestricted(ctx), database.GetAPIKeyByNameParams{
		UserID:    second.AIAgentID,
		TokenName: profile.TokenName,
	})
	require.NoError(t, err)
	require.NotEqual(t, firstKey.ID, secondKey.ID)

	require.Equal(t, http.StatusUnauthorized, aiSandboxSessionTokenStatus(t, fixture.db, first.SessionToken))
	require.Equal(t, http.StatusNoContent, aiSandboxSessionTokenStatus(t, fixture.db, second.SessionToken))
}

func TestAISandboxLifecycleValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		sandboxName       string
		egressEnforcement codersdk.AISandboxEgressEnforcement
		validationField   string
	}{
		{
			name:              "EmptyName",
			sandboxName:       "",
			egressEnforcement: codersdk.AISandboxEgressEnforcementForced,
			validationField:   "name",
		},
		{
			name:              "SpaceInName",
			sandboxName:       "has space",
			egressEnforcement: codersdk.AISandboxEgressEnforcementForced,
			validationField:   "name",
		},
		{
			name:              "SlashInName",
			sandboxName:       "has/slash",
			egressEnforcement: codersdk.AISandboxEgressEnforcementForced,
			validationField:   "name",
		},
		{
			name:              "NameTooLong",
			sandboxName:       strings.Repeat("a", 65),
			egressEnforcement: codersdk.AISandboxEgressEnforcementForced,
			validationField:   "name",
		},
		{
			name:              "InvalidEgressEnforcement",
			sandboxName:       "valid-" + uuid.NewString(),
			egressEnforcement: codersdk.AISandboxEgressEnforcement("invalid"),
			validationField:   "egress_enforcement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fixture := newAISandboxLifecycleFixture(t, nil)
			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := fixture.agentClient.CreateAISandbox(ctx, agentsdk.CreateAISandboxRequest{
				Name:              tt.sandboxName,
				EgressEnforcement: tt.egressEnforcement,
			})
			sdkErr := requireAISandboxLifecycleStatus(t, err, http.StatusBadRequest)
			require.NotEmpty(t, sdkErr.Validations)
			require.Equal(t, tt.validationField, sdkErr.Validations[0].Field)
		})
	}
}

func TestAISandboxLifecycleNestingRefused(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxLifecycleFixture(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	created, err := fixture.agentClient.CreateAISandbox(ctx, agentsdk.CreateAISandboxRequest{
		Name:              "parent-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
	})
	require.NoError(t, err)

	childClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(created.AgentToken))
	_, err = childClient.CreateAISandbox(ctx, agentsdk.CreateAISandboxRequest{
		Name:              "nested-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
	})
	requireAISandboxLifecycleStatus(t, err, http.StatusForbidden)
}

func TestAISandboxLifecycleList(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxLifecycleFixture(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	request := agentsdk.CreateAISandboxRequest{
		Name:              "list-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementAdvisory,
	}
	created, err := fixture.agentClient.CreateAISandbox(ctx, request)
	require.NoError(t, err)

	listed, err := fixture.agentClient.AISandboxes(ctx)
	require.NoError(t, err)
	require.Equal(t, []agentsdk.AISandbox{{
		ID:                created.ID,
		ChildAgentID:      created.ChildAgentID,
		AIAgentID:         created.AIAgentID,
		Name:              request.Name,
		EgressEnforcement: request.EgressEnforcement,
	}}, listed)

	res, err := fixture.agentClient.SDK.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/ai-sandboxes", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "agent_token")
	require.NotContains(t, string(body), "session_token")

	otherWorkspace := dbfake.WorkspaceBuild(t, fixture.db, database.WorkspaceTable{
		OrganizationID: fixture.owner.OrganizationID,
		OwnerID:        fixture.owner.UserID,
	}).WithAgent().Do()
	otherClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(otherWorkspace.AgentToken))
	otherListed, err := otherClient.AISandboxes(ctx)
	require.NoError(t, err)
	require.Empty(t, otherListed)
}

func TestAISandboxLifecycleDelete(t *testing.T) {
	t.Parallel()

	fixture := newAISandboxLifecycleFixture(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	created, err := fixture.agentClient.CreateAISandbox(ctx, agentsdk.CreateAISandboxRequest{
		Name:              "delete-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementNone,
	})
	require.NoError(t, err)

	otherWorkspace := dbfake.WorkspaceBuild(t, fixture.db, database.WorkspaceTable{
		OrganizationID: fixture.owner.OrganizationID,
		OwnerID:        fixture.owner.UserID,
	}).WithAgent().Do()
	otherClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(otherWorkspace.AgentToken))
	err = otherClient.DeleteAISandbox(ctx, created.ID)
	requireAISandboxLifecycleStatus(t, err, http.StatusNotFound)

	err = fixture.agentClient.DeleteAISandbox(ctx, created.ID)
	require.NoError(t, err)

	childClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(created.AgentToken))
	_, err = childClient.AIEgressPolicy(ctx)
	requireAISandboxLifecycleStatus(t, err, http.StatusUnauthorized)

	profile := aiagentidentity.SandboxIdentityProfile(fixture.workspace.Workspace.ID, created.ID)
	_, err = fixture.db.GetAPIKeyByName(dbauthz.AsSystemRestricted(ctx), database.GetAPIKeyByNameParams{
		UserID:    created.AIAgentID,
		TokenName: profile.TokenName,
	})
	require.ErrorIs(t, err, sql.ErrNoRows)

	err = fixture.agentClient.DeleteAISandbox(ctx, created.ID)
	requireAISandboxLifecycleStatus(t, err, http.StatusNotFound)
}

func TestAISandboxLifecycleCredentialStarvation(t *testing.T) {
	t.Parallel()

	providerID := "sandbox-" + uuid.NewString()
	accessToken := uuid.NewString()
	secretValue := uuid.NewString()
	fixture := newAISandboxLifecycleFixture(t, &coderdtest.Options{
		ExternalAuthConfigs: []*externalauth.Config{
			fakeExternalAuthConfig(providerID, accessToken, regexp.MustCompile(`.*`)),
		},
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	secretSuffix := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	_, err := fixture.client.CreateUserSecret(ctx, codersdk.Me, codersdk.CreateUserSecretRequest{
		Name:    "sandbox-secret-" + uuid.NewString(),
		Value:   secretValue,
		EnvName: "SANDBOX_SECRET_" + secretSuffix,
	})
	require.NoError(t, err)
	dbgen.ExternalAuthLink(t, fixture.db, database.ExternalAuthLink{
		ProviderID:       providerID,
		UserID:           fixture.owner.UserID,
		OAuthAccessToken: accessToken,
		OAuthExpiry:      dbtime.Now().Add(24 * time.Hour),
	})

	created, err := fixture.agentClient.CreateAISandbox(ctx, agentsdk.CreateAISandboxRequest{
		Name:              "starved-" + uuid.NewString(),
		EgressEnforcement: codersdk.AISandboxEgressEnforcementForced,
	})
	require.NoError(t, err)
	childClient := agentsdk.New(fixture.client.URL, agentsdk.WithFixedToken(created.AgentToken))

	conn, err := childClient.ConnectRPC(ctx)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, conn.Close())
	}()
	manifest, err := agentproto.NewDRPCAgentClient(conn).GetManifest(ctx, &agentproto.GetManifestRequest{})
	require.NoError(t, err)
	require.Empty(t, agentsdk.SecretsFromProto(manifest.Secrets))

	_, err = childClient.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{ID: providerID})
	requireAISandboxLifecycleStatus(t, err, http.StatusForbidden)
	_, err = childClient.GitSSHKey(ctx)
	requireAISandboxLifecycleStatus(t, err, http.StatusForbidden)
}
