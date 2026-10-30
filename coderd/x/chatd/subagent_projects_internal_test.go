package chatd

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/quartz"
)

func TestSpawnAgent_InvokableAgent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	templateID := uuid.New()
	securityUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: securityUser.ID, OrganizationID: org.ID})
	security := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		AgentUserID:        uuid.NullUUID{UUID: securityUser.ID, Valid: true},
		Name:               "Security",
		Description:        "Reviews changes for vulnerabilities.",
		AllowedTemplateIDs: []uuid.UUID{templateID},
		TemplateParameterRules: database.ChatProjectTemplateParameterRules{
			{TemplateID: templateID, ParameterName: "region", AllowedValues: []string{"eu"}},
		},
	})
	root := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Name:              "Lead",
		InvokableAgentIDs: []uuid.UUID{security.ID},
	})
	// An agent only another agent may invoke is never offered.
	unrelated := dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID, Name: "Unrelated"})
	dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Name:              "Other",
		InvokableAgentIDs: []uuid.UUID{unrelated.ID},
	})

	created, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		ProjectID:          uuid.NullUUID{UUID: root.ID, Valid: true},
		Title:              "lead chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parent, err := db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)

	tool := findToolByName(server.subagentTools(ctx, func() database.Chat { return parent }, model.ID), spawnAgentToolName)
	require.NotNil(t, tool)
	description := tool.Info().Description
	require.Contains(t, description, security.ID.String())
	require.Contains(t, description, "Reviews changes for vulnerabilities.")
	require.NotContains(t, description, unrelated.ID.String())

	resp := runSpawnAgentTool(ctx, t, server, parent, spawnAgentArgs{
		AgentID: unrelated.ID.String(),
		Prompt:  "review",
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "is not an agent this chat may invoke")

	resp = runSpawnAgentTool(ctx, t, server, parent, spawnAgentArgs{
		AgentID: security.ID.String(),
		Prompt:  "review the diff",
	})
	child, err := db.GetChatByID(ctx, requireSpawnAgentChildChatID(t, resp))
	require.NoError(t, err)
	require.Equal(t, uuid.NullUUID{UUID: security.ID, Valid: true}, child.ProjectID)
	require.Equal(t, uuid.NullUUID{UUID: parent.ID, Valid: true}, child.ParentChatID)
	require.False(t, child.WorkspaceID.Valid, "an invoked agent starts without the parent's workspace")
	// The invoked agent's chat runs as that agent's user, with its own MCP
	// tokens, and is shared back with the person who started the parent.
	require.Equal(t, securityUser.ID, child.OwnerID)
	require.Contains(t, child.UserACL, user.ID.String())
	require.Contains(t, child.UserACL[user.ID.String()].Permissions, policy.ActionUpdate)

	// The child is limited to the invoked agent's templates, and the parent to
	// its own agent's (none configured, so every template).
	require.Equal(t, []uuid.UUID{templateID}, server.chatTemplatePolicy(ctx, child).AllowedTemplateIDs)
	require.Equal(t, map[uuid.UUID]map[string][]string{templateID: {"region": {"eu"}}},
		server.chatTemplatePolicy(ctx, child).ParameterRules)
	require.True(t, server.chatTemplatePolicy(ctx, parent).IsZero())

	// Turning workspace tools off applies to the agent's own chats, not to
	// the agents it invokes.
	require.True(t, server.chatWorkspaceToolsEnabled(ctx, parent))
	_, err = db.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
		ID:                      root.ID,
		Name:                    root.Name,
		AllowedTemplateIDs:      root.AllowedTemplateIDs,
		MCPServerIDs:            root.MCPServerIDs,
		ExternalAuthProviderIDs: root.ExternalAuthProviderIDs,
		TemplateParameterRules:  root.TemplateParameterRules,
		InvokableAgentIDs:       root.InvokableAgentIDs,
		WorkspaceToolsEnabled:   false,
	})
	require.NoError(t, err)
	require.False(t, server.chatWorkspaceToolsEnabled(ctx, parent))
	require.True(t, server.chatWorkspaceToolsEnabled(ctx, child))
}

