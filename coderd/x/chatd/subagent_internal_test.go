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
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

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
// into the database and returns the created user, organization,
// and model. This deliberately does NOT create an Anthropic
// provider.
func seedInternalChatDeps(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		BaseUrl:              "",
		ApiKeyKeyID:          sql.NullString{},
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
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

	return user, org, model
}

func insertInternalChatModelConfig(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	model string,
	enabled bool,
) database.ChatModelConfig {
	t.Helper()

	modelConfig, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                model,
		DisplayName:          model,
		CreatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
		Enabled:              enabled,
		IsDefault:            false,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	return modelConfig
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
	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, build, agent := seedWorkspaceBinding(t, db, user.ID)

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
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
	require.Equal(t, parentChat.OrganizationID, childChat.OrganizationID)
	require.Equal(t, parentChat.WorkspaceID, childChat.WorkspaceID)
	require.Equal(t, parentChat.BuildID, childChat.BuildID)
	require.Equal(t, parentChat.AgentID, childChat.AgentID)
}

func createInternalParentChat(
	ctx context.Context,
	t *testing.T,
	server *Server,
	db database.Store,
	orgID uuid.UUID,
	userID uuid.UUID,
	modelConfigID uuid.UUID,
	title string,
) database.Chat {
	t.Helper()

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     orgID,
		OwnerID:            userID,
		Title:              title,
		ModelConfigID:      modelConfigID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	return parentChat
}

func runSubagentTool(
	ctx context.Context,
	t *testing.T,
	server *Server,
	parentChat database.Chat,
	currentModelConfigID uuid.UUID,
	toolName string,
	args any,
) fantasy.ToolResponse {
	t.Helper()

	tools := server.subagentTools(
		ctx,
		func() database.Chat { return parentChat },
		currentModelConfigID,
	)
	tool := findToolByName(tools, toolName)
	require.NotNil(t, tool, "%s tool must be present", toolName)

	input, err := json.Marshal(args)
	require.NoError(t, err)

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    uuid.NewString(),
		Name:  toolName,
		Input: string(input),
	})
	require.NoError(t, err)

	return resp
}

func runSpawnAgentTool(
	ctx context.Context,
	t *testing.T,
	server *Server,
	parentChat database.Chat,
	args spawnAgentArgs,
) fantasy.ToolResponse {
	t.Helper()
	return runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		args,
	)
}

func requireSpawnAgentResponse(t *testing.T, resp fantasy.ToolResponse) struct {
	ChatID       string `json:"chat_id"`
	SubagentType string `json:"type"`
} {
	t.Helper()
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	var result struct {
		ChatID       string `json:"chat_id"`
		SubagentType string `json:"type"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.NotEmpty(t, result.ChatID, "response must contain chat_id")
	require.NotEmpty(t, result.SubagentType, "response must contain type")
	return result
}

func requireSpawnAgentChildChatID(t *testing.T, resp fantasy.ToolResponse) uuid.UUID {
	t.Helper()
	require.False(t, resp.IsError, "expected success but got: %s", resp.Content)

	var result struct {
		ChatID string `json:"chat_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.NotEmpty(t, result.ChatID, "response must contain chat_id")

	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)
	return childID
}

func requireToolResponseMap(
	t *testing.T,
	resp fantasy.ToolResponse,
	wantError bool,
) map[string]any {
	t.Helper()
	require.Equal(t, wantError, resp.IsError, "unexpected tool error state: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}

func TestCreateChildSubagentChatCopiesPlanMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	planMode := database.NullChatPlanMode{
		ChatPlanMode: database.ChatPlanModePlan,
		Valid:        true,
	}

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "plan-parent",
		ModelConfigID:  model.ID,
		PlanMode:       planMode,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("plan this change"),
		},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	require.Equal(t, planMode, parentChat.PlanMode)

	child, err := server.createChildSubagentChat(ctx, parentChat, "inspect bindings", "")
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, planMode, childChat.PlanMode)
}

