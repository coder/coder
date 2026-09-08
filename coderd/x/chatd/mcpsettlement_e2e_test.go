package chatd_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// controllableMCPServer wraps an MCP server with a gate that blocks
// tool registration until explicitly released.
type controllableMCPServer struct {
	server     *mcp.Server
	httpServer *httptest.Server
	toolName   string
	nonce      string
	releaseCh  chan struct{}
	arrived    sync.Once
	arrivedCh  chan struct{}
	invokedCh  chan struct{}
	released   atomic.Bool
	invoked    atomic.Bool
}

func newControllableMCPServer(t *testing.T, toolName string) *controllableMCPServer {
	t.Helper()

	nonce := uuid.NewString()[:8]
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "test-settlement-mcp",
		Version: "1.0.0",
	}, nil)

	ctrl := &controllableMCPServer{
		server:    srv,
		toolName:  toolName,
		nonce:     nonce,
		releaseCh: make(chan struct{}),
		arrivedCh: make(chan struct{}),
		invokedCh: make(chan struct{}),
	}

	srv.AddTool(&mcp.Tool{
		Name:        toolName,
		Description: "Test fixture tool for MCP settlement gate",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "The input string",
				},
			},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if ctrl.invoked.CompareAndSwap(false, true) {
			close(ctrl.invokedCh)
		}
		var arguments map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &arguments)
		input, _ := arguments["input"].(string)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf("nonce=%s input=%s", nonce, input),
			}},
		}, nil
	})

	realHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return srv
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if ctrl.released.Load() {
			realHandler.ServeHTTP(w, r)
			return
		}
		// Signal arrival exactly once via sync.Once (race-safe).
		ctrl.arrived.Do(func() { close(ctrl.arrivedCh) })
		select {
		case <-ctrl.releaseCh:
			realHandler.ServeHTTP(w, r)
		case <-r.Context().Done():
			http.Error(w, "canceled", http.StatusServiceUnavailable)
		}
	})

	ctrl.httpServer = httptest.NewServer(mux)
	t.Cleanup(func() {
		ctrl.release()
		ctrl.httpServer.Close()
	})

	return ctrl
}

func (c *controllableMCPServer) release() {
	if c.released.CompareAndSwap(false, true) {
		close(c.releaseCh)
	}
}

func (c *controllableMCPServer) url() string {
	return c.httpServer.URL
}

func writeMCPConfigFile(t *testing.T, dir, serverName, serverURL string) string {
	t.Helper()
	config := map[string]any{
		"mcpServers": map[string]any{
			serverName: map[string]any{
				"url": serverURL,
			},
		},
	}
	data, err := json.Marshal(config)
	require.NoError(t, err)
	path := filepath.Join(dir, ".mcp.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// mcpSetupResult holds the common infrastructure for an MCP E2E subtest.
type mcpSetupResult struct {
	client    *codersdk.Client
	expClient *codersdk.ExperimentalClient
	api       *coderd.API
	user      codersdk.CreateFirstUserResponse
}

// setupMCPTest creates a coderd instance with provisioner and AI bridge.
func setupMCPTest(t *testing.T) mcpSetupResult {
	t.Helper()
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:         coderdtest.DeploymentValues(t),
		IncludeProvisionerDaemon: true,
	})
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
	user := coderdtest.CreateFirstUser(t, client)
	return mcpSetupResult{
		client:    client,
		expClient: codersdk.NewExperimentalClient(client),
		api:       api,
		user:      user,
	}
}

