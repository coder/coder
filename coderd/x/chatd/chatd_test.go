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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
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
	Messages           []chattest.OpenAIMessage
	Tools              []string
	Store              *bool
	PreviousResponseID *string
	ContentLength      int64
}

func openAIToolName(tool chattest.OpenAITool) string {
	return cmp.Or(tool.Function.Name, tool.Name, tool.Type)
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

	var previousResponseID *string
	if req.PreviousResponseID != nil {
		value := *req.PreviousResponseID
		previousResponseID = &value
	}

	var contentLength int64
	if req.Request != nil {
		contentLength = req.Request.ContentLength
	}

	return recordedOpenAIRequest{
		Messages:           messages,
		Tools:              tools,
		Store:              store,
		PreviousResponseID: previousResponseID,
		ContentLength:      contentLength,
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
) *chatd.Server {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().ListMCPTools(gomock.Any()).
		Return(workspacesdk.ListMCPToolsResponse{}, nil).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, path string, _, _ int64) (io.ReadCloser, string, error) {
			if path == "/home/coder/PLAN.md" {
				return io.NopCloser(strings.NewReader(planContent)), "", nil
			}
			return io.NopCloser(strings.NewReader("")), "", nil
		}).AnyTimes()

	return newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, gotAgentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, agentID, gotAgentID)
			return mockConn, func() {}, nil
		}
	})
}

func TestInterruptChatBroadcastsStatusAcrossInstances(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replicaA := newTestServer(t, db, ps, uuid.New())
	replicaB := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replicaA.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-me",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	runningWorker := uuid.New()
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: runningWorker, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	_, events, cancel, ok := replicaB.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	updated := replicaA.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)
	require.False(t, updated.WorkerID.Valid)

	require.Eventually(t, func() bool {
		select {
		case event := <-events:
			if event.Type == codersdk.ChatStreamEventTypeStatus && event.Status != nil {
				return event.Status.Status == codersdk.ChatStatusWaiting
			}
			t.Logf("skipping unexpected event: type=%s", event.Type)
			return false
		default:
			return false
		}
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestSubagentChatExcludesWorkspaceProvisioningTools(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
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

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

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

	workspaceTools := []string{"list_templates", "read_template", "create_workspace"}
	subagentTools := []string{"spawn_agent", "wait_agent", "message_agent", "close_agent"}

	// Identify root and subagent calls. Root chat calls include
	// spawn_agent; the subagent call does not. Because the root chat
	// makes multiple LLM calls (before and after spawn_agent), we
	// find exactly one call that lacks spawn_agent — that's the
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

	// Standard turns (no turn mode) should hide propose_plan.
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
}

func TestPlanModeSubagentChatExcludesAskUserQuestion(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
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

	_, err = expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

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
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
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

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

	_, err = expClient.CreateChat(ctx, codersdk.CreateChatRequest{
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

	rootChats, err := db.GetChats(dbauthz.AsChatd(ctx), database.GetChatsParams{OwnerID: user.UserID})
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

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	webSearchEnabled := true
	storeEnabled := true
	// OpenAI only serializes web_search through the Responses API.
	// Store=true routes there only for supported Responses models.
	webSearchModel := insertChatModelConfigWithCallConfig(
		ctx,
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
	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "External Snapshot MCP",
		Slug:          "external-snapshot-mcp",
		Url:           externalMCPServer.URL,
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
	_, err = db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "Second MCP",
		Slug:          "second-mcp",
		Url:           secondMCPServer.URL,
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

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	rootChat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		AgentID:           uuid.NullUUID{UUID: dbAgent.ID, Valid: true},
		LastModelConfigID: webSearchModel.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeApi,
	})
	require.NoError(t, err)

	exploreChat, err := db.InsertChat(ctx, database.InsertChatParams{
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
		Status:       database.ChatStatusPending,
		MCPServerIDs: []uuid.UUID{mcpConfig.ID},
		ClientType:   database.ChatClientTypeApi,
	})
	require.NoError(t, err)
	insertUserTextMessage(ctx, t, db, exploreChat.ID, user.ID, webSearchModel.ID, "inspect the codebase")

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	workspaceToolName := "workspace-snapshot-mcp__echo"
	mockConn.EXPECT().ListMCPTools(gomock.Any()).
		Return(workspacesdk.ListMCPToolsResponse{Tools: []workspacesdk.MCPToolInfo{{
			ServerName:  "workspace-snapshot-mcp",
			Name:        workspaceToolName,
			Description: "Workspace echo tool",
			Schema: map[string]any{
				"input": map[string]any{"type": "string"},
			},
			Required: []string{"input"},
		}}}, nil).
		AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			require.Equal(t, dbAgent.ID, agentID)
			return mockConn, func() {}, nil
		}
	})
	_ = server

	chatResult := waitForTerminalChat(ctx, t, db, exploreChat.ID)
	if chatResult.Status == database.ChatStatusError {
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatResult.LastError.String)
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "Root Explore Runtime MCP",
		Slug:          "root-explore-runtime-mcp",
		Url:           externalMCPServer.URL,
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

	server := newActiveTestServer(t, db, ps)

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
		require.FailNowf(t, "explore chat failed", "last_error=%q", storedChat.LastError.String)
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

	user, org, _ := seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
	webSearchEnabled := true
	storeEnabled := true
	// OpenAI only serializes web_search through the Responses API.
	// Store=true routes there only for supported Responses models.
	webSearchModel := insertChatModelConfigWithCallConfig(
		ctx,
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

	server := newActiveTestServer(t, db, ps)

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
		require.FailNowf(t, "explore chat failed", "last_error=%q", storedChat.LastError.String)
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
	parentConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "Runtime Parent MCP",
		Slug:          "runtime-parent-mcp",
		Url:           parentTS.URL,
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
	injectedConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "Runtime Injected MCP",
		Slug:          "runtime-injected-mcp",
		Url:           injectedTS.URL,
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

	server := newActiveTestServer(t, db, ps)

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
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatResult.LastError.String)
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
		require.FailNowf(t, "explore chat failed", "last_error=%q", chatResult.LastError.String)
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	approvedConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:     "Plan Approved MCP",
		Slug:            "plan-approved-mcp",
		Url:             echoTS.URL,
		Transport:       "streamable_http",
		AuthType:        "none",
		Availability:    "default_off",
		Enabled:         true,
		AllowInPlanMode: true,
		ToolAllowList:   []string{},
		ToolDenyList:    []string{},
		CreatedBy:       user.ID,
		UpdatedBy:       user.ID,
	})
	require.NoError(t, err)

	blockedConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:     "Plan Blocked MCP",
		Slug:            "plan-blocked-mcp",
		Url:             echoTS.URL,
		Transport:       "streamable_http",
		AuthType:        "none",
		Availability:    "default_off",
		Enabled:         true,
		AllowInPlanMode: false,
		ToolAllowList:   []string{},
		ToolDenyList:    []string{},
		CreatedBy:       user.ID,
		UpdatedBy:       user.ID,
	})
	require.NoError(t, err)

	filteredConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:     "Plan Filtered MCP",
		Slug:            "plan-filtered-mcp",
		Url:             filteredTS.URL,
		Transport:       "streamable_http",
		AuthType:        "none",
		Availability:    "default_off",
		Enabled:         true,
		AllowInPlanMode: true,
		ToolAllowList:   []string{"visible"},
		ToolDenyList:    []string{},
		CreatedBy:       user.ID,
		UpdatedBy:       user.ID,
	})
	require.NoError(t, err)

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	workspaceToolName := "workspace-plan-mcp__echo"
	mockConn.EXPECT().ListMCPTools(gomock.Any()).
		Return(workspacesdk.ListMCPToolsResponse{Tools: []workspacesdk.MCPToolInfo{{
			ServerName:  "workspace-plan-mcp",
			Name:        workspaceToolName,
			Description: "Workspace echo tool",
			Schema: map[string]any{
				"input": map[string]any{"type": "string"},
			},
			Required: []string{"input"},
		}}}, nil).
		Times(1)
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{AbsolutePathString: "/home/coder"}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
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

