package chatd_test

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/provisioner/echo"
	proto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type recordedOpenAIRequest struct {
	Messages      []chattest.OpenAIMessage
	Tools         []string
	Store         *bool
	ContentLength int64
}

func chatAIGatewayTransportFactoryPointer(factory aibridge.TransportFactory) *atomic.Pointer[aibridge.TransportFactory] {
	var factoryPtr atomic.Pointer[aibridge.TransportFactory]
	factoryPtr.Store(&factory)
	return &factoryPtr
}

func openAIToolName(tool chattest.OpenAITool) string {
	return cmp.Or(tool.Function.Name, tool.Name, tool.Type)
}

func mustChatLastErrorRawMessage(t testing.TB, payload codersdk.ChatError) pqtype.NullRawMessage {
	t.Helper()

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	return pqtype.NullRawMessage{RawMessage: encoded, Valid: true}
}

func requireChatLastErrorPayload(t testing.TB, raw pqtype.NullRawMessage) codersdk.ChatError {
	t.Helper()
	require.True(t, raw.Valid, "last error should be set")

	var payload codersdk.ChatError
	require.NoError(t, json.Unmarshal(raw.RawMessage, &payload))
	return payload
}

func chatLastErrorMessage(raw pqtype.NullRawMessage) string {
	if !raw.Valid {
		return ""
	}

	var payload codersdk.ChatError
	if err := json.Unmarshal(raw.RawMessage, &payload); err == nil && payload.Message != "" {
		return payload.Message
	}
	return string(raw.RawMessage)
}

func recordOpenAIRequest(req *chattest.OpenAIRequest) recordedOpenAIRequest {
	messages := append([]chattest.OpenAIMessage(nil), req.Messages...)
	tools := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, openAIToolName(tool))
	}

	var store *bool
	if req.Store != nil {
		value := *req.Store
		store = &value
	}

	var contentLength int64
	if req.Request != nil {
		contentLength = req.Request.ContentLength
	}

	return recordedOpenAIRequest{
		Messages:      messages,
		Tools:         tools,
		Store:         store,
		ContentLength: contentLength,
	}
}

func requestHasSystemSubstring(req recordedOpenAIRequest, want string) bool {
	for _, msg := range req.Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, want) {
			return true
		}
	}
	return false
}

func newWorkspaceToolTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	agentID uuid.UUID,
	planContent string,
	overrides ...func(cfg *chatd.Config),
) *chatd.Server {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, path string, _, _ int64) (io.ReadCloser, string, error) {
			if strings.HasPrefix(path, "/home/coder/.coder/plans/PLAN-") || path == "/home/coder/PLAN.md" {
				return io.NopCloser(strings.NewReader(planContent)), "", nil
			}
			return io.NopCloser(strings.NewReader("")), "", nil
		}).AnyTimes()

	configOverrides := append([]func(cfg *chatd.Config){
		func(cfg *chatd.Config) {
			cfg.AgentConn = func(_ context.Context, gotAgentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, agentID, gotAgentID)
				return mockConn, func() {}, nil
			}
		},
	}, overrides...)
	return newActiveTestServer(t, db, ps, configOverrides...)
}

func TestSubagentChatExcludesWorkspaceProvisioningTools(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	_ = agenttest.New(t, client.URL, agentToken)

	// Track tools sent in LLM requests. The first call is for the
	// root chat which spawns a subagent; the second call is for the
	// subagent itself.
	var toolsMu sync.Mutex
	toolsByCall := make([][]string, 0, 2)

	var callCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		toolsMu.Lock()
		toolsByCall = append(toolsByCall, names)
		toolsMu.Unlock()

		if callCount.Add(1) == 1 {
			// Root chat: model calls spawn_agent.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("spawn_agent", `{"type":"general","prompt":"do the thing","title":"sub"}`),
			)
		}
		// Subsequent calls (including the subagent): just reply.
		// Include literal \u0000 in the response text, which is
		// what a real LLM writes when explaining binary output.
		// json.Marshal encodes the backslash as \\, producing
		// \\u0000 in the JSON bytes. The sanitizer must not
		// corrupt this into invalid JSON.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("The file contains \\u0000 null bytes.")...,
		)
	})

	coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, openAIURL)

	// Create a root chat whose first model call will spawn a subagent.
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Spawn a subagent to do the thing.",
			},
		},
	})
	require.NoError(t, err)

	// Wait for the root chat AND the subagent to finish.
	// The root chat finishes first, then the chatd server
	// picks up and runs the child (subagent) chat.
	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != codersdk.ChatStatusWaiting && got.Status != codersdk.ChatStatusError {
			return false
		}
		// Also ensure the subagent LLM call has been made.
		toolsMu.Lock()
		n := len(toolsByCall)
		toolsMu.Unlock()
		// Expect at least 3 calls: root-1 (spawn_agent), child-1, root-2.
		return n >= 3
	}, testutil.WaitLong, testutil.IntervalFast)

	// There should be at least two streamed calls: one for the root
	// chat and one for the subagent child chat.
	toolsMu.Lock()
	recorded := append([][]string(nil), toolsByCall...)
	toolsMu.Unlock()

	require.GreaterOrEqual(t, len(recorded), 2,
		"expected at least 2 streamed LLM calls (root + subagent)")

	workspaceTools := []string{
		"list_templates", "read_template", "create_workspace",
		"start_workspace", "stop_workspace",
	}
	subagentTools := []string{"spawn_agent", "wait_agent", "message_agent", "interrupt_agent", "list_agents"}

	// Identify root and subagent calls. Root chat calls include
	// spawn_agent; the subagent call does not. Because the root chat
	// makes multiple LLM calls (before and after spawn_agent), we
	// find exactly one call that lacks spawn_agent. That's the
	// subagent.
	var rootCalls, childCalls [][]string
	for _, tools := range recorded {
		hasSpawnAgent := slice.Contains(tools, "spawn_agent")
		if hasSpawnAgent {
			rootCalls = append(rootCalls, tools)
		} else {
			childCalls = append(childCalls, tools)
		}
	}

	require.NotEmpty(t, rootCalls, "expected at least one root chat LLM call")
	require.NotEmpty(t, childCalls, "expected at least one subagent LLM call")

	// Root chat calls must include workspace and subagent tools.
	for _, tool := range workspaceTools {
		require.Contains(t, rootCalls[0], tool,
			"root chat should have workspace tool %q", tool)
	}
	for _, tool := range subagentTools {
		require.Contains(t, rootCalls[0], tool,
			"root chat should have subagent tool %q", tool)
	}

	// Standard turns (no turn mode) hide plan-only tools until
	// plan mode.
	require.NotContains(t, rootCalls[0], "ask_user_question",
		"standard-turn root chat should NOT have ask_user_question")
	require.NotContains(t, rootCalls[0], "propose_plan",
		"standard-turn root chat should NOT have propose_plan")

	// Subagent calls must NOT include workspace or subagent tools.
	for _, tool := range workspaceTools {
		require.NotContains(t, childCalls[0], tool,
			"subagent chat should NOT have workspace tool %q", tool)
	}
	for _, tool := range subagentTools {
		require.NotContains(t, childCalls[0], tool,
			"subagent chat should NOT have subagent tool %q", tool)
	}
	require.NotContains(t, childCalls[0], "ask_user_question",
		"subagent chat should NOT have ask_user_question")
}

func TestPlanModeSubagentChatExcludesAskUserQuestion(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	_ = agenttest.New(t, client.URL, agentToken)

	// Start an external MCP server whose tools should remain available to the
	// root plan-mode chat but stay hidden from plan-mode subagents.
	mcpSrv := mcpserver.NewMCPServer("plan-root-mcp", "1.0.0")
	mcpSrv.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	mcpTS := httptest.NewServer(mcpserver.NewStreamableHTTPServer(mcpSrv))
	t.Cleanup(mcpTS.Close)

	mcpConfig, err := client.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:     "Plan Root MCP",
		Slug:            "plan-root-mcp",
		Transport:       "streamable_http",
		URL:             mcpTS.URL,
		AuthType:        "none",
		Availability:    "default_off",
		Enabled:         true,
		AllowInPlanMode: true,
	})
	require.NoError(t, err)

	var toolsMu sync.Mutex
	toolsByCall := make([][]string, 0, 2)
	requestsByCall := make([]recordedOpenAIRequest, 0, 2)

	var callCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		toolsMu.Lock()
		toolsByCall = append(toolsByCall, names)
		requestsByCall = append(requestsByCall, recordOpenAIRequest(req))
		toolsMu.Unlock()

		if callCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("spawn_agent", `{"type":"general","prompt":"inspect the codebase","title":"sub"}`),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, openAIURL)

	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		PlanMode:       codersdk.ChatPlanModePlan,
		MCPServerIDs:   []uuid.UUID{mcpConfig.ID},
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Spawn a subagent to inspect the codebase.",
			},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != codersdk.ChatStatusWaiting && got.Status != codersdk.ChatStatusError {
			return false
		}
		toolsMu.Lock()
		n := len(toolsByCall)
		toolsMu.Unlock()
		return n >= 3
	}, testutil.WaitLong, testutil.IntervalFast)

	toolsMu.Lock()
	recorded := append([][]string(nil), toolsByCall...)
	recordedRequests := append([]recordedOpenAIRequest(nil), requestsByCall...)
	toolsMu.Unlock()

	require.GreaterOrEqual(t, len(recorded), 2,
		"expected at least 2 streamed LLM calls (root + subagent)")
	require.Len(t, recordedRequests, len(recorded))

	var rootCalls, childCalls [][]string
	var rootRequests, childRequests []recordedOpenAIRequest
	for i, tools := range recorded {
		if slice.Contains(tools, "spawn_agent") {
			rootCalls = append(rootCalls, tools)
			rootRequests = append(rootRequests, recordedRequests[i])
			continue
		}
		childCalls = append(childCalls, tools)
		childRequests = append(childRequests, recordedRequests[i])
	}

	require.NotEmpty(t, rootCalls, "expected at least one root chat LLM call")
	require.NotEmpty(t, childCalls, "expected at least one subagent LLM call")
	require.NotEmpty(t, rootRequests, "expected at least one root prompt")
	require.NotEmpty(t, childRequests, "expected at least one subagent prompt")
	require.Contains(t, rootCalls[0], "ask_user_question",
		"root plan-mode chat should have ask_user_question")
	require.Contains(t, rootCalls[0], "write_file",
		"root plan-mode chat should have write_file")
	require.Contains(t, rootCalls[0], "edit_files",
		"root plan-mode chat should have edit_files")
	require.Contains(t, rootCalls[0], "execute",
		"root plan-mode chat should have execute")
	require.Contains(t, rootCalls[0], "process_output",
		"root plan-mode chat should have process_output")
	require.Contains(t, rootCalls[0], "plan-root-mcp__echo",
		"root plan-mode chat should have approved external MCP tools")
	require.NotContains(t, childCalls[0], "ask_user_question",
		"plan-mode subagent should NOT have ask_user_question")
	require.NotContains(t, childCalls[0], "write_file",
		"plan-mode subagent should NOT have write_file")
	require.NotContains(t, childCalls[0], "edit_files",
		"plan-mode subagent should NOT have edit_files")
	require.Contains(t, childCalls[0], "execute",
		"plan-mode subagent should have execute")
	require.Contains(t, childCalls[0], "process_output",
		"plan-mode subagent should have process_output")
	require.NotContains(t, childCalls[0], "plan-root-mcp__echo",
		"plan-mode subagent should NOT have external MCP tools")
	require.True(t, requestHasSystemSubstring(rootRequests[0], "You are in Plan Mode."))
	require.True(t, requestHasSystemSubstring(childRequests[0], "You are in Plan Mode as a delegated sub-agent."))
	require.False(t, requestHasSystemSubstring(childRequests[0], "When the plan is ready, call propose_plan"))
}

func TestExploreSubagentIsReadOnly(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	db := api.Database
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID, func(cwr *codersdk.CreateWorkspaceRequest) {
		cwr.AutomaticUpdates = codersdk.AutomaticUpdatesNever
	})
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	_ = agenttest.New(t, client.URL, agentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

	var toolsMu sync.Mutex
	toolsByCall := make([][]string, 0, 2)
	requestsByCall := make([]recordedOpenAIRequest, 0, 2)

	var callCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		toolsMu.Lock()
		toolsByCall = append(toolsByCall, names)
		requestsByCall = append(requestsByCall, recordOpenAIRequest(req))
		toolsMu.Unlock()

		if callCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("spawn_agent", `{"type":"explore","prompt":"investigate the codebase","title":"sub"}`),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, openAIURL)

	_, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		WorkspaceID:    &workspace.ID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Spawn an Explore subagent to inspect the codebase.",
			},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		toolsMu.Lock()
		defer toolsMu.Unlock()

		sawRoot := false
		sawChild := false
		for _, tools := range toolsByCall {
			if slice.Contains(tools, "spawn_agent") {
				sawRoot = true
				continue
			}
			sawChild = true
		}
		return sawRoot && sawChild
	}, testutil.WaitLong, testutil.IntervalFast)

	toolsMu.Lock()
	recorded := append([][]string(nil), toolsByCall...)
	recordedRequests := append([]recordedOpenAIRequest(nil), requestsByCall...)
	toolsMu.Unlock()

	require.GreaterOrEqual(t, len(recorded), 2,
		"expected at least 2 streamed LLM calls (root + subagent)")
	require.Len(t, recordedRequests, len(recorded))

	var rootCalls, childCalls [][]string
	var rootRequests, childRequests []recordedOpenAIRequest
	for i, tools := range recorded {
		if slice.Contains(tools, "spawn_agent") {
			rootCalls = append(rootCalls, tools)
			rootRequests = append(rootRequests, recordedRequests[i])
			continue
		}
		childCalls = append(childCalls, tools)
		childRequests = append(childRequests, recordedRequests[i])
	}

	require.NotEmpty(t, rootCalls, "expected at least one root chat LLM call")
	require.NotEmpty(t, childCalls, "expected at least one subagent LLM call")
	require.NotEmpty(t, rootRequests, "expected at least one root prompt")
	require.NotEmpty(t, childRequests, "expected at least one subagent prompt")
	require.Contains(t, rootCalls[0], "spawn_agent")
	require.Contains(t, rootCalls[0], "write_file")
	require.Contains(t, rootCalls[0], "edit_files")
	require.NotContains(t, childCalls[0], "write_file")
	require.NotContains(t, childCalls[0], "edit_files")
	require.NotContains(t, childCalls[0], "spawn_agent")
	require.NotContains(t, childCalls[0], "wait_agent")
	require.Contains(t, childCalls[0], "read_file")
	require.Contains(t, childCalls[0], "execute")
	require.Contains(t, childCalls[0], "process_output")
	require.True(t, requestHasSystemSubstring(childRequests[0], "You are in Explore Mode as a delegated sub-agent."))
	require.False(t, requestHasSystemSubstring(rootRequests[0], "You are in Explore Mode as a delegated sub-agent."))

	rootChats, err := db.GetChats(dbauthz.AsChatd(ctx), database.GetChatsParams{
		OwnedOnly: true,
		ViewerID:  user.UserID,
	})
	require.NoError(t, err)
	rootIDs := make([]uuid.UUID, 0, len(rootChats))
	for _, root := range rootChats {
		rootIDs = append(rootIDs, root.Chat.ID)
	}
	childRows, err := db.GetChildChatsByParentIDs(dbauthz.AsChatd(ctx), database.GetChildChatsByParentIDsParams{
		ParentIds: rootIDs,
	})
	require.NoError(t, err)
	var exploreChildren []database.Chat
	for _, candidate := range childRows {
		if candidate.Chat.Mode.Valid && candidate.Chat.Mode.ChatMode == database.ChatModeExplore {
			exploreChildren = append(exploreChildren, candidate.Chat)
		}
	}
	require.Len(t, exploreChildren, 1)
}

func TestExploreChatUsesPersistedMCPSnapshot(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	externalMCP := mcpserver.NewMCPServer("external-snapshot-mcp", "1.0.0")
	externalMCP.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	externalMCPServer := httptest.NewServer(mcpserver.NewStreamableHTTPServer(externalMCP))
	defer externalMCPServer.Close()

	secondMCP := mcpserver.NewMCPServer("second-mcp", "1.0.0")
	secondMCP.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	secondMCPServer := httptest.NewServer(mcpserver.NewStreamableHTTPServer(secondMCP))
	defer secondMCPServer.Close()

	var (
		requestsMu sync.Mutex
		requests   []recordedOpenAIRequest
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		requestsMu.Lock()
		requests = append(requests, recordOpenAIRequest(req))
		requestsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	webSearchEnabled := true
	storeEnabled := true
	// OpenAI only serializes web_search through the Responses API.
	// Store=true routes there only for supported Responses models.
	webSearchModel := insertChatModelConfigWithCallConfig(
		t,
		db,
		user.ID,
		"openai",
		"gpt-4o",
		codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					Store:            &storeEnabled,
					WebSearchEnabled: &webSearchEnabled,
				},
			},
		},
	)
	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "External Snapshot MCP",
		Slug:        "external-snapshot-mcp",
		Url:         externalMCPServer.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})
	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "Second MCP",
		Slug:        "second-mcp",
		Url:         secondMCPServer.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	rootChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		LastModelConfigID: webSearchModel.ID,
		Title:             "root",
		ClientType:        database.ChatClientTypeApi,
	})

	userContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("inspect the codebase"),
	})
	require.NoError(t, err)
	createdExplore, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: rootChat.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: rootChat.ID, Valid: true},
		LastModelConfigID: webSearchModel.ID,
		Title:             "explore",
		Mode: database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		},
		MCPServerIDs: []uuid.UUID{mcpConfig.ID},
		ClientType:   database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        userContent,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: webSearchModel.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	exploreChat := createdExplore.Chat

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	workspaceToolName := "workspace-snapshot-mcp__echo"
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	_ = server

	chatResult := waitForTerminalChat(ctx, t, db, exploreChat.ID)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	requestsMu.Lock()
	recorded := append([]recordedOpenAIRequest(nil), requests...)
	requestsMu.Unlock()
	require.Len(t, recorded, 1)

	tools := recorded[0].Tools
	require.Contains(t, tools, "read_file")
	require.Contains(t, tools, "execute")
	require.Contains(t, tools, "process_output")
	require.Contains(t, tools, "external-snapshot-mcp__echo")
	require.Contains(t, tools, "web_search", "Explore provider tool filter should let web_search through when the current model supports it")
	require.NotContains(t, tools, "second-mcp__echo")
	require.NotContains(t, tools, workspaceToolName)
	require.NotContains(t, tools, "write_file")
	require.NotContains(t, tools, "edit_files")
	require.NotContains(t, tools, "spawn_agent")
}

func TestRootExploreChatStaysBuiltinOnlyAtRuntime(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	externalMCP := mcpserver.NewMCPServer("root-explore-runtime-mcp", "1.0.0")
	externalMCP.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	externalMCPServer := httptest.NewServer(mcpserver.NewStreamableHTTPServer(externalMCP))
	defer externalMCPServer.Close()

	var (
		requestsMu sync.Mutex
		requests   []recordedOpenAIRequest
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		requestsMu.Lock()
		requests = append(requests, recordOpenAIRequest(req))
		requestsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "Root Explore Runtime MCP",
		Slug:        "root-explore-runtime-mcp",
		Url:         externalMCPServer.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	exploreChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "root-explore-builtin-only",
		ModelConfigID:  model.ID,
		ChatMode: database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		},
		MCPServerIDs: []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Inspect the codebase."),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, exploreChat.ID, server)

	storedChat, err := db.GetChatByID(ctx, exploreChat.ID)
	require.NoError(t, err)
	if storedChat.Status == database.ChatStatusError {
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatLastErrorMessage(storedChat.LastError))
	}
	require.Equal(t, database.ChatStatusWaiting, storedChat.Status)
	require.ElementsMatch(t, []uuid.UUID{mcpConfig.ID}, storedChat.MCPServerIDs)

	requestsMu.Lock()
	recorded := append([]recordedOpenAIRequest(nil), requests...)
	requestsMu.Unlock()
	require.Len(t, recorded, 1)

	tools := recorded[0].Tools
	require.Contains(t, tools, "read_file")
	require.Contains(t, tools, "execute")
	require.NotContains(t, tools, "write_file")
	require.NotContains(t, tools, "root-explore-runtime-mcp__echo",
		"root Explore chats should strip persisted external MCP tools at runtime")
}

func TestRootExploreChatExcludesWebSearchProviderToolAtRuntime(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var (
		requestsMu sync.Mutex
		requests   []recordedOpenAIRequest
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		requestsMu.Lock()
		requests = append(requests, recordOpenAIRequest(req))
		requestsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	user, org, _ := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	webSearchEnabled := true
	storeEnabled := true
	// OpenAI only serializes web_search through the Responses API.
	// Store=true routes there only for supported Responses models.
	webSearchModel := insertChatModelConfigWithCallConfig(
		t,
		db,
		user.ID,
		"openai",
		"gpt-4o",
		codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
					Store:            &storeEnabled,
					WebSearchEnabled: &webSearchEnabled,
				},
			},
		},
	)

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	exploreChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "root-explore-no-provider-web-search",
		ModelConfigID:  webSearchModel.ID,
		ChatMode: database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Inspect the codebase."),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, exploreChat.ID, server)

	storedChat, err := db.GetChatByID(ctx, exploreChat.ID)
	require.NoError(t, err)
	if storedChat.Status == database.ChatStatusError {
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatLastErrorMessage(storedChat.LastError))
	}
	require.Equal(t, database.ChatStatusWaiting, storedChat.Status)

	requestsMu.Lock()
	recorded := append([]recordedOpenAIRequest(nil), requests...)
	requestsMu.Unlock()
	require.Len(t, recorded, 1)

	tools := recorded[0].Tools
	require.Contains(t, tools, "read_file")
	require.Contains(t, tools, "execute")
	require.NotContains(t, tools, "web_search",
		"root Explore chats should stay builtin-only and must not inherit provider-native web_search at runtime")
	require.NotContains(t, tools, "write_file")
}

func TestExploreChatSendMessageCannotMutateMCPSnapshot(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	newEchoMCPServer := func(name string) *httptest.Server {
		t.Helper()

		mcpSrv := mcpserver.NewMCPServer(name, "1.0.0")
		mcpSrv.AddTools(mcpserver.ServerTool{
			Tool: mcpgo.NewTool("echo",
				mcpgo.WithDescription("Echoes the input"),
				mcpgo.WithString("input",
					mcpgo.Description("The input string"),
					mcpgo.Required(),
				),
			),
			Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				input, _ := req.GetArguments()["input"].(string)
				return mcpgo.NewToolResultText("echo: " + input), nil
			},
		})
		mcpTS := httptest.NewServer(mcpserver.NewStreamableHTTPServer(mcpSrv))
		t.Cleanup(mcpTS.Close)
		return mcpTS
	}

	parentTS := newEchoMCPServer("runtime-parent-mcp")
	injectedTS := newEchoMCPServer("runtime-injected-mcp")

	var (
		requestsMu sync.Mutex
		requests   []recordedOpenAIRequest
	)
	childRequests := func() []recordedOpenAIRequest {
		requestsMu.Lock()
		defer requestsMu.Unlock()

		filtered := make([]recordedOpenAIRequest, 0, len(requests))
		for _, req := range requests {
			if requestHasSystemSubstring(req, "You are in Explore Mode as a delegated sub-agent.") {
				filtered = append(filtered, req)
			}
		}
		return filtered
	}

	var streamCallCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}

		requestsMu.Lock()
		requests = append(requests, recordOpenAIRequest(req))
		requestsMu.Unlock()

		if streamCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("spawn_agent", `{"type":"explore","prompt":"inspect the codebase","title":"sub"}`),
			)
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	parentConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "Runtime Parent MCP",
		Slug:        "runtime-parent-mcp",
		Url:         parentTS.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})
	injectedConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "Runtime Injected MCP",
		Slug:        "runtime-injected-mcp",
		Url:         injectedTS.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	rootChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "runtime-parent",
		ModelConfigID:  model.ID,
		MCPServerIDs:   []uuid.UUID{parentConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Spawn an Explore subagent to inspect the codebase."),
		},
	})
	require.NoError(t, err)

	var exploreChat database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		childRows, err := db.GetChildChatsByParentIDs(dbauthz.AsChatd(ctx), database.GetChildChatsByParentIDsParams{
			ParentIds: []uuid.UUID{rootChat.ID},
		})
		if err != nil {
			return false
		}
		for _, candidate := range childRows {
			if candidate.Chat.Mode.Valid && candidate.Chat.Mode.ChatMode == database.ChatModeExplore {
				exploreChat = candidate.Chat
				return true
			}
		}
		return false
	}, testutil.IntervalFast)

	chatResult := waitForTerminalChat(ctx, t, db, exploreChat.ID)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	exploreChat, err = db.GetChatByID(ctx, exploreChat.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{parentConfig.ID}, exploreChat.MCPServerIDs)

	initialChildRequestCount := len(childRequests())
	require.GreaterOrEqual(t, initialChildRequestCount, 1)

	updatedMCPServerIDs := []uuid.UUID{injectedConfig.ID}
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       exploreChat.ID,
		CreatedBy:    user.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("inspect the codebase again")},
		MCPServerIDs: &updatedMCPServerIDs,
	})
	require.NoError(t, err)

	storedExploreChat, err := db.GetChatByID(ctx, exploreChat.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{parentConfig.ID}, storedExploreChat.MCPServerIDs)

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return len(childRequests()) > initialChildRequestCount
	}, testutil.IntervalFast)

	chatResult = waitForTerminalChat(ctx, t, db, exploreChat.ID)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	recordedChildRequests := childRequests()
	require.GreaterOrEqual(t, len(recordedChildRequests), initialChildRequestCount+1)

	tools := recordedChildRequests[len(recordedChildRequests)-1].Tools
	require.Contains(t, tools, "runtime-parent-mcp__echo")
	require.NotContains(t, tools, "runtime-injected-mcp__echo",
		"Explore child runtime should keep the spawn-time MCP snapshot after SendMessage")
}

func TestPlanModeRootChatAllowsApprovedExternalMCPTools(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	echoMCP := mcpserver.NewMCPServer("plan-visibility-echo", "1.0.0")
	echoMCP.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	echoTS := httptest.NewServer(mcpserver.NewStreamableHTTPServer(echoMCP))
	t.Cleanup(echoTS.Close)

	filteredMCP := mcpserver.NewMCPServer("plan-visibility-filtered", "1.0.0")
	filteredMCP.AddTools(
		mcpserver.ServerTool{
			Tool: mcpgo.NewTool("visible",
				mcpgo.WithDescription("Visible tool"),
				mcpgo.WithString("input",
					mcpgo.Description("The input string"),
					mcpgo.Required(),
				),
			),
			Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				input, _ := req.GetArguments()["input"].(string)
				return mcpgo.NewToolResultText("visible: " + input), nil
			},
		},
		mcpserver.ServerTool{
			Tool: mcpgo.NewTool("hidden",
				mcpgo.WithDescription("Hidden tool"),
				mcpgo.WithString("input",
					mcpgo.Description("The input string"),
					mcpgo.Required(),
				),
			),
			Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				input, _ := req.GetArguments()["input"].(string)
				return mcpgo.NewToolResultText("hidden: " + input), nil
			},
		},
	)
	filteredTS := httptest.NewServer(mcpserver.NewStreamableHTTPServer(filteredMCP))
	t.Cleanup(filteredTS.Close)

	var (
		requests   []recordedOpenAIRequest
		requestsMu sync.Mutex
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		requestsMu.Lock()
		requests = append(requests, recordOpenAIRequest(req))
		requestsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Done.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	approvedConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName:     "Plan Approved MCP",
		Slug:            "plan-approved-mcp",
		Url:             echoTS.URL,
		AllowInPlanMode: true,
		CreatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	blockedConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "Plan Blocked MCP",
		Slug:        "plan-blocked-mcp",
		Url:         echoTS.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	filteredConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName:     "Plan Filtered MCP",
		Slug:            "plan-filtered-mcp",
		Url:             filteredTS.URL,
		AllowInPlanMode: true,
		ToolAllowList:   []string{"visible"},
		CreatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	// Workspace MCP tools now come from the agent's pinned snapshot, not live
	// discovery. Seed the workspace MCP server so chats bound to the agent
	// hydrate the "workspace-plan-mcp__echo" tool.
	seedAgentMCPToolContext(ctx, t, db, agentMCPToolContext{
		AgentID:         dbAgent.ID,
		ServerName:      "workspace-plan-mcp",
		ToolName:        "echo",
		ToolDescription: "Workspace echo tool",
	})
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	workspaceToolName := "workspace-plan-mcp__echo"
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	planChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "plan-mode-root-mcp-visibility",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		MCPServerIDs:   []uuid.UUID{approvedConfig.ID, blockedConfig.ID, filteredConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("List the available tools in plan mode."),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, planChat.ID, server)

	planChatResult, err := db.GetChatByID(ctx, planChat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, planChatResult.Status)

	askChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "ask-mode-root-mcp-visibility",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		MCPServerIDs:   []uuid.UUID{approvedConfig.ID, blockedConfig.ID, filteredConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("List the available tools outside plan mode."),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, askChat.ID, server)

	askChatResult, err := db.GetChatByID(ctx, askChat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, askChatResult.Status)

	requestsMu.Lock()
	recorded := append([]recordedOpenAIRequest(nil), requests...)
	requestsMu.Unlock()
	require.Len(t, recorded, 2, "expected exactly one streamed model call per chat")

	planTools := recorded[0].Tools
	askTools := recorded[1].Tools

	require.Contains(t, planTools, "plan-approved-mcp__echo",
		"root plan mode should expose approved external MCP tools")
	require.NotContains(t, planTools, "plan-blocked-mcp__echo",
		"root plan mode should hide unapproved external MCP tools")
	require.Contains(t, planTools, "plan-filtered-mcp__visible",
		"root plan mode should keep allowlisted tools from approved MCP servers")
	require.NotContains(t, planTools, "plan-filtered-mcp__hidden",
		"root plan mode should still respect MCP tool allowlists")
	require.NotContains(t, planTools, workspaceToolName,
		"root plan mode should exclude workspace MCP tools")

	require.Contains(t, askTools, "plan-approved-mcp__echo",
		"ask mode should keep approved external MCP tools")
	require.Contains(t, askTools, "plan-blocked-mcp__echo",
		"ask mode should keep unapproved-for-plan external MCP tools")
	require.Contains(t, askTools, "plan-filtered-mcp__visible",
		"ask mode should keep allowlisted tools from external MCP servers")
	require.NotContains(t, askTools, "plan-filtered-mcp__hidden",
		"ask mode should continue respecting MCP tool allowlists")
	require.Contains(t, askTools, workspaceToolName,
		"ask mode should continue exposing workspace MCP tools")
}

// TestUnarchiveChildChat covers the deterministic branches of the
// Server.UnarchiveChat child path: every child unarchive attempt is
// rejected with chatd.ErrArchiveRequiresRootChat.
func TestUnarchiveChildChat(t *testing.T) {
	t.Parallel()

	t.Run("ChildWithActiveParentRejected", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		parent, child := insertParentWithArchivedChild(ctx, t, db, user, org, model)

		err := replica.UnarchiveChat(ctx, child)
		require.ErrorIs(t, err, chatd.ErrArchiveRequiresRootChat)

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.True(t, dbChild.Archived, "child should remain archived")

		dbParent, err := db.GetChatByID(ctx, parent.ID)
		require.NoError(t, err)
		require.False(t, dbParent.Archived, "parent should stay active")
	})

	t.Run("ChildWithArchivedParentRejected", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		parent, child := insertParentWithArchivedChild(ctx, t, db, user, org, model)
		_, err := db.ArchiveChatByID(ctx, parent.ID)
		require.NoError(t, err)

		err = replica.UnarchiveChat(ctx, child)
		require.ErrorIs(t, err, chatd.ErrArchiveRequiresRootChat)

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.True(t, dbChild.Archived, "child should remain archived")
	})

	t.Run("ActiveChildRejected", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		_, child := insertParentWithActiveChild(t, db, user, org, model)

		err := replica.UnarchiveChat(ctx, child)
		require.ErrorIs(t, err, chatd.ErrArchiveRequiresRootChat)

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.False(t, dbChild.Archived, "child should stay active")
	})
}

// TestArchiveChat_RejectsChildChat verifies that Server.ArchiveChat
// refuses every child chat with chatd.ErrArchiveRequiresRootChat
// regardless of the family's current archive state. Archive state
// changes must always be issued against the root chat so the whole
// family flips together.
func TestArchiveChat_RejectsChildChat(t *testing.T) {
	t.Parallel()

	t.Run("ActiveChildRejected", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		parent, child := insertParentWithActiveChild(t, db, user, org, model)

		err := replica.ArchiveChat(ctx, child)
		require.ErrorIs(t, err, chatd.ErrArchiveRequiresRootChat)

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.False(t, dbChild.Archived, "child should stay active after rejected archive")

		dbParent, err := db.GetChatByID(ctx, parent.ID)
		require.NoError(t, err)
		require.False(t, dbParent.Archived, "parent should stay active after rejected child archive")
	})

	t.Run("AlreadyArchivedChildRejected", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		parent, child := insertParentWithArchivedChild(ctx, t, db, user, org, model)

		err := replica.ArchiveChat(ctx, child)
		require.ErrorIs(t, err, chatd.ErrArchiveRequiresRootChat,
			"child archive must be rejected even when the child is already archived")

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.True(t, dbChild.Archived, "child archived flag should not change")

		dbParent, err := db.GetChatByID(ctx, parent.ID)
		require.NoError(t, err)
		require.False(t, dbParent.Archived, "parent should stay active")
	})
}

// insertParentWithActiveChild creates a parent chat and an active
// child chat linked to it. Both are returned in their initial
// (active) state.
func insertParentWithActiveChild(
	t *testing.T,
	db database.Store,
	user database.User,
	org database.Organization,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()
	parent = dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "parent",
	})
	child = dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
	})
	return parent, child
}

// insertParentWithArchivedChild creates an active parent and an
// individually-archived child. The returned child reflects its
// current (archived) state in the DB.
func insertParentWithArchivedChild(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	user database.User,
	org database.Organization,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()
	parent, child = insertParentWithActiveChild(t, db, user, org, model)
	_, err := db.ArchiveChatByID(ctx, child.ID)
	require.NoError(t, err)
	child, err = db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	return parent, child
}

func TestUpdateChatHeartbeatsRequiresOwnership(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "heartbeat-ownership",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	workerID := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	// Wrong worker_id should return no IDs.
	ids, err := db.UpdateChatHeartbeats(ctx, database.UpdateChatHeartbeatsParams{
		IDs:      []uuid.UUID{chat.ID},
		WorkerID: uuid.New(),
		Now:      time.Now(),
	})
	require.NoError(t, err)
	require.Empty(t, ids)

	// Correct worker_id should return the chat's ID.
	ids, err = db.UpdateChatHeartbeats(ctx, database.UpdateChatHeartbeatsParams{
		IDs:      []uuid.UUID{chat.ID},
		WorkerID: workerID,
		Now:      time.Now(),
	})
	require.NoError(t, err)
	require.Len(t, ids, 1)
	require.Equal(t, chat.ID, ids[0])
}

