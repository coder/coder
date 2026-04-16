package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

type agentChatRunnerTestSetup struct {
	expClient   *codersdk.ExperimentalClient
	db          database.Store
	user        codersdk.CreateFirstUserResponse
	workspace   dbfake.WorkspaceResponse
	agentClient *agentsdk.Client
}

func TestWorkspaceAgentChatRunnerRuntimeContext(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)
		insertAgentChatRunnerUserMessage(ctx, t, setup.db, chat, setup.user.UserID, "hello from the user")

		resp, err := setup.agentClient.ChatRunnerRuntimeContext(ctx, agentsdk.ChatRunnerRuntimeContextRequest{ChatID: chat.ID})
		require.NoError(t, err)
		require.Equal(t, chat.ID, resp.ChatID)
		require.Equal(t, model.ID, resp.ModelConfigID)
		require.Equal(t, model.Provider, resp.Provider)
		require.Equal(t, model.Model, resp.Model)
		require.Contains(t, resp.ProviderAPIKeys, model.Provider)
		require.Equal(t, "test-api-key", resp.ProviderAPIKeys[model.Provider])
		require.Equal(t, chat.LeaseEpoch, resp.LeaseEpoch)
		require.NotEmpty(t, resp.Messages)
		requireChatRunnerTextMessage(t, resp.Messages[len(resp.Messages)-1], string(codersdk.ChatMessageRoleUser), "hello from the user")
	})

	t.Run("StaleLeaseReturned", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		resp, err := setup.agentClient.ChatRunnerRuntimeContext(ctx, agentsdk.ChatRunnerRuntimeContextRequest{ChatID: chat.ID})
		require.NoError(t, err)
		require.Equal(t, chat.LeaseEpoch, resp.LeaseEpoch)
	})

	t.Run("RuntimeContextIncludesMCPTools", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		mcpServer := newTestMCPServer(t, echoMCPTool())
		config := insertTestMCPServerConfig(
			ctx,
			t,
			setup.db,
			setup.user.UserID,
			mcpServer.URL,
			"runtime-context",
		)
		chat := createRunningAgentChatRunnerChatWithMCP(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
			[]uuid.UUID{config.ID},
		)
		insertAgentChatRunnerUserMessage(ctx, t, setup.db, chat, setup.user.UserID, "show me MCP tools")

		resp, err := setup.agentClient.ChatRunnerRuntimeContext(ctx, agentsdk.ChatRunnerRuntimeContextRequest{ChatID: chat.ID})
		require.NoError(t, err)
		require.NotEmpty(t, resp.MCPTools)

		var (
			mcpTool agentsdk.ChatRunnerMCPTool
			found   bool
		)
		for _, tool := range resp.MCPTools {
			if tool.MCPServerConfigID != config.ID {
				continue
			}
			mcpTool = tool
			found = true
			break
		}
		require.True(t, found, "expected runtime context to include MCP config %s", config.ID)
		require.Equal(t, config.Slug+"__echo", mcpTool.ToolName)
		require.Equal(t, "Echoes the input", mcpTool.Description)
		require.NotEmpty(t, mcpTool.InputSchema)
		require.True(t, json.Valid(mcpTool.InputSchema))
		require.Equal(t, config.DisplayName, mcpTool.ServerDisplayName)
	})

	t.Run("ChatNotOwnedByAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		otherWorkspace := dbfake.WorkspaceBuild(t, setup.db, database.WorkspaceTable{
			OrganizationID: setup.user.OrganizationID,
			OwnerID:        setup.user.UserID,
		}).WithAgent().Do()
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			otherWorkspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerRuntimeContext(ctx, agentsdk.ChatRunnerRuntimeContextRequest{ChatID: chat.ID})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat does not belong to this agent.", sdkErr.Message)
	})
}