func TestSpawnAgent_GeneralInheritsParentModelWhenOmitted(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-inherited-model",
	)

	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "delegate work",
	})
	result := requireSpawnAgentResponse(t, resp)
	require.Equal(t, subagentTypeGeneral, result.SubagentType)
	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, parentChat.LastModelConfigID, childChat.LastModelConfigID)
}

func TestCreateChildSubagentChat_OverrideWorksWhenParentHasNoModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	overrideModel := insertInternalChatModelConfig(
		ctx, t, db, user.ID, "override-no-parent-model-"+uuid.NewString(), true,
	)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-no-model",
	)

	// The chats table enforces a foreign key for last_model_config_id, so
	// use a synthetic parent value here to exercise the override path.
	parentChat.LastModelConfigID = uuid.Nil
	child, err := server.createChildSubagentChatWithOptions(
		ctx,
		parentChat,
		"delegate work",
		"",
		childSubagentChatOptions{modelConfigIDOverride: &overrideModel.ID},
	)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.Equal(t, overrideModel.ID, childChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreUsesConfiguredModelOverride(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	overrideModel := insertInternalChatModelConfig(
		ctx, t, db, user.ID, "explore-override-"+uuid.NewString(), true,
	)
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, overrideModel.ID.String()))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-explore-override",
	)

	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "investigate the codebase"},
	)
	result := requireSpawnAgentResponse(t, resp)
	require.Equal(t, subagentTypeExplore, result.SubagentType)
	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, overrideModel.ID, childChat.LastModelConfigID)
	require.True(t, childChat.Mode.Valid)
	require.Equal(t, database.ChatModeExplore, childChat.Mode.ChatMode)
	require.False(t, childChat.PlanMode.Valid)
}

func TestSpawnAgent_ExploreFallsBackToCurrentTurnModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(ctx, t, db)
	currentTurnModel := insertInternalChatModelConfig(
		ctx, t, db, user.ID, "explore-current-turn-"+uuid.NewString(), true,
	)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-fallback",
	)

	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "trace the request flow"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
	require.Equal(t, parentModel.ID, parentChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreFallsBackOnInvalidUUID(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(ctx, t, db)
	currentTurnModel := insertInternalChatModelConfig(
		ctx, t, db, user.ID, "explore-invalid-override-"+uuid.NewString(), true,
	)
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, "not-a-uuid"))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-invalid-override",
	)

	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "inspect the handler flow"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreFallsBackWhenOverrideIsUnavailable(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(ctx, t, db)
	currentTurnModel := insertInternalChatModelConfig(
		ctx, t, db, user.ID, "explore-fallback-current-"+uuid.NewString(), true,
	)
	disabledModel := insertInternalChatModelConfig(
		ctx, t, db, user.ID, "explore-disabled-"+uuid.NewString(), false,
	)
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, disabledModel.ID.String()))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-disabled",
	)

	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "inspect the service boundaries"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
}

func TestSpawnAgent_ExploreFallsBackWhenOverrideCredentialsAreUnavailable(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, parentModel := seedInternalChatDeps(ctx, t, db)
	currentTurnModel := insertInternalChatModelConfig(
		ctx, t, db, user.ID, "explore-missing-user-key-current-"+uuid.NewString(), true,
	)
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:                   "openai-compat",
		DisplayName:                "OpenAI Compat",
		APIKey:                     "",
		BaseUrl:                    "",
		CreatedBy:                  uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:                    true,
		CentralApiKeyEnabled:       false,
		AllowUserApiKey:            true,
		AllowCentralApiKeyFallback: false,
	})
	require.NoError(t, err)
	overrideModel, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai-compat",
		Model:                "gpt-4o-mini",
		DisplayName:          "Explore Override Missing User Key",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            false,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.NoError(t, db.UpsertChatExploreModelOverride(ctx, overrideModel.ID.String()))
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, parentModel.ID, "parent-explore-missing-user-key",
	)

	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		currentTurnModel.ID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeExplore, Prompt: "inspect provider credential handling"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.Equal(t, currentTurnModel.ID, childChat.LastModelConfigID)
}