func TestMCPSettlementGate_EndToEnd(t *testing.T) {
	t.Parallel()

	// DelayedRegistration_FindToolsInvoke verifies the full flow:
	// create_workspace -> gate blocks on pending MCP -> release ->
	// model discovers tool via find_tools -> invokes it -> nonce verified.
	t.Run("DelayedRegistration_FindToolsInvoke", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		mcpCtrl := newControllableMCPServer(t, "gate-echo")
		configDir := t.TempDir()
		configPath := writeMCPConfigFile(t, configDir, "gate-srv", mcpCtrl.url())

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		_ = agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: configPath,
			}
		})

		workspaceName := "mcp-gate-" + uuid.NewString()[:8]
		createWorkspaceArgs := fmt.Sprintf(
			`{"template_id":%q,"name":%q}`,
			template.ID.String(), workspaceName,
		)

		// modelCalledWhilePending is set if the post-create_workspace
		// generation fires before MCP registration was released.
		var modelCalledWhilePending atomic.Bool
		var streamedCallCount atomic.Int32

		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("MCP gate test")
			}

			callNum := streamedCallCount.Add(1)
			switch callNum {
			case 1:
				// First: model calls create_workspace.
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("create_workspace", createWorkspaceArgs),
				)
			case 2:
				// Second: after create_workspace completes and
				// WaitForMCPSettled returns, this is the first
				// actionable generation. Record whether MCP was
				// still pending.
				if !mcpCtrl.released.Load() {
					modelCalledWhilePending.Store(true)
				}

				// With MCPToolSearch experiment on, workspace MCP
				// tools are deferred behind find_tools. Call it.
				findArgs := `{"queries":["gate-echo"]}`
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("find_tools", findArgs),
				)
			case 3:
				// Third: find_tools result is in history. The
				// activated workspace MCP tool should now be in
				// req.Tools. Invoke it.
				var foundTool string
				for _, tool := range req.Tools {
					name := openAIToolName(tool)
					if strings.Contains(name, "gate-echo") {
						foundTool = name
						break
					}
				}
				if foundTool != "" {
					return chattest.OpenAIStreamingResponse(
						chattest.OpenAIToolCallChunk(foundTool, `{"input":"hello"}`),
					)
				}
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("No MCP tool found after find_tools.")...,
				)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("Done with MCP tools.")...,
				)
			}
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		// Release MCP registration 3 seconds after the agent connects.
		// This proves the gate holds: the model's second streamed call
		// cannot fire until WaitForMCPSettled returns, which requires
		// mcp_settled=true, which requires release.
		var releaseTime atomic.Int64
		go func() {
			select {
			case <-mcpCtrl.arrivedCh:
				t.Log("MCP server received registration request; holding 3s")
				time.Sleep(3 * time.Second)
				releaseTime.Store(time.Now().UnixMilli())
				t.Log("Releasing MCP registration")
				mcpCtrl.release()
			case <-ctx.Done():
			}
		}()

		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Create a workspace and use its tools.",
				},
			},
		})
		require.NoError(t, err)

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should reach waiting status; last_error=%v", chatResult.LastError)

		require.False(t, modelCalledWhilePending.Load(),
			"actionable generation should not fire before MCP registration released")
		// Verify the release happened (proves the gate held for 3s).
		require.NotZero(t, releaseTime.Load(),
			"MCP should have been released during the test")

		// Verify the fixture tool was actually invoked.
		assert.True(t, mcpCtrl.invoked.Load(),
			"fixture MCP tool should have been invoked")

		// Verify nonce in chat messages.
		chatMsgs, err := setup.expClient.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var foundNonce bool
		for _, msg := range chatMsgs.Messages {
			for _, part := range msg.Content {
				if part.Type == codersdk.ChatMessagePartTypeToolResult {
					if strings.Contains(string(part.Result), mcpCtrl.nonce) {
						foundNonce = true
					}
				}
			}
		}
		assert.True(t, foundNonce,
			"expected nonce %q in chat tool results", mcpCtrl.nonce)
	})

	// APICreatedChat_PendingSnapshot creates a chat attached to a
	// workspace while MCP registration is still pending, proving the
	// generation preparer gate blocks until settlement.
	t.Run("APICreatedChat_PendingSnapshot", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		mcpCtrl := newControllableMCPServer(t, "api-echo")
		configDir := t.TempDir()
		configPath := writeMCPConfigFile(t, configDir, "api-srv", mcpCtrl.url())

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		workspace := coderdtest.CreateWorkspace(t, setup.client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, workspace.LatestBuild.ID)

		_ = agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: configPath,
			}
		})
		coderdtest.AwaitWorkspaceAgents(t, setup.client, workspace.ID)

		// Wait for the agent to reach the blocked MCP server.
		select {
		case <-mcpCtrl.arrivedCh:
			t.Log("MCP registration request arrived (still blocked)")
		case <-time.After(testutil.WaitLong):
			t.Fatal("agent never reached MCP server")
		}

		// Create chat while MCP is demonstrably pending.
		require.False(t, mcpCtrl.released.Load(),
			"MCP must still be pending when chat is created")

		var modelCalledWhilePending atomic.Bool
		var streamedCallCount atomic.Int32

		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("API chat test")
			}
			if streamedCallCount.Add(1) == 1 {
				if !mcpCtrl.released.Load() {
					modelCalledWhilePending.Store(true)
				}
			}
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("API chat response.")...,
			)
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			WorkspaceID:    &workspace.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Hello from API-created chat.",
				},
			},
		})
		require.NoError(t, err)

		// Release after a brief delay to prove the gate held.
		go func() {
			time.Sleep(2 * time.Second)
			t.Log("Releasing MCP registration")
			mcpCtrl.release()
		}()

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should settle; last_error=%v", chatResult.LastError)

		require.False(t, modelCalledWhilePending.Load(),
			"first generation should not fire while MCP registration was pending")
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(1),
			"model should have been called after release")
	})

	// NoMCPServers verifies zero-MCP workspaces complete quickly.
	t.Run("NoMCPServers", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		_ = agenttest.New(t, setup.client.URL, agentToken)

		workspaceName := "no-mcp-" + uuid.NewString()[:8]
		createWorkspaceArgs := fmt.Sprintf(
			`{"template_id":%q,"name":%q}`,
			template.ID.String(), workspaceName,
		)

		var chatStarted atomic.Bool
		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("No MCP test")
			}
			if streamedCallCount.Add(1) == 1 {
				chatStarted.Store(true)
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("create_workspace", createWorkspaceArgs),
				)
			}
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Done.")...,
			)
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		startTime := time.Now()
		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Create a workspace.",
				},
			},
		})
		require.NoError(t, err)

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		elapsed := time.Since(startTime)

		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should settle; last_error=%v", chatResult.LastError)
		require.True(t, chatStarted.Load(), "chat should have started")

		// The agent pushes mcp_settled=true on its first snapshot
		// (zero servers configured). The gate should return
		// immediately without adding the 15s timeout. On slow CI
		// runners provisioning alone can exceed 25s, so use a
		// bound that catches the 15s gate delay (>40s) without
		// being fragile on scheduling jitter.
		assert.Less(t, elapsed, testutil.WaitSuperLong,
			"no-MCP test should not block for 15s settlement timeout on top of provisioning")
	})

	// HungRegistrationFailsOpen verifies bounded fail-open: the model
	// is eventually called even when MCP registration never completes.
	t.Run("HungRegistrationFailsOpen", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		mcpCtrl := newControllableMCPServer(t, "hung-echo")
		// Do NOT call mcpCtrl.release() - simulates hung registration.

		configDir := t.TempDir()
		configPath := writeMCPConfigFile(t, configDir, "hung-srv", mcpCtrl.url())

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		_ = agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: configPath,
			}
		})

		workspace := coderdtest.CreateWorkspace(t, setup.client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, workspace.LatestBuild.ID)
		coderdtest.AwaitWorkspaceAgents(t, setup.client, workspace.ID)

		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("Hung MCP test")
			}
			streamedCallCount.Add(1)
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Proceeded despite hung MCP.")...,
			)
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		startTime := time.Now()
		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			WorkspaceID:    &workspace.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Hello with hung MCP.",
				},
			},
		})
		require.NoError(t, err)

		// The gate should time out at 15s and proceed (fail-open).
		// Use a generous poll window.
		var chatStatus string
		require.Eventually(t, func() bool {
			got, getErr := setup.expClient.GetChat(ctx, chat.ID)
			if getErr != nil {
				return false
			}
			chatStatus = string(got.Status)
			return chatStatus == "waiting" || chatStatus == "error"
		}, testutil.WaitSuperLong, testutil.IntervalFast,
			"chat should reach terminal state")

		elapsed := time.Since(startTime)

		// Require the model was actually called (fail-open success,
		// not merely a terminal error).
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(1),
			"model should have been called at least once (fail-open)")
		require.Equal(t, "waiting", chatStatus,
			"chat should succeed with fail-open, not error")

		// The gate should take roughly 15s, not run forever.
		assert.Less(t, elapsed, testutil.WaitSuperLong,
			"hung MCP should not block beyond the settlement timeout")
	})

	// RegistrationError verifies that a failing MCP server (500s)
	// settles quickly and the first turn proceeds.
	t.Run("RegistrationError", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}))
		defer badServer.Close()

		configDir := t.TempDir()
		configPath := writeMCPConfigFile(t, configDir, "bad-srv", badServer.URL)

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		_ = agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: configPath,
			}
		})

		workspace := coderdtest.CreateWorkspace(t, setup.client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, workspace.LatestBuild.ID)
		coderdtest.AwaitWorkspaceAgents(t, setup.client, workspace.ID)

		// Allow agent to attempt and fail registration.
		time.Sleep(3 * time.Second)

		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("Error MCP test")
			}
			streamedCallCount.Add(1)
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Proceeded after MCP error.")...,
			)
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			WorkspaceID:    &workspace.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Hello after MCP error.",
				},
			},
		})
		require.NoError(t, err)

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should settle after MCP error; last_error=%v", chatResult.LastError)
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(1),
			"model should have been called (error settles the gate)")
	})

	// ZeroMCPAgent verifies that a modern agent with no MCP config
	// pushes mcp_settled=true immediately (zero servers configured).
	// The gate returns without the 15s timeout. The unit test
	// NoSnapshot_RecentAgent_DelegatesToFullWait covers the
	// NULL/no-snapshot protocol-level paths.
	t.Run("ZeroMCPAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		// Start agent with no MCP config. The agent pushes
		// mcp_settled=true on its first snapshot (zero servers).
		_ = agenttest.New(t, setup.client.URL, agentToken)

		workspace := coderdtest.CreateWorkspace(t, setup.client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, workspace.LatestBuild.ID)
		coderdtest.AwaitWorkspaceAgents(t, setup.client, workspace.ID)

		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("Zero MCP test")
			}
			streamedCallCount.Add(1)
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Zero MCP response.")...,
			)
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		startTime := time.Now()
		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			WorkspaceID:    &workspace.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Hello zero MCP agent.",
				},
			},
		})
		require.NoError(t, err)

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		elapsed := time.Since(startTime)

		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should settle; last_error=%v", chatResult.LastError)
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(1),
			"model should have been called")

		// Zero-MCP agent pushes settled immediately. The gate should
		// not add the 15s timeout. Chat processing itself takes a
		// few seconds, so assert well under the 15s boundary.
		assert.Less(t, elapsed, 10*time.Second,
			"zero-MCP agent should not block for settlement timeout")
	})

	// Restart_FreshNonce verifies that after an agent restarts on the
	// same workspace (same agent row), the first chat discovers the
	// new agent's MCP tools (nonce B), not the old ones (nonce A).
	t.Run("Restart_FreshNonce", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		// Two controllable MCP servers with different nonces.
		mcpA := newControllableMCPServer(t, "restart-echo")
		mcpB := newControllableMCPServer(t, "restart-echo")
		require.NotEqual(t, mcpA.nonce, mcpB.nonce,
			"fixture nonces must differ to prove freshness")

		configDir := t.TempDir()

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		// Create workspace and start agent A pointing to MCP server A.
		workspace := coderdtest.CreateWorkspace(t, setup.client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, workspace.LatestBuild.ID)

		configPathA := writeMCPConfigFile(t, configDir, "restart-srv", mcpA.url())
		mcpA.release() // Let agent A register immediately.
		agentA := agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: configPathA,
			}
		})
		coderdtest.AwaitWorkspaceAgents(t, setup.client, workspace.ID)

		// Wait for agent A to push settled snapshot with nonce A tools.
		time.Sleep(2 * time.Second)

		// Close agent A (simulates crash/restart).
		require.NoError(t, agentA.Close())

		// Rewrite MCP config to point to server B (different nonce).
		writeMCPConfigFile(t, configDir, "restart-srv", mcpB.url())
		mcpB.release() // Let agent B register immediately.

		// Start agent B on the same token (same DB row).
		_ = agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: filepath.Join(configDir, ".mcp.json"),
			}
		})
		coderdtest.AwaitWorkspaceAgents(t, setup.client, workspace.ID)

		// Wait for agent B to push its settled snapshot.
		time.Sleep(2 * time.Second)

		// Now create an API-attached chat. The generation preparer
		// should use agent.ReadyAt as notBefore. Agent B's ReadyAt
		// is after agent A's snapshot, so agent A's snapshot is stale.
		var streamedCallCount atomic.Int32

		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("Restart test")
			}
			callNum := streamedCallCount.Add(1)
			switch callNum {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("find_tools", `{"queries":["restart-echo"]}`),
				)
			case 2:
				// Second: invoke the discovered tool.
				var foundTool string
				for _, tool := range req.Tools {
					name := openAIToolName(tool)
					if strings.Contains(name, "restart-echo") {
						foundTool = name
						break
					}
				}
				if foundTool != "" {
					return chattest.OpenAIStreamingResponse(
						chattest.OpenAIToolCallChunk(foundTool, `{"input":"restart-test"}`),
					)
				}
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("No tool found.")...,
				)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("Done with restart test.")...,
				)
			}
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			WorkspaceID:    &workspace.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Use the restart-echo tool.",
				},
			},
		})
		require.NoError(t, err)

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should settle; last_error=%v", chatResult.LastError)

		// Verify nonce B (not A) appears in the chat tool results.
		chatMsgs, err := setup.expClient.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var foundNonceB, foundNonceA bool
		for _, msg := range chatMsgs.Messages {
			for _, part := range msg.Content {
				if part.Type == codersdk.ChatMessagePartTypeToolResult {
					resultStr := string(part.Result)
					if strings.Contains(resultStr, mcpB.nonce) {
						foundNonceB = true
					}
					if strings.Contains(resultStr, mcpA.nonce) {
						foundNonceA = true
					}
				}
			}
		}
		assert.True(t, foundNonceB,
			"expected nonce B (%s) in tool results (fresh agent)", mcpB.nonce)
		assert.False(t, foundNonceA,
			"nonce A (%s) should NOT appear (stale agent)", mcpA.nonce)
	})

	// StartWorkspace_DelayedMCP verifies the start_workspace tool path:
	// create workspace, stop it, model calls start_workspace, the gate
	// blocks until MCP registration settles, then find_tools discovers
	// and invokes the fixture tool with verified nonce.
	t.Run("StartWorkspace_DelayedMCP", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		mcpCtrl := newControllableMCPServer(t, "start-echo")
		configDir := t.TempDir()
		configPath := writeMCPConfigFile(t, configDir, "start-srv", mcpCtrl.url())

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		// Create workspace, then stop it.
		workspace := coderdtest.CreateWorkspace(t, setup.client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, workspace.LatestBuild.ID)
		workspace = coderdtest.MustTransitionWorkspace(
			t, setup.client, workspace.ID,
			codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionStop,
		)

		// Start agent with MCP config. The agent will authenticate
		// once the start build provisions a new agent row with the
		// same token.
		_ = agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: configPath,
			}
		})

		// Release MCP after the agent reaches the blocked server.
		go func() {
			select {
			case <-mcpCtrl.arrivedCh:
				t.Log("start_workspace: MCP arrived, holding 2s")
				time.Sleep(2 * time.Second)
				t.Log("start_workspace: releasing MCP")
				mcpCtrl.release()
			case <-ctx.Done():
			}
		}()

		var modelCalledWhilePending atomic.Bool
		var streamedCallCount atomic.Int32

		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("Start WS test")
			}
			callNum := streamedCallCount.Add(1)
			switch callNum {
			case 1:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("start_workspace", "{}"),
				)
			case 2:
				if !mcpCtrl.released.Load() {
					modelCalledWhilePending.Store(true)
				}
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("find_tools", `{"queries":["start-echo"]}`),
				)
			case 3:
				var foundTool string
				for _, tool := range req.Tools {
					name := openAIToolName(tool)
					if strings.Contains(name, "start-echo") {
						foundTool = name
						break
					}
				}
				if foundTool != "" {
					return chattest.OpenAIStreamingResponse(
						chattest.OpenAIToolCallChunk(foundTool, `{"input":"start-test"}`),
					)
				}
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("No tool found.")...,
				)
			default:
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAITextChunks("Done.")...,
				)
			}
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			WorkspaceID:    &workspace.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Start the workspace.",
				},
			},
		})
		require.NoError(t, err)

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should settle; last_error=%v", chatResult.LastError)

		require.False(t, modelCalledWhilePending.Load(),
			"post-start generation should not fire before MCP released")

		// Verify nonce in chat results.
		chatMsgs, err := setup.expClient.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var foundNonce bool
		for _, msg := range chatMsgs.Messages {
			for _, part := range msg.Content {
				if part.Type == codersdk.ChatMessagePartTypeToolResult {
					if strings.Contains(string(part.Result), mcpCtrl.nonce) {
						foundNonce = true
					}
				}
			}
		}
		assert.True(t, foundNonce,
			"expected nonce %q in start_workspace tool results", mcpCtrl.nonce)
	})

	// StartWorkspace_AlreadyRunning verifies that calling start_workspace
	// on a running workspace does not introduce a 15s delay: the existing
	// settled snapshot is accepted immediately via zero notBefore.
	t.Run("StartWorkspace_AlreadyRunning", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitSuperLong)
		setup := setupMCPTest(t)

		mcpCtrl := newControllableMCPServer(t, "running-echo")
		mcpCtrl.release() // Let registration complete immediately.

		configDir := t.TempDir()
		configPath := writeMCPConfigFile(t, configDir, "running-srv", mcpCtrl.url())

		agentToken := uuid.NewString()
		version := coderdtest.CreateTemplateVersion(t, setup.client, setup.user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ApplyComplete,
			ProvisionGraph: echo.ProvisionGraphWithAgent(agentToken),
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, setup.client, version.ID)
		template := coderdtest.CreateTemplate(t, setup.client, setup.user.OrganizationID, version.ID)

		workspace := coderdtest.CreateWorkspace(t, setup.client, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, setup.client, workspace.LatestBuild.ID)

		_ = agenttest.New(t, setup.client.URL, agentToken, func(o *agent.Options) {
			o.ContextConfig = agentcontextconfig.Config{
				MCPConfigFiles: configPath,
			}
		})
		coderdtest.AwaitWorkspaceAgents(t, setup.client, workspace.ID)

		// Wait for settled snapshot.
		time.Sleep(2 * time.Second)

		var streamedCallCount atomic.Int32
		openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if !req.Stream {
				return chattest.OpenAINonStreamingResponse("Already running test")
			}
			if streamedCallCount.Add(1) == 1 {
				// Model calls start_workspace on an already-running workspace.
				return chattest.OpenAIStreamingResponse(
					chattest.OpenAIToolCallChunk("start_workspace", "{}"),
				)
			}
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("Already running.")...,
			)
		})

		coderdtest.CreateOpenAICompatChatModel(t, setup.expClient, openAIURL)

		startTime := time.Now()
		chat, err := setup.expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: setup.user.OrganizationID,
			WorkspaceID:    &workspace.ID,
			Content: []codersdk.ChatInputPart{
				{
					Type: codersdk.ChatInputPartTypeText,
					Text: "Start the workspace.",
				},
			},
		})
		require.NoError(t, err)

		chatResult := coderdtest.WaitForChatSettled(ctx, t, setup.api, chat.ID)
		elapsed := time.Since(startTime)

		require.Equal(t, "waiting", string(chatResult.Status),
			"chat should settle; last_error=%v", chatResult.LastError)
		require.GreaterOrEqual(t, streamedCallCount.Load(), int32(2),
			"model should have been called at least twice")

		// Already-running passes zero notBefore, so the existing
		// settled snapshot is accepted immediately. No 15s delay.
		assert.Less(t, elapsed, 10*time.Second,
			"already-running start should not block for settlement timeout")
	})
}