func TestWorkspaceAgentChatRunnerMCPToolCall(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		mcpServer := newTestMCPServer(t, echoMCPTool())
		config := insertTestMCPServerConfig(
			ctx,
			t,
			setup.db,
			setup.user.UserID,
			mcpServer.URL,
			"success",
		)
		chat := createRunningAgentChatRunnerChatWithMCP(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
			[]uuid.UUID{config.ID},
		)
		insertAgentChatRunnerUserMessage(ctx, t, setup.db, chat, setup.user.UserID, "call the MCP tool")

		resp, err := setup.agentClient.ChatRunnerMCPToolCall(ctx, agentsdk.ChatRunnerMCPToolCallRequest{
			ChatID:            chat.ID,
			LeaseEpoch:        chat.LeaseEpoch,
			MCPServerConfigID: config.ID,
			ToolName:          config.Slug + "__echo",
			Args:              json.RawMessage(`{"input":"test"}`),
		})
		require.NoError(t, err)
		require.False(t, resp.IsError)

		var result string
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.Contains(t, result, "echo: test")
	})

	t.Run("RejectedServerID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		mcpServer := newTestMCPServer(t, echoMCPTool())
		detachedConfig := insertTestMCPServerConfig(
			ctx,
			t,
			setup.db,
			setup.user.UserID,
			mcpServer.URL,
			"detached",
		)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerMCPToolCall(ctx, agentsdk.ChatRunnerMCPToolCallRequest{
			ChatID:            chat.ID,
			LeaseEpoch:        chat.LeaseEpoch,
			MCPServerConfigID: detachedConfig.ID,
			ToolName:          detachedConfig.Slug + "__echo",
			Args:              json.RawMessage(`{"input":"test"}`),
		})
		sdkErr := requireSDKError(t, err, http.StatusInternalServerError)
		require.Equal(t, "Failed to execute MCP tool call.", sdkErr.Message)
	})

	t.Run("StaleLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		mcpServer := newTestMCPServer(t, echoMCPTool())
		config := insertTestMCPServerConfig(
			ctx,
			t,
			setup.db,
			setup.user.UserID,
			mcpServer.URL,
			"stale-lease",
		)
		chat := createRunningAgentChatRunnerChatWithMCP(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
			[]uuid.UUID{config.ID},
		)

		_, err := setup.agentClient.ChatRunnerMCPToolCall(ctx, agentsdk.ChatRunnerMCPToolCallRequest{
			ChatID:            chat.ID,
			LeaseEpoch:        999,
			MCPServerConfigID: config.ID,
			ToolName:          config.Slug + "__echo",
			Args:              json.RawMessage(`{"input":"test"}`),
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat lease epoch changed.", sdkErr.Message)
	})

	t.Run("WrongWorkerOwnership", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		otherWorkspace := dbfake.WorkspaceBuild(t, setup.db, database.WorkspaceTable{
			OrganizationID: setup.user.OrganizationID,
			OwnerID:        setup.user.UserID,
		}).WithAgent().Do()
		mcpServer := newTestMCPServer(t, echoMCPTool())
		config := insertTestMCPServerConfig(
			ctx,
			t,
			setup.db,
			setup.user.UserID,
			mcpServer.URL,
			"wrong-worker",
		)
		chat := createRunningAgentChatRunnerChatWithMCP(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			otherWorkspace.Agents[0].ID,
			t.Name(),
			[]uuid.UUID{config.ID},
		)

		_, err := setup.agentClient.ChatRunnerMCPToolCall(ctx, agentsdk.ChatRunnerMCPToolCallRequest{
			ChatID:            chat.ID,
			LeaseEpoch:        chat.LeaseEpoch,
			MCPServerConfigID: config.ID,
			ToolName:          config.Slug + "__echo",
			Args:              json.RawMessage(`{"input":"test"}`),
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat does not belong to this agent.", sdkErr.Message)
	})
}

func TestWorkspaceAgentChatRunnerPersistStep(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)
		toolResult := codersdk.ChatMessageToolResult(
			"tool-call-1",
			"read_file",
			json.RawMessage(`{"ok":true}`),
			false,
			false,
		)

		resp, err := setup.agentClient.ChatRunnerPersistStep(ctx, agentsdk.ChatRunnerPersistStepRequest{
			ChatID:         chat.ID,
			LeaseEpoch:     chat.LeaseEpoch,
			ModelConfigID:  model.ID,
			AssistantParts: []codersdk.ChatMessagePart{codersdk.ChatMessageText("assistant reply")},
			ToolResults:    []codersdk.ChatMessagePart{toolResult},
		})
		require.NoError(t, err)
		require.True(t, resp.OK)

		messages := requireAgentChatContextMessages(ctx, t, setup.db, chat.ID)
		require.Len(t, messages, 2)
		require.Equal(t, database.ChatMessageRoleAssistant, messages[0].Role)
		require.Equal(t, database.ChatMessageRoleTool, messages[1].Role)

		assistantParts := requireAgentChatContextParts(t, messages[0].Content.RawMessage)
		require.Len(t, assistantParts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeText, assistantParts[0].Type)
		require.Equal(t, "assistant reply", assistantParts[0].Text)

		toolResultParts := requireAgentChatContextParts(t, messages[1].Content.RawMessage)
		require.Len(t, toolResultParts, 1)
		require.Equal(t, codersdk.ChatMessagePartTypeToolResult, toolResultParts[0].Type)
		require.Equal(t, toolResult.ToolCallID, toolResultParts[0].ToolCallID)
		require.Equal(t, toolResult.ToolName, toolResultParts[0].ToolName)
		require.JSONEq(t, string(toolResult.Result), string(toolResultParts[0].Result))
		require.False(t, toolResultParts[0].IsError)
		require.False(t, toolResultParts[0].IsMedia)
	})

	t.Run("StaleLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerPersistStep(ctx, agentsdk.ChatRunnerPersistStepRequest{
			ChatID:         chat.ID,
			LeaseEpoch:     999,
			ModelConfigID:  model.ID,
			AssistantParts: []codersdk.ChatMessagePart{codersdk.ChatMessageText("assistant reply")},
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat lease epoch changed.", sdkErr.Message)
		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, chat.ID))
	})
}