func TestInterruptChatClearsWorkerInDatabase(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "db-transition",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	updated := replica.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)
	require.False(t, updated.WorkerID.Valid)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)
	require.False(t, fromDB.WorkerID.Valid)
}

func TestArchiveChatMovesPendingChatToWaiting(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "archive-pending",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusPending,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

	err = replica.ArchiveChat(ctx, chat)
	require.NoError(t, err)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)
	require.False(t, fromDB.WorkerID.Valid)
	require.False(t, fromDB.StartedAt.Valid)
	require.False(t, fromDB.HeartbeatAt.Valid)
	require.True(t, fromDB.Archived)
	require.Zero(t, fromDB.PinOrder)
}

// TestUnarchiveChildChat covers the deterministic branches of the
// Server.UnarchiveChat child path: happy path, archived-parent reject,
// and already-active no-op.
func TestUnarchiveChildChat(t *testing.T) {
	t.Parallel()

	t.Run("ChildWithActiveParentUnarchives", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(ctx, t, db)

		parent, child := insertParentWithArchivedChild(ctx, t, db, user, org, model)

		require.NoError(t, replica.UnarchiveChat(ctx, child))

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.False(t, dbChild.Archived, "child should be unarchived")

		dbParent, err := db.GetChatByID(ctx, parent.ID)
		require.NoError(t, err)
		require.False(t, dbParent.Archived, "parent should stay active")
	})

	t.Run("ChildWithArchivedParentRejected", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(ctx, t, db)

		parent, child := insertParentWithArchivedChild(ctx, t, db, user, org, model)
		_, err := db.ArchiveChatByID(ctx, parent.ID)
		require.NoError(t, err)

		err = replica.UnarchiveChat(ctx, child)
		require.ErrorIs(t, err, chatd.ErrChildUnarchiveParentArchived)

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.True(t, dbChild.Archived, "child should remain archived")
	})

	t.Run("AlreadyActiveChildNoOp", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		replica := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(ctx, t, db)

		_, child := insertParentWithActiveChild(ctx, t, db, user, org, model)

		require.NoError(t, replica.UnarchiveChat(ctx, child))

		dbChild, err := db.GetChatByID(ctx, child.ID)
		require.NoError(t, err)
		require.False(t, dbChild.Archived, "child should stay active")
	})
}

// insertParentWithActiveChild creates a parent chat and an active
// child chat linked to it. Both are returned in their initial
// (active) state.
func insertParentWithActiveChild(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	user database.User,
	org database.Organization,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()
	var err error
	parent, err = db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		LastModelConfigID: model.ID,
		Title:             "parent",
	})
	require.NoError(t, err)
	child, err = db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		LastModelConfigID: model.ID,
		Title:             "child",
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
	})
	require.NoError(t, err)
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
	parent, child = insertParentWithActiveChild(ctx, t, db, user, org, model)
	_, err := db.ArchiveChatByID(ctx, child.ID)
	require.NoError(t, err)
	child, err = db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	return parent, child
}

func TestArchiveChatInterruptsActiveProcessing(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	streamStarted := make(chan struct{})
	streamCanceled := make(chan struct{})
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
			<-req.Context().Done()
			select {
			case <-streamCanceled:
			default:
				close(streamCanceled)
			}
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	})

	server := newActiveTestServer(t, db, ps)
	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "archive-interrupt",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

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

	_, events, cancel, ok := server.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	defer cancel()

	queuedResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, queuedResult.Queued)
	require.NotNil(t, queuedResult.QueuedMessage)

	err = server.ArchiveChat(ctx, chat)
	require.NoError(t, err)

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case <-streamCanceled:
			return true
		default:
			return false
		}
	}, testutil.IntervalFast)

	gotWaitingStatus := false
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		for {
			select {
			case ev := <-events:
				if ev.Type == codersdk.ChatStreamEventTypeStatus &&
					ev.Status != nil &&
					ev.Status.Status == codersdk.ChatStatusWaiting {
					gotWaitingStatus = true
					return true
				}
			default:
				return gotWaitingStatus
			}
		}
	}, testutil.IntervalFast)
	require.True(t, gotWaitingStatus, "expected a waiting status event after archive")

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Archived &&
			fromDB.Status == database.ChatStatusWaiting &&
			!fromDB.WorkerID.Valid &&
			!fromDB.StartedAt.Valid &&
			!fromDB.HeartbeatAt.Valid
	}, testutil.IntervalFast)

	queuedMessages, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queuedMessages, 1)
	require.Equal(t, queuedResult.QueuedMessage.ID, queuedMessages[0].ID)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	userMessages := 0
	for _, msg := range messages {
		if msg.Role == database.ChatMessageRoleUser {
			userMessages++
		}
	}
	require.Equal(t, 1, userMessages, "expected queued message to stay queued after archive")
}

func TestUpdateChatHeartbeatsRequiresOwnership(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

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
	user, org, model := seedChatDependencies(ctx, t, db)

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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
	planModeInstructions := "Ask about deployment sequencing before finalizing the plan."
	err := db.UpsertChatPlanModeInstructions(dbauthz.AsSystemRestricted(ctx), planModeInstructions)
	require.NoError(t, err)
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)
	server := newWorkspaceToolTestServer(t, db, ps, dbAgent.ID, "# Plan\n")

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

func TestSendMessageQueuesWhenWaitingWithQueuedBacklog(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "queue-when-waiting-with-backlog",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("older queued"),
	})
	require.NoError(t, err)
	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  chat.ID,
		Content: queuedContent,
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusWaiting,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("newer queued")},
	})
	require.NoError(t, err)
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)
	require.Equal(t, database.ChatStatusWaiting, result.Chat.Status)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 2)

	olderSDK := db2sdk.ChatQueuedMessage(queued[0])
	require.Len(t, olderSDK.Content, 1)
	require.Equal(t, "older queued", olderSDK.Content[0].Text)

	newerSDK := db2sdk.ChatQueuedMessage(queued[1])
	require.Len(t, newerSDK.Content, 1)
	require.Equal(t, "newer queued", newerSDK.Content[0].Text)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func TestSendMessageInterruptBehaviorQueuesAndInterruptsWhenBusy(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-when-busy",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// CreateChat calls signalWake which triggers processOnce in
	// the background. Wait for that processing to finish so it
	// doesn't race with the manual status update below.
	waitForChatProcessed(ctx, t, db, chat.ID, replica)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	result, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("interrupt")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)

	// The message should be queued, not inserted directly.
	require.True(t, result.Queued)
	require.NotNil(t, result.QueuedMessage)

	// The chat should transition to waiting (interrupt signal),
	// not pending.
	require.Equal(t, database.ChatStatusWaiting, result.Chat.Status)

	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status)

	// The message should be in the queue, not in chat_messages.
	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 1)

	// Only messages from the initial processing round should be in
	// chat_messages (user + assistant). The "interrupt" message must
	// be in the queue, not inserted directly.
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 2)
}