func TestSendMessageQueueBehaviorQueuesWhenBusy(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "queue-when-busy",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	workerID := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: workerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)
	require.Equal(t, database.ChatStatusRunning, result.Chat.Status)
	require.Equal(t, workerID, result.Chat.WorkerID.UUID)
	require.True(t, result.Chat.WorkerID.Valid)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 1)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func TestPlanTurnPromptContract(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)

	var (
		requests   []recordedOpenAIRequest
		requestsMu sync.Mutex
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		requestsMu.Lock()
		requests = append(requests, recordOpenAIRequest(req))
		requestsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("plan acknowledged")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	planModeInstructions := "Ask about deployment sequencing before finalizing the plan."
	err := db.UpsertChatPlanModeInstructions(dbauthz.AsSystemRestricted(ctx), planModeInstructions)
	require.NoError(t, err)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newWorkspaceToolTestServer(t, db, ps, dbAgent.ID, "# Plan\n", func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		Title:          "plan-turn-prompt-contract",
		ModelConfigID:  model.ID,
		PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Plan the rollout."),
		},
	})
	require.NoError(t, err)

	waitForChatProcessed(ctx, t, db, chat.ID, server)

	requestsMu.Lock()
	recorded := append([]recordedOpenAIRequest(nil), requests...)
	requestsMu.Unlock()

	require.Len(t, recorded, 1, "expected exactly 1 streamed model call")
	require.True(t, requestHasSystemSubstring(recorded[0], "You are in Plan Mode."))
	require.True(t, requestHasSystemSubstring(recorded[0], "The only intentional authored workspace artifact is the plan file"))
	require.True(t, requestHasSystemSubstring(recorded[0], "You may use execute and process_output for exploration"))
	require.True(t, requestHasSystemSubstring(recorded[0], "approved external MCP tools when available"))
	require.True(t, requestHasSystemSubstring(recorded[0], "Workspace MCP tools are not available in root plan mode"))
	require.True(t, requestHasSystemSubstring(recorded[0], "After a successful propose_plan call, stop immediately"))
	require.True(t, requestHasSystemSubstring(recorded[0], planModeInstructions))
	for _, msg := range recorded[0].Messages {
		if msg.Role != "system" {
			continue
		}
		// The overlay prompt includes a placeholder that is replaced at
		// runtime, so strip only the stable body text before checking.
		overlayBody := strings.TrimSuffix(
			chatd.PlanningOverlayPrompt(),
			"{{CODER_CHAT_PLAN_FILE_PATH_BLOCK}}",
		)
		sanitized := strings.ReplaceAll(msg.Content, overlayBody, "")
		require.NotContains(t, sanitized, "propose_plan")
	}
}

func TestSendMessageRejectsInvalidQueuedModelConfigID(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, modelConfig := seedChatDependencies(t, db)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusRunning,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
		Title:             "reject invalid queued model config",
	})

	invalidModelConfigID := uuid.New()
	_, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		ModelConfigID: invalidModelConfigID,
	})
	require.ErrorIs(t, err, chatd.ErrInvalidModelConfigID)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)
}

func TestCreateChatInsertsWorkspaceAwarenessMessage(t *testing.T) {
	t.Parallel()

	t.Run("WithWorkspace", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())

		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		tpl := dbgen.Template(t, db, database.Template{
			CreatedBy:       user.ID,
			OrganizationID:  org.ID,
			ActiveVersionID: tv.ID,
		})
		workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			WorkspaceID:        uuid.NullUUID{UUID: workspace.ID, Valid: true},
			Title:              "test-with-workspace",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		})
		require.NoError(t, err)

		messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)

		var workspaceMsg *database.ChatMessage
		for _, msg := range messages {
			if msg.Role == database.ChatMessageRoleSystem {
				content := string(msg.Content.RawMessage)
				if strings.Contains(content, "attached to a workspace") {
					workspaceMsg = &msg
					break
				}
			}
		}
		require.NotNil(t, workspaceMsg, "workspace awareness system message should exist")
		require.Equal(t, database.ChatMessageRoleSystem, workspaceMsg.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, workspaceMsg.Visibility)
	})

	t.Run("WithoutWorkspace", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())

		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			Title:              "test-without-workspace",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
		})
		require.NoError(t, err)

		messages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)

		var workspaceMsg *database.ChatMessage
		for _, msg := range messages {
			if msg.Role == database.ChatMessageRoleSystem {
				content := string(msg.Content.RawMessage)
				if strings.Contains(content, "No workspace is attached to this chat yet") {
					workspaceMsg = &msg
					break
				}
			}
		}
		require.NotNil(t, workspaceMsg, "workspace awareness system message should exist")
		require.Equal(t, database.ChatMessageRoleSystem, workspaceMsg.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, workspaceMsg.Visibility)
		workspaceContent := string(workspaceMsg.Content.RawMessage)
		require.Contains(t, workspaceContent, "Do not create or start a workspace by default")
		require.Contains(t, workspaceContent, "Only call create_workspace or start_workspace")
		require.NotContains(t, workspaceContent, "Create one using the create_workspace tool before using workspace tools")
	})
}

func TestCreateChatRejectsWhenUsageLimitReached(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	existingChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Title:             "existing-limit-chat",
		LastModelConfigID: model.ID,
	})

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          existingChat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		ContentVersion:  chatprompt.CurrentContentVersion,
		Content:         assistantContent,
		TotalCostMicros: sql.NullInt64{Int64: 100, Valid: true},
	})

	beforeChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnedOnly: true,
		ViewerID:  user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)
	require.Len(t, beforeChats, 1)

	_, err = replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "over-limit",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.Error(t, err)

	var limitErr *chatd.UsageLimitExceededError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, int64(100), limitErr.LimitMicros)
	require.Equal(t, int64(100), limitErr.ConsumedMicros)

	afterChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnedOnly: true,
		ViewerID:  user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)
	require.Len(t, afterChats, len(beforeChats))
}

func TestAutoPromoteQueuedMessagesPreservesPerTurnModelOrder(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	firstRunStarted := make(chan struct{})
	secondRunStarted := make(chan struct{}, 1)
	thirdRunStarted := make(chan struct{}, 1)
	allowFirstRunFinish := make(chan struct{})
	var requestCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		switch requestCount.Add(1) {
		case 1:
			chunks := make(chan chattest.OpenAIChunk, 1)
			go func() {
				defer close(chunks)
				chunks <- chattest.OpenAITextChunks("first run partial")[0]
				select {
				case <-firstRunStarted:
				default:
					close(firstRunStarted)
				}
				<-allowFirstRunFinish
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		case 2:
			select {
			case secondRunStarted <- struct{}{}:
			default:
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("second run done")...)
		case 3:
			select {
			case thirdRunStarted <- struct{}{}:
			default:
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("third run done")...)
		default:
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("extra run done")...)
		}
	})

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		// Disable periodic polling so chained promotions must be driven by
		// signalWake.
		cfg.PendingChatAcquireInterval = time.Hour
	})
	user, org, modelConfigA := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	modelConfigB := insertChatModelConfigWithCallConfig(
		t,
		db,
		user.ID,
		"openai-compat",
		"gpt-4o-mini-queue-b-"+uuid.NewString(),
		codersdk.ChatModelCallConfig{},
	)
	modelConfigC := insertChatModelConfigWithCallConfig(
		t,
		db,
		user.ID,
		"openai-compat",
		"gpt-4o-mini-queue-c-"+uuid.NewString(),
		codersdk.ChatModelCallConfig{},
	)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "auto-promote per-turn model order",
		ModelConfigID:      modelConfigA.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	testutil.TryReceive(ctx, t, firstRunStarted)

	queuedB, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued b")},
		ModelConfigID: modelConfigB.ID,
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, queuedB.Queued)

	queuedC, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued c")},
		ModelConfigID: modelConfigC.ID,
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, queuedC.Queued)

	close(allowFirstRunFinish)

	testutil.TryReceive(ctx, t, secondRunStarted)
	testutil.TryReceive(ctx, t, thirdRunStarted)
	require.GreaterOrEqual(t, requestCount.Load(), int32(3))
	chatd.WaitUntilIdleForTest(server)

	queuedMessages, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queuedMessages)

	storedChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, storedChat.Status)
	require.Equal(t, modelConfigC.ID, storedChat.LastModelConfigID)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)

	var userTexts []string
	var userModelConfigIDs []uuid.UUID
	for _, message := range messages {
		if message.Role != database.ChatMessageRoleUser {
			continue
		}
		sdkMessage := db2sdk.ChatMessage(message)
		require.Len(t, sdkMessage.Content, 1)
		userTexts = append(userTexts, sdkMessage.Content[0].Text)
		require.True(t, message.ModelConfigID.Valid)
		userModelConfigIDs = append(userModelConfigIDs, message.ModelConfigID.UUID)
	}
	require.Equal(t, []string{"hello", "queued b", "queued c"}, userTexts)
	require.Equal(t, []uuid.UUID{modelConfigA.ID, modelConfigB.ID, modelConfigC.ID}, userModelConfigIDs)
}

func TestInterruptAutoPromotionIgnoresLaterUsageLimitIncrease(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)

	streamStarted := make(chan struct{})
	interrupted := make(chan struct{})
	secondRequestStarted := make(chan struct{}, 1)
	thirdRequestStarted := make(chan struct{}, 1)
	allowFinish := make(chan struct{})
	allowSecondRequestFinish := make(chan struct{})
	allowThirdRequestFinish := make(chan struct{})
	var requestCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		switch requestCount.Add(1) {
		case 1:
			chunks := make(chan chattest.OpenAIChunk, 1)
			go func() {
				defer close(chunks)
				chunks <- chattest.OpenAITextChunks("partial")[0]
				select {
				case <-streamStarted:
				default:
					close(streamStarted)
				}
				<-req.Context().Done()
				select {
				case <-interrupted:
				default:
					close(interrupted)
				}
				<-allowFinish
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		case 2:
			select {
			case secondRequestStarted <- struct{}{}:
			default:
			}
			chunks := make(chan chattest.OpenAIChunk, 1)
			go func() {
				defer close(chunks)
				chunks <- chattest.OpenAITextChunks("second run partial")[0]
				select {
				case <-allowSecondRequestFinish:
				case <-req.Context().Done():
				}
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		case 3:
			select {
			case thirdRequestStarted <- struct{}{}:
			default:
			}
			chunks := make(chan chattest.OpenAIChunk, 1)
			go func() {
				defer close(chunks)
				chunks <- chattest.OpenAITextChunks("third run partial")[0]
				select {
				case <-allowThirdRequestFinish:
				case <-req.Context().Done():
				}
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.Clock = clock
		// Keep periodic polling frozen so request handoff is synchronized
		// through explicit mock channels.
		cfg.PendingChatAcquireInterval = time.Hour
		cfg.InFlightChatStaleAfter = testutil.WaitSuperLong
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-autopromote-limit",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	testutil.TryReceive(ctx, t, streamStarted)

	queuedResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	require.True(t, queuedResult.Queued)
	require.NotNil(t, queuedResult.QueuedMessage)

	testutil.TryReceive(ctx, t, interrupted)

	close(allowFinish)
	testutil.TryReceive(ctx, t, secondRequestStarted)

	laterQueuedResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("later queued")},
	})
	require.NoError(t, err)
	require.True(t, laterQueuedResult.Queued)
	require.NotNil(t, laterQueuedResult.QueuedMessage)

	spendChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "other-spend",
	})

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("spent elsewhere"),
	})
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          spendChat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		ContentVersion:  chatprompt.CurrentContentVersion,
		Content:         assistantContent,
		TotalCostMicros: sql.NullInt64{Int64: 100, Valid: true},
	})

	close(allowSecondRequestFinish)
	testutil.TryReceive(ctx, t, thirdRequestStarted)
	require.GreaterOrEqual(t, requestCount.Load(), int32(3))

	close(allowThirdRequestFinish)
	chatd.WaitUntilIdleForTest(server)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)
	require.False(t, fromDB.WorkerID.Valid)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)

	userTexts := make([]string, 0, 3)
	for _, message := range messages {
		if message.Role != database.ChatMessageRoleUser {
			continue
		}
		sdkMessage := db2sdk.ChatMessage(message)
		if len(sdkMessage.Content) != 1 {
			continue
		}
		userTexts = append(userTexts, sdkMessage.Content[0].Text)
	}
	require.Equal(t, []string{"hello", "queued", "later queued"}, userTexts)
}

func TestEditMessageRejectsWhenUsageLimitReached(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "edit-limit-reached",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
	})
	require.NoError(t, err)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	editedMessageID := messages[0].ID

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:          chat.ID,
		ModelConfigID:   uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:            database.ChatMessageRoleAssistant,
		ContentVersion:  chatprompt.CurrentContentVersion,
		Content:         assistantContent,
		TotalCostMicros: sql.NullInt64{Int64: 100, Valid: true},
	})

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: editedMessageID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.Error(t, err)

	var limitErr *chatd.UsageLimitExceededError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, int64(100), limitErr.LimitMicros)
	require.Equal(t, int64(100), limitErr.ConsumedMicros)

	messages, err = db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	originalMessage := db2sdk.ChatMessage(messages[0])
	require.Len(t, originalMessage.Content, 1)
	require.Equal(t, "original", originalMessage.Content[0].Text)
}

func TestEditMessageRejectsMissingMessage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "missing-edited-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: 999999,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, chatd.ErrEditedMessageNotFound))
}

func TestEditMessageRejectsNonUserMessage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "non-user-edited-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	assistantMessage := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         chat.ID,
		ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:           database.ChatMessageRoleAssistant,
		ContentVersion: chatprompt.CurrentContentVersion,
		Content:        assistantContent,
	})

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: assistantMessage.ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, chatd.ErrEditedMessageNotUser))
}

// TestEditMessageDebugCleanupDeletesPreEditRuns verifies that
// EditMessage schedules the chat debug cleanup goroutine when debug
// logging is enabled and that it deletes debug runs tied to the
// pre-edit conversation branch. This exercises the chatd wiring end
// to end: lazy debugService init, editCutoff sampling from the DB,
// and the scheduleDebugCleanup retry loop against a real Postgres
// store.
func TestEditMessageDebugCleanupDeletesPreEditRuns(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newDebugEnabledTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "debug-edit-cleanup",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("first")},
	})
	require.NoError(t, err)

	msgs, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: chat.ID, AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	editedMsgID := msgs[0].ID

	// Stale debug run tied to the pre-edit message branch. Stamped
	// well outside the clock-skew buffer so the fast retry path
	// deletes it instead of deferring to the stale sweeper.
	staleStart := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	staleRun, err := db.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: model.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: editedMsgID, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: editedMsgID, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: "openai", Valid: true},
		Model:               sql.NullString{String: model.Model, Valid: true},
		StartedAt:           sql.NullTime{Time: staleStart, Valid: true},
		UpdatedAt:           sql.NullTime{Time: staleStart, Valid: true},
	})
	require.NoError(t, err)

	// Run tied to an earlier message branch that the message-id
	// filter should leave alone even though it predates the edit.
	unrelatedRun, err := db.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: model.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: editedMsgID - 1, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: editedMsgID - 1, Valid: true},
		Kind:                "chat_turn",
		Status:              "completed",
		Provider:            sql.NullString{String: "openai", Valid: true},
		Model:               sql.NullString{String: model.Model, Valid: true},
		StartedAt:           sql.NullTime{Time: staleStart, Valid: true},
		UpdatedAt:           sql.NullTime{Time: staleStart, Valid: true},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: editedMsgID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.NoError(t, err)

	chatd.WaitUntilIdleForTest(replica)

	// ErrNoRows on staleRun proves the fast-retry path DELETED the
	// row: FinalizeStale (the only other debug-row writer on the
	// server) only UPDATEs finished_at in place, it never deletes,
	// so the row can only disappear via DeleteAfterMessageID which
	// is reached solely from scheduleDebugCleanup.
	_, err = db.GetChatDebugRunByID(ctx, staleRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows,
		"pre-edit run matching the message-id filter should be deleted")

	remaining, err := db.GetChatDebugRunByID(ctx, unrelatedRun.ID)
	require.NoError(t, err,
		"runs outside the edited message branch must survive cleanup")
	require.Equal(t, unrelatedRun.ID, remaining.ID)

	// Count the seeded rows that survive so the delete count is
	// verified directly (not just by negative lookup). Scoped to
	// seeded IDs because the processor may start a new chat_turn
	// run in parallel when EditMessage transitions the chat back to
	// pending.
	remainingRuns, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID: chat.ID, LimitVal: 100,
	})
	require.NoError(t, err)
	seeded := map[uuid.UUID]bool{staleRun.ID: true, unrelatedRun.ID: true}
	survivors := 0
	for _, r := range remainingRuns {
		if seeded[r.ID] {
			survivors++
		}
	}
	require.Equal(t, 1, survivors,
		"exactly one of the two seeded runs should survive (the unrelated run)")
}

// TestEditMessageDebugCleanupPreservesRecentRuns verifies that the
// clock-skew buffer in the edit-cleanup cutoff prevents the fast
// retry from deleting debug runs that started within the buffer
// window. The stale sweep handles those leftovers later.
func TestEditMessageDebugCleanupPreservesRecentRuns(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newDebugEnabledTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "debug-edit-buffer",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("first")},
	})
	require.NoError(t, err)

	msgs, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: chat.ID, AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	editedMsgID := msgs[0].ID

	// Within the 30s skew buffer, so the fast retry must leave it
	// alone even though its message ID matches the delete filter.
	recentStart := time.Now().Add(-time.Second).UTC().Truncate(time.Microsecond)
	recentRun, err := db.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:              chat.ID,
		ModelConfigID:       uuid.NullUUID{UUID: model.ID, Valid: true},
		TriggerMessageID:    sql.NullInt64{Int64: editedMsgID, Valid: true},
		HistoryTipMessageID: sql.NullInt64{Int64: editedMsgID, Valid: true},
		Kind:                "chat_turn",
		Status:              "in_progress",
		Provider:            sql.NullString{String: "openai", Valid: true},
		Model:               sql.NullString{String: model.Model, Valid: true},
		StartedAt:           sql.NullTime{Time: recentStart, Valid: true},
		UpdatedAt:           sql.NullTime{Time: recentStart, Valid: true},
	})
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: editedMsgID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.NoError(t, err)

	chatd.WaitUntilIdleForTest(replica)

	remaining, err := db.GetChatDebugRunByID(ctx, recentRun.ID)
	require.NoError(t, err,
		"runs inside the clock-skew buffer must survive the fast retry")
	require.Equal(t, recentRun.ID, remaining.ID)

	// If the clock-skew buffer were removed the fast retry would
	// have deleted recentRun. Verify the count of seeded survivors
	// directly, ignoring any new chat_turn run the processor may
	// create after the pending status transition.
	remainingRuns, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID: chat.ID, LimitVal: 100,
	})
	require.NoError(t, err)
	survivors := 0
	for _, r := range remainingRuns {
		if r.ID == recentRun.ID {
			survivors++
		}
	}
	require.Equal(t, 1, survivors,
		"the buffered run must survive the fast retry")
}

func TestRecoverStaleRequiresActionChat(t *testing.T) {
	t.Parallel()

	db, ps, rawDB := dbtestutil.NewDBWithSQLDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	openAIURL := chattest.OpenAI(t)
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)

	toolName := "my_dynamic_tool"
	dynamicToolsJSON, err := json.Marshal([]mcpgo.Tool{{
		Name:        toolName,
		Description: "A test dynamic tool.",
		InputSchema: mcpgo.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{},
		},
	}})
	require.NoError(t, err)

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("hello"),
	})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "stale-requires-action",
		DynamicTools:      nullRawMessage(dynamicToolsJSON),
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)

	toolCallID := "call_" + uuid.NewString()
	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Args:       json.RawMessage(`{}`),
		},
	})
	require.NoError(t, err)
	machine := chatstate.NewChatMachine(db, ps, created.Chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{
				{
					Role:           database.ChatMessageRoleAssistant,
					Content:        assistantContent,
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
				},
			},
		})
		return err
	}))
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
		return err
	}))
	_, err = rawDB.ExecContext(ctx,
		"UPDATE chats SET requires_action_deadline_at = $1 WHERE id = $2",
		time.Now().Add(-time.Hour), created.Chat.ID)
	require.NoError(t, err)

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	server.Start()

	chatResult := waitForTerminalChat(ctx, t, db, created.Chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)
	require.False(t, chatResult.RequiresActionDeadlineAt.Valid)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: created.Chat.ID,
	})
	require.NoError(t, err)
	require.Len(t, messages, 4)
	parts, err := chatprompt.ParseContent(messages[2])
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[0].Type)
	require.Equal(t, toolCallID, parts[0].ToolCallID)
	require.Equal(t, toolName, parts[0].ToolName)
	require.True(t, parts[0].IsError)
	require.JSONEq(t, `"Tool execution timed out"`, string(parts[0].Result))
}

func TestNewReplicaRecoversStaleChatFromDeadReplica(t *testing.T) {
	t.Parallel()

	db, ps, rawDB := dbtestutil.NewDBWithSQLDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	openAIURL := chattest.OpenAI(t)
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("hello"),
	})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "orphaned-chat",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)

	deadWorkerID := uuid.New()
	deadRunnerID := uuid.New()
	machine := chatstate.NewChatMachine(db, ps, created.Chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: deadWorkerID, RunnerID: deadRunnerID})
		return err
	}))
	// Simulate a chat left running by a dead replica with a stale
	// heartbeat (well beyond the stale threshold).
	_, err = rawDB.ExecContext(ctx,
		"UPDATE chat_heartbeats SET heartbeat_at = $1 WHERE chat_id = $2 AND runner_id = $3",
		time.Now().Add(-time.Hour), created.Chat.ID, deadRunnerID)
	require.NoError(t, err)

	newWorkerID := uuid.New()
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newTestServer(t, db, ps, newWorkerID, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})
	// Start a new replica. It should recover the stale chat on
	// startup.
	server.Start()

	var recovered database.Chat
	require.Eventually(t, func() bool {
		recovered, err = db.GetChatByID(ctx, created.Chat.ID)
		if err != nil {
			return false
		}
		return recovered.Status == database.ChatStatusWaiting &&
			!recovered.WorkerID.Valid &&
			!recovered.RunnerID.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestWaitingChatsAreNotRecoveredAsStale(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	// Create a chat in waiting status. This should NOT be touched
	// by stale recovery.
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Title:             "waiting-chat",
		LastModelConfigID: model.ID,
	})

	// Start a replica with a short stale threshold.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: testutil.WaitLong,
		InFlightChatStaleAfter:     500 * time.Millisecond,
	})
	server.Start()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// Wait long enough for multiple periodic recovery cycles to
	// run (staleAfter/5 = 100ms intervals).
	require.Never(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status != database.ChatStatusWaiting
	}, time.Second, testutil.IntervalFast,
		"waiting chat should not be modified by stale recovery")
}

func TestUpdateChatStatusPersistsLastError(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	_ = newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Title:             "error-persisted",
		LastModelConfigID: model.ID,
	})

	// Write a minimal structured last_error payload through the
	// query layer, then verify it round-trips through storage.
	errorMessage := "stream response: status 500: internal server error"
	wantPayload := codersdk.ChatError{
		Message: errorMessage,
		Kind:    codersdk.ChatErrorKindGeneric,
	}
	chat, err := db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusError,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   mustChatLastErrorRawMessage(t, wantPayload),
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, chat.Status)
	require.Equal(t, wantPayload, requireChatLastErrorPayload(t, chat.LastError))

	// Verify the error is persisted when re-read from the database.
	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, fromDB.Status)
	require.Equal(t, wantPayload, requireChatLastErrorPayload(t, fromDB.LastError))

	// Verify the error is cleared when the chat transitions to a
	// non-error status (e.g. running after a retry).
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   pqtype.NullRawMessage{},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, chat.Status)
	require.False(t, chat.LastError.Valid)

	fromDB, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, fromDB.LastError.Valid)
}

func TestSubscribeSnapshotIncludesStatusEvent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "status-snapshot",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	snapshot, _, cancel, ok := replica.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Passive server: status is always Pending.
	require.NotEmpty(t, snapshot)
	statusIdx := -1
	for i, event := range snapshot {
		if event.Type == codersdk.ChatStreamEventTypeStatus {
			statusIdx = i
			break
		}
	}
	require.NotEqual(t, -1, statusIdx)
	require.NotNil(t, snapshot[statusIdx].Status)
}

func TestPersistToolResultWithBinaryData(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const binaryOutputBase64 = "SEVBREVSAAAAc29tZSBkYXRhAABtb3JlIGRhdGEARU5E"
	binaryOutput, err := io.ReadAll(base64.NewDecoder(
		base64.StdEncoding,
		strings.NewReader(binaryOutputBase64),
	))
	require.NoError(t, err)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Binary tool result test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"execute",
					`{"command":"cat /home/coder/binary_file.bin"}`,
				),
			)
		}
		// Include literal \u0000 in the response text, which is
		// what a real LLM writes when explaining binary output.
		// json.Marshal encodes the backslash as \\, producing
		// \\u0000 in the JSON bytes. The sanitizer must not
		// corrupt this into invalid JSON.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("The file contains \\u0000 null bytes.")...,
		)
	})

	// Use "openai-compat" provider so the chatd framework uses the
	// /chat/completions endpoint, where the mock server supports
	// streaming tool calls. The default "openai" provider routes to
	// /responses which only handles text deltas in the mock.
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().
		SetExtraHeaders(gomock.Any()).
		AnyTimes()
	mockConn.EXPECT().
		ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).
		AnyTimes()
	mockConn.EXPECT().
		LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).
		AnyTimes()
	mockConn.EXPECT().
		ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).
		AnyTimes()
	mockConn.EXPECT().
		StartProcess(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req workspacesdk.StartProcessRequest) (workspacesdk.StartProcessResponse, error) {
			require.Equal(t, "cat /home/coder/binary_file.bin", req.Command)
			return workspacesdk.StartProcessResponse{ID: "proc-binary", Started: true}, nil
		}).
		Times(1)
	mockConn.EXPECT().
		ProcessOutput(gomock.Any(), "proc-binary", gomock.Any()).
		Return(workspacesdk.ProcessOutputResponse{
			Output:   string(binaryOutput),
			Running:  false,
			ExitCode: ptrRef(0),
		}, nil).
		AnyTimes()

	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "binary-tool-result",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Read /home/coder/binary_file.bin."),
		},
	})
	require.NoError(t, err)

	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	var toolMessage *database.ChatMessage
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for i := range messages {
			if messages[i].Role == database.ChatMessageRoleTool {
				toolMessage = &messages[i]
				return true
			}
		}
		return false
	}, testutil.IntervalFast)
	require.NotNil(t, toolMessage)

	parts, err := chatprompt.ParseContent(*toolMessage)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[0].Type)
	require.Equal(t, "execute", parts[0].ToolName)

	var result chattool.ExecuteResult
	require.NoError(t, json.Unmarshal(parts[0].Result, &result))
	require.True(t, result.Success)
	require.Equal(t, string(binaryOutput), result.Output)
	require.Equal(t, 0, result.ExitCode)

	require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	streamedCallsMu.Lock()
	recordedStreamCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.GreaterOrEqual(t, len(recordedStreamCalls), 2)

	var foundToolResultInSecondCall bool
	for _, message := range recordedStreamCalls[1] {
		if message.Role != "tool" {
			continue
		}
		if !json.Valid([]byte(message.Content)) {
			continue
		}
		var result chattool.ExecuteResult
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		if result.Output == string(binaryOutput) {
			foundToolResultInSecondCall = true
			break
		}
	}
	require.True(t, foundToolResultInSecondCall, "expected second streamed model call to include execute tool output")
}

func TestRequiresActionChatPersistsWaitingStatusLabel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Dynamic tool test")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAIToolCallChunk(
				"my_dynamic_tool",
				`{"input":"hello world"}`,
			),
		)
	})

	mockPush := &mockWebpushDispatcher{}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(factory),
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	dynamicToolsJSON, err := json.Marshal([]mcpgo.Tool{{
		Name:        "my_dynamic_tool",
		Description: "A test dynamic tool.",
		InputSchema: mcpgo.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"input": map[string]any{"type": "string"},
			},
			Required: []string{"input"},
		},
	}})
	require.NoError(t, err)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "requires-action-status-label",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Please call the dynamic tool."),
		},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)
	seedLastTurnSummary(ctx, t, db, chat, "previous summary")

	server.Start()

	var fromDB database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		got, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		fromDB = got
		if got.Status == database.ChatStatusError {
			return true
		}
		return got.Status == database.ChatStatusRequiresAction &&
			got.LastTurnSummary.Valid &&
			got.LastTurnSummary.String == "Waiting for user input"
	}, testutil.IntervalFast)
	chatd.WaitUntilIdleForTest(server)

	require.Equal(t, database.ChatStatusRequiresAction, fromDB.Status,
		"expected requires_action, got %s (last_error=%q)",
		fromDB.Status, string(fromDB.LastError.RawMessage))
	require.Equal(t, sql.NullString{String: "Waiting for user input", Valid: true}, fromDB.LastTurnSummary,
		"requires action chats should persist a waiting status label")
	require.Equal(t, int32(0), mockPush.dispatchCount.Load(),
		"expected no web push dispatch for a requires_action chat")
}

func TestActiveServer_InterruptionBehavior(t *testing.T) {
	t.Parallel()

	t.Run("partial stream commits synthetic tool result and promotes queued message", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		streamStarted := make(chan struct{})
		var requestCount atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}

			if requestCount.Add(1) != 1 {
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("queued response")...)
			}
			chunks := make(chan chattest.AnthropicChunk, 5)
			go func() {
				defer close(chunks)
				chunks <- chattest.AnthropicChunk{
					Type: "message_start",
					Message: chattest.AnthropicChunkMessage{
						ID:    "msg-partial-interrupt",
						Type:  "message",
						Role:  "assistant",
						Model: "claude-3-opus-20240229",
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_start",
					Index: 0,
					ContentBlock: chattest.AnthropicContentBlock{
						Type: "text",
						Text: "",
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_delta",
					Index: 0,
					Delta: chattest.AnthropicDeltaBlock{Type: "text_delta", Text: "partial assistant output"},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_start",
					Index: 1,
					ContentBlock: chattest.AnthropicContentBlock{
						Type: "tool_use",
						ID:   "interrupt-tool-1",
						Name: "read_file",
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_delta",
					Index: 1,
					Delta: chattest.AnthropicDeltaBlock{Type: "input_json_delta", PartialJSON: `{"path":"main.go"}`},
				}
				select {
				case <-streamStarted:
				default:
					close(streamStarted)
				}
				<-req.Context().Done()
			}()
			return chattest.AnthropicResponse{StreamingChunks: chunks}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "interrupt-partial-tool",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("start and call a tool"),
			},
		})
		require.NoError(t, err)

		testutil.TryReceive(ctx, t, streamStarted)
		queued, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued after interrupt")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorInterrupt,
		})
		require.NoError(t, err)
		require.True(t, queued.Queued)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.GreaterOrEqual(t, requestCount.Load(), int32(2))

		messages := chatMessages(ctx, t, db, chat.ID)
		var userTexts []string
		var foundPartial bool
		for _, msg := range messages {
			parts, parseErr := chatprompt.ParseContent(msg)
			require.NoError(t, parseErr)
			switch msg.Role {
			case database.ChatMessageRoleUser:
				for _, part := range parts {
					if part.Type == codersdk.ChatMessagePartTypeText {
						userTexts = append(userTexts, part.Text)
					}
				}
			case database.ChatMessageRoleAssistant:
				for _, part := range parts {
					if part.Type == codersdk.ChatMessagePartTypeText && strings.Contains(part.Text, "partial assistant output") {
						foundPartial = true
					}
				}
			}
		}
		require.Equal(t, []string{"start and call a tool", "queued after interrupt"}, userTexts)
		require.True(t, foundPartial)

		parts := chatToolParts(ctx, t, db, chat.ID)
		call := requireToolCallPart(t, parts, "read_file")
		require.Equal(t, "interrupt-tool-1", call.ToolCallID)
		require.Empty(t, call.Args)
		require.Nil(t, call.CreatedAt, "incomplete streamed call should not have a durable call timestamp")
		result := requireToolResultPart(t, parts, "read_file")
		require.Equal(t, "interrupt-tool-1", result.ToolCallID)
		require.True(t, result.IsError)
		require.JSONEq(t, `{"error":"tool call was interrupted before it produced a result"}`, string(result.Result))
		require.NotNil(t, result.CreatedAt)
	})

	t.Run("tool execution cancellation commits interrupted result", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var requestCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}

			if requestCount.Add(1) == 1 {
				chunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/slow.txt"}`)
				chunk.Choices[0].ToolCalls[0].ID = "tc-slow"
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("calling tool")[0],
					chunk,
				)
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("after interrupt")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
		toolStarted := make(chan struct{})

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/slow.txt", int64(1), int64(0), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _, _ int64, _ workspacesdk.ReadFileLinesLimits) (workspacesdk.ReadFileLinesResponse, error) {
				close(toolStarted)
				<-ctx.Done()
				return workspacesdk.ReadFileLinesResponse{}, ctx.Err()
			}).Times(1)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "interrupt-tool-execution",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("run the slow tool"),
			},
		})
		require.NoError(t, err)

		testutil.TryReceive(ctx, t, toolStarted)
		queued, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue after interrupt")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorInterrupt,
		})
		require.NoError(t, err)
		require.True(t, queued.Queued)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.GreaterOrEqual(t, requestCount.Load(), int32(2))

		messages := chatMessages(ctx, t, db, chat.ID)
		var foundText bool
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleAssistant {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			require.NoError(t, parseErr)
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeText && strings.Contains(part.Text, "calling tool") {
					foundText = true
				}
			}
		}
		require.True(t, foundText)

		parts := chatToolParts(ctx, t, db, chat.ID)
		call := requireToolCallPart(t, parts, "read_file")
		require.Equal(t, "tc-slow", call.ToolCallID)
		require.NotNil(t, call.CreatedAt)
		result := requireToolResultPart(t, parts, "read_file")
		require.Equal(t, "tc-slow", result.ToolCallID)
		require.True(t, result.IsError)
		require.JSONEq(t, `{"error":"tool call was interrupted before it produced a result"}`, string(result.Result))
		require.NotNil(t, result.CreatedAt)
		require.False(t, result.CreatedAt.Before(*call.CreatedAt))
	})

	t.Run("anthropic provider-only interruption commits no synthetic result", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		webSearchEnabled := true
		providerToolStarted := make(chan struct{})
		var requestCount atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}

			if requestCount.Add(1) != 1 {
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("after interrupt")...)
			}
			chunks := make(chan chattest.AnthropicChunk, 2)
			go func() {
				defer close(chunks)
				chunks <- chattest.AnthropicChunk{
					Type: "message_start",
					Message: chattest.AnthropicChunkMessage{
						ID:    "msg-provider-interrupt",
						Type:  "message",
						Role:  "assistant",
						Model: "claude-3-opus-20240229",
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_start",
					Index: 0,
					ContentBlock: chattest.AnthropicContentBlock{
						Type:  "server_tool_use",
						ID:    "ws-interrupt",
						Name:  "web_search",
						Input: json.RawMessage(`{"query":"coder"}`),
					},
				}
				select {
				case <-providerToolStarted:
				default:
					close(providerToolStarted)
				}
				<-req.Context().Done()
			}()
			return chattest.AnthropicResponse{StreamingChunks: chunks}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		model = updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{WebSearchEnabled: &webSearchEnabled},
			},
		})

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search for coder")
		testutil.TryReceive(ctx, t, providerToolStarted)
		queued, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue after provider interrupt")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorInterrupt,
		})
		require.NoError(t, err)
		require.True(t, queued.Queued)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		parts := chatToolParts(ctx, t, db, chat.ID)
		require.False(t, toolResultPartExists(parts, "web_search"),
			"provider-executed web_search should not get a synthetic local result")
	})

	t.Run("anthropic mixed provider and local interruption keeps local synthetic result", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		webSearchEnabled := true
		streamStarted := make(chan struct{})
		var requestCount atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}

			if requestCount.Add(1) != 1 {
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("after interrupt")...)
			}
			chunks := make(chan chattest.AnthropicChunk, 3)
			go func() {
				defer close(chunks)
				chunks <- chattest.AnthropicChunk{
					Type: "message_start",
					Message: chattest.AnthropicChunkMessage{
						ID:    "msg-mixed-interrupt",
						Type:  "message",
						Role:  "assistant",
						Model: "claude-3-opus-20240229",
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_start",
					Index: 0,
					ContentBlock: chattest.AnthropicContentBlock{
						Type:  "server_tool_use",
						ID:    "ws-interrupt",
						Name:  "web_search",
						Input: json.RawMessage(`{"query":"coder"}`),
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_start",
					Index: 1,
					ContentBlock: chattest.AnthropicContentBlock{
						Type: "tool_use",
						ID:   "tc-local",
						Name: "read_file",
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_delta",
					Index: 1,
					Delta: chattest.AnthropicDeltaBlock{Type: "input_json_delta", PartialJSON: `{"path":"main.go"}`},
				}
				select {
				case <-streamStarted:
				default:
					close(streamStarted)
				}
				<-req.Context().Done()
			}()
			return chattest.AnthropicResponse{StreamingChunks: chunks}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		model = updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{WebSearchEnabled: &webSearchEnabled},
			},
		})
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "anthropic-mixed-interrupt",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("search and read"),
			},
		})
		require.NoError(t, err)
		testutil.TryReceive(ctx, t, streamStarted)
		queued, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue after mixed interrupt")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorInterrupt,
		})
		require.NoError(t, err)
		require.True(t, queued.Queued)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		parts := chatToolParts(ctx, t, db, chat.ID)
		require.False(t, toolResultPartExists(parts, "web_search"))
		call := requireToolCallPart(t, parts, "read_file")
		require.Equal(t, "tc-local", call.ToolCallID)
		require.False(t, call.ProviderExecuted)
		result := requireToolResultPart(t, parts, "read_file")
		require.Equal(t, "tc-local", result.ToolCallID)
		require.False(t, result.ProviderExecuted)
		require.True(t, result.IsError)
		require.JSONEq(t, `{"error":"tool call was interrupted before it produced a result"}`, string(result.Result))
	})

	t.Run("interrupted reasoning persists timestamps", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		sendReasoning := true
		thinkingBudget := int64(1024)
		reasoningStarted := make(chan struct{})
		var requestCount atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}

			if requestCount.Add(1) != 1 {
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("after interrupt")...)
			}
			chunks := make(chan chattest.AnthropicChunk, 3)
			go func() {
				defer close(chunks)
				chunks <- chattest.AnthropicChunk{
					Type: "message_start",
					Message: chattest.AnthropicChunkMessage{
						ID:    "msg-reasoning-interrupt",
						Type:  "message",
						Role:  "assistant",
						Model: "claude-3-opus-20240229",
					},
				}
				chunks <- chattest.AnthropicChunk{
					Type:         "content_block_start",
					Index:        0,
					ContentBlock: chattest.AnthropicContentBlock{Type: "thinking"},
				}
				chunks <- chattest.AnthropicChunk{
					Type:  "content_block_delta",
					Index: 0,
					Delta: chattest.AnthropicDeltaBlock{Type: "thinking_delta", Thinking: "interrupted thought"},
				}
				select {
				case <-reasoningStarted:
				default:
					close(reasoningStarted)
				}
				<-req.Context().Done()
			}()
			return chattest.AnthropicResponse{StreamingChunks: chunks}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		model = updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
					SendReasoning: &sendReasoning,
					Thinking:      &codersdk.ChatModelAnthropicThinkingOptions{BudgetTokens: &thinkingBudget},
				},
			},
		})

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "think")
		testutil.TryReceive(ctx, t, reasoningStarted)
		queued, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue after reasoning")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorInterrupt,
		})
		require.NoError(t, err)
		require.True(t, queued.Queued)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		messages := chatMessages(ctx, t, db, chat.ID)
		var reasoningParts []codersdk.ChatMessagePart
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleAssistant {
				continue
			}
			reasoningParts = append(reasoningParts, reasoningPartsFromMessage(t, msg)...)
		}
		require.Len(t, reasoningParts, 1)
		require.Equal(t, "interrupted thought", strings.TrimSpace(reasoningParts[0].Text))
		require.NotNil(t, reasoningParts[0].CreatedAt)
		require.NotNil(t, reasoningParts[0].CompletedAt)
		require.False(t, reasoningParts[0].CreatedAt.IsZero())
		require.False(t, reasoningParts[0].CompletedAt.IsZero())
		require.False(t, reasoningParts[0].CompletedAt.Before(*reasoningParts[0].CreatedAt))
	})
}