func TestWorkspaceAgentChatRunnerPublishStreamPart(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		resp, err := setup.agentClient.ChatRunnerPublishStreamPart(ctx, agentsdk.ChatRunnerPublishStreamPartRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Role:       codersdk.ChatMessageRoleAssistant,
			Part:       codersdk.ChatMessageText("streaming..."),
		})
		require.NoError(t, err)
		require.True(t, resp.OK)
	})

	t.Run("StaleLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerPublishStreamPart(ctx, agentsdk.ChatRunnerPublishStreamPartRequest{
			ChatID:     chat.ID,
			LeaseEpoch: 999,
			Role:       codersdk.ChatMessageRoleAssistant,
			Part:       codersdk.ChatMessageText("streaming..."),
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat lease epoch changed.", sdkErr.Message)
	})
}

func TestWorkspaceAgentChatRunnerPublishStreamParts(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		resp, err := setup.agentClient.ChatRunnerPublishStreamParts(ctx, agentsdk.ChatRunnerPublishStreamPartsRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Parts: []agentsdk.ChatRunnerPublishStreamPart{
				{
					Role: codersdk.ChatMessageRoleAssistant,
					Part: codersdk.ChatMessageText("streaming"),
				},
				{
					Role: codersdk.ChatMessageRoleAssistant,
					Part: codersdk.ChatMessageText("still"),
				},
				{
					Role: codersdk.ChatMessageRoleAssistant,
					Part: codersdk.ChatMessageText("going"),
				},
			},
		})
		require.NoError(t, err)
		require.True(t, resp.OK)
	})

	t.Run("StaleLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerPublishStreamParts(ctx, agentsdk.ChatRunnerPublishStreamPartsRequest{
			ChatID:     chat.ID,
			LeaseEpoch: 999,
			Parts: []agentsdk.ChatRunnerPublishStreamPart{{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessageText("streaming..."),
			}},
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat lease epoch changed.", sdkErr.Message)
	})

	t.Run("EmptyParts", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerPublishStreamParts(ctx, agentsdk.ChatRunnerPublishStreamPartsRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Parts:      []agentsdk.ChatRunnerPublishStreamPart{},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "At least one stream part is required.", sdkErr.Message)
	})
}

func TestWorkspaceAgentChatRunnerPublishRateLimit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	setup := newAgentChatRunnerTestSetupWithOptions(t, &coderdtest.Options{APIRateLimit: 1})
	model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
	chat := createRunningAgentChatRunnerChat(
		ctx,
		t,
		setup.db,
		setup.user.OrganizationID,
		setup.user.UserID,
		model.ID,
		setup.workspace.Agents[0].ID,
		t.Name(),
	)

	requestBuildInfo := func() *http.Response {
		t.Helper()

		resp, err := setup.agentClient.SDK.Request(ctx, http.MethodGet, "/api/v2/buildinfo", nil)
		require.NoError(t, err)
		return resp
	}

	resp := requestBuildInfo()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = requestBuildInfo()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// The API rate limiter keys by endpoint, so we repeat each publish call to
	// prove these routes are outside the /api/v2 limiter entirely.
	for _, text := range []string{"streaming one", "streaming two"} {
		publishResp, err := setup.agentClient.ChatRunnerPublishStreamPart(ctx, agentsdk.ChatRunnerPublishStreamPartRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Role:       codersdk.ChatMessageRoleAssistant,
			Part:       codersdk.ChatMessageText(text),
		})
		require.NoError(t, err)
		require.True(t, publishResp.OK)
	}

	pluralRequests := []agentsdk.ChatRunnerPublishStreamPartsRequest{
		{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Parts: []agentsdk.ChatRunnerPublishStreamPart{{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessageText("batch one"),
			}},
		},
		{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Parts: []agentsdk.ChatRunnerPublishStreamPart{{
				Role: codersdk.ChatMessageRoleAssistant,
				Part: codersdk.ChatMessageText("batch two"),
			}},
		},
	}
	for _, req := range pluralRequests {
		publishResp, err := setup.agentClient.ChatRunnerPublishStreamParts(ctx, req)
		require.NoError(t, err)
		require.True(t, publishResp.OK)
	}
}