func TestEditMessageUpdatesAndTruncatesAndClearsQueue(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "edit-message",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("original")},
	})
	require.NoError(t, err)

	initialMessages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, initialMessages, 1)
	editedMessageID := initialMessages[0].ID

	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("follow-up")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)
	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("another")},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	})
	require.NoError(t, err)

	queuedContent, err := json.Marshal([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("queued"),
	})
	require.NoError(t, err)
	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:  chat.ID,
		Content: queuedContent,
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	editResult, err := replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: editedMessageID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.NoError(t, err)
	// The edited message is soft-deleted and a new message is inserted,
	// so the returned message ID will differ from the original.
	require.NotEqual(t, editedMessageID, editResult.Message.ID)
	require.Equal(t, database.ChatStatusPending, editResult.Chat.Status)
	require.False(t, editResult.Chat.WorkerID.Valid)

	editedSDK := db2sdk.ChatMessage(editResult.Message)
	require.Len(t, editedSDK.Content, 1)
	require.Equal(t, "edited", editedSDK.Content[0].Text)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, editResult.Message.ID, messages[0].ID)
	onlyMessage := db2sdk.ChatMessage(messages[0])
	require.Len(t, onlyMessage.Content, 1)
	require.Equal(t, "edited", onlyMessage.Content[0].Text)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, queued, 0)

	// The wake channel may trigger immediate processing after EditMessage,
	// transitioning the chat from pending to running then error before we
	// read the DB. Wait for any in-flight processing to settle.
	// Note: WaitUntilIdleForTest must be called from the test goroutine
	// (not inside require.Eventually) to avoid a WaitGroup Add/Wait race.
	chatd.WaitUntilIdleForTest(replica)
	var chatFromDB database.Chat
	require.Eventually(t, func() bool {
		c, e := db.GetChatByID(ctx, chat.ID)
		if e != nil {
			return false
		}
		chatFromDB = c
		return chatFromDB.Status != database.ChatStatusRunning
	}, testutil.WaitShort, testutil.IntervalFast)
	require.False(t, chatFromDB.WorkerID.Valid)
}

func TestCreateChatInsertsWorkspaceAwarenessMessage(t *testing.T) {
	t.Parallel()

	t.Run("WithWorkspace", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())

		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(ctx, t, db)

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
		user, org, model := seedChatDependencies(ctx, t, db)

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
				if strings.Contains(content, "no workspace associated") {
					workspaceMsg = &msg
					break
				}
			}
		}
		require.NotNil(t, workspaceMsg, "workspace awareness system message should exist")
		require.Equal(t, database.ChatMessageRoleSystem, workspaceMsg.Role)
		require.Equal(t, database.ChatMessageVisibilityModel, workspaceMsg.Visibility)
	})
}

func TestCreateChatRejectsWhenUsageLimitReached(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	existingChat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		Title:             "existing-limit-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              existingChat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	beforeChats, err := db.GetChats(ctx, database.GetChatsParams{
		OwnerID:   user.ID,
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
		OwnerID:   user.ID,
		AfterID:   uuid.Nil,
		OffsetOpt: 0,
		LimitOpt:  100,
	})
	require.NoError(t, err)
	require.Len(t, afterChats, len(beforeChats))
}

func TestPromoteQueuedAllowsAlreadyQueuedMessageWhenUsageLimitReached(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "queued-limit-reached",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// CreateChat calls signalWake which triggers processOnce in
	// the background. Wait for that processing to finish so it
	// doesn't race with the manual status update below.
	waitForChatProcessed(ctx, t, db, chat.ID, replica)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	queuedResult, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, queuedResult.Queued)
	require.NotNil(t, queuedResult.QueuedMessage)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("assistant"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusWaiting,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

	result, err := replica.PromoteQueued(ctx, chatd.PromoteQueuedOptions{
		ChatID:          chat.ID,
		QueuedMessageID: queuedResult.QueuedMessage.ID,
		CreatedBy:       user.ID,
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatMessageRoleUser, result.PromotedMessage.Role)

	queued, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, queued)

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	require.Len(t, messages, 4)
	require.Equal(t, database.ChatMessageRoleUser, messages[3].Role)
}

func TestInterruptAutoPromotionIgnoresLaterUsageLimitIncrease(t *testing.T) {
	t.Parallel()

	const acquireInterval = 10 * time.Millisecond

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100,
		Period:             string(codersdk.ChatUsageLimitPeriodDay),
	})
	require.NoError(t, err)

	clock := quartz.NewMock(t)
	acquireTrap := clock.Trap().NewTicker("chatd", "acquire")
	defer acquireTrap.Close()

	assertPendingWithoutQueuedMessages := func(chatID uuid.UUID) {
		t.Helper()

		queued, dbErr := db.GetChatQueuedMessages(ctx, chatID)
		require.NoError(t, dbErr)
		require.Empty(t, queued)

		fromDB, dbErr := db.GetChatByID(ctx, chatID)
		require.NoError(t, dbErr)
		require.Equal(t, database.ChatStatusPending, fromDB.Status)
		require.False(t, fromDB.WorkerID.Valid)
	}

	streamStarted := make(chan struct{})
	interrupted := make(chan struct{})
	secondRequestStarted := make(chan struct{})
	thirdRequestStarted := make(chan struct{})
	allowFinish := make(chan struct{})
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
			close(secondRequestStarted)
		case 3:
			close(thirdRequestStarted)
		}

		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("done")...,
		)
	})

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		cfg.Clock = clock
		cfg.PendingChatAcquireInterval = acquireInterval
		cfg.InFlightChatStaleAfter = testutil.WaitSuperLong
	})
	acquireTrap.MustWait(ctx).MustRelease(ctx)

	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-autopromote-limit",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	clock.Advance(acquireInterval).MustWait(ctx)
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
	chatd.WaitUntilIdleForTest(server)
	assertPendingWithoutQueuedMessages(chat.ID)

	// Keep the acquire loop frozen here so "queued" stays pending.
	// That makes the later send queue because the chat is still busy,
	// rather than because the scheduler happened to be slow.
	laterQueuedResult, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("later queued")},
	})
	require.NoError(t, err)
	require.True(t, laterQueuedResult.Queued)
	require.NotNil(t, laterQueuedResult.QueuedMessage)

	spendChat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{},
		ParentChatID:      uuid.NullUUID{},
		RootChatID:        uuid.NullUUID{},
		LastModelConfigID: model.ID,
		Title:             "other-spend",
		Mode:              database.NullChatMode{},
	})
	require.NoError(t, err)

	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("spent elsewhere"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              spendChat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

	clock.Advance(acquireInterval).MustWait(ctx)
	testutil.TryReceive(ctx, t, secondRequestStarted)
	chatd.WaitUntilIdleForTest(server)
	assertPendingWithoutQueuedMessages(chat.ID)

	clock.Advance(acquireInterval).MustWait(ctx)
	testutil.TryReceive(ctx, t, thirdRequestStarted)
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
	user, org, model := seedChatDependencies(ctx, t, db)

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

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{100},
		RuntimeMs:           []int64{0},
	})
	require.NoError(t, err)

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
	user, org, model := seedChatDependencies(ctx, t, db)

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
	user, org, model := seedChatDependencies(ctx, t, db)

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

	assistantMessages, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(assistantContent.RawMessage)},
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
	assistantMessage := assistantMessages[0]

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
	user, org, model := seedChatDependencies(ctx, t, db)

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
	user, org, model := seedChatDependencies(ctx, t, db)

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

