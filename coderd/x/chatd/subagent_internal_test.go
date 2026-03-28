package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestComputerUseSubagentSystemPrompt(t *testing.T) {
	t.Parallel()

	// Verify the system prompt constant is non-empty and contains
	// key instructions for the computer use agent.
	assert.NotEmpty(t, computerUseSubagentSystemPrompt)
	assert.Contains(t, computerUseSubagentSystemPrompt, "computer")
	assert.Contains(t, computerUseSubagentSystemPrompt, "screenshot")
}

func TestSubagentFallbackChatTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "EmptyPrompt",
			input: "",
			want:  "New Chat",
		},
		{
			name:  "ShortPrompt",
			input: "Open Firefox",
			want:  "Open Firefox",
		},
		{
			name:  "LongPrompt",
			input: "Please open the Firefox browser and navigate to the settings page",
			want:  "Please open the Firefox browser and...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := subagentFallbackChatTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// newInternalTestServer creates a Server for internal tests with
// custom provider API keys. The server is automatically closed
// when the test finishes.
func newInternalTestServer(
	t *testing.T,
	db database.Store,
	ps pubsub.Pubsub,
	keys chatprovider.ProviderAPIKeys,
) *Server {
	return newInternalTestServerWithClock(t, db, ps, keys, nil)
}

func newInternalTestServerWithClock(
	t *testing.T,
	db database.Store,
	ps pubsub.Pubsub,
	keys chatprovider.ProviderAPIKeys,
	clk quartz.Clock,
) *Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := New(Config{
		Logger:    logger,
		Database:  db,
		ReplicaID: uuid.New(),
		Pubsub:    ps,
		Clock:     clk,
		// Use a very long interval so the background loop
		// does not interfere with test assertions.
		PendingChatAcquireInterval: testutil.WaitLong,
		ProviderAPIKeys:            keys,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

// seedInternalChatDeps inserts an OpenAI provider and model config
// into the database and returns the created user and model. This
// deliberately does NOT create an Anthropic provider.
func seedInternalChatDeps(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseUrl:     "",
		ApiKeyKeyID: sql.NullString{},
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	return user, model
}

func seedWorkspaceBinding(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) (database.WorkspaceTable, database.WorkspaceBuild, database.WorkspaceAgent) {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      userID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		CreatedBy:       userID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        userID,
		OrganizationID: org.ID,
	})
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		InitiatorID:    userID,
		OrganizationID: org.ID,
	})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       workspace.ID,
		JobID:             job.ID,
	})
	resource := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      job.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: resource.ID})
	return workspace, build, agent
}

// findToolByName returns the tool with the given name from the
// slice, or nil if no match is found.
func findToolByName(tools []fantasy.AgentTool, name string) fantasy.AgentTool {
	for _, tool := range tools {
		if tool.Info().Name == name {
			return tool
		}
	}
	return nil
}

func chatdTestContext(t *testing.T) context.Context {
	t.Helper()
	return dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
}

func TestCreateChildSubagentChatInheritsWorkspaceBinding(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)
	workspace, build, agent := seedWorkspaceBinding(t, db, user.ID)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspace.ID,
			Valid: true,
		},
		BuildID: uuid.NullUUID{
			UUID:  build.ID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agent.ID,
			Valid: true,
		},
		Title:              "bound-parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	child, err := server.createChildSubagentChat(ctx, parentChat, "inspect bindings", "")
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, parentChat.WorkspaceID, childChat.WorkspaceID)
	require.Equal(t, parentChat.BuildID, childChat.BuildID)
	require.Equal(t, parentChat.AgentID, childChat.AgentID)
}

func TestSpawnComputerUseAgent_NoAnthropicProvider(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	// No Anthropic key in ProviderAPIKeys.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Create a root parent chat.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-no-anthropic",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Re-fetch so LastModelConfigID is populated from the DB.
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	assert.Nil(t, tool, "spawn_computer_use_agent tool must be omitted when Anthropic is not configured")
}