func TestWorkspaceAgentChatRunnerInterruptSafety(t *testing.T) {
	t.Parallel()

	t.Run("PersistStepRejectedAfterInterrupt", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		interrupted, err := setup.expClient.InterruptChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, interrupted.ID)
		require.Equal(t, codersdk.ChatStatusWaiting, interrupted.Status)
		requireAgentChatRunnerLeaseRevoked(ctx, t, setup.db, chat.ID, database.ChatStatusWaiting)

		_, err = setup.agentClient.ChatRunnerPersistStep(ctx, agentsdk.ChatRunnerPersistStepRequest{
			ChatID:         chat.ID,
			LeaseEpoch:     chat.LeaseEpoch,
			ModelConfigID:  model.ID,
			AssistantParts: []codersdk.ChatMessagePart{codersdk.ChatMessageText("assistant reply")},
		})
		requireAgentChatRunnerStaleWriteConflict(t, err)
		require.Empty(t, requireAgentChatContextMessages(ctx, t, setup.db, chat.ID))
	})

	t.Run("StreamPartRejectedAfterInterrupt", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		interrupted, err := setup.expClient.InterruptChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, interrupted.ID)
		require.Equal(t, codersdk.ChatStatusWaiting, interrupted.Status)
		requireAgentChatRunnerLeaseRevoked(ctx, t, setup.db, chat.ID, database.ChatStatusWaiting)

		_, err = setup.agentClient.ChatRunnerPublishStreamPart(ctx, agentsdk.ChatRunnerPublishStreamPartRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Role:       codersdk.ChatMessageRoleAssistant,
			Part:       codersdk.ChatMessageText("streaming after interrupt"),
		})
		requireAgentChatRunnerStaleWriteConflict(t, err)
	})
}

func TestWorkspaceAgentChatRunnerEditSafety(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	setup := newAgentChatRunnerTestSetup(t)
	model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
	chat := createRunningAgentChatRunnerChat(
		ctx,
		t,
		setup.db,
		setup.user.OrganizationID,
		setup.user.UserID,
		model.ID,
		setup.workspace.Agents[0].ID,
		t.Name(),
	)
	userMessage := insertAgentChatRunnerUserMessage(ctx, t, setup.db, chat, setup.user.UserID, "edit me")

	edited, err := setup.expClient.EditChatMessage(ctx, chat.ID, userMessage.ID, codersdk.EditChatMessageRequest{
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "edited by user",
		}},
	})
	require.NoError(t, err)
	require.NotZero(t, edited.Message.ID)
	requireAgentChatRunnerLeaseRevoked(ctx, t, setup.db, chat.ID, database.ChatStatusPending)

	_, err = setup.agentClient.ChatRunnerPersistStep(ctx, agentsdk.ChatRunnerPersistStepRequest{
		ChatID:         chat.ID,
		LeaseEpoch:     chat.LeaseEpoch,
		ModelConfigID:  model.ID,
		AssistantParts: []codersdk.ChatMessagePart{codersdk.ChatMessageText("assistant reply")},
	})
	requireAgentChatRunnerStaleWriteConflict(t, err)
}

func TestWorkspaceAgentChatRunnerReloadMessages(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)
		insertAgentChatRunnerUserMessage(ctx, t, setup.db, chat, setup.user.UserID, "reload me")

		resp, err := setup.agentClient.ChatRunnerReloadMessages(ctx, agentsdk.ChatRunnerReloadMessagesRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
		})
		require.NoError(t, err)
		require.NotEmpty(t, resp.Messages)
		requireChatRunnerTextMessage(t, resp.Messages[len(resp.Messages)-1], string(codersdk.ChatMessageRoleUser), "reload me")
	})

	t.Run("StaleLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerReloadMessages(ctx, agentsdk.ChatRunnerReloadMessagesRequest{
			ChatID:     chat.ID,
			LeaseEpoch: 999,
		})
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat lease epoch changed.", sdkErr.Message)
	})
}