func TestSpawnAgent_DescriptionListsAllAvailableTypes(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-description-all",
	)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")
	description := tool.Info().Description
	require.Contains(t, description, subagentTypeGeneral)
	require.Contains(t, description, subagentTypeExplore)
	require.Contains(t, description, subagentTypeComputerUse)
}

func TestSpawnAgent_DescriptionOmitsComputerUseWhenUnavailable(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-description-unavailable",
	)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")
	description := tool.Info().Description
	require.Contains(t, description, subagentTypeGeneral)
	require.Contains(t, description, subagentTypeExplore)
	require.NotContains(t, description, subagentTypeComputerUse)
}

func TestSpawnAgent_PlanModeDescriptionOmitsComputerUse(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "plan-parent-description",
		ModelConfigID:  model.ID,
		PlanMode: database.NullChatPlanMode{
			ChatPlanMode: database.ChatPlanModePlan,
			Valid:        true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("plan this change")},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return parentChat }, parentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")
	description := tool.Info().Description
	require.Contains(t, description, subagentTypeGeneral)
	require.Contains(t, description, subagentTypeExplore)
	require.NotContains(t, description, subagentTypeComputerUse)
	require.Contains(t, description, "must not implement changes or intentionally modify workspace files")
}

func TestSpawnAgent_PlanModeRejectsComputerUse(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "plan-parent-computer-use-reject",
		ModelConfigID:  model.ID,
		PlanMode: database.NullChatPlanMode{
			ChatPlanMode: database.ChatPlanModePlan,
			Valid:        true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("plan this change")},
	})
	require.NoError(t, err)
	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeComputerUse,
		Prompt: "open the browser and click around",
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, `type "computer_use" is unavailable in plan mode`)
}

func TestPlanningOverlaySubagentGuidance_UsesPlanModeSafeDescriptions(t *testing.T) {
	t.Parallel()

	guidance := planningOverlaySubagentGuidance()

	require.Contains(t, guidance, subagentTypeGeneral)
	require.Contains(t, guidance, subagentTypeExplore)
	require.NotContains(t, guidance, subagentTypeComputerUse)
	require.NotContains(t, guidance, "modify")
	require.NotContains(t, guidance, "may inspect or modify workspace files")
}

func TestSpawnAgent_InvalidTypeAndUnavailableTypeAreDistinct(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-invalid-type",
	)

	invalidResp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: "invalid", Prompt: "delegate work"},
	)
	require.True(t, invalidResp.IsError)
	require.Contains(t, invalidResp.Content, "type must be one of: general, explore")

	unavailableResp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeComputerUse, Prompt: "open browser"},
	)
	require.True(t, unavailableResp.IsError)
	require.Contains(t, unavailableResp.Content, `type "computer_use" is unavailable because computer use is not configured`)
}

func TestSpawnAgent_BlankTypeReturnsValidOptions(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-blank-type",
	)

	tests := []struct {
		name         string
		subagentType string
	}{
		{name: "empty", subagentType: ""},
		{name: "space", subagentType: " "},
		{name: "whitespace", subagentType: "\n\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
				Type:   tt.subagentType,
				Prompt: "delegate work",
			})
			require.True(t, resp.IsError)
			require.Contains(t, resp.Content, "type must be one of:")
			require.Contains(t, resp.Content, subagentTypeGeneral)
			require.Contains(t, resp.Content, subagentTypeExplore)
			require.Contains(t, resp.Content, subagentTypeComputerUse)
		})
	}
}

