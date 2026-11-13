package coderd_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// identityClient returns a client that acts with a credential of a delegated
// identity. The credential itself is not narrowed: what it is worth is read
// from the identity on every request.
func identityClient(t *testing.T, db database.Store, client *codersdk.Client, identityID uuid.UUID) *codersdk.Client {
	t.Helper()
	_, token := dbgen.APIKey(t, db, database.APIKey{UserID: identityID, LoginType: database.LoginTypeToken})
	identity := codersdk.New(client.URL)
	identity.SetSessionToken(token)
	return identity
}

// TestDelegatedIdentities covers a delegated identity from its creation to
// its deletion: who may create and change it, what it may do, and how a
// workspace comes to be owned by it.
func TestDelegatedIdentities(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentDelegatedIdentities)}
	auditor := audit.NewMock()
	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues:         dv,
		Auditor:                  auditor,
		IncludeProvisionerDaemon: true,
	})
	admin := coderdtest.CreateFirstUser(t, adminClient)
	aliceClient, alice := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	bobClient, bob := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, adminClient, admin.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
	template := coderdtest.CreateTemplate(t, adminClient, admin.OrganizationID, version.ID)

	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// The template is open to a group that Alice is in and to nobody else:
	// not to all members, and so not to any identity's own account.
	group := dbgen.Group(t, db, database.Group{OrganizationID: admin.OrganizationID})
	dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: alice.ID, GroupID: group.ID})
	adminUser, err := adminClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	require.NoError(t, db.UpdateTemplateACLByID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(adminUser)), database.UpdateTemplateACLByIDParams{
		ID:       template.ID,
		GroupACL: database.TemplateACL{group.ID.String(): {policy.ActionRead, policy.ActionUse}},
		UserACL:  database.TemplateACL{},
	}))

	// A workspace of Alice's own, which her identities must not reach.
	aliceBuild := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: admin.OrganizationID,
		OwnerID:        alice.ID,
	}).WithAgent().Do()

	// Creating an identity returns a record. It starts with nothing.
	identity, err := aliceClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{Name: "agent-1"})
	require.NoError(t, err)
	require.Equal(t, "agent-1", identity.Name)
	require.Equal(t, alice.ID, identity.ParentID)
	require.Equal(t, alice.ID, identity.RootUserID)
	require.Equal(t, alice.ID, identity.CreatedBy)
	require.Equal(t, alice.Username, identity.RootUsername)
	require.Equal(t, admin.OrganizationID, identity.OrganizationID)
	require.Empty(t, identity.Scopes)
	require.Empty(t, identity.EffectiveScopes)
	require.True(t, auditor.Contains(t, database.AuditLog{
		Action:       database.AuditActionCreate,
		ResourceType: database.ResourceTypeDelegatedIdentity,
		ResourceID:   identity.ID,
		UserID:       alice.ID,
	}))

	_, err = aliceClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{Name: "agent-1"})
	requireStatus(t, err, http.StatusConflict)
	_, err = aliceClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{
		Scopes: []codersdk.APIKeyScope{"not:a-scope"},
	})
	requireStatus(t, err, http.StatusBadRequest)

	// The identity is Alice's. Bob can neither see it nor add to what is
	// below Alice.
	_, err = bobClient.DelegatedIdentity(ctx, identity.ID.String())
	requireStatus(t, err, http.StatusNotFound)
	_, err = bobClient.UpdateDelegatedIdentity(ctx, "agent-1", codersdk.UpdateDelegatedIdentityRequest{})
	requireStatus(t, err, http.StatusNotFound)
	_, err = bobClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{ParentID: alice.ID})
	requireStatus(t, err, http.StatusForbidden)
	_, err = bobClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{ParentID: identity.ID})
	requireStatus(t, err, http.StatusForbidden)
	bobIdentities, err := bobClient.DelegatedIdentities(ctx)
	require.NoError(t, err)
	require.Empty(t, bobIdentities)

	// Whoever may create identities across the organization can create
	// one under a member, as a trusted service does.
	serviceMade, err := adminClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{
		Name:     "relay-session",
		ParentID: alice.ID,
	})
	require.NoError(t, err)
	require.Equal(t, alice.ID, serviceMade.ParentID)
	require.Equal(t, admin.UserID, serviceMade.CreatedBy)
	listed, err := aliceClient.DelegatedIdentities(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 2, "an identity a service created under Alice is Alice's")
	require.NoError(t, aliceClient.DeleteDelegatedIdentity(ctx, "relay-session"))

	// Nobody but an account above the identity, or one with permission
	// across the organization, creates workspaces on its behalf.
	createRequest := codersdk.CreateWorkspaceRequest{TemplateID: template.ID, Name: "task"}
	_, err = bobClient.CreateUserWorkspace(ctx, identity.ID.String(), createRequest)
	require.Error(t, err)
	bobIdentity, err := bobClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{Name: "bobs-agent"})
	require.NoError(t, err)
	// Nothing is created on behalf of an identity that its creator could
	// not create for itself: Bob may not use the template.
	_, err = bobClient.CreateUserWorkspace(ctx, bobIdentity.ID.String(), createRequest)
	require.Error(t, err)

	// Alice creates a workspace on behalf of her identity. The identity
	// owns it, and Alice is who asked. The identity's account is in no
	// group: it may use the template because its root user may.
	workspace, err := aliceClient.CreateUserWorkspace(ctx, identity.Name, createRequest)
	require.NoError(t, err)
	require.Equal(t, identity.ID, workspace.OwnerID)
	require.Equal(t, identity.Name, workspace.OwnerName)
	require.Equal(t, alice.ID, workspace.LatestBuild.InitiatorID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, workspace.LatestBuild.ID)
	require.True(t, auditor.Contains(t, database.AuditLog{
		Action:       database.AuditActionCreate,
		ResourceType: database.ResourceTypeWorkspace,
		ResourceID:   workspace.ID,
		UserID:       alice.ID,
	}))

	// The workspace daemon works for an identity that may do nothing with
	// the API: what a daemon needs does not come out of the identity's
	// scopes.
	_ = agenttest.New(t, adminClient.URL, agentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, adminClient, workspace.ID).Wait()

	// The workspace is shared with Alice. That goes one way.
	shared, err := aliceClient.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, shared.ID)
	_, err = bobClient.Workspace(ctx, workspace.ID)
	requireStatus(t, err, http.StatusNotFound)

	// With no scopes, the identity's credential is worth nothing.
	asIdentity := identityClient(t, db, adminClient, identity.ID)
	_, err = asIdentity.Workspace(ctx, workspace.ID)
	requireStatus(t, err, http.StatusNotFound)
	_, err = asIdentity.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{TemplateID: template.ID, Name: "own"})
	require.Error(t, err)
	_, err = asIdentity.UpdateDelegatedIdentity(ctx, codersdk.DelegatedIdentityMe, codersdk.UpdateDelegatedIdentityRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeAll},
	})
	require.Error(t, err, "an identity cannot raise itself")

	// Alice raises it. The credential it already has is worth more from
	// the next request on.
	updated, err := aliceClient.UpdateDelegatedIdentity(ctx, identity.Name, codersdk.UpdateDelegatedIdentityRequest{
		Scopes: []codersdk.APIKeyScope{
			codersdk.APIKeyScopeWorkspaceRead,
			codersdk.APIKeyScopeTemplateRead,
			codersdk.APIKeyScopeDelegatedIdentityCreate,
			codersdk.APIKeyScopeDelegatedIdentityRead,
		},
	})
	require.NoError(t, err)
	require.Len(t, updated.EffectiveScopes, 4)
	own, err := asIdentity.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, own.ID)
	self, err := asIdentity.DelegatedIdentity(ctx, codersdk.DelegatedIdentityMe)
	require.NoError(t, err)
	require.Equal(t, identity.ID, self.ID)
	// It acts as itself, with the roles of its root user. What Alice owns
	// is not the identity's.
	_, err = asIdentity.Workspace(ctx, aliceBuild.Workspace.ID)
	requireStatus(t, err, http.StatusNotFound)
	// Reading is all it may do with its workspace.
	_, err = asIdentity.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
	})
	require.Error(t, err)
	_, err = asIdentity.UpdateDelegatedIdentity(ctx, codersdk.DelegatedIdentityMe, codersdk.UpdateDelegatedIdentityRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeAll},
	})
	require.Error(t, err, "an identity cannot raise itself")

	// The identity creates an identity below itself. It can hand on no
	// more than it holds.
	_, err = asIdentity.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{
		Name:   "sub-agent",
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceDelete},
	})
	requireStatus(t, err, http.StatusBadRequest)
	child, err := asIdentity.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{
		Name:   "sub-agent",
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead},
	})
	require.NoError(t, err)
	require.Equal(t, identity.ID, child.ParentID)
	require.Equal(t, identity.ID, child.CreatedBy)
	require.Equal(t, alice.ID, child.RootUserID)
	require.Equal(t, []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead}, child.EffectiveScopes)

	// Alice is above the child as well, and holds it as her own.
	listed, err = aliceClient.DelegatedIdentities(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	// Lowering the parent lowers the child at once, with nothing changed
	// on the child.
	_, err = aliceClient.UpdateDelegatedIdentity(ctx, identity.Name, codersdk.UpdateDelegatedIdentityRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeDelegatedIdentityRead},
	})
	require.NoError(t, err)
	child, err = aliceClient.DelegatedIdentity(ctx, child.Name)
	require.NoError(t, err)
	require.Equal(t, []codersdk.APIKeyScope{codersdk.APIKeyScopeWorkspaceRead}, child.Scopes)
	require.Empty(t, child.EffectiveScopes)
	_, err = asIdentity.Workspace(ctx, workspace.ID)
	requireStatus(t, err, http.StatusNotFound)

	// An identity that owns a workspace or has identities below it stays.
	requireStatus(t, aliceClient.DeleteDelegatedIdentity(ctx, identity.Name), http.StatusConflict)
	require.NoError(t, aliceClient.DeleteDelegatedIdentity(ctx, child.Name))
	requireStatus(t, aliceClient.DeleteDelegatedIdentity(ctx, identity.Name), http.StatusConflict)

	// Alice deletes the workspace on behalf of the identity: sharing alone
	// would not let her.
	build, err := aliceClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
	})
	require.NoError(t, err)
	require.Equal(t, alice.ID, build.InitiatorID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, build.ID)

	// Deleting the identity ends its credentials. Its record stays.
	require.NoError(t, aliceClient.DeleteDelegatedIdentity(ctx, identity.Name))
	_, err = asIdentity.DelegatedIdentity(ctx, codersdk.DelegatedIdentityMe)
	requireStatus(t, err, http.StatusUnauthorized)
	_, err = aliceClient.DelegatedIdentity(ctx, identity.ID.String())
	requireStatus(t, err, http.StatusNotFound)
	//nolint:gocritic // The test looks at what no user may.
	record, err := db.GetDelegatedIdentityByID(dbauthz.AsSystemRestricted(ctx), identity.ID)
	require.NoError(t, err)
	require.Equal(t, alice.ID, record.RootUserID)
	require.True(t, auditor.Contains(t, database.AuditLog{
		Action:       database.AuditActionDelete,
		ResourceType: database.ResourceTypeDelegatedIdentity,
		ResourceID:   identity.ID,
		UserID:       alice.ID,
	}))
	_ = bob
}