func TestWorkspaceAgentChatRunnerListTemplates(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		ownerCtx, err := chattool.AsOwner(dbauthz.AsSystemRestricted(ctx), setup.db, setup.user.UserID)
		require.NoError(t, err)
		err = setup.db.UpdateTemplateDeletedByID(ownerCtx, database.UpdateTemplateDeletedByIDParams{
			ID:        setup.workspace.Template.ID,
			Deleted:   true,
			UpdatedAt: time.Now(),
		})
		require.NoError(t, err)

		tv1 := dbgen.TemplateVersion(t, setup.db, database.TemplateVersion{
			OrganizationID: setup.user.OrganizationID,
			CreatedBy:      setup.user.UserID,
		})
		tpl1 := dbgen.Template(t, setup.db, database.Template{
			OrganizationID:  setup.user.OrganizationID,
			ActiveVersionID: tv1.ID,
			CreatedBy:       setup.user.UserID,
			Name:            "list-template-one",
			DisplayName:     "Template One",
			Description:     "The first test template",
			Icon:            "/icon-one.png",
		})
		tv2 := dbgen.TemplateVersion(t, setup.db, database.TemplateVersion{
			OrganizationID: setup.user.OrganizationID,
			CreatedBy:      setup.user.UserID,
		})
		tpl2 := dbgen.Template(t, setup.db, database.Template{
			OrganizationID:  setup.user.OrganizationID,
			ActiveVersionID: tv2.ID,
			CreatedBy:       setup.user.UserID,
			Name:            "list-template-two",
			DisplayName:     "Template Two",
			Description:     "The second test template",
			Icon:            "/icon-two.png",
		})
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		resp, err := setup.agentClient.ChatRunnerListTemplates(ctx, agentsdk.ChatRunnerListTemplatesRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Page:       1,
		})
		require.NoError(t, err)
		require.Len(t, resp.Templates, 2)
		require.Equal(t, 2, resp.TotalCount)
		require.Equal(t, 1, resp.Page)
		require.Equal(t, 10, resp.PageSize)

		templatesByID := make(map[uuid.UUID]agentsdk.ChatRunnerTemplate, len(resp.Templates))
		for _, template := range resp.Templates {
			templatesByID[template.ID] = template
		}

		got1, ok := templatesByID[tpl1.ID]
		require.True(t, ok)
		require.Equal(t, tpl1.Name, got1.Name)
		require.Equal(t, tpl1.DisplayName, got1.DisplayName)
		require.Equal(t, tpl1.Description, got1.Description)
		require.Equal(t, tpl1.Icon, got1.Icon)

		got2, ok := templatesByID[tpl2.ID]
		require.True(t, ok)
		require.Equal(t, tpl2.Name, got2.Name)
		require.Equal(t, tpl2.DisplayName, got2.DisplayName)
		require.Equal(t, tpl2.Description, got2.Description)
		require.Equal(t, tpl2.Icon, got2.Icon)
	})

	t.Run("WithAllowlist", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		tv1 := dbgen.TemplateVersion(t, setup.db, database.TemplateVersion{
			OrganizationID: setup.user.OrganizationID,
			CreatedBy:      setup.user.UserID,
		})
		allowedTemplate := dbgen.Template(t, setup.db, database.Template{
			OrganizationID:  setup.user.OrganizationID,
			ActiveVersionID: tv1.ID,
			CreatedBy:       setup.user.UserID,
			Name:            "allowed-template",
			DisplayName:     "Allowed Template",
			Description:     "The template that should be returned",
			Icon:            "/allowed-icon.png",
		})
		tv2 := dbgen.TemplateVersion(t, setup.db, database.TemplateVersion{
			OrganizationID: setup.user.OrganizationID,
			CreatedBy:      setup.user.UserID,
		})
		dbgen.Template(t, setup.db, database.Template{
			OrganizationID:  setup.user.OrganizationID,
			ActiveVersionID: tv2.ID,
			CreatedBy:       setup.user.UserID,
			Name:            "blocked-template",
			DisplayName:     "Blocked Template",
			Description:     "The template that should be filtered out",
			Icon:            "/blocked-icon.png",
		})

		allowlistJSON, err := json.Marshal([]string{allowedTemplate.ID.String()})
		require.NoError(t, err)
		err = setup.db.UpsertChatTemplateAllowlist(dbauthz.AsSystemRestricted(ctx), string(allowlistJSON))
		require.NoError(t, err)

		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		resp, err := setup.agentClient.ChatRunnerListTemplates(ctx, agentsdk.ChatRunnerListTemplatesRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Page:       1,
		})
		require.NoError(t, err)
		require.Len(t, resp.Templates, 1)
		require.Equal(t, 1, resp.TotalCount)
		require.Equal(t, allowedTemplate.ID, resp.Templates[0].ID)
		require.Equal(t, allowedTemplate.Name, resp.Templates[0].Name)
	})

	t.Run("StaleLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerListTemplates(ctx, agentsdk.ChatRunnerListTemplatesRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch + 99,
			Page:       1,
		})
		requireAgentChatRunnerStaleWriteConflict(t, err)
	})

	t.Run("WrongAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		otherWorkspace := dbfake.WorkspaceBuild(t, setup.db, database.WorkspaceTable{
			OrganizationID: setup.user.OrganizationID,
			OwnerID:        setup.user.UserID,
		}).WithAgent().Do()
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			otherWorkspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerListTemplates(ctx, agentsdk.ChatRunnerListTemplatesRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			Page:       1,
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat does not belong to this agent.", sdkErr.Message)
	})

	t.Run("SubagentRejection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		rootChat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)
		childChat := createRunningChildChatRunnerChat(ctx, t, setup.db, rootChat, setup.workspace.Agents[0].ID)

		_, err := setup.agentClient.ChatRunnerListTemplates(ctx, agentsdk.ChatRunnerListTemplatesRequest{
			ChatID:     childChat.ID,
			LeaseEpoch: childChat.LeaseEpoch,
			Page:       1,
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Template operations are only available for root chats.", sdkErr.Message)
	})
}