func TestActiveServer_DynamicToolsAndStopAfterToolBehavior(t *testing.T) {
	t.Parallel()

	t.Run("dynamic tool enters requires action", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			streamedCallCount.Add(1)
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("my_dynamic_tool", `{"query":"test"}`),
			)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		dynamicToolsJSON := dynamicToolJSON(t, "my_dynamic_tool")

		factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "dynamic-tool-requires-action",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("call the dynamic tool"),
			},
			DynamicTools: dynamicToolsJSON,
		})
		require.NoError(t, err)

		var chatResult database.Chat
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			got, getErr := db.GetChatByID(ctx, chat.ID)
			if getErr != nil {
				return false
			}
			chatResult = got
			return got.Status == database.ChatStatusRequiresAction || got.Status == database.ChatStatusError
		}, testutil.IntervalFast)
		require.Equal(t, database.ChatStatusRequiresAction, chatResult.Status,
			"expected requires_action, got %s (last_error=%q)",
			chatResult.Status, chatLastErrorMessage(chatResult.LastError))
		require.True(t, chatResult.RequiresActionDeadlineAt.Valid)
		require.Equal(t, int32(1), streamedCallCount.Load())

		parts := chatToolParts(ctx, t, db, chat.ID)
		call := requireToolCallPart(t, parts, "my_dynamic_tool")
		require.JSONEq(t, `{"query":"test"}`, string(call.Args))
		require.False(t, toolResultPartExists(parts, "my_dynamic_tool"),
			"dynamic tool should wait for submitted results")
	})

	t.Run("successful stop after tool finishes turn", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("propose_plan", `{}`),
				)
			default:
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("should not continue")...)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
		factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
		server := newWorkspaceToolTestServer(t, db, ps, dbAgent.ID, "# Plan\n", func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		})

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "stop-after-success",
			ModelConfigID:  model.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("propose a plan"),
			},
		})
		require.NoError(t, err)
		chatResult := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chatResult.WorkerID.Valid)
		require.False(t, chatResult.RunnerID.Valid)
		require.Equal(t, int32(1), streamedCallCount.Load(),
			"stop after tool should finish without another assistant call")

		result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), "propose_plan")
		require.False(t, result.IsError,
			"stop after tool should be based on a successful tool result")
	})

	t.Run("error stop after tool continues generation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("propose_plan", `{"path":"/tmp/not-plan.txt"}`),
				)
			default:
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("tool failed, continue")...)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
		factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
		server := newWorkspaceToolTestServer(t, db, ps, dbAgent.ID, "# Plan\n", func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		})

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "stop-after-error",
			ModelConfigID:  model.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("propose a plan with a bad path"),
			},
		})
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.Equal(t, int32(2), streamedCallCount.Load(),
			"error stop after tool result should not finish the turn by itself")

		parts := chatToolParts(ctx, t, db, chat.ID)
		result := requireToolResultPart(t, parts, "propose_plan")
		require.True(t, result.IsError)
		messages := chatMessages(ctx, t, db, chat.ID)
		requireTextPart(t, messages[len(messages)-1], "tool failed, continue")
	})
}

func TestDynamicToolCallPausesAndResumes(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Track streaming calls to the mock LLM.
	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([]chattest.OpenAIRequest, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		// Non-streaming requests are title generation. Return a
		// simple title.
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Dynamic tool test")
		}

		// Capture the full request for later assertions.
		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, chattest.OpenAIRequest{
			Messages: append([]chattest.OpenAIMessage(nil), req.Messages...),
			Tools:    append([]chattest.OpenAITool(nil), req.Tools...),
			Stream:   req.Stream,
		})
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			// First call: the LLM invokes our dynamic tool.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"my_dynamic_tool",
					`{"input":"hello world"}`,
				),
			)
		}
		// Second call: the LLM returns a normal text response.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Dynamic tool result received.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	// Dynamic tools do not need a workspace connection, but the
	// chatd server always builds workspace tools. Use an active
	// server without an agent connection, so the built-in tools
	// are never invoked because the only tool call targets our
	// dynamic tool.
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	// Create a chat with a dynamic tool.
	dynamicToolsJSON, err := json.Marshal([]mcpgo.Tool{{
		Name:        "my_dynamic_tool",
		Description: "A test dynamic tool.",
		InputSchema: mcpgo.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"input": map[string]any{"type": "string"},
			},
			Required: []string{"input"},
		},
	}})
	require.NoError(t, err)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "dynamic-tool-pause-resume",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Please call the dynamic tool."),
		},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)

	// 1. Wait for the chat to reach requires_action status.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusRequiresAction ||
			got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	require.Equal(t, database.ChatStatusRequiresAction, chatResult.Status,
		"expected requires_action, got %s (last_error=%q)",
		chatResult.Status, chatLastErrorMessage(chatResult.LastError))

	// 2. Read the assistant message to find the tool-call ID.
	var toolCallID string
	var toolCallFound bool
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleAssistant {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil {
				continue
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName == "my_dynamic_tool" {
					toolCallID = part.ToolCallID
					toolCallFound = true
					return true
				}
			}
		}
		return false
	}, testutil.IntervalFast)
	require.True(t, toolCallFound, "expected to find tool call for my_dynamic_tool")
	require.NotEmpty(t, toolCallID)

	// 3. Submit tool results via SubmitToolResults.
	toolResultOutput := json.RawMessage(`{"result":"dynamic tool output"}`)
	err = server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: chatResult.LastModelConfigID,
		Results: []codersdk.ToolResult{{
			ToolCallID: toolCallID,
			Output:     toolResultOutput,
		}},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)

	// 4. Wait for the chat to reach a terminal status.
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	// 5. Verify the chat completed successfully.
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// 6. Verify the mock received exactly 2 streaming calls.
	require.Equal(t, int32(2), streamedCallCount.Load(),
		"expected exactly 2 streaming calls to the LLM")

	streamedCallsMu.Lock()
	recordedCalls := append([]chattest.OpenAIRequest(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.Len(t, recordedCalls, 2)

	// 7. Verify the dynamic tool appeared in the first call's tool list.
	var foundDynamicTool bool
	for _, tool := range recordedCalls[0].Tools {
		if tool.Function.Name == "my_dynamic_tool" {
			foundDynamicTool = true
			break
		}
	}
	require.True(t, foundDynamicTool,
		"expected 'my_dynamic_tool' in the first LLM call's tool list")

	// 8. Verify the second call's messages contain the tool result.
	var foundToolResultInSecondCall bool
	for _, message := range recordedCalls[1].Messages {
		if message.Role != "tool" {
			continue
		}
		if strings.Contains(message.Content, "dynamic tool output") {
			foundToolResultInSecondCall = true
			break
		}
	}
	require.True(t, foundToolResultInSecondCall,
		"expected second LLM call to include the submitted dynamic tool result")
}

func TestDynamicToolNamedProposePlanRemainsAvailableOutsidePlanMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var streamedCallsMu sync.Mutex
	streamedCalls := make([]chattest.OpenAIRequest, 0, 1)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Dynamic tool collision test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, chattest.OpenAIRequest{
			Messages: append([]chattest.OpenAIMessage(nil), req.Messages...),
			Tools:    append([]chattest.OpenAITool(nil), req.Tools...),
			Stream:   req.Stream,
		})
		streamedCallsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Dynamic tool list captured.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	dynamicToolsJSON, err := json.Marshal([]mcpgo.Tool{{
		Name:        "propose_plan",
		Description: "A dynamic tool whose name collides with the hidden built-in.",
		InputSchema: mcpgo.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"input": map[string]any{"type": "string"},
			},
			Required: []string{"input"},
		},
	}})
	require.NoError(t, err)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "dynamic-propose-plan-collision",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("List the available tools."),
		},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)

	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	streamedCallsMu.Lock()
	recordedCalls := append([]chattest.OpenAIRequest(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.NotEmpty(t, recordedCalls)

	var foundDynamicTool bool
	for _, tool := range recordedCalls[0].Tools {
		if tool.Function.Name == "propose_plan" {
			foundDynamicTool = true
			break
		}
	}
	require.True(t, foundDynamicTool,
		"expected the dynamic propose_plan tool to remain visible outside plan mode")
}

func TestDynamicToolCallMixedWithBuiltIn(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Track streaming calls to the mock LLM.
	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([]chattest.OpenAIRequest, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Mixed tool test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, chattest.OpenAIRequest{
			Messages: append([]chattest.OpenAIMessage(nil), req.Messages...),
			Tools:    append([]chattest.OpenAITool(nil), req.Tools...),
			Stream:   req.Stream,
		})
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			// First call: return TWO tool calls in one
			// response: a built-in tool (read_file) and a
			// dynamic tool (my_dynamic_tool).
			builtinChunk := chattest.OpenAIToolCallChunk(
				"read_file",
				`{"path":"/tmp/test.txt"}`,
			)
			dynamicChunk := chattest.OpenAIToolCallChunk(
				"my_dynamic_tool",
				`{"input":"hello world"}`,
			)
			// Merge both tool calls into one chunk with
			// separate indices so the LLM appears to have
			// requested both tools simultaneously.
			mergedChunk := builtinChunk
			dynCall := dynamicChunk.Choices[0].ToolCalls[0]
			dynCall.Index = 1
			mergedChunk.Choices[0].ToolCalls = append(
				mergedChunk.Choices[0].ToolCalls,
				dynCall,
			)
			return chattest.OpenAIStreamingResponse(mergedChunk)
		}
		// Second call (after tool results): normal text
		// response.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("All done.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	// Create a chat with a dynamic tool.
	dynamicToolsJSON, err := json.Marshal([]mcpgo.Tool{{
		Name:        "my_dynamic_tool",
		Description: "A test dynamic tool.",
		InputSchema: mcpgo.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"input": map[string]any{"type": "string"},
			},
			Required: []string{"input"},
		},
	}})
	require.NoError(t, err)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "mixed-builtin-dynamic",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Call both tools."),
		},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)

	// 1. Wait for the chat to reach requires_action status.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusRequiresAction ||
			got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	require.Equal(t, database.ChatStatusRequiresAction, chatResult.Status,
		"expected requires_action, got %s (last_error=%q)",
		chatResult.Status, chatLastErrorMessage(chatResult.LastError))

	// 2. Verify the built-in tool (read_file) was already
	//    executed by checking that a tool result message
	//    exists for it in the database.
	var builtinToolResultFound bool
	var toolCallID string
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil {
				continue
			}
			for _, part := range parts {
				// Check for the built-in tool result.
				if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == "read_file" {
					builtinToolResultFound = true
				}
				// Find the dynamic tool call ID.
				if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName == "my_dynamic_tool" {
					toolCallID = part.ToolCallID
				}
			}
		}
		return builtinToolResultFound && toolCallID != ""
	}, testutil.IntervalFast)

	require.True(t, builtinToolResultFound,
		"expected read_file tool result in the DB before dynamic tool resolution")
	require.NotEmpty(t, toolCallID)

	// 3. Submit dynamic tool results.
	err = server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: chatResult.LastModelConfigID,
		Results: []codersdk.ToolResult{{
			ToolCallID: toolCallID,
			Output:     json.RawMessage(`{"result":"dynamic output"}`),
		}},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)

	// 4. Wait for the chat to complete.
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// 5. Verify the LLM received exactly 2 streaming calls.
	require.Equal(t, int32(2), streamedCallCount.Load(),
		"expected exactly 2 streaming calls to the LLM")
}

func TestSubmitToolResultsConcurrency(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// The mock LLM returns a dynamic tool call on the first streaming
	// request, then a plain text reply on the second.
	var streamedCallCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Concurrency test")
		}
		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"my_dynamic_tool",
					`{"input":"hello"}`,
				),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Done.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
	})

	// Create a chat with a dynamic tool.
	dynamicToolsJSON, err := json.Marshal([]mcpgo.Tool{{
		Name:        "my_dynamic_tool",
		Description: "A test dynamic tool.",
		InputSchema: mcpgo.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"input": map[string]any{"type": "string"},
			},
			Required: []string{"input"},
		},
	}})
	require.NoError(t, err)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "concurrency-tool-results",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Please call the dynamic tool."),
		},
		DynamicTools: dynamicToolsJSON,
	})
	require.NoError(t, err)

	// Wait for the chat to reach requires_action status.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusRequiresAction ||
			got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)
	require.Equal(t, database.ChatStatusRequiresAction, chatResult.Status,
		"expected requires_action, got %s (last_error=%q)",
		chatResult.Status, chatLastErrorMessage(chatResult.LastError))

	// Find the tool call ID from the assistant message.
	var toolCallID string
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleAssistant {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil {
				continue
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName == "my_dynamic_tool" {
					toolCallID = part.ToolCallID
					return true
				}
			}
		}
		return false
	}, testutil.IntervalFast)
	require.NotEmpty(t, toolCallID)

	// Spawn N goroutines that all try to submit tool results at the
	// same time. Exactly one should succeed; the rest must get a
	// ToolResultStatusConflictError.
	const numGoroutines = 10
	var (
		wg               sync.WaitGroup
		ready            = make(chan struct{})
		successes        atomic.Int32
		conflicts        atomic.Int32
		unexpectedErrors = make(chan error, numGoroutines)
	)

	for range numGoroutines {
		wg.Go(func() {
			// Wait for all goroutines to be ready.
			<-ready

			submitErr := server.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
				ChatID:        chat.ID,
				UserID:        user.ID,
				ModelConfigID: chatResult.LastModelConfigID,
				Results: []codersdk.ToolResult{{
					ToolCallID: toolCallID,
					Output:     json.RawMessage(`{"result":"concurrent output"}`),
				}},
				DynamicTools: dynamicToolsJSON,
			})

			if submitErr == nil {
				successes.Add(1)
				return
			}
			var conflict *chatd.ToolResultStatusConflictError
			if errors.As(submitErr, &conflict) {
				conflicts.Add(1)
				return
			}
			// Collect unexpected errors for assertion
			// outside the goroutine (require.NoError
			// calls t.FailNow which is illegal here).
			unexpectedErrors <- submitErr
		})
	}
	// Release all goroutines at once.
	close(ready)

	wg.Wait()
	close(unexpectedErrors)

	for ue := range unexpectedErrors {
		require.NoError(t, ue, "unexpected error from SubmitToolResults")
	}

	require.Equal(t, int32(1), successes.Load(),
		"expected exactly 1 goroutine to succeed")
	require.Equal(t, int32(numGoroutines-1), conflicts.Load(),
		"expected %d conflict errors", numGoroutines-1)
}

func ptrRef[T any](v T) *T {
	return &v
}

func TestSubscribeNoDuplicateMessageParts(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "no-dup-parts",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	snapshot, events, cancel, ok := replica.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Snapshot should have events (at minimum: status + message).
	require.NotEmpty(t, snapshot)

	// The events channel should NOT immediately produce any
	// events. The snapshot already contained everything. Before
	// the fix, localSnapshot was replayed into the channel,
	// causing duplicates.
	require.Never(t, func() bool {
		select {
		case <-events:
			return true
		default:
			return false
		}
	}, 200*time.Millisecond, testutil.IntervalFast,
		"expected no duplicate events after snapshot")
}

func TestSubscribeAfterMessageID(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "after-id-test",
		Status:            database.ChatStatusWaiting,
	})

	// Seed all messages directly so this subscription test is independent
	// of chat processing lifecycle behavior.
	firstContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("first"),
	})
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         chat.ID,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:           database.ChatMessageRoleUser,
		ContentVersion: chatprompt.CurrentContentVersion,
		Content:        firstContent,
	})

	secondContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("second"),
	})
	require.NoError(t, err)

	msg2 := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         chat.ID,
		ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:           database.ChatMessageRoleAssistant,
		ContentVersion: chatprompt.CurrentContentVersion,
		Content:        secondContent,
	})

	thirdContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("third"),
	})
	require.NoError(t, err)

	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         chat.ID,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		ModelConfigID:  uuid.NullUUID{UUID: model.ID, Valid: true},
		Role:           database.ChatMessageRoleUser,
		ContentVersion: chatprompt.CurrentContentVersion,
		Content:        thirdContent,
	})

	// Control: Subscribe with afterMessageID=0 returns ALL messages.
	allSnapshot, _, cancelAll, ok := replica.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	cancelAll()

	allMessages := filterMessageEvents(allSnapshot)
	require.Len(t, allMessages, 3, "afterMessageID=0 should return all three messages")

	// Subscribe with afterMessageID set to the second message's ID.
	// Only the third message (inserted after msg2) should appear.
	partialSnapshot, _, cancelPartial, ok := replica.Subscribe(ctx, chat.ID, nil, msg2.ID)
	require.True(t, ok)
	cancelPartial()

	partialMessages := filterMessageEvents(partialSnapshot)
	require.Len(t, partialMessages, 1, "afterMessageID=msg2.ID should return only messages after msg2")
	require.Equal(t, codersdk.ChatMessageRoleUser, partialMessages[0].Message.Role)
}

// filterMessageEvents returns only the Message-type events from a
// snapshot slice, which is useful for ignoring status / queue events.
func filterMessageEvents(events []codersdk.ChatStreamEvent) []codersdk.ChatStreamEvent {
	return slice.Filter(events, func(e codersdk.ChatStreamEvent) bool {
		return e.Type == codersdk.ChatStreamEventTypeMessage
	})
}

func TestCreateWorkspaceTool_EndToEnd(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	// Add a startup script so the agent spends time in the
	// "starting" lifecycle state. This lets us verify that
	// create_workspace waits for scripts to finish.
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken, func(g *proto.GraphComplete) {
			g.Resources[0].Agents[0].Scripts = []*proto.Script{{
				DisplayName: "setup",
				Script:      "sleep 5",
				RunOnStart:  true,
			}}
		}),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	// Start the test workspace agent so create_workspace can wait for
	// the agent to become reachable before returning.
	_ = agenttest.New(t, client.URL, agentToken)

	workspaceName := "chat-ws-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	createWorkspaceArgs := fmt.Sprintf(
		`{"template_id":%q,"name":%q}`,
		template.ID.String(),
		workspaceName,
	)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Create workspace test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("create_workspace", createWorkspaceArgs),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Workspace created and ready.")...,
		)
	})

	coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, openAIURL)

	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Create a workspace from the template and continue.",
			},
		},
	})
	require.NoError(t, err)

	var chatResult codersdk.Chat
	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == codersdk.ChatStatusWaiting || got.Status == codersdk.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == codersdk.ChatStatusError {
		lastError := ""
		if chatResult.LastError != nil {
			lastError = chatResult.LastError.Message
		}
		require.FailNowf(t, "chat run failed", "last_error=%q", lastError)
	}

	require.NotNil(t, chatResult.WorkspaceID)
	workspaceID := *chatResult.WorkspaceID
	workspace, err := client.Workspace(ctx, workspaceID)
	require.NoError(t, err)
	require.Equal(t, workspaceName, workspace.Name)

	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)

	var foundCreateWorkspaceResult bool
	for _, message := range chatMsgs.Messages {
		if message.Role != codersdk.ChatMessageRoleTool {
			continue
		}
		for _, part := range message.Content {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != "create_workspace" {
				continue
			}
			var result map[string]any
			require.NoError(t, json.Unmarshal(part.Result, &result))
			created, ok := result["created"].(bool)
			require.True(t, ok)
			require.True(t, created)
			foundCreateWorkspaceResult = true
		}
	}
	require.True(t, foundCreateWorkspaceResult, "expected create_workspace tool result message")

	// Verify that the tool waited for startup scripts to
	// complete. The agent should be in "ready" state by the
	// time create_workspace returns its result.
	workspace, err = client.Workspace(ctx, workspaceID)
	require.NoError(t, err)
	var agentLifecycle codersdk.WorkspaceAgentLifecycle
	for _, res := range workspace.LatestBuild.Resources {
		for _, agt := range res.Agents {
			agentLifecycle = agt.LifecycleState
		}
	}
	require.Equal(t, codersdk.WorkspaceAgentLifecycleReady, agentLifecycle,
		"agent should be ready after create_workspace returns; startup scripts were not awaited")

	require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	streamedCallsMu.Lock()
	recordedStreamCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.GreaterOrEqual(t, len(recordedStreamCalls), 2)

	var foundToolResultInSecondCall bool
	for _, message := range recordedStreamCalls[1] {
		if message.Role != "tool" {
			continue
		}
		if !json.Valid([]byte(message.Content)) {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		created, ok := result["created"].(bool)
		if ok && created {
			foundToolResultInSecondCall = true
			break
		}
	}
	require.True(t, foundToolResultInSecondCall, "expected second streamed model call to include create_workspace tool output")
}

func TestStartWorkspaceTool_EndToEnd(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

	// Create a workspace, then stop it so start_workspace has
	// something to start. We intentionally skip starting a test
	// agent. The echo provisioner creates new agent rows for each
	// build, so an agent started for build 1 cannot serve build 3.
	// The tool handles the no-agent case gracefully.
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
	workspace = coderdtest.MustTransitionWorkspace(
		t, client, workspace.ID,
		codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop,
	)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Start workspace test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("start_workspace", "{}"),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Workspace started and ready.")...,
		)
	})

	coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, openAIURL)

	// Create a chat with the stopped workspace pre-associated.
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Start the workspace.",
			},
		},
		WorkspaceID: &workspace.ID,
	})
	require.NoError(t, err)

	var chatResult codersdk.Chat
	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == codersdk.ChatStatusWaiting || got.Status == codersdk.ChatStatusError
	}, testutil.WaitSuperLong, testutil.IntervalFast)

	if chatResult.Status == codersdk.ChatStatusError {
		lastError := ""
		if chatResult.LastError != nil {
			lastError = chatResult.LastError.Message
		}
		require.FailNowf(t, "chat run failed", "last_error=%q", lastError)
	}

	// Verify the workspace was started.
	require.NotNil(t, chatResult.WorkspaceID)
	updatedWorkspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceTransitionStart, updatedWorkspace.LatestBuild.Transition)

	chatMsgs, err := expClient.GetChatMessages(ctx, chat.ID, nil)
	require.NoError(t, err)

	// Verify start_workspace tool result exists in the chat messages.
	var foundStartWorkspaceResult bool
	for _, message := range chatMsgs.Messages {
		if message.Role != codersdk.ChatMessageRoleTool {
			continue
		}
		for _, part := range message.Content {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.ToolName != "start_workspace" {
				continue
			}
			var result map[string]any
			require.NoError(t, json.Unmarshal(part.Result, &result))
			started, ok := result["started"].(bool)
			require.True(t, ok)
			require.True(t, started)
			foundStartWorkspaceResult = true
		}
	}
	require.True(t, foundStartWorkspaceResult, "expected start_workspace tool result message")

	// Verify the LLM received the tool result in its second call.
	require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	streamedCallsMu.Lock()
	recordedStreamCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.GreaterOrEqual(t, len(recordedStreamCalls), 2)

	var foundToolResultInSecondCall bool
	for _, message := range recordedStreamCalls[1] {
		if message.Role != "tool" {
			continue
		}
		if !json.Valid([]byte(message.Content)) {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		started, ok := result["started"].(bool)
		if ok && started {
			foundToolResultInSecondCall = true
			break
		}
	}
	require.True(t, foundToolResultInSecondCall, "expected second streamed model call to include start_workspace tool output")
}

func TestStoppedWorkspaceWithPersistedAgentBindingDoesNotBlockChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)
	toolsByCall := make([][]string, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Stopped workspace regression")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		toolsByCall = append(toolsByCall, names)
		streamedCallsMu.Unlock()

		if streamedCallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("execute", `{"command":"echo hi"}`),
			)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("The workspace is unavailable. Start it before retrying workspace tools.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	inactive := newTestServer(t, db, ps, uuid.New())
	chat, err := inactive.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "stopped-workspace-regression",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Run echo hi in the workspace."),
		},
	})
	require.NoError(t, err)

	// Close the inactive server. The chat remains in the valid
	// state-machine `running` state created by CreateChat, and the
	// active server created below can acquire it because it is unowned.
	require.NoError(t, inactive.Close())

	build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, ws.ID)
	require.NoError(t, err)
	chat, err = db.UpdateChatBuildAgentBinding(ctx, database.UpdateChatBuildAgentBindingParams{
		ID:      chat.ID,
		BuildID: uuid.NullUUID{UUID: build.ID, Valid: true},
		AgentID: uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
	})
	require.NoError(t, err)

	dbfake.WorkspaceBuild(t, db, ws).Seed(database.WorkspaceBuild{
		Transition:  database.WorkspaceTransitionStop,
		BuildNumber: 2,
	}).Do()

	var dialCalls atomic.Int32
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.AgentConn = func(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			dialCalls.Add(1)
			require.Equal(t, dbAgent.ID, agentID)
			<-ctx.Done()
			return nil, nil, ctx.Err()
		}
	})

	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	require.EqualValues(t, 1, dialCalls.Load())
	require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))

	streamedCallsMu.Lock()
	recordedCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	recordedTools := append([][]string(nil), toolsByCall...)
	streamedCallsMu.Unlock()
	require.GreaterOrEqual(t, len(recordedCalls), 2)
	require.NotEmpty(t, recordedTools)
	require.Contains(t, recordedTools[0], "execute")
	require.Contains(t, recordedTools[0], "start_workspace")

	var foundUnavailableToolResult bool
	for _, message := range recordedCalls[1] {
		if message.Role != "tool" {
			continue
		}
		if strings.Contains(message.Content, "workspace has no running agent") {
			foundUnavailableToolResult = true
			break
		}
		if !json.Valid([]byte(message.Content)) {
			continue
		}
		var toolResult map[string]any
		if err := json.Unmarshal([]byte(message.Content), &toolResult); err != nil {
			continue
		}
		errMsg, _ := toolResult["error"].(string)
		outputMsg, _ := toolResult["output"].(string)
		if strings.Contains(errMsg, "workspace has no running agent") ||
			strings.Contains(outputMsg, "workspace has no running agent") {
			foundUnavailableToolResult = true
			break
		}
	}
	require.True(t, foundUnavailableToolResult,
		"expected the second streamed model call to include the unavailable workspace tool result")

	var toolMessage *database.ChatMessage
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for i := range messages {
			if messages[i].Role == database.ChatMessageRoleTool {
				toolMessage = &messages[i]
				return true
			}
		}
		return false
	}, testutil.IntervalFast)
	require.NotNil(t, toolMessage)

	parts, err := chatprompt.ParseContent(*toolMessage)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[0].Type)
	require.Equal(t, "execute", parts[0].ToolName)
	require.True(t, parts[0].IsError)
	require.Contains(t, string(parts[0].Result), "workspace has no running agent")
}

func TestHeartbeatNoWorkspaceNoBump(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}
		chunks := make(chan chattest.OpenAIChunk)
		go func() {
			defer close(chunks)
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	}))

	// Set up UsageTracker with manual tick/flush.
	usageTickCh := make(chan time.Time)
	flushCh := make(chan int, 1)
	tracker := workspacestats.NewTracker(db,
		workspacestats.TrackerWithTickFlush(usageTickCh, flushCh),
		workspacestats.TrackerWithLogger(slogtest.Make(t, nil)),
	)
	t.Cleanup(func() { tracker.Close() })

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
		ChatHeartbeatInterval:      100 * time.Millisecond,
	})
	server.Start()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// Create a chat WITHOUT linking a workspace.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "no-workspace-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the chat to be acquired and at least one runner
	// heartbeat to be written.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, listErr := db.GetChatByID(ctx, chat.ID)
		if listErr != nil || fromDB.Status != database.ChatStatusRunning || !fromDB.RunnerID.Valid {
			return false
		}
		heartbeat, heartbeatErr := db.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
			ChatID:   chat.ID,
			RunnerID: fromDB.RunnerID.UUID,
		})
		if heartbeatErr != nil {
			return false
		}
		return heartbeat.HeartbeatAt.After(fromDB.CreatedAt)
	}, testutil.IntervalFast,
		"chat should be running with at least one heartbeat")

	// Flush the tracker. Since no workspace was linked, count
	// should be 0.
	testutil.RequireSend(ctx, t, usageTickCh, time.Now())
	count := testutil.RequireReceive(ctx, t, flushCh)
	require.Equal(t, 0, count, "expected no workspaces to be flushed when chat has no workspace")
}

// waitForChatProcessed waits for the chat worker to fully handle a
// wake for the given chat. It polls until the chat leaves the running
// state (the worker has finished the turn and updated the DB), then
// calls WaitUntilIdleForTest so tracked background work settles.
func waitForChatProcessed(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	server *chatd.Server,
) {
	t.Helper()
	require.Eventually(t, func() bool {
		c, err := db.GetChatByID(ctx, chatID)
		if err != nil {
			return false
		}
		// Wait until the chat reaches a settled state (not being
		// processed). This guarantees the wake was picked up and its
		// background work registered before WaitUntilIdleForTest
		// checks for idleness.
		return c.Status != database.ChatStatusRunning
	}, testutil.WaitShort, testutil.IntervalFast)
	chatd.WaitUntilIdleForTest(server)
}

// newTestServer creates a passive server whose periodic chat
// acquisition is effectively disabled, so chats are only processed in
// response to explicit wakes.
func newTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	replicaID uuid.UUID,
	overrides ...func(*chatd.Config),
) *chatd.Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	cfg := chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  replicaID,
		PendingChatAcquireInterval: testutil.WaitLong,
		Experiments:                codersdk.ExperimentsKnown,
	}
	for _, o := range overrides {
		o(&cfg)
	}
	server := chatd.New(ps, cfg)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

func highUsageTextResponse(text string) chattest.AnthropicResponse {
	return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
		InputTokens:  80,
		OutputTokens: 5,
	}, text)...)
}

func anthropicCompactionResponse(text string) chattest.AnthropicResponse {
	return chattest.AnthropicResponse{Response: &chattest.AnthropicMessage{
		ID:         "msg-compaction",
		Type:       "message",
		Role:       "assistant",
		Content:    text,
		Model:      "claude-3-opus-20240229",
		StopReason: "end_turn",
	}}
}

func highUsageReadFileResponse(path string) chattest.AnthropicResponse {
	chunks := chattest.AnthropicToolCallChunks("read_file", fmt.Sprintf(`{"path":%q}`, path))
	for i := range chunks {
		if chunks[i].Type == "message_start" {
			chunks[i].Message.Usage = map[string]int{"input_tokens": 80}
		}
		if chunks[i].Type == "message_delta" {
			chunks[i].UsageMap = map[string]int{"output_tokens": 5}
		}
	}
	return chattest.AnthropicStreamingResponse(chunks...)
}

func TestActiveServer_RoutingPreservesAPIKeyAfterCompaction(t *testing.T) {
	t.Parallel()

	const (
		compactionSummary = "summary text for AI Gateway compaction"
		contextLimit      = int64(100)
		thresholdPercent  = int32(70)
	)

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var streamCount atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		body := anthropicRequestBody(t, *req)
		if !req.Stream {
			if strings.Contains(body, "You are performing a context compaction") {
				return chattest.AnthropicNonStreamingResponse(compactionSummary)
			}
			return chattest.AnthropicNonStreamingResponse("AI Gateway Compaction")
		}

		switch streamCount.Add(1) {
		case 1:
			return highUsageReadFileResponse("/tmp/a.txt")
		case 2:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
				InputTokens:  20,
				OutputTokens: 5,
			}, "continued after compaction")...)
		default:
			t.Fatalf("unexpected streamed model call %d", streamCount.Load())
			return chattest.AnthropicStreamingResponse()
		}
	})
	factory := chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath())
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCompressionThreshold(t, db, model, contextLimit, thresholdPercent)
	provider, err := db.GetAIProviderByID(ctx, model.AIProviderID.UUID)
	require.NoError(t, err)
	_, err = db.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
		ID:           uuid.New(),
		UserID:       user.ID,
		AIProviderID: provider.ID,
		APIKey:       "sk-user-aibridge",
	})
	require.NoError(t, err)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main"}, nil).
		Times(1)

	creator := newTestServer(t, db, ps, uuid.New())
	chat, err := creator.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "aigateway-compaction",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("trigger compaction"),
		},
	})
	require.NoError(t, err)
	contextContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:                 codersdk.ChatMessagePartTypeContextFile,
		ContextFileAgentID:   uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		ContextFilePath:      "/home/coder/project/AGENTS.md",
		ContextFileContent:   "# Project instructions",
		ContextFileOS:        "linux",
		ContextFileDirectory: "/home/coder/project",
	}})
	require.NoError(t, err)
	_, err = db.InsertChatMessages(ctx, chatd.BuildSingleChatMessageInsertParams(
		chat.ID,
		database.ChatMessageRoleUser,
		contextContent,
		database.ChatMessageVisibilityBoth,
		model.ID,
		chatprompt.CurrentContentVersion,
		user.ID,
	))
	require.NoError(t, err)

	_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.AllowBYOK = true
		cfg.AllowBYOKSet = true
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)
	require.False(t, chatResult.LastError.Valid)
	gatewayKey, err := db.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
		UserID:    user.ID,
		TokenName: chatd.GatewayTokenName(user.ID),
	})
	require.NoError(t, err)

	messages := chatMessages(ctx, t, db, chat.ID)
	promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	compressed := compressedChatSummarizedMessages(t, append(promptMessages, messages...))
	require.Len(t, compressed.summaries, 1)

	requests := factory.RequestsSnapshot()
	require.NotEmpty(t, requests)
	for _, req := range requests {
		require.Equal(t, provider.Name, req.ProviderName)
		require.Equal(t, aibridge.SourceAgents, req.Source)
		require.Equal(t, gatewayKey.ID, req.APIKeyID)
		require.Equal(t, "sk-user-aibridge", req.Request.Header.Get("X-Api-Key"))
		require.Equal(t, "delegated", req.Request.Header.Get(aibridge.HeaderCoderToken))
	}
}

