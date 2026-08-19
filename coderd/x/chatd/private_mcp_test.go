package chatd_test

import (
	"context"
	"encoding/json"
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
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestPrivateMCPServerChatScopeAndRedaction(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Private-Canary"
		headerValue = "private-canary-value-12345"
	)
	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var headerSeen atomic.Bool
	mcpServer := newTestMCPServer("private")
	mcpServer.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "private description " + headerName + " " + headerValue,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "private schema " + headerValue,
				},
			},
			"required": []string{"input"},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		headerSeen.Store(req.Extra.Header.Get(headerName) == headerValue)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{
			Text: "private result " + headerName + " " + headerValue,
		}}}, nil
	})
	mcpHTTP := httptest.NewServer(testMCPHTTPHandler(mcpServer))
	t.Cleanup(mcpHTTP.Close)

	var (
		recordsMu      sync.Mutex
		toolsByPrompt  = map[string][][]string{}
		providerBodies [][]byte
		toolResultSeen atomic.Bool
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		recordsMu.Lock()
		providerBodies = append(providerBodies, append([]byte(nil), req.RawBody...))
		recordsMu.Unlock()
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}

		lastUser := ""
		toolResultPresent := false
		for _, message := range req.Messages {
			if message.Role == "user" {
				lastUser = message.Content
			}
			if message.Role == "tool" && strings.Contains(message.Content, "private result") {
				toolResultPresent = true
				if !strings.Contains(message.Content, headerName) && !strings.Contains(message.Content, headerValue) {
					toolResultSeen.Store(true)
				}
			}
		}
		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, openAIToolName(tool))
		}
		recordsMu.Lock()
		toolsByPrompt[lastUser] = append(toolsByPrompt[lastUser], names)
		recordsMu.Unlock()

		if strings.Contains(lastUser, "PRIVATE_FIRST") && !toolResultPresent {
			return chattest.OpenAIStreamingResponse(chattest.OpenAIToolCallChunk(
				"private__echo",
				`{"input":"hello"}`,
			))
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	server := newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		withoutMCPToolSearch(cfg)
		cfg.PrivateMCPHTTPClient = mcpHTTP.Client()
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(
			chattest.NewMockAIBridgeTransport(t, openAIURL),
		)
	})

	privateConfigs, err := json.Marshal([]codersdk.PrivateMCPServerConfig{{
		Name:    "private",
		URL:     mcpHTTP.URL,
		Headers: map[string]string{headerName: headerValue},
	}})
	require.NoError(t, err)
	privateChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID:          org.ID,
		OwnerID:                 user.ID,
		Title:                   "private MCP chat",
		ModelConfigID:           model.ID,
		PrivateMCPServerConfigs: privateConfigs,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("PRIVATE_FIRST"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, privateChat.ID, server)
	require.True(t, headerSeen.Load(), "private MCP headers must reach the configured server")
	require.True(t, toolResultSeen.Load(), "redacted private MCP tool results must return to the model")

	_, err = server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    privateChat.ID,
		CreatedBy: user.ID,
		Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("PRIVATE_LATER")},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, privateChat.ID, server)

	controlChat, err := server.CreateChat(ctx, chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "control chat",
		ModelConfigID:  model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("CONTROL_CHAT"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, controlChat.ID, server)

	recordsMu.Lock()
	firstCalls := append([][]string(nil), toolsByPrompt["PRIVATE_FIRST"]...)
	laterCalls := append([][]string(nil), toolsByPrompt["PRIVATE_LATER"]...)
	controlCalls := append([][]string(nil), toolsByPrompt["CONTROL_CHAT"]...)
	bodies := append([][]byte(nil), providerBodies...)
	recordsMu.Unlock()
	require.NotEmpty(t, firstCalls)
	require.Contains(t, firstCalls[0], "private__echo")
	require.NotEmpty(t, laterCalls)
	require.Contains(t, laterCalls[0], "private__echo", "later turns in the same chat retain private MCP tools")
	require.NotEmpty(t, controlCalls)
	require.NotContains(t, controlCalls[0], "private__echo", "unrelated chats must not receive private MCP tools")

	for _, body := range bodies {
		bodyString := string(body)
		require.NotContains(t, bodyString, mcpHTTP.URL)
		require.NotContains(t, bodyString, headerName)
		require.NotContains(t, bodyString, headerValue)
	}

	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  privateChat.ID,
		AfterID: 0,
	})
	require.NoError(t, err)
	for _, message := range messages {
		parts, err := chatprompt.ParseContent(message)
		require.NoError(t, err)
		encoded, err := json.Marshal(parts)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), mcpHTTP.URL)
		require.NotContains(t, string(encoded), headerName)
		require.NotContains(t, string(encoded), headerValue)
	}
}