// TestArchiveChatDebugCleanupDeletesPreArchiveRuns verifies that
// ArchiveChat schedules cleanup that deletes pre-archive debug runs
// for the archived chat. Covers the archiveCutoff sampled from
// ArchiveChatByID's DB-stamped updated_at and the DeleteByChatID
// delete path.
func TestArchiveChatDebugCleanupDeletesPreArchiveRuns(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newDebugEnabledTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "debug-archive-cleanup",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	staleStart := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	staleRun, err := db.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Kind:          "chat_turn",
		Status:        "in_progress",
		Provider:      sql.NullString{String: "openai", Valid: true},
		Model:         sql.NullString{String: model.Model, Valid: true},
		StartedAt:     sql.NullTime{Time: staleStart, Valid: true},
		UpdatedAt:     sql.NullTime{Time: staleStart, Valid: true},
	})
	require.NoError(t, err)

	// Freshly-inserted run inside the skew buffer must survive the
	// fast retry for the same reason as the edit-cleanup buffer test.
	recentStart := time.Now().Add(-time.Second).UTC().Truncate(time.Microsecond)
	recentRun, err := db.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Kind:          "chat_turn",
		Status:        "in_progress",
		Provider:      sql.NullString{String: "openai", Valid: true},
		Model:         sql.NullString{String: model.Model, Valid: true},
		StartedAt:     sql.NullTime{Time: recentStart, Valid: true},
		UpdatedAt:     sql.NullTime{Time: recentStart, Valid: true},
	})
	require.NoError(t, err)

	err = replica.ArchiveChat(ctx, chat)
	require.NoError(t, err)

	chatd.WaitUntilIdleForTest(replica)

	// ErrNoRows proves the fast-retry path DELETED the row:
	// FinalizeStale only UPDATEs in place, never deletes.
	_, err = db.GetChatDebugRunByID(ctx, staleRun.ID)
	require.ErrorIs(t, err, sql.ErrNoRows,
		"pre-archive run outside the buffer should be deleted")

	remaining, err := db.GetChatDebugRunByID(ctx, recentRun.ID)
	require.NoError(t, err,
		"runs inside the clock-skew buffer must survive the fast retry")
	require.Equal(t, recentRun.ID, remaining.ID)

	// Count the seeded survivors directly so the delete is verified
	// not just by absence of a specific row. Scoped to seeded IDs
	// because the archive transition may still race with other
	// background debug writes.
	remainingRuns, err := db.GetChatDebugRunsByChatID(ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID: chat.ID, LimitVal: 100,
	})
	require.NoError(t, err)
	seeded := map[uuid.UUID]bool{staleRun.ID: true, recentRun.ID: true}
	survivors := 0
	for _, r := range remainingRuns {
		if seeded[r.ID] {
			survivors++
		}
	}
	require.Equal(t, 1, survivors,
		"only the recent (buffered) seeded run should survive")
}

func TestRecoverStaleChatsPeriodically(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	// Use a very short stale threshold so the periodic recovery
	// kicks in quickly during the test.
	staleAfter := 500 * time.Millisecond

	// Create a chat and simulate a dead worker by setting the chat
	// to running with a heartbeat in the past.
	deadWorkerID := uuid.New()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		Title:             "stale-recovery-periodic",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadWorkerID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// Start a new replica. Its startup recovery will reset the
	// chat (since the heartbeat is old), but the key point is that
	// the periodic loop also recovers newly-stale chats.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
		InFlightChatStaleAfter:     staleAfter,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// The startup recovery should have already reset our stale
	// chat.
	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Now simulate a second stale chat appearing AFTER startup.
	// This tests the periodic recovery, not just the startup one.
	deadWorkerID2 := uuid.New()
	chat2, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		Title:             "stale-recovery-periodic-2",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat2.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadWorkerID2, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// The periodic stale recovery loop (running at staleAfter/5 =
	// 100ms intervals) should pick this up without a restart.
	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat2.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestRecoverStaleRequiresActionChat(t *testing.T) {
	t.Parallel()

	db, ps, rawDB := dbtestutil.NewDBWithSQLDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	// Use a very short stale threshold so the periodic recovery
	// kicks in quickly during the test.
	staleAfter := 500 * time.Millisecond

	// Create a chat and set it to requires_action to simulate a
	// client that disappeared while the chat was waiting for
	// dynamic tool results.
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		Title:             "stale-requires-action",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusRequiresAction,
	})
	require.NoError(t, err)

	// Backdate updated_at so the chat appears stale to the
	// recovery loop without needing time.Sleep.
	_, err = rawDB.ExecContext(ctx,
		"UPDATE chats SET updated_at = $1 WHERE id = $2",
		time.Now().Add(-time.Hour), chat.ID)
	require.NoError(t, err)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
		InFlightChatStaleAfter:     staleAfter,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// The stale recovery should transition the requires_action
	// chat to error with the timeout message.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		chatResult, err = db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return chatResult.Status == database.ChatStatusError
	}, testutil.WaitMedium, testutil.IntervalFast)

	require.Contains(t, chatResult.LastError.String, "Dynamic tool execution timed out")
	require.False(t, chatResult.WorkerID.Valid)
}

func TestNewReplicaRecoversStaleChatFromDeadReplica(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	// Simulate a chat left running by a dead replica with a stale
	// heartbeat (well beyond the stale threshold).
	deadReplicaID := uuid.New()
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		Title:             "orphaned-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Set the heartbeat far in the past so it's definitely stale.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: deadReplicaID, Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	require.NoError(t, err)

	// Start a new replica — it should recover the stale chat on
	// startup.
	newReplica := newTestServer(t, db, ps, uuid.New())
	_ = newReplica

	require.Eventually(t, func() bool {
		fromDB, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending &&
			!fromDB.WorkerID.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestWaitingChatsAreNotRecoveredAsStale(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	// Create a chat in waiting status — this should NOT be touched
	// by stale recovery.
	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		Title:             "waiting-chat",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Start a replica with a short stale threshold.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
		InFlightChatStaleAfter:     500 * time.Millisecond,
	})
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
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           user.ID,
		Title:             "error-persisted",
		LastModelConfigID: model.ID,
	})
	require.NoError(t, err)

	// Simulate a chat that failed with an error.
	errorMessage := "stream response: status 500: internal server error"
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusError,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{String: errorMessage, Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, chat.Status)
	require.Equal(t, sql.NullString{String: errorMessage, Valid: true}, chat.LastError)

	// Verify the error is persisted when re-read from the database.
	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, fromDB.Status)
	require.Equal(t, sql.NullString{String: errorMessage, Valid: true}, fromDB.LastError)

	// Verify the error is cleared when the chat transitions to a
	// non-error status (e.g. pending after a retry).
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusPending,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusPending, chat.Status)
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
	user, org, model := seedChatDependencies(ctx, t, db)

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

	// The first event in the snapshot must be a status event.
	// The exact status depends on timing: CreateChat sets
	// pending, but the wake signal may trigger processing
	// before Subscribe is called.
	require.NotEmpty(t, snapshot)
	require.Equal(t, codersdk.ChatStreamEventTypeStatus, snapshot[0].Type)
	require.NotNil(t, snapshot[0].Status)
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
	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
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
		ListMCPTools(gomock.Any()).
		Return(workspacesdk.ListMCPToolsResponse{}, nil).
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

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
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
		require.FailNowf(t, "chat run failed", "last_error=%q", chatResult.LastError.String)
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

func TestDynamicToolCallPausesAndResumes(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Track streaming calls to the mock LLM.
	var streamedCallCount atomic.Int32
	var streamedCallsMu sync.Mutex
	streamedCalls := make([]chattest.OpenAIRequest, 0, 2)

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		// Non-streaming requests are title generation — return a
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Dynamic tools do not need a workspace connection, but the
	// chatd server always builds workspace tools. Use an active
	// server without an agent connection — the built-in tools
	// are never invoked because the only tool call targets our
	// dynamic tool.
	server := newActiveTestServer(t, db, ps)

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
		chatResult.Status, chatResult.LastError.String)

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
		require.FailNowf(t, "chat run failed", "last_error=%q", chatResult.LastError.String)
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
	server := newActiveTestServer(t, db, ps)

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
		require.FailNowf(t, "chat run failed", "last_error=%q", chatResult.LastError.String)
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
			// response — a built-in tool (read_file) and a
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
	server := newActiveTestServer(t, db, ps)

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
		chatResult.Status, chatResult.LastError.String)

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
		require.FailNowf(t, "chat run failed", "last_error=%q", chatResult.LastError.String)
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
	server := newActiveTestServer(t, db, ps)

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
		chatResult.Status, chatResult.LastError.String)

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