func TestWorkspaceAgentChatRunnerReadTemplate(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		tv := dbgen.TemplateVersion(t, setup.db, database.TemplateVersion{
			OrganizationID: setup.user.OrganizationID,
			CreatedBy:      setup.user.UserID,
		})
		tpl := dbgen.Template(t, setup.db, database.Template{
			OrganizationID:  setup.user.OrganizationID,
			ActiveVersionID: tv.ID,
			CreatedBy:       setup.user.UserID,
			Name:            "read-template",
			DisplayName:     "My Template",
			Description:     "A test template",
			Icon:            "/icon.png",
		})
		param := dbgen.TemplateVersionParameter(t, setup.db, database.TemplateVersionParameter{
			TemplateVersionID: tv.ID,
			Name:              "region",
			DisplayName:       "Region",
			Description:       "Select a region",
			Type:              "string",
			DefaultValue:      "us-east-1",
			Required:          true,
			Mutable:           false,
			Options:           []byte(`[{"name":"US East","description":"Virginia","value":"us-east-1"},{"name":"EU West","description":"Ireland","value":"eu-west-1"}]`),
		})
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		resp, err := setup.agentClient.ChatRunnerReadTemplate(ctx, agentsdk.ChatRunnerReadTemplateRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			TemplateID: tpl.ID,
		})
		require.NoError(t, err)
		require.Equal(t, tpl.ID, resp.Template.ID)
		require.Equal(t, tpl.Name, resp.Template.Name)
		require.Equal(t, tpl.DisplayName, resp.Template.DisplayName)
		require.Equal(t, tpl.Description, resp.Template.Description)
		require.Equal(t, tpl.Icon, resp.Template.Icon)
		require.Len(t, resp.Parameters, 1)

		gotParam := resp.Parameters[0]
		require.Equal(t, param.Name, gotParam.Name)
		require.Equal(t, param.DisplayName, gotParam.DisplayName)
		require.Equal(t, param.Description, gotParam.Description)
		require.Equal(t, param.Type, gotParam.Type)
		require.Equal(t, param.DefaultValue, gotParam.DefaultValue)
		require.Equal(t, param.Required, gotParam.Required)
		require.Equal(t, param.Mutable, gotParam.Mutable)
		require.Len(t, gotParam.Options, 2)

		foundOption := false
		for _, option := range gotParam.Options {
			if option.Value != "us-east-1" {
				continue
			}
			require.Equal(t, "US East", option.Name)
			require.Equal(t, "Virginia", option.Description)
			foundOption = true
			break
		}
		require.True(t, foundOption)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerReadTemplate(ctx, agentsdk.ChatRunnerReadTemplateRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			TemplateID: uuid.New(),
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "Template not found.", sdkErr.Message)
	})

	t.Run("BlockedByAllowlist", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		tv := dbgen.TemplateVersion(t, setup.db, database.TemplateVersion{
			OrganizationID: setup.user.OrganizationID,
			CreatedBy:      setup.user.UserID,
		})
		tpl := dbgen.Template(t, setup.db, database.Template{
			OrganizationID:  setup.user.OrganizationID,
			ActiveVersionID: tv.ID,
			CreatedBy:       setup.user.UserID,
			Name:            "hidden-template",
			DisplayName:     "Blocked Template",
			Description:     "This template should be hidden by the allowlist",
			Icon:            "/blocked-icon.png",
		})
		allowlistJSON, err := json.Marshal([]string{uuid.New().String()})
		require.NoError(t, err)
		err = setup.db.UpsertChatTemplateAllowlist(dbauthz.AsSystemRestricted(ctx), string(allowlistJSON))
		require.NoError(t, err)

		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err = setup.agentClient.ChatRunnerReadTemplate(ctx, agentsdk.ChatRunnerReadTemplateRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch,
			TemplateID: tpl.ID,
		})
		sdkErr := requireSDKError(t, err, http.StatusNotFound)
		require.Equal(t, "Template not found.", sdkErr.Message)
	})

	t.Run("StaleLease", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		chat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)

		_, err := setup.agentClient.ChatRunnerReadTemplate(ctx, agentsdk.ChatRunnerReadTemplateRequest{
			ChatID:     chat.ID,
			LeaseEpoch: chat.LeaseEpoch + 99,
			TemplateID: uuid.New(),
		})
		requireAgentChatRunnerStaleWriteConflict(t, err)
	})

	t.Run("SubagentRejection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		setup := newAgentChatRunnerTestSetup(t)
		model := coderd.InsertAgentChatTestModelConfig(t, setup.db, setup.user.UserID)
		rootChat := createRunningAgentChatRunnerChat(
			ctx,
			t,
			setup.db,
			setup.user.OrganizationID,
			setup.user.UserID,
			model.ID,
			setup.workspace.Agents[0].ID,
			t.Name(),
		)
		childChat := createRunningChildChatRunnerChat(ctx, t, setup.db, rootChat, setup.workspace.Agents[0].ID)

		_, err := setup.agentClient.ChatRunnerReadTemplate(ctx, agentsdk.ChatRunnerReadTemplateRequest{
			ChatID:     childChat.ID,
			LeaseEpoch: childChat.LeaseEpoch,
			TemplateID: uuid.New(),
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Template operations are only available for root chats.", sdkErr.Message)
	})
}