func TestSpawnComputerUseAgent_NotAvailableForChildChats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	// Provide an Anthropic key so the provider check passes.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Create a root parent chat.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "root-parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Create a child chat under the parent.
	child, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		Title:              "child-subagent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do something")},
	})
	require.NoError(t, err)

	// Re-fetch the child so ParentChatID is populated.
	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, childChat.ParentChatID.Valid,
		"child chat must have a parent")

	// Get tools as if the child chat is the current chat.
	tools := server.subagentTools(ctx, func() database.Chat { return childChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	require.NotNil(t, tool, "spawn_computer_use_agent tool must be present")

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-2",
		Name:  "spawn_computer_use_agent",
		Input: `{"prompt":"open browser"}`,
	})
	require.NoError(t, err)

	assert.True(t, resp.IsError, "expected an error response")
	assert.Contains(t, resp.Content, "delegated chats cannot create child subagents")
}

func TestSpawnComputerUseAgent_DesktopDisabled(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-desktop-disabled",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	assert.Nil(t, tool, "spawn_computer_use_agent tool must be omitted when desktop is disabled")
}

func TestSpawnComputerUseAgent_UsesComputerUseModelNotParent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	// Provide an Anthropic key so the tool can proceed.
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)
	workspace, build, agent := seedWorkspaceBinding(t, db, user.ID)

	// The parent uses an OpenAI model.
	require.Equal(t, "openai", model.Provider,
		"seed helper must create an OpenAI model")

	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspace.ID,
			Valid: true,
		},
		BuildID: uuid.NullUUID{
			UUID:  build.ID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agent.ID,
			Valid: true,
		},
		Title:              "parent-openai",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	require.NotNil(t, tool)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-3",
		Name:  "spawn_computer_use_agent",
		Input: `{"prompt":"take a screenshot"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	// Parse the response to get the child chat ID.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	childIDStr, ok := result["chat_id"].(string)
	require.True(t, ok, "response must contain chat_id")

	childID, err := uuid.Parse(childIDStr)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)

	require.Equal(t, parentChat.WorkspaceID, childChat.WorkspaceID)
	require.Equal(t, parentChat.BuildID, childChat.BuildID)
	require.Equal(t, parentChat.AgentID, childChat.AgentID)

	// The child must have Mode=computer_use which causes
	// runChat to override the model to the predefined computer
	// use model instead of using the parent's model config.
	require.True(t, childChat.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, childChat.Mode.ChatMode)

	// The predefined computer use model is Anthropic, which
	// differs from the parent's OpenAI model. This confirms
	// that the child will not inherit the parent's model at
	// runtime.
	assert.NotEqual(t, model.Provider, chattool.ComputerUseModelProvider,
		"computer use model provider must differ from parent model provider")
	assert.Equal(t, "anthropic", chattool.ComputerUseModelProvider)
	assert.NotEmpty(t, chattool.ComputerUseModelName)
}

func TestCreateChildSubagentChat_InheritsMCPServerIDs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Insert two MCP server configs so we can verify both are
	// inherited by the child chat.
	mcpA, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "MCP A",
		Slug:          "mcp-a",
		Url:           "https://mcp-a.example.com",
		Transport:     "streamable_http",
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
		CreatedBy:     user.ID,
		UpdatedBy:     user.ID,
	})
	require.NoError(t, err)

	mcpB, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "MCP B",
		Slug:          "mcp-b",
		Url:           "https://mcp-b.example.com",
		Transport:     "streamable_http",
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
		CreatedBy:     user.ID,
		UpdatedBy:     user.ID,
	})
	require.NoError(t, err)

	parentMCPIDs := []uuid.UUID{mcpA.ID, mcpB.ID}

	// Create a parent chat with MCP servers.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-with-mcp",
		ModelConfigID:      model.ID,
		MCPServerIDs:       parentMCPIDs,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Refetch the parent to get DB-populated fields.
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, parentMCPIDs, parentChat.MCPServerIDs,
		"parent chat must have the MCP server IDs we set")

	// Spawn a child subagent chat.
	child, err := server.createChildSubagentChat(
		ctx,
		parentChat,
		"do some work",
		"child-task",
	)
	require.NoError(t, err)

	// Verify the child inherited the parent's MCP server IDs.
	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, parentMCPIDs, childChat.MCPServerIDs,
		"child chat must inherit MCP server IDs from parent")
}

func TestSpawnComputerUseAgent_InheritsMCPServerIDs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Insert an MCP server config.
	mcpCfg, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "MCP Test",
		Slug:          "mcp-test",
		Url:           "https://mcp.example.com",
		Transport:     "streamable_http",
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
		CreatedBy:     user.ID,
		UpdatedBy:     user.ID,
	})
	require.NoError(t, err)

	parentMCPIDs := []uuid.UUID{mcpCfg.ID}

	// Create a parent chat with MCP servers.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-cu-mcp",
		ModelConfigID:      model.ID,
		MCPServerIDs:       parentMCPIDs,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	// Call spawn_computer_use_agent via the tool.
	tools := server.subagentTools(ctx, func() database.Chat { return parentChat })
	tool := findToolByName(tools, "spawn_computer_use_agent")
	require.NotNil(t, tool)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-mcp",
		Name:  "spawn_computer_use_agent",
		Input: `{"prompt":"check the UI"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	// Parse the child chat ID from the response.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	childIDStr, ok := result["chat_id"].(string)
	require.True(t, ok)

	childID, err := uuid.Parse(childIDStr)
	require.NoError(t, err)

	// Verify the child inherited MCP server IDs.
	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	assert.ElementsMatch(t, parentMCPIDs, childChat.MCPServerIDs,
		"computer use child chat must inherit MCP server IDs from parent")
}