func TestSpawnAgent_SharedAgent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	owner, org, model := seedInternalChatDeps(t, db)
	member := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: member.ID, OrganizationID: org.ID})
	shared := database.ChatACL{member.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}}

	sharedChild := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		Name:           "Shared reviewer",
		UserACL:        shared,
	})
	privateChild := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		Name:           "Private reviewer",
	})
	root := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		Name:              "Lead",
		UserACL:           shared,
		InvokableAgentIDs: []uuid.UUID{sharedChild.ID, privateChild.ID},
	})

	created, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            member.ID,
		ProjectID:          uuid.NullUUID{UUID: root.ID, Valid: true},
		Title:              "shared chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parent, err := db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)

	// A shared user can invoke only agents shared with them.
	agents := server.spawnableProjectAgents(ctx, parent)
	require.Len(t, agents, 1)
	require.Equal(t, sharedChild.ID, agents[0].ID)
	require.NotEqual(t, privateChild.ID, agents[0].ID)

	// Once the root agent is no longer shared, nothing is offered.
	require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
		ID:       root.ID,
		UserACL:  database.ChatACL{},
		GroupACL: database.ChatACL{},
	}))
	require.Empty(t, server.spawnableProjectAgents(ctx, parent))
}

func TestSpawnAgent_AgentUserDelegates(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	owner, org, model := seedInternalChatDeps(t, db)
	agentUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: agentUser.ID, OrganizationID: org.ID})

	child := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: org.ID,
		OwnerID:        owner.ID,
		Name:           "Reviewer",
	})
	root := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		Name:              "Lead",
		AgentUserID:       uuid.NullUUID{UUID: agentUser.ID, Valid: true},
		InvokableAgentIDs: []uuid.UUID{child.ID},
	})

	created, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            agentUser.ID,
		CreatedBy:          owner.ID,
		ProjectID:          uuid.NullUUID{UUID: root.ID, Valid: true},
		Title:              "agent chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parent, err := db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)

	// A chat owned by the agent's user can invoke the agents its agent
	// lists that the agent's owner owns. Listing grants the agent's user
	// no access to the invoked agent itself.
	agents := server.spawnableProjectAgents(ctx, parent)
	require.Len(t, agents, 1)
	require.Equal(t, child.ID, agents[0].ID)
	require.False(t, server.chatOwnerCanUseProject(ctx, child.ID, agentUser.ID))
}

func TestSpawnAgent_InvokedAgentAccessIsCheckedLive(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	owner, org, model := seedInternalChatDeps(t, db)
	otherOwner := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: otherOwner.ID, OrganizationID: org.ID})
	agentUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: agentUser.ID, OrganizationID: org.ID})

	edit := database.ChatACL{owner.ID.String(): {Permissions: []policy.Action{policy.ActionRead, policy.ActionUpdate}}}
	use := database.ChatACL{owner.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}}
	// Someone else's agent, shared with the inviting agent's owner.
	invoked := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: org.ID,
		OwnerID:        otherOwner.ID,
		Name:           "Deployer",
		UserACL:        edit,
	})
	root := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		Name:              "Lead",
		AgentUserID:       uuid.NullUUID{UUID: agentUser.ID, Valid: true},
		InvokableAgentIDs: []uuid.UUID{invoked.ID},
	})
	created, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            agentUser.ID,
		CreatedBy:          owner.ID,
		ProjectID:          uuid.NullUUID{UUID: root.ID, Valid: true},
		Title:              "agent chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parent, err := db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)

	agents := server.spawnableProjectAgents(ctx, parent)
	require.Len(t, agents, 1)
	require.Equal(t, invoked.ID, agents[0].ID)

	setACL := func(acl database.ChatACL) {
		t.Helper()
		require.NoError(t, db.UpdateChatProjectACLByID(ctx, database.UpdateChatProjectACLByIDParams{
			ID: invoked.ID, UserACL: acl, GroupACL: database.ChatACL{},
		}))
	}
	// Use access is not enough to lend the agent's identity.
	setACL(use)
	require.Empty(t, server.spawnableProjectAgents(ctx, parent))
	resp := runSpawnAgentTool(ctx, t, server, parent, spawnAgentArgs{AgentID: invoked.ID.String(), Prompt: "deploy"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "is not an agent this chat may invoke")
	// Revoking the share stops invocation too.
	setACL(database.ChatACL{})
	require.Empty(t, server.spawnableProjectAgents(ctx, parent))
	setACL(edit)
	require.Len(t, server.spawnableProjectAgents(ctx, parent), 1)
}