func requireChatRunnerTextMessage(t testing.TB, message agentsdk.ChatRunnerMessage, role string, text string) {
	t.Helper()

	require.Equal(t, role, message.Role)
	require.Len(t, message.Content, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeText, message.Content[0].Type)
	require.Equal(t, text, message.Content[0].Text)
}

func requireAgentChatRunnerLeaseRevoked(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	chatID uuid.UUID,
	wantStatus database.ChatStatus,
) database.Chat {
	t.Helper()

	chat, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chatID)
	require.NoError(t, err)
	require.Equal(t, wantStatus, chat.Status)
	require.False(t, chat.WorkerID.Valid)
	require.False(t, chat.StartedAt.Valid)
	require.False(t, chat.HeartbeatAt.Valid)
	return chat
}

func requireAgentChatRunnerStaleWriteConflict(
	t *testing.T,
	err error,
) *codersdk.Error {
	t.Helper()

	sdkErr := requireSDKError(t, err, http.StatusConflict)
	require.Contains(t, []string{
		"Chat lease epoch changed.",
		"Chat lease is no longer active for this agent.",
	}, sdkErr.Message)
	return sdkErr
}

func newTestMCPServer(t *testing.T, tools ...mcpserver.ServerTool) *httptest.Server {
	t.Helper()

	srv := mcpserver.NewMCPServer("test-server", "1.0.0")
	srv.AddTools(tools...)
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)
	return ts
}

func echoMCPTool() mcpserver.ServerTool {
	return mcpserver.ServerTool{
		Tool: mcp.NewTool("echo",
			mcp.WithDescription("Echoes the input"),
			mcp.WithString("input", mcp.Description("The input"), mcp.Required()),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcp.NewToolResultText("echo: " + input), nil
		},
	}
}

func insertTestMCPServerConfig(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	userID uuid.UUID,
	serverURL string,
	nameSuffix string,
) database.MCPServerConfig {
	t.Helper()

	slug := mcpTestSlug(t, nameSuffix)
	config, err := db.InsertMCPServerConfig(
		dbauthz.AsSystemRestricted(ctx),
		database.InsertMCPServerConfigParams{
			DisplayName:   "Test MCP " + slug,
			Slug:          slug,
			Description:   "A test MCP server.",
			Transport:     "streamable_http",
			Url:           serverURL,
			AuthType:      "none",
			Availability:  "default_off",
			Enabled:       true,
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
			CreatedBy:     userID,
			UpdatedBy:     userID,
		},
	)
	require.NoError(t, err)
	return config
}

func mcpTestSlug(t testing.TB, suffix string) string {
	t.Helper()

	raw := t.Name()
	if suffix != "" {
		raw += "-" + suffix
	}
	base := strings.ToLower(raw)
	base = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, base)
	base = strings.Trim(base, "-")
	for strings.Contains(base, "--") {
		base = strings.ReplaceAll(base, "--", "-")
	}
	if base == "" {
		base = "mcp-test"
	}

	hash := uuid.NewSHA1(uuid.NameSpaceOID, []byte(raw)).String()[:8]
	maxBaseLen := 63 - len(hash) - 1
	if len(base) > maxBaseLen {
		base = strings.Trim(base[:maxBaseLen], "-")
	}
	if base == "" {
		base = "mcp-test"
	}
	return base + "-" + hash
}