func TestSpawnAgent_NotAvailableForChildChats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	_, child := createParentChildChats(ctx, t, server, user, org, model)

	childChat, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, childChat.ParentChatID.Valid, "child chat must have a parent")

	tools := server.subagentTools(ctx, func() database.Chat { return childChat }, childChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-child",
		Name:  spawnAgentToolName,
		Input: `{"type":"general","prompt":"open browser"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "delegated chats cannot create child subagents")
}

func TestSpawnAgent_NotAvailableForExploreChats(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	exploreChat, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "root-explore",
		ModelConfigID:  model.ID,
		ChatMode: database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("inspect the codebase")},
	})
	require.NoError(t, err)
	currentChat, err := db.GetChatByID(ctx, exploreChat.ID)
	require.NoError(t, err)

	tools := server.subagentTools(ctx, func() database.Chat { return currentChat }, currentChat.LastModelConfigID)
	tool := findToolByName(tools, spawnAgentToolName)
	require.NotNil(t, tool, "spawn_agent tool must be present")

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-explore",
		Name:  spawnAgentToolName,
		Input: `{"type":"general","prompt":"delegate work"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "explore chats cannot create child subagents")
}

func TestSubagentLifecycleToolsIncludePersistedSubagentTypeAcrossVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		variant string
	}{
		{name: "General", variant: subagentTypeGeneral},
		{name: "Explore", variant: subagentTypeExplore},
		{name: "ComputerUse", variant: subagentTypeComputerUse},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			if tt.variant == subagentTypeComputerUse {
				require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
			}

			providerKeys := chatprovider.ProviderAPIKeys{}
			if tt.variant == subagentTypeComputerUse {
				providerKeys = chatprovider.ProviderAPIKeys{Anthropic: "test-anthropic-key"}
			}
			server := newInternalTestServer(t, db, ps, providerKeys)

			ctx := chatdTestContext(t)
			user, org, model := seedInternalChatDeps(ctx, t, db)
			parentChat := createInternalParentChat(
				ctx,
				t,
				server,
				db,
				org.ID,
				user.ID,
				model.ID,
				"parent-lifecycle-"+tt.variant,
			)

			spawnResp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
				Type:   tt.variant,
				Prompt: "delegate work",
			})
			spawnResult := requireSpawnAgentResponse(t, spawnResp)
			require.Equal(t, tt.variant, spawnResult.SubagentType)
			childID, err := uuid.Parse(spawnResult.ChatID)
			require.NoError(t, err)

			setChatStatus(ctx, t, db, childID, database.ChatStatusWaiting, "")
			insertAssistantMessage(ctx, t, db, childID, model.ID, "task complete")
			waitResult := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				parentChat,
				parentChat.LastModelConfigID,
				"wait_agent",
				waitAgentArgs{ChatID: childID.String()},
			), false)
			require.Equal(t, tt.variant, waitResult["type"])

			messageResult := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				parentChat,
				parentChat.LastModelConfigID,
				"message_agent",
				messageAgentArgs{ChatID: childID.String(), Message: "follow up"},
			), false)
			require.Equal(t, tt.variant, messageResult["type"])

			setChatStatus(ctx, t, db, childID, database.ChatStatusRunning, "")
			closeResult := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				parentChat,
				parentChat.LastModelConfigID,
				"close_agent",
				closeAgentArgs{ChatID: childID.String()},
			), false)
			require.Equal(t, tt.variant, closeResult["type"])
		})
	}
}

func TestSubagentLifecycleToolErrorsIncludePersistedSubagentType(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	_, child := createParentChildChats(ctx, t, server, user, org, model)
	unrelated, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "unrelated-lifecycle-parent",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("other")},
	})
	require.NoError(t, err)
	unrelatedChat, err := db.GetChatByID(ctx, unrelated.ID)
	require.NoError(t, err)

	tests := []struct {
		name      string
		toolName  string
		args      any
		wantError string
	}{
		{
			name:      "WaitAgent",
			toolName:  "wait_agent",
			args:      waitAgentArgs{ChatID: child.ID.String()},
			wantError: ErrSubagentNotDescendant.Error(),
		},
		{
			name:      "MessageAgent",
			toolName:  "message_agent",
			args:      messageAgentArgs{ChatID: child.ID.String(), Message: "follow up"},
			wantError: ErrSubagentNotDescendant.Error(),
		},
		{
			name:      "CloseAgent",
			toolName:  "close_agent",
			args:      closeAgentArgs{ChatID: child.ID.String()},
			wantError: ErrSubagentNotDescendant.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := requireToolResponseMap(t, runSubagentTool(
				ctx,
				t,
				server,
				unrelatedChat,
				unrelatedChat.LastModelConfigID,
				tt.toolName,
				tt.args,
			), true)
			require.Equal(t, subagentTypeGeneral, result["type"])
			require.Equal(t, tt.wantError, result["error"])
		})
	}
}