func TestSubscribeNoPubsubNoDuplicateMessageParts(t *testing.T) {
	t.Parallel()

	// Use nil pubsub to force the no-pubsub path.
	db, _ := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, nil, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "no-dup-parts",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for any wake-triggered processing to settle before
	// subscribing, so the snapshot captures the final state.
	// The wake signal may trigger processOnce which will fail
	// (no LLM configured) and set the chat to error status.
	// Poll until the chat reaches a terminal state (not pending
	// and not running), then wait for the goroutine to finish.
	waitForChatProcessed(ctx, t, db, chat.ID, replica)

	snapshot, events, cancel, ok := replica.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Snapshot should have events (at minimum: status + message).
	require.NotEmpty(t, snapshot)

	// The events channel should NOT immediately produce any
	// events — the snapshot already contained everything. Before
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
	user, org, model := seedChatDependencies(ctx, t, db)

	// Create a chat — this inserts one initial "user" message.
	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "after-id-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("first")},
	})
	require.NoError(t, err)

	// Insert two more messages so we have three total visible
	// messages (the initial user message plus these two).
	secondContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("second"),
	})
	require.NoError(t, err)

	msg2Results, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(secondContent.RawMessage)},
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
	msg2 := msg2Results[0]

	thirdContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("third"),
	})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chat.ID,
		CreatedBy:           []uuid.UUID{uuid.Nil},
		ModelConfigID:       []uuid.UUID{model.ID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Content:             []string{string(thirdContent.RawMessage)},
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
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
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

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

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
			lastError = *chatResult.LastError
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
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:         deploymentValues,
		IncludeProvisionerDaemon: true,
	})
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
	// agent — the echo provisioner creates new agent rows for each
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

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

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
			lastError = *chatResult.LastError
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)
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

	// Close the inactive server so its wake-triggered processing
	// stops and releases the chat. Then reset to pending so the
	// active server (created below) can acquire it cleanly.
	require.NoError(t, inactive.Close())
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusPending,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
		LastError:   sql.NullString{},
	})
	require.NoError(t, err)

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
	_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
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
		require.FailNowf(t, "chat failed", "last_error=%q", chatResult.LastError.String)
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

func TestHeartbeatBumpsWorkspaceUsage(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("ok")
		}
		// Block until the request context is canceled so the chat
		// stays in a processing state long enough for heartbeats
		// to fire.
		chunks := make(chan chattest.OpenAIChunk)
		go func() {
			defer close(chunks)
			<-req.Context().Done()
		}()
		return chattest.OpenAIResponse{StreamingChunks: chunks}
	}))

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
	// Build deadline is 30 minutes in the past — close enough to
	// be bumped by the default 1-hour activity bump.
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:       ws.ID,
		TemplateVersionID: tv.ID,
		JobID:             pj.ID,
		Transition:        database.WorkspaceTransitionStart,
		Deadline:          dbtime.Now().Add(-30 * time.Minute),
	})
	originalDeadline := build.Deadline

	// Set up a short heartbeat interval and a UsageTracker that
	// flushes frequently so last_used_at gets updated in the DB.
	flushTick := make(chan time.Time)
	flushDone := make(chan int, 1)
	tracker := workspacestats.NewTracker(db,
		workspacestats.TrackerWithTickFlush(flushTick, flushDone),
		workspacestats.TrackerWithLogger(slogtest.Make(t, nil)),
	)
	t.Cleanup(func() { tracker.Close() })

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	// Wrap the database with dbauthz so the chatd server's
	// AsChatd context is enforced on every query, matching
	// production behavior.
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   authzDB,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
		ChatHeartbeatInterval:      100 * time.Millisecond,
		UsageTracker:               tracker,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	// Create a chat WITHOUT a workspace, the normal starting state.
	// In production, CreateChat is called from the HTTP handler with
	// the authenticated user's context. Here we use AsChatd since
	// the chatd server processes everything under that role.
	chatCtx := dbauthz.AsChatd(ctx)
	chat, err := server.CreateChat(chatCtx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "usage-tracking-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Wait for the chat to start processing and at least one
	// heartbeat to fire.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, listErr := db.GetChatByID(ctx, chat.ID)
		if listErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning &&
			fromDB.HeartbeatAt.Valid &&
			fromDB.HeartbeatAt.Time.After(fromDB.CreatedAt)
	}, testutil.IntervalFast,
		"chat should be running with at least one heartbeat")

	// Flush the tracker and verify nothing was tracked yet
	// (no workspace linked).
	testutil.RequireSend(ctx, t, flushTick, time.Now())
	count := testutil.RequireReceive(ctx, t, flushDone)
	require.Equal(t, 0, count,
		"expected no workspaces to be flushed before association")

	// Link the workspace to the chat in the DB, simulating what
	// the create_workspace tool does mid-conversation.
	_, err = db.UpdateChatWorkspaceBinding(ctx, database.UpdateChatWorkspaceBindingParams{
		WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
		ID:          chat.ID,
	})
	require.NoError(t, err)

	// The heartbeat re-reads the workspace association from the DB
	// on each tick. Wait for the tracker to pick it up.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case flushTick <- time.Now():
		case <-ctx.Done():
			return false
		}
		select {
		case c := <-flushDone:
			return c > 0
		case <-ctx.Done():
			return false
		}
	}, testutil.IntervalMedium,
		"expected usage tracker to flush the late-associated workspace")

	// Verify the workspace's last_used_at was actually updated.
	updatedWs, err := db.GetWorkspaceByID(ctx, ws.ID)
	require.NoError(t, err)
	require.True(t, updatedWs.LastUsedAt.After(ws.LastUsedAt),
		"workspace last_used_at should have been bumped")

	// Verify the workspace build deadline was also extended.
	// The SQL only writes when 5% of the deadline has elapsed —
	// most calls perform a read-only CTE lookup. Wider ±2
	// minute tolerance than activitybump_test.go because the bump
	// happens asynchronously via the heartbeat goroutine.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		updatedBuild, buildErr := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, ws.ID)
		if buildErr != nil || !updatedBuild.Deadline.After(originalDeadline) {
			return false
		}
		now := dbtime.Now()
		return updatedBuild.Deadline.After(now.Add(time.Hour-2*time.Minute)) &&
			updatedBuild.Deadline.Before(now.Add(time.Hour+2*time.Minute))
	}, testutil.IntervalFast,
		"workspace build deadline should have been bumped to ~now+1h")
}

func TestHeartbeatNoWorkspaceNoBump(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)
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
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
		ChatHeartbeatInterval:      100 * time.Millisecond,
	})
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

	// Wait for the chat to be acquired and at least one heartbeat
	// to fire.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, listErr := db.GetChatByID(ctx, chat.ID)
		if listErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusRunning &&
			fromDB.HeartbeatAt.Valid &&
			fromDB.HeartbeatAt.Time.After(fromDB.CreatedAt)
	}, testutil.IntervalFast,
		"chat should be running with at least one heartbeat")

	// Flush the tracker. Since no workspace was linked, count
	// should be 0.
	testutil.RequireSend(ctx, t, usageTickCh, time.Now())
	count := testutil.RequireReceive(ctx, t, flushCh)
	require.Equal(t, 0, count, "expected no workspaces to be flushed when chat has no workspace")
}

// waitForChatProcessed waits for a wake-triggered processOnce to
// fully complete for the given chat. It polls until the chat leaves
// both pending and running states (meaning processChat has finished
// its cleanup and updated the DB), then calls WaitUntilIdleForTest.
//
// Waiting for a terminal state (not just "not pending") avoids a
// WaitGroup Add/Wait race: AcquireChats changes the DB status to
// running before processOnce calls inflight.Add(1). If we only
// waited for status != pending, we could call Wait() while Add(1)
// hasn't happened yet.
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
		// Wait until the chat reaches a terminal state — neither
		// pending (waiting to be acquired) nor running (being
		// processed). This guarantees that inflight.Add(1) has
		// already been called by processOnce.
		return c.Status != database.ChatStatusPending &&
			c.Status != database.ChatStatusRunning
	}, testutil.WaitShort, testutil.IntervalFast)
	chatd.WaitUntilIdleForTest(server)
}

func newTestServer(
	t *testing.T,
	db database.Store,
	ps dbpubsub.Pubsub,
	replicaID uuid.UUID,
) *chatd.Server {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  replicaID,
		Pubsub:                     ps,
		PendingChatAcquireInterval: testutil.WaitLong,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
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
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  replicaID,
		Pubsub:                     ps,
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
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
	}
	for _, o := range overrides {
		o(&cfg)
	}
	server := chatd.New(cfg)
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})
	return server
}