func TestActiveServer_CompactionRecordsMetric(t *testing.T) {
	t.Parallel()

	const (
		compactionSummary = "summary text for compaction"
		contextLimit      = int64(100)
		thresholdPercent  = int32(70)
	)

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	reg := prometheus.NewRegistry()
	var streamCount atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		body := anthropicRequestBody(t, *req)
		if !req.Stream {
			if strings.Contains(body, "You are performing a context compaction") {
				return anthropicCompactionResponse(compactionSummary)
			}
			return chattest.AnthropicNonStreamingResponse("title")
		}
		switch streamCount.Add(1) {
		case 1:
			return highUsageReadFileResponse("/tmp/a.txt")
		case 2:
			require.Contains(t, body, compactionSummary)
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
				InputTokens:  20,
				OutputTokens: 5,
			}, "continued after compaction")...)
		default:
			t.Fatalf("unexpected generation request: %s", body)
			return chattest.AnthropicStreamingResponse()
		}
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCompressionThreshold(t, db, model, contextLimit, thresholdPercent)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main"}, nil).
		Times(1)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.PrometheusRegistry = reg
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "compaction-metric",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the file and continue"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	requireChatdMetricCounter(t, reg, "coderd_chatd_compaction_total", 1, map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-20250514",
		"result":   "success",
	})
}

func TestActiveServer_Compaction(t *testing.T) {
	t.Parallel()

	const (
		compactionSummary = "summary text for compaction"
		contextLimit      = int64(100)
		thresholdPercent  = int32(70)
	)

	newHighUsageReadFileResponse := func(path string) chattest.AnthropicResponse {
		chunks := chattest.AnthropicToolCallChunks("read_file", fmt.Sprintf(`{"path":%q}`, path))
		for i := range chunks {
			if chunks[i].Type == "message_start" {
				chunks[i].Message.Usage = map[string]int{"input_tokens": 80}
			}
			if chunks[i].Type == "message_delta" {
				chunks[i].UsageMap = map[string]int{"output_tokens": 5}
			}
		}
		return chattest.AnthropicStreamingResponse(chunks...)
	}

	t.Run("commits summary when threshold reached and continues from committed summary", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		requests := newAnthropicRequestRecorder()
		var streamCount atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			requests.record(req)
			body := anthropicRequestBody(t, *req)
			if !req.Stream {
				if strings.Contains(body, "You are performing a context compaction") {
					require.Contains(t, body, "read_file")
					require.Contains(t, body, "package main")
					return anthropicCompactionResponse(compactionSummary)
				}
				return chattest.AnthropicNonStreamingResponse("title")
			}
			switch streamCount.Add(1) {
			case 1:
				return newHighUsageReadFileResponse("/tmp/a.txt")
			default:
				require.Contains(t, body, compactionSummary)
				require.Contains(t, body, "The following is a summary of the earlier conversation")
				require.Contains(t, body, `"role":"user"`)
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
					InputTokens:  20,
					OutputTokens: 5,
				}, "continued after compaction")...)
			}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		model = updateChatModelCompressionThreshold(t, db, model, contextLimit, thresholdPercent)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1	package main"}, nil).
			Times(1)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "compaction-continues",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("read the file and continue"),
			},
		})
		require.NoError(t, err)
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.WorkerID.Valid)
		require.False(t, chat.RunnerID.Valid)

		generationRequests := filterAnthropicStreamingRequests(requests.all())
		require.GreaterOrEqual(t, len(generationRequests), 2)
		require.Equal(t, int32(2), streamCount.Load())

		messages := chatMessages(ctx, t, db, chat.ID)
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		compressed := compressedChatSummarizedMessages(t, append(promptMessages, messages...))
		require.Len(t, compressed.summaries, 1)
		require.Len(t, compressed.calls, 1)
		require.Len(t, compressed.results, 1)

		require.Equal(t, database.ChatMessageRoleUser, compressed.summaries[0].Role)
		require.Equal(t, database.ChatMessageVisibilityModel, compressed.summaries[0].Visibility)
		summaryText := messageText(t, compressed.summaries[0])
		require.Contains(t, summaryText, "The following is a summary of the earlier conversation")
		require.Contains(t, summaryText, compactionSummary)

		callPart := singlePartOfType(t, compressed.calls[0], codersdk.ChatMessagePartTypeToolCall)
		resultPart := singlePartOfType(t, compressed.results[0], codersdk.ChatMessagePartTypeToolResult)
		require.Equal(t, callPart.ToolCallID, resultPart.ToolCallID)
		require.Equal(t, "chat_summarized", resultPart.ToolName)
		require.JSONEq(t, `{"summary":"summary text for compaction","source":"automatic","threshold_percent":70,"usage_percent":80,"context_tokens":80,"context_limit_tokens":100}`, string(resultPart.Result))
		requireTextPart(t, messages[len(messages)-1], "continued after compaction")
	})

	t.Run("does not compact when high usage finishes the turn", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		var compactionRequests atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			body := anthropicRequestBody(t, *req)
			if strings.Contains(body, "You are performing a context compaction") {
				compactionRequests.Add(1)
				return anthropicCompactionResponse(compactionSummary)
			}
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}
			streamCount.Add(1)
			return highUsageTextResponse("done without compaction")
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		model = updateChatModelCompressionThreshold(t, db, model, contextLimit, thresholdPercent)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "finish with high usage")
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		require.Equal(t, int32(1), streamCount.Load())
		require.Equal(t, int32(0), compactionRequests.Load())
		messages := chatMessages(ctx, t, db, chat.ID)
		compressed := compressedChatSummarizedMessages(t, messages)
		require.Empty(t, compressed.summaries)
		require.Empty(t, compressed.calls)
		require.Empty(t, compressed.results)
		for _, msg := range messages {
			require.False(t, msg.Compressed, "message %d should not be compressed", msg.ID)
		}
		requireTextPart(t, messages[len(messages)-1], "done without compaction")
	})

	t.Run("next message fails when compaction continuation stayed over limit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		logSink := testutil.NewFakeSink(t)
		var streamCount atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			body := anthropicRequestBody(t, *req)
			if !req.Stream {
				if strings.Contains(body, "You are performing a context compaction") {
					return anthropicCompactionResponse(compactionSummary)
				}
				return chattest.AnthropicNonStreamingResponse("title")
			}
			switch streamCount.Add(1) {
			case 1:
				return newHighUsageReadFileResponse("/tmp/a.txt")
			default:
				require.Contains(t, body, compactionSummary)
				return highUsageTextResponse("still too large")
			}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		model = updateChatModelCompressionThreshold(t, db, model, contextLimit, thresholdPercent)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1	package main"}, nil).
			Times(1)

		reg := prometheus.NewRegistry()
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			cfg.Logger = logSink.Logger()
			cfg.PrometheusRegistry = reg
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "compaction-next-message-over-limit",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("read the file and stay too large"),
			},
		})
		require.NoError(t, err)
		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.False(t, chat.LastError.Valid)
		require.Equal(t, int32(2), streamCount.Load())
		messages := chatMessages(ctx, t, db, chat.ID)
		requireTextPart(t, messages[len(messages)-1], "still too large")

		_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("continue after the large compacted turn"),
			},
		})
		require.NoError(t, err)

		chat = waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
		require.Equal(t,
			"Conversation compaction could not reduce the history below the configured limit. Raise the compaction limit in settings, or start a new conversation.",
			chatLastErrorMessage(chat.LastError),
		)
		require.Equal(t, int32(2), streamCount.Load(), "over-limit history should fail before another model stream")
		requireChatdMetricCounter(t, reg, "coderd_chatd_compaction_total", 1, map[string]string{
			"provider": "anthropic",
			"model":    "claude-sonnet-4-20250514",
			"result":   "error",
		})

		isCompactionFailureLog := func(e slog.SinkEntry) bool {
			if e.Level != slog.LevelWarn || e.Message != "chat generation failed" {
				return false
			}
			errValue, ok := sinkFieldValue(e.Fields, "error")
			return ok && strings.Contains(fmt.Sprintf("%v", errValue), "compaction left the chat above the compaction limit")
		}
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return len(logSink.Entries(isCompactionFailureLog)) > 0
		}, testutil.IntervalFast)
	})
}

type compressedCompactionMessages struct {
	summaries []database.ChatMessage
	calls     []database.ChatMessage
	results   []database.ChatMessage
}

func compressedChatSummarizedMessages(t *testing.T, messages []database.ChatMessage) compressedCompactionMessages {
	t.Helper()
	seen := map[int64]bool{}
	var out compressedCompactionMessages
	for _, msg := range messages {
		if !msg.Compressed || seen[msg.ID] {
			continue
		}
		seen[msg.ID] = true
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parts {
			switch part.Type {
			case codersdk.ChatMessagePartTypeText:
				if msg.Role == database.ChatMessageRoleUser {
					out.summaries = append(out.summaries, msg)
				}
			case codersdk.ChatMessagePartTypeToolCall:
				if part.ToolName == "chat_summarized" {
					out.calls = append(out.calls, msg)
				}
			case codersdk.ChatMessagePartTypeToolResult:
				if part.ToolName == "chat_summarized" {
					out.results = append(out.results, msg)
				}
			}
		}
	}
	return out
}

func messageText(t *testing.T, msg database.ChatMessage) string {
	t.Helper()
	parts, err := chatprompt.ParseContent(msg)
	require.NoError(t, err)
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			_, _ = builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func singlePartOfType(t *testing.T, msg database.ChatMessage, typ codersdk.ChatMessagePartType) codersdk.ChatMessagePart {
	t.Helper()
	parts, err := chatprompt.ParseContent(msg)
	require.NoError(t, err)
	var matches []codersdk.ChatMessagePart
	for _, part := range parts {
		if part.Type == typ {
			matches = append(matches, part)
		}
	}
	require.Len(t, matches, 1)
	return matches[0]
}

func TestActiveServer_CompactionModelOverride(t *testing.T) {
	t.Parallel()

	const (
		compactionSummary = "summary text for compaction"
		chatModelName     = "claude-sonnet-4-20250514"
		overrideModelName = "claude-3-5-haiku-latest"
		thresholdPercent  = int32(70)
	)

	seedOverrideModel := func(ctx context.Context, t *testing.T, db database.Store, chatModel database.ChatModelConfig, modelName, effort string, contextLimit int64) database.ChatModelConfig {
		t.Helper()
		overrideModel := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Model:        modelName,
			AIProviderID: chatModel.AIProviderID,
			ContextLimit: contextLimit,
		})
		overrideModel = updateChatModelCallConfig(t, db, overrideModel, codersdk.ChatModelCallConfig{
			ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
				Default: &effort,
				Max:     &effort,
			},
		})
		require.NoError(t, db.UpsertChatCompactionModelOverride(ctx, overrideModel.ID.String()))
		return overrideModel
	}

	routingCases := []struct {
		name                 string
		overrideModel        string
		effort               string
		assertSummaryRequest func(t *testing.T, req *chattest.AnthropicRequest)
	}{
		{
			// Claude 3.5 predates extended thinking, so effort sends
			// neither thinking nor output_config.
			name:          "pre-thinking override model",
			overrideModel: overrideModelName,
			effort:        "high",
			assertSummaryRequest: func(t *testing.T, req *chattest.AnthropicRequest) {
				require.Empty(t, string(req.OutputConfig))
				require.Empty(t, string(req.Thinking))
			},
		},
		{
			// 3276 is 0.8 (high) of the summary call's default 4096 max_tokens.
			name:          "legacy budget-thinking override model",
			overrideModel: "claude-haiku-4-5",
			effort:        "high",
			assertSummaryRequest: func(t *testing.T, req *chattest.AnthropicRequest) {
				require.Empty(t, string(req.OutputConfig))
				require.Contains(t, string(req.Thinking), `"type":"enabled"`)
				require.Contains(t, string(req.Thinking), `"budget_tokens":3276`)
			},
		},
		{
			name:          "adaptive-capable override model",
			overrideModel: "claude-sonnet-4-6",
			effort:        "low",
			assertSummaryRequest: func(t *testing.T, req *chattest.AnthropicRequest) {
				require.Contains(t, string(req.OutputConfig), `"effort":"low"`)
				require.Contains(t, string(req.Thinking), `"type":"adaptive"`)
			},
		},
	}

	for _, tc := range routingCases {
		t.Run("summary routes to the override model and continuation stays on the chat model/"+tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			reg := prometheus.NewRegistry()
			var streamCount atomic.Int32
			anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
				body := anthropicRequestBody(t, *req)
				if !req.Stream {
					if strings.Contains(body, "You are performing a context compaction") {
						require.Equal(t, tc.overrideModel, req.Model)
						tc.assertSummaryRequest(t, req)
						return anthropicCompactionResponse(compactionSummary)
					}
					return chattest.AnthropicNonStreamingResponse("title")
				}
				require.Equal(t, chatModelName, req.Model)
				switch streamCount.Add(1) {
				case 1:
					return highUsageReadFileResponse("/tmp/a.txt")
				default:
					require.Contains(t, body, compactionSummary)
					require.Empty(t, string(req.OutputConfig),
						"the override reasoning effort must not leak into chat model generations")
					require.Empty(t, string(req.Thinking),
						"the override reasoning effort must not leak into chat model generations")
					return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
						InputTokens:  20,
						OutputTokens: 5,
					}, "continued after compaction")...)
				}
			})
			user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
			model = updateChatModelCompressionThreshold(t, db, model, 100, thresholdPercent)
			overrideModel := seedOverrideModel(ctx, t, db, model, tc.overrideModel, tc.effort, 1_000_000)
			ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			setupToolExecutionAgentConn(t, mockConn)
			mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
				Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main"}, nil).
				Times(1)

			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
				cfg.PrometheusRegistry = reg
				cfg.AlwaysEnableDebugLogs = true
				cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
					require.Equal(t, dbAgent.ID, agentID)
					return mockConn, func() {}, nil
				}
			})
			chat, err := server.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
				AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
				Title:          "compaction-override",
				ModelConfigID:  model.ID,
				InitialUserContent: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("read the file and continue"),
				},
			})
			require.NoError(t, err)
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

			messages := chatMessages(ctx, t, db, chat.ID)
			promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
			require.NoError(t, err)
			compressed := compressedChatSummarizedMessages(t, append(promptMessages, messages...))
			require.Len(t, compressed.summaries, 1)
			require.Contains(t, messageText(t, compressed.summaries[0]), compactionSummary)
			requireTextPart(t, messages[len(messages)-1], "continued after compaction")

			requireChatdMetricCounter(t, reg, "coderd_chatd_compaction_total", 1, map[string]string{
				"provider": "anthropic",
				"model":    tc.overrideModel,
				"result":   "success",
			})

			require.NoError(t, server.Close())
			debugCtx := testutil.Context(t, testutil.WaitLong)
			var compactionRun database.ChatDebugRun
			testutil.Eventually(debugCtx, t, func(ctx context.Context) bool {
				runs, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
					ChatID:   chat.ID,
					LimitVal: 100,
				})
				if err != nil {
					return false
				}
				for _, run := range runs {
					if run.Kind == string(chatdebug.KindCompaction) {
						compactionRun = run
						return true
					}
				}
				return false
			}, testutil.IntervalMedium)
			require.True(t, compactionRun.Provider.Valid)
			require.Equal(t, "anthropic", compactionRun.Provider.String)
			require.True(t, compactionRun.Model.Valid)
			require.Equal(t, tc.overrideModel, compactionRun.Model.String)
			require.True(t, compactionRun.ModelConfigID.Valid)
			require.Equal(t, overrideModel.ID, compactionRun.ModelConfigID.UUID)
		})
	}

	t.Run("compaction triggers at the stricter override context limit", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamCount atomic.Int32
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			body := anthropicRequestBody(t, *req)
			if !req.Stream {
				if strings.Contains(body, "You are performing a context compaction") {
					require.Equal(t, overrideModelName, req.Model)
					return anthropicCompactionResponse(compactionSummary)
				}
				return chattest.AnthropicNonStreamingResponse("title")
			}
			switch streamCount.Add(1) {
			case 1:
				return highUsageReadFileResponse("/tmp/a.txt")
			default:
				require.Contains(t, body, compactionSummary)
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
					InputTokens:  20,
					OutputTokens: 5,
				}, "continued after compaction")...)
			}
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		// The chat model alone would not compact: 80 tokens of usage is
		// 8% of its 1000-token limit. The override model's 100-token
		// limit makes the effective threshold 70 tokens, so compaction
		// must trigger.
		model = updateChatModelCompressionThreshold(t, db, model, 1_000, thresholdPercent)
		seedOverrideModel(ctx, t, db, model, overrideModelName, "high", 100)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main"}, nil).
			Times(1)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "compaction-override-limit",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("read the file and continue"),
			},
		})
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		messages := chatMessages(ctx, t, db, chat.ID)
		promptMessages, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		require.NoError(t, err)
		compressed := compressedChatSummarizedMessages(t, append(promptMessages, messages...))
		require.Len(t, compressed.summaries, 1)
		requireTextPart(t, messages[len(messages)-1], "continued after compaction")
	})
}

func TestActiveServer_AnthropicModelReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		model         string
		effort        string
		checkThinking func(t *testing.T, thinking string)
	}{
		{
			// Pre-4.6 models reject adaptive thinking, so effort becomes an
			// enabled-thinking budget derived from max_tokens.
			name:   "LegacyModelEffortHigh",
			model:  "claude-haiku-4-5",
			effort: "high",
			checkThinking: func(t *testing.T, thinking string) {
				require.Contains(t, thinking, `"type":"enabled"`)
				require.Contains(t, thinking, `"budget_tokens":3276`)
			},
		},
		{
			// Claude 5+ models run adaptive thinking when the request omits
			// the thinking field, so effort none must send an explicit
			// disable.
			name:   "AdaptiveDefaultModelEffortNone",
			model:  "claude-sonnet-5",
			effort: "none",
			checkThinking: func(t *testing.T, thinking string) {
				require.Contains(t, thinking, `"type":"disabled"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			requests := newAnthropicRequestRecorder()
			anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
				requests.record(req)
				if !req.Stream {
					return chattest.AnthropicNonStreamingResponse("title")
				}
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
			})
			user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
			model.Model = tt.model
			model = updateChatModelContextLimit(t, db, model)
			model = updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
				MaxOutputTokens: ptr.Ref(int64(4096)),
				ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
					Default: &tt.effort,
					Max:     &tt.effort,
				},
			})

			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			})
			chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

			messages := chatMessages(ctx, t, db, chat.ID)
			requireTextPart(t, messages[len(messages)-1], "done")

			generationRequests := filterAnthropicStreamingRequests(requests.all())
			require.Len(t, generationRequests, 1)
			req := generationRequests[0]
			require.Equal(t, tt.model, req.Model)
			require.Empty(t, string(req.OutputConfig))
			tt.checkThinking(t, string(req.Thinking))
		})
	}
}

func TestActiveServer_BasicAssistantGenerationAndPromptPreparation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests.record(req)
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model.ContextLimit = 4096
	model = updateChatModelContextLimit(t, db, model)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertSystemTextMessage(ctx, t, db, chat.ID, "sys-2", model.ID)
	insertAssistantTextMessage(ctx, t, db, chat.ID, "working", model.ID)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, generationRequests, 2)
	recovered := generationRequests[1]
	require.Contains(t, string(recovered.System), `"cache_control":{"type":"ephemeral"}`)
	require.Len(t, recovered.Messages, 4)
	require.False(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[0]))
	require.False(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[1]))
	require.True(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[2]))
	require.True(t, anthropicMessageHasEphemeralCacheControl(t, recovered.Messages[3]))
	require.False(t, anthropicRequestContainsPromptSentinel(t, recovered))
	toolNames := anthropicRequestToolNames(recovered)
	require.Contains(t, toolNames, "read_file")
	require.Contains(t, toolNames, "write_file")

	messages := chatMessages(ctx, t, db, chat.ID)
	last := messages[len(messages)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, last.Role)
	require.True(t, last.ContextLimit.Valid)
	require.Equal(t, int64(4096), last.ContextLimit.Int64)
	require.GreaterOrEqual(t, last.RuntimeMs.Int64, int64(0))
	requireTextPart(t, last, "done")

	requests = newAnthropicRequestRecorder()
	server = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	planChat := createPlanSubagentChatWithHistory(ctx, t, db, org.ID, user.ID, model.ID)
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        planChat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, planChat.ID, database.ChatStatusWaiting)

	planRequests := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, planRequests, 1)
	toolNames = anthropicRequestToolNames(planRequests[0])
	require.Contains(t, toolNames, "read_file")
	require.NotContains(t, toolNames, "write_file")
}

func TestActiveServer_ToolExecutionAndPolicy(t *testing.T) {
	t.Parallel()

	t.Run("rejects disallowed active tool", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamedCallCount.Add(1) == 1 {
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("write_file", `{"path":"/tmp/nope","content":"blocked"}`),
				)
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "active-tool-reject",
			ModelConfigID:  model.ID,
			ChatMode:       database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("try to write a file"),
			},
		})
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		parts := chatToolParts(ctx, t, db, chat.ID)
		result := requireToolResultPart(t, parts, "write_file")
		require.True(t, result.IsError)
		require.JSONEq(t, `{"error":"Tool not active in this turn: write_file"}`, string(result.Result))
	})

	t.Run("provider runner executes and preserves metadata", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		const computerResultMetadata = `{"openai":{"type":"openai.responses.computer_call_output_options","data":{"detail":"original"}}}`
		var streamedCallCount atomic.Int32
		var secondRawBody []byte
		var callsMu sync.Mutex
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamedCallCount.Add(1) == 1 {
				callsMu.Lock()
				secondRawBody = append([]byte(nil), req.RawBody...)
				callsMu.Unlock()
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, _, model := seedChatDependenciesWithProviderPolicy(t, db, "openai", openAIURL, "test-key", true, false, true)
		apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
		model.Model = "gpt-5.5"
		model = updateChatModelContextLimit(t, db, model)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.AllowBYOKSet = true
			cfg.AllowBYOK = false
		})
		result := codersdk.ChatMessageToolResult(
			"computer-call",
			"computer",
			json.RawMessage(`{"data":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==","mime_type":"image/png"}`),
			false,
			true,
		)
		result.ProviderMetadata = json.RawMessage(computerResultMetadata)
		computerCall := codersdk.ChatMessageToolCall(
			"computer-call",
			"computer",
			json.RawMessage(`{"type":"screenshot"}`),
		)
		computerCall.ProviderExecuted = true
		created, err := chatstate.CreateChat(dbauthz.AsSystemRestricted(ctx), db, ps, chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "provider-runner-replay-active",
			MCPServerIDs:      []uuid.UUID{},
			ClientType:        database.ChatClientTypeApi,
			InitialMessages: []chatstate.Message{
				userMessageForTest(t, "use provider runner", model.ID, user.ID, apiKey.ID),
				assistantMessageForTest(t, []codersdk.ChatMessagePart{computerCall}, model.ID),
				toolMessageForTest(t, []codersdk.ChatMessagePart{result}, model.ID),
			},
		})
		require.NoError(t, err)
		chat := created.Chat
		_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
		})
		require.NoError(t, err)
		waitForTerminalChat(ctx, t, db, chat.ID)
		gotChat, gotErr := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, gotErr)
		require.Equal(t, database.ChatStatusWaiting, gotChat.Status)
		require.Eventually(t, func() bool { return streamedCallCount.Load() >= 1 }, testutil.WaitShort, testutil.IntervalFast)

		callsMu.Lock()
		body := string(secondRawBody)
		callsMu.Unlock()
		require.Contains(t, body, "computer_call_output")
		require.Contains(t, body, `"detail":"original"`)
	})

	t.Run("multi step local tool execution", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		var secondCallMessages []chattest.OpenAIMessage
		var callsMu sync.Mutex
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamedCallCount.Add(1) == 1 {
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/a.txt"}`),
				)
			}
			callsMu.Lock()
			secondCallMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
			callsMu.Unlock()
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("all done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{
				Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main",
			}, nil).
			Times(1)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "multi-step-tool",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("read the file"),
			},
		})
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
		parts := chatToolParts(ctx, t, db, chat.ID)
		call := requireToolCallPart(t, parts, "read_file")
		result := requireToolResultPart(t, parts, "read_file")
		require.False(t, result.IsError)
		require.NotNil(t, call.CreatedAt)
		require.NotNil(t, result.CreatedAt)
		require.False(t, result.CreatedAt.Before(*call.CreatedAt))
		messages := chatMessages(ctx, t, db, chat.ID)
		requireTextPart(t, messages[len(messages)-1], "all done")

		callsMu.Lock()
		secondMessages := append([]chattest.OpenAIMessage(nil), secondCallMessages...)
		callsMu.Unlock()
		require.NotEmpty(t, secondMessages)
		require.True(t, openAIMessagesContain(secondMessages, "1\\tpackage main"))
	})

	t.Run("parallel local and provider executed timestamps", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		webSearchEnabled := true
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamedCallCount.Add(1) == 1 {
				readA := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/a.txt"}`)
				readB := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/b.txt"}`)
				second := readB.Choices[0].ToolCalls[0]
				second.Index = 1
				readA.Choices[0].ToolCalls = append(readA.Choices[0].ToolCalls, second)
				return chattest.OpenAIResponse{
					StreamingChunks: chattest.OpenAIStreamingResponse(readA).StreamingChunks,
					WebSearch:       &chattest.OpenAIWebSearchCall{ID: "ws-timestamps", Query: "coder"},
				}
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
		model = updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{WebSearchEnabled: &webSearchEnabled},
			},
		})
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true, Content: "a", FileSize: 1, TotalLines: 1, LinesRead: 1}, nil).
			Times(1)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/b.txt", int64(1), int64(0), gomock.Any()).
			Return(workspacesdk.ReadFileLinesResponse{Success: true, Content: "b", FileSize: 1, TotalLines: 1, LinesRead: 1}, nil).
			Times(1)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "parallel-timestamps",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("search and read files"),
			},
		})
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		parts := chatToolParts(ctx, t, db, chat.ID)
		for _, toolName := range []string{"read_file", "web_search"} {
			call := requireToolCallPart(t, parts, toolName)
			result := requireToolResultPart(t, parts, toolName)
			require.NotNil(t, call.CreatedAt, toolName)
			require.NotNil(t, result.CreatedAt, toolName)
			require.False(t, result.CreatedAt.Before(*call.CreatedAt), toolName)
			if toolName == "web_search" {
				require.True(t, call.ProviderExecuted)
				require.True(t, result.ProviderExecuted)
			} else {
				require.False(t, call.ProviderExecuted)
				require.False(t, result.ProviderExecuted)
			}
		}
	})
}

func TestActiveServer_RecordsGenerationMetrics(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	reg := prometheus.NewRegistry()
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(openAITextChunksWithStop("hello")...)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.PrometheusRegistry = reg
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	requireChatdMetricCounter(t, reg, "coderd_chatd_steps_total", 1, map[string]string{
		"provider": "openai",
		"model":    "gpt-4o-mini",
	})
	requireChatdMetricHistogram(t, reg, "coderd_chatd_message_count", 1, map[string]string{
		"provider": "openai",
		"model":    "gpt-4o-mini",
	}, chatdMetricHistogramRequirement{})
	requireChatdMetricHistogram(t, reg, "coderd_chatd_prompt_size_bytes", 1, map[string]string{
		"provider": "openai",
		"model":    "gpt-4o-mini",
	}, chatdMetricHistogramRequirement{PositiveSum: true})
	requireChatdMetricHistogram(t, reg, "coderd_chatd_ttft_seconds", 1, map[string]string{
		"provider": "openai",
		"model":    "gpt-4o-mini",
	}, chatdMetricHistogramRequirement{})
}

func TestActiveServer_ToolErrorRecordsMetric(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		toolName    string
		toolArgs    string
		chatMode    database.NullChatMode
		setupAgent  func(*agentconnmock.MockAgentConn)
		seedContext func(ctx context.Context, t *testing.T, db database.Store, agentID uuid.UUID)
	}{
		{
			name:     "builtin tool IsError",
			toolName: "read_file",
			toolArgs: `{"path":"/tmp/missing.txt"}`,
			setupAgent: func(mockConn *agentconnmock.MockAgentConn) {
				mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/missing.txt", int64(1), int64(0), gomock.Any()).
					Return(workspacesdk.ReadFileLinesResponse{Success: false, Error: "file not found"}, nil).
					Times(1)
			},
		},
		{
			name:     "non builtin MCP style tool IsError",
			toolName: "dynamic__error_tool",
			toolArgs: `{"input":"hello"}`,
			setupAgent: func(mockConn *agentconnmock.MockAgentConn) {
				mockConn.EXPECT().CallMCPTool(gomock.Any(), gomock.Any()).
					Return(workspacesdk.CallMCPToolResponse{
						IsError: true,
						Content: []workspacesdk.MCPToolContent{{
							Type: "text",
							Text: "dynamic failed",
						}},
					}, nil).
					Times(1)
			},
			seedContext: func(ctx context.Context, t *testing.T, db database.Store, agentID uuid.UUID) {
				seedAgentMCPToolContext(ctx, t, db, agentMCPToolContext{
					AgentID:         agentID,
					ServerName:      "dynamic",
					ToolName:        "error_tool",
					ToolDescription: "dynamic error tool",
				})
			},
		},
		{
			name:     "tool Run returns error",
			toolName: "read_file",
			toolArgs: `{"path":"/tmp/error.txt"}`,
			setupAgent: func(mockConn *agentconnmock.MockAgentConn) {
				mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/error.txt", int64(1), int64(0), gomock.Any()).
					Return(workspacesdk.ReadFileLinesResponse{}, xerrors.New("connection refused")).
					Times(1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			reg := prometheus.NewRegistry()
			var streamedCallCount atomic.Int32
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				if !req.Stream {
					return chattest.OpenAINonStreamingResponse("title")
				}
				if streamedCallCount.Add(1) == 1 {
					return chattest.OpenAIStreamingResponse(
						chattest.OpenAIToolCallChunk(tt.toolName, tt.toolArgs),
					)
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			})
			user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
			model.Model = "test-model"
			model = updateChatModelContextLimit(t, db, model)
			ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			setupToolExecutionAgentConn(t, mockConn)
			tt.setupAgent(mockConn)

			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
				cfg.PrometheusRegistry = reg
				cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
					require.Equal(t, dbAgent.ID, agentID)
					return mockConn, func() {}, nil
				}
			})
			chatOpts := chatd.CreateOptions{
				OrganizationID: org.ID,
				OwnerID:        user.ID,
				WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
				AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
				Title:          "tool-error-metric",
				ModelConfigID:  model.ID,
				ChatMode:       tt.chatMode,
				InitialUserContent: []codersdk.ChatMessagePart{
					codersdk.ChatMessageText("run an erroring tool"),
				},
			}
			if tt.seedContext != nil {
				tt.seedContext(ctx, t, db, dbAgent.ID)
			}
			chat, err := server.CreateChat(ctx, chatOpts)
			require.NoError(t, err)
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

			result := requireToolResultPart(t, chatToolParts(ctx, t, db, chat.ID), tt.toolName)
			require.True(t, result.IsError)
			requireChatdMetricCounter(t, reg, "coderd_chatd_tool_errors_total", 1, map[string]string{
				"provider":  "openai-compat",
				"model":     "test-model",
				"tool_name": tt.toolName,
			})
		})
	}
}

func userMessageForTest(
	t *testing.T,
	text string,
	modelID uuid.UUID,
	createdBy uuid.UUID,
	apiKeyID string,
) chatstate.Message {
	t.Helper()
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleUser,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
		CreatedBy:      uuid.NullUUID{UUID: createdBy, Valid: true},
	}
}

func assistantMessageForTest(
	t *testing.T,
	parts []codersdk.ChatMessagePart,
	modelID uuid.UUID,
) chatstate.Message {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
	}
}

func toolMessageForTest(
	t *testing.T,
	parts []codersdk.ChatMessagePart,
	modelID uuid.UUID,
) chatstate.Message {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return chatstate.Message{
		Role:           database.ChatMessageRoleTool,
		Content:        content,
		Visibility:     database.ChatMessageVisibilityBoth,
		ContentVersion: chatprompt.CurrentContentVersion,
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
	}
}

func setupToolExecutionAgentConn(
	t *testing.T,
	mockConn *agentconnmock.MockAgentConn,
) {
	t.Helper()
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()
}

func dynamicToolJSON(t *testing.T, name string) []byte {
	t.Helper()
	encoded, err := json.Marshal([]mcpgo.Tool{{
		Name:        name,
		Description: "A test dynamic tool.",
		InputSchema: mcpgo.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}})
	require.NoError(t, err)
	return encoded
}

func toolResultPartExists(parts []codersdk.ChatMessagePart, toolName string) bool {
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == toolName {
			return true
		}
	}
	return false
}

func chatToolParts(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
) []codersdk.ChatMessagePart {
	t.Helper()
	var parts []codersdk.ChatMessagePart
	for _, msg := range chatMessages(ctx, t, db, chatID) {
		parsed, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parsed {
			if part.Type == codersdk.ChatMessagePartTypeToolCall ||
				part.Type == codersdk.ChatMessagePartTypeToolResult {
				parts = append(parts, part)
			}
		}
	}
	return parts
}

func requireToolCallPart(
	t *testing.T,
	parts []codersdk.ChatMessagePart,
	toolName string,
) codersdk.ChatMessagePart {
	t.Helper()
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolName == toolName {
			return part
		}
	}
	t.Fatalf("missing tool-call part for %q", toolName)
	return codersdk.ChatMessagePart{}
}

func requireToolResultPart(
	t *testing.T,
	parts []codersdk.ChatMessagePart,
	toolName string,
) codersdk.ChatMessagePart {
	t.Helper()
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == toolName {
			return part
		}
	}
	t.Fatalf("missing tool-result part for %q", toolName)
	return codersdk.ChatMessagePart{}
}

func openAIMessagesContain(messages []chattest.OpenAIMessage, text string) bool {
	for _, msg := range messages {
		if strings.Contains(msg.Content, text) {
			return true
		}
	}
	return false
}

func requireChatdMetricCounter(
	t *testing.T,
	reg *prometheus.Registry,
	name string,
	wantValue float64,
	wantLabels map[string]string,
) {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := metricLabels(metric)
			if !metricLabelsMatch(labels, wantLabels) {
				continue
			}
			require.Equal(t, wantValue, metric.GetCounter().GetValue())
			return
		}
		t.Fatalf("metric %s with labels %v not found", name, wantLabels)
	}
	t.Fatalf("metric %s not found", name)
}

type chatdMetricHistogramRequirement struct {
	PositiveSum bool
}

func requireChatdMetricHistogram(
	t *testing.T,
	reg *prometheus.Registry,
	name string,
	wantSampleCount uint64,
	wantLabels map[string]string,
	requirement chatdMetricHistogramRequirement,
) {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := metricLabels(metric)
			if !metricLabelsMatch(labels, wantLabels) {
				continue
			}
			histogram := metric.GetHistogram()
			require.Equal(t, wantSampleCount, histogram.GetSampleCount())
			if requirement.PositiveSum {
				require.Positive(t, histogram.GetSampleSum())
			}
			return
		}
		t.Fatalf("metric %s with labels %v not found", name, wantLabels)
	}
	t.Fatalf("metric %s not found", name)
}