// TestDelegatedIdentitiesTrustedService covers a service that works for the
// members of an organization, the way Agent Relay does: it matches a session
// to a member, and builds a workspace for the session. With permission to
// create and use delegated identities across the organization, each session
// runs as an identity under the member instead of as the member.
func TestDelegatedIdentitiesTrustedService(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentDelegatedIdentities)}
	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues:         dv,
		IncludeProvisionerDaemon: true,
	})
	admin := coderdtest.CreateFirstUser(t, adminClient)
	developerClient, developer := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

	version := coderdtest.CreateTemplateVersion(t, adminClient, admin.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
	template := coderdtest.CreateTemplate(t, adminClient, admin.OrganizationID, version.ID)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	adminUser, err := adminClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	adminCtx := dbauthz.As(ctx, coderdtest.AuthzUserSubject(adminUser))

	// What Agent Relay's service account is documented to need in an
	// organization today.
	relayPermissions := []database.CustomRolePermission{
		{ResourceType: "organization_member", Action: policy.ActionRead},
		{ResourceType: "workspace", Action: policy.ActionCreate},
		{ResourceType: "workspace", Action: policy.ActionRead},
		{ResourceType: "workspace", Action: policy.ActionWorkspaceStart},
		{ResourceType: "workspace", Action: policy.ActionUpdate},
		{ResourceType: "workspace", Action: policy.ActionDelete},
		{ResourceType: "workspace_secret", Action: policy.ActionCreate},
	}
	// What it needs on top to run sessions as delegated identities.
	identityPermissions := []database.CustomRolePermission{
		{ResourceType: "delegated_identity", Action: policy.ActionCreate},
		{ResourceType: "delegated_identity", Action: policy.ActionRead},
		{ResourceType: "delegated_identity", Action: policy.ActionUse},
		{ResourceType: "delegated_identity", Action: policy.ActionDelete},
	}
	serviceClient := func(name string, permissions []database.CustomRolePermission) (*codersdk.Client, database.User) {
		_, err := db.InsertCustomRole(adminCtx, database.InsertCustomRoleParams{
			Name:              name,
			DisplayName:       name,
			OrganizationID:    uuid.NullUUID{UUID: admin.OrganizationID, Valid: true},
			SitePermissions:   []database.CustomRolePermission{},
			OrgPermissions:    permissions,
			UserPermissions:   []database.CustomRolePermission{},
			MemberPermissions: []database.CustomRolePermission{},
		})
		require.NoError(t, err)
		account := dbgen.User(t, db, database.User{IsServiceAccount: true})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: admin.OrganizationID,
			UserID:         account.ID,
			Roles:          []string{name},
		})
		_, token := dbgen.APIKey(t, db, database.APIKey{UserID: account.ID, LoginType: database.LoginTypeToken})
		client := codersdk.New(adminClient.URL)
		client.SetSessionToken(token)
		return client, account
	}

	// Without the new permissions the service works as it does today:
	// the session's workspace is the developer's, and the agent in it is
	// the developer to Coder. It cannot create an identity under anyone.
	legacyClient, _ := serviceClient("relay-today", relayPermissions)
	_, err = legacyClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{ParentID: developer.ID})
	requireStatus(t, err, http.StatusForbidden)
	legacyWorkspace, err := legacyClient.CreateUserWorkspace(ctx, developer.ID.String(), codersdk.CreateWorkspaceRequest{
		TemplateID: template.ID, Name: "session-0",
	})
	require.NoError(t, err)
	require.Equal(t, developer.ID, legacyWorkspace.OwnerID)

	// With them, it creates an identity under the developer it matched.
	// It is not above the developer: the permission across the
	// organization is what lets it.
	relayClient, relay := serviceClient("relay-identities", append(relayPermissions, identityPermissions...))
	identity, err := relayClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{
		Name:     "relay-session-1",
		ParentID: developer.ID,
	})
	require.NoError(t, err)
	require.Equal(t, developer.ID, identity.ParentID)
	require.Equal(t, developer.ID, identity.RootUserID)
	require.Equal(t, relay.ID, identity.CreatedBy)

	// The session's workspace is built on behalf of the identity. The
	// identity owns it, the service is who asked, and the developer can
	// enter it.
	workspace, err := relayClient.CreateUserWorkspace(ctx, identity.ID.String(), codersdk.CreateWorkspaceRequest{
		TemplateID: template.ID, Name: "session-1",
	})
	require.NoError(t, err)
	require.Equal(t, identity.ID, workspace.OwnerID)
	require.Equal(t, relay.ID, workspace.LatestBuild.InitiatorID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, workspace.LatestBuild.ID)
	_, err = developerClient.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	listed, err := developerClient.DelegatedIdentities(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1, "the developer sees the identity a service created under them")

	// The service has no identities of its own, and the identity is no
	// way for it to reach what the developer owns.
	own, err := relayClient.DelegatedIdentities(ctx)
	require.NoError(t, err)
	require.Empty(t, own)

	// When the session ends, the service deletes the workspace and the
	// identity.
	build, err := relayClient.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		Transition: codersdk.WorkspaceTransitionDelete,
	})
	require.NoError(t, err)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, adminClient, build.ID)
	require.NoError(t, relayClient.DeleteDelegatedIdentity(ctx, identity.ID.String()))
}

