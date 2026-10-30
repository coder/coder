package coderd_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectAgentUser(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID

	project, err := client.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{
		Name: "Release Notes",
		Icon: "/emojis/1f4dd.png",
	})
	require.NoError(t, err)
	require.NotNil(t, project.AgentUser)
	require.Equal(t, "agent-release-notes", project.AgentUser.Username)
	require.Equal(t, "Release Notes", project.AgentUser.Name)

	// The agent's user is an agent user that cannot sign in and can use
	// Coder Agents in the agent's organization.
	agentUser, err := client.User(ctx, project.AgentUser.ID.String())
	require.NoError(t, err)
	require.True(t, agentUser.IsAgent)
	require.False(t, agentUser.IsServiceAccount)
	require.Equal(t, codersdk.LoginTypeNone, agentUser.LoginType)
	require.Equal(t, codersdk.UserStatusActive, agentUser.Status)
	require.Empty(t, agentUser.Email)
	require.Equal(t, "/emojis/1f4dd.png", agentUser.AvatarURL)
	member, err := client.OrganizationMember(ctx, orgID.String(), agentUser.ID.String())
	require.NoError(t, err)
	roleNames := make([]string, 0, len(member.Roles))
	for _, role := range member.Roles {
		roleNames = append(roleNames, role.Name)
	}
	require.Contains(t, roleNames, codersdk.RoleAgentsAccess)
	require.True(t, member.IsAgent)

	// Agent users can be filtered like service accounts.
	agentUsers, err := client.Users(ctx, codersdk.UsersRequest{SearchQuery: "agent:true"})
	require.NoError(t, err)
	require.Len(t, agentUsers.Users, 1)
	require.Equal(t, agentUser.ID, agentUsers.Users[0].ID)

	// Agent users are removed by deleting the agent, not the user.
	err = client.DeleteUser(ctx, agentUser.ID)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

	// Agent users never sign in, so they are not marked dormant.
	//nolint:gocritic // The dormancy job runs as the system.
	dormant, err := db.UpdateInactiveUsersToDormant(dbauthz.AsSystemRestricted(ctx), database.UpdateInactiveUsersToDormantParams{
		LastSeenAfter: dbtime.Now().Add(time.Hour),
		UpdatedAt:     dbtime.Now(),
	})
	require.NoError(t, err)
	for _, row := range dormant {
		require.NotEqual(t, agentUser.ID, row.ID)
	}

	// A second agent with the same name gets a distinct username.
	sameName := createChatProject(t, client, orgID, "Release Notes")
	require.NotNil(t, sameName.AgentUser)
	require.NotEqual(t, project.AgentUser.Username, sameName.AgentUser.Username)
	require.True(t, strings.HasPrefix(sameName.AgentUser.Username, "agent-release-notes-"))

	// Renaming keeps the username and updates the display name.
	name := "Changelog"
	_, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
	require.NoError(t, err)
	agentUser, err = client.User(ctx, project.AgentUser.ID.String())
	require.NoError(t, err)
	require.Equal(t, "Changelog", agentUser.Name)
	require.Equal(t, "agent-release-notes", agentUser.Username)

	// The agent's name is its user's display name, so renaming the user
	// renames the agent, and invalid agent names are rejected for both.
	_, err = client.UpdateUserProfile(ctx, agentUser.ID.String(), codersdk.UpdateUserProfileRequest{
		Username:  agentUser.Username,
		Name:      "Release Bot",
		AvatarURL: "/emojis/1f916.png",
	})
	require.NoError(t, err)
	renamed, err := client.GetChatProject(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Equal(t, "Release Bot", renamed.Name)
	require.Equal(t, "/emojis/1f916.png", renamed.Icon)
	require.Equal(t, "Release Bot", renamed.AgentUser.Name)
	_, err = client.UpdateUserProfile(ctx, agentUser.ID.String(), codersdk.UpdateUserProfileRequest{
		Username: agentUser.Username,
		Name:     "",
	})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
	tooLong := strings.Repeat("a", 65)
	_, err = client.UpdateUserProfile(ctx, agentUser.ID.String(), codersdk.UpdateUserProfileRequest{
		Username: agentUser.Username,
		Name:     tooLong,
	})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
	// Names that are valid for an agent but not as a display name are
	// rejected rather than shortened.
	multibyte := strings.Repeat("界", 50)
	_, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Name: &multibyte})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	// Agents whose user drifted are brought back in step at startup.
	//nolint:gocritic // Simulates a profile change made before syncing.
	_, err = db.UpdateUserProfile(dbauthz.AsSystemRestricted(ctx), database.UpdateUserProfileParams{
		ID:        agentUser.ID,
		Username:  agentUser.Username,
		Name:      "Stale",
		AvatarURL: "",
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	//nolint:gocritic // The startup sync runs as the system.
	require.NoError(t, db.SyncChatProjectAgentUserProfiles(dbauthz.AsSystemRestricted(ctx)))
	agentUser, err = client.User(ctx, project.AgentUser.ID.String())
	require.NoError(t, err)
	require.Equal(t, "Release Bot", agentUser.Name)
	require.Equal(t, "/emojis/1f916.png", agentUser.AvatarURL)

	// Deleting the agent suspends its user and keeps it.
	require.NoError(t, client.DeleteChatProject(ctx, orgID, project.ID))
	agentUser, err = client.User(ctx, project.AgentUser.ID.String())
	require.NoError(t, err)
	require.Equal(t, codersdk.UserStatusSuspended, agentUser.Status)
}

func TestChatProjectAgentOwnsChats(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)

	project := createChatProject(t, client, orgID, "Helper")
	require.NotNil(t, project.AgentUser)
	memberSDK, member := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	memberClient := codersdk.NewExperimentalClient(memberSDK)
	viewerSDK, viewer := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	viewerClient := codersdk.NewExperimentalClient(viewerSDK)
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{
			member.ID.String(): codersdk.ChatProjectRoleUse,
			viewer.ID.String(): codersdk.ChatProjectRoleUse,
		},
	}))

	// A chat someone starts in the agent belongs to the agent's user and
	// is shared back to them for writing.
	chat := createChatInProject(t, memberClient, orgID, &project.ID)
	require.Equal(t, project.AgentUser.ID, chat.OwnerID)
	acl, err := client.GetChatACL(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, acl.Users, 1)
	require.Equal(t, member.ID, acl.Users[0].ID)
	require.Equal(t, codersdk.ChatRoleWrite, acl.Users[0].Role)

	// It is listed as the starter's own chat.
	owned, err := memberClient.ListChats(ctx, &codersdk.ListChatsOptions{ProjectID: &project.ID})
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.Equal(t, chat.ID, owned[0].ID)

	// The starter can keep talking to it.
	_, err = memberClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "follow up"}},
	})
	require.NoError(t, err)

	// Read access to the chat is not enough to drive it.
	require.NoError(t, client.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
		UserRoles: map[string]codersdk.ChatRole{viewer.ID.String(): codersdk.ChatRoleRead},
	}))
	_, err = viewerClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "read only"}},
	})
	require.Error(t, err)

	// Losing access to the agent stops the starter from driving the chat.
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{member.ID.String(): codersdk.ChatProjectRoleDeleted},
	}))
	_, err = memberClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "revoked"}},
	})
	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
}

