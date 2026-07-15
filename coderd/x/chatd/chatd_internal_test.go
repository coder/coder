package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	openaicomputeruse "github.com/coder/coder/v2/coderd/x/chatd/chatopenai/computeruse"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type testAgentTool struct {
	info            fantasy.ToolInfo
	providerOptions fantasy.ProviderOptions
}

func newTestAgentTool(name string) fantasy.AgentTool {
	return &testAgentTool{info: fantasy.ToolInfo{Name: name}}
}

func (t *testAgentTool) Info() fantasy.ToolInfo {
	return t.info
}

func (t *testAgentTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	_ = t
	return fantasy.ToolResponse{}, nil
}

func (t *testAgentTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *testAgentTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}

type testMCPAgentTool struct {
	*testAgentTool
	configID uuid.UUID
}

func newTestMCPAgentTool(name string, configID uuid.UUID) fantasy.AgentTool {
	return &testMCPAgentTool{
		testAgentTool: &testAgentTool{info: fantasy.ToolInfo{Name: name}},
		configID:      configID,
	}
}

func (t *testMCPAgentTool) MCPServerConfigID() uuid.UUID {
	return t.configID
}

func TestComputerUseProviderAndModelFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		rawProvider  string
		wantProvider codersdk.ChatComputerUseProvider
		wantErr      string
	}{
		{
			name:         "DefaultAnthropic",
			rawProvider:  "",
			wantProvider: codersdk.ChatComputerUseProviderAnthropic,
		},
		{
			name:         "OpenAI",
			rawProvider:  " openai ",
			wantProvider: codersdk.ChatComputerUseProviderOpenAI,
		},
		{
			name:        "Unknown",
			rawProvider: "bogus",
			wantErr:     `unknown computer-use provider "bogus" configured in agents_computer_use_provider`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			server := &Server{db: db}

			db.EXPECT().GetChatComputerUseProvider(gomock.Any()).DoAndReturn(
				func(ctx context.Context) (string, error) {
					_, ok := dbauthz.ActorFromContext(ctx)
					require.True(t, ok, "config reads must have an actor")
					return tt.rawProvider, nil
				},
			)

			provider, modelProvider, modelName, err := server.computerUseProviderAndModelFromConfig(context.Background())
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantProvider, provider)

			wantModelProvider, wantModelName, ok := chattool.DefaultComputerUseModel(tt.wantProvider)
			require.True(t, ok)
			require.Equal(t, wantModelProvider, modelProvider)
			require.Equal(t, wantModelName, modelName)
		})
	}
}

func TestResolveUserProviderAPIKeysAndProviderForProviderTypeProviderMatch(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()
	providerID := uuid.New()

	db.EXPECT().GetAIProviders(gomock.Any(), database.GetAIProvidersParams{}).Return([]database.AIProvider{
		{ID: uuid.New(), Type: database.AIProviderTypeAnthropic, Enabled: true},
		{ID: providerID, Type: database.AIProviderTypeOpenai, Enabled: true},
	}, nil)
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil)

	server := &Server{db: db}
	keys, aiProvider, err := server.resolveUserProviderAPIKeysAndProviderForProviderType(
		ctx,
		ownerID,
		string(codersdk.ChatComputerUseProviderOpenAI),
	)
	require.NoError(t, err)
	require.Equal(t, "test-key", keys.APIKey(string(codersdk.ChatComputerUseProviderOpenAI)))
	require.NotNil(t, aiProvider)
	require.Equal(t, providerID, aiProvider.ID)
	require.Equal(t, database.AIProviderTypeOpenai, aiProvider.Type)
}

func TestResolveModelRouteForProviderTypeAIGatewayRequiresProvider(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	db.EXPECT().GetAIProviders(gomock.Any(), database.GetAIProvidersParams{}).Return(nil, nil)

	server := &Server{db: db}
	_, err := server.resolveModelRouteForProviderType(
		ctx,
		uuid.New(),
		string(codersdk.ChatComputerUseProviderOpenAI),
	)
	require.ErrorContains(t, err, "AI Gateway routing requires a usable AI provider")
}

func TestAppendComputerUseProviderTool(t *testing.T) {
	t.Parallel()

	providerTools, err := appendComputerUseProviderTool(
		nil,
		computerUseProviderToolOptions{
			provider:      codersdk.ChatComputerUseProviderOpenAI,
			isComputerUse: true,
			logger:        slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		},
	)
	require.NoError(t, err)
	require.Len(t, providerTools, 1)
	require.True(t, openaicomputeruse.IsTool(providerTools[0].Definition))
	require.Equal(t, "computer", providerTools[0].Definition.GetName())
	require.Equal(t, "computer", providerTools[0].Runner.Info().Name)
	require.NotNil(t, providerTools[0].ResultProviderMetadata)

	metadata := providerTools[0].ResultProviderMetadata(
		fantasy.NewImageResponse([]byte("png"), "image/png"),
	)
	require.NotNil(t, metadata)

	errorResponse := fantasy.NewTextErrorResponse("failed")
	require.Nil(t, providerTools[0].ResultProviderMetadata(errorResponse))
	require.Nil(t, providerTools[0].ResultProviderMetadata(fantasy.NewTextResponse("not media")))
}

func TestAppendComputerUseProviderTool_Gates(t *testing.T) {
	t.Parallel()

	baseTools := []chatloop.ProviderTool{{
		Definition: fantasy.ProviderDefinedTool{
			ID:   "web_search",
			Name: "web_search",
		},
	}}

	tests := []struct {
		name           string
		isPlanModeTurn bool
		isComputerUse  bool
	}{
		{name: "PlanMode", isPlanModeTurn: true, isComputerUse: true},
		// Non-computer-use includes regular, master, general, and explore chats.
		// Mode cannot be both ChatModeComputerUse and another chat mode.
		{name: "NonComputerUseModes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			providerTools, err := appendComputerUseProviderTool(
				baseTools,
				computerUseProviderToolOptions{
					provider:       codersdk.ChatComputerUseProviderOpenAI,
					isPlanModeTurn: tt.isPlanModeTurn,
					isComputerUse:  tt.isComputerUse,
				},
			)
			require.NoError(t, err)
			require.Len(t, providerTools, 1)
			require.Equal(t, "web_search", providerTools[0].Definition.GetName())
		})
	}
}

func TestAppendComputerUseProviderTool_AnthropicHasNoResultMetadata(t *testing.T) {
	t.Parallel()

	providerTools, err := appendComputerUseProviderTool(
		nil,
		computerUseProviderToolOptions{
			provider:      codersdk.ChatComputerUseProviderAnthropic,
			isComputerUse: true,
			logger:        slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		},
	)
	require.NoError(t, err)
	require.Len(t, providerTools, 1)
	require.Equal(t, "computer", providerTools[0].Definition.GetName())
	require.Nil(t, providerTools[0].ResultProviderMetadata)
}

func TestFilterExternalMCPConfigsForTurn(t *testing.T) {
	t.Parallel()

	approvedConfig := database.MCPServerConfig{ID: uuid.New(), AllowInPlanMode: true}
	blockedConfig := database.MCPServerConfig{ID: uuid.New(), AllowInPlanMode: false}
	configs := []database.MCPServerConfig{approvedConfig, blockedConfig}
	planMode := database.NullChatPlanMode{
		ChatPlanMode: database.ChatPlanModePlan,
		Valid:        true,
	}

	t.Run("NonPlanModePassesThroughAllConfigs", func(t *testing.T) {
		t.Parallel()

		filtered, approvedIDs := filterExternalMCPConfigsForTurn(
			configs,
			database.NullChatPlanMode{},
			uuid.NullUUID{},
		)

		require.Equal(t, configs, filtered)
		require.Nil(t, approvedIDs)
	})

	t.Run("PlanModeSubagentsReturnNoConfigs", func(t *testing.T) {
		t.Parallel()

		filtered, approvedIDs := filterExternalMCPConfigsForTurn(
			configs,
			planMode,
			uuid.NullUUID{UUID: uuid.New(), Valid: true},
		)

		require.Nil(t, filtered)
		require.NotNil(t, approvedIDs)
		require.Empty(t, approvedIDs)
	})

	t.Run("PlanModeRootFiltersToApprovedConfigs", func(t *testing.T) {
		t.Parallel()

		filtered, approvedIDs := filterExternalMCPConfigsForTurn(
			configs,
			planMode,
			uuid.NullUUID{},
		)

		require.Equal(t, []database.MCPServerConfig{approvedConfig}, filtered)
		require.Equal(t, map[uuid.UUID]struct{}{approvedConfig.ID: {}}, approvedIDs)
	})
}

func TestChatWorkspaceRecoveryErrorsDifferentiateSignalStrength(t *testing.T) {
	t.Parallel()

	// Disconnected recovery is gated by a DB-confirmed duration
	// threshold, so the message can give direct stop/start guidance
	// without asking the user.
	disconnected := errChatAgentDisconnected.Error()
	require.Contains(t, disconnected, "90 seconds")
	require.Contains(t, disconnected, "stop_workspace")
	require.Contains(t, disconnected, "start_workspace")
	require.NotContains(t, disconnected, "ask_user_question")

	neverConnected := errChatAgentNeverConnected.Error()
	require.Contains(t, neverConnected, "stop_workspace")
	require.Contains(t, neverConnected, "start_workspace")
	require.NotContains(t, neverConnected, "ask_user_question")

	dialTimeout := errChatDialTimeout.Error()
	require.NotContains(t, dialTimeout, "ask_user_question")
	require.NotContains(t, dialTimeout, "stop_workspace")
	require.NotContains(t, dialTimeout, "start_workspace")
}