func seedChatDependencies(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()
	openAIURL := chattest.OpenAI(t)
	return seedChatDependenciesWithProvider(ctx, t, db, "openai", openAIURL)
}

// seedChatDependenciesWithProvider creates a user, organization,
// chat provider, and model config for the given provider type and
// base URL.
func seedChatDependenciesWithProvider(
	ctx context.Context,
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
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             provider,
		DisplayName:          provider,
		APIKey:               "test-key",
		BaseUrl:              baseURL,
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)
	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             provider,
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

func seedChatDependenciesWithProviderPolicy(
	ctx context.Context,
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
	providerConfig, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:                   provider,
		DisplayName:                provider,
		APIKey:                     apiKey,
		BaseUrl:                    baseURL,
		CreatedBy:                  uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:                    true,
		CentralApiKeyEnabled:       centralAPIKeyEnabled,
		AllowUserApiKey:            allowUserAPIKey,
		AllowCentralApiKeyFallback: allowCentralAPIKeyFallback,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             provider,
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

	return user, org, providerConfig, model
}

func waitForTerminalChatStatusEvent(
	ctx context.Context,
	t *testing.T,
	events <-chan codersdk.ChatStreamEvent,
) codersdk.ChatStatus {
	t.Helper()

	var terminalStatus codersdk.ChatStatus
	testutil.Eventually(ctx, t, func(context.Context) bool {
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return false
				}
				if event.Type != codersdk.ChatStreamEventTypeStatus || event.Status == nil {
					continue
				}
				if event.Status.Status == codersdk.ChatStatusWaiting || event.Status.Status == codersdk.ChatStatusError {
					terminalStatus = event.Status.Status
					return true
				}
			default:
				return false
			}
		}
	}, testutil.IntervalFast)

	return terminalStatus
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
	ctx context.Context,
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

	modelConfig, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             provider,
		Model:                model,
		DisplayName:          model,
		CreatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: userID, Valid: true},
		Enabled:              true,
		IsDefault:            false,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              options,
	})
	require.NoError(t, err)
	return modelConfig
}

func insertUserTextMessage(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	userID uuid.UUID,
	modelConfigID uuid.UUID,
	text string,
) {
	t.Helper()

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(text)})
	require.NoError(t, err)

	_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{userID},
		ModelConfigID:       []uuid.UUID{modelConfigID},
		Role:                []database.ChatMessageRole{database.ChatMessageRoleUser},
		Content:             []string{string(content.RawMessage)},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
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
		ProviderResponseID:  []string{""},
	})
	require.NoError(t, err)
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
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: res.ID,
	})
	return ws, agent
}

func setOpenAIProviderBaseURL(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	baseURL string,
) {
	t.Helper()

	provider, err := db.GetChatProviderByProvider(ctx, "openai")
	require.NoError(t, err)

	_, err = db.UpdateChatProvider(ctx, database.UpdateChatProviderParams{
		ID:                         provider.ID,
		DisplayName:                provider.DisplayName,
		APIKey:                     provider.APIKey,
		BaseUrl:                    baseURL,
		ApiKeyKeyID:                provider.ApiKeyKeyID,
		Enabled:                    provider.Enabled,
		CentralApiKeyEnabled:       provider.CentralApiKeyEnabled,
		AllowUserApiKey:            provider.AllowUserApiKey,
		AllowCentralApiKeyFallback: provider.AllowCentralApiKeyFallback,
	})
	require.NoError(t, err)
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
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-no-push",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

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

	// Interrupt the chat.
	updated := server.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)

	// Wait for the chat to finish processing and return to waiting.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusWaiting && !fromDB.WorkerID.Valid
	}, testutil.IntervalFast)

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
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(ctx, t, db)
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
	serverA := chatd.New(chatd.Config{
		Logger:                     loggerA,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
	})
	t.Cleanup(func() {
		require.NoError(t, serverA.Close())
	})

	user, org, model := seedChatDependencies(ctx, t, db)
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

	require.Eventually(t, func() bool {
		fromDB, dbErr := db.GetChatByID(ctx, chat.ID)
		if dbErr != nil {
			return false
		}
		return fromDB.Status == database.ChatStatusPending &&
			!fromDB.WorkerID.Valid &&
			!fromDB.LastError.Valid
	}, testutil.WaitMedium, testutil.IntervalFast)

	loggerB := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	serverB := chatd.New(chatd.Config{
		Logger:                     loggerB,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitLong,
	})
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
	const summaryText = "Completed task and verified all tests pass."

	var nonStreamingRequests atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			nonStreamingRequests.Add(1)
			return chattest.OpenAINonStreamingResponse(summaryText)
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks(assistantText)...,
		)
	})

	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	_, err := server.CreateChat(ctx, chatd.CreateOptions{
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
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return mockPush.dispatchCount.Load() >= 1
	}, testutil.IntervalFast)

	msg := mockPush.getLastMessage()
	require.Equal(t, summaryText, msg.Body,
		"push body should be the LLM-generated summary")
	require.NotEqual(t, "Agent has finished running.", msg.Body,
		"push body should not use the default fallback text")
	require.Equal(t, int32(1), nonStreamingRequests.Load(),
		"expected exactly one non-streaming request for push summary generation")
}

func TestSuccessfulChatSendsWebPushFallbackWithoutSummaryForEmptyAssistantText(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var nonStreamingRequests atomic.Int32
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			nonStreamingRequests.Add(1)
			return chattest.OpenAINonStreamingResponse("unexpected summary request")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("   ")...,
		)
	})

	mockPush := &mockWebpushDispatcher{}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
		WebpushDispatcher:          mockPush,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	_, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "empty-summary-push-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("do the thing")},
	})
	require.NoError(t, err)

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		return mockPush.dispatchCount.Load() >= 1
	}, testutil.IntervalFast)

	msg := mockPush.getLastMessage()
	require.Equal(t, "Agent has finished running.", msg.Body,
		"push body should fall back when the final assistant text is empty")
	require.Equal(t, int32(0), nonStreamingRequests.Load(),
		"push summary should not be requested when final assistant text has no usable text")
}