func TestChatProjectAgentExternalAuth(t *testing.T) {
	t.Parallel()

	const providerID = "fake-github"
	fake := oidctest.NewFakeIDP(t, oidctest.WithServing())
	client, _ := newChatClientWithDatabase(t, func(options *coderdtest.Options) {
		options.ExternalAuthConfigs = []*externalauth.Config{
			fake.ExternalAuthConfig(t, providerID, nil, func(cfg *externalauth.Config) {
				cfg.Type = codersdk.EnhancedExternalAuthProviderGitHub.String()
			}),
		}
	}, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	ctx := testutil.Context(t, testutil.WaitLong)
	project := createChatProject(t, client, orgID, "Reviewer")

	// Someone who can only use the agent cannot connect accounts for it.
	memberSDK, member := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	memberClient := codersdk.NewExperimentalClient(memberSDK)
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{member.ID.String(): codersdk.ChatProjectRoleUse},
	}))
	_, err := memberClient.ConnectChatProjectExternalAuth(ctx, orgID, project.ID, providerID, codersdk.ConnectChatProjectExternalAuthRequest{})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())

	// Chats act with the agent's accounts, so even an editor cannot change
	// them. Only the agent's owner can.
	editorSDK, editor := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	editorClient := codersdk.NewExperimentalClient(editorSDK)
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{editor.ID.String(): codersdk.ChatProjectRoleEdit},
	}))
	_, err = editorClient.ConnectChatProjectExternalAuth(ctx, orgID, project.ID, providerID, codersdk.ConnectChatProjectExternalAuthRequest{})
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	err = editorClient.ChatProjectExternalAuthDeviceExchange(ctx, orgID, project.ID, providerID, codersdk.ExternalAuthDeviceExchange{DeviceCode: "code"})
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
	_, err = editorClient.ChatProjectExternalAuth(ctx, orgID, project.ID)
	require.NoError(t, err, "an editor can still see which accounts are connected")

	// The manager connects an account through the provider's normal flow,
	// and the link is stored for the agent's user, not the manager.
	connectAgentExternalAuth(t, client, fake, orgID, project.ID, providerID)

	links, err := client.ChatProjectExternalAuth(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Len(t, links.Links, 1)
	require.Equal(t, providerID, links.Links[0].ProviderID)
	mine, err := client.ListExternalAuths(ctx)
	require.NoError(t, err)
	require.Empty(t, mine.Links, "the manager's own account must not be linked")

	_, err = editorClient.DisconnectChatProjectExternalAuth(ctx, orgID, project.ID, providerID)
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())

	_, err = client.DisconnectChatProjectExternalAuth(ctx, orgID, project.ID, providerID)
	require.NoError(t, err)
	links, err = client.ChatProjectExternalAuth(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Empty(t, links.Links)
}