func metricLabels(metric interface {
	GetLabel() []*io_prometheus_client.LabelPair
},
) map[string]string {
	labels := map[string]string{}
	for _, label := range metric.GetLabel() {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}

func metricLabelsMatch(labels, wantLabels map[string]string) bool {
	for key, value := range wantLabels {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func TestActiveServer_AnthropicUsageMatchesFinalDelta(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	anthropicURL := chattest.NewAnthropic(t, func(_ *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
			InputTokens:              200,
			OutputTokens:             75,
			CacheCreationInputTokens: 30,
			CacheReadInputTokens:     150,
		}, "cached response")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	messages := chatMessages(ctx, t, db, chat.ID)
	last := messages[len(messages)-1]
	require.Equal(t, database.ChatMessageRoleAssistant, last.Role)
	require.Equal(t, sql.NullInt64{Int64: 200, Valid: true}, last.InputTokens)
	require.Equal(t, sql.NullInt64{Int64: 75, Valid: true}, last.OutputTokens)
	require.Equal(t, sql.NullInt64{Int64: 275, Valid: true}, last.TotalTokens)
	require.Equal(t, sql.NullInt64{Int64: 30, Valid: true}, last.CacheCreationTokens)
	require.Equal(t, sql.NullInt64{Int64: 150, Valid: true}, last.CacheReadTokens)
}

func TestActiveServer_ChatTurnDebugRunRecordsStreamStep(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse(`{"label":"Debug response"}`)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
			InputTokens:              200,
			OutputTokens:             75,
			CacheCreationInputTokens: 30,
			CacheReadInputTokens:     150,
		}, "debug response")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.AlwaysEnableDebugLogs = true
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello debug")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.NoError(t, server.Close())
	debugCtx := testutil.Context(t, testutil.WaitLong)

	var chatTurnRuns []database.ChatDebugRun
	testutil.Eventually(debugCtx, t, func(ctx context.Context) bool {
		runs, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
			ChatID:   chat.ID,
			LimitVal: 100,
		})
		if err != nil {
			return false
		}
		chatTurnRuns = chatTurnRuns[:0]
		for _, run := range runs {
			if run.Kind == string(codersdk.ChatDebugRunKindChatTurn) {
				chatTurnRuns = append(chatTurnRuns, run)
			}
		}
		return len(chatTurnRuns) == 1 && chatTurnRuns[0].FinishedAt.Valid
	}, testutil.IntervalFast)

	require.Len(t, chatTurnRuns, 1)
	run := chatTurnRuns[0]
	require.Equal(t, string(codersdk.ChatDebugStatusCompleted), run.Status)

	steps, err := db.GetChatDebugStepsByRunID(debugCtx, run.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	step := steps[0]
	require.Equal(t, string(codersdk.ChatDebugStepOperationStream), step.Operation)
	require.Equal(t, string(codersdk.ChatDebugStatusCompleted), step.Status)
	require.NotEmpty(t, step.NormalizedRequest)
	require.True(t, step.NormalizedResponse.Valid)
	require.True(t, step.Usage.Valid)
	require.NotEmpty(t, step.Attempts)
	require.True(t, step.FinishedAt.Valid)

	var normalizedRequest map[string]any
	require.NoError(t, json.Unmarshal(step.NormalizedRequest, &normalizedRequest))
	require.NotEmpty(t, normalizedRequest["messages"])

	var normalizedResponse map[string]any
	require.NoError(t, json.Unmarshal(step.NormalizedResponse.RawMessage, &normalizedResponse))
	require.NotEmpty(t, normalizedResponse["content"])
	require.NotEmpty(t, normalizedResponse["usage"])

	var usage map[string]any
	require.NoError(t, json.Unmarshal(step.Usage.RawMessage, &usage))
	require.EqualValues(t, 200, usage["input_tokens"])
	require.EqualValues(t, 75, usage["output_tokens"])
	require.EqualValues(t, 30, usage["cache_creation_tokens"])
	require.EqualValues(t, 150, usage["cache_read_tokens"])

	var attempts []map[string]any
	require.NoError(t, json.Unmarshal(step.Attempts, &attempts))
	require.Len(t, attempts, 1)
	require.NotEmpty(t, attempts[0]["request_body"])
	require.NotEmpty(t, attempts[0]["response_body"])

	var summary map[string]any
	require.NoError(t, json.Unmarshal(run.Summary, &summary))
	require.Equal(t, "POST /v1/messages", summary["endpoint_label"])
	require.Equal(t, "hello debug", summary["first_message"])
	require.EqualValues(t, 1, summary["step_count"])
	require.EqualValues(t, 200, summary["total_input_tokens"])
	require.EqualValues(t, 75, summary["total_output_tokens"])
	require.EqualValues(t, 30, summary["total_cache_creation_tokens"])
	require.EqualValues(t, 150, summary["total_cache_read_tokens"])
}

func TestActiveServer_ChatTurnDebugRunRecordsMultipleStreamSteps(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	var streamCount atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse(`{"label":"Read file"}`)
		}
		switch streamCount.Add(1) {
		case 1:
			return chattest.AnthropicStreamingResponse(
				chattest.AnthropicToolCallChunks("read_file", `{"path":"/tmp/a.txt"}`)...,
			)
		case 2:
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunksWithCacheUsage(chattest.AnthropicUsage{
				InputTokens:  20,
				OutputTokens: 7,
			}, "final debug response")...)
		default:
			t.Fatalf("unexpected stream request %d", streamCount.Load())
			return chattest.AnthropicStreamingResponse()
		}
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "/tmp/a.txt", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, FileSize: 12, TotalLines: 1, LinesRead: 1, Content: "1\tpackage main"}, nil).
		Times(1)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.AlwaysEnableDebugLogs = true
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "multi-step-debug",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("read the file and continue"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	require.NoError(t, server.Close())
	debugCtx := testutil.Context(t, testutil.WaitLong)

	var chatTurnRuns []database.ChatDebugRun
	testutil.Eventually(debugCtx, t, func(ctx context.Context) bool {
		runs, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
			ChatID:   chat.ID,
			LimitVal: 100,
		})
		if err != nil {
			return false
		}
		chatTurnRuns = chatTurnRuns[:0]
		for _, run := range runs {
			if run.Kind == string(codersdk.ChatDebugRunKindChatTurn) {
				chatTurnRuns = append(chatTurnRuns, run)
			}
		}
		if len(chatTurnRuns) != 1 || !chatTurnRuns[0].FinishedAt.Valid {
			return false
		}
		steps, err := db.GetChatDebugStepsByRunID(ctx, chatTurnRuns[0].ID)
		return err == nil && len(steps) == 2
	}, testutil.IntervalFast)

	require.Len(t, chatTurnRuns, 1)
	run := chatTurnRuns[0]
	require.Equal(t, string(codersdk.ChatDebugStatusCompleted), run.Status)

	steps, err := db.GetChatDebugStepsByRunID(debugCtx, run.ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	for i, step := range steps {
		require.EqualValues(t, i+1, step.StepNumber)
		require.Equal(t, string(codersdk.ChatDebugStepOperationStream), step.Operation)
		require.Equal(t, string(codersdk.ChatDebugStatusCompleted), step.Status)
		require.NotEmpty(t, step.Attempts)
		require.True(t, step.FinishedAt.Valid)
	}

	var firstResponse map[string]any
	require.NoError(t, json.Unmarshal(steps[0].NormalizedResponse.RawMessage, &firstResponse))
	require.NotEmpty(t, firstResponse["content"])

	var secondResponse map[string]any
	require.NoError(t, json.Unmarshal(steps[1].NormalizedResponse.RawMessage, &secondResponse))
	require.NotEmpty(t, secondResponse["content"])

	var summary map[string]any
	require.NoError(t, json.Unmarshal(run.Summary, &summary))
	require.Equal(t, "POST /v1/messages", summary["endpoint_label"])
	require.EqualValues(t, 2, summary["step_count"])
	require.EqualValues(t, 30, summary["total_input_tokens"])
	require.EqualValues(t, 12, summary["total_output_tokens"])
}

func TestActiveServer_AnthropicSanitizesProviderToolBeforeRequest(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests.record(req)
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search for coder")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertOrphanProviderToolCall(ctx, t, db, chat.ID, model.ID)
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: model.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
		BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)

	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, generationRequests, 2)
	body := anthropicRequestBody(t, generationRequests[1])
	require.NotContains(t, body, "web_search")
	require.Contains(t, body, "partial")
	require.Contains(t, body, "continue")
	require.Contains(t, body, "redacted-payload")
}

func TestActiveServer_AnthropicContext1MBetaHeader(t *testing.T) {
	t.Parallel()

	runChat := func(t *testing.T, callConfig *codersdk.ChatModelCallConfig) []chattest.AnthropicRequest {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		requests := newAnthropicRequestRecorder()
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			requests.record(req)
			if !req.Stream {
				return chattest.AnthropicNonStreamingResponse("title")
			}
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		if callConfig != nil {
			model = updateChatModelCallConfig(t, db, model, *callConfig)
		}

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		generationRequests := filterAnthropicStreamingRequests(requests.all())
		require.NotEmpty(t, generationRequests)
		return generationRequests
	}

	t.Run("enabled sends beta header", func(t *testing.T) {
		t.Parallel()

		generationRequests := runChat(t, &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
					Context1MEnabled: ptr.Ref(true),
				},
			},
		})
		for _, req := range generationRequests {
			require.Equal(t, chatprovider.AnthropicBetaContext1M, req.Header.Get(chatprovider.HeaderAnthropicBeta))
		}
	})

	t.Run("unset omits beta header", func(t *testing.T) {
		t.Parallel()

		generationRequests := runChat(t, nil)
		for _, req := range generationRequests {
			require.Empty(t, req.Header.Get(chatprovider.HeaderAnthropicBeta))
		}
	})
}

func TestActiveServer_AnthropicProviderToolPreRequestGuard(t *testing.T) {
	t.Parallel()

	webSearchEnabled := true
	callConfig := codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
				WebSearchEnabled: &webSearchEnabled,
			},
		},
	}

	t.Run("allowed web search survives when provider tool is enabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		requests := newAnthropicRequestRecorder()
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			requests.record(req)
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
		model = updateChatModelCallConfig(t, db, model, callConfig)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search")
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		insertProviderToolPairMessageWithLocalTool(ctx, t, db, chat.ID, model.ID, "ws-allowed")
		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
		})
		require.NoError(t, err)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		generationRequests := filterAnthropicStreamingRequests(requests.all())
		require.Len(t, generationRequests, 2)
		body := anthropicRequestBody(t, generationRequests[1])
		require.Contains(t, body, "ws-allowed")
		require.Contains(t, body, "web_search")
	})

	t.Run("web search history survives when provider tool is disabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		requests := newAnthropicRequestRecorder()
		anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
			requests.record(req)
			return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
		})
		user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search and read")
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		insertProviderToolPairMessageWithLocalTool(ctx, t, db, chat.ID, model.ID, "ws-disabled")
		_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:        chat.ID,
			CreatedBy:     user.ID,
			ModelConfigID: model.ID,
			Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue")},
			BusyBehavior:  chatd.SendMessageBusyBehaviorQueue,
		})
		require.NoError(t, err)

		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		generationRequests := filterAnthropicStreamingRequests(requests.all())
		require.Len(t, generationRequests, 2)
		body := anthropicRequestBody(t, generationRequests[1])
		require.Contains(t, body, "ws-disabled")
		require.Contains(t, body, "web_search")
		require.Contains(t, body, "tc-1")
		require.Contains(t, body, "file")
	})
}