// TestDelegatedIdentitiesChat covers a chat that runs under a delegated
// identity. A person creates the chat on behalf of the identity. The chat
// is the identity's, the workspace its tools create is the identity's, and
// its model calls are made with a key of the identity's. The person works
// with the chat on behalf of the identity.
func TestDelegatedIdentitiesChat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentDelegatedIdentities)}
	adminClient, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         dv,
		IncludeProvisionerDaemon: true,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	admin := coderdtest.CreateFirstUser(t, adminClient)
	aliceClient, alice := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	bobClient, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	aliceChats := codersdk.NewExperimentalClient(aliceClient)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, adminClient, admin.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
	template := coderdtest.CreateTemplate(t, adminClient, admin.OrganizationID, version.ID)

	// The model asks for a workspace, and is done once it has the result.
	var streamedCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Delegated identity chat")
		}
		if streamedCalls.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk("create_workspace",
				fmt.Sprintf(`{"template_id":%q,"name":"chat-task"}`, template.ID.String())))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Done.")...)
	})
	coderdtest.CreateOpenAICompatChatModel(t, codersdk.NewExperimentalClient(adminClient), openAIURL)

	// The identity may chat because its root user may. What its tools may
	// do with workspaces is what it was given: nothing yet.
	identity, err := aliceClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{Name: "chat-agent"})
	require.NoError(t, err)

	message := []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Create a workspace."}}
	// Bob is not above the identity.
	_, err = codersdk.NewExperimentalClient(bobClient).CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: admin.OrganizationID,
		OwnerID:        &identity.ID,
		Content:        message,
	})
	requireStatus(t, err, http.StatusForbidden)

	awaitChat := func(chatID uuid.UUID) codersdk.Chat {
		var got codersdk.Chat
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var err error
			got, err = aliceChats.GetChat(ctx, chatID)
			return err == nil && (got.Status == codersdk.ChatStatusWaiting || got.Status == codersdk.ChatStatusError)
		}, testutil.IntervalFast)
		return got
	}

	// Without scopes the chat runs, and its tool is refused.
	chat, err := aliceChats.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: admin.OrganizationID,
		OwnerID:        &identity.ID,
		Content:        message,
	})
	require.NoError(t, err)
	require.Equal(t, identity.ID, chat.OwnerID)
	refused := awaitChat(chat.ID)
	require.Nil(t, refused.WorkspaceID, "an identity that may not create workspaces gets none")
	// Somebody who is not above the identity cannot see its chat.
	_, err = codersdk.NewExperimentalClient(bobClient).GetChat(ctx, chat.ID)
	requireStatus(t, err, http.StatusNotFound)

	// With the scope, a second chat of the same identity gets a workspace.
	_, err = aliceClient.UpdateDelegatedIdentity(ctx, identity.Name, codersdk.UpdateDelegatedIdentityRequest{
		Scopes: []codersdk.APIKeyScope{"coder:workspaces.create"},
	})
	require.NoError(t, err)
	streamedCalls.Store(0)
	chat, err = aliceChats.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: admin.OrganizationID,
		OwnerID:        &identity.ID,
		Content:        message,
	})
	require.NoError(t, err)
	var workspace codersdk.Workspace
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		got, err := aliceChats.GetChat(ctx, chat.ID)
		if err != nil || got.WorkspaceID == nil {
			return false
		}
		workspace, err = aliceClient.Workspace(ctx, *got.WorkspaceID)
		return err == nil && workspace.LatestBuild.Status == codersdk.WorkspaceStatusRunning
	}, testutil.IntervalFast)
	_ = agenttest.New(t, adminClient.URL, agentToken)
	done := awaitChat(chat.ID)
	require.Equal(t, codersdk.ChatStatusWaiting, done.Status, "last error: %+v", done.LastError)

	// The workspace is the identity's, and Alice can enter it.
	require.Equal(t, identity.ID, workspace.OwnerID)
	require.Equal(t, identity.ID, workspace.LatestBuild.InitiatorID)

	// The model was called with a key of the identity's, so the calls
	// are attributed to the identity and not to Alice.
	//nolint:gocritic // The test looks at what no user may.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	identityKeys, err := api.Database.GetAPIKeysByUserID(sysCtx, database.GetAPIKeysByUserIDParams{
		LoginType: database.LoginTypeNone, UserID: identity.ID,
	})
	require.NoError(t, err)
	// The identity has that key and the session token of its workspace.
	tokenNames := make([]string, 0, len(identityKeys))
	for _, key := range identityKeys {
		tokenNames = append(tokenNames, key.TokenName)
	}
	require.Len(t, tokenNames, 2)
	require.Contains(t, tokenNames, chatd.GatewayTokenName(identity.ID))
	aliceKeys, err := api.Database.GetAPIKeysByUserID(sysCtx, database.GetAPIKeysByUserIDParams{
		LoginType: database.LoginTypePassword, UserID: alice.ID,
	})
	require.NoError(t, err)
	for _, key := range aliceKeys {
		require.NotEqual(t, chatd.GatewayTokenName(alice.ID), key.TokenName, "no model call was made as Alice")
	}

	// Alice carries the chat on, on behalf of the identity.
	_, err = aliceChats.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Thanks."}},
	})
	require.NoError(t, err)
	done = awaitChat(chat.ID)
	require.Equal(t, codersdk.ChatStatusWaiting, done.Status, "last error: %+v", done.LastError)
}