// connectAgentExternalAuth runs the browser flow that links an external auth
// account to an agent's user.
func connectAgentExternalAuth(t *testing.T, client *codersdk.ExperimentalClient, fake *oidctest.FakeIDP, orgID, projectID uuid.UUID, providerID string) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	res, err := client.Request(ctx, http.MethodPost, "/api/experimental/organizations/"+orgID.String()+"/chats/projects/"+projectID.String()+"/external-auth/"+providerID+"/connect", codersdk.ConnectChatProjectExternalAuthRequest{})
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var connect codersdk.ConnectChatProjectExternalAuthResponse
	require.NoError(t, codersdk.ReadBodyAsJSON(res, &connect))
	connectURL, err := url.Parse(connect.URL)
	require.NoError(t, err)
	state := connectURL.Query().Get("oidc_merge_state")
	require.True(t, strings.HasPrefix(state, "agent-"))

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	jar.SetCookies(client.URL, res.Cookies())
	httpClient := *client.HTTPClient
	httpClient.Jar = jar
	loginClient := codersdk.New(client.URL, codersdk.WithHTTPClient(&httpClient), codersdk.WithSessionToken(client.SessionToken()))
	fake.ExternalLogin(t, loginClient, func(r *http.Request) {
		q := r.URL.Query()
		q.Set("oidc_merge_state", state)
		r.URL.RawQuery = q.Encode()
	})
}