func TestActiveToolNamesForTurn(t *testing.T) {
	t.Parallel()

	makeTools := func(names ...string) []fantasy.AgentTool {
		tools := make([]fantasy.AgentTool, 0, len(names))
		for _, name := range names {
			tools = append(tools, newTestAgentTool(name))
		}
		return tools
	}

	planMode := database.NullChatPlanMode{
		ChatPlanMode: database.ChatPlanModePlan,
		Valid:        true,
	}

	t.Run("NormalModeReturnsAllRegisteredTools", func(t *testing.T) {
		t.Parallel()

		got := activeToolNamesForTurn(makeTools(
			"read_file",
			"propose_plan",
			"custom_tool",
			"execute",
		), database.NullChatPlanMode{}, uuid.NullUUID{}, nil)

		require.Equal(t, []string{
			"read_file",
			"propose_plan",
			"custom_tool",
			"execute",
		}, got)
	})

	t.Run("PlanModeIncludesOnlyAllowlistedBuiltIns", func(t *testing.T) {
		t.Parallel()

		got := activeToolNamesForTurn(makeTools(
			"read_file",
			"write_file",
			"edit_files",
			"execute",
			"process_output",
			"process_list",
			"process_signal",
			"list_templates",
			"read_template",
			"create_workspace",
			"start_workspace",
			"stop_workspace",
			"propose_plan",
			"spawn_agent",
			"wait_agent",
			"message_agent",
			"interrupt_agent",
			"list_agents",
			"read_skill",
			"read_skill_file",
			"ask_user_question",
		), planMode, uuid.NullUUID{}, nil)

		require.Equal(t, []string{
			"read_file",
			"write_file",
			"edit_files",
			"execute",
			"process_output",
			"list_templates",
			"read_template",
			"create_workspace",
			"start_workspace",
			"stop_workspace",
			"propose_plan",
			"spawn_agent",
			"wait_agent",
			"list_agents",
			"read_skill",
			"read_skill_file",
			"ask_user_question",
		}, got)
	})

	t.Run("PlanModeChildChatsAllowExplorationOnly", func(t *testing.T) {
		t.Parallel()

		got := activeToolNamesForTurn(makeTools(
			"read_file",
			"write_file",
			"edit_files",
			"execute",
			"process_output",
			"list_templates",
			"read_template",
			"create_workspace",
			"start_workspace",
			"stop_workspace",
			"propose_plan",
			"spawn_agent",
			"wait_agent",
			"read_skill",
			"read_skill_file",
			"ask_user_question",
		), planMode, uuid.NullUUID{UUID: uuid.New(), Valid: true}, nil)

		require.Equal(t, []string{
			"read_file",
			"execute",
			"process_output",
			"read_skill",
			"read_skill_file",
		}, got)
		require.NotContains(t, got, "write_file")
		require.NotContains(t, got, "edit_files")
		require.NotContains(t, got, "ask_user_question")
		require.NotContains(t, got, "propose_plan")
		require.NotContains(t, got, "start_workspace")
		require.NotContains(t, got, "stop_workspace")
		require.NotContains(t, got, "spawn_explore_agent")
	})

	t.Run("PlanModeStillExcludesDangerousTools", func(t *testing.T) {
		t.Parallel()

		got := activeToolNamesForTurn(makeTools(
			"execute",
			"process_output",
			"message_agent",
			"spawn_computer_use_agent",
			"propose_plan",
		), planMode, uuid.NullUUID{}, nil)

		require.Equal(t, []string{"execute", "process_output", "propose_plan"}, got)
		require.NotContains(t, got, "message_agent")
		require.NotContains(t, got, "spawn_computer_use_agent")
	})

	t.Run("PlanModeExcludesUnknownTools", func(t *testing.T) {
		t.Parallel()

		got := activeToolNamesForTurn(makeTools(
			"read_file",
			"custom_tool",
			"another_custom_tool",
			"propose_plan",
		), planMode, uuid.NullUUID{}, nil)

		require.Equal(t, []string{
			"read_file",
			"propose_plan",
		}, got)
		require.NotContains(t, got, "custom_tool")
		require.NotContains(t, got, "another_custom_tool")
	})

	t.Run("PlanModeIncludesOnlyApprovedExternalMCPTools", func(t *testing.T) {
		t.Parallel()

		approvedConfigID := uuid.New()
		blockedConfigID := uuid.New()
		got := activeToolNamesForTurn([]fantasy.AgentTool{
			newTestAgentTool("read_file"),
			newTestMCPAgentTool("approved-mcp__echo", approvedConfigID),
			newTestMCPAgentTool("blocked-mcp__echo", blockedConfigID),
			newTestAgentTool("workspace-mcp__echo"),
		}, planMode, uuid.NullUUID{}, map[uuid.UUID]struct{}{
			approvedConfigID: {},
		})

		require.Equal(t, []string{
			"read_file",
			"approved-mcp__echo",
		}, got)
		require.NotContains(t, got, "blocked-mcp__echo")
		require.NotContains(t, got, "workspace-mcp__echo")
	})
}

func TestAllowedExploreToolNames(t *testing.T) {
	t.Parallel()

	externalConfigID := uuid.New()
	got := allowedExploreToolNames([]fantasy.AgentTool{
		newTestAgentTool("read_file"),
		newTestAgentTool("write_file"),
		newTestMCPAgentTool("external-mcp__echo", externalConfigID),
		newTestAgentTool("workspace-mcp__echo"),
		newTestAgentTool("start_workspace"),
		newTestAgentTool("stop_workspace"),
		newTestAgentTool("execute"),
		newTestAgentTool("process_output"),
		newTestAgentTool("process_list"),
		newTestAgentTool("process_signal"),
		newTestAgentTool("spawn_agent"),
		newTestAgentTool("wait_agent"),
		newTestAgentTool("read_skill"),
		newTestAgentTool("read_skill_file"),
		newTestAgentTool("ask_user_question"),
	})

	require.Equal(t, []string{
		"read_file",
		"external-mcp__echo",
		"execute",
		"process_output",
		"read_skill",
		"read_skill_file",
	}, got)
	require.NotContains(t, got, "workspace-mcp__echo")
	require.NotContains(t, got, "start_workspace")
	require.NotContains(t, got, "stop_workspace")
	require.NotContains(t, got, "ask_user_question")
}

func TestAllowedBehaviorToolNames(t *testing.T) {
	t.Parallel()

	makeTools := func(names ...string) []fantasy.AgentTool {
		tools := make([]fantasy.AgentTool, 0, len(names))
		for _, name := range names {
			tools = append(tools, newTestAgentTool(name))
		}
		return tools
	}

	allTools := makeTools("read_file", "custom_tool", "spawn_agent")
	exploreMode := database.NullChatMode{
		ChatMode: database.ChatModeExplore,
		Valid:    true,
	}

	t.Run("DefaultModeReturnsAllTools", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"read_file", "custom_tool", "spawn_agent"}, allowedBehaviorToolNames(
			allTools,
			database.NullChatMode{},
		))
	})

	t.Run("ExploreModeUsesExploreAllowlist", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []string{"read_file"}, allowedBehaviorToolNames(
			allTools,
			exploreMode,
		))
	})
}

func TestStopAfterPlanTools(t *testing.T) {
	t.Parallel()

	planMode := database.NullChatPlanMode{
		ChatPlanMode: database.ChatPlanModePlan,
		Valid:        true,
	}

	t.Run("NormalModeReturnsNil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, stopAfterPlanTools(database.NullChatPlanMode{}, uuid.NullUUID{}))
	})

	t.Run("RootPlanModeIncludesClarificationTool", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, map[string]struct{}{
			"propose_plan":      {},
			"ask_user_question": {},
		}, stopAfterPlanTools(planMode, uuid.NullUUID{}))
	})

	t.Run("ChildPlanModeSkipsClarificationTool", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, map[string]struct{}{
			"propose_plan": {},
		}, stopAfterPlanTools(planMode, uuid.NullUUID{UUID: uuid.New(), Valid: true}))
	})
}

func TestStopAfterBehaviorTools(t *testing.T) {
	t.Parallel()

	planMode := database.NullChatPlanMode{
		ChatPlanMode: database.ChatPlanModePlan,
		Valid:        true,
	}
	exploreMode := database.NullChatMode{
		ChatMode: database.ChatModeExplore,
		Valid:    true,
	}

	t.Run("DefaultModeReturnsNil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, stopAfterBehaviorTools(
			database.NullChatPlanMode{},
			database.NullChatMode{},
			uuid.NullUUID{},
		))
	})

	t.Run("PlanModeDelegatesToPlanTools", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, stopAfterPlanTools(planMode, uuid.NullUUID{}), stopAfterBehaviorTools(
			planMode,
			database.NullChatMode{},
			uuid.NullUUID{},
		))
	})

	t.Run("ExploreModeReturnsNil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, stopAfterBehaviorTools(planMode, exploreMode, uuid.NullUUID{}))
	})
}

// TestWaitForActiveChatStop and TestWaitForActiveChatStop_WaitsForReplacementRun
// were removed along with the process-local activeChats mechanism.
// Debug cleanup is now best-effort; stale finalization handles orphaned rows.

// TestArchiveChatWaitsForActiveChatStop and
// TestArchiveChatWaitsForEveryInterruptedChat were removed along with
// the process-local activeChats mechanism. Archive cleanup is now
// best-effort; stale finalization handles any orphaned rows.

func TestRenameChatTitle(t *testing.T) {
	t.Parallel()

	t.Run("WritesAndReturnsWroteTrue", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		chatID := uuid.New()
		workerID := uuid.New()
		stored := database.Chat{
			ID:       chatID,
			Status:   database.ChatStatusRunning,
			WorkerID: uuid.NullUUID{UUID: workerID, Valid: true},
			Title:    "original",
		}
		updated := stored
		updated.Title = "renamed"

		server := &Server{db: db, logger: logger}

		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(stored, nil)
		db.EXPECT().UpdateChatTitleByID(gomock.Any(), database.UpdateChatTitleByIDParams{
			ID:    chatID,
			Title: "renamed",
		}).Return(updated, nil)

		got, wrote, err := server.RenameChatTitle(ctx, stored, "renamed")
		require.NoError(t, err)
		require.True(t, wrote, "fresh rename must report wrote=true")
		require.Equal(t, updated, got)
	})

	t.Run("SkipsWriteWhenAlreadyAtNewTitle", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		chatID := uuid.New()
		workerID := uuid.New()
		stale := database.Chat{
			ID:       chatID,
			Status:   database.ChatStatusRunning,
			WorkerID: uuid.NullUUID{UUID: workerID, Valid: true},
			Title:    "pre-race",
		}
		landed := stale
		landed.Title = "landed-concurrently"

		server := &Server{db: db, logger: logger}

		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(landed, nil)

		got, wrote, err := server.RenameChatTitle(ctx, stale, "landed-concurrently")
		require.NoError(t, err)
		require.False(t, wrote,
			"must report wrote=false when the stored row already matches newTitle so the handler suppresses a redundant title_change event")
		require.Equal(t, landed, got)
	})
}

func withChatMessageAPIKeyID(message database.ChatMessage, apiKeyID string) database.ChatMessage {
	message.APIKeyID = sqlNullString(apiKeyID)
	return message
}