func TestSpawnAgent_ComputerUseUsesComputerUseModelNotParent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, build, agent := seedWorkspaceBinding(t, db, user.ID)

	require.Equal(t, "openai", model.Provider, "seed helper must create an OpenAI model")

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		WorkspaceID:        uuid.NullUUID{UUID: workspace.ID, Valid: true},
		BuildID:            uuid.NullUUID{UUID: build.ID, Valid: true},
		AgentID:            uuid.NullUUID{UUID: agent.ID, Valid: true},
		Title:              "parent-openai",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeComputerUse, Prompt: "take a screenshot"},
	)
	result := requireSpawnAgentResponse(t, resp)
	require.Equal(t, subagentTypeComputerUse, result.SubagentType)
	childID, err := uuid.Parse(result.ChatID)
	require.NoError(t, err)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)

	require.Equal(t, parentChat.WorkspaceID, childChat.WorkspaceID)
	require.Equal(t, parentChat.BuildID, childChat.BuildID)
	require.Equal(t, parentChat.AgentID, childChat.AgentID)
	require.True(t, childChat.Mode.Valid)
	assert.Equal(t, database.ChatModeComputerUse, childChat.Mode.ChatMode)
	assert.NotEqual(t, model.Provider, chattool.ComputerUseModelProvider,
		"computer use model provider must differ from parent model provider")
	assert.Equal(t, "anthropic", chattool.ComputerUseModelProvider)
	assert.NotEmpty(t, chattool.ComputerUseModelName)
}

func TestSpawnAgent_ComputerUseInheritsMCPServerIDs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	require.NoError(t, db.UpsertChatDesktopEnabled(chatdTestContext(t), true))
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{
		Anthropic: "test-anthropic-key",
	})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)

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

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent-cu-mcp",
		ModelConfigID:      model.ID,
		MCPServerIDs:       parentMCPIDs,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	parentChat, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)

	resp := runSubagentTool(
		ctx,
		t,
		server,
		parentChat,
		parentChat.LastModelConfigID,
		spawnAgentToolName,
		spawnAgentArgs{Type: subagentTypeComputerUse, Prompt: "check the UI"},
	)
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	assert.ElementsMatch(t, parentMCPIDs, childChat.MCPServerIDs,
		"computer use child chat must inherit MCP server IDs from parent")
}

func TestCreateChildSubagentChat_InheritsMCPServerIDs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)

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
		OrganizationID:     org.ID,
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

func TestCreateChildSubagentChat_NoMCPServersStaysEmpty(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)

	// Create a parent chat without any MCP servers.
	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
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
	user, org, model := seedInternalChatDeps(ctx, t, db)

	// Build a chain: root -> child -> grandchild.
	root, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("root")},
	})
	require.NoError(t, err)

	child, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
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
		OrganizationID: org.ID,
		OwnerID:        user.ID,
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
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "unrelated-root",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("unrelated")},
	})
	require.NoError(t, err)

	unrelatedChild, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
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
	org database.Organization,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()

	parent, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "parent-" + t.Name(),
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	child, err = server.CreateChat(ctx, CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
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

func insertLinkedChatFile(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	name string,
	mediaType string,
	data []byte,
) uuid.UUID {
	t.Helper()

	file, err := db.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
		Name:           name,
		Mimetype:       mediaType,
		Data:           data,
	})
	require.NoError(t, err)

	rejected, err := db.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{file.ID},
	})
	require.NoError(t, err)
	require.Zero(t, rejected)

	return file.ID
}