func newAgentChatRunnerTestSetup(t *testing.T) agentChatRunnerTestSetup {
	t.Helper()
	return newAgentChatRunnerTestSetupWithOptions(t, nil)
}

func newAgentChatRunnerTestSetupWithOptions(
	t *testing.T,
	options *coderdtest.Options,
) agentChatRunnerTestSetup {
	t.Helper()

	if options == nil {
		options = &coderdtest.Options{}
	}
	if options.DeploymentValues == nil {
		options.DeploymentValues = coderdtest.DeploymentValues(t)
	}
	options.DeploymentValues.Experiments = []string{string(codersdk.ExperimentAgentChatRunner)}

	client, db := coderdtest.NewWithDatabase(t, options)
	user := coderdtest.CreateFirstUser(t, client)
	workspace := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: user.OrganizationID,
		OwnerID:        user.UserID,
	}).WithAgent().Do()

	return agentChatRunnerTestSetup{
		expClient:   codersdk.NewExperimentalClient(client),
		db:          db,
		user:        user,
		workspace:   workspace,
		agentClient: agentsdk.New(client.URL, agentsdk.WithFixedToken(workspace.AgentToken)),
	}
}

func createRunningAgentChatRunnerChat(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	orgID uuid.UUID,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
	agentID uuid.UUID,
	title string,
) database.Chat {
	return createRunningAgentChatRunnerChatWithMCP(
		ctx,
		t,
		db,
		orgID,
		ownerID,
		modelConfigID,
		agentID,
		title,
		nil,
	)
}

func createRunningAgentChatRunnerChatWithMCP(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	orgID uuid.UUID,
	ownerID uuid.UUID,
	modelConfigID uuid.UUID,
	agentID uuid.UUID,
	title string,
	mcpServerIDs []uuid.UUID,
) database.Chat {
	t.Helper()

	chat, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		Status:            database.ChatStatusPending,
		OrganizationID:    orgID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfigID,
		Title:             title,
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
		MCPServerIDs:      append([]uuid.UUID(nil), mcpServerIDs...),
	})
	require.NoError(t, err)

	acquired, err := db.AcquireChats(dbauthz.AsSystemRestricted(ctx), database.AcquireChatsParams{
		StartedAt:         time.Now(),
		WorkerID:          agentID,
		SkipAgentEligible: false,
		NumChats:          1,
	})
	require.NoError(t, err)
	require.Len(t, acquired, 1)
	require.Equal(t, chat.ID, acquired[0].ID)
	require.Equal(t, database.ChatStatusRunning, acquired[0].Status)
	require.True(t, acquired[0].WorkerID.Valid)
	require.Equal(t, agentID, acquired[0].WorkerID.UUID)
	require.Greater(t, acquired[0].LeaseEpoch, int64(0))
	return acquired[0]
}

func createRunningChildChatRunnerChat(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	parentChat database.Chat,
	agentID uuid.UUID,
) database.Chat {
	t.Helper()

	child, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		Status:            database.ChatStatusPending,
		OrganizationID:    parentChat.OrganizationID,
		OwnerID:           parentChat.OwnerID,
		LastModelConfigID: parentChat.LastModelConfigID,
		Title:             "child-" + t.Name(),
		AgentID:           uuid.NullUUID{UUID: agentID, Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: parentChat.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parentChat.ID, Valid: true},
	})
	require.NoError(t, err)

	acquired, err := db.AcquireChats(dbauthz.AsSystemRestricted(ctx), database.AcquireChatsParams{
		StartedAt:         time.Now(),
		WorkerID:          agentID,
		SkipAgentEligible: false,
		NumChats:          1,
	})
	require.NoError(t, err)
	require.Len(t, acquired, 1)
	require.Equal(t, child.ID, acquired[0].ID)
	return acquired[0]
}

func insertAgentChatRunnerUserMessage(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	chat database.Chat,
	createdBy uuid.UUID,
	text string,
) database.ChatMessage {
	t.Helper()

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)

	messages, err := db.InsertChatMessages(
		dbauthz.AsSystemRestricted(ctx),
		chatd.BuildSingleChatMessageInsertParams(
			chat.ID,
			database.ChatMessageRoleUser,
			content,
			database.ChatMessageVisibilityBoth,
			chat.LastModelConfigID,
			chatprompt.CurrentContentVersion,
			createdBy,
		),
	)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	return messages[0]
}