func TestActiveServer_AnthropicDropsUnpairedProviderToolBeforePersist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		toolName  string
		toolInput json.RawMessage
	}{
		{
			name:      "web_search",
			toolName:  "web_search",
			toolInput: json.RawMessage(`{"query":"coder"}`),
		},
		{
			name:      "code_execution",
			toolName:  "code_execution",
			toolInput: json.RawMessage(`{"code":"print(1)"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps := dbtestutil.NewDB(t)
			requests := newAnthropicRequestRecorder()
			var requestCount atomic.Int32
			anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
				requests.record(req)
				if !req.Stream {
					return chattest.AnthropicNonStreamingResponse("title")
				}
				if requestCount.Add(1) == 1 {
					return chattest.AnthropicStreamingResponse(
						anthropicServerToolUseChunks("pt-1", tt.toolName, tt.toolInput, "tool_use")...,
					)
				}
				return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("after sanitized step")...)
			})
			user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
			model = enableAnthropicWebSearchForTest(t, db, model)

			server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
				cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
			})
			chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "run provider tool")
			waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

			generationRequests := filterAnthropicStreamingRequests(requests.all())
			require.Len(t, generationRequests, 1)
			messages := chatMessages(ctx, t, db, chat.ID)
			last := messages[len(messages)-1]
			require.Equal(t, database.ChatMessageRoleUser, last.Role)
			requireTextPart(t, last, "run provider tool")
			require.False(t, toolPartExists(chatToolParts(ctx, t, db, chat.ID), tt.toolName),
				"unpaired provider tool content should not be committed")
		})
	}
}

func TestActiveServer_AnthropicKeepsPairedWebSearchBeforePersist(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests.record(req)
		return chattest.AnthropicStreamingResponse(
			anthropicWebSearchPairChunks("ws-1", `{"query":"coder"}`, "search done", "end_turn")...,
		)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = enableAnthropicWebSearchForTest(t, db, model)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search for coder")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, generationRequests, 1)
	parts := chatToolParts(ctx, t, db, chat.ID)
	toolCall := requireToolCallPart(t, parts, "web_search")
	require.Equal(t, "ws-1", toolCall.ToolCallID)
	require.True(t, toolCall.ProviderExecuted)
	toolResult := requireToolResultPart(t, parts, "web_search")
	require.Equal(t, "ws-1", toolResult.ToolCallID)
	require.True(t, toolResult.ProviderExecuted)
	require.NotEmpty(t, toolResult.ProviderMetadata)
	messages := chatMessages(ctx, t, db, chat.ID)
	requireTextPart(t, messages[len(messages)-1], "search done")
}

// TestActiveServer_AnthropicWebSearchFollowUpHasNoSyntheticCancellation
// reproduces a bug where sending a follow-up user message after a
// completed provider-executed web_search turn inserted a synthetic
// cancellation tool-result ("Tool execution interrupted by new user
// message") for the server tool call. The provider-executed result
// lives inside the assistant message, so the cancellation synthesizer
// saw the call as outstanding and emitted a client-style tool-role
// result for a srvtoolu_ ID. On the next request that result replays
// as a plain tool_result block, which Anthropic rejects:
//
//	unexpected `tool_use_id` found in `tool_result` blocks:
//	srvtoolu_... Each `tool_result` block must have a
//	corresponding `tool_use` block in the previous message.
func TestActiveServer_AnthropicWebSearchFollowUpHasNoSyntheticCancellation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	var streamingRequestCount atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests.record(req)
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		if streamingRequestCount.Add(1) == 1 {
			return chattest.AnthropicStreamingResponse(
				anthropicWebSearchPairChunks("srvtoolu_ws1", `{"query":"coder"}`, "search done", "end_turn")...,
			)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("follow-up done")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = enableAnthropicWebSearchForTest(t, db, model)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search for coder")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	// Simulate a web search turn followed by a user follow-up.
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("thanks, tell me more")},
	})
	require.NoError(t, err)

	// Wait for the follow-up turn to run and the chat to settle.
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return streamingRequestCount.Load() >= 2
	}, testutil.IntervalFast)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	// The provider-executed web_search call is answered by the
	// provider-executed result inside the assistant message. No
	// tool-role message may carry a synthetic result for it.
	for _, msg := range chatMessages(ctx, t, db, chat.ID) {
		if msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult {
				continue
			}
			require.NotEqual(t, "srvtoolu_ws1", part.ToolCallID,
				"provider-executed web_search call received a synthetic tool-role result: %s", string(part.Result))
		}
	}
}

func TestActiveServer_AnthropicSanitizesWebSearchBeforeContinuation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	requests := newAnthropicRequestRecorder()
	var requestCount atomic.Int32
	anthropicURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requests.record(req)
		if !req.Stream {
			return chattest.AnthropicNonStreamingResponse("title")
		}
		if requestCount.Add(1) == 1 {
			chunks := anthropicServerToolUseChunks("ws-1", "web_search", json.RawMessage(`{"query":"coder"}`), "tool_use")
			chunks = append(chunks[:len(chunks)-2], anthropicToolUseChunksWithoutMessageEnvelope(1, "tc-1", "read_file", `{"path":"main.go"}`)...)
			chunks = append(chunks,
				chattest.AnthropicChunk{
					Type:       "message_delta",
					StopReason: "tool_use",
					Usage:      chattest.AnthropicUsage{InputTokens: 10, OutputTokens: 5},
				},
				chattest.AnthropicChunk{Type: "message_stop"},
			)
			return chattest.AnthropicStreamingResponse(chunks...)
		}
		return chattest.AnthropicStreamingResponse(chattest.AnthropicTextChunks("done")...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = enableAnthropicWebSearchForTest(t, db, model)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	setupToolExecutionAgentConn(t, mockConn)
	mockConn.EXPECT().ReadFileLines(gomock.Any(), "main.go", int64(1), int64(0), gomock.Any()).
		Return(workspacesdk.ReadFileLinesResponse{Success: true, Content: "package main", FileSize: 12, TotalLines: 1, LinesRead: 1}, nil).
		Times(1)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		Title:          "anthropic-web-search-continuation",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("search and read"),
		},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	generationRequests := filterAnthropicStreamingRequests(requests.all())
	require.Len(t, generationRequests, 2)
	continuationBody := anthropicRequestBody(t, generationRequests[1])
	require.NotContains(t, continuationBody, "server_tool_use")
	require.NotContains(t, continuationBody, "web_search_tool_result")
	require.NotContains(t, continuationBody, "ws-1")
	require.Contains(t, continuationBody, "tc-1")
	require.Contains(t, continuationBody, "package main")

	parts := chatToolParts(ctx, t, db, chat.ID)
	require.False(t, toolPartExists(parts, "web_search"))
	toolCall := requireToolCallPart(t, parts, "read_file")
	require.Equal(t, "tc-1", toolCall.ToolCallID)
	require.False(t, toolCall.ProviderExecuted)
	toolResult := requireToolResultPart(t, parts, "read_file")
	require.Equal(t, "tc-1", toolResult.ToolCallID)
	require.False(t, toolResult.ProviderExecuted)
}

func TestActiveServer_ExclusiveToolPolicy(t *testing.T) {
	t.Parallel()

	t.Run("mixed exclusive and local tools commit policy errors", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamedCallCount.Add(1) == 1 {
				advisorChunk := chattest.OpenAIToolCallChunk("advisor", `{"question":"help"}`)
				readChunk := chattest.OpenAIToolCallChunk("read_file", `{"path":"/tmp/a.txt"}`)
				readCall := readChunk.Choices[0].ToolCalls[0]
				readCall.Index = 1
				advisorChunk.Choices[0].ToolCalls = append(advisorChunk.Choices[0].ToolCalls, readCall)
				return chattest.OpenAIStreamingResponse(advisorChunk)
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{Enabled: true, MaxUsesPerRun: 3, MaxOutputTokens: 1024})
		ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		setupToolExecutionAgentConn(t, mockConn)
		mockConn.EXPECT().ReadFileLines(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
			cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
				require.Equal(t, dbAgent.ID, agentID)
				return mockConn, func() {}, nil
			}
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
			AgentID:        uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
			Title:          "exclusive-local-policy",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("advise and read"),
			},
		})
		require.NoError(t, err)
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		parts := chatToolParts(ctx, t, db, chat.ID)
		advisorResult := requireToolResultPart(t, parts, "advisor")
		readResult := requireToolResultPart(t, parts, "read_file")
		require.True(t, advisorResult.IsError)
		require.True(t, readResult.IsError)
		require.Contains(t, string(advisorResult.Result), "advisor must be called alone, without other tools in the same batch")
		require.Contains(t, string(readResult.Result), "this tool was skipped because advisor must run alone in its batch")
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	})

	t.Run("mixed exclusive and dynamic tools commit policy errors", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			if streamedCallCount.Add(1) == 1 {
				advisorChunk := chattest.OpenAIToolCallChunk("advisor", `{"question":"help"}`)
				dynamicChunk := chattest.OpenAIToolCallChunk("mcp_tool", `{"q":"docs"}`)
				dynamicCall := dynamicChunk.Choices[0].ToolCalls[0]
				dynamicCall.Index = 1
				advisorChunk.Choices[0].ToolCalls = append(advisorChunk.Choices[0].ToolCalls, dynamicCall)
				return chattest.OpenAIStreamingResponse(advisorChunk)
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{Enabled: true, MaxUsesPerRun: 3, MaxOutputTokens: 1024})
		dynamicToolsJSON, err := json.Marshal([]mcpgo.Tool{{
			Name:        "mcp_tool",
			Description: "dynamic test tool",
			InputSchema: mcpgo.ToolInputSchema{Type: "object", Properties: map[string]any{"q": map[string]any{"type": "string"}}},
		}})
		require.NoError(t, err)

		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		})
		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "exclusive-dynamic-policy",
			ModelConfigID:  model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("advise and call dynamic"),
			},
			DynamicTools: dynamicToolsJSON,
		})
		require.NoError(t, err)
		chatResult := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
		require.NotEqual(t, database.ChatStatusRequiresAction, chatResult.Status)

		parts := chatToolParts(ctx, t, db, chat.ID)
		advisorResult := requireToolResultPart(t, parts, "advisor")
		dynamicResult := requireToolResultPart(t, parts, "mcp_tool")
		require.True(t, advisorResult.IsError)
		require.True(t, dynamicResult.IsError)
		require.Contains(t, string(advisorResult.Result), "advisor must be called alone, without other tools in the same batch")
		require.Contains(t, string(dynamicResult.Result), "this tool was skipped because advisor must run alone in its batch")
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2))
	})

	t.Run("solo exclusive tool executes", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("advisor", `{"question":"help me decide"}`),
				)
			case 2:
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("nested advice")...)
			default:
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
		seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{Enabled: true, MaxUsesPerRun: 3, MaxOutputTokens: 1024})
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "advise only")
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		parts := chatToolParts(ctx, t, db, chat.ID)
		result := requireToolResultPart(t, parts, "advisor")
		require.False(t, result.IsError)
		require.Contains(t, string(result.Result), "nested advice")
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(3))
	})

	t.Run("exclusive tool with provider executed tool executes", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		webSearchEnabled := true
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("title")
			}
			switch streamedCallCount.Add(1) {
			case 1:
				return chattest.OpenAIResponse{
					StreamingChunks: chattest.OpenAIStreamingResponse(
						chattest.OpenAIToolCallChunk("advisor", `{"question":"search informed advice"}`),
					).StreamingChunks,
					WebSearch: &chattest.OpenAIWebSearchCall{ID: "ws-advisor", Query: "coder"},
				}
			case 2:
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("nested advice")...)
			default:
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
			}
		})
		user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
		model = updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{WebSearchEnabled: &webSearchEnabled},
			},
		})
		seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{Enabled: true, MaxUsesPerRun: 3, MaxOutputTokens: 1024})
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		})
		chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "search then advise")
		waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

		parts := chatToolParts(ctx, t, db, chat.ID)
		advisorResult := requireToolResultPart(t, parts, "advisor")
		webResult := requireToolResultPart(t, parts, "web_search")
		require.False(t, advisorResult.IsError)
		require.True(t, webResult.ProviderExecuted)
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(3))
	})
}

func TestActiveServer_ReasoningTimestamps(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	sendReasoning := true
	thinkingBudget := int64(1024)
	anthropicURL := chattest.NewAnthropic(t, func(_ *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return chattest.AnthropicStreamingResponse(chattest.AnthropicReasoningTextChunks(
			[]chattest.AnthropicReasoningBlock{
				{Text: "first thought", Signature: "sig_1"},
				{Text: "second thought", Signature: "sig_2"},
			},
			"answer",
		)...)
	})
	user, org, model := seedAnthropicChatDependencies(t, db, anthropicURL)
	model = updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
				SendReasoning: &sendReasoning,
				Thinking: &codersdk.ChatModelAnthropicThinkingOptions{
					BudgetTokens: &thinkingBudget,
				},
			},
		},
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, anthropicURL, chattest.WithPreservePath()))
	})
	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "think")
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	messages := chatMessages(ctx, t, db, chat.ID)
	assistant := messages[len(messages)-1]
	reasoningParts := reasoningPartsFromMessage(t, assistant)
	require.Len(t, reasoningParts, 2)
	require.Equal(t, []string{"first thought", "second thought"}, []string{
		strings.TrimSpace(reasoningParts[0].Text),
		strings.TrimSpace(reasoningParts[1].Text),
	})
	for i := range reasoningParts {
		require.NotNil(t, reasoningParts[i].CreatedAt)
		require.NotNil(t, reasoningParts[i].CompletedAt)
		require.False(t, reasoningParts[i].CreatedAt.IsZero())
		require.False(t, reasoningParts[i].CompletedAt.IsZero())
		require.False(t, reasoningParts[i].CompletedAt.Before(*reasoningParts[i].CreatedAt))
	}
	require.False(t, reasoningParts[1].CreatedAt.Before(*reasoningParts[0].CompletedAt))
}

func TestAnthropicProviderToolPreRequestGuard(t *testing.T) {
	t.Parallel()

	providerPair := func(id string) []fantasy.MessagePart {
		return []fantasy.MessagePart{
			fantasy.ToolCallPart{
				ToolCallID:       id,
				ToolName:         "web_search",
				Input:            `{"query":"coder"}`,
				ProviderExecuted: true,
			},
			fantasy.ToolResultPart{
				ToolCallID:       id,
				Output:           fantasy.ToolResultOutputContentText{Text: "ok"},
				ProviderExecuted: true,
				ProviderOptions:  fantasy.ProviderOptions(validWebSearchProviderMetadataForTest()),
			},
		}
	}

	t.Run("orphan provider result is textified", func(t *testing.T) {
		t.Parallel()

		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "keep"},
						fantasy.ToolResultPart{
							ToolCallID:       "ws-orphan",
							Output:           fantasy.ToolResultOutputContentText{Text: "search result"},
							ProviderExecuted: true,
						},
					},
				},
			},
		)
		require.NoError(t, err)

		requireNoProviderExecutedToolResultPrompt(t, guarded)
		requireAnthropicProviderToolPromptSafe(t, guarded)
		require.Len(t, guarded, 1)
		require.Len(t, guarded[0].Content, 2)
		textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](guarded[0].Content[0])
		require.True(t, ok)
		require.Equal(t, "keep", textPart.Text)
		textPart, ok = fantasy.AsMessagePart[fantasy.TextPart](guarded[0].Content[1])
		require.True(t, ok)
		require.Equal(t, "search result", textPart.Text)
	})

	t.Run("valid provider history is unchanged", func(t *testing.T) {
		t.Parallel()

		content := []fantasy.MessagePart{fantasy.TextPart{Text: "keep"}}
		content = append(content, providerPair("ws-one")...)
		content = append(content, providerPair("ws-two")...)
		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{{Role: fantasy.MessageRoleAssistant, Content: content}},
		)
		require.NoError(t, err)

		requireAnthropicProviderToolPromptSafe(t, guarded)
		require.Len(t, guarded, 1)
		require.Len(t, guarded[0].Content, len(content))
		requireProviderExecutedToolCallPrompt(t, guarded, "ws-one")
		requireProviderExecutedToolResultPrompt(t, guarded, "ws-one")
		requireProviderExecutedToolCallPrompt(t, guarded, "ws-two")
		requireProviderExecutedToolResultPrompt(t, guarded, "ws-two")
	})

	t.Run("non Anthropic providers are unchanged", func(t *testing.T) {
		t.Parallel()

		prompt := []fantasy.Message{
			{
				Role:    fantasy.MessageRoleAssistant,
				Content: providerPair("ws-other-provider"),
			},
		}
		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			"fake",
			"fake-model",
			prompt,
		)
		require.NoError(t, err)
		require.Equal(t, prompt, guarded)
	})

	t.Run("logs removals", func(t *testing.T) {
		t.Parallel()

		logSink := testutil.NewFakeSink(t)
		logger := logSink.Logger()
		logPair := providerPair("ws-log")
		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			logger,
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						logPair[1],
						logPair[0],
					},
				},
			},
		)
		require.NoError(t, err)

		requireNoProviderExecutedToolCallPrompt(t, guarded)
		requireNoProviderExecutedToolResultPrompt(t, guarded)
		requireTextPrompt(t, guarded, "ok")
		entries := logSink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelWarn &&
				e.Message == "removed provider-executed tool history"
		})
		require.Len(t, entries, 1)
		require.Equal(t, "pre_request_guard", requireLogField(t, entries[0], "phase"))
		require.Equal(t, 1, requireLogField(t, entries[0], "removed_tool_calls"))
		require.Equal(t, 1, requireLogField(t, entries[0], "removed_tool_results"))
	})
}

func enableAnthropicWebSearchForTest(
	t *testing.T,
	db database.Store,
	model database.ChatModelConfig,
) database.ChatModelConfig {
	t.Helper()
	webSearchEnabled := true
	return updateChatModelCallConfig(t, db, model, codersdk.ChatModelCallConfig{
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
				WebSearchEnabled: &webSearchEnabled,
			},
		},
	})
}

func anthropicMessageStartChunk(messageID string) chattest.AnthropicChunk {
	return chattest.AnthropicChunk{
		Type: "message_start",
		Message: chattest.AnthropicChunkMessage{
			ID:    messageID,
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-opus-20240229",
		},
	}
}

func anthropicServerToolUseChunks(
	toolCallID string,
	toolName string,
	input json.RawMessage,
	stopReason string,
) []chattest.AnthropicChunk {
	chunks := []chattest.AnthropicChunk{
		anthropicMessageStartChunk("msg-" + toolCallID),
	}
	chunks = append(chunks, anthropicServerToolUseChunksWithoutMessageEnvelope(0, toolCallID, toolName, input)...)
	chunks = append(chunks,
		chattest.AnthropicChunk{
			Type:       "message_delta",
			StopReason: stopReason,
			Usage:      chattest.AnthropicUsage{InputTokens: 10, OutputTokens: 5},
		},
		chattest.AnthropicChunk{Type: "message_stop"},
	)
	return chunks
}

func anthropicServerToolUseChunksWithoutMessageEnvelope(
	index int,
	toolCallID string,
	toolName string,
	input json.RawMessage,
) []chattest.AnthropicChunk {
	return []chattest.AnthropicChunk{
		{
			Type:  "content_block_start",
			Index: index,
			ContentBlock: chattest.AnthropicContentBlock{
				Type:  "server_tool_use",
				ID:    toolCallID,
				Name:  toolName,
				Input: input,
			},
		},
		{
			Type:  "content_block_stop",
			Index: index,
		},
	}
}

func anthropicToolUseChunksWithoutMessageEnvelope(
	index int,
	toolCallID string,
	toolName string,
	input string,
) []chattest.AnthropicChunk {
	return []chattest.AnthropicChunk{
		{
			Type:  "content_block_start",
			Index: index,
			ContentBlock: chattest.AnthropicContentBlock{
				Type:  "tool_use",
				ID:    toolCallID,
				Name:  toolName,
				Input: json.RawMessage(`{}`),
			},
		},
		{
			Type:  "content_block_delta",
			Index: index,
			Delta: chattest.AnthropicDeltaBlock{
				Type:        "input_json_delta",
				PartialJSON: input,
			},
		},
		{
			Type:  "content_block_stop",
			Index: index,
		},
	}
}

func anthropicWebSearchPairChunks(
	toolCallID string,
	queryInput string,
	text string,
	stopReason string,
) []chattest.AnthropicChunk {
	resultContent := []map[string]any{{
		"type":              "web_search_result",
		"url":               "https://example.com/coder",
		"title":             "Coder",
		"encrypted_content": "encrypted-coder",
	}}
	chunks := []chattest.AnthropicChunk{
		anthropicMessageStartChunk("msg-" + toolCallID),
	}
	chunks = append(chunks, anthropicServerToolUseChunksWithoutMessageEnvelope(0, toolCallID, "web_search", json.RawMessage(queryInput))...)
	chunks = append(chunks,
		chattest.AnthropicChunk{
			Type:  "content_block_start",
			Index: 1,
			ContentBlock: chattest.AnthropicContentBlock{
				Type:      "web_search_tool_result",
				ToolUseID: toolCallID,
				Content:   resultContent,
			},
		},
		chattest.AnthropicChunk{Type: "content_block_stop", Index: 1},
		chattest.AnthropicChunk{
			Type:  "content_block_start",
			Index: 2,
			ContentBlock: chattest.AnthropicContentBlock{
				Type: "text",
			},
		},
		chattest.AnthropicChunk{
			Type:  "content_block_delta",
			Index: 2,
			Delta: chattest.AnthropicDeltaBlock{
				Type: "text_delta",
				Text: text,
			},
		},
		chattest.AnthropicChunk{Type: "content_block_stop", Index: 2},
		chattest.AnthropicChunk{
			Type:       "message_delta",
			StopReason: stopReason,
			Usage:      chattest.AnthropicUsage{InputTokens: 10, OutputTokens: 5},
		},
		chattest.AnthropicChunk{Type: "message_stop"},
	)
	return chunks
}

func toolPartExists(parts []codersdk.ChatMessagePart, toolName string) bool {
	for _, part := range parts {
		if (part.Type == codersdk.ChatMessagePartTypeToolCall || part.Type == codersdk.ChatMessagePartTypeToolResult) &&
			part.ToolName == toolName {
			return true
		}
	}
	return false
}

func updateChatModelCompressionThreshold(t *testing.T, db database.Store, model database.ChatModelConfig, contextLimit int64, threshold int32) database.ChatModelConfig {
	t.Helper()
	model.ContextLimit = contextLimit
	model.CompressionThreshold = threshold
	updated, err := db.UpdateChatModelConfig(context.Background(), database.UpdateChatModelConfigParams{
		ID:                   model.ID,
		DisplayName:          model.DisplayName,
		Model:                model.Model,
		Enabled:              model.Enabled,
		ContextLimit:         model.ContextLimit,
		CompressionThreshold: model.CompressionThreshold,
		Options:              model.Options,
		AIProviderID:         model.AIProviderID,
	})
	require.NoError(t, err)
	return updated
}

func updateChatModelContextLimit(t *testing.T, db database.Store, model database.ChatModelConfig) database.ChatModelConfig {
	t.Helper()
	updated, err := db.UpdateChatModelConfig(context.Background(), database.UpdateChatModelConfigParams{
		ID:                   model.ID,
		DisplayName:          model.DisplayName,
		Model:                model.Model,
		Enabled:              model.Enabled,
		ContextLimit:         model.ContextLimit,
		CompressionThreshold: model.CompressionThreshold,
		Options:              model.Options,
		AIProviderID:         model.AIProviderID,
	})
	require.NoError(t, err)
	return updated
}

func updateChatModelCallConfig(t *testing.T, db database.Store, model database.ChatModelConfig, callConfig codersdk.ChatModelCallConfig) database.ChatModelConfig {
	t.Helper()
	options, err := json.Marshal(callConfig)
	require.NoError(t, err)
	updated, err := db.UpdateChatModelConfig(context.Background(), database.UpdateChatModelConfigParams{
		ID:                   model.ID,
		DisplayName:          model.DisplayName,
		Model:                model.Model,
		Enabled:              model.Enabled,
		ContextLimit:         model.ContextLimit,
		CompressionThreshold: model.CompressionThreshold,
		Options:              options,
		AIProviderID:         model.AIProviderID,
	})
	require.NoError(t, err)
	return updated
}

func insertAssistantTextMessage(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	text string,
	modelID uuid.UUID,
) {
	t.Helper()
	insertChatMessageParts(ctx, t, db, chatID, database.ChatMessageRoleAssistant, modelID, uuid.Nil, []codersdk.ChatMessagePart{
		codersdk.ChatMessageText(text),
	})
}

func insertProviderToolPairMessageWithLocalTool(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	modelID uuid.UUID,
	toolCallID string,
) {
	t.Helper()
	metadata, err := json.Marshal(fantasy.ProviderMetadata{
		fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{
			Results: []fantasyanthropic.WebSearchResultItem{{
				URL:              "https://example.com",
				Title:            "Example",
				EncryptedContent: "encrypted",
			}},
		},
	})
	require.NoError(t, err)
	parts := []codersdk.ChatMessagePart{
		{
			Type:             codersdk.ChatMessagePartTypeToolCall,
			ToolCallID:       toolCallID,
			ToolName:         "web_search",
			Args:             json.RawMessage(`{"query":"coder"}`),
			ProviderExecuted: true,
		},
		{
			Type:             codersdk.ChatMessagePartTypeToolResult,
			ToolCallID:       toolCallID,
			ToolName:         "web_search",
			Result:           json.RawMessage(`"ok"`),
			ProviderExecuted: true,
			ProviderMetadata: metadata,
		},
	}
	parts = append(parts, codersdk.ChatMessagePart{
		Type:       codersdk.ChatMessagePartTypeToolCall,
		ToolCallID: "tc-1",
		ToolName:   "read_file",
		Args:       json.RawMessage(`{"path":"main.go"}`),
	})
	insertChatMessageParts(ctx, t, db, chatID, database.ChatMessageRoleAssistant, modelID, uuid.Nil, parts)
	insertChatMessageParts(ctx, t, db, chatID, database.ChatMessageRoleTool, modelID, uuid.Nil, []codersdk.ChatMessagePart{
		{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "tc-1",
			ToolName:   "read_file",
			Result:     json.RawMessage(`"file"`),
		},
	})
}

func insertChatMessageParts(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	role database.ChatMessageRole,
	modelID uuid.UUID,
	createdBy uuid.UUID,
	parts []codersdk.ChatMessagePart,
) database.ChatMessage {
	t.Helper()
	content, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	params := chatd.BuildSingleChatMessageInsertParams(
		chatID,
		role,
		content,
		database.ChatMessageVisibilityBoth,
		modelID,
		chatprompt.CurrentContentVersion,
		createdBy,
	)
	messages, err := db.InsertChatMessages(ctx, params)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	return messages[0]
}

func createPlanSubagentChatWithHistory(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	orgID uuid.UUID,
	userID uuid.UUID,
	modelID uuid.UUID,
) database.Chat {
	t.Helper()
	rootChat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           userID,
		LastModelConfigID: modelID,
		Title:             "plan subagent active tools root",
		Status:            database.ChatStatusWaiting,
		PlanMode:          database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		MCPServerIDs:      []uuid.UUID{},
		ClientType:        database.ChatClientTypeApi,
	})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           userID,
		LastModelConfigID: modelID,
		Title:             "plan subagent active tools",
		Status:            database.ChatStatusWaiting,
		PlanMode:          database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		ParentChatID:      uuid.NullUUID{UUID: rootChat.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: rootChat.ID, Valid: true},
		MCPServerIDs:      []uuid.UUID{},
		ClientType:        database.ChatClientTypeApi,
	})
	insertSystemTextMessage(ctx, t, db, chat.ID, "You are not currently connected to a workspace.", modelID)
	insertChatMessageParts(ctx, t, db, chat.ID, database.ChatMessageRoleUser, modelID, userID, []codersdk.ChatMessagePart{
		codersdk.ChatMessageText("hello"),
	})
	return chat
}

func anthropicRequestToolNames(req chattest.AnthropicRequest) []string {
	names := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func anthropicRequestContainsPromptSentinel(t *testing.T, req chattest.AnthropicRequest) bool {
	t.Helper()
	body := anthropicRequestBody(t, req)
	return strings.Contains(body, "__chatd_agent_prompt_sentinel_")
}

func reasoningPartsFromMessage(t *testing.T, msg database.ChatMessage) []codersdk.ChatMessagePart {
	t.Helper()
	parts, err := chatprompt.ParseContent(msg)
	require.NoError(t, err)
	var reasoning []codersdk.ChatMessagePart
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeReasoning {
			reasoning = append(reasoning, part)
		}
	}
	return reasoning
}

func validWebSearchProviderMetadataForTest() fantasy.ProviderMetadata {
	return fantasy.ProviderMetadata{
		fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{
			Results: []fantasyanthropic.WebSearchResultItem{
				{
					URL:              "https://example.com",
					Title:            "Example",
					EncryptedContent: "encrypted",
				},
			},
		},
	}
}

func safeToolCallPart(part fantasy.MessagePart) (fantasy.ToolCallPart, bool) {
	var zero fantasy.ToolCallPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolCallPart); ok && value == nil {
		return zero, false
	}
	type toolCallPart = fantasy.ToolCallPart
	return fantasy.AsMessagePart[toolCallPart](part)
}

func safeToolResultPart(part fantasy.MessagePart) (fantasy.ToolResultPart, bool) {
	var zero fantasy.ToolResultPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolResultPart); ok && value == nil {
		return zero, false
	}
	type toolResultPart = fantasy.ToolResultPart
	return fantasy.AsMessagePart[toolResultPart](part)
}

func requireProviderExecutedToolCallPrompt(
	t *testing.T,
	prompt []fantasy.Message,
	id string,
) fantasy.ToolCallPart {
	t.Helper()
	for _, message := range prompt {
		for _, part := range message.Content {
			toolCall, ok := safeToolCallPart(part)
			if ok && toolCall.ProviderExecuted && toolCall.ToolCallID == id {
				return toolCall
			}
		}
	}
	t.Fatalf("missing provider-executed prompt tool call %q", id)
	return fantasy.ToolCallPart{}
}

func requireProviderExecutedToolResultPrompt(
	t *testing.T,
	prompt []fantasy.Message,
	id string,
) fantasy.ToolResultPart {
	t.Helper()
	for _, message := range prompt {
		for _, part := range message.Content {
			toolResult, ok := safeToolResultPart(part)
			if ok && toolResult.ProviderExecuted && toolResult.ToolCallID == id {
				return toolResult
			}
		}
	}
	t.Fatalf("missing provider-executed prompt tool result %q", id)
	return fantasy.ToolResultPart{}
}

func requireNoProviderExecutedToolCallPrompt(t *testing.T, prompt []fantasy.Message) {
	t.Helper()
	for i, message := range prompt {
		for j, part := range message.Content {
			toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
			if ok && toolCall.ProviderExecuted {
				t.Fatalf("prompt[%d].content[%d]: unexpected provider-executed call", i, j)
			}
		}
	}
}

func requireNoProviderExecutedToolResultPrompt(t *testing.T, prompt []fantasy.Message) {
	t.Helper()
	for i, message := range prompt {
		for j, part := range message.Content {
			toolResult, ok := safeToolResultPart(part)
			if ok && toolResult.ProviderExecuted {
				t.Fatalf("prompt[%d].content[%d]: unexpected provider-executed result", i, j)
			}
		}
	}
}

func requireTextPrompt(t *testing.T, prompt []fantasy.Message, text string) fantasy.TextPart {
	t.Helper()
	for _, message := range prompt {
		for _, part := range message.Content {
			textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
			if ok && textPart.Text == text {
				return textPart
			}
		}
	}
	t.Fatalf("missing prompt text %q", text)
	return fantasy.TextPart{}
}

func requireAnthropicProviderToolPromptSafe(t *testing.T, prompt []fantasy.Message) {
	t.Helper()
	require.Empty(t, chatsanitize.ValidateAnthropicProviderToolHistory(prompt))
}

func requireLogField(t *testing.T, entry slog.SinkEntry, name string) any {
	t.Helper()
	for _, field := range entry.Fields {
		if field.Name == name {
			return field.Value
		}
	}
	t.Fatalf("missing log field %q", name)
	return nil
}

func TestPassiveServerDoesNotProcess(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user, org, model := seedChatDependencies(t, db)

	server := newTestServer(t, db, ps, uuid.New())
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "should-stay-pending",
		InitialUserContent: []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeText, Text: "hello"}},
		ModelConfigID:      model.ID,
	})
	require.NoError(t, err)

	chatd.WaitUntilIdleForTest(server)

	// Re-read from DB to catch any unexpected processing.
	stored, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, stored.Status)
	require.False(t, stored.WorkerID.Valid)
	require.False(t, stored.RunnerID.Valid)
}

// newDebugEnabledTestServer creates a passive test server with
// AlwaysEnableDebugLogs=true so that IsEnabled(ctx, chatID, ownerID)
// always returns true regardless of runtime admin config. This lets
// chatd-level integration tests exercise the debug cleanup wiring
// without seeding the admin/user opt-in settings tables.
func newDebugEnabledTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	replicaID uuid.UUID,
) *chatd.Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  replicaID,
		PendingChatAcquireInterval: testutil.WaitLong,
		AlwaysEnableDebugLogs:      true,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

// newActiveTestServer creates a chatd server that actively polls for
// and processes pending chats. Use this instead of newTestServer when
// the test needs the chat loop to actually run. Optional config
// overrides are applied after the defaults.
func newActiveTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	overrides ...func(*chatd.Config),
) *chatd.Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	cfg := chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		Experiments:                codersdk.ExperimentsKnown,
	}
	for _, o := range overrides {
		o(&cfg)
	}
	server := chatd.New(ps, cfg)
	server.Start()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

// sinkFieldValue returns the value of the named field from a captured log
// entry.
func sinkFieldValue(fields slog.Map, name string) (any, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f.Value, true
		}
	}
	return nil, false
}

// TestActiveServer_GenerationErrorLogged drives a full chat worker against a
// provider that returns a terminal error and asserts that chatd logs the
// unsanitized failure so an administrator can later diagnose the underlying
// reason, even though the user-facing message is sanitized.
func TestActiveServer_GenerationErrorLogged(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	sink := testutil.NewFakeSink(t)

	const providerErrMessage = "synthetic provider failure for logging test"
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		// A 400 is non-retryable, so the worker fails the turn immediately
		// instead of entering retry backoff.
		return chattest.OpenAIErrorResponse(http.StatusBadRequest, "invalid_request_error", providerErrMessage)
	})
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.Logger = sink.Logger()
	})

	chat := createChatThroughServer(ctx, t, db, server, org.ID, user.ID, model.ID, "hello")
	failed := waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusError)
	require.True(t, failed.LastError.Valid)

	isGenerationFailure := func(e slog.SinkEntry) bool {
		return e.Level == slog.LevelWarn && e.Message == "chat generation failed"
	}
	var entry slog.SinkEntry
	testutil.Eventually(ctx, t, func(context.Context) bool {
		entries := sink.Entries(isGenerationFailure)
		if len(entries) == 0 {
			return false
		}
		entry = entries[0]
		return true
	}, testutil.IntervalFast)

	chatID, ok := sinkFieldValue(entry.Fields, "chat_id")
	require.True(t, ok, "chat_id field present")
	require.Equal(t, chat.ID, chatID)

	provider, ok := sinkFieldValue(entry.Fields, "provider")
	require.True(t, ok, "provider field present")
	require.Equal(t, "openai", provider)

	statusCode, ok := sinkFieldValue(entry.Fields, "status_code")
	require.True(t, ok, "status_code field present")
	require.Equal(t, http.StatusBadRequest, statusCode)

	// The unsanitized cause must be logged so administrators can see the
	// underlying provider reason, even though the persisted user-facing
	// message omits it.
	errValue, ok := sinkFieldValue(entry.Fields, "error")
	require.True(t, ok, "error field present")
	require.Contains(t, fmt.Sprintf("%v", errValue), providerErrMessage)
	require.NotContains(t, chatLastErrorMessage(failed.LastError), providerErrMessage)
}

func TestProposeChatTitle_DebugRun(t *testing.T) {
	t.Parallel()

	wantTitle := "Debug proposal title"
	tests := []struct {
		name                    string
		alwaysEnableDebugLogs   bool
		response                func() chattest.OpenAIResponse
		wantErr                 bool
		wantTitle               string
		wantTitleGenerationRuns int
		wantDebugStatus         codersdk.ChatDebugStatus
	}{
		{
			name:                  "Enabled",
			alwaysEnableDebugLogs: true,
			response: func() chattest.OpenAIResponse {
				return chattest.OpenAINonStreamingResponse(
					"{\"title\":\"" + wantTitle + "\"}",
				)
			},
			wantTitle:               wantTitle,
			wantTitleGenerationRuns: 1,
			wantDebugStatus:         codersdk.ChatDebugStatusCompleted,
		},
		{
			name:                  "Disabled",
			alwaysEnableDebugLogs: false,
			response: func() chattest.OpenAIResponse {
				return chattest.OpenAINonStreamingResponse(
					"{\"title\":\"" + wantTitle + "\"}",
				)
			},
			wantTitle: wantTitle,
		},
		{
			name:                  "GenerationErrorFinalizesDebugRun",
			alwaysEnableDebugLogs: true,
			response: func() chattest.OpenAIResponse {
				return chattest.OpenAINonStreamingResponse("not json")
			},
			wantErr:                 true,
			wantTitleGenerationRuns: 1,
			wantDebugStatus:         codersdk.ChatDebugStatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			db, ps, rawDB := dbtestutil.NewDBWithSQLDB(t)
			openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				require.False(t, req.Stream)
				return tt.response()
			})
			user, org, model := seedChatDependenciesWithProvider(
				t,
				db,
				"openai",
				openAIURL,
			)
			server := chatd.New(ps, chatd.Config{
				Logger:                     slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				Database:                   db,
				ReplicaID:                  uuid.New(),
				PendingChatAcquireInterval: testutil.WaitLong,
				AlwaysEnableDebugLogs:      tt.alwaysEnableDebugLogs,
				AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
			})
			t.Cleanup(func() {
				require.NoError(t, server.Close())
			})

			chat := dbgen.Chat(t, db, database.Chat{
				OrganizationID:    org.ID,
				Status:            database.ChatStatusWaiting,
				ClientType:        database.ChatClientTypeUi,
				OwnerID:           user.ID,
				Title:             "original title",
				LastModelConfigID: model.ID,
			})
			message := insertUserTextMessage(
				t,
				db,
				chat.ID,
				user.ID,
				model.ID,
				"summarize debug title generation",
				model.ContextLimit,
			)
			require.NotEqual(t, uuid.Nil, message.ID)

			gotTitle, err := server.ProposeChatTitle(ctx, chat)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantTitle, gotTitle)
			}

			runs, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
				ChatID:   chat.ID,
				LimitVal: 100,
			})
			require.NoError(t, err)
			require.Len(t, runs, tt.wantTitleGenerationRuns)
			if tt.wantTitleGenerationRuns > 0 {
				require.Equal(t, string(codersdk.ChatDebugRunKindTitleGeneration), runs[0].Kind)
				require.Equal(t, string(tt.wantDebugStatus), runs[0].Status)
				require.True(t, runs[0].Provider.Valid)
				require.Equal(t, "openai", runs[0].Provider.String)
				require.True(t, runs[0].FinishedAt.Valid)
				require.True(t, runs[0].HistoryTipMessageID.Valid)
				require.Equal(t, message.ID, runs[0].HistoryTipMessageID.Int64)
			}
			if !tt.wantErr {
				// Title generation must not write accounting rows to
				// chat_messages; usage is tracked by AI Gateway.
				var usageMessages int
				err = rawDB.QueryRowContext(
					ctx,
					`SELECT count(*) FROM chat_messages WHERE chat_id = $1 AND visibility = 'model' AND deleted = true`,
					chat.ID,
				).Scan(&usageMessages)
				require.NoError(t, err)
				require.Equal(t, 0, usageMessages)
			}
		})
	}
}

func seedChatDependencies(
	t *testing.T,
	db database.Store,
) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()
	openAIURL := chattest.OpenAI(t)
	return seedChatDependenciesWithProvider(t, db, "openai", openAIURL)
}

// seedChatDependenciesWithProvider creates a user, organization,
// chat provider, and model config for the given provider type and
// base URL.
func seedChatDependenciesWithProvider(
	t *testing.T,
	db database.Store,
	provider string,
	baseURL string,
) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	providerConfig := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    provider,
		DisplayName: provider,
		BaseUrl:     baseURL,
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: providerConfig.ID, Valid: true},
		IsDefault:    true,
	})
	return user, org, model
}

func seedChatDependenciesWithProviderPolicy(
	t *testing.T,
	db database.Store,
	provider string,
	baseURL string,
	apiKey string,
	centralAPIKeyEnabled bool,
	allowUserAPIKey bool,
	allowCentralAPIKeyFallback bool,
) (database.User, database.Organization, database.ChatProvider, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	providerConfig := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    provider,
		DisplayName: provider,
		BaseUrl:     baseURL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:     true,
	}, func(p *database.InsertChatProviderParams) {
		p.APIKey = apiKey
		p.CentralApiKeyEnabled = centralAPIKeyEnabled
		p.AllowUserApiKey = allowUserAPIKey
		p.AllowCentralApiKeyFallback = allowCentralAPIKeyFallback
	})

	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: providerConfig.ID, Valid: true},
		IsDefault:    true,
	})

	return user, org, providerConfig, model
}

func seedLastTurnSummary(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chat database.Chat,
	summary string,
) {
	t.Helper()

	affected, err := db.UpdateChatLastTurnSummary(ctx, database.UpdateChatLastTurnSummaryParams{
		ID:                     chat.ID,
		ExpectedHistoryVersion: chat.HistoryVersion,
		LastTurnSummary:        sql.NullString{String: summary, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
}

func waitForTerminalChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
) database.Chat {
	t.Helper()

	var chatResult database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		got, err := db.GetChatByID(ctx, chatID)
		if err != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.IntervalFast)

	return chatResult
}

func insertChatModelConfigWithCallConfig(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	provider string,
	model string,
	callConfig codersdk.ChatModelCallConfig,
) database.ChatModelConfig {
	t.Helper()

	options, err := json.Marshal(callConfig)
	require.NoError(t, err)

	// Reuse the newest AI provider of this type (creating a bare one only when
	// none exists) so the config links the seeded provider carrying the mock
	// base URL and API key rather than a fresh credential-less one.
	providers, err := db.GetAIProviders(context.Background(), database.GetAIProvidersParams{IncludeDisabled: true})
	require.NoError(t, err)
	var aiProvider database.AIProvider
	for _, candidate := range providers {
		if candidate.Type != database.AIProviderType(provider) {
			continue
		}
		if aiProvider.ID == uuid.Nil || candidate.CreatedAt.After(aiProvider.CreatedAt) {
			aiProvider = candidate
		}
	}
	if aiProvider.ID == uuid.Nil {
		aiProvider = dbgen.AIProvider(t, db, database.AIProvider{
			Type: database.AIProviderType(provider),
		})
	}
	return dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: aiProvider.ID, Valid: true},
		Model:        model,
		DisplayName:  model,
		CreatedBy:    uuid.NullUUID{UUID: userID, Valid: true},
		UpdatedBy:    uuid.NullUUID{UUID: userID, Valid: true},
		Options:      options,
	})
}

func insertUserTextMessage(
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	userID uuid.UUID,
	modelConfigID uuid.UUID,
	text string,
	contextLimit ...int64,
) database.ChatMessage {
	t.Helper()
	require.LessOrEqual(t, len(contextLimit), 1)

	contextLimitValue := int64(0)
	if len(contextLimit) == 1 {
		contextLimitValue = contextLimit[0]
	}
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)

	return dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:        chatID,
		CreatedBy:     uuid.NullUUID{UUID: userID, Valid: true},
		ModelConfigID: uuid.NullUUID{UUID: modelConfigID, Valid: true},
		Role:          database.ChatMessageRoleUser,
		Content:       pqtype.NullRawMessage{RawMessage: content.RawMessage, Valid: true},
		ContextLimit:  sql.NullInt64{Int64: contextLimitValue, Valid: contextLimitValue != 0},
	})
}

// seedWorkspaceWithAgent creates a full workspace chain with a connected
// agent. This is the common setup needed by tests that exercise tool
// execution against a workspace.
func seedWorkspaceWithAgent(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) (database.WorkspaceTable, database.WorkspaceAgent) {
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
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        userID,
		OrganizationID: org.ID,
	})
	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		InitiatorID:    userID,
		OrganizationID: org.ID,
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       ws.ID,
		JobID:             pj.ID,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	dbAgent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID:      res.ID,
		Directory:       "/home/coder/project",
		OperatingSystem: "linux",
	})
	require.NoError(t, db.UpdateWorkspaceAgentStartupByID(context.Background(), database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                dbAgent.ID,
		Version:           "v1.0.0",
		ExpandedDirectory: "/home/coder/project",
	}))
	dbAgent, err := db.GetWorkspaceAgentByID(context.Background(), dbAgent.ID)
	require.NoError(t, err)
	return ws, dbAgent
}

func setOpenAIProviderBaseURL(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	baseURL string,
) {
	t.Helper()

	providers, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{IncludeDisabled: true})
	require.NoError(t, err)
	for _, provider := range providers {
		if provider.Type != database.AIProviderTypeOpenai {
			continue
		}
		_, err = db.UpdateAIProvider(ctx, database.UpdateAIProviderParams{
			ID:            provider.ID,
			Type:          provider.Type,
			DisplayName:   provider.DisplayName,
			Icon:          provider.Icon,
			Enabled:       provider.Enabled,
			BaseUrl:       baseURL,
			Settings:      provider.Settings,
			SettingsKeyID: provider.SettingsKeyID,
		})
		require.NoError(t, err)
		return
	}
	require.Fail(t, "openai provider not found")
}

func TestInterruptChatDoesNotSendWebPushNotification(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Set up a mock OpenAI that blocks until the request context is
	// canceled (i.e. until the chat is interrupted).
	streamStarted := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunks := make(chan chattest.OpenAIChunk, 1)
		go func() {
			defer close(chunks)
			chunks <- chattest.OpenAITextChunks("partial")[0]
			select {
			case <-streamStarted:
			default:
				close(streamStarted)
			}
			// Block until the chat context is canceled by the interrupt.
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	})

	// Mock webpush dispatcher that records calls.
	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-no-push",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	seedLastTurnSummary(ctx, t, db, chat, "previous summary")

	server.Start()

	// Wait for the chat to be picked up and start streaming.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning && fromDB.WorkerID.Valid
	}, testutil.IntervalFast)

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case <-streamStarted:
			return true
		default:
			return false
		}
	}, testutil.IntervalFast)

	// Interrupt the chat. The worker finalizes the interruption asynchronously.
	updated, _ := server.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusInterrupting, updated.Status)

	// Wait for the chat to finish processing and return to waiting.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusWaiting && !fromDB.WorkerID.Valid
	}, testutil.IntervalFast)
	chatd.WaitUntilIdleForTest(server)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, fromDB.LastTurnSummary.Valid,
		"interrupted chats should clear cached turn summaries")

	// Verify no web push notification was dispatched.
	require.Equal(t, int32(0), mockPush.dispatchCount.Load(),
		"expected no web push dispatch for an interrupted chat")
}

// mockWebpushDispatcher implements webpush.Dispatcher and records Dispatch calls.
type mockWebpushDispatcher struct {
	dispatchCount atomic.Int32
	mu            sync.Mutex
	lastMessage   codersdk.WebpushMessage
	lastUserID    uuid.UUID
}

func (m *mockWebpushDispatcher) Dispatch(_ context.Context, userID uuid.UUID, msg codersdk.WebpushMessage) error {
	m.dispatchCount.Add(1)
	m.mu.Lock()
	m.lastMessage = msg
	m.lastUserID = userID
	m.mu.Unlock()
	return nil
}

func (m *mockWebpushDispatcher) getLastMessage() codersdk.WebpushMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastMessage
}

func (*mockWebpushDispatcher) Test(_ context.Context, _ codersdk.WebpushSubscription) error {
	return nil
}

func (*mockWebpushDispatcher) PublicKey() string {
	return "test-vapid-public-key"
}

func TestSuccessfulChatSendsWebPushWithNavigationData(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Set up a mock OpenAI that returns a simple successful response.
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	// Mock webpush dispatcher that captures the dispatched message.
	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
	})
	server.Start()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "push-nav-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the chat to complete and return to waiting status.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusWaiting && !fromDB.WorkerID.Valid && mockPush.dispatchCount.Load() == 1
	}, testutil.IntervalFast)

	// Verify a web push notification was dispatched exactly once.
	require.Equal(t, int32(1), mockPush.dispatchCount.Load(),
		"expected exactly one web push dispatch for a completed chat")

	// Verify the notification was sent to the correct user.
	mockPush.mu.Lock()
	capturedMsg := mockPush.lastMessage
	capturedUserID := mockPush.lastUserID
	mockPush.mu.Unlock()

	require.Equal(t, user.ID, capturedUserID,
		"web push should be dispatched to the chat owner")

	// Verify the Data field contains the correct navigation URL.
	expectedURL := fmt.Sprintf("/agents/%s", chat.ID)
	require.Equal(t, expectedURL, capturedMsg.Data["url"],
		"web push Data should contain the chat navigation URL")
}

func TestCloseDuringShutdownContextCanceledShouldRetryOnNewReplica(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var requestCount atomic.Int32
	streamStarted := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		// Ignore non-streaming requests (e.g. title generation) so
		// they don't interfere with the request counter used to
		// coordinate the streaming chat flow.
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("shutdown-retry")
		}
		if requestCount.Add(1) == 1 {
			chunks := make(chan chattest.OpenAIChunk, 1)
			go func() {
				defer close(chunks)
				chunks <- chattest.OpenAITextChunks("partial")[0]
				select {
				case <-streamStarted:
				default:
					close(streamStarted)
				}
				<-req.Context().Done()
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("retry", " complete")...)
	})

	loggerA := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	serverA := chatd.New(ps, chatd.Config{
		Logger:                     loggerA,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(factory),
	})
	serverA.Start()
	t.Cleanup(func() {
		require.NoError(t, serverA.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := serverA.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "shutdown-retry",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning && fromDB.WorkerID.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.Eventually(t, func() bool {
		select {
		case <-streamStarted:
			return true
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.NoError(t, serverA.Close())

	loggerB := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	serverB := chatd.New(ps, chatd.Config{
		Logger:                     loggerB,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(factory),
	})
	serverB.Start()
	t.Cleanup(func() {
		require.NoError(t, serverB.Close())
	})

	require.Eventually(t, func() bool {
		return requestCount.Load() >= 2
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.Eventually(t, func() bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusWaiting &&
			!fromDB.WorkerID.Valid &&
			!fromDB.LastError.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestSuccessfulChatSendsWebPushWithSummary(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const assistantText = "I have completed the task successfully and all tests are passing now."
	const summaryText = "Finished unit tests"

	var nonStreamingRequests atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			if strings.Contains(string(req.RawBody), "propose_turn_status_label") {
				nonStreamingRequests.Add(1)
				return chattest.OpenAINonStreamingResponse(fmt.Sprintf(`{"label":%q}`, summaryText))
			}
			return chattest.OpenAINonStreamingResponse(`{"title":"Summary push test"}`)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks(assistantText)...,
		)
	})

	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
	})
	server.Start()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "summary-push-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do the thing")},
	})
	require.NoError(t, err)

	// The push notification is dispatched asynchronously after the
	// chat finishes, so we poll for it rather than checking
	// immediately after the status transitions to waiting.
	var fromDB database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var dbErr error
		fromDB, dbErr = db.GetChatByID(ctx, chat.ID)
		return dbErr == nil && mockPush.dispatchCount.Load() >= 1 && fromDB.LastTurnSummary.Valid
	}, testutil.IntervalFast)

	msg := mockPush.getLastMessage()
	require.Equal(t, summaryText, fromDB.LastTurnSummary.String,
		"last turn summary should be the LLM-generated status label")
	require.Equal(t, fromDB.LastTurnSummary.String, msg.Body,
		"push body should reuse the persisted generated status label")
	require.Equal(t, int32(1), nonStreamingRequests.Load(),
		"expected exactly one non-streaming request for status label generation")
}

func TestSuccessfulChatPersistsTurnSummaryWithoutWebPush(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const assistantText = "I fixed the bug and added regression coverage."
	const summaryText = "Fixed regression bug"

	var nonStreamingRequests atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			if strings.Contains(string(req.RawBody), "propose_turn_status_label") {
				nonStreamingRequests.Add(1)
				return chattest.OpenAINonStreamingResponse(fmt.Sprintf(`{"label":%q}`, summaryText))
			}
			return chattest.OpenAINonStreamingResponse(`{"title":"Summary push test"}`)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks(assistantText)...,
		)
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "summary-no-webpush-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do the thing")},
	})
	require.NoError(t, err)

	var fromDB database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var dbErr error
		fromDB, dbErr = db.GetChatByID(ctx, chat.ID)
		return dbErr == nil && fromDB.LastTurnSummary.Valid
	}, testutil.IntervalFast)

	require.Equal(t, summaryText, fromDB.LastTurnSummary.String,
		"status label should persist even when web push is unavailable")
	require.Equal(t, int32(1), nonStreamingRequests.Load(),
		"expected exactly one non-streaming request for status label generation")
}

func TestSuccessfulChatSendsWebPushFallbackWithoutSummaryForEmptyAssistantText(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var nonStreamingRequests atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			if strings.Contains(string(req.RawBody), "propose_turn_status_label") {
				nonStreamingRequests.Add(1)
				return chattest.OpenAINonStreamingResponse(`{"label":"Unexpected label"}`)
			}
			return chattest.OpenAINonStreamingResponse(`{"title":"Empty summary push test"}`)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("   ")...,
		)
	})

	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "empty-summary-push-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do the thing")},
	})
	require.NoError(t, err)
	seedLastTurnSummary(ctx, t, db, chat, "previous summary")

	server.Start()

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return mockPush.dispatchCount.Load() >= 1
	}, testutil.IntervalFast)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullString{String: "Finished latest turn", Valid: true}, fromDB.LastTurnSummary,
		"fallback status label should be persisted")

	msg := mockPush.getLastMessage()
	require.Equal(t, "Finished latest turn", msg.Body,
		"push body should fall back when the final assistant text is empty")
	require.Equal(t, int32(0), nonStreamingRequests.Load(),
		"status label model should not run when final assistant text has no usable text")
}

func TestErroredChatClearsLastTurnSummaryAndSendsWebPush(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		return chattest.OpenAIErrorResponse(http.StatusBadRequest, "invalid_request_error", "Bad request")
	})

	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "error-summary-clear-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do the thing")},
	})
	require.NoError(t, err)
	seedLastTurnSummary(ctx, t, db, chat, "previous summary")

	server.Start()

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		return dbErr == nil &&
			fromDB.Status == database.ChatStatusError &&
			mockPush.dispatchCount.Load() >= 1
	}, testutil.IntervalFast)
	chatd.WaitUntilIdleForTest(server)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, fromDB.LastTurnSummary.Valid,
		"errored chats should clear cached turn summaries")

	msg := mockPush.getLastMessage()
	require.NotEqual(t, "Hit an error", msg.Body)
	require.Contains(t, msg.Body, "OpenAI returned an unexpected error")
}

func TestComputerUseSubagentToolsAndModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	computerUseModelProvider, computerUseModelName, ok := chattool.DefaultComputerUseModel(codersdk.ChatComputerUseProviderAnthropic)
	require.True(t, ok)
	require.EqualValues(t, codersdk.ChatComputerUseProviderAnthropic, computerUseModelProvider)

	// Track tools and model from the Anthropic LLM calls (the
	// computer use child chat). We use a raw HTTP handler because
	// the chattest AnthropicRequest struct does not capture tools.
	type anthropicCall struct {
		Model  string
		Tools  []string
		Stream bool
	}
	var anthropicMu sync.Mutex
	var anthropicCalls []anthropicCall

	anthropicSrv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var req struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
				Tools  []struct {
					Name string `json:"name"`
				} `json:"tools"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			names := make([]string, len(req.Tools))
			for i, tool := range req.Tools {
				names[i] = tool.Name
			}
			anthropicMu.Lock()
			anthropicCalls = append(anthropicCalls, anthropicCall{
				Model:  req.Model,
				Tools:  names,
				Stream: req.Stream,
			})
			anthropicMu.Unlock()

			if !req.Stream {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg-test",
					"type":        "message",
					"role":        "assistant",
					"model":       computerUseModelName,
					"content":     []map[string]any{{"type": "text", "text": "Done."}},
					"stop_reason": "end_turn",
					"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
				})
				return
			}

			// Stream a minimal Anthropic SSE response.
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, _ := w.(http.Flusher)

			chunks := []map[string]any{
				{
					"type": "message_start",
					"message": map[string]any{
						"id":    "msg-test",
						"type":  "message",
						"role":  "assistant",
						"model": computerUseModelName,
					},
				},
				{
					"type":  "content_block_start",
					"index": 0,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				},
				{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]any{
						"type": "text_delta",
						"text": "Done.",
					},
				},
				{"type": "content_block_stop", "index": 0},
				{
					"type":  "message_delta",
					"delta": map[string]any{"stop_reason": "end_turn"},
					"usage": map[string]any{"output_tokens": 5},
				},
				{"type": "message_stop"},
			}

			for _, chunk := range chunks {
				chunkBytes, _ := json.Marshal(chunk)
				eventType, _ := chunk["type"].(string)
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n",
					eventType, chunkBytes)
				flusher.Flush()
			}
		},
	))
	t.Cleanup(anthropicSrv.Close)

	// OpenAI mock for the root chat. The first streaming call
	// triggers spawn_agent; subsequent calls reply
	// with text.
	var openAICallCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if openAICallCount.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"spawn_agent",
					`{"type":"computer_use","prompt":"do the desktop thing","title":"cu-sub"}`,
				),
			)
		}
		// Include literal \u0000 in the response text, which is
		// what a real LLM writes when explaining binary output.
		// json.Marshal encodes the backslash as \\, producing
		// \\u0000 in the JSON bytes. The sanitizer must not
		// corrupt this into invalid JSON.
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("The file contains \\u0000 null bytes.")...,
		)
	})

	// Seed the DB: user, openai-compat provider, model config.
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	// Add an Anthropic provider pointing to our mock server.
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "anthropic",
		DisplayName: "Anthropic",
		APIKey:      "test-anthropic-key",
		BaseUrl:     anthropicSrv.URL,
	})

	// Build workspace + agent records so getWorkspaceConn can
	// resolve the agent for the computer use child.
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	// Mock agent connection that returns valid display dimensions
	// for the initial screenshot check in the computer use path.
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().
		ExecuteDesktopAction(gomock.Any(), gomock.Any()).
		Return(workspacesdk.DesktopActionResponse{
			ScreenshotWidth:  1920,
			ScreenshotHeight: 1080,
			ScreenshotData:   "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==",
		}, nil).
		AnyTimes()
	mockConn.EXPECT().
		SetExtraHeaders(gomock.Any()).
		AnyTimes()
	mockConn.EXPECT().
		ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).
		AnyTimes()
	mockConn.EXPECT().
		LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, xerrors.New("not found")).
		AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		providers, providersErr := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
		require.NoError(t, providersErr)
		routes := make(map[string]aibridge.TransportFactory, len(providers))
		for _, provider := range providers {
			switch provider.Type {
			case database.AIProviderTypeOpenaiCompat:
				routes[provider.Name] = chattest.NewMockAIBridgeTransport(t, openAIURL)
			case database.AIProviderTypeAnthropic:
				routes[provider.Name] = chattest.NewMockAIBridgeTransport(t, anthropicSrv.URL)
			}
		}
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(providerRoutedTransportFactory{routes: routes})
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	// Create a root chat with a workspace so the child inherits it.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "computer-use-detection",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Use the desktop to check the UI"),
		},
	})
	require.NoError(t, err)

	// Wait for the root chat AND the computer use child to finish.
	// The root chat spawns the child, then the chatd server picks
	// up and runs the child (which hits the Anthropic mock).
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != database.ChatStatusWaiting &&
			got.Status != database.ChatStatusError {
			return false
		}
		// Ensure the Anthropic mock received the child streaming call.
		anthropicMu.Lock()
		defer anthropicMu.Unlock()
		for _, call := range anthropicCalls {
			if call.Stream {
				return true
			}
		}
		return false
	}, testutil.WaitLong, testutil.IntervalFast)

	anthropicMu.Lock()
	calls := append([]anthropicCall(nil), anthropicCalls...)
	anthropicMu.Unlock()

	require.NotEmpty(t, calls,
		"expected at least one Anthropic LLM call")

	var childCall anthropicCall
	for _, call := range calls {
		if call.Stream {
			childCall = call
			break
		}
	}
	require.True(t, childCall.Stream,
		"expected at least one streaming Anthropic child LLM call")

	childModel := childCall.Model
	childTools := childCall.Tools

	// 1. Verify the model is the computer use model.
	require.Equal(t, computerUseModelName, childModel,
		"computer use subagent should use %s",
		computerUseModelName)

	// 2. Verify the computer tool is present.
	require.Contains(t, childTools, "computer",
		"computer use subagent should have the computer tool")

	// 3. Verify standard workspace tools are present (the same
	//    set a regular subagent gets).
	standardTools := []string{
		"read_file", "write_file", "edit_files", "execute",
		"process_output", "process_list", "process_signal",
	}
	for _, tool := range standardTools {
		require.Contains(t, childTools, tool,
			"computer use subagent should have standard tool %q",
			tool)
	}

	// 4. Verify workspace provisioning tools are NOT present.
	workspaceProvisioningTools := []string{
		"list_templates", "read_template",
		"create_workspace", "start_workspace", "stop_workspace",
	}
	for _, tool := range workspaceProvisioningTools {
		require.NotContains(t, childTools, tool,
			"computer use subagent should NOT have workspace "+
				"provisioning tool %q", tool)
	}

	// 5. Verify subagent tools are NOT present.
	subagentTools := []string{
		"spawn_agent",
		"wait_agent", "message_agent", "interrupt_agent", "list_agents",
	}
	for _, tool := range subagentTools {
		require.NotContains(t, childTools, tool,
			"computer use subagent should NOT have subagent "+
				"tool %q", tool)
	}

	// 6. Verify the child chat has Mode = computer_use in
	//    the DB.
	childRows, err := db.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{
		ParentIds: []uuid.UUID{chat.ID},
	})
	require.NoError(t, err)
	children := make([]database.Chat, 0, len(childRows))
	for _, row := range childRows {
		children = append(children, row.Chat)
	}
	require.Len(t, children, 1)
	require.True(t, children[0].Mode.Valid)
	require.Equal(t, database.ChatModeComputerUse,
		children[0].Mode.ChatMode)
}