func TestWaitAgentDoesNotRelayComputerUseSubagentAttachments(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, agent := seedWorkspaceBinding(t, db, user.ID)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	parent, child := createComputerUseParentChild(
		ctx, t, server, user, org, model, workspace, agent,
		"parent-relay", "child-relay",
	)

	insertedFile := insertLinkedChatFile(
		ctx,
		t,
		db,
		child.ID,
		user.ID,
		workspace.OrganizationID,
		"screenshot.png",
		"image/png",
		[]byte("fake-png"),
	)
	insertAssistantMessage(ctx, t, db, child.ID, model.ID, "Shared the screenshot.")
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, "Shared the screenshot.", result["report"])
	require.Equal(t, string(database.ChatStatusWaiting), result["status"])
	assert.NotContains(t, result, "attachment_count")
	assert.NotContains(t, result, "attachment_warning")

	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)
	parts := buildAssistantPartsForPersist(
		context.Background(),
		testutil.Logger(t),
		nil,
		[]fantasy.ToolResultContent{{
			ToolCallID:     "call-1",
			ToolName:       "wait_agent",
			ClientMetadata: resp.Metadata,
		}},
		chatloop.PersistedStep{},
		nil,
	)
	assert.Empty(t, parts)

	parentFiles, err := db.GetChatFileMetadataByChatID(ctx, parent.ID)
	require.NoError(t, err)
	assert.Empty(t, parentFiles)

	childFiles, err := db.GetChatFileMetadataByChatID(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, childFiles, 1)
	assert.Equal(t, insertedFile, childFiles[0].ID)
	assert.Equal(t, "screenshot.png", childFiles[0].Name)
	assert.Equal(t, "image/png", childFiles[0].Mimetype)
}