func TestComputerUseSubagentToolsAndModel(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Track tools and model from the Anthropic LLM calls (the
	// computer use child chat). We use a raw HTTP handler because
	// the chattest AnthropicRequest struct does not capture tools.
	type anthropicCall struct {
		Model string
		Tools []string
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
				Model: req.Model,
				Tools: names,
			})
			anthropicMu.Unlock()

			if !req.Stream {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":          "msg-test",
					"type":        "message",
					"role":        "assistant",
					"model":       chattool.ComputerUseModelName,
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
						"model": chattool.ComputerUseModelName,
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
	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Add an Anthropic provider pointing to our mock server.
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "anthropic",
		DisplayName:          "Anthropic",
		APIKey:               "test-anthropic-key",
		BaseUrl:              anthropicSrv.URL,
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	err = db.UpsertChatDesktopEnabled(ctx, true)
	require.NoError(t, err)

	// Build workspace + agent records so getWorkspaceConn can
	// resolve the agent for the computer use child.
	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	// Mock agent connection that returns valid display dimensions
	// for the initial screenshot check in the computer use path.
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().
		ListMCPTools(gomock.Any()).
		Return(workspacesdk.ListMCPToolsResponse{}, nil).
		AnyTimes()
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
		// Ensure the Anthropic mock received at least one call.
		anthropicMu.Lock()
		n := len(anthropicCalls)
		anthropicMu.Unlock()
		return n >= 1
	}, testutil.WaitLong, testutil.IntervalFast)

	anthropicMu.Lock()
	calls := append([]anthropicCall(nil), anthropicCalls...)
	anthropicMu.Unlock()

	require.NotEmpty(t, calls,
		"expected at least one Anthropic LLM call")

	childModel := calls[0].Model
	childTools := calls[0].Tools

	// 1. Verify the model is the computer use model.
	require.Equal(t, chattool.ComputerUseModelName, childModel,
		"computer use subagent should use %s",
		chattool.ComputerUseModelName)

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
		"create_workspace", "start_workspace",
	}
	for _, tool := range workspaceProvisioningTools {
		require.NotContains(t, childTools, tool,
			"computer use subagent should NOT have workspace "+
				"provisioning tool %q", tool)
	}

	// 5. Verify subagent tools are NOT present.
	subagentTools := []string{
		"spawn_agent",
		"wait_agent", "message_agent", "close_agent",
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
	server := chatd.New(chatd.Config{
		Logger:                     logger,
		Database:                   db,
		ReplicaID:                  uuid.New(),
		Pubsub:                     ps,
		PendingChatAcquireInterval: 10 * time.Millisecond,
		InFlightChatStaleAfter:     testutil.WaitSuperLong,
	})
	t.Cleanup(func() {
		require.NoError(t, server.Close())
	})

	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "interrupt-persist-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Subscribe to the chat's event stream so we can observe
	// message_part events — proof the chatloop has actually
	// processed the streamed chunks.
	_, events, subCancel, ok := server.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	defer subCancel()

	// Wait for the mock to finish sending chunks.
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		select {
		case <-chunksDelivered:
			return true
		default:
			return false
		}
	}, testutil.IntervalFast)

	// Drain the event channel until we see a message_part event,
	// which means the chatloop has consumed and published the chunk.
	gotMessagePart := false
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		for {
			select {
			case ev := <-events:
				if ev.Type == codersdk.ChatStreamEventTypeMessagePart {
					gotMessagePart = true
					return true
				}
			default:
				return gotMessagePart
			}
		}
	}, testutil.IntervalFast)
	require.True(t, gotMessagePart, "should have received at least one message_part event")

	// Now interrupt the chat — the chatloop has processed content.
	updated := server.InterruptChat(ctx, chat)
	require.Equal(t, database.ChatStatusWaiting, updated.Status)

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
		ctx,
		t,
		db,
		"openai-compat",
		openAIURL,
		"",
		false,
		true,
		false,
	)
	_, err := db.UpsertUserChatProviderKey(ctx, database.UpsertUserChatProviderKeyParams{
		UserID:         user.ID,
		ChatProviderID: provider.ID,
		APIKey:         userAPIKey,
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

	_, events, cancel, ok := creator.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	_ = newActiveTestServer(t, db, ps)

	terminalStatus := waitForTerminalChatStatusEvent(ctx, t, events)
	require.Equal(t, codersdk.ChatStatusWaiting, terminalStatus)

	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chatResult.Status)
	require.False(t, chatResult.LastError.Valid)

	authHeadersMu.Lock()
	recordedAuthHeaders := append([]string(nil), authHeaders...)
	authHeadersMu.Unlock()
	require.Contains(t, recordedAuthHeaders, "Bearer "+userAPIKey)
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
		ctx,
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

	_, events, cancel, ok := creator.Subscribe(ctx, chat.ID, nil, 0)
	require.True(t, ok)
	t.Cleanup(cancel)

	_ = newActiveTestServer(t, db, ps)

	terminalStatus := waitForTerminalChatStatusEvent(ctx, t, events)
	require.Equal(t, codersdk.ChatStatusError, terminalStatus)

	chatResult := waitForTerminalChat(ctx, t, db, chat.ID)
	require.Equal(t, database.ChatStatusError, chatResult.Status)
	require.True(t, chatResult.LastError.Valid, "LastError should be set")
	require.NotEmpty(t, chatResult.LastError.String)
	require.NotContains(t, chatResult.LastError.String, "panicked")
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

	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("Panic recovery test")
		}
		return chattest.OpenAIStreamingResponse(
			chattest.OpenAITextChunks("hello")...,
		)
	})

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Pass the panic wrapper to the server, but use the real
	// database for seeding so those operations don't panic.
	server := newActiveTestServer(t, panicWrapper, ps)

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

	// Enable the panic now that CreateChat's InTx has completed.
	// The next InTx call is PersistStep inside the chatloop,
	// running synchronously on the processChat goroutine.
	panicWrapper.enablePanic()

	// Wait for the panic to be recovered and the chat to
	// transition to error status.
	var chatResult database.Chat
	require.Eventually(t, func() bool {
		got, getErr := db.GetChatByID(ctx, chat.ID)
		if getErr != nil {
			return false
		}
		chatResult = got
		return got.Status == database.ChatStatusError
	}, testutil.WaitLong, testutil.IntervalFast)

	require.True(t, chatResult.LastError.Valid, "LastError should be set")
	require.Contains(t, chatResult.LastError.String, "chat processing panicked")
	require.Contains(t, chatResult.LastError.String, "intentional test panic")
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Seed the MCP server config in the database. This must
	// happen after seedChatDependencies so user.ID exists for
	// the foreign key.
	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:   "Test MCP",
		Slug:          "test-mcp",
		Url:           mcpTS.URL,
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

	ws, dbAgent := seedWorkspaceWithAgent(t, db, user.ID)

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()
	mockConn.EXPECT().ContextConfig(gomock.Any()).
		Return(workspacesdk.ContextConfigResponse{}, xerrors.New("not supported")).AnyTimes()
	mockConn.EXPECT().ListMCPTools(gomock.Any()).
		Return(workspacesdk.ListMCPToolsResponse{}, nil).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()

	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
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
		require.FailNowf(t, "chat failed", "last_error=%q", chatResult.LastError.String)
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:     "Plan Mode MCP",
		Slug:            "plan-mode-mcp",
		Url:             mcpTS.URL,
		Transport:       "streamable_http",
		AuthType:        "none",
		Availability:    "default_off",
		Enabled:         true,
		AllowInPlanMode: true,
		ToolAllowList:   []string{},
		ToolDenyList:    []string{},
		CreatedBy:       user.ID,
		UpdatedBy:       user.ID,
	})
	require.NoError(t, err)

	server := newActiveTestServer(t, db, ps)

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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:     "Plan Workflow MCP",
		Slug:            "plan-workflow-mcp",
		Url:             mcpTS.URL,
		Transport:       "streamable_http",
		AuthType:        "none",
		Availability:    "default_off",
		Enabled:         true,
		AllowInPlanMode: true,
		ToolAllowList:   []string{},
		ToolDenyList:    []string{},
		CreatedBy:       user.ID,
		UpdatedBy:       user.ID,
	})
	require.NoError(t, err)

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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	// Seed the MCP server config with OAuth2 auth pointing to our
	// mock token endpoint.
	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:    "Authed MCP",
		Slug:           "authed-mcp",
		Url:            mcpTS.URL,
		Transport:      "streamable_http",
		AuthType:       "oauth2",
		OAuth2ClientID: "test-client-id",
		OAuth2TokenURL: tokenSrv.URL,
		Availability:   "default_off",
		Enabled:        true,
		ToolAllowList:  []string{},
		ToolDenyList:   []string{},
		CreatedBy:      user.ID,
		UpdatedBy:      user.ID,
	})
	require.NoError(t, err)

	// Seed an expired OAuth2 token with a valid refresh_token.
	_, err = db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
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
	mockConn.EXPECT().ListMCPTools(gomock.Any()).
		Return(workspacesdk.ListMCPToolsResponse{}, nil).AnyTimes()
	mockConn.EXPECT().LS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(workspacesdk.LSResponse{}, nil).AnyTimes()
	mockConn.EXPECT().ReadFile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(io.NopCloser(strings.NewReader("")), "", nil).AnyTimes()
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
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
		require.FailNowf(t, "chat failed", "last_error=%q", chatResult.LastError.String)
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

	// The LLM just replies with text — no tool calls.
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

	mcpConfig, err := db.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
		DisplayName:    "Broken MCP",
		Slug:           "broken-mcp",
		Url:            "http://127.0.0.1:0/does-not-exist",
		Transport:      "streamable_http",
		AuthType:       "oauth2",
		OAuth2ClientID: "test-client-id",
		OAuth2TokenURL: tokenSrv.URL,
		Availability:   "default_off",
		Enabled:        true,
		ToolAllowList:  []string{},
		ToolDenyList:   []string{},
		CreatedBy:      user.ID,
		UpdatedBy:      user.ID,
	})
	require.NoError(t, err)

	_, err = db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
		MCPServerConfigID: mcpConfig.ID,
		UserID:            user.ID,
		AccessToken:       "old-expired-token",
		RefreshToken:      "old-refresh-token",
		TokenType:         "Bearer",
		Expiry:            sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	})
	require.NoError(t, err)

	server := newActiveTestServer(t, db, ps)

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
		require.FailNowf(t, "chat should not fail", "last_error=%q", chatResult.LastError.String)
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
	//  2. read_template  (blocked template — should fail)
	//  3. read_template  (allowed template — should succeed)
	//  4. create_workspace (blocked template — should fail)
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

	user, org, model := seedChatDependenciesWithProvider(ctx, t, db, "openai-compat", openAIURL)

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
		require.FailNowf(t, "chat run failed", "last_error=%q", chatResult.LastError.String)
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