// requireOutgoingRequestModel asserts that the outgoing request body
// requests wantModel. This is so that mock transports can still
// verify the outgoing request asked for the expected model.
func requireOutgoingRequestModel(t testing.TB, req *http.Request, wantModel string) {
	t.Helper()

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	var decoded struct {
		Model string `json:"model"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Equal(t, wantModel, decoded.Model)
}

func TestRegenerateChatTitle_PersistsAndBroadcasts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	usageTx := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	pubsub := dbpubsub.NewInMemory()
	clock := quartz.NewReal()

	ownerID := uuid.New()
	chatID := uuid.New()
	modelConfigID := uuid.New()
	workerID := uuid.New()
	userPrompt := "review pull request 23633 and fix review threads"
	activeAPIKeyID := "key-" + uuid.NewString()
	wantTitle := "Review PR 23633"

	chat := database.Chat{
		ID:                chatID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Status:            database.ChatStatusRunning,
		WorkerID:          uuid.NullUUID{UUID: workerID, Valid: true},
		Title:             chatprompt.FallbackTitle(userPrompt),
	}
	providerID := uuid.New()
	modelConfig := database.ChatModelConfig{
		ID:           modelConfigID,
		Model:        "gpt-4o-mini",
		ContextLimit: 8192,
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
	}
	updatedChat := chat
	updatedChat.Title = wantTitle

	messageEvents := make(chan struct {
		payload codersdk.ChatWatchEvent
		err     error
	}, 1)
	cancelSub, err := pubsub.SubscribeWithErr(
		coderdpubsub.ChatWatchEventChannel(ownerID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, payload codersdk.ChatWatchEvent, err error) {
			messageEvents <- struct {
				payload codersdk.ChatWatchEvent
				err     error
			}{payload: payload, err: err}
		}),
	)
	require.NoError(t, err)
	defer cancelSub()

	// Title generation routes through the transport factory, so the model
	// response is synthesized by the RoundTripper (see aibridgeTestFactory).
	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireOutgoingRequestModel(t, req, modelConfig.Model)
		text := strconv.Quote(`{"title":"` + wantTitle + `"}`)
		body := `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4o-mini","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":` + text + `}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}

	server := &Server{
		db:                       db,
		logger:                   logger,
		pubsub:                   pubsub,
		clock:                    quartz.NewReal(),
		configCache:              newChatConfigCache(context.Background(), db, clock),
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}

	db.EXPECT().GetChatModelConfigByID(gomock.Any(), modelConfigID).Return(modelConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
		ID:      providerID,
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}, nil).AnyTimes()

	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{
		ID:      providerID,
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}}, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{ProviderID: providerID, APIKey: "test-key"}}, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), gomock.Any()).Return([]database.AIProviderKey{{ProviderID: providerID, APIKey: "test-key"}}, nil).AnyTimes()
	db.EXPECT().GetChatUsageLimitConfig(gomock.Any()).Return(database.ChatUsageLimitConfig{}, sql.ErrNoRows)
	db.EXPECT().GetChatMessagesByChatIDAscPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDAscPaginatedParams{
			ChatID:   chatID,
			AfterID:  0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return([]database.ChatMessage{
		withChatMessageAPIKeyID(mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText(userPrompt),
		), activeAPIKeyID),
		mustChatMessage(
			t,
			database.ChatMessageRoleAssistant,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText("checking the diff now"),
		),
	}, nil)
	db.EXPECT().GetChatMessagesByChatIDDescPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDDescPaginatedParams{
			ChatID:   chatID,
			BeforeID: 0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return(nil, nil)
	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return(nil, nil)

	db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
		func(fn func(database.Store) error, opts *database.TxOptions) error {
			require.Nil(t, opts)
			return fn(usageTx)
		},
	)

	usageTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(chat, nil)
	usageTx.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
		ID:    chatID,
		Title: wantTitle,
	}).Return(updatedChat, nil)

	gotChat, err := server.RegenerateChatTitle(ctx, chat)
	require.NoError(t, err)
	require.Equal(t, updatedChat, gotChat)

	select {
	case event := <-messageEvents:
		require.NoError(t, event.err)
		require.Equal(t, codersdk.ChatWatchEventKindTitleChange, event.payload.Kind)
		require.Equal(t, chatID, event.payload.Chat.ID)
		require.Equal(t, wantTitle, event.payload.Chat.Title)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for title change pubsub event")
	}
}

