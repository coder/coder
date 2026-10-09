package mcpclient

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/testutil"
)

// discoverZeroArgTool serves a single tool whose inputSchema has no
// "properties" key over an in-memory transport and returns it as the
// client sees it after ListTools, together with the live session.
func discoverZeroArgTool(t *testing.T) (*mcp.Tool, *mcp.ClientSession) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "ping",
		Description: "Takes no arguments",
		InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientSession.Close() })

	listed, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 1)
	return listed.Tools[0], clientSession
}

// TestMCPTool_Info_NilPropertiesBecomeEmptyObject verifies that a
// tool whose inputSchema omits "properties" never serializes
// "properties" to JSON null. OpenAI rejects null with "None is not
// of type 'object'". With model_intent enabled the original
// properties are nested under properties.properties, where
// chatloop's outer nil check cannot reach, so both shapes are
// asserted on the exact bytes a provider would receive.
func TestMCPTool_Info_NilPropertiesBecomeEmptyObject(t *testing.T) {
	t.Parallel()
	tool, session := discoverZeroArgTool(t)

	t.Run("Flat", func(t *testing.T) {
		t.Parallel()
		wrapper := newMCPTool(uuid.New(), "srv", tool, session, false)
		info := wrapper.Info()
		require.NotNil(t, info.Parameters, "Parameters should never be nil")

		bs, err := json.Marshal(info.Parameters)
		require.NoError(t, err)
		require.JSONEq(t, `{}`, string(bs))
	})

	t.Run("ModelIntent", func(t *testing.T) {
		t.Parallel()
		wrapper := newMCPTool(uuid.New(), "srv", tool, session, true)
		info := wrapper.Info()

		outer, ok := info.Parameters["properties"].(map[string]any)
		require.True(t, ok, "model_intent wrapper should nest properties")
		bs, err := json.Marshal(outer["properties"])
		require.NoError(t, err)
		require.JSONEq(t, `{}`, string(bs), "nested properties must be {} not null")

		// The bytes that actually reach the provider go through
		// chatloop, which normalizes only the outer map.
		defs := chatloop.BuildToolDefinitions([]fantasy.AgentTool{wrapper}, nil, nil)
		require.Len(t, defs, 1)
		fn, ok := defs[0].(fantasy.FunctionTool)
		require.True(t, ok)
		schemaBytes, err := json.Marshal(fn.InputSchema)
		require.NoError(t, err)

		var got struct {
			Properties struct {
				Properties struct {
					Properties json.RawMessage `json:"properties"`
				} `json:"properties"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(schemaBytes, &got))
		require.JSONEq(t, `{}`, string(got.Properties.Properties.Properties),
			"provider-facing properties.properties.properties must be {} not null")
	})
}