// TestSignalWakeImmediateAcquisition verifies that CreateChat triggers
// immediate processing via signalWake without waiting for the polling
// ticker to fire. The ticker interval is set to an hour so it never
// fires during the test — any processing must come from the wake
// channel.
func TestSignalWakeImmediateAcquisition(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	processed := make(chan struct{})
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		// Signal that the LLM was reached — this proves the chat
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
	})

	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	// CreateChat sets status=pending and calls signalWake().
	chat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		Title:              "wake-test",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// The chat should be processed immediately — the LLM handler
	// closes the `processed` channel when it receives a streaming
	// request. Without signalWake this would hang forever because
	// the 1-hour ticker never fires.
	testutil.TryReceive(ctx, t, processed)

	chatd.WaitUntilIdleForTest(server)

	// Verify the chat was fully processed.
	fromDB, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, fromDB.Status,
		"chat should be in waiting status after processing completes")
}

// TestSignalWakeSendMessage verifies that SendMessage on an idle chat
// triggers immediate processing via signalWake.
func TestSignalWakeSendMessage(t *testing.T) {
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
	})

	user, org, model := seedChatDependencies(ctx, t, db)
	setOpenAIProviderBaseURL(ctx, t, db, openAIURL)

	// CreateChat triggers wake -> processes first turn.
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

	// Now send a follow-up message — this should also be
	// processed immediately via signalWake.
	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:  chat.ID,
		Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("second")},
	})
	require.NoError(t, err)

	testutil.TryReceive(ctx, t, secondProcessed)
	chatd.WaitUntilIdleForTest(server)

	// Both turns processed — verify second request reached the LLM.
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
	deploymentValues := coderdtest.DeploymentValues(t)
	deploymentValues.Experiments = []string{string(codersdk.ExperimentAgents)}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues:              deploymentValues,
		IncludeProvisionerDaemon:      true,
		ChatdInstructionLookupTimeout: testutil.WaitLong,
	})
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

	_ = agenttest.New(t, client.URL, agentToken)
	coderdtest.NewWorkspaceAgentWaiter(t, client, workspace.ID).Wait()

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

	_, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider: "openai-compat",
		APIKey:   "test-api-key",
		BaseURL:  openAIURL,
	})
	require.NoError(t, err)

	contextLimit := int64(4096)
	isDefault := true
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:     "openai-compat",
		Model:        "gpt-4o-mini",
		ContextLimit: &contextLimit,
		IsDefault:    &isDefault,
	})
	require.NoError(t, err)

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

func TestSendMessageRejectsArchivedChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "send-archived",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	err = replica.ArchiveChat(ctx, chat)
	require.NoError(t, err)

	_, err = replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("should fail")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.ErrorIs(t, err, chatd.ErrChatArchived)
}

func TestEditMessageRejectsArchivedChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "edit-archived",
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

	err = replica.ArchiveChat(ctx, chat)
	require.NoError(t, err)

	_, err = replica.EditMessage(ctx, chatd.EditMessageOptions{
		ChatID:          chat.ID,
		EditedMessageID: messages[0].ID,
		Content:         []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")},
	})
	require.ErrorIs(t, err, chatd.ErrChatArchived)
}

func TestPromoteQueuedRejectsArchivedChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "promote-archived",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	// Queue a message by setting the chat to running first.
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusRunning,
		WorkerID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		StartedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		HeartbeatAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	require.NoError(t, err)

	queuedResult, err := replica.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:       chat.ID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText("queued")},
		BusyBehavior: chatd.SendMessageBusyBehaviorQueue,
	})
	require.NoError(t, err)
	require.True(t, queuedResult.Queued)

	// Move back to waiting, then archive.
	chat, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:          chat.ID,
		Status:      database.ChatStatusWaiting,
		WorkerID:    uuid.NullUUID{},
		StartedAt:   sql.NullTime{},
		HeartbeatAt: sql.NullTime{},
	})
	require.NoError(t, err)

	err = replica.ArchiveChat(ctx, chat)
	require.NoError(t, err)

	_, err = replica.PromoteQueued(ctx, chatd.PromoteQueuedOptions{
		ChatID:          chat.ID,
		QueuedMessageID: queuedResult.QueuedMessage.ID,
		CreatedBy:       user.ID,
	})
	require.ErrorIs(t, err, chatd.ErrChatArchived)
}

func TestSubmitToolResultsRejectsArchivedChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	replica := newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	chat, err := replica.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:            user.ID,
		OrganizationID:     org.ID,
		Title:              "submit-tool-archived",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)

	err = replica.ArchiveChat(ctx, chat)
	require.NoError(t, err)

	// Set requires_action so the test exercises a realistic
	// scenario where SubmitToolResults would be called.
	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     chat.ID,
		Status: database.ChatStatusRequiresAction,
	})
	require.NoError(t, err)

	err = replica.SubmitToolResults(ctx, chatd.SubmitToolResultsOptions{
		ChatID:        chat.ID,
		UserID:        user.ID,
		ModelConfigID: model.ID,
		Results: []codersdk.ToolResult{{
			ToolCallID: "fake-tool-call-id",
			Output:     json.RawMessage(`{"result":"ignored"}`),
		}},
	})
	require.ErrorIs(t, err, chatd.ErrChatArchived)
}

func TestAcquireChatsSkipsArchivedPendingChat(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	_ = newTestServer(t, db, ps, uuid.New())

	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(ctx, t, db)

	archivedChat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		Title:             "acquire-skip-archived",
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
	})
	require.NoError(t, err)

	// Archive the chat, then force it to pending.
	_, err = db.ArchiveChatByID(ctx, archivedChat.ID)
	require.NoError(t, err)

	_, err = db.UpdateChatStatus(ctx, database.UpdateChatStatusParams{
		ID:     archivedChat.ID,
		Status: database.ChatStatusPending,
	})
	require.NoError(t, err)

	// Insert a second, non-archived pending chat so the result
	// slice is non-empty and the assertion is not vacuously true.
	activeChat, err := db.InsertChat(ctx, database.InsertChatParams{
		OwnerID:           user.ID,
		OrganizationID:    org.ID,
		Title:             "acquire-active",
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusPending,
		ClientType:        database.ChatClientTypeUi,
	})
	require.NoError(t, err)

	now := time.Now()
	acquired, err := db.AcquireChats(ctx, database.AcquireChatsParams{
		WorkerID:  uuid.New(),
		StartedAt: now,
		NumChats:  10,
	})
	require.NoError(t, err)
	require.Len(t, acquired, 1, "only the non-archived chat should be acquired")
	require.Equal(t, activeChat.ID, acquired[0].ID)
}
