package chatd_test

// Prototype coverage for the MCP Gateway experiment: with the
// experiment on, chatd must reach upstream MCP servers only through the
// gateway, as a plain MCP client.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/coderd/x/mcpgateway"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestGeneration_ThroughMCPGateway drives a full generation with the
// mcp-gateway experiment on. The model must be offered the double
// prefixed gw__<slug>__<tool> name, the upstream must receive exactly
// one forwarded tools/call, and its result must reach the model.
func TestGeneration_ThroughMCPGateway(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	var upstreamCalls atomic.Int32
	upstream := newTestMCPServer("gateway-upstream")
	upstream.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "Echoes the input",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
			"required": []string{"input"},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		upstreamCalls.Add(1)
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		input, _ := args["input"].(string)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + input}},
		}, nil
	})
	upstreamSrv := httptest.NewServer(testMCPHTTPHandler(upstream))
	t.Cleanup(upstreamSrv.Close)

	var (
		mu            sync.Mutex
		offeredTools  [][]string
		modelMessages [][]chattest.OpenAIMessage
		streamedCalls atomic.Int32
	)
	openAIURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("title")
		}
		names := make([]string, 0, len(req.Tools))
		for _, tool := range req.Tools {
			names = append(names, openAIToolName(tool))
		}
		mu.Lock()
		offeredTools = append(offeredTools, names)
		modelMessages = append(modelMessages, append([]chattest.OpenAIMessage(nil), req.Messages...))
		mu.Unlock()

		if streamedCalls.Add(1) == 1 {
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("gw__up__echo", `{"input":"hi"}`),
			)
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
	})

	user, org, model := seedChatDependenciesWithProvider(t, db, "openai-compat", openAIURL)
	mcpConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		DisplayName:    "Upstream",
		Slug:           "up",
		Url:            upstreamSrv.URL,
		Enabled:        true,
		CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
	})

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	// The gateway relies on the dbauthz post-filter for MCP server ACLs.
	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		logger,
		coderdtest.AccessControlStorePointer(),
	)
	gateway := mcpgateway.New(mcpgateway.Options{
		Logger:     logger,
		Database:   authzDB,
		HTTPClient: testMCPHTTPClient(),
	})

	server := newActiveTestServer(t, authzDB, ps, func(cfg *chatd.Config) {
		withoutMCPToolSearch(cfg)
		require.True(t, cfg.Experiments.Enabled(codersdk.ExperimentMCPGateway))
		cfg.MCPGateway = gateway.Handler()
		cfg.AIBridgeTransportFactory = chatAIGatewayTransportFactoryPointer(chattest.NewMockAIBridgeTransport(t, openAIURL))
	})

	// CreateChat authorizes against the caller; the worker uses its own actor.
	ownerSubject, _, err := httpmw.UserRBACSubject(dbauthz.AsSystemRestricted(ctx), db, user.ID, rbac.ScopeAll)
	require.NoError(t, err)

	chat, err := server.CreateChat(dbauthz.As(ctx, ownerSubject), chatd.CreateOptions{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Title:          "mcp-gateway",
		ModelConfigID:  model.ID,
		MCPServerIDs:   []uuid.UUID{mcpConfig.ID},
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("echo hi"),
		},
	})
	require.NoError(t, err)
	waitForChatProcessed(ctx, t, db, chat.ID, server)

	result, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	if result.Status == database.ChatStatusError {
		require.FailNowf(t, "chat failed", "last_error=%q", chatLastErrorMessage(result.LastError))
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, offeredTools)
	require.Contains(t, offeredTools[0], "gw__up__echo",
		"the gateway's tools must reach the model under the gateway slug")
	require.NotContains(t, offeredTools[0], "up__echo",
		"chatd must not connect to the upstream directly")
	require.Equal(t, int32(1), upstreamCalls.Load(),
		"the upstream must receive exactly one forwarded tools/call")

	require.GreaterOrEqual(t, len(modelMessages), 2)
	var sawResult bool
	for _, msg := range modelMessages[len(modelMessages)-1] {
		if msg.Role == "tool" && strings.Contains(msg.Content, "echo: hi") {
			sawResult = true
		}
	}
	require.True(t, sawResult, "the upstream result must reach the model: %+v", modelMessages[len(modelMessages)-1])
}
