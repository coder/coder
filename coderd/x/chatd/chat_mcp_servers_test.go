package chatd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const chatAttachedMCPSecret = "bot-secret-value"

// chatAttachedMCPServer is a streamable HTTP MCP server with one echo
// tool. It records the request headers and the _meta payload of every
// tool call so tests can assert what chatd sent.
type chatAttachedMCPServer struct {
	url string

	mu          sync.Mutex
	requests    []http.Header
	toolCallIDs []string
}

func newChatAttachedMCPServer(t *testing.T, name string, toolDescription string) *chatAttachedMCPServer {
	t.Helper()

	recorder := &chatAttachedMCPServer{}
	srv := newTestMCPServer(name)
	srv.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: toolDescription,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
			"required": []string{"input"},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var arguments map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &arguments)
		input, _ := arguments["input"].(string)
		if id, ok := req.Params.Meta["com.coder/tool_call_id"].(string); ok {
			recorder.mu.Lock()
			recorder.toolCallIDs = append(recorder.toolCallIDs, id)
			recorder.mu.Unlock()
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + input}},
		}, nil
	})

	handler := testMCPHTTPHandler(srv)
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		recorder.mu.Lock()
		recorder.requests = append(recorder.requests, r.Header.Clone())
		recorder.mu.Unlock()
		handler.ServeHTTP(rw, r)
	}))
	t.Cleanup(ts.Close)
	recorder.url = ts.URL
	return recorder
}

func (s *chatAttachedMCPServer) snapshot() ([]http.Header, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]http.Header(nil), s.requests...), append([]string(nil), s.toolCallIDs...)
}

// chatAttachedMCPModel is an OpenAI mock that calls toolName on its first
// streamed request and records the tool names and raw body it received.
type chatAttachedMCPModel struct {
	url string

	mu           sync.Mutex
	callCount    atomic.Int32
	firstTools   []string
	firstRawBody string
	sawResult    atomic.Bool
}

func newChatAttachedMCPModel(t *testing.T, toolName string) *chatAttachedMCPModel {
	t.Helper()

	model := &chatAttachedMCPModel{}
	model.url = chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		if model.callCount.Add(1) == 1 {
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Function.Name)
			}
			model.mu.Lock()
			model.firstTools = names
			model.firstRawBody = string(req.RawBody)
			model.mu.Unlock()
			if toolName == "" {
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Done.")...)
			}
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk(toolName, `{"input":"hello from LLM"}`),
			)
		}
		for _, msg := range req.Messages {
			if msg.Role == "tool" && strings.Contains(msg.Content, "echo: hello from LLM") {
				model.sawResult.Store(true)
			}
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Got it!")...)
	})
	return model
}

func (m *chatAttachedMCPModel) first() ([]string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.firstTools...), m.firstRawBody
}

func TestChatAttachedMCPServers(t *testing.T) {
	t.Parallel()

	t.Run("ToolInvocation", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		bot := newChatAttachedMCPServer(t, "bot", "Talks to "+chatAttachedMCPSecret+" over the wire.")
		model := newChatAttachedMCPModel(t, "bot__echo")
		user, org, modelConfig := seedChatDependenciesWithProvider(t, db, "openai-compat", model.url)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			withoutMCPToolSearch(cfg)
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, model.url))
		})

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "chat-attached-mcp",
			ModelConfigID:  modelConfig.ID,
			MCPServers: []codersdk.ChatMCPServerRequest{{
				Slug:                "bot",
				URL:                 bot.url,
				Headers:             map[string]string{"X-Bot-Key": chatAttachedMCPSecret},
				ForwardCoderHeaders: true,
			}},
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("Echo something.")},
		})
		require.NoError(t, err)
		waitForChatProcessed(ctx, t, db, chat.ID, server)

		chatResult, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusWaiting, chatResult.Status, chatLastErrorMessage(chatResult.LastError))

		tools, rawBody := model.first()
		require.Contains(t, tools, "bot__echo")
		require.True(t, model.sawResult.Load(), "tool result must reach the second model call")
		require.Contains(t, rawBody, "Talks to [REDACTED] over the wire.")
		require.NotContains(t, rawBody, chatAttachedMCPSecret, "header values must be redacted before reaching the model")
		require.NotContains(t, rawBody, bot.url, "server URL must be redacted before reaching the model")

		requests, toolCallIDs := bot.snapshot()
		require.NotEmpty(t, requests)
		for _, h := range requests {
			require.Equal(t, chatAttachedMCPSecret, h.Get("X-Bot-Key"))
			require.Equal(t, user.ID.String(), h.Get(chatprovider.HeaderCoderOwnerID))
			require.Equal(t, chat.ID.String(), h.Get(chatprovider.HeaderCoderChatID))
		}
		require.Len(t, toolCallIDs, 1)
		require.NotEmpty(t, toolCallIDs[0])
	})

	t.Run("CallerSuppliedToolsDisabled", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		bot := newChatAttachedMCPServer(t, "bot", "Echoes the input")
		model := newChatAttachedMCPModel(t, "")
		user, org, modelConfig := seedChatDependenciesWithProvider(t, db, "openai-compat", model.url)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			withoutMCPToolSearch(cfg)
			cfg.DisableCallerSuppliedTools = true
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, model.url))
		})

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			Title:              "chat-attached-mcp-disabled",
			ModelConfigID:      modelConfig.ID,
			MCPServers:         []codersdk.ChatMCPServerRequest{{Slug: "bot", URL: bot.url}},
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("List tools.")},
		})
		require.NoError(t, err)
		waitForChatProcessed(ctx, t, db, chat.ID, server)

		tools, _ := model.first()
		require.NotContains(t, tools, "bot__echo")
		requests, _ := bot.snapshot()
		require.Empty(t, requests, "chatd must not connect when caller-supplied tools are disabled")
	})

	t.Run("PlanMode", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		approved := newChatAttachedMCPServer(t, "approved", "Echoes the input")
		blocked := newChatAttachedMCPServer(t, "blocked", "Echoes the input")
		model := newChatAttachedMCPModel(t, "approved__echo")
		user, org, modelConfig := seedChatDependenciesWithProvider(t, db, "openai-compat", model.url)
		server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
			withoutMCPToolSearch(cfg)
			cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, model.url))
		})

		chat, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID: org.ID,
			OwnerID:        user.ID,
			Title:          "chat-attached-mcp-plan",
			ModelConfigID:  modelConfig.ID,
			PlanMode:       database.NullChatPlanMode{ChatPlanMode: database.ChatPlanModePlan, Valid: true},
			MCPServers: []codersdk.ChatMCPServerRequest{
				{Slug: "approved", URL: approved.url, AllowInPlanMode: true},
				{Slug: "blocked", URL: blocked.url},
			},
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("Plan with the approved tool.")},
		})
		require.NoError(t, err)
		waitForChatProcessed(ctx, t, db, chat.ID, server)

		chatResult, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusWaiting, chatResult.Status, chatLastErrorMessage(chatResult.LastError))

		tools, _ := model.first()
		require.Contains(t, tools, "approved__echo")
		require.NotContains(t, tools, "blocked__echo")
		require.True(t, model.sawResult.Load(), "approved tool must be active in plan mode")
		blockedRequests, _ := blocked.snapshot()
		require.Empty(t, blockedRequests, "plan mode must not connect to servers without allow_in_plan_mode")
	})
}