func TestSpawnAgent_InheritWorkspace(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	reviewerUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: reviewerUser.ID, OrganizationID: org.ID})
	reviewer := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		AgentUserID:    uuid.NullUUID{UUID: reviewerUser.ID, Valid: true},
		Name:           "Reviewer",
	})
	offlineUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: offlineUser.ID, OrganizationID: org.ID})
	offline := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		AgentUserID:    uuid.NullUUID{UUID: offlineUser.ID, Valid: true},
		Name:           "Planner",
	})
	_, err := db.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
		ID:                      offline.ID,
		Name:                    offline.Name,
		AllowedTemplateIDs:      []uuid.UUID{},
		MCPServerIDs:            []uuid.UUID{},
		ExternalAuthProviderIDs: []string{},
		InvokableAgentIDs:       []uuid.UUID{},
		WorkspaceToolsEnabled:   false,
	})
	require.NoError(t, err)
	root := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Name:              "Lead",
		InvokableAgentIDs: []uuid.UUID{reviewer.ID, offline.ID},
	})
	newParent := func(workspace database.WorkspaceTable, build database.WorkspaceBuild, agent database.WorkspaceAgent) database.Chat {
		t.Helper()
		opts := CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			ProjectID:          uuid.NullUUID{UUID: root.ID, Valid: true},
			Title:              "lead chat",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		}
		if workspace.ID != uuid.Nil {
			opts.WorkspaceID = uuid.NullUUID{UUID: workspace.ID, Valid: true}
			opts.BuildID = uuid.NullUUID{UUID: build.ID, Valid: true}
			opts.AgentID = uuid.NullUUID{UUID: agent.ID, Valid: true}
		}
		created, err := server.CreateChat(ctx, opts)
		require.NoError(t, err)
		parent, err := db.GetChatByID(ctx, created.ID)
		require.NoError(t, err)
		return parent
	}
	workspace, build, workspaceAgent := seedWorkspaceBinding(t, db, user.ID)
	parent := newParent(workspace, build, workspaceAgent)

	tool := findToolByName(server.subagentTools(ctx, func() database.Chat { return parent }, model.ID), spawnAgentToolName)
	require.NotNil(t, tool)
	require.Contains(t, tool.Info().Description, "Set inherit_workspace to true to work in this chat's workspace")
	require.Contains(t, tool.Info().Description, "Planner\"; no workspace tools")

	t.Log("The invoked agent shares the root chat's workspace")
	resp := runSpawnAgentTool(ctx, t, server, parent, spawnAgentArgs{
		AgentID:          reviewer.ID.String(),
		Prompt:           "review the files in the workspace",
		InheritWorkspace: true,
	})
	require.False(t, resp.IsError, resp.Content)
	child, err := db.GetChatByID(ctx, requireSpawnAgentChildChatID(t, resp))
	require.NoError(t, err)
	require.Equal(t, reviewerUser.ID, child.OwnerID)
	require.Equal(t, parent.WorkspaceID, child.WorkspaceID)
	require.Equal(t, parent.BuildID, child.BuildID)
	require.Equal(t, parent.AgentID, child.AgentID)
	require.True(t, server.chatWorkspaceToolsEnabled(ctx, child))
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, child.ID)
	require.NoError(t, err)
	var toldShared bool
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleSystem && strings.Contains(string(message.Content.RawMessage), "This chat shares the parent chat's workspace") {
			toldShared = true
		}
	}
	require.True(t, toldShared, "the child is told it works in the parent's workspace")

	t.Log("It may not create, start, stop, or switch workspaces there")
	require.True(t, server.chatBorrowsWorkspace(ctx, child))
	require.False(t, server.chatBorrowsWorkspace(ctx, parent), "the root chat manages its own workspace")

	t.Log("Workspace tools reach the root chat's workspace agent")
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), nil)
	ctrl := gomock.NewController(t)
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	var dialed []uuid.UUID
	connServer := &Server{
		db:                             authzDB,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: time.Minute,
		dialTimeout:                    time.Minute,
		agentConnFn: func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			dialed = append(dialed, agentID)
			return conn, func() {}, nil
		},
	}
	currentChild := child
	workspaceCtx := turnWorkspaceContext{
		server:           connServer,
		chatStateMu:      &sync.Mutex{},
		currentChat:      &currentChild,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return currentChild, nil },
	}
	t.Cleanup(workspaceCtx.close)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, conn, gotConn)
	require.Equal(t, []uuid.UUID{workspaceAgent.ID}, dialed)

	t.Log("A workspace the invoked agent creates for itself is its own to manage")
	ownWorkspace, _, _ := seedWorkspaceBinding(t, db, reviewerUser.ID)
	ownChild := child
	ownChild.WorkspaceID = uuid.NullUUID{UUID: ownWorkspace.ID, Valid: true}
	require.False(t, server.chatBorrowsWorkspace(ctx, ownChild))

	t.Log("An agent without workspace tools cannot inherit a workspace")
	resp = runSpawnAgentTool(ctx, t, server, parent, spawnAgentArgs{
		AgentID:          offline.ID.String(),
		Prompt:           "plan",
		InheritWorkspace: true,
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, `agent "Planner" does not allow workspace tools`)

	t.Log("A chat without a workspace has none to share")
	noWorkspace := newParent(database.WorkspaceTable{}, database.WorkspaceBuild{}, database.WorkspaceAgent{})
	resp = runSpawnAgentTool(ctx, t, server, noWorkspace, spawnAgentArgs{
		AgentID:          reviewer.ID.String(),
		Prompt:           "review",
		InheritWorkspace: true,
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "this chat has no workspace to share")

	t.Log("Without inherit_workspace the invoked agent still starts without one")
	resp = runSpawnAgentTool(ctx, t, server, parent, spawnAgentArgs{
		AgentID: reviewer.ID.String(),
		Prompt:  "review",
	})
	isolated, err := db.GetChatByID(ctx, requireSpawnAgentChildChatID(t, resp))
	require.NoError(t, err)
	require.False(t, isolated.WorkspaceID.Valid)
}

func TestSpawnAgent_InvokedAgentModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	agentModel := insertInternalChatModelConfig(t, db, org.ID, "agent-"+uuid.NewString(), true)
	disabledModel := insertInternalChatModelConfig(t, db, org.ID, "disabled-"+uuid.NewString(), false)

	securityUser := dbgen.User(t, db, database.User{IsAgent: true, LoginType: database.LoginTypeNone, Email: ""})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: securityUser.ID, OrganizationID: org.ID})
	security := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		AgentUserID:    uuid.NullUUID{UUID: securityUser.ID, Valid: true},
		Name:           "Security",
		ModelConfigID:  uuid.NullUUID{UUID: agentModel.ID, Valid: true},
	})
	root := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Name:              "Lead",
		InvokableAgentIDs: []uuid.UUID{security.ID},
	})
	created, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		ProjectID:          uuid.NullUUID{UUID: root.ID, Valid: true},
		Title:              "lead chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parent, err := db.GetChatByID(ctx, created.ID)
	require.NoError(t, err)

	spawn := func(args spawnAgentArgs) database.Chat {
		t.Helper()
		args.AgentID = security.ID.String()
		args.Prompt = "review the diff"
		child, err := db.GetChatByID(ctx, requireSpawnAgentChildChatID(t, runSpawnAgentTool(ctx, t, server, parent, args)))
		require.NoError(t, err)
		return child
	}

	// The invoked agent's model applies to its child.
	require.Equal(t, agentModel.ID, spawn(spawnAgentArgs{}).LastModelConfigID)
	// A model named in the call wins.
	require.Equal(t, model.ID, spawn(spawnAgentArgs{ModelConfigID: model.ID.String()}).LastModelConfigID)

	// An unusable agent model falls back to the parent's.
	_, err = db.UpdateChatProjectByID(ctx, database.UpdateChatProjectByIDParams{
		ID:                      security.ID,
		Name:                    security.Name,
		AllowedTemplateIDs:      security.AllowedTemplateIDs,
		MCPServerIDs:            security.MCPServerIDs,
		ExternalAuthProviderIDs: security.ExternalAuthProviderIDs,
		TemplateParameterRules:  security.TemplateParameterRules,
		InvokableAgentIDs:       security.InvokableAgentIDs,
		WorkspaceToolsEnabled:   security.WorkspaceToolsEnabled,
		ModelConfigID:           uuid.NullUUID{UUID: disabledModel.ID, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, model.ID, spawn(spawnAgentArgs{}).LastModelConfigID)
}