func TestInterruptChatPersistsPartialResponse(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Set up a mock OpenAI that streams a partial response and then
	// blocks until the request context is canceled (simulating an
	// interrupt mid-stream).
	chunksDelivered := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		chunks := make(chan chattest.OpenAIChunk, 1)
		go func() {
			defer close(chunks)
			// Send two partial text chunks so there is meaningful
			// content to persist.
			for _, c := range chattest.OpenAITextChunks("hello world") {
				chunks <- c
			}
			// Signal that chunks have been written to the HTTP response.
			select {
			case <-chunksDelivered:
			default:
				close(chunksDelivered)
			}
			// Block until interrupt cancels the context.
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(ps, chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		Experiments:                codersdk.ExperimentsKnown,
		AIBridgeTransportFactory:   chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL)),
	})
	server.Start()
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-persist-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the mock to finish sending chunks.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case <-chunksDelivered:
			return true
		default:
			return false
		}
	}, testutil.IntervalFast)

	// Now interrupt the chat. The provider has sent partial content.
	updated, _ := server.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusInterrupting, updated.Status)

	// Wait for the partial assistant message to be persisted.
	// After the interrupt, the chatloop runs persistInterruptedStep
	// which inserts the message and publishes a "message" event.
	// We poll the DB directly for the assistant message rather than
	// relying on the chat status (which transitions to "waiting"
	// before the persist completes).
	var assistantMsg *database.ChatMessage
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		msgs, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for i := range msgs {
			if msgs[i].Role == database.ChatMessageRoleAssistant {
				assistantMsg = &msgs[i]
				return true
			}
		}
		return false
	}, testutil.IntervalFast)
	require.NotNilf(t, assistantMsg, "expected a persisted assistant message after interrupt")

	// Parse the content and verify it contains the partial text.
	parts, err := chatprompt.ParseContent(*assistantMsg)
	require.NoError(t, err)

	var foundText string
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			foundText += part.Text
		}
	}
	require.Contains(t, foundText, "hello world",
		"partial assistant response should contain the streamed text")
}

func TestProcessChat_UserProviderKey_Success(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const userAPIKey = "user-test-key"

	var authHeadersMu sync.Mutex
	authHeaders := make([]string, 0, 1)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		authHeadersMu.Lock()
		authHeaders = append(authHeaders, req.Header.Get("Authorization"))
		authHeadersMu.Unlock()

		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("user provider key success")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("hello from the saved user key")...,
		)
	})

	user, org, provider, model := seedChatDependenciesWithProviderPolicy(
		t,
		db,
		"openai-compat",
		openAIURL,
		"",
		false,
		true,
		false,
	)
	_, err := db.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
		ID:           uuid.New(),
		UserID:       user.ID,
		AIProviderID: provider.ID,
		APIKey:       userAPIKey,
	})
	require.NoError(t, err)

	creator := newTestServer(t, db, ps, uuid.New())
	chat, err := creator.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "user-provider-key-success",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("say hello"),
		},
	})
	require.NoError(t, err)

	_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)
	require.False(t, chatResult.LastError.Valid)

	authHeadersMu.Lock()
	recordedAuthHeaders := append([]string(nil), authHeaders...)
	authHeadersMu.Unlock()
	require.Contains(t, recordedAuthHeaders, "Bearer "+userAPIKey)
}

func seedAIGatewayOpenAITestDependencies(
	t *testing.T,
	db database.Store,
	openAIURL string,
) (database.User, database.Organization, database.AIProvider, database.ChatModelConfig) {
	t.Helper()

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	provider := dbgen.AIProvider(t, db, database.AIProvider{
		Type:    database.AIProviderTypeOpenai,
		Name:    "primary-openai-" + uuid.NewString(),
		BaseUrl: openAIURL,
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		IsDefault:    true,
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true},
	})
	_, err := db.UpsertUserAIProviderKey(context.Background(), database.UpsertUserAIProviderKeyParams{
		ID:           uuid.New(),
		UserID:       user.ID,
		AIProviderID: provider.ID,
		APIKey:       "sk-user-aibridge",
	})
	require.NoError(t, err)

	return user, org, provider, model
}

func TestProcessChat_RoutingUsesDelegatedAPIKey(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if req.Stream {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("hello through AI Gateway")...,
			)
		}
		return chattest.OpenAINonStreamingResponse(`{"title":"AI Gateway Chat"}`)
	})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)

	user, org, provider, model := seedAIGatewayOpenAITestDependencies(t, db, openAIURL)

	creator := newTestServer(t, db, ps, uuid.New())
	chat, err := creator.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "aigateway-routing",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("say hello"),
		},
	})
	require.NoError(t, err)

	_, events, cancel, ok := creator.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.AllowBYOK = true
		cfg.AllowBYOKSet = true
	})

	_ = events

	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)
	require.False(t, chatResult.LastError.Valid)
	gatewayKey, err := db.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
		UserID:    user.ID,
		TokenName: chatd.GatewayTokenName(user.ID),
	})
	require.NoError(t, err)

	requests := factory.RequestsSnapshot()
	require.NotEmpty(t, requests)
	require.True(t, slices.ContainsFunc(requests, func(req chattest.RecordedRequest) bool {
		return req.Request.URL.Path == "/v1/responses"
	}), "no request to /v1/responses found")
	for _, req := range requests {
		require.Equal(t, provider.Name, req.ProviderName)
		require.Equal(t, aibridge.SourceAgents, req.Source)
		require.Equal(t, gatewayKey.ID, req.APIKeyID)
		require.Equal(t, "Bearer sk-user-aibridge", req.Request.Header.Get("Authorization"))
		require.Empty(t, req.Request.Header.Get("X-Api-Key"))
		require.Equal(t, "delegated", req.Request.Header.Get(aibridge.HeaderCoderToken))
		require.True(t, strings.HasPrefix(req.Request.URL.Path, "/v1/"), "unexpected aibridge path %q", req.Request.URL.Path)
	}
}

func TestProcessChat_RoutingPreservesAPIKeyAfterWorkspaceContext(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if req.Stream {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("hello after workspace context")...,
			)
		}
		return chattest.OpenAINonStreamingResponse(`{"title":"AI Gateway Workspace"}`)
	})
	factory := chattest.NewMockAIBridgeTransport(t, openAIURL)
	user, org, provider, model := seedAIGatewayOpenAITestDependencies(t, db, openAIURL)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	creator := newTestServer(t, db, ps, uuid.New())
	chat, err := creator.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "aigateway-workspace-context",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("use the workspace context"),
		},
	})
	require.NoError(t, err)

	const contextText = "# Project instructions\nAlways keep routing metadata."
	// Workspace context is sourced from the agent's pinned snapshot. Seed it so
	// the chat hydrates it on the lazy first-turn bind and the workspace-context
	// path runs before AI gateway routing resolves the key.
	seedAgentInstructionContext(ctx, t, db, dbAgent.ID,
		"/home/coder/project/AGENTS.md", contextText)
	_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(factory)
		cfg.AllowBYOK = true
		cfg.AllowBYOKSet = true
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			setupWorkspaceContextAgentConn(t, mockConn, dbAgent, contextText, nil)
			return mockConn, func() {}, nil
		}
	})

	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)
	require.False(t, chatResult.LastError.Valid)

	// Workspace context is pinned to the chat, not injected as a user message.
	// Confirm the agent's pushed instruction hydrated onto the chat so the
	// workspace-context path ran before AI gateway routing resolved the key.
	pinned, err := db.ListChatContextResourcesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.NotEmpty(t, pinned, "workspace context should be pinned to the chat")
	gatewayKey, err := db.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
		UserID:    user.ID,
		TokenName: chatd.GatewayTokenName(user.ID),
	})
	require.NoError(t, err)

	requests := factory.RequestsSnapshot()
	require.NotEmpty(t, requests)
	for _, req := range requests {
		require.Equal(t, provider.Name, req.ProviderName)
		require.Equal(t, aibridge.SourceAgents, req.Source)
		require.Equal(t, gatewayKey.ID, req.APIKeyID)
		require.Equal(t, "Bearer sk-user-aibridge", req.Request.Header.Get("Authorization"))
		require.Equal(t, "delegated", req.Request.Header.Get(aibridge.HeaderCoderToken))
	}
}

func TestProcessChat_UserProviderKey_MissingKeyError(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var llmCalls atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		llmCalls.Add(1)
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("unexpected non-streaming request")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("unexpected streaming request")...,
		)
	})

	user, org, _, model := seedChatDependenciesWithProviderPolicy(
		t,
		db,
		"openai-compat",
		openAIURL,
		"",
		false,
		true,
		false,
	)

	creator := newTestServer(t, db, ps, uuid.New())
	chat, err := creator.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "user-provider-key-missing",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("say hello"),
		},
	})
	require.NoError(t, err)

	_ = newActiveTestServer(t, db, ps)

	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusError, chatResult.Status)
	persistedError := requireChatLastErrorPayload(t, chatResult.LastError)
	require.NotEmpty(t, persistedError.Message)
	require.NotContains(t, persistedError.Message, "panicked")
	require.Equal(t, codersdk.ChatErrorKindGeneric, persistedError.Kind)
	require.NotEqual(t, database.ChatStatusRunning, chatResult.Status)
	require.Zero(t, llmCalls.Load(), "missing user key should fail before any LLM request")
}

func TestProcessChatPanicRecovery(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	// Wrap the database so we can trigger a panic on the main
	// goroutine of processChat. The chatloop's executeTools has
	// its own recover, so panicking inside a tool goroutine won't
	// reach the processChat-level recovery. Instead, we panic
	// during PersistStep's InTx call, which runs synchronously on
	// the processChat goroutine.
	panicWrapper := &panicOnInTxDB{Store: db}

	firstOpenAICallStarted := make(chan struct{})
	continueFirstOpenAICall := make(chan struct{})
	var openAICallCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Panic recovery test")
		}

		if openAICallCount.Add(1) == 1 {
			close(firstOpenAICallStarted)
			<-continueFirstOpenAICall
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("hello")...,
		)
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	// Pass the panic wrapper to the server, but use the real
	// database for seeding so those operations don't panic.
	server := newActiveTestServer(t, panicWrapper, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "panic-recovery",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hello"),
		},
	})
	require.NoError(t, err)

	testutil.TryReceive(ctx, t, firstOpenAICallStarted)

	// Enable the panic while the first provider call is blocked. The next InTx
	// call is PersistStep inside the chatloop, running synchronously on the
	// processChat goroutine after the provider returns.
	panicWrapper.enablePanic()
	close(continueFirstOpenAICall)

	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting
	}, testutil.WaitLong, testutil.IntervalFast)
	require.Equal(t, int32(2), openAICallCount.Load())

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)

	var assistantText string
	for _, message := range messages {
		if message.Role != database.ChatMessageRoleAssistant {
			continue
		}
		parts, parseErr := chatprompt.ParseContent(message)
		require.NoError(t, parseErr)
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeText {
				assistantText += part.Text
			}
		}
	}
	require.Equal(t, "hello", assistantText)
	require.False(t, chatResult.LastError.Valid)
}

// panicOnInTxDB wraps a database.Store and panics on the first InTx
// call after enablePanic is called. Subsequent calls pass through
// so the processChat cleanup defer can update the chat status.
type panicOnInTxDB struct {
	database.Store
	active   atomic.Bool
	panicked atomic.Bool
}

func (d *panicOnInTxDB) enablePanic() { d.active.Store(true) }

func (d *panicOnInTxDB) InTx(f func(database.Store) error, opts *database.TxOptions) error {
	if d.active.Load() && !d.panicked.Load() {
		d.panicked.Store(true)
		panic("intentional test panic")
	}
	return d.Store.InTx(f, opts)
}

// TestMCPServerToolInvocation verifies that when a chat has
// mcp_server_ids set, the chat loop connects to those MCP servers,
// discovers their tools, and the LLM can invoke them.
//
// NOTE: This test uses a raw database.Store (no dbauthz wrapper).
// The chatd RBAC authorization of GetMCPServerConfigsByIDs (which
// requires ActionRead on ResourceDeploymentConfig) is covered by
// the chatd role definition tests, not here.
func TestMCPServerToolInvocation(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Start a real MCP server that exposes an "echo" tool.
	mcpSrv := mcpserver.NewMCPServer("test-mcp", "1.0.0")
	mcpSrv.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv)
	mcpTS := httptest.NewServer(mcpHTTP)
	t.Cleanup(mcpTS.Close)

	// Track which tool names are sent to the LLM and capture
	// whether the MCP tool result appears in the second call.
	var (
		callCount      atomic.Int32
		llmToolNames   []string
		llmToolsMu     sync.Mutex
		foundMCPResult atomic.Bool
	)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		// Record tool names from the first streamed call.
		if callCount.Add(1) == 1 {
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			llmToolsMu.Lock()
			llmToolNames = names
			llmToolsMu.Unlock()

			// Ask the LLM to call the MCP echo tool.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"test-mcp__echo",
					`{"input":"hello from LLM"}`,
				),
			)
		}

		// Second call: verify the tool result was fed back.
		for _, msg := range req.Messages {
			if msg.Role == "tool" && strings.Contains(msg.Content, "echo: hello from LLM") {
				foundMCPResult.Store(true)
			}
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Got it!")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	// Seed the MCP server config in the database. This must
	// happen after seedChatDependencies so user.ID exists for
	// the foreign key.
	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName: "Test MCP",
		Slug:        "test-mcp",
		Url:         mcpTS.URL,
		CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "mcp-tool-test",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		MCPServerIDs:   []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Echo something via MCP."),
		},
	})
	require.NoError(t, err)

	// Verify MCPServerIDs were persisted on the chat record.
	dbChat, getErr := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, getErr)
	require.Equal(t, []uuid.UUID{mcpConfig.ID}, dbChat.MCPServerIDs)

	// Wait for the chat to finish processing.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// The MCP tool (test-mcp__echo) should appear in the tool
	// list sent to the LLM.
	llmToolsMu.Lock()
	recordedNames := append([]string(nil), llmToolNames...)
	llmToolsMu.Unlock()
	require.Contains(t, recordedNames, "test-mcp__echo",
		"MCP tool should be in the tool list sent to the LLM")

	// The tool result from the MCP server ("echo: hello from
	// LLM") should have been fed back to the LLM as a tool
	// message in the second call.
	require.True(t, foundMCPResult.Load(),
		"MCP tool result should appear in the second LLM call")

	// Verify the tool result was persisted in the database.
	var foundToolMessage bool
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleTool {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil || len(parts) == 0 {
				continue
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult &&
					part.ToolName == "test-mcp__echo" &&
					strings.Contains(string(part.Result), "echo: hello from LLM") {
					foundToolMessage = true
					return true
				}
			}
		}
		return false
	}, testutil.IntervalFast)
	require.True(t, foundToolMessage,
		"MCP tool result should be persisted as a tool message in the database")
}

func TestPlanModeRootChatApprovedExternalMCPToolInvocation(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	mcpSrv := mcpserver.NewMCPServer("plan-mode-mcp", "1.0.0")
	mcpSrv.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	mcpTS := httptest.NewServer(mcpserver.NewStreamableHTTPServer(mcpSrv))
	t.Cleanup(mcpTS.Close)

	var (
		callCount      atomic.Int32
		llmToolNames   []string
		llmToolsMu     sync.Mutex
		foundMCPResult atomic.Bool
	)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		if callCount.Add(1) == 1 {
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			llmToolsMu.Lock()
			llmToolNames = names
			llmToolsMu.Unlock()

			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"plan-mode-mcp__echo",
					`{"input":"hello from root plan mode"}`,
				),
			)
		}

		for _, msg := range req.Messages {
			if msg.Role == "tool" && strings.Contains(msg.Content, "echo: hello from root plan mode") {
				foundMCPResult.Store(true)
			}
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Planning complete.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName:     "Plan Mode MCP",
		Slug:            "plan-mode-mcp",
		Url:             mcpTS.URL,
		AllowInPlanMode: true,
		CreatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "plan-mode-mcp-invocation",
		ModelConfigID:  model.ID,
		PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		MCPServerIDs:   []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Use the approved MCP tool while planning."),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)

	chatResult, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)

	llmToolsMu.Lock()
	recordedNames := append([]string(nil), llmToolNames...)
	llmToolsMu.Unlock()
	require.Contains(t, recordedNames, "plan-mode-mcp__echo",
		"approved external MCP tools should be available in root plan mode")
	require.True(t, foundMCPResult.Load(),
		"approved external MCP tool results should feed back into the follow-up plan-mode turn")
}

func TestPlanModeRootChatApprovedExternalMCPWorkflowCanReachProposePlan(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	mcpSrv := mcpserver.NewMCPServer("plan-workflow-mcp", "1.0.0")
	mcpSrv.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	mcpTS := httptest.NewServer(mcpserver.NewStreamableHTTPServer(mcpSrv))
	t.Cleanup(mcpTS.Close)

	var (
		callCount          atomic.Int32
		llmToolNames       []string
		llmToolsMu         sync.Mutex
		sawMCPResult       atomic.Bool
		proposePlanReached atomic.Bool
	)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		switch callCount.Add(1) {
		case 1:
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			llmToolsMu.Lock()
			llmToolNames = names
			llmToolsMu.Unlock()
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"plan-workflow-mcp__echo",
					`{"input":"prepare the plan"}`,
				),
			)
		case 2:
			for _, msg := range req.Messages {
				if msg.Role == "tool" && strings.Contains(msg.Content, "echo: prepare the plan") {
					sawMCPResult.Store(true)
				}
			}
			proposePlanReached.Store(true)
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("propose_plan", `{}`),
			)
		default:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("should not continue")...,
			)
		}
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName:     "Plan Workflow MCP",
		Slug:            "plan-workflow-mcp",
		Url:             mcpTS.URL,
		AllowInPlanMode: true,
		CreatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:       uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, path string, _, _ int64) (io.ReadCloser, string, error) {
			if strings.HasSuffix(path, ".md") {
				return io.NopCloser(strings.NewReader("# Plan\n- Use the approved MCP tool findings.\n")), "", nil
			}
			return io.NopCloser(strings.NewReader("")), "", nil
		}).AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "plan-mode-mcp-propose-plan",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		MCPServerIDs:   []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Use the approved MCP tool, then propose the plan."),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)

	chatResult, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)

	llmToolsMu.Lock()
	recordedNames := append([]string(nil), llmToolNames...)
	llmToolsMu.Unlock()
	require.Contains(t, recordedNames, "plan-workflow-mcp__echo",
		"approved external MCP tools should be available in the root plan-mode workflow")
	require.True(t, sawMCPResult.Load(),
		"the root plan-mode workflow should feed the approved MCP result into the propose_plan turn")
	require.True(t, proposePlanReached.Load(),
		"the root plan-mode workflow should reach propose_plan after using the approved MCP tool")
	require.Equal(t, int32(2), callCount.Load(),
		"the workflow should stop immediately after propose_plan succeeds")

	var foundProposePlanResult bool
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleTool {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil {
				continue
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == "propose_plan" {
					foundProposePlanResult = true
					return true
				}
			}
		}
		return false
	}, testutil.IntervalFast)
	require.True(t, foundProposePlanResult,
		"the root plan-mode workflow should persist a propose_plan tool result")
}

// TestMCPServerOAuth2TokenRefresh verifies that when a chat uses an
// MCP server with OAuth2 auth and the stored access token is expired,
// chatd refreshes the token using the stored refresh_token before
// connecting. The refreshed token is persisted to the database and
// the MCP tool call succeeds.
func TestMCPServerOAuth2TokenRefresh(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// The "fresh" token that the mock OAuth2 server returns after
	// a successful refresh_token grant.
	freshAccessToken := "fresh-access-token-" + uuid.New().String()

	// Mock OAuth2 token endpoint that exchanges a refresh token
	// for a new access token.
	var refreshCalled atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalled.Add(1)

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		grantType := r.FormValue("grant_type")
		if grantType != "refresh_token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"unsupported_grant_type"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh-token"}`, freshAccessToken)
	}))
	t.Cleanup(tokenSrv.Close)

	// Start a real MCP server with an auth middleware that only
	// accepts the fresh access token. An expired token (or any
	// other value) gets a 401.
	mcpSrv := mcpserver.NewMCPServer("authed-mcp", "1.0.0")
	mcpSrv.AddTools(mcpserver.ServerTool{
		Tool: mcpgo.NewTool("echo",
			mcpgo.WithDescription("Echoes the input"),
			mcpgo.WithString("input",
				mcpgo.Description("The input string"),
				mcpgo.Required(),
			),
		),
		Handler: func(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcpgo.NewToolResultText("echo: " + input), nil
		},
	})
	mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv)
	// Wrap with auth check.
	authMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+freshAccessToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_token","error_description":"The access token is invalid or expired"}`))
			return
		}
		mcpHTTP.ServeHTTP(w, r)
	})
	mcpTS := httptest.NewServer(authMux)
	t.Cleanup(mcpTS.Close)

	// Track LLM interactions.
	var (
		callCount      atomic.Int32
		llmToolNames   []string
		llmToolsMu     sync.Mutex
		foundMCPResult atomic.Bool
	)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		if callCount.Add(1) == 1 {
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			llmToolsMu.Lock()
			llmToolNames = names
			llmToolsMu.Unlock()

			// Ask the LLM to call the MCP echo tool.
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(
					"authed-mcp__echo",
					`{"input":"hello via refreshed token"}`,
				),
			)
		}

		// Second call: verify the tool result was fed back.
		for _, msg := range req.Messages {
			if msg.Role == "tool" && strings.Contains(msg.Content, "echo: hello via refreshed token") {
				foundMCPResult.Store(true)
			}
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Done!")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	// Seed the MCP server config with OAuth2 auth pointing to our
	// mock token endpoint.
	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName:    "Authed MCP",
		Slug:           "authed-mcp",
		Url:            mcpTS.URL,
		AuthType:       "oauth2",
		OAuth2ClientID: "test-client-id",
		OAuth2TokenURL: tokenSrv.URL,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	// Seed an expired OAuth2 token with a valid refresh_token.
	_, err := db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: mcpConfig.ID,
		UserID:            user.ID,
		AccessToken:       "old-expired-access-token",
		RefreshToken:      "old-refresh-token",
		TokenType:         "Bearer",
		Expiry:            sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	})
	require.NoError(t, err)

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "oauth2-refresh-test",
		ModelConfigID:  model.ID,
		WorkspaceID:    uuid.NullUUID{UUID: ws.ID, Valid: true},
		MCPServerIDs:   []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Echo something via the authed MCP."),
		},
	})
	require.NoError(t, err)

	// Wait for the chat to finish processing.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// The token should have been refreshed.
	require.Greater(t, refreshCalled.Load(), int32(0),
		"OAuth2 token endpoint should have been called to refresh the expired token")

	// The MCP tool should appear in the tool list.
	llmToolsMu.Lock()
	recordedNames := append([]string(nil), llmToolNames...)
	llmToolsMu.Unlock()
	require.Contains(t, recordedNames, "authed-mcp__echo",
		"MCP tool should be in the tool list sent to the LLM")

	// The tool result should have been fed back to the LLM.
	require.True(t, foundMCPResult.Load(),
		"MCP tool result should appear in the second LLM call")

	// Verify the refreshed token was persisted to the database.
	dbToken, err := db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
		MCPServerConfigID: mcpConfig.ID,
		UserID:            user.ID,
	})
	require.NoError(t, err)
	require.Equal(t, freshAccessToken, dbToken.AccessToken,
		"refreshed access token should be persisted in the database")
	require.Equal(t, "rotated-refresh-token", dbToken.RefreshToken,
		"rotated refresh token should be persisted in the database")
}