func TestCreateChildSubagentChat_NoMCPServersStaysEmpty(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Create a parent chat without any MCP servers.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-no-mcp",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	// Spawn a child.
	child, err := server.createChildSubagentChat(
		ctx,
		parentChat,
		"do some work",
		"child-no-mcp",
	)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Empty(t, childChat.MCPServerIDs,
		"child chat must have empty MCP server IDs when parent has none")
}

func TestIsSubagentDescendant(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	// Build a chain: root -> child -> grandchild.
	root, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("root")},
	})
	require.NoError(t, err)

	child, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		ParentChatID: uuid.NullUUID{
			UUID:  root.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  root.ID,
			Valid: true,
		},
		Title:              "child",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("child")},
	})
	require.NoError(t, err)

	grandchild, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		ParentChatID: uuid.NullUUID{
			UUID:  child.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  root.ID,
			Valid: true,
		},
		Title:              "grandchild",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("grandchild")},
	})
	require.NoError(t, err)

	// Build a separate, unrelated chain.
	unrelated, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "unrelated-root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("unrelated")},
	})
	require.NoError(t, err)

	unrelatedChild, err := server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		ParentChatID: uuid.NullUUID{
			UUID:  unrelated.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  unrelated.ID,
			Valid: true,
		},
		Title:              "unrelated-child",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("unrelated-child")},
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		ancestor uuid.UUID
		target   uuid.UUID
		want     bool
	}{
		{
			name:     "SameID",
			ancestor: root.ID,
			target:   root.ID,
			want:     false,
		},
		{
			name:     "DirectChild",
			ancestor: root.ID,
			target:   child.ID,
			want:     true,
		},
		{
			name:     "GrandChild",
			ancestor: root.ID,
			target:   grandchild.ID,
			want:     true,
		},
		{
			name:     "Unrelated",
			ancestor: root.ID,
			target:   unrelatedChild.ID,
			want:     false,
		},
		{
			name:     "RootChat",
			ancestor: child.ID,
			target:   root.ID,
			want:     false,
		},
		{
			name:     "BrokenChain",
			ancestor: root.ID,
			target:   uuid.New(),
			want:     false,
		},
		{
			name:     "NotDescendant",
			ancestor: unrelated.ID,
			target:   child.ID,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := chatdTestContext(t)
			got, err := isSubagentDescendant(ctx, db, tt.ancestor, tt.target)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// createParentChildChats creates a parent and child chat pair for
// subagent tests. The child starts in pending status.
func createParentChildChats(
	ctx context.Context,
	t *testing.T,
	server *Server,
	user database.User,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()

	parent, err := server.CreateChat(ctx, CreateOptions{
		OwnerID:            user.ID,
		Title:              "parent-" + t.Name(),
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	child, err = server.CreateChat(ctx, CreateOptions{
		OwnerID: user.ID,
		ParentChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		RootChatID: uuid.NullUUID{
			UUID:  parent.ID,
			Valid: true,
		},
		Title:              "child-" + t.Name(),
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do work")},
	})
	require.NoError(t, err)

	return parent, child
}

// setChatStatus transitions a chat to the given status.
func setChatStatus(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	status database.ChatStatus,
	lastError string,
) {
	t.Helper()

	params := database.UpdateChatStatusParams{
		ID:     chatID,
		Status: status,
	}
	if lastError != "" {
		params.LastError = sql.NullString{String: lastError, Valid: true}
	}
	_, err := db.UpdateChatStatus(ctx, params)
	require.NoError(t, err)
}

// insertAssistantMessage inserts an assistant message with v1 content
// into a chat.
func insertAssistantMessage(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	modelID uuid.UUID,
	text string,
) {
	t.Helper()

	parts := []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}
	data, err := json.Marshal(parts)
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{modelID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		Content:             []string{string(data)},
		ContentVersion:      []int16{chatprompt.ContentVersionV1},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)
}

func TestAwaitSubagentCompletion(t *testing.T) {
	t.Parallel()

	// Shared fixtures for subtests that use a real clock. Each
	// subtest creates its own parent+child chats (unique IDs)
	// so they don't collide. Mock-clock subtests need their own
	// DB and server because the Server's background tickers
	// also use the mock clock.
	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, model := seedInternalChatDeps(ctx, t, db)

	t.Run("NotDescendant", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, _ := createParentChildChats(ctx, t, server, user, model)

		unrelated, err := server.CreateChat(ctx, CreateOptions{
			OwnerID:            user.ID,
			Title:              "unrelated",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("other")},
		})
		require.NoError(t, err)

		_, _, err = server.awaitSubagentCompletion(
			ctx, parent.ID, unrelated.ID, time.Second,
		)
		require.ErrorIs(t, err, ErrSubagentNotDescendant)
	})

	t.Run("AlreadyWaiting", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		insertAssistantMessage(ctx, t, db, child.ID, model.ID, "task complete")

		gotChat, report, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, time.Second,
		)
		require.NoError(t, err)
		assert.Equal(t, child.ID, gotChat.ID)
		assert.Equal(t, database.ChatStatusWaiting, gotChat.Status)
		assert.Equal(t, "task complete", report)
	})

	t.Run("AlreadyError", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		setChatStatus(ctx, t, db, child.ID, database.ChatStatusError, "something broke")
		insertAssistantMessage(ctx, t, db, child.ID, model.ID, "partial work done")

		_, _, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, time.Second,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "partial work done")
	})

	t.Run("AlreadyErrorNoReport", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		setChatStatus(ctx, t, db, child.ID, database.ChatStatusError, "crash")

		_, _, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, time.Second,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent reached error status")
	})

	t.Run("CompletesViaPoll", func(t *testing.T) {
		t.Parallel()

		// Use nil pubsub so awaitSubagentCompletion falls back to
		// the fast 200ms poll interval.
		db, _ := dbtestutil.NewDB(t)
		mClock := quartz.NewMock(t)
		server := newInternalTestServerWithClock(t, db, nil, chatprovider.ProviderAPIKeys{}, mClock)
		ctx := chatdTestContext(t)
		user, model := seedInternalChatDeps(ctx, t, db)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		// Set the trap BEFORE starting the goroutine so we
		// deterministically catch the ticker creation.
		tickTrap := mClock.Trap().NewTicker("chatd", "subagent_poll")

		type awaitResult struct {
			chat   database.Chat
			report string
			err    error
		}
		resultCh := make(chan awaitResult, 1)
		go func() {
			chat, report, err := server.awaitSubagentCompletion(
				ctx, parent.ID, child.ID, 5*time.Second,
			)
			resultCh <- awaitResult{chat, report, err}
		}()

		// Wait for the poll ticker to be created, confirming
		// the function passed its initial check and entered
		// the loop. Then release the call.
		tickTrap.MustWait(ctx).MustRelease(ctx)
		tickTrap.Close()

		// Now set the state and advance the clock to the next
		// tick so the poll detects the transition.
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		insertAssistantMessage(ctx, t, db, child.ID, model.ID, "poll result")
		mClock.Advance(subagentAwaitPollInterval).MustWait(ctx)

		result := testutil.RequireReceive(ctx, t, resultCh)
		require.NoError(t, result.err)
		assert.Equal(t, child.ID, result.chat.ID)
		assert.Equal(t, "poll result", result.report)
	})

	t.Run("CompletesViaPubsub", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		mClock := quartz.NewMock(t)
		server := newInternalTestServerWithClock(t, db, ps, chatprovider.ProviderAPIKeys{}, mClock)
		ctx := chatdTestContext(t)
		user, model := seedInternalChatDeps(ctx, t, db)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		// signalWake from CreateChat may trigger immediate processing.
		// Wait for it to settle, then reset chats to the state we need.
		server.inflight.Wait()
		setChatStatus(ctx, t, db, parent.ID, database.ChatStatusRunning, "")
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

		// Trap the fallback poll ticker to know when the
		// function has subscribed to pubsub and entered
		// its select loop.
		tickTrap := mClock.Trap().NewTicker("chatd", "subagent_poll")

		type awaitResult struct {
			chat   database.Chat
			report string
			err    error
		}
		resultCh := make(chan awaitResult, 1)
		go func() {
			chat, report, err := server.awaitSubagentCompletion(
				ctx, parent.ID, child.ID, 5*time.Second,
			)
			resultCh <- awaitResult{chat, report, err}
		}()

		// Wait for the ticker to be created (confirms pubsub
		// subscription is set up and select loop entered).
		tickTrap.MustWait(ctx).MustRelease(ctx)
		tickTrap.Close()

		// Transition child and publish. The pubsub notification
		// wakes the function without needing a clock advance.
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		insertAssistantMessage(ctx, t, db, child.ID, model.ID, "pubsub result")
		_ = ps.Publish(
			coderdpubsub.ChatStreamNotifyChannel(child.ID),
			[]byte("done"),
		)

		result := testutil.RequireReceive(ctx, t, resultCh)
		require.NoError(t, result.err)
		assert.Equal(t, child.ID, result.chat.ID)
		assert.Equal(t, "pubsub result", result.report)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		mClock := quartz.NewMock(t)
		server := newInternalTestServerWithClock(t, db, ps, chatprovider.ProviderAPIKeys{}, mClock)
		ctx := chatdTestContext(t)
		user, model := seedInternalChatDeps(ctx, t, db)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		// Trap the timeout timer to know when the function
		// has entered its poll loop.
		timerTrap := mClock.Trap().NewTimer("chatd", "subagent_await")

		type awaitResult struct {
			err error
		}
		resultCh := make(chan awaitResult, 1)
		go func() {
			_, _, err := server.awaitSubagentCompletion(
				ctx, parent.ID, child.ID, time.Second,
			)
			resultCh <- awaitResult{err}
		}()

		// Wait for the timer to be created, release it.
		timerTrap.MustWait(ctx).MustRelease(ctx)
		timerTrap.Close()

		// Advance to the timeout. With pubsub, the fallback
		// poll is at 5s, so the 1s timer fires first.
		mClock.Advance(time.Second).MustWait(ctx)

		result := testutil.RequireReceive(ctx, t, resultCh)
		require.Error(t, result.err)
		assert.Contains(t, result.err.Error(), "timed out waiting for delegated subagent completion")
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		// signalWake from CreateChat may have triggered background
		// processing that transitions the child to "error". Wait
		// for that to finish, then reset to "running" so the test
		// exercises the context-cancellation path. Using "running"
		// (not "pending") prevents re-acquisition by the shared
		// server's background loop.
		server.inflight.Wait()
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

		// Use a short-lived context instead of goroutine + sleep.
		shortCtx, cancel := context.WithTimeout(ctx, testutil.IntervalMedium)
		defer cancel()

		_, _, err := server.awaitSubagentCompletion(
			shortCtx, parent.ID, child.ID, 5*time.Second,
		)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("ZeroTimeoutUsesDefault", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, model)

		// Pre-complete the child so it returns immediately.
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		insertAssistantMessage(ctx, t, db, child.ID, model.ID, "zero timeout ok")

		gotChat, report, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, 0,
		)
		require.NoError(t, err)
		assert.Equal(t, child.ID, gotChat.ID)
		assert.Equal(t, "zero timeout ok", report)
	})
}