func TestChatProjectAgentChatMCPServers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	//nolint:gocritic // Reads stored chats and credentials directly.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	newServer := func(slug string) codersdk.MCPServerConfig {
		server, err := client.CreateMCPServerConfig(ctx, orgID, codersdk.CreateMCPServerConfigRequest{
			DisplayName:  slug,
			Slug:         slug,
			Transport:    "streamable_http",
			URL:          "https://mcp.example.com/" + slug,
			AuthType:     codersdk.MCPServerAuthTypeUserAPIKey,
			Availability: "default_off",
			Enabled:      true,
		})
		require.NoError(t, err)
		return server
	}
	agentServer := newServer("agent-tools")
	otherServer := newServer("other-tools")

	project := createChatProject(t, client, orgID, "Helper")
	attached := []uuid.UUID{agentServer.ID}
	_, err := client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{MCPServerIDs: &attached})
	require.NoError(t, err)
	_, err = client.SetChatProjectMCPServerToken(ctx, orgID, project.ID, agentServer.ID, codersdk.UpdateMCPServerUserTokenRequest{Token: "agent-token"})
	require.NoError(t, err)

	memberSDK, member := coderdtest.CreateAnotherUser(t, client.Client, orgID)
	memberClient := codersdk.NewExperimentalClient(memberSDK)
	require.NoError(t, client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACLRequest{
		UserRoles: map[string]codersdk.ChatProjectRole{member.ID.String(): codersdk.ChatProjectRoleUse},
	}))
	storedServers := func(chatID uuid.UUID) []uuid.UUID {
		chat, err := db.GetChatByID(sysCtx, chatID)
		require.NoError(t, err)
		return chat.MCPServerIDs
	}

	t.Log("A chat the agent owns only gets the agent's servers")
	chat, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		ProjectID:      &project.ID,
		MCPServerIDs:   []uuid.UUID{otherServer.ID},
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, project.AgentUser.ID, chat.OwnerID)
	require.Equal(t, []uuid.UUID{agentServer.ID}, storedServers(chat.ID))

	t.Log("Later messages cannot add other servers either")
	_, err = memberClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
		Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "follow up"}},
		MCPServerIDs: &[]uuid.UUID{agentServer.ID, otherServer.ID},
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{agentServer.ID}, storedServers(chat.ID))

	t.Log("A person's own chat keeps the servers they pick")
	own, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		MCPServerIDs:   []uuid.UUID{otherServer.ID},
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "mine"}},
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{otherServer.ID}, storedServers(own.ID))

	t.Log("Detaching a server deletes the agent's credential for it")
	tokens, err := db.GetMCPServerUserTokensByUserID(sysCtx, project.AgentUser.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	attached = []uuid.UUID{}
	_, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{MCPServerIDs: &attached})
	require.NoError(t, err)
	tokens, err = db.GetMCPServerUserTokensByUserID(sysCtx, project.AgentUser.ID)
	require.NoError(t, err)
	require.Empty(t, tokens)
}

func TestChatProjectAgentChildInheritedWorkspace(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	model := createChatModel(t, client)
	project := createChatProject(t, client, orgID, "Reviewer")

	// A delegated chat that inherited the root chat's workspace is owned by
	// the invoked agent's user but bound to the root owner's workspace.
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        firstUser.UserID,
	}).WithAgent().Do()
	root := createChatInProject(t, client, orgID, nil)
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           project.AgentUser.ID,
		ProjectID:         uuid.NullUUID{UUID: project.ID, Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		WorkspaceID:       uuid.NullUUID{UUID: workspace.Workspace.ID, Valid: true},
		BuildID:           uuid.NullUUID{UUID: workspace.Build.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: workspace.Agents[0].ID, Valid: true},
		LastModelConfigID: model.ID,
		Title:             "inherited workspace",
	})

	// The root chat's owner sees the child and the workspace it works in.
	got, err := client.GetChat(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, project.AgentUser.ID, got.OwnerID)
	require.NotNil(t, got.WorkspaceID)
	require.Equal(t, workspace.Workspace.ID, *got.WorkspaceID)
	require.NotNil(t, got.AgentID)
	require.Equal(t, workspace.Agents[0].ID, *got.AgentID)
	ws, err := client.Workspace(ctx, *got.WorkspaceID)
	require.NoError(t, err)
	require.Equal(t, firstUser.UserID, ws.OwnerID)
}
