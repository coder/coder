package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRegisterSDKTool(t *testing.T) {
	t.Parallel()
	for _, structured := range []bool{false, true} {
		t.Run(fmt.Sprintf("structured=%t", structured), func(t *testing.T) {
			t.Parallel()

			type arguments struct {
				Fail  bool   `json:"fail"`
				Value string `json:"value"`
			}
			type response struct {
				Value string `json:"value"`
			}
			shapes := map[string]any{
				"array":  []string{"hello"},
				"string": "hello",
				"null":   nil,
				"empty":  map[string]any{},
			}
			tool := toolsdk.Tool[arguments, any]{
				Tool: aisdk.Tool{
					Name: "test_typed_tool",
					Schema: aisdk.Schema{
						Properties: map[string]any{
							"fail":  map[string]any{"type": "boolean"},
							"value": map[string]any{"type": "string"},
						},
						Required: []string{"value"},
					},
				},
				Handler: func(_ context.Context, _ toolsdk.Deps, args arguments) (any, error) {
					if args.Fail {
						return nil, assert.AnError
					}
					if shape, ok := shapes[args.Value]; ok {
						return shape, nil
					}
					return response{Value: args.Value}, nil
				},
			}.Generic()

			server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
			if structured {
				registerSDKTool(server, tool, toolsdk.Deps{}, true)
			} else {
				RegisterSDKTool(server, tool, toolsdk.Deps{})
			}
			serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
			ctx := testutil.Context(t, testutil.WaitShort)
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, serverSession.Close())
			})
			client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
			clientSession, err := client.Connect(ctx, clientTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, clientSession.Close())
			})

			result, err := clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      tool.Name,
				Arguments: map[string]any{"value": "hello"},
			})
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.Len(t, result.Content, 1)
			content, ok := result.Content[0].(*sdkmcp.TextContent)
			require.True(t, ok)
			require.JSONEq(t, `{"value":"hello"}`, content.Text)
			if structured {
				require.Equal(t, map[string]any{"value": "hello"}, result.StructuredContent)
			} else {
				require.Nil(t, result.StructuredContent)
			}
			for name, shape := range shapes {
				result, err := clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
					Name: tool.Name, Arguments: map[string]any{"value": name},
				})
				require.NoError(t, err)
				require.False(t, result.IsError)
				require.Len(t, result.Content, 1)
				content, ok := result.Content[0].(*sdkmcp.TextContent)
				require.True(t, ok)
				expected, err := json.Marshal(shape)
				require.NoError(t, err)
				require.JSONEq(t, string(expected), content.Text)
				if structured && name == "empty" {
					require.Equal(t, map[string]any{}, result.StructuredContent)
				} else {
					require.Nil(t, result.StructuredContent)
				}
			}

			result, err = clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      tool.Name,
				Arguments: map[string]any{"fail": true, "value": "hello"},
			})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Len(t, result.Content, 1)
			content, ok = result.Content[0].(*sdkmcp.TextContent)
			require.True(t, ok)
			require.Equal(t, assert.AnError.Error(), content.Text)
			require.Nil(t, result.StructuredContent)
		})
	}
}