// With no request-level locking, persistManualTitle's re-read under
// GetChatByIDForUpdate is the only protection against clobbering a title
// that changed while the model call ran. The strict mock has no
// UpdateChatByID expectation, so any persist attempt fails the test.
// A skipped persist must also not publish a title_change event; the
// wroteTitle comment in regenerateChatTitleWithStore explains why.
func TestRegenerateChatTitle_SkipsPersistWhenTitleChangedConcurrently(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	usageTx := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	pubsub := dbpubsub.NewInMemory()
	clock := quartz.NewReal()

	ownerID := uuid.New()
	chatID := uuid.New()
	modelConfigID := uuid.New()
	providerID := uuid.New()
	userPrompt := "review pull request 23633 and fix review threads"
	activeAPIKeyID := "key-" + uuid.NewString()
	generatedTitle := "Review PR 23633"

	chat := database.Chat{
		ID:                chatID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Status:            database.ChatStatusWaiting,
		Title:             chatprompt.FallbackTitle(userPrompt),
	}
	modelConfig := database.ChatModelConfig{
		ID:           modelConfigID,
		Model:        "gpt-4o-mini",
		ContextLimit: 8192,
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
	}
	// Another writer (rename or a second regenerate) landed while the
	// model call was in flight.
	landedChat := chat
	landedChat.Title = "landed-concurrently"

	titleEvents := make(chan codersdk.ChatWatchEvent, 1)
	cancelSub, err := pubsub.SubscribeWithErr(
		coderdpubsub.ChatWatchEventChannel(ownerID),
		coderdpubsub.HandleChatWatchEvent(func(_ context.Context, payload codersdk.ChatWatchEvent, err error) {
			require.NoError(t, err)
			titleEvents <- payload
		}),
	)
	require.NoError(t, err)
	defer cancelSub()

	factory := &aibridgeTestFactory{rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requireOutgoingRequestModel(t, req, modelConfig.Model)
		text := strconv.Quote(`{"title":"` + generatedTitle + `"}`)
		body := `{"id":"resp_test","object":"response","created_at":0,"status":"completed","model":"gpt-4o-mini","output":[{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"output_text","text":` + text + `}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}

	server := &Server{
		db:                       db,
		logger:                   logger,
		pubsub:                   pubsub,
		clock:                    quartz.NewReal(),
		configCache:              newChatConfigCache(context.Background(), db, clock),
		aibridgeTransportFactory: aibridgeTestFactoryPointer(factory),
	}

	db.EXPECT().GetChatModelConfigByID(gomock.Any(), modelConfigID).Return(modelConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
		ID:      providerID,
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}, nil).AnyTimes()
	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{
		ID:      providerID,
		Name:    "primary-openai",
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}}, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{ProviderID: providerID, APIKey: "test-key"}}, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), gomock.Any()).Return([]database.AIProviderKey{{ProviderID: providerID, APIKey: "test-key"}}, nil).AnyTimes()
	db.EXPECT().GetChatUsageLimitConfig(gomock.Any()).Return(database.ChatUsageLimitConfig{}, sql.ErrNoRows)
	db.EXPECT().GetChatMessagesByChatIDAscPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDAscPaginatedParams{
			ChatID:   chatID,
			AfterID:  0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return([]database.ChatMessage{
		withChatMessageAPIKeyID(mustChatMessage(
			t,
			database.ChatMessageRoleUser,
			database.ChatMessageVisibilityBoth,
			codersdk.ChatMessageText(userPrompt),
		), activeAPIKeyID),
	}, nil)
	db.EXPECT().GetChatMessagesByChatIDDescPaginated(
		gomock.Any(),
		database.GetChatMessagesByChatIDDescPaginatedParams{
			ChatID:   chatID,
			BeforeID: 0,
			LimitVal: manualTitleMessageWindowLimit,
		},
	).Return(nil, nil)
	db.EXPECT().GetChatTitleGenerationModelOverride(gomock.Any()).Return("", nil)
	db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return(nil, nil)

	db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(usageTx)
		},
	)

	usageTx.EXPECT().GetChatByIDForUpdate(gomock.Any(), chatID).Return(landedChat, nil)

	gotChat, err := server.RegenerateChatTitle(ctx, chat)
	require.NoError(t, err)
	require.Equal(t, landedChat.Title, gotChat.Title,
		"the concurrently landed title must survive; the generated title must not be persisted")

	// The in-memory pubsub delivers synchronously, so any event published
	// during RegenerateChatTitle is already buffered by now.
	select {
	case event := <-titleEvents:
		t.Fatalf("unexpected %s event published for skipped regeneration (title %q)",
			event.Kind, event.Chat.Title)
	default:
	}
}

func TestResolveUserProviderAPIKeys_StripsDisabledFallbackKeys(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()

	server := &Server{
		db: db,
		configCache: newChatConfigCache(
			context.Background(),
			db,
			quartz.NewReal(),
		),
		providerAPIKeys: chatprovider.ProviderAPIKeys{
			OpenAI:    "openai-deployment-key",
			Anthropic: "anthropic-deployment-key",
			ByProvider: map[string]string{
				"openai":    "openai-deployment-key",
				"anthropic": "anthropic-deployment-key",
			},
			BaseURLByProvider: map[string]string{
				"openai":    "https://openai.example.com",
				"anthropic": "https://anthropic.example.com",
			},
		},
	}

	providerID := uuid.New()
	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{
		ID:      providerID,
		Type:    database.AIProviderTypeAnthropic,
		Enabled: true,
	}}, nil)
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), []uuid.UUID{providerID}).Return(nil, nil)

	keys, err := server.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
	require.NoError(t, err)
	require.Empty(t, keys.OpenAI)
	require.Empty(t, keys.APIKey("openai"))
	require.Empty(t, keys.BaseURL("openai"))
	require.Equal(t, "anthropic-deployment-key", keys.Anthropic)
	require.Equal(t, "anthropic-deployment-key", keys.APIKey("anthropic"))
	require.Equal(t, "https://anthropic.example.com", keys.BaseURL("anthropic"))
	require.Equal(t, map[string]string{"anthropic": "anthropic-deployment-key"}, keys.ByProvider)
	require.Equal(t, map[string]string{"anthropic": "https://anthropic.example.com"}, keys.BaseURLByProvider)
}

func TestResolveUserProviderAPIKeys_SelectedAIProviderDoesNotUseDeploymentFallback(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()
	providerID := uuid.New()

	server := &Server{
		db: db,
		providerAPIKeys: chatprovider.ProviderAPIKeys{
			OpenAI: "openai-deployment-key",
			ByProvider: map[string]string{
				"openai": "openai-deployment-key",
			},
		},
	}

	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
		ID:      providerID,
		Type:    database.AIProviderTypeOpenai,
		Name:    "agents-openai",
		Enabled: true,
	}, nil)
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return(nil, nil)

	keys, err := server.resolveUserProviderAPIKeys(ctx, ownerID, providerID)
	require.NoError(t, err)
	require.Empty(t, keys.OpenAI)
	require.Empty(t, keys.APIKey("openai"))
	require.False(t, keys.HasProvider("openai"))
}

func TestResolveUserProviderAPIKeys_SkipsUserKeyLookupWhenNoProviderAllowsUserKeys(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	ownerID := uuid.New()

	server := &Server{
		db: db,
		configCache: newChatConfigCache(
			context.Background(),
			db,
			quartz.NewReal(),
		),
		providerAPIKeys: chatprovider.ProviderAPIKeys{
			OpenAI: "openai-deployment-key",
			ByProvider: map[string]string{
				"openai": "openai-deployment-key",
			},
		},
	}

	providerID := uuid.New()
	db.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return([]database.AIProvider{{
		ID:      providerID,
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}}, nil)
	db.EXPECT().GetAIProviderKeysByProviderIDs(gomock.Any(), []uuid.UUID{providerID}).Return(nil, nil)

	keys, err := server.resolveUserProviderAPIKeys(ctx, ownerID, uuid.Nil)
	require.NoError(t, err)
	require.Equal(t, "openai-deployment-key", keys.OpenAI)
	require.Equal(t, "openai-deployment-key", keys.APIKey("openai"))
}

func TestRefreshChatWorkspaceSnapshot_NoReloadWhenWorkspacePresent(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			calls++
			return database.Chat{}, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, chat, refreshed)
	require.Equal(t, 0, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReloadsWhenWorkspaceMissing(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	workspaceID := uuid.New()
	chat := database.Chat{ID: chatID}
	reloaded := database.Chat{
		ID: chatID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(_ context.Context, id uuid.UUID) (database.Chat, error) {
			calls++
			require.Equal(t, chatID, id)
			return reloaded, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, reloaded, refreshed)
	require.Equal(t, 1, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReturnsReloadError(t *testing.T) {
	t.Parallel()

	chat := database.Chat{ID: uuid.New()}
	loadErr := xerrors.New("boom")

	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			return database.Chat{}, loadErr
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "reload chat workspace state")
	require.ErrorContains(t, err, loadErr.Error())
	require.Equal(t, chat, refreshed)
}

func TestTurnWorkspaceContext_BindingFirstPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{ID: agentID}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).Return(workspaceAgent, nil).Times(1)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, chat, chatSnapshot)
	require.Equal(t, workspaceAgent, agent)

	gotAgent, err := workspaceCtx.getWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, workspaceAgent, gotAgent)
	require.Equal(t, chat, currentChat)
}

func TestTurnWorkspaceContext_NullBindingLazyBind(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	buildID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}
	workspaceAgent := database.WorkspaceAgent{ID: agentID}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: agentID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return([]database.WorkspaceAgent{workspaceAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: agentID, Valid: true},
			ID:      chat.ID,
		}).Return(updatedChat, nil),
	)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, updatedChat, chatSnapshot)
	require.Equal(t, workspaceAgent, agent)
	require.Equal(t, updatedChat, currentChat)

	gotAgent, err := workspaceCtx.getWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, workspaceAgent, gotAgent)
}

// expectBestEffortContextRepin lets persistBuildAgentBinding's best-effort
// context re-pin run against a mock store. The re-pin fires whenever a turn
// rebinds a chat to a different agent; these agent-switch tests set up no
// context snapshot, so it takes the no-snapshot clear path. The re-pin
// behavior itself is covered by TestPersistBuildAgentBindingRepinsContext.
func expectBestEffortContextRepin(db *dbmock.MockStore) {
	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(f func(database.Store) error, _ *database.TxOptions) error { return f(db) }).AnyTimes()
	db.EXPECT().GetLatestWorkspaceAgentContextSnapshot(gomock.Any(), gomock.Any()).
		Return(database.WorkspaceAgentContextSnapshot{}, sql.ErrNoRows).AnyTimes()
	db.EXPECT().SetChatContextSnapshot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	db.EXPECT().DeleteChatContextResourcesByChatID(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
}

func TestTurnWorkspaceContext_StaleBindingRepair(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	expectBestEffortContextRepin(db)

	workspaceID := uuid.New()
	staleAgentID := uuid.New()
	buildID := uuid.New()
	currentAgentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  staleAgentID,
			Valid: true,
		},
	}
	currentAgent := database.WorkspaceAgent{ID: currentAgentID}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: currentAgentID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), staleAgentID).Return(database.WorkspaceAgent{}, xerrors.New("missing agent")),
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return([]database.WorkspaceAgent{currentAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: currentAgentID, Valid: true},
			ID:      chat.ID,
		}).Return(updatedChat, nil),
	)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, updatedChat, chatSnapshot)
	require.Equal(t, currentAgent, agent)
	require.Equal(t, updatedChat, currentChat)
}

func TestTurnWorkspaceContextGetWorkspaceConnLazyValidationSwitchesWorkspaceAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	expectBestEffortContextRepin(db)

	workspaceID := uuid.New()
	staleAgentID := uuid.New()
	currentAgentID := uuid.New()
	buildID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  staleAgentID,
			Valid: true,
		},
	}
	staleAgent := database.WorkspaceAgent{ID: staleAgentID}
	currentAgent := database.WorkspaceAgent{ID: currentAgentID}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: currentAgentID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), staleAgentID).Return(staleAgent, nil),
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).Return([]database.WorkspaceAgent{currentAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), currentAgentID).Return(currentAgent, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: currentAgentID, Valid: true},
			ID:      chat.ID,
		}).Return(updatedChat, nil),
	)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)

	var dialed []uuid.UUID
	server := &Server{
		db:                             db,
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    30 * time.Second,
	}
	server.agentConnFn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dialed = append(dialed, agentID)
		if agentID == staleAgentID {
			return nil, nil, xerrors.New("dial failed")
		}
		return conn, func() {}, nil
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	t.Cleanup(workspaceCtx.close)

	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, conn, gotConn)
	require.Equal(t, []uuid.UUID{staleAgentID, currentAgentID}, dialed)
	require.Equal(t, updatedChat, currentChat)

	gotAgent, err := workspaceCtx.getWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, currentAgent, gotAgent)
}

func TestTurnWorkspaceContextGetWorkspaceConnFastFailsWithoutCurrentAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	staleAgentID := uuid.New()
	resourceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  staleAgentID,
			Valid: true,
		},
	}

	staleAgent := database.WorkspaceAgent{ID: staleAgentID, ResourceID: resourceID}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), staleAgentID).
		Return(staleAgent, nil).
		Times(1)
	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{}, nil).
		Times(1)
	db.EXPECT().GetWorkspaceResourceByID(gomock.Any(), resourceID).
		Return(database.WorkspaceResource{
			ID:   resourceID,
			Type: chattool.ExternalAgentResourceType,
		}, nil).
		AnyTimes()

	server := &Server{
		db:                             db,
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    30 * time.Second,
	}
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return nil, nil, xerrors.New("dial failed")
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.Nil(t, gotConn)
	require.ErrorIs(t, err, errChatHasNoWorkspaceAgent)
	require.NotErrorIs(t, err, errChatExternalAgentUnavailable)

	workspaceCtx.mu.Lock()
	defer workspaceCtx.mu.Unlock()
	require.Equal(t, database.WorkspaceAgent{}, workspaceCtx.agent)
	require.False(t, workspaceCtx.agentLoaded)
	require.Nil(t, workspaceCtx.conn)
	require.Nil(t, workspaceCtx.releaseConn)
	require.Equal(t, uuid.NullUUID{}, workspaceCtx.cachedWorkspaceID)
}

func TestTurnWorkspaceContext_SelectWorkspaceClearsCachedState(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	currentChat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: true,
		},
	}
	updatedChat := database.Chat{
		ID: currentChat.ID,
		WorkspaceID: uuid.NullUUID{
			UUID:  uuid.New(),
			Valid: true,
		},
	}
	cachedConn := agentconnmock.NewMockAgentConn(ctrl)
	releaseCalls := 0

	workspaceCtx := turnWorkspaceContext{
		chatStateMu: &sync.Mutex{},
		currentChat: &currentChat,
	}
	workspaceCtx.agent = database.WorkspaceAgent{ID: uuid.New()}
	workspaceCtx.agentLoaded = true
	workspaceCtx.conn = cachedConn
	workspaceCtx.cachedWorkspaceID = currentChat.WorkspaceID
	workspaceCtx.releaseConn = func() {
		releaseCalls++
	}

	workspaceCtx.selectWorkspace(updatedChat)

	require.Equal(t, updatedChat, currentChat)
	require.Equal(t, 1, releaseCalls)

	workspaceCtx.mu.Lock()
	defer workspaceCtx.mu.Unlock()
	require.Equal(t, database.WorkspaceAgent{}, workspaceCtx.agent)
	require.False(t, workspaceCtx.agentLoaded)
	require.Nil(t, workspaceCtx.conn)
	require.Nil(t, workspaceCtx.releaseConn)
	require.Equal(t, uuid.NullUUID{}, workspaceCtx.cachedWorkspaceID)
}

func TestTurnWorkspaceContext_EnsureWorkspaceAgentIgnoresCachedAgentForDifferentWorkspace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceOneID := uuid.New()
	workspaceTwoID := uuid.New()
	buildID := uuid.New()
	cachedAgent := database.WorkspaceAgent{ID: uuid.New()}
	resolvedAgent := database.WorkspaceAgent{ID: uuid.New()}
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceTwoID,
			Valid: true,
		},
	}
	updatedChat := chat
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	updatedChat.AgentID = uuid.NullUUID{UUID: resolvedAgent.ID, Valid: true}

	gomock.InOrder(
		db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceTwoID).Return([]database.WorkspaceAgent{resolvedAgent}, nil),
		db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceTwoID).Return(database.WorkspaceBuild{ID: buildID}, nil),
		db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
			ID:      chat.ID,
			BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
			AgentID: uuid.NullUUID{UUID: resolvedAgent.ID, Valid: true},
		}).Return(updatedChat, nil),
	)

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           &Server{db: db},
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	workspaceCtx.agent = cachedAgent
	workspaceCtx.agentLoaded = true
	workspaceCtx.cachedWorkspaceID = uuid.NullUUID{UUID: workspaceOneID, Valid: true}
	defer workspaceCtx.close()

	chatSnapshot, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
	require.NoError(t, err)
	require.Equal(t, updatedChat, chatSnapshot)
	require.Equal(t, resolvedAgent, agent)
	require.Equal(t, updatedChat, currentChat)
}

func TestSubscribeRejectsUnauthorizedCallerBeforeSharedFetches(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := newSubscribeTestServer(t, db)

	chatID := uuid.New()
	db.EXPECT().GetChatByID(gomock.Any(), chatID).
		Return(database.Chat{}, dbauthz.NotAuthorizedError{Err: xerrors.New("not authorized")})

	snapshot, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.False(t, ok)
	require.Nil(t, snapshot)
	require.Nil(t, events)
	require.Nil(t, cancel)
}

func TestSubscribeSurfacesTransientLookupFailureAsInitialError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := newSubscribeTestServer(t, db)

	chatID := uuid.New()
	db.EXPECT().GetChatByID(gomock.Any(), chatID).
		Return(database.Chat{}, xerrors.New("transient lookup failure"))

	snapshot, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	require.NotNil(t, cancel)
	require.Len(t, snapshot, 1)
	require.Equal(t, codersdk.ChatStreamEventTypeError, snapshot[0].Type)
	require.Equal(t, chatID, snapshot[0].ChatID)
	require.Equal(t, "failed to load initial snapshot", snapshot[0].Error.Message)

	_, open := <-events
	require.False(t, open)
}

func newSubscribeTestServer(t *testing.T, db database.Store) *Server {
	t.Helper()

	poller := newStreamSyncPoller(context.Background(), db, nil, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
	t.Cleanup(poller.Close)
	return &Server{
		db:               db,
		logger:           slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		pubsub:           dbpubsub.NewInMemory(),
		clock:            quartz.NewReal(),
		streamSyncPoller: poller,
	}
}

func TestResolveUserCompactionThreshold(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	modelConfigID := uuid.New()
	expectedKey := codersdk.CompactionThresholdKey(modelConfigID)

	tests := []struct {
		name        string
		dbReturn    string
		dbErr       error
		wantVal     int32
		wantOK      bool
		wantWarnLog bool
	}{
		{
			name:   "NoRowsReturnsDefault",
			dbErr:  sql.ErrNoRows,
			wantOK: false,
		},
		{
			name:     "ValidOverride",
			dbReturn: "75",
			wantVal:  75,
			wantOK:   true,
		},
		{
			name:     "OutOfRangeValue",
			dbReturn: "101",
			wantOK:   false,
		},
		{
			name:     "NonIntegerValue",
			dbReturn: "abc",
			wantOK:   false,
		},
		{
			name:        "UnexpectedDBError",
			dbErr:       xerrors.New("connection refused"),
			wantOK:      false,
			wantWarnLog: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockDB := dbmock.NewMockStore(ctrl)
			sink := testutil.NewFakeSink(t)

			srv := &Server{
				db:     mockDB,
				logger: sink.Logger(),
			}

			mockDB.EXPECT().GetUserChatCompactionThreshold(gomock.Any(), database.GetUserChatCompactionThresholdParams{
				UserID: userID,
				Key:    expectedKey,
			}).Return(tc.dbReturn, tc.dbErr)

			val, ok := srv.resolveUserCompactionThreshold(context.Background(), userID, modelConfigID)
			require.Equal(t, tc.wantVal, val)
			require.Equal(t, tc.wantOK, ok)

			warns := sink.Entries(func(e slog.SinkEntry) bool {
				return e.Level == slog.LevelWarn
			})
			if tc.wantWarnLog {
				require.NotEmpty(t, warns, "expected a warning log entry")
				return
			}
			require.Empty(t, warns, "unexpected warning log entry")
		})
	}
}

// requireFieldValue asserts that a SinkEntry contains a field with
// the given name and value.
func requireFieldValue(t *testing.T, entry slog.SinkEntry, name string, expected interface{}) {
	t.Helper()
	for _, f := range entry.Fields {
		if f.Name == name {
			require.Equal(t, expected, f.Value, "field %q value mismatch", name)
			return
		}
	}
	t.Fatalf("field %q not found in log entry", name)
}

func TestPersonalSkillsInSystemPrompt(t *testing.T) {
	t.Parallel()

	prompt := buildSystemPrompt(
		nil,
		"",
		"",
		mergeTurnSkills(
			[]skillspkg.Skill{{
				Name:        "personal-review",
				Description: "Personal review process",
				Source:      skillspkg.SourcePersonal,
			}},
			nil,
		),
		"",
		systemPromptBehaviorContext{},
	)

	text := systemPromptText(t, prompt)
	require.Contains(t, text, "<available-skills>")
	require.Contains(t, text, "- personal-review: Personal review process")
	require.NotContains(t, text, `"skill"`)
}

func TestPersonalAndWorkspaceSkillCollisionInSystemPrompt(t *testing.T) {
	t.Parallel()

	resolved := mergeTurnSkills(
		[]skillspkg.Skill{{
			Name:        "deploy",
			Description: "Personal deployment process",
			Source:      skillspkg.SourcePersonal,
		}},
		[]chattool.SkillMeta{{
			Name:        "deploy",
			Description: "Workspace deployment process",
			Dir:         "/skills/deploy",
		}},
	)
	prompt := buildSystemPrompt(
		nil,
		"",
		"",
		resolved,
		"",
		systemPromptBehaviorContext{},
	)

	text := systemPromptText(t, prompt)
	require.Contains(t, text, "<available-skills>")
	require.Contains(t, text, "- personal/deploy: Personal deployment process")
	require.Contains(t, text, "- workspace/deploy: Workspace deployment process")
	require.NotContains(t, text, "\n- deploy: ")
	require.NotContains(t, text, "\n- deploy\n")

	personal, err := skillspkg.Lookup(resolved, "personal/deploy")
	require.NoError(t, err)
	require.Equal(t, "deploy", personal.Name)
	require.Equal(t, skillspkg.SourcePersonal, personal.Source)

	workspace, err := skillspkg.Lookup(resolved, "workspace/deploy")
	require.NoError(t, err)
	require.Equal(t, "deploy", workspace.Name)
	require.Equal(t, skillspkg.SourceWorkspace, workspace.Source)

	_, err = skillspkg.Lookup(resolved, "deploy")
	require.ErrorIs(t, err, skillspkg.ErrSkillAmbiguous)
	require.ErrorContains(t, err, "personal/deploy")
	require.ErrorContains(t, err, "workspace/deploy")
}

func TestSkillIndexRefreshReplacesStaleAliases(t *testing.T) {
	t.Parallel()

	initialResolved := mergeTurnSkills(
		[]skillspkg.Skill{{
			Name:        "deploy",
			Description: "Personal deployment process",
			Source:      skillspkg.SourcePersonal,
		}},
		nil,
	)
	prompt := buildSystemPrompt(
		[]fantasy.Message{{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "Create a workspace."},
			},
		}},
		"",
		"",
		initialResolved,
		"",
		systemPromptBehaviorContext{},
	)

	mergedIndex := chattool.FormatResolvedSkillIndex(mergeTurnSkills(
		[]skillspkg.Skill{{
			Name:        "deploy",
			Description: "Personal deployment process",
			Source:      skillspkg.SourcePersonal,
		}},
		[]chattool.SkillMeta{{
			Name:        "deploy",
			Description: "Workspace deployment process",
			Dir:         "/skills/deploy",
		}},
	))
	prompt = removeSkillIndexMessages(prompt)
	prompt = chatprompt.InsertSystem(prompt, mergedIndex)

	text := systemPromptText(t, prompt)
	require.Equal(t, 1, strings.Count(text, "<available-skills>"))
	require.NotContains(t, text, "\n- deploy: Personal deployment process")
	require.Contains(t, text, "- personal/deploy: Personal deployment process")
	require.Contains(t, text, "- workspace/deploy: Workspace deployment process")
}

func requireUserSkillContextActor(ctx context.Context, t *testing.T, userID uuid.UUID) {
	t.Helper()
	actor, ok := dbauthz.ActorFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, rbac.SubjectTypeUser, actor.Type)
	require.Equal(t, userID.String(), actor.ID)
	require.Equal(t, rbac.RoleIdentifiers{rbac.RoleMember()}, actor.Roles)
}

func TestFetchPersonalSkillMetadata(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		server := &Server{db: db}
		userID := uuid.New()

		db.EXPECT().ListUserSkillMetadataByUserID(gomock.Any(), userID).DoAndReturn(
			func(ctx context.Context, gotUserID uuid.UUID) ([]database.ListUserSkillMetadataByUserIDRow, error) {
				requireUserSkillContextActor(ctx, t, userID)
				require.Equal(t, userID, gotUserID)
				return []database.ListUserSkillMetadataByUserIDRow{{
					UserID:      userID,
					Name:        "personal-review",
					Description: "Personal review process",
				}}, nil
			},
		)

		got := server.fetchPersonalSkillMetadata(context.Background(), userID, logger)
		require.Equal(t, []skillspkg.Skill{{
			Name:        "personal-review",
			Description: "Personal review process",
			Source:      skillspkg.SourcePersonal,
		}}, got)
	})

	t.Run("ListFailure", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		sink := testutil.NewFakeSink(t)
		logger := sink.Logger().Leveled(slog.LevelDebug)
		server := &Server{db: db}
		userID := uuid.New()

		db.EXPECT().ListUserSkillMetadataByUserID(gomock.Any(), userID).Return(nil, xerrors.New("boom"))

		got := server.fetchPersonalSkillMetadata(context.Background(), userID, logger)
		require.Empty(t, got)
		warns := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelWarn && strings.Contains(e.Message, "personal skill metadata")
		})
		require.NotEmpty(t, warns)
	})
}

func TestLoadPersonalSkillBody(t *testing.T) {
	t.Parallel()

	t.Run("ParsesCurrentContent", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db}
		userID := uuid.New()
		params := database.GetUserSkillByUserIDAndNameParams{
			UserID: userID,
			Name:   "personal-review",
		}

		db.EXPECT().GetUserSkillByUserIDAndName(gomock.Any(), params).DoAndReturn(
			func(ctx context.Context, gotParams database.GetUserSkillByUserIDAndNameParams) (database.UserSkill, error) {
				requireUserSkillContextActor(ctx, t, userID)
				require.Equal(t, params, gotParams)
				return database.UserSkill{
					UserID:  userID,
					Name:    "personal-review",
					Content: "---\nname: personal-review\ndescription: Personal review process\n---\n\nUpdated instructions.\n",
				}, nil
			},
		)

		got, err := server.loadPersonalSkillBody(context.Background(), userID, "personal-review")
		require.NoError(t, err)
		require.Equal(t, "personal-review", got.Name)
		require.Equal(t, "Personal review process", got.Description)
		require.Equal(t, skillspkg.SourcePersonal, got.Source)
		require.Contains(t, got.Body, "Updated instructions.")
	})

	t.Run("DeletedSkill", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		server := &Server{db: db}
		userID := uuid.New()
		params := database.GetUserSkillByUserIDAndNameParams{
			UserID: userID,
			Name:   "missing-skill",
		}

		db.EXPECT().GetUserSkillByUserIDAndName(gomock.Any(), params).DoAndReturn(
			func(ctx context.Context, gotParams database.GetUserSkillByUserIDAndNameParams) (database.UserSkill, error) {
				requireUserSkillContextActor(ctx, t, userID)
				require.Equal(t, params, gotParams)
				return database.UserSkill{}, sql.ErrNoRows
			},
		)

		_, err := server.loadPersonalSkillBody(context.Background(), userID, "missing-skill")
		require.ErrorIs(t, err, skillspkg.ErrSkillNotFound)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		sink := testutil.NewFakeSink(t)
		server := &Server{db: db, logger: sink.Logger()}
		userID := uuid.New()
		params := database.GetUserSkillByUserIDAndNameParams{
			UserID: userID,
			Name:   "error-skill",
		}
		dbErr := xerrors.New("database unavailable")

		db.EXPECT().GetUserSkillByUserIDAndName(gomock.Any(), params).DoAndReturn(
			func(ctx context.Context, gotParams database.GetUserSkillByUserIDAndNameParams) (database.UserSkill, error) {
				requireUserSkillContextActor(ctx, t, userID)
				require.Equal(t, params, gotParams)
				return database.UserSkill{}, dbErr
			},
		)

		_, err := server.loadPersonalSkillBody(context.Background(), userID, "error-skill")

		require.ErrorContains(t, err, "load personal skill body")
		require.ErrorIs(t, err, dbErr)
		entries := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelError && e.Message == "load personal skill body failed"
		})
		require.Len(t, entries, 1)
		requireFieldValue(t, entries[0], "error", dbErr)
	})

	t.Run("ParseError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		sink := testutil.NewFakeSink(t)
		server := &Server{db: db, logger: sink.Logger()}
		userID := uuid.New()
		params := database.GetUserSkillByUserIDAndNameParams{
			UserID: userID,
			Name:   "broken-skill",
		}

		db.EXPECT().GetUserSkillByUserIDAndName(gomock.Any(), params).DoAndReturn(
			func(ctx context.Context, gotParams database.GetUserSkillByUserIDAndNameParams) (database.UserSkill, error) {
				requireUserSkillContextActor(ctx, t, userID)
				require.Equal(t, params, gotParams)
				return database.UserSkill{
					UserID:  userID,
					Name:    "broken-skill",
					Content: "---\nname: broken-skill\ndescription: Broken\n---\n\n   \n",
				}, nil
			},
		)

		_, err := server.loadPersonalSkillBody(context.Background(), userID, "broken-skill")

		require.ErrorContains(t, err, "parse personal skill body")
		require.ErrorIs(t, err, skillspkg.ErrSkillBodyRequired)
		entries := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelError && e.Message == "parse personal skill body failed"
		})
		require.Len(t, entries, 1)
		requireFieldValue(t, entries[0], "user_id", userID)
		requireFieldValue(t, entries[0], "name", "broken-skill")
	})
}

func systemPromptText(t *testing.T, prompt []fantasy.Message) string {
	t.Helper()

	var b strings.Builder
	for _, msg := range prompt {
		if msg.Role != fantasy.MessageRoleSystem {
			continue
		}
		for _, part := range msg.Content {
			textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
			if ok {
				_, _ = b.WriteString(textPart.Text)
				_, _ = b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func TestGetWorkspaceConn_StaleAgentRecovery(t *testing.T) {
	// Regression test: when a workspace is rebuilt, the chat's stored
	// agent ID points to a disconnected agent from the old build. The
	// cache-miss path must let dialWithLazyValidation discover the new
	// agent instead of rejecting the old one immediately.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	expectBestEffortContextRepin(db)

	workspaceID := uuid.New()
	oldAgentID := uuid.New()
	newAgentID := uuid.New()
	buildID := uuid.New()

	// Old agent: disconnected (from previous build).
	oldAgent := database.WorkspaceAgent{
		ID: oldAgentID,
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
		DisconnectedAt: sql.NullTime{
			Time:  time.Now().Add(-9 * time.Minute),
			Valid: true,
		},
	}

	// New agent: connected (from latest build).
	newAgent := database.WorkspaceAgent{
		ID:   newAgentID,
		Name: "main",
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-1 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  oldAgentID,
			Valid: true,
		},
	}

	// ensureWorkspaceAgent fetches the stale agent.
	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), oldAgentID).
		Return(oldAgent, nil).Times(1)
	// Lazy validation discovers the new agent.
	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{newAgent}, nil).Times(1)
	// Post-switch: persist the new binding.
	db.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return(database.WorkspaceBuild{ID: buildID}, nil).Times(1)
	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), newAgentID).
		Return(newAgent, nil).Times(1)

	updatedChat := chat
	updatedChat.AgentID = uuid.NullUUID{UUID: newAgentID, Valid: true}
	updatedChat.BuildID = uuid.NullUUID{UUID: buildID, Valid: true}
	db.EXPECT().UpdateChatBuildAgentBinding(gomock.Any(), database.UpdateChatBuildAgentBindingParams{
		ID:      chat.ID,
		BuildID: uuid.NullUUID{UUID: buildID, Valid: true},
		AgentID: uuid.NullUUID{UUID: newAgentID, Valid: true},
	}).Return(updatedChat, nil).Times(1)

	newConn := agentconnmock.NewMockAgentConn(ctrl)
	newConn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)

	server := &Server{
		db:                             db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    defaultDialTimeout,
	}
	server.agentConnFn = func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		switch id {
		case oldAgentID:
			return nil, nil, xerrors.New("agent is not connected")
		case newAgentID:
			return newConn, func() {}, nil
		default:
			return nil, nil, xerrors.Errorf("unexpected agent ID: %s", id)
		}
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:      server,
		chatStateMu: chatStateMu,
		currentChat: &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) {
			return database.Chat{}, nil
		},
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitMedium)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err, "getWorkspaceConn should recover stale agent binding")
	require.Same(t, newConn, gotConn, "should return the connection to the new agent")

	// Verify the cache was updated to the new agent so subsequent
	// cache-hit calls use the correct agent ID.
	workspaceCtx.mu.Lock()
	defer workspaceCtx.mu.Unlock()
	require.Equal(t, newAgentID, workspaceCtx.agent.ID, "cached agent should be the new agent")
	require.True(t, workspaceCtx.agentLoaded)
	require.Same(t, newConn, workspaceCtx.conn, "connection should be cached for subsequent calls")
}

func TestGetWorkspaceConn_SameBuildAgentCrash(t *testing.T) {
	// When an agent crashes on the same build (disconnected, but still
	// in the latest build), dialWithLazyValidation dials, fails fast,
	// validation finds the same agent, and the retry also fails. The
	// wrapped dial error propagates (not errChatAgentDisconnected).
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()

	// Agent: disconnected (crashed on current build).
	agent := database.WorkspaceAgent{
		ID:   agentID,
		Name: "main",
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
		DisconnectedAt: sql.NullTime{
			Time:  time.Now().Add(-9 * time.Minute),
			Valid: true,
		},
	}

	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}

	// ensureWorkspaceAgent fetches the (crashed) agent.
	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(agent, nil).Times(1)
	// Validation finds the same agent in the latest build.
	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{agent}, nil).Times(1)

	dialErr := xerrors.New("agent is not connected")
	server := &Server{
		db:                             db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    defaultDialTimeout,
	}
	server.agentConnFn = func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		return nil, nil, dialErr
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:      server,
		chatStateMu: chatStateMu,
		currentChat: &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) {
			return database.Chat{}, nil
		},
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitMedium)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.Nil(t, gotConn)
	require.Error(t, err)
	// The error should be a wrapped dial error, not the
	// agent-disconnected sentinel.
	require.NotErrorIs(t, err, errChatAgentDisconnected)
	require.ErrorIs(t, err, dialErr)

	// Cache should not have a connection, but the agent should
	// still be loaded (ensureWorkspaceAgent cached it).
	workspaceCtx.mu.Lock()
	defer workspaceCtx.mu.Unlock()
	require.True(t, workspaceCtx.agentLoaded)
	require.Nil(t, workspaceCtx.conn)
}

func TestGetWorkspaceConn_StatusCheck(t *testing.T) {
	// The cache-hit status check re-fetches the agent row for a fresh
	// heartbeat timestamp. Healthy, timed-out, and DB-error paths return
	// the cached connection. Disconnected agents are covered separately
	// because they now trigger a fresh dial before recovery.
	t.Parallel()

	type testCase struct {
		name       string
		buildAgent func(now time.Time) database.WorkspaceAgent
		dbError    bool
	}

	tests := []testCase{
		{
			name: "TimedOutAgentCacheHit",
			buildAgent: func(now time.Time) database.WorkspaceAgent {
				return database.WorkspaceAgent{
					CreatedAt:                now.Add(-10 * time.Minute),
					ConnectionTimeoutSeconds: 60,
				}
			},
		},
		{
			name: "CacheHitHealthyAgent",
			buildAgent: func(now time.Time) database.WorkspaceAgent {
				return database.WorkspaceAgent{
					FirstConnectedAt: sql.NullTime{
						Time:  now.Add(-5 * time.Minute),
						Valid: true,
					},
					LastConnectedAt: sql.NullTime{
						Time:  now,
						Valid: true,
					},
				}
			},
		},
		{
			// When GetWorkspaceAgentByID returns an error on
			// cache hit, the cached connection should be returned.
			name: "CacheHitDBError",
			buildAgent: func(now time.Time) database.WorkspaceAgent {
				return database.WorkspaceAgent{
					FirstConnectedAt: sql.NullTime{
						Time:  now.Add(-5 * time.Minute),
						Valid: true,
					},
					LastConnectedAt: sql.NullTime{
						Time:  now,
						Valid: true,
					},
				}
			},
			dbError: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)

			workspaceID := uuid.New()
			agentID := uuid.New()
			chat := database.Chat{
				ID: uuid.New(),
				WorkspaceID: uuid.NullUUID{
					UUID:  workspaceID,
					Valid: true,
				},
				AgentID: uuid.NullUUID{
					UUID:  agentID,
					Valid: true,
				},
			}

			// Stamp the agent with the generated ID. Use the
			// subtest's mock clock so the agent's timestamps are
			// anchored to the same `now` the server uses. Using
			// time.Now() at slice-literal construction time
			// produced a Windows-CI flake because a slow scheduler
			// could insert more than agentInactiveDisconnectTimeout
			// of wall-clock delay between the literal and the
			// subtest body.
			clock := quartz.NewMock(t)
			now := clock.Now()
			agent := tc.buildAgent(now)
			agent.ID = agentID

			// Set up the DB mock for GetWorkspaceAgentByID.
			if tc.dbError {
				db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
					Return(database.WorkspaceAgent{}, xerrors.New("connection reset")).
					Times(1)
			} else {
				db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
					Return(agent, nil).
					Times(1)
			}

			var releaseCalled bool

			server := &Server{
				db:                             db,
				logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				clock:                          clock,
				agentInactiveDisconnectTimeout: 30 * time.Second,
				dialTimeout:                    defaultDialTimeout,
			}
			server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				return nil, nil, xerrors.New("should not be called")
			}

			chatStateMu := &sync.Mutex{}
			currentChat := chat
			cachedConn := agentconnmock.NewMockAgentConn(ctrl)
			workspaceCtx := turnWorkspaceContext{
				server:      server,
				chatStateMu: chatStateMu,
				currentChat: &currentChat,
				loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) {
					return database.Chat{}, nil
				},
				agent:             agent,
				agentLoaded:       true,
				conn:              cachedConn,
				releaseConn:       func() { releaseCalled = true },
				cachedWorkspaceID: chat.WorkspaceID,
			}
			defer workspaceCtx.close()

			ctx := testutil.Context(t, testutil.WaitShort)
			gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
			require.NoError(t, err)
			require.Same(t, cachedConn, gotConn)
			require.False(t, releaseCalled, "release called")
		})
	}
}

func TestGetWorkspaceConn_DialTimeoutDisconnectedRecoveryThreshold(t *testing.T) {
	t.Parallel()

	buildDisconnectedAgent := func(disconnectedFor time.Duration) func(time.Time) database.WorkspaceAgent {
		return func(now time.Time) database.WorkspaceAgent {
			return database.WorkspaceAgent{
				FirstConnectedAt: sql.NullTime{
					Time:  now.Add(-10 * time.Minute),
					Valid: true,
				},
				LastConnectedAt: sql.NullTime{
					Time:  now.Add(-10 * time.Minute),
					Valid: true,
				},
				DisconnectedAt: sql.NullTime{
					Time:  now.Add(-disconnectedFor),
					Valid: true,
				},
			}
		}
	}

	buildTimedOutAgent := func(now time.Time) database.WorkspaceAgent {
		return database.WorkspaceAgent{
			CreatedAt:                now.Add(-10 * time.Minute),
			ConnectionTimeoutSeconds: 60,
		}
	}

	testCases := []struct {
		name                 string
		buildAgent           func(now time.Time) database.WorkspaceAgent
		staleExternalBinding bool
		wantErr              error
	}{
		{
			name:       "RecentDisconnectReturnsDialTimeout",
			buildAgent: buildDisconnectedAgent(agentDisconnectedRecoveryThreshold / 2),
			wantErr:    errChatDialTimeout,
		},
		{
			name:       "PastThresholdEscalates",
			buildAgent: buildDisconnectedAgent(agentDisconnectedRecoveryThreshold),
			wantErr:    errChatAgentDisconnected,
		},
		{
			name:       "NeverConnectedTimeoutEscalates",
			buildAgent: buildTimedOutAgent,
			wantErr:    errChatAgentNeverConnected,
		},
		{
			name:                 "StaleExternalBindingUsesLatestInternalAgent",
			buildAgent:           buildTimedOutAgent,
			staleExternalBinding: true,
			wantErr:              errChatAgentNeverConnected,
		},
		{
			name: "NeverConnectedWithinTimeoutStaysSoft",
			buildAgent: func(now time.Time) database.WorkspaceAgent {
				return database.WorkspaceAgent{
					CreatedAt:                now.Add(-30 * time.Second),
					ConnectionTimeoutSeconds: 120,
				}
			},
			wantErr: errChatDialTimeout,
		},
		{
			name: "NeverConnectedNoTimeoutStaysSoft",
			buildAgent: func(now time.Time) database.WorkspaceAgent {
				return database.WorkspaceAgent{
					CreatedAt: now.Add(-10 * time.Minute),
				}
			},
			wantErr: errChatDialTimeout,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)

			workspaceID := uuid.New()
			agentID := uuid.New()
			chat := database.Chat{
				ID: uuid.New(),
				WorkspaceID: uuid.NullUUID{
					UUID:  workspaceID,
					Valid: true,
				},
				AgentID: uuid.NullUUID{
					UUID:  agentID,
					Valid: true,
				},
			}

			clock := quartz.NewMock(t)
			timeoutTrap := clock.Trap().AfterFunc("chatd", dialTimeoutTimerTag)
			defer timeoutTrap.Close()
			delayTrap := clock.Trap().NewTimer("chatd", dialValidationDelayTimerTag)
			defer delayTrap.Close()
			now := clock.Now()
			boundAgent := tc.buildAgent(now)
			boundAgent.ID = agentID
			latestAgent := boundAgent
			latestAgent.ID = uuid.New()
			latestAgentLookups := 1
			if tc.staleExternalBinding {
				boundAgent.ResourceID = uuid.New()
				latestAgent.ResourceID = uuid.Nil
				latestAgentLookups++
				db.EXPECT().GetWorkspaceResourceByID(gomock.Any(), boundAgent.ResourceID).
					Return(database.WorkspaceResource{Type: chattool.ExternalAgentResourceType}, nil).
					AnyTimes()
			}
			db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), boundAgent.ID).
				Return(boundAgent, nil).
				Times(1)
			db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), latestAgent.ID).
				Return(latestAgent, nil).
				Times(1)
			db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
				Return([]database.WorkspaceAgent{latestAgent}, nil).
				Times(latestAgentLookups)

			server := &Server{
				db:                             db,
				logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				clock:                          clock,
				agentInactiveDisconnectTimeout: 30 * time.Second,
				dialTimeout:                    10 * time.Millisecond,
			}
			dialEntered := make(chan struct{})
			var closeDialEntered sync.Once
			server.agentConnFn = func(ctx context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				closeDialEntered.Do(func() { close(dialEntered) })
				<-ctx.Done()
				return nil, nil, ctx.Err()
			}

			chatStateMu := &sync.Mutex{}
			currentChat := chat
			workspaceCtx := turnWorkspaceContext{
				server:           server,
				chatStateMu:      chatStateMu,
				currentChat:      &currentChat,
				loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
			}
			defer workspaceCtx.close()

			ctx := testutil.Context(t, testutil.WaitShort)
			type workspaceConnResult struct {
				conn workspacesdk.AgentConn
				err  error
			}
			resultCh := make(chan workspaceConnResult, 1)
			go func() {
				gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
				resultCh <- workspaceConnResult{conn: gotConn, err: err}
			}()

			timeoutCall := timeoutTrap.MustWait(ctx)
			require.Equal(t, server.dialTimeout, timeoutCall.Duration)
			timeoutCall.MustRelease(ctx)
			delayCall := delayTrap.MustWait(ctx)
			require.Equal(t, workspaceDialValidationDelay, delayCall.Duration)
			delayCall.MustRelease(ctx)
			select {
			case <-dialEntered:
			case <-ctx.Done():
				t.Fatal("timed out waiting for dial to start")
			}
			clock.Advance(server.dialTimeout).MustWait(ctx)

			var result workspaceConnResult
			select {
			case result = <-resultCh:
			case <-ctx.Done():
				t.Fatal("timed out waiting for getWorkspaceConn")
			}
			require.Nil(t, result.conn)
			require.ErrorIs(t, result.err, tc.wantErr)
			if !xerrors.Is(tc.wantErr, errChatAgentDisconnected) {
				require.NotErrorIs(t, result.err, errChatAgentDisconnected)
			}
			if !xerrors.Is(tc.wantErr, errChatAgentNeverConnected) {
				require.NotErrorIs(t, result.err, errChatAgentNeverConnected)
			}

			workspaceCtx.mu.Lock()
			defer workspaceCtx.mu.Unlock()
			require.False(t, workspaceCtx.agentLoaded)
			require.Nil(t, workspaceCtx.conn)
		})
	}
}

func TestGetWorkspaceConn_DisconnectedStatusDialSuccessDoesNotEscalate(t *testing.T) {
	// A stale disconnected row must not prompt stop/start if the
	// agent can still be dialed successfully.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}

	disconnectedAgent := database.WorkspaceAgent{
		ID: agentID,
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
	}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(disconnectedAgent, nil).
		Times(1)

	server := &Server{
		db:                             db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    10 * time.Millisecond,
	}
	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)
	var dialCalled bool
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dialCalled = true
		return conn, nil, nil
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitShort)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, conn, gotConn)
	require.True(t, dialCalled, "dial called")
}

func TestGetWorkspaceConn_CacheHitDisconnectedRetriesDialBeforeEscalating(t *testing.T) {
	// A disconnected cached connection is discarded first. Recovery is
	// only surfaced if the replacement dial also times out.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}
	disconnectedAgent := database.WorkspaceAgent{
		ID: agentID,
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-10 * time.Minute),
			Valid: true,
		},
	}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(disconnectedAgent, nil).
		Times(2)

	server := &Server{
		db:                             db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    10 * time.Millisecond,
	}
	newConn := agentconnmock.NewMockAgentConn(ctrl)
	newConn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)
	var dialCalled bool
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dialCalled = true
		return newConn, nil, nil
	}

	var releaseCalled bool
	chatStateMu := &sync.Mutex{}
	currentChat := chat
	oldConn := agentconnmock.NewMockAgentConn(ctrl)
	workspaceCtx := turnWorkspaceContext{
		server:            server,
		chatStateMu:       chatStateMu,
		currentChat:       &currentChat,
		loadChatSnapshot:  func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
		agent:             disconnectedAgent,
		agentLoaded:       true,
		conn:              oldConn,
		releaseConn:       func() { releaseCalled = true },
		cachedWorkspaceID: chat.WorkspaceID,
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitShort)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, newConn, gotConn)
	require.True(t, releaseCalled, "release called")
	require.True(t, dialCalled, "dial called")
}

func TestGetWorkspaceConn_DialTimeout(t *testing.T) {
	// When dialWithLazyValidation blocks beyond the dial
	// timeout, getWorkspaceConn should return
	// errChatDialTimeout instead of hanging indefinitely.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}

	// Agent appears connected so the status check passes.
	connectedAgent := database.WorkspaceAgent{
		ID: agentID,
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-1 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(connectedAgent, nil).
		Times(2)
	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{connectedAgent}, nil).
		Times(1)

	server := &Server{
		db:                             db,
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    10 * time.Millisecond,
	}
	// Dial blocks forever (simulates unreachable agent).
	server.agentConnFn = func(ctx context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitShort)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.Nil(t, gotConn)
	require.ErrorIs(t, err, errChatDialTimeout)
}

func TestGetWorkspaceConn_DialTimeoutParentCanceled(t *testing.T) {
	// When the parent context is canceled, the parent's error
	// must propagate unchanged (not wrapped as a dial timeout).
	// This is critical because the chatloop checks
	// context.Cause(ctx) for ErrInterrupted.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}

	connectedAgent := database.WorkspaceAgent{
		ID: agentID,
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-1 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(connectedAgent, nil).
		Times(1)

	parentErr := xerrors.New("parent canceled")
	ctx, cancel := context.WithCancelCause(testutil.Context(t, testutil.WaitShort))

	server := &Server{
		db:                             db,
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		// Use a very long dial timeout so the parent cancel fires
		// first.
		dialTimeout: 10 * time.Minute,
	}
	// Signal when the dial goroutine has started so we can
	// cancel the parent at the right time without time.Sleep.
	dialStarted := make(chan struct{})
	server.agentConnFn = func(ctx context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		close(dialStarted)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	// Cancel the parent after the dial starts.
	go func() {
		<-dialStarted
		cancel(parentErr)
	}()

	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.Nil(t, gotConn)
	// The error must NOT be errChatDialTimeout.
	require.NotErrorIs(t, err, errChatDialTimeout)
	// The parent context's error should propagate.
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestGetWorkspaceConn_PreflightExternalAgentTimedOut(t *testing.T) {
	// External agent never connected and the connection window has
	// elapsed (Timeout). Preflight must short-circuit before any
	// dial attempt and return the external-agent error.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	resourceID := uuid.New()
	agent := database.WorkspaceAgent{
		ID:                       agentID,
		Name:                     "main",
		ResourceID:               resourceID,
		CreatedAt:                time.Now().Add(-10 * time.Minute),
		ConnectionTimeoutSeconds: 60,
	}
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(agent, nil).
		Times(1)
	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{agent}, nil).
		Times(1)
	db.EXPECT().GetWorkspaceResourceByID(gomock.Any(), resourceID).
		Return(database.WorkspaceResource{
			ID:   resourceID,
			Type: chattool.ExternalAgentResourceType,
		}, nil).
		Times(1)

	server := &Server{
		db:                             db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    defaultDialTimeout,
	}
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		t.Fatal("unexpected agent dial for external agent preflight")
		return nil, nil, xerrors.New("unexpected agent dial")
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitMedium)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.Nil(t, gotConn)
	require.ErrorIs(t, err, errChatExternalAgentUnavailable)
	require.Equal(t, chattool.ExternalAgentUnavailableMessage(agent), err.Error())
}

func TestGetWorkspaceConn_PreflightExternalAgentConnectingDials(t *testing.T) {
	// External agent in the Connecting state (never connected yet,
	// still inside ConnectionTimeoutSeconds) must fall through to the
	// dial so the user can succeed in the same turn if they just
	// started the agent on their host.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	resourceID := uuid.New()
	agent := database.WorkspaceAgent{
		ID:                       agentID,
		Name:                     "main",
		ResourceID:               resourceID,
		CreatedAt:                time.Now().Add(-1 * time.Second),
		ConnectionTimeoutSeconds: 600,
	}
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(agent, nil).
		Times(1)

	conn := agentconnmock.NewMockAgentConn(ctrl)
	conn.EXPECT().SetExtraHeaders(gomock.Any()).Times(1)

	dialed := false
	server := &Server{
		db:                             db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    defaultDialTimeout,
	}
	server.agentConnFn = func(_ context.Context, id uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		dialed = true
		require.Equal(t, agentID, id)
		return conn, func() {}, nil
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitMedium)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)
	require.Same(t, conn, gotConn)
	require.True(t, dialed, "preflight must let Connecting external agents reach the dial")
}

func TestGetWorkspaceConn_DialErrorNotMisclassifiedAsTimeout(t *testing.T) {
	// Regression test: a non-timeout dial error (e.g. auth
	// failure) with the parent context still alive must NOT be
	// converted to errChatDialTimeout or masked as external-agent
	// unavailability.
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	workspaceID := uuid.New()
	agentID := uuid.New()
	resourceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
		AgentID: uuid.NullUUID{
			UUID:  agentID,
			Valid: true,
		},
	}

	connectedAgent := database.WorkspaceAgent{
		ID:         agentID,
		ResourceID: resourceID,
		FirstConnectedAt: sql.NullTime{
			Time:  time.Now().Add(-1 * time.Minute),
			Valid: true,
		},
		LastConnectedAt: sql.NullTime{
			Time:  time.Now(),
			Valid: true,
		},
	}

	db.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agentID).
		Return(connectedAgent, nil).
		Times(1)
	// When the initial dial fails immediately, dialWithLazyValidation
	// calls resolveFastFailure which validates the binding. Mock the
	// validation to return the same agent, triggering a synchronous
	// redial that also returns the error.
	db.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), workspaceID).
		Return([]database.WorkspaceAgent{connectedAgent}, nil).
		AnyTimes()
	db.EXPECT().GetWorkspaceResourceByID(gomock.Any(), resourceID).
		Return(database.WorkspaceResource{
			ID:   resourceID,
			Type: chattool.ExternalAgentResourceType,
		}, nil).
		AnyTimes()

	dialErr := xerrors.New("authentication failed")
	server := &Server{
		db:                             db,
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		// Generous timeout so the dial error fires well before
		// the timeout.
		dialTimeout: defaultDialTimeout,
	}
	server.agentConnFn = func(context.Context, uuid.UUID) (workspacesdk.AgentConn, func(), error) {
		// Return an error immediately (not a timeout).
		return nil, nil, dialErr
	}

	chatStateMu := &sync.Mutex{}
	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: func(context.Context, uuid.UUID) (database.Chat, error) { return database.Chat{}, nil },
	}
	defer workspaceCtx.close()

	ctx := testutil.Context(t, testutil.WaitShort)
	gotConn, err := workspaceCtx.getWorkspaceConn(ctx)
	require.Nil(t, gotConn)
	// Must NOT be misclassified as a dial timeout or external-agent outage.
	require.NotErrorIs(t, err, errChatDialTimeout)
	require.NotErrorIs(t, err, errChatExternalAgentUnavailable)
	// The original dial error should propagate.
	require.ErrorIs(t, err, dialErr)
	require.ErrorContains(t, err, "authentication failed")
}

// TestGetWorkspaceConnBumpsWorkspaceUsage verifies that acquiring a
// workspace agent connection bumps the workspace's last_used_at via
// the usage tracker and extends the build's autostop deadline.
func TestGetWorkspaceConnBumpsWorkspaceUsage(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})

	// Create a workspace with a full build chain so we can verify
	// both last_used_at (dormancy) and deadline (autostop) bumps.
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
		CreatedBy:       user.ID,
	})
	require.NoError(t, db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
		ID:                tmpl.ID,
		UpdatedAt:         dbtime.Now(),
		AllowUserAutostop: true,
		ActivityBump:      int64(time.Hour),
	}))
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     tmpl.ID,
		Ttl:            sql.NullInt64{Valid: true, Int64: int64(8 * time.Hour)},
	})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: org.ID,
		CompletedAt: sql.NullTime{
			Valid: true,
			Time:  dbtime.Now().Add(-30 * time.Minute),
		},
	})
	// Build deadline is 30 minutes in the past, close enough to
	// be bumped by the 1-hour activity bump.
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             pj.ID,
		Transition:        database.WorkspaceTransitionStart,
		Deadline:          dbtime.Now().Add(-30 * time.Minute),
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	dbAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: res.ID,
	})
	originalDeadline := build.Deadline

	chat := dbgen.Chat(t, db, database.Chat{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		LastModelConfigID: modelConfig.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
	})

	// Usage tracker with manual tick/flush so the test controls
	// when last_used_at is written to the DB.
	flushTick := make(chan time.Time)
	flushDone := make(chan int, 1)
	tracker := workspacestats.NewTracker(db,
		workspacestats.TrackerWithTickFlush(flushTick, flushDone),
		workspacestats.TrackerWithLogger(slogtest.Make(t, nil)),
	)
	t.Cleanup(func() { tracker.Close() })

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().AwaitReachable(gomock.Any()).Return(true).AnyTimes()

	server := &Server{
		db:                             db,
		logger:                         slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		clock:                          quartz.NewReal(),
		agentInactiveDisconnectTimeout: 30 * time.Second,
		dialTimeout:                    testutil.WaitLong,
		usageTracker:                   tracker,
		agentConnFn: func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		},
	}

	currentChat := chat
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      &sync.Mutex{},
		currentChat:      &currentChat,
		loadChatSnapshot: db.GetChatByID,
	}
	t.Cleanup(workspaceCtx.close)

	_, err := workspaceCtx.getWorkspaceConn(ctx)
	require.NoError(t, err)

	// getWorkspaceConn tracks usage synchronously; flushing the
	// tracker must write last_used_at for the linked workspace.
	testutil.RequireSend(ctx, t, flushTick, time.Now())
	count := testutil.RequireReceive(ctx, t, flushDone)
	require.Greater(t, count, 0,
		"expected the usage tracker to flush the chat workspace")

	updatedWs, err := db.GetWorkspaceByID(ctx, ws.ID)
	require.NoError(t, err)
	require.True(t, updatedWs.LastUsedAt.After(ws.LastUsedAt),
		"workspace last_used_at should have been bumped")

	// The activity bump runs synchronously inside
	// getWorkspaceConn, so the deadline is already extended.
	// ±2 minute tolerance mirrors activitybump_test.go.
	updatedBuild, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, ws.ID)
	require.NoError(t, err)
	require.True(t, updatedBuild.Deadline.After(originalDeadline),
		"workspace build deadline should have been bumped")
	now := dbtime.Now()
	require.True(t, updatedBuild.Deadline.After(now.Add(time.Hour-2*time.Minute)))
	require.True(t, updatedBuild.Deadline.Before(now.Add(time.Hour+2*time.Minute)))
}

func TestServer_inflightContext(t *testing.T) {
	t.Parallel()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	t.Cleanup(serverCancel)
	server := &Server{ctx: serverCtx}

	type ctxKey string
	const key ctxKey = "inflight-test"
	reqCtx, reqCancel := context.WithCancel(context.WithValue(context.Background(), key, "value"))
	t.Cleanup(reqCancel)

	inflightCtx, stop := server.inflightContext(reqCtx)
	t.Cleanup(stop)

	// Auth and routing values must carry over from the request.
	require.Equal(t, "value", inflightCtx.Value(key))

	// Request cancellation must not cancel in-flight work: it has to outlive
	// the originating request.
	reqCancel()
	select {
	case <-inflightCtx.Done():
		t.Fatal("inflight context canceled by request cancellation")
	case <-time.After(testutil.IntervalFast):
	}

	// Server shutdown must cancel in-flight work so Close does not block
	// on long-running callees while a provider is unreachable.
	serverCancel()
	select {
	case <-inflightCtx.Done():
	case <-time.After(testutil.WaitShort):
		t.Fatal("inflight context not canceled on server shutdown")
	}
}

// TestPrepareManualTitleDebugRun_RouteFailureDerivesProviderFromConfig drives
// the fallback branch in prepareManualTitleDebugRun: AI-gateway route
// resolution fails (the BYOK key lookup returns a non-ErrNoRows error) while
// the linked provider stays enabled, so the debug run records the provider
// type derived from modelConfig.AIProviderID instead of an empty string.
func TestPrepareManualTitleDebugRun_RouteFailureDerivesProviderFromConfig(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ownerID := uuid.New()
	providerID := uuid.New()
	chat := database.Chat{ID: uuid.New(), OwnerID: ownerID}
	modelConfig := database.ChatModelConfig{
		ID:           uuid.New(),
		Model:        "claude-sonnet-4",
		AIProviderID: uuid.NullUUID{UUID: providerID, Valid: true},
	}
	provider := database.AIProvider{
		ID:      providerID,
		Type:    database.AIProviderTypeAnthropic,
		Name:    "anthropic",
		Enabled: true,
	}

	// Resolved twice: once by gatewayProviderForConfig during route resolution,
	// once by the fallback's own enabledAIProviderByID lookup.
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(provider, nil).AnyTimes()
	// A non-ErrNoRows BYOK error fails route resolution while the provider stays
	// enabled, which is exactly the gap the fallback covers.
	db.EXPECT().GetUserAIProviderKeyByProviderID(gomock.Any(), database.GetUserAIProviderKeyByProviderIDParams{
		UserID:       ownerID,
		AIProviderID: providerID,
	}).Return(database.UserAIProviderKey{}, sql.ErrConnDone)

	var gotProvider sql.NullString
	db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params database.InsertChatDebugRunParams) (database.ChatDebugRun, error) {
			gotProvider = params.Provider
			return database.ChatDebugRun{ChatID: params.ChatID, Provider: params.Provider}, nil
		},
	)

	server := &Server{
		db:        db,
		logger:    logger,
		allowBYOK: true,
	}
	debugSvc := chatdebug.NewService(db, logger, nil)
	fallbackModel := &chattest.FakeModel{ProviderName: "stub", ModelName: "stub"}

	server.prepareManualTitleDebugRun(
		ctx,
		debugSvc,
		chat,
		modelConfig,
		modelBuildOptions{},
		nil,
		fallbackModel,
	)

	require.True(t, gotProvider.Valid, "debug run provider should be populated from the linked config")
	require.Equal(t, "anthropic", gotProvider.String)
}