// TestDelegatedIdentitiesRootUser checks that a delegated identity cannot
// act without its root user: not while the user is suspended, and the user
// cannot be deleted from under it.
func TestDelegatedIdentitiesRootUser(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentDelegatedIdentities)}
	adminClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
	admin := coderdtest.CreateFirstUser(t, adminClient)
	userClient, user := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	identity, err := userClient.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{
		Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeDelegatedIdentityRead},
	})
	require.NoError(t, err)
	asIdentity := identityClient(t, db, adminClient, identity.ID)
	_, err = asIdentity.DelegatedIdentity(ctx, codersdk.DelegatedIdentityMe)
	require.NoError(t, err)

	_, err = adminClient.UpdateUserStatus(ctx, user.ID.String(), codersdk.UserStatusSuspended)
	require.NoError(t, err)
	_, err = asIdentity.DelegatedIdentity(ctx, codersdk.DelegatedIdentityMe)
	requireStatus(t, err, http.StatusUnauthorized)
	_, err = adminClient.UpdateUserStatus(ctx, user.ID.String(), codersdk.UserStatusActive)
	require.NoError(t, err)
	_, err = asIdentity.DelegatedIdentity(ctx, codersdk.DelegatedIdentityMe)
	require.NoError(t, err)

	requireStatus(t, adminClient.DeleteUser(ctx, user.ID), http.StatusExpectationFailed)
	require.NoError(t, userClient.DeleteDelegatedIdentity(ctx, identity.ID.String()))
	require.NoError(t, adminClient.DeleteUser(ctx, user.ID))
}

// TestDelegatedIdentitiesExperiment checks that nothing is served without
// the experiment.
func TestDelegatedIdentitiesExperiment(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitShort)
	_, err := client.CreateDelegatedIdentity(ctx, codersdk.CreateDelegatedIdentityRequest{})
	requireStatus(t, err, http.StatusForbidden)
}