// TestMCPServerOAuth2TokenRefreshFailureGraceful verifies that when
// the OAuth2 token endpoint is down, the chat still proceeds without
// the MCP server's tools. The expired token is preserved unchanged.
func TestMCPServerOAuth2TokenRefreshFailureGraceful(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Token endpoint that always returns an error.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"server_error","error_description":"token endpoint unavailable"}`))
	}))
	t.Cleanup(tokenSrv.Close)

	// The LLM just replies with text, no tool calls.
	var callCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		callCount.Add(1)
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("I responded without MCP tools.")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		DisplayName:    "Broken MCP",
		Slug:           "broken-mcp",
		Url:            "http://127.0.0.1:0/does-not-exist",
		AuthType:       "oauth2",
		OAuth2ClientID: "test-client-id",
		OAuth2TokenURL: tokenSrv.URL,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})
	_, err := db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: mcpConfig.ID,
		UserID:            user.ID,
		AccessToken:       "old-expired-token",
		RefreshToken:      "old-refresh-token",
		TokenType:         "Bearer",
		Expiry:            sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	})

	require.NoError(t, err)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "graceful-degradation-test",
		ModelConfigID:  model.ID,
		MCPServerIDs:   []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Hello, just reply."),
		},
	})
	require.NoError(t, err)

	// Chat should finish successfully despite the failed refresh.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat should not fail", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// The LLM should have been called at least once.
	require.Greater(t, callCount.Load(), int32(0),
		"LLM should be called even when MCP token refresh fails")

	// The original token should be unchanged in the database.
	dbToken, err := db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
		MCPServerConfigID: mcpConfig.ID,
		UserID:            user.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "old-expired-token", dbToken.AccessToken,
		"original token should be preserved when refresh fails")
}

func TestChatTemplateAllowlistEnforcement(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)

	// Declare templates before the handler so the closure can
	// reference their IDs when building tool-call arguments.
	var tplAllowed, tplBlocked database.Template

	// Set up a mock OpenAI server that chains tool calls:
	//  1. list_templates
	//  2. read_template  (blocked template, should fail)
	//  3. read_template  (allowed template, should succeed)
	//  4. create_workspace (blocked template, should fail)
	//  5. text response
	var callCount atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch callCount.Add(1) {
		case 1:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("list_templates", `{}`),
			)
		case 2:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("read_template",
					fmt.Sprintf(`{"template_id":%q}`, tplBlocked.ID.String())),
			)
		case 3:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("read_template",
					fmt.Sprintf(`{"template_id":%q}`, tplAllowed.ID.String())),
			)
		case 4:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("create_workspace",
					fmt.Sprintf(`{"template_id":%q}`, tplBlocked.ID.String())),
			)
		default:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Done testing.")...,
			)
		}
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)

	// Create two templates the user can see.
	tplAllowed = dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "allowed-template",
	})
	tplBlocked = dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "blocked-template",
	})

	// Set the allowlist to only tplAllowed.
	allowlistJSON, err := json.Marshal([]string{tplAllowed.ID.String()})
	require.NoError(t, err)
	err = db.UpsertChatTemplateAllowlist(dbauthz.AsSystemRestricted(ctx), string(allowlistJSON))
	require.NoError(t, err)

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		// Provide a CreateWorkspace function so the tool reaches
		// the allowlist check instead of bailing with "not
		// configured". If the allowlist is enforced correctly
		// this function will never be called.
		cfg.CreateWorkspace = func(
			_ context.Context,
			_ uuid.UUID,
			_ codersdk.CreateWorkspaceRequest,
		) (codersdk.Workspace, error) {
			t.Error("CreateWorkspace should not be called for a blocked template")
			return codersdk.Workspace{}, xerrors.New("unexpected call")
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "allowlist-test",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Test allowlist enforcement"),
		},
	})
	require.NoError(t, err)

	// Wait for the chat to finish processing.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	// Collect all tool results keyed by tool name. Each tool may
	// have been called more than once, so we store a slice.
	var toolResults map[string][]string
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		toolResults = map[string][]string{}
		messages, dbErr := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  chat.ID,
			AfterID: 0,
		})
		if dbErr != nil {
			return false
		}
		for _, msg := range messages {
			if msg.Role != database.ChatMessageRoleTool {
				continue
			}
			parts, parseErr := chatprompt.ParseContent(msg)
			if parseErr != nil {
				continue
			}
			for _, part := range parts {
				if part.Type == codersdk.ChatMessagePartTypeToolResult {
					toolResults[part.ToolName] = append(
						toolResults[part.ToolName], string(part.Result))
				}
			}
		}
		// We expect results from all four tool calls.
		return len(toolResults["list_templates"]) >= 1 &&
			len(toolResults["read_template"]) >= 2 &&
			len(toolResults["create_workspace"]) >= 1
	}, testutil.IntervalFast)

	// list_templates: only the allowed template should appear.
	require.Contains(t, toolResults["list_templates"][0], tplAllowed.ID.String(),
		"allowed template should appear in list_templates result")
	require.NotContains(t, toolResults["list_templates"][0], tplBlocked.ID.String(),
		"blocked template should NOT appear in list_templates result")

	// read_template: blocked ID → error, allowed ID → success.
	require.Contains(t, toolResults["read_template"][0], "not found",
		"read_template for blocked template should return not-found error")
	require.Contains(t, toolResults["read_template"][1], tplAllowed.ID.String(),
		"read_template for allowed template should return template details")

	// create_workspace: blocked ID → rejected.
	require.Contains(t, toolResults["create_workspace"][0], "not available",
		"create_workspace for blocked template should be rejected")
}

func TestChatAsksUserWhenListTemplatesRequiresSelection(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)

	var tplCode, tplDocker database.Template
	var callCount atomic.Int32
	var sawSelectionRule atomic.Bool
	var sawSelectionRequiredResult atomic.Bool

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		switch callCount.Add(1) {
		case 1:
			promptAndTools := string(req.RawBody)
			for _, message := range req.Messages {
				promptAndTools += "\n" + message.Content
			}
			if strings.Contains(promptAndTools, "follow its next_step") {
				sawSelectionRule.Store(true)
			}
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("list_templates", `{}`),
			)
		case 2:
			if listTemplatesResultRequiresUserSelection(req.Messages) {
				sawSelectionRequiredResult.Store(true)
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks(
						"I found two templates, typescript-alpha and Docker Containers. Which template should I use?",
					)...,
				)
			}

			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("create_workspace",
					fmt.Sprintf(`{"template_id":%q}`, tplCode.ID.String())),
			)
		default:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Done.")...,
			)
		}
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	tplCode = dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "code-2",
		DisplayName:    "typescript-alpha",
		Description:    "this is a long description",
	})
	tplDocker = dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "docker",
		DisplayName:    "Docker Containers",
		Description:    "Provision Docker containers as Coder workspaces",
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
		cfg.CreateWorkspace = func(
			context.Context,
			uuid.UUID,
			codersdk.CreateWorkspaceRequest,
		) (codersdk.Workspace, error) {
			t.Error("create_workspace should not be called when list_templates requires user selection")
			return codersdk.Workspace{}, xerrors.New("unexpected create_workspace call")
		}
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "ask-template-selection-test",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("Create a workspace."),
		},
	})
	require.NoError(t, err)

	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusWaiting || got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "chat run failed", "last_error=%q", chatLastErrorMessage(chatResult.LastError))
	}

	require.True(t, sawSelectionRule.Load(), "model request should include the next_step selection rule")
	require.True(t, sawSelectionRequiredResult.Load(), "model should receive a list_templates result requiring user selection")

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)

	var listTemplatesResult map[string]any
	var assistantText string
	var sawCreateWorkspaceResult bool
	for _, message := range messages {
		parts, parseErr := chatprompt.ParseContent(message)
		require.NoError(t, parseErr)
		for _, part := range parts {
			switch {
			case part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == "list_templates":
				require.NoError(t, json.Unmarshal(part.Result, &listTemplatesResult))
			case part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolName == "create_workspace":
				sawCreateWorkspaceResult = true
			case message.Role == database.ChatMessageRoleAssistant && part.Type == codersdk.ChatMessagePartTypeText:
				assistantText += part.Text
			}
		}
	}

	require.NotNil(t, listTemplatesResult, "expected list_templates tool result")
	require.Equal(t, chattool.NextStepAskUser, listTemplatesResult["next_step"])
	require.NotContains(t, listTemplatesResult, "recommended_template_id")
	require.Contains(t, listTemplatesResult["templates"], any(map[string]any{
		"id":           tplCode.ID.String(),
		"name":         "code-2",
		"display_name": "typescript-alpha",
		"description":  "this is a long description",
	}))
	require.Contains(t, listTemplatesResult["templates"], any(map[string]any{
		"id":           tplDocker.ID.String(),
		"name":         "docker",
		"display_name": "Docker Containers",
		"description":  "Provision Docker containers as Coder workspaces",
	}))
	require.False(t, sawCreateWorkspaceResult, "agent should ask instead of calling create_workspace")
	require.Contains(t, assistantText, "Which template should I use?")
}

func listTemplatesResultRequiresUserSelection(messages []chattest.OpenAIMessage) bool {
	for _, message := range messages {
		if message.Role != "tool" || !json.Valid([]byte(message.Content)) {
			continue
		}

		var result map[string]any
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		if result["next_step"] == chattool.NextStepAskUser {
			return true
		}
	}
	return false
}

// TestCreateChatImmediatelyProcessesNewChat verifies that CreateChat
// starts processing a new chat without waiting for the acquire ticker
// to fire. The ticker interval is set to an hour so it never fires
// during the test.
func TestCreateChatImmediatelyProcessesNewChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	processed := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		// Signal that the LLM was reached. This proves the chat
		// was acquired and processing started.
		select {
		case <-processed:
		default:
			close(processed)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("hello from the model")...,
		)
	})

	// Use a 1-hour acquire interval so the ticker never fires.
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.PendingChatAcquireInterval = time.Hour
		cfg.InFlightChatStaleAfter = testutil.WaitSuperLong
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	// CreateChat should start the first turn without waiting for the
	// acquire ticker.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "wake-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// The chat should be processed immediately. The LLM handler
	// closes the `processed` channel when it receives a streaming
	// request. If CreateChat only relied on the 1-hour ticker,
	// this receive would time out.
	testutil.TryReceive(ctx, t, processed)

	chatd.WaitUntilIdleForTest(server)

	// Verify the chat was fully processed.
	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status,
		"chat should be in waiting status after processing completes")
}

// TestSendMessageImmediatelyProcessesWaitingChat verifies that sending
// a follow-up message to a waiting chat starts the next turn without
// waiting for the acquire ticker.
func TestSendMessageImmediatelyProcessesWaitingChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	firstProcessed := make(chan struct{})
	var requestCount atomic.Int32
	secondProcessed := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		switch requestCount.Add(1) {
		case 1:
			select {
			case <-firstProcessed:
			default:
				close(firstProcessed)
			}
		case 2:
			close(secondProcessed)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("response")...,
		)
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.PendingChatAcquireInterval = time.Hour
		cfg.InFlightChatStaleAfter = testutil.WaitSuperLong
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	user, org, model := seedChatDependencies(t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	// CreateChat processes the first turn immediately.
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "wake-send-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("first")},
	})
	require.NoError(t, err)

	// Wait for the first turn to actually reach the LLM, then
	// wait for the processing goroutine to finish so the chat
	// transitions to "waiting" status.
	testutil.TryReceive(ctx, t, firstProcessed)
	chatd.WaitUntilIdleForTest(server)

	// Now send a follow-up message, which should also be
	// processed immediately without waiting for the acquire ticker.
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("second")},
	})
	require.NoError(t, err)

	testutil.TryReceive(ctx, t, secondProcessed)
	chatd.WaitUntilIdleForTest(server)

	// Both turns processed. Verify the second request reached the LLM.
	require.GreaterOrEqual(t, requestCount.Load(), int32(2),
		"LLM should have received at least 2 streaming requests")
}

// TestAgentContextFilesAndSkillsLoadedIntoChat verifies the full
// end-to-end path: the workspace agent reads instruction files and
// discovers skills from the filesystem, chatd fetches them via a
// real tailnet agent connection, and both the <workspace-context>
// block and <available-skills> index appear in the LLM prompt.
//
// This test is NOT parallel because it sets process-wide environment
// variables via t.Setenv to configure the agent's context config.
func TestAgentContextFilesAndSkillsLoadedIntoChat(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)

	instructionsDir := filepath.Join(fakeHome, ".coder")
	skillsDir := filepath.Join(fakeHome, ".coder", "skills")
	require.NoError(t, os.MkdirAll(instructionsDir, 0o755))
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))

	t.Setenv(agentcontextconfig.EnvInstructionsDirs, instructionsDir)
	t.Setenv(agentcontextconfig.EnvInstructionsFile, "AGENTS.md")
	t.Setenv(agentcontextconfig.EnvSkillsDirs, skillsDir)
	t.Setenv(agentcontextconfig.EnvSkillMetaFile, "SKILL.md")
	t.Setenv(agentcontextconfig.EnvMCPConfigFiles, filepath.Join(fakeHome, "nonexistent-mcp.json"))

	require.NoError(t, os.WriteFile(
		filepath.Join(instructionsDir, "AGENTS.md"),
		[]byte("# Project Rules\nAlways write tests."),
		0o600,
	))

	skillDir := filepath.Join(skillsDir, "my-cool-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: my-cool-skill\ndescription: A test skill\n---\nDo the cool thing.\n"),
		0o600,
	))

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:              coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon:      true,
		ChatdInstructionLookupTimeout: testutil.WaitLong,
	})
	db := api.Database
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)

	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ApplyComplete,
		ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	_ = agenttest.New(t, client.URL, agentToken, agenttest.WithContextConfigFromEnv())
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

	// Pinned context is the sole source of workspace context. The chat binds
	// its agent lazily on the first turn and re-pins from that agent's pushed
	// snapshot, so the snapshot must exist before the turn runs. Wait for the
	// agent to push its instruction file and skill before creating the chat;
	// otherwise the re-pin copies nothing and the prompt omits them.
	builtWorkspace, err := client.Workspace(ctx, workspace.ID)
	require.NoError(t, err)
	var agentID uuid.UUID
	for _, res := range builtWorkspace.LatestBuild.Resources {
		for _, agent := range res.Agents {
			agentID = agent.ID
		}
	}
	require.NotEqual(t, uuid.Nil, agentID, "workspace should expose an agent")
	require.Eventually(t, func() bool {
		pushed, lerr := db.ListWorkspaceAgentContextResources(
			dbauthz.AsSystemRestricted(ctx), agentID)
		return lerr == nil && len(pushed) >= 2
	}, testutil.WaitSuperLong, testutil.IntervalFast)

	// Capture LLM requests so we can inspect the system prompt.
	var streamedCallsMu sync.Mutex
	streamedCalls := make([][]chattest.OpenAIMessage, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("context test")
		}

		streamedCallsMu.Lock()
		streamedCalls = append(streamedCalls, append([]chattest.OpenAIMessage(nil), req.Messages...))
		streamedCallsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("Got it.")...,
		)
	})

	coderdtest.CreateOpenAICompatChatModelConfig(t, expClient, openAIURL)

	workspaceID := workspace.ID
	chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		WorkspaceID:    &workspaceID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Hello, what are the project rules?",
			},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := expClient.GetChat(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		return got.Status == codersdk.ChatStatusWaiting || got.Status == codersdk.ChatStatusError
	}, testutil.WaitSuperLong, testutil.IntervalFast)

	streamedCallsMu.Lock()
	recordedCalls := append([][]chattest.OpenAIMessage(nil), streamedCalls...)
	streamedCallsMu.Unlock()
	require.NotEmpty(t, recordedCalls, "LLM should have received at least one streaming request")

	var allSystemContent string
	for _, msg := range recordedCalls[0] {
		if msg.Role == "system" {
			allSystemContent += msg.Content + "\n"
		}
	}

	require.Contains(t, allSystemContent, "<workspace-context>",
		"system prompt should contain workspace-context block")
	require.Contains(t, allSystemContent, "Always write tests.",
		"system prompt should contain AGENTS.md content")
	require.Contains(t, allSystemContent, "AGENTS.md",
		"system prompt should reference the source file")

	planBlockCount := 0
	standalonePlanBlockCount := 0
	for _, msg := range recordedCalls[0] {
		if msg.Role != "system" {
			continue
		}
		planBlockCount += strings.Count(
			msg.Content,
			"<plan-file-path>\nYour plan file path for this chat is:",
		)
		trimmed := strings.TrimSpace(msg.Content)
		if strings.HasPrefix(trimmed, "<plan-file-path>") &&
			strings.HasSuffix(trimmed, "</plan-file-path>") {
			standalonePlanBlockCount++
		}
	}

	require.Contains(t, allSystemContent, "<available-skills>",
		"system prompt should contain available-skills block")
	require.Contains(t, allSystemContent, "my-cool-skill",
		"system prompt should list the discovered skill")
	require.Contains(t, allSystemContent, "A test skill",
		"system prompt should include the skill description")
	require.Contains(t, allSystemContent, "<plan-file-path>",
		"system prompt should contain the plan-file-path block")
	require.Contains(t, allSystemContent, "PLAN-"+chat.ID.String()+".md",
		"system prompt should use the chat-specific plan path")
	require.Contains(t, allSystemContent,
		"Do not use "+strings.TrimRight(fakeHome, "/")+"/PLAN.md.",
		"system prompt should warn against the home-root plan path")
	require.Equal(t, 1, planBlockCount,
		"system prompt should contain a single plan-file-path block")
	require.Zero(t, standalonePlanBlockCount,
		"plan-file-path block should be part of the main system prompt, not a standalone message")
}

// TestEditMessageWithModelConfigOverride verifies that callers can
// change the model when editing a previous user message. The
// replacement message must persist with the new model and the chat's
// LastModelConfigID must be advanced so the assistant turn that follows
// runs against the new selection.
func TestEditMessageWithModelConfigOverride(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, modelA := seedChatDependencies(t, db)
	modelB := insertChatModelConfigWithCallConfig(
		t,
		db,
		user.ID,
		"openai",
		"gpt-4o-mini-edit-"+uuid.NewString(),
		codersdk.ChatModelCallConfig{},
	)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "edit-with-model-override",
		ModelConfigID:      modelA.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
	})
	require.NoError(t, err)

	initial, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, initial, 1)
	require.Equal(t, modelA.ID, initial[0].ModelConfigID.UUID)

	result, err := replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: initial[0].ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
		ModelConfigID:   modelB.ID,
	})
	require.NoError(t, err)
	require.True(t, result.Message.ModelConfigID.Valid)
	require.Equal(t, modelB.ID, result.Message.ModelConfigID.UUID)

	storedChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, modelB.ID, storedChat.LastModelConfigID,
		"edit must update last_model_config_id so the assistant turn picks up the new model")
}

// TestEditMessagePreservesModelConfigByDefault verifies that omitting
// ModelConfigID on edit keeps the original message's model. This is the
// existing default for callers that only edit the text.
func TestEditMessagePreservesModelConfigByDefault(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, modelA := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "edit-preserves-model",
		ModelConfigID:      modelA.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
	})
	require.NoError(t, err)

	initial, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, initial, 1)

	result, err := replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: initial[0].ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.NoError(t, err)
	require.True(t, result.Message.ModelConfigID.Valid)
	require.Equal(t, modelA.ID, result.Message.ModelConfigID.UUID)

	storedChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, modelA.ID, storedChat.LastModelConfigID,
		"edit without model override must not change last_model_config_id")
}

func TestEditMessageReasoningEffort(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		requested *string
		want      string
	}{
		{name: "PreservesByDefault", want: "low"},
		{name: "Overrides", requested: ptr.Ref("high"), want: "high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, ps := dbtestutil.NewDB(t)
			replica := newTestServer(t, db, ps, uuid.New())

			ctx := testutil.Context(t, testutil.WaitLong)
			user, org, model := seedChatDependencies(t, db)

			chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
				OwnerID:            user.ID,
				OrganizationID:     org.ID,
				Title:              "edit-reasoning-effort",
				ModelConfigID:      model.ID,
				ReasoningEffort:    ptr.Ref("low"),
				InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
			})
			require.NoError(t, err)

			initial, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
				ChatID:  chat.ID,
				AfterID: 0,
			})
			require.NoError(t, err)
			require.Len(t, initial, 1)
			require.True(t, initial[0].ReasoningEffort.Valid)
			require.Equal(t, database.ChatReasoningEffortLow, initial[0].ReasoningEffort.ChatReasoningEffort)

			result, err := replica.EditMessage(ctx, chatd.EditMessageOptions{
				ChatID:          chat.ID,
				EditedMessageID: initial[0].ID,
				Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
				ReasoningEffort: tc.requested,
			})
			require.NoError(t, err)
			require.True(t, result.Message.ReasoningEffort.Valid)
			require.Equal(t, database.ChatReasoningEffort(tc.want), result.Message.ReasoningEffort.ChatReasoningEffort)

			storedChat, err := db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			require.True(t, storedChat.LastReasoningEffort.Valid)
			require.Equal(t, database.ChatReasoningEffort(tc.want), storedChat.LastReasoningEffort.ChatReasoningEffort)
		})
	}
}

// TestEditMessageRejectsUnknownModelConfig verifies the edit handler
// returns ErrInvalidModelConfigID when the requested model does not
// exist, mirroring SendMessage's validation.
func TestEditMessageRejectsUnknownModelConfig(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, modelA := seedChatDependencies(t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "edit-unknown-model",
		ModelConfigID:      modelA.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
	})
	require.NoError(t, err)

	initial, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, initial, 1)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: initial[0].ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
		ModelConfigID:   uuid.New(),
	})
	require.ErrorIs(t, err, chatd.ErrInvalidModelConfigID)

	// The edit must roll back: the original message should still be
	// present and the chat's LastModelConfigID unchanged.
	stillThere, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, stillThere, 1)
	require.Equal(t, initial[0].ID, stillThere[0].ID)

	storedChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, modelA.ID, storedChat.LastModelConfigID)
}

func TestPromoteQueuedPreservesReasoningEffort(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "promote-reasoning-effort",
		Status:            database.ChatStatusError,
	})
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")})
	require.NoError(t, err)
	queued, err := db.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:          chat.ID,
		Content:         content.RawMessage,
		ModelConfigID:   uuid.NullUUID{UUID: model.ID, Valid: true},
		ReasoningEffort: database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true},
		CreatedBy:       user.ID,
	})
	require.NoError(t, err)
	require.True(t, queued.ReasoningEffort.Valid)
	require.Equal(t, database.ChatReasoningEffortHigh, queued.ReasoningEffort.ChatReasoningEffort)

	result, err := replica.PromoteQueued(ctx, chatd.PromoteQueuedOptions{
		ChatID:          chat.ID,
		CreatedBy:       user.ID,
		QueuedMessageID: queued.ID,
	})
	require.NoError(t, err)
	require.True(t, result.PromotedMessage.ReasoningEffort.Valid)
	require.Equal(t, database.ChatReasoningEffortHigh, result.PromotedMessage.ReasoningEffort.ChatReasoningEffort)

	storedChat, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.True(t, storedChat.LastReasoningEffort.Valid)
	require.Equal(t, database.ChatReasoningEffortHigh, storedChat.LastReasoningEffort.ChatReasoningEffort)
}

// TestAdvisorGating_ExperimentDisabled verifies that the advisor tool is
// not attached when the chat-advisor experiment is absent from the
// experiments list, even if the DB-stored advisor config has Enabled=true.
func TestAdvisorGating_ExperimentDisabled(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	var firstCallTools []string

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		if streamedCallCount.Add(1) == 1 {
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			streamedCallsMu.Lock()
			firstCallTools = names
			streamedCallsMu.Unlock()
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   3,
		MaxOutputTokens: 16384,
	})
	experiments := slices.DeleteFunc(
		slices.Clone(codersdk.ExperimentsKnown),
		func(e codersdk.Experiment) bool { return e == codersdk.ExperimentChatAdvisor },
	)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.Experiments = experiments
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(
			chattest.NewMockAIBridgeTransport(t, openAIURL),
		)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "advisor-experiment-disabled",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("help me plan this"),
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != database.ChatStatusWaiting &&
			got.Status != database.ChatStatusError {
			return false
		}
		return streamedCallCount.Load() >= 1
	}, testutil.WaitLong, testutil.IntervalFast)

	streamedCallsMu.Lock()
	tools := append([]string(nil), firstCallTools...)
	streamedCallsMu.Unlock()

	require.NotEmpty(t, tools, "expected at least one streamed LLM request")
	require.NotContains(t, tools, "advisor",
		"advisor tool must not be registered when the chat-advisor experiment is absent")
}

func TestAdvisorGating_RootChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	var firstCallTools []string
	var firstCallMessages []chattest.OpenAIMessage
	var secondCallMessages []chattest.OpenAIMessage

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		switch streamedCallCount.Add(1) {
		case 1:
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			streamedCallsMu.Lock()
			firstCallTools = names
			firstCallMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
			streamedCallsMu.Unlock()

			advisorChunk := chattest.OpenAIToolCallChunk(
				"advisor",
				`{"question":"help me plan"}`,
			)
			readChunk := chattest.OpenAIToolCallChunk(
				"read_file",
				`{"path":"/tmp/test.txt"}`,
			)
			mergedChunk := advisorChunk
			readCall := readChunk.Choices[0].ToolCalls[0]
			readCall.Index = 1
			mergedChunk.Choices[0].ToolCalls = append(
				mergedChunk.Choices[0].ToolCalls,
				readCall,
			)
			return chattest.OpenAIStreamingResponse(mergedChunk)
		case 2:
			streamedCallsMu.Lock()
			secondCallMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
			streamedCallsMu.Unlock()
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   3,
		MaxOutputTokens: 16384,
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(
			chattest.NewMockAIBridgeTransport(t, openAIURL),
		)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "advisor-root",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("help me plan this"),
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != database.ChatStatusWaiting &&
			got.Status != database.ChatStatusError {
			return false
		}
		return streamedCallCount.Load() >= 2
	}, testutil.WaitLong, testutil.IntervalFast)

	streamedCallsMu.Lock()
	tools := append([]string(nil), firstCallTools...)
	messages := append([]chattest.OpenAIMessage(nil), firstCallMessages...)
	secondMessages := append([]chattest.OpenAIMessage(nil), secondCallMessages...)
	streamedCallsMu.Unlock()

	// Exactly two streamed LLM calls are expected: the first that
	// returned the mixed advisor + read_file batch, and the second
	// that received the exclusive-policy rejection. A third call
	// would indicate that either tool had slipped past the exclusive
	// policy; the >= 2 wait would have missed that regression.
	require.Equal(t, int32(2), streamedCallCount.Load(),
		"exclusive policy must block execution of both tools; no third call expected")
	require.NotEmpty(t, messages, "expected a first streamed LLM request")
	require.NotEmpty(t, secondMessages, "expected a second streamed LLM request")
	require.Contains(t, tools, "advisor",
		"advisor tool should be registered for root chats when enabled")

	var hasGuidance bool
	for _, msg := range messages {
		if strings.Contains(msg.Content, chatadvisor.ParentGuidanceBlock) {
			hasGuidance = true
			break
		}
	}
	require.True(t, hasGuidance,
		"root chat should contain advisor guidance in the prompt")

	var hasExclusiveAdvisorError bool
	var hasSkippedToolError bool
	for _, msg := range secondMessages {
		if strings.Contains(msg.Content, "advisor must be called alone") {
			hasExclusiveAdvisorError = true
		}
		if strings.Contains(msg.Content, "this tool was skipped because advisor must run alone") {
			hasSkippedToolError = true
		}
	}
	require.True(t, hasExclusiveAdvisorError,
		"mixed advisor batches should surface the exclusive advisor error")
	require.True(t, hasSkippedToolError,
		"mixed advisor batches should skip sibling tools with an explanatory error")
}

// TestAdvisorHappyPath_RootChat walks the advisor tool end-to-end:
// parent calls advisor alone, the nested advisor call produces text, and
// the structured result flows back into the parent conversation. The
// exclusive-policy test above only proves the rejection path; this test
// covers the glue from chatd wiring -> chatadvisor.Tool -> Runtime.Run ->
// nested model call -> structured result back to the outer model.
func TestAdvisorHappyPath_RootChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const advisorReply = "break the problem into smaller pieces first"
	advisorDeltas := []string{"break the problem ", "into smaller pieces first"}

	var (
		streamedCallCount atomic.Int32
		streamedCallsMu   sync.Mutex
		advisorCallSeen   atomic.Bool
		advisorMessages   []chattest.OpenAIMessage
		finalCallMessages []chattest.OpenAIMessage
	)

	// Declared before the OpenAI handler so the nested advisor stream can
	// gate its completion on the live collector below having observed the
	// streamed deltas.
	var (
		livePartsMu       sync.Mutex
		liveAdvisorDeltas []string
	)
	liveDeltasCaptured := func() bool {
		livePartsMu.Lock()
		defer livePartsMu.Unlock()
		return slices.Equal(advisorDeltas, liveAdvisorDeltas)
	}

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		switch streamedCallCount.Add(1) {
		case 1:
			// Parent turn 1: call advisor solo.
			chunk := chattest.OpenAIToolCallChunk(
				"advisor",
				`{"question":"how should I approach this refactor?"}`,
			)
			chunk.Choices[0].ToolCalls[0].ID = "advisor-happy-path-call"
			return chattest.OpenAIStreamingResponse(chunk)
		case 2:
			// Nested advisor turn. The nested call has no tools because
			// chatadvisor.RunAdvisor runs with MaxSteps=1 and no tool
			// set.
			require.Empty(t, req.Tools,
				"advisor's nested call must run without tools")
			streamedCallsMu.Lock()
			advisorMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
			streamedCallsMu.Unlock()
			advisorCallSeen.Store(true)
			// Stream the deltas, then hold the nested response open until
			// the live subscriber has captured them. Advisor deltas are
			// stream-only: they live in the generation attempt's message
			// part episode, and once the tool result is committed the
			// subscriber's stream loop targets the next episode and never
			// replays this one. Without the hold, a slow pubsub sync makes
			// the subscriber skip the episode entirely and the deltas are
			// lost, flaking the streaming assertions below.
			chunks := make(chan chattest.OpenAIChunk)
			go func() {
				defer close(chunks)
				for _, chunk := range chattest.OpenAITextChunks(advisorDeltas...) {
					chunks <- chunk
				}
				deadline := time.NewTimer(testutil.WaitLong)
				defer deadline.Stop()
				for !liveDeltasCaptured() {
					select {
					case <-deadline.C:
						// Give up and let the assertions below report the
						// failure instead of hanging the stream forever.
						return
					case <-time.After(testutil.IntervalFast):
					}
				}
			}()
			return chattest.OpenAIResponse{StreamingChunks: chunks}
		default:
			// Parent turn 2: observe the advisor tool result and close
			// out with a final text reply.
			streamedCallsMu.Lock()
			finalCallMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
			streamedCallsMu.Unlock()
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("acknowledged")...,
			)
		}
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   3,
		MaxOutputTokens: 16384,
	})
	server := newTestServer(t, db, ps, uuid.New(), func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(
			chattest.NewMockAIBridgeTransport(t, openAIURL),
		)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "advisor-happy-path",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("help me refactor this module"),
		},
	})
	require.NoError(t, err)

	// Advisor deltas are transient; a late subscriber misses them.
	_, liveEvents, cancelLive, ok := server.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	liveCollectorDone := make(chan struct{})
	go func() {
		defer close(liveCollectorDone)
		for {
			select {
			case <-ctx.Done():
				return
			case event, eventsOK := <-liveEvents:
				if !eventsOK {
					return
				}
				if event.Type != codersdk.ChatStreamEventTypeMessagePart ||
					event.MessagePart == nil {
					continue
				}
				part := event.MessagePart.Part
				if event.MessagePart.Role != codersdk.ChatMessageRoleTool ||
					part.Type != codersdk.ChatMessagePartTypeToolResult ||
					part.ToolName != chatadvisor.ToolName ||
					part.ToolCallID != "advisor-happy-path-call" ||
					part.ResultDelta == "" {
					continue
				}
				livePartsMu.Lock()
				liveAdvisorDeltas = append(liveAdvisorDeltas, part.ResultDelta)
				livePartsMu.Unlock()
			}
		}
	}()

	server.Start()

	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		if got.Status != database.ChatStatusWaiting &&
			got.Status != database.ChatStatusError {
			return false
		}
		return streamedCallCount.Load() >= 3
	}, testutil.WaitLong, testutil.IntervalFast)

	streamedCallsMu.Lock()
	gotAdvisorMessages := append([]chattest.OpenAIMessage(nil), advisorMessages...)
	gotFinalMessages := append([]chattest.OpenAIMessage(nil), finalCallMessages...)
	streamedCallsMu.Unlock()

	require.True(t, advisorCallSeen.Load(),
		"the nested advisor call must execute; missing it means the tool never ran")
	require.NotEmpty(t, gotAdvisorMessages,
		"advisor call must receive the nested prompt messages")
	require.NotEmpty(t, gotFinalMessages,
		"parent must make a follow-up call after the advisor result")

	var advisorSawQuestion bool
	var advisorSawUserTurn bool
	for _, msg := range gotAdvisorMessages {
		if strings.Contains(msg.Content, "how should I approach this refactor?") {
			advisorSawQuestion = true
		}
		if msg.Role == "user" && strings.Contains(msg.Content, "help me refactor this module") {
			advisorSawUserTurn = true
		}
	}
	require.True(t, advisorSawQuestion,
		"advisor must receive the parent's question verbatim")
	require.True(t, advisorSawUserTurn,
		"advisor must receive the parent's conversation snapshot as nested context")

	for _, msg := range gotAdvisorMessages {
		require.NotContains(t, msg.Content, chatadvisor.ParentGuidanceBlock,
			"ParentGuidanceBlock must be stripped before reaching the advisor")
	}

	var parentSawAdvisorResult bool
	for _, msg := range gotFinalMessages {
		if msg.Role == "tool" && strings.Contains(msg.Content, advisorReply) {
			parentSawAdvisorResult = true
			break
		}
	}
	require.True(t, parentSawAdvisorResult,
		"parent must see the advisor reply in its continuation call")

	// Stop the live collector and assert it captured the streaming
	// advisor deltas during processing. Late subscribers no longer
	// see committed parts because publishMessage claims them out of
	// new snapshots, so the assertion must use the live collector.
	require.Eventually(t, liveDeltasCaptured, testutil.WaitLong, testutil.IntervalFast,
		"advisor nested text deltas must stream into the parent tool card")
	cancelLive()
	<-liveCollectorDone
	livePartsMu.Lock()
	collectedAdvisorDeltas := append([]string(nil), liveAdvisorDeltas...)
	livePartsMu.Unlock()
	require.Equal(t, advisorDeltas, collectedAdvisorDeltas,
		"advisor nested text deltas must stream into the parent tool card")

	persisted, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	for _, msg := range persisted {
		require.NotContains(t, string(msg.Content.RawMessage), "result_delta",
			"advisor deltas are stream-only and must not be persisted")
	}
}

// TestAdvisorGating_ChildChat guards the second dimension of the advisor
// eligibility condition: even with advisor enabled, a chat whose
// ParentChatID is set must not register the advisor tool or receive the
// advisor guidance block. Without this coverage, a refactor that removes
// or weakens the !chat.ParentChatID.Valid guard would leak advisor into
// child chats, and the recursive advisor-inside-subagent cost risk the
// guard exists to prevent would ship silently.
//
// The earlier version of this test drove the gating path through
// spawn_agent, which made it dependent on subagent wiring that changed
// repeatedly upstream. This version seeds the parent chat directly in the
// database and asks the server to create a child chat with a valid
// ParentChatID, exercising the same gating path with no subagent tooling
// in the way.
func TestAdvisorGating_ChildChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var toolsMu sync.Mutex
	var capturedTools []string
	var capturedMessages []chattest.OpenAIMessage

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		toolsMu.Lock()
		capturedTools = names
		capturedMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
		toolsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   3,
		MaxOutputTokens: 16384,
	})

	// Seed the parent chat directly in the database so the test server
	// never executes the root turn. That keeps this test focused on the
	// child-chat gating path without depending on subagent wiring.
	parent := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		LastModelConfigID: model.ID,
		Title:             "advisor-root-parent",
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(
			chattest.NewMockAIBridgeTransport(t, openAIURL),
		)
	})

	childChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "advisor-child",
		ModelConfigID:  model.ID,
		ParentChatID:   uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:     uuid.NullUUID{UUID: parent.ID, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("hi"),
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, childChat.ID)
		if getErr != nil {
			return false
		}
		return got.Status == database.ChatStatusWaiting ||
			got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	toolsMu.Lock()
	tools := append([]string(nil), capturedTools...)
	messages := append([]chattest.OpenAIMessage(nil), capturedMessages...)
	toolsMu.Unlock()

	require.NotEmpty(t, messages, "expected a streamed LLM request for the child chat")
	require.NotContains(t, tools, chatadvisor.ToolName,
		"advisor tool must not be registered for child chats even when enabled")
	for _, msg := range messages {
		require.NotContains(t, msg.Content, chatadvisor.ParentGuidanceBlock,
			"child chat must not contain advisor guidance")
	}
}

// TestAdvisorGating_PlanMode guards the third dimension of the advisor
// eligibility condition: plan-mode turns must not register the advisor tool
// or inject the parent guidance block. Without this test, deleting the
// !isPlanModeTurn guard would still leave the other two gating tests green
// even though advisor would now leak into plan mode.
func TestAdvisorGating_PlanMode(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var toolsMu sync.Mutex
	var capturedTools []string
	var capturedMessages []chattest.OpenAIMessage

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		toolsMu.Lock()
		capturedTools = names
		capturedMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
		toolsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("plan mode reply")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   3,
		MaxOutputTokens: 16384,
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(
			chattest.NewMockAIBridgeTransport(t, openAIURL),
		)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "advisor-plan-mode",
		ModelConfigID:  model.ID,
		PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("draft a plan"),
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		return got.Status == database.ChatStatusWaiting ||
			got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	toolsMu.Lock()
	tools := append([]string(nil), capturedTools...)
	messages := append([]chattest.OpenAIMessage(nil), capturedMessages...)
	toolsMu.Unlock()

	require.NotEmpty(t, messages, "expected a streamed LLM request")
	require.NotContains(t, tools, "advisor",
		"plan-mode turns must not register the advisor tool even when enabled")
	for _, msg := range messages {
		require.NotContains(t, msg.Content, chatadvisor.ParentGuidanceBlock,
			"plan-mode turns must not inject advisor guidance")
	}
}

// TestAdvisorGating_ExploreSubagent guards the fourth dimension of the
// advisor eligibility condition: Explore chats (root or subagent) run
// under allowedExploreToolNames, whose policy does not include advisor,
// so the runtime must not register the advisor tool or inject the
// parent guidance block there. Without this test, deleting the
// !isExploreSubagent guard would leave the other gating tests green
// while leaking advisor into explore chats.
func TestAdvisorGating_ExploreSubagent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var toolsMu sync.Mutex
	var capturedTools []string
	var capturedMessages []chattest.OpenAIMessage

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		toolsMu.Lock()
		capturedTools = names
		capturedMessages = append([]chattest.OpenAIMessage(nil), req.Messages...)
		toolsMu.Unlock()

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("explore reply")...,
		)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	seedAdvisorConfig(ctx, t, db, codersdk.AdvisorConfig{
		Enabled:         true,
		MaxUsesPerRun:   3,
		MaxOutputTokens: 16384,
	})
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(
			chattest.NewMockAIBridgeTransport(t, openAIURL),
		)
	})

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "advisor-explore",
		ModelConfigID:  model.ID,
		ChatMode: database.NullChatMode{
			ChatMode: database.ChatModeExplore,
			Valid:    true,
		},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("inspect the codebase"),
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		return got.Status == database.ChatStatusWaiting ||
			got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	toolsMu.Lock()
	tools := append([]string(nil), capturedTools...)
	messages := append([]chattest.OpenAIMessage(nil), capturedMessages...)
	toolsMu.Unlock()

	require.NotEmpty(t, messages, "expected a streamed LLM request")
	require.NotContains(t, tools, chatadvisor.ToolName,
		"explore chats must not register the advisor tool even when enabled")
	for _, msg := range messages {
		require.NotContains(t, msg.Content, chatadvisor.ParentGuidanceBlock,
			"explore chats must not inject advisor guidance")
	}
}

// TestProviderSwitchSanitizesAndRestoresPEToolHistory verifies the A→B→A
// provider-switch contract:
//
//  1. A turn using model MA (backed by provider A) produces a
//     provider-executed (PE) tool call in the DB.
//  2. A subsequent turn using model MB (backed by provider B) does NOT
//     send that PE tool call to provider B.
//  3. A further turn back to MA sends the PE tool call to provider A again.
//  4. The DB row is never mutated; the filter is read-time only.
func TestProviderSwitchSanitizesAndRestoresPEToolHistory(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	// Given: two AI providers A and B
	const peToolCallID = "pe_switch_test_id"

	chanA := make(chan string, 4)
	chanB := make(chan string, 4)

	serverAURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"switch-test"}`)
		}
		chanA <- string(req.RawBody)
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("answer from A")...)
	})
	serverBURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse(`{"title":"switch-test"}`)
		}
		chanB <- string(req.RawBody)
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("answer from B")...)
	})
	cpA := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider: "openai-compat",
		BaseUrl:  serverAURL,
	})
	cpB := dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider: "openai-compat",
		BaseUrl:  serverBURL,
	})

	mA := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		DisplayName:  "Model A",
		Enabled:      true,
		AIProviderID: uuid.NullUUID{UUID: cpA.ID, Valid: true},
	})
	mB := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:        "gpt-4o-mini",
		DisplayName:  "Model B",
		Enabled:      true,
		AIProviderID: uuid.NullUUID{UUID: cpB.ID, Valid: true},
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		provA, err := db.GetAIProviderByID(ctx, cpA.ID)
		require.NoError(t, err)
		provB, err := db.GetAIProviderByID(ctx, cpB.ID)
		require.NoError(t, err)
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(providerRoutedTransportFactory{
			routes: map[string]aibridge.TransportFactory{
				provA.Name: chattest.NewMockAIBridgeTransport(t, serverAURL),
				provB.Name: chattest.NewMockAIBridgeTransport(t, serverBURL),
			},
		})
	})

	// Given: an initial conversation turn with model A that produces provider-executed
	// tool call results
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "provider-switch-test",
		ModelConfigID:      mA.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)
	insertChatMessageParts(ctx, t, db, chat.ID, database.ChatMessageRoleAssistant, mA.ID, uuid.Nil,
		[]codersdk.ChatMessagePart{
			{
				Type:             codersdk.ChatMessagePartTypeToolCall,
				ToolCallID:       peToolCallID,
				ToolName:         "web_search",
				Args:             json.RawMessage(`{"query":"coder"}`),
				ProviderExecuted: true,
			},
			{
				Type:             codersdk.ChatMessagePartTypeToolResult,
				ToolCallID:       peToolCallID,
				ToolName:         "web_search",
				Result:           json.RawMessage(`"search results"`),
				ProviderExecuted: true,
			},
			codersdk.ChatMessageText("here is the answer"),
		},
	)

	// When: a conversation turn is executed with model B
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: mB.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("continue with B")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	// When: a further conversation turn is executed with model A again
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:        chat.ID,
		CreatedBy:     user.ID,
		ModelConfigID: mA.ID,
		Content:       []codersdk.ChatMessagePart{codersdk.ChatMessageText("back to A")},
	})
	require.NoError(t, err)
	waitForChatStatus(ctx, t, db, chat.ID, database.ChatStatusWaiting)

	// Then: the provider-executed tool call results should still be in the database
	allMessages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: chat.ID,
	})
	require.NoError(t, err)
	var peRowFound bool
	for _, msg := range allMessages {
		if msg.Role != database.ChatMessageRoleAssistant || msg.ModelConfigID.UUID != mA.ID {
			continue
		}
		parts, parseErr := chatprompt.ParseContent(msg)
		require.NoError(t, parseErr)
		for _, p := range parts {
			if p.ProviderExecuted && p.ToolCallID == peToolCallID {
				peRowFound = true
			}
		}
	}
	require.True(t, peRowFound, "PE tool call must still be in the DB after provider switches")

	// Skip initial generation request
	_ = testutil.TryReceive(ctx, t, chanA)

	// Then: PE tool call ID from A must not appear in the request to provider B
	turn2Body := testutil.TryReceive(ctx, t, chanB)
	require.NotContains(t, turn2Body, peToolCallID,
		"provider B must not receive the PE tool call from provider A")

	// Then: PE tool call ID must appear in the second request to provider A
	turn3Body := testutil.TryReceive(ctx, t, chanA)
	require.Contains(t, turn3Body, peToolCallID,
		"provider A must receive its own PE tool call when switching back")
}

// providerRoutedTransportFactory implements aibridge.TransportFactory by
// dispatching to a different mock transport per AI provider name. It
// exists for tests that exercise two providers in the same chat (e.g.
// switching models mid-conversation), where a single
// chattest.MockAIBridgeTransport's fixed target can't represent both.
type providerRoutedTransportFactory struct {
	routes map[string]aibridge.TransportFactory
}

func (f providerRoutedTransportFactory) TransportFor(providerName string, source aibridge.Source) (http.RoundTripper, error) {
	route, ok := f.routes[providerName]
	if !ok {
		return nil, xerrors.Errorf("no mock transport configured for provider %q", providerName)
	}
	return route.TransportFor(providerName, source)
}

func seedAdvisorConfig(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	cfg codersdk.AdvisorConfig,
) {
	t.Helper()

	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	err = db.UpsertChatAdvisorConfig(
		dbauthz.AsSystemRestricted(ctx),
		string(data),
	)
	require.NoError(t, err)
}

// nullRawMessage wraps raw JSON in a NullRawMessage. An empty input
// becomes the zero value (Valid=false).
func nullRawMessage(raw []byte) pqtype.NullRawMessage {
	if len(raw) == 0 {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}
}

func setupWorkspaceContextAgentConn(
	t *testing.T,
	mockConn *agentconnmock.MockAgentConn,
	agent database.WorkspaceAgent,
	contextText string,
	contextConfigCalls *atomic.Int32,
) {
	t.Helper()
	directory := agent.ExpandedDirectory
	if directory == "" {
		directory = agent.Directory
	}
	if directory == "" {
		directory = "/home/coder/project"
	}
	operatingSystem := agent.OperatingSystem
	if operatingSystem == "" {
		operatingSystem = "linux"
	}
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).DoAndReturn(
		func(context.Context) (workspacesdk.ContextConfigResponse, error) {
			if contextConfigCalls != nil {
				contextConfigCalls.Add(1)
			}
			return workspacesdk.ContextConfigResponse{
				Parts: []codersdk.ChatMessagePart{{
					Type:                 codersdk.ChatMessagePartTypeContextFile,
					ContextFilePath:      directory + "/AGENTS.md",
					ContextFileContent:   contextText,
					ContextFileOS:        operatingSystem,
					ContextFileDirectory: directory,
				}},
			}, nil
		},
	).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()
}