func TestWaitAgentDoesNotRelayRegularSubagentAttachments(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(ctx, t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	parent, child := createParentChildChats(ctx, t, server, user, org, model)
	server.drainInflight()

	insertedFile := insertLinkedChatFile(
		ctx,
		t,
		db,
		child.ID,
		user.ID,
		workspace.OrganizationID,
		"notes.txt",
		"text/plain",
		[]byte("release notes"),
	)
	insertAssistantMessage(ctx, t, db, child.ID, model.ID, "Shared the release notes.")
	setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

	resp, err := invokeWaitAgentTool(ctx, t, server, db, parent.ID, child.ID, 5)
	require.NoError(t, err)
	require.False(t, resp.IsError, "expected successful response, got: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, "Shared the release notes.", result["report"])
	assert.NotContains(t, result, "attachment_count")
	assert.NotContains(t, result, "attachment_warning")
	attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
	require.NoError(t, err)
	assert.Empty(t, attachments)

	parentFiles, err := db.GetChatFileMetadataByChatID(ctx, parent.ID)
	require.NoError(t, err)
	assert.Empty(t, parentFiles)

	childFiles, err := db.GetChatFileMetadataByChatID(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, childFiles, 1)
	assert.Equal(t, insertedFile, childFiles[0].ID)
	assert.Equal(t, "notes.txt", childFiles[0].Name)
	assert.Equal(t, "text/plain", childFiles[0].Mimetype)
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
	user, org, model := seedInternalChatDeps(ctx, t, db)

	t.Run("NotDescendant", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, _ := createParentChildChats(ctx, t, server, user, org, model)

		unrelated, err := server.CreateChat(ctx, CreateOptions{
			OrganizationID:     org.ID,
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

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

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

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

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

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

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
		user, org, model := seedInternalChatDeps(ctx, t, db)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

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
		user, org, model := seedInternalChatDeps(ctx, t, db)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		// signalWake from CreateChat may trigger immediate processing.
		// Wait for it to settle, then reset chats to the state we need.
		server.drainInflight()
		setChatStatus(ctx, t, db, parent.ID, database.ChatStatusRunning, "")
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusRunning, "")

		// Trap the fallback poll ticker to know when the
		// function has entered the wait setup path. We still
		// need an explicit subscription handshake below because
		// the ticker can be created before SubscribeWithErr has
		// finished registering the listener.
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

		// Wait for the ticker to be created so the waiter has
		// entered its setup path, then subscribe our own probe on
		// the same channel. Because MemoryPubsub publishes only to
		// listeners already present at Publish time, waiting for
		// our probe to receive a message proves the waiter's
		// subscription is also registered before we assert on the
		// wake-up behavior.
		tickTrap.MustWait(ctx).MustRelease(ctx)
		tickTrap.Close()

		probeCh := make(chan struct{}, 1)
		cancelProbe, err := ps.SubscribeWithErr(
			coderdpubsub.ChatStreamNotifyChannel(child.ID),
			func(_ context.Context, _ []byte, _ error) {
				select {
				case probeCh <- struct{}{}:
				default:
				}
			},
		)
		require.NoError(t, err)
		defer cancelProbe()

		// Insert the message BEFORE transitioning to Waiting.
		// Stale PG LISTEN/NOTIFY notifications from the
		// processor's earlier run can still be buffered in the
		// pgListener after drainInflight returns. If such a
		// notification is dispatched between setChatStatus and
		// insertAssistantMessage, checkSubagentCompletion would
		// see done=true (Waiting) with an empty report. By
		// inserting the message first, the report is guaranteed
		// to be committed before the status makes it visible.
		insertAssistantMessage(ctx, t, db, child.ID, model.ID, "pubsub result")
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			chat, report, done, err := server.checkSubagentCompletion(ctx, child.ID)
			require.NoError(c, err)
			assert.True(c, done)
			assert.Equal(c, child.ID, chat.ID)
			assert.Equal(c, "pubsub result", report)
		}, testutil.WaitMedium, testutil.IntervalFast)
		require.NoError(t, ps.Publish(
			coderdpubsub.ChatStreamNotifyChannel(child.ID),
			[]byte("done"),
		))
		testutil.RequireReceive(ctx, t, probeCh)

		result := testutil.RequireReceive(ctx, t, resultCh)
		require.NoError(t, result.err)
		assert.Equal(t, child.ID, result.chat.ID)
		assert.Equal(t, "pubsub result", result.report)
	})

	t.Run("AlreadyWaitingNoReport", func(t *testing.T) {
		t.Parallel()
		ctx := chatdTestContext(t)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		// signalWake from CreateChat may trigger immediate processing.
		// Wait for it to settle, then set the terminal state we need.
		// This case should return immediately, so use the shared
		// real-clock server instead of a mock clock.
		server.drainInflight()
		setChatStatus(ctx, t, db, child.ID, database.ChatStatusWaiting, "")

		gotChat, report, err := server.awaitSubagentCompletion(
			ctx, parent.ID, child.ID, 5*time.Second,
		)
		require.NoError(t, err)
		assert.Equal(t, child.ID, gotChat.ID)
		assert.Empty(t, report)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		mClock := quartz.NewMock(t)
		server := newInternalTestServerWithClock(t, db, ps, chatprovider.ProviderAPIKeys{}, mClock)
		ctx := chatdTestContext(t)
		user, org, model := seedInternalChatDeps(ctx, t, db)

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

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

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

		// signalWake from CreateChat triggers background
		// processing. drainInflight waits for in-flight goroutines
		// but can't guarantee a pending DB row has been acquired
		// yet — the child chat may still be pending if the second
		// wake signal hasn't been consumed. Poll until the child
		// reaches a terminal DB state so processChat has fully
		// finished, then reset to running for the cancellation
		// test.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			c, err := db.GetChatByID(ctx, child.ID)
			if err != nil {
				return false
			}
			return c.Status != database.ChatStatusPending && c.Status != database.ChatStatusRunning
		}, testutil.IntervalFast)
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

		parent, child := createParentChildChats(ctx, t, server, user, org, model)

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
