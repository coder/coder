package chattool_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// fakeAgentConn implements just enough of workspacesdk.AgentConn
// for testing CallMCPTool.
type fakeAgentConn struct {
	workspacesdk.AgentConn
	callMCPToolFunc func(ctx context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error)
}

func (f *fakeAgentConn) CallMCPTool(ctx context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
	return f.callMCPToolFunc(ctx, req)
}

func TestWorkspaceMCPTool_InvalidateOn404(t *testing.T) {
	t.Parallel()

	t.Run("404ErrorInvalidatesCache", func(t *testing.T) {
		t.Parallel()

		var invalidated atomic.Bool
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, codersdk.NewError(
							http.StatusNotFound,
							codersdk.Response{
								Message: "MCP tool call failed.",
								Detail:  `unknown MCP server: "test"`,
							},
						)
					},
				}, nil
			},
			func() { invalidated.Store(true) },
			false,
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError, "response should be an error")
		assert.True(t, invalidated.Load(),
			"invalidateCache should fire on 404")
	})

	t.Run("Non404DoesNotInvalidate", func(t *testing.T) {
		t.Parallel()

		var invalidated atomic.Bool
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, codersdk.NewError(
							http.StatusBadGateway,
							codersdk.Response{
								Message: "Bad Gateway",
							},
						)
					},
				}, nil
			},
			func() { invalidated.Store(true) },
			false,
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.False(t, invalidated.Load(),
			"invalidateCache should NOT fire on non-404 error")
	})

	t.Run("ToolLevelErrorNoInvalidation", func(t *testing.T) {
		t.Parallel()

		var invalidated atomic.Bool
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{
							IsError: true,
							Content: []workspacesdk.MCPToolContent{
								{Type: "text", Text: "tool error"},
							},
						}, nil
					},
				}, nil
			},
			func() { invalidated.Store(true) },
			false,
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.False(t, invalidated.Load(),
			"invalidateCache should NOT fire on tool-level error (HTTP 200)")
	})

	t.Run("NilInvalidateCallbackSafe", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, codersdk.NewError(
							http.StatusNotFound,
							codersdk.Response{
								Message: "MCP tool call failed.",
								Detail:  `unknown MCP server: "test"`,
							},
						)
					},
				}, nil
			},
			nil,
			false,
		)

		// Should not panic.
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
	})
}

func TestWorkspaceMCPTool_SanitizesModelNameKeepsRoutingName(t *testing.T) {
	t.Parallel()

	t.Run("InvalidCharsSanitizedForModelOriginalForRouting", func(t *testing.T) {
		t.Parallel()

		var gotToolName string
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				// "@" is outside the provider's allowed tool-name set; the
				// model must never see it or the whole request is rejected.
				Name:        "weather@home__get_forecast",
				Description: "test tool",
			},
			func(_ context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						gotToolName = req.ToolName
						return workspacesdk.CallMCPToolResponse{
							Content: []workspacesdk.MCPToolContent{{Type: "text", Text: "ok"}},
						}, nil
					},
				}, nil
			},
			nil,
			false,
		)

		// The model-facing name is sanitized to the provider-safe set.
		assert.Equal(t, "weather_home__get_forecast", tool.Info().Name)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		// The agent receives the original name so it can route the call to
		// the correct server and original tool.
		assert.Equal(t, "weather@home__get_forecast", gotToolName)
	})

	t.Run("ValidNameUnchanged", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "github__create_issue",
				Description: "test tool",
			},
			func(_ context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, nil
					},
				}, nil
			},
			nil,
			false,
		)

		// A name already within the allowed set is left untouched.
		assert.Equal(t, "github__create_issue", tool.Info().Name)
	})

	t.Run("LongNameTruncatedForModel", func(t *testing.T) {
		t.Parallel()

		// A name longer than the provider limit is truncated. "srv__" plus a
		// 64-char tool name exceeds the 64-char cap.
		longName := "srv__" + strings.Repeat("a", 64)
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        longName,
				Description: "test tool",
			},
			func(_ context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, nil
					},
				}, nil
			},
			nil,
			false,
		)

		// The model-facing name is capped at the strictest provider limit.
		assert.LessOrEqual(t, len(tool.Info().Name), 64)
	})
}

func TestWorkspaceMCPTool_MCPApp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                              string
		enabled, noURI, isError, noResult bool
		size                              int
	}{
		{name: "Enabled", enabled: true},
		{name: "Disabled"},
		{name: "NoURI", enabled: true, noURI: true},
		{name: "Error", enabled: true, isError: true},
		{name: "NoResult", enabled: true, noResult: true},
		{name: "NoResultError", enabled: true, noResult: true, isError: true},
		{name: "AtLimit", enabled: true, size: 256 << 10},
		{name: "Oversize", enabled: true, size: (256 << 10) + 1},
		{name: "OversizeError", enabled: true, isError: true, size: (256 << 10) + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			original := workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{{Type: "text", Text: "model output"}, {Type: "image", Data: "aW1hZ2U=", MediaType: "image/png"}},
				Result:  json.RawMessage(`{"content":[{"type":"text","text":"display output"}],"structuredContent":{"display":""},"_meta":{"view":"chart"},"isError":` + strconv.FormatBool(tc.isError) + `}`),
				IsError: tc.isError,
			}
			if tc.noResult {
				original.Result = nil
			} else if tc.size > 0 {
				original.Result = json.RawMessage(strings.Replace(string(original.Result), `"display":""`, `"display":"`+strings.Repeat("x", tc.size-len(original.Result))+`"`, 1))
			}
			info := workspacesdk.MCPToolInfo{Name: "my.server__view", ServerName: "my.server"}
			if !tc.noURI {
				info.UIResourceURI = "ui://view/app.html"
			}
			tool := chattool.NewWorkspaceMCPTool(info, func(context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{callMCPToolFunc: func(_ context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
					require.Equal(t, "my.server__view", req.ToolName)
					return original, nil
				}}, nil
			}, nil, tc.enabled)
			response, err := tool.Run(context.Background(), fantasy.ToolCall{})
			require.NoError(t, err)
			require.Equal(t, "model output", response.Content)
			require.Equal(t, tc.isError, response.IsError)
			app, err := chattool.MCPAppFromMetadata(response.Metadata)
			require.NoError(t, err)
			if !tc.enabled || tc.noURI {
				require.Nil(t, app)
				require.Empty(t, response.Metadata)
				return
			}
			require.NotNil(t, app)
			require.Equal(t, "my.server", app.ServerName)
			require.Equal(t, info.UIResourceURI, app.ResourceURI)
			switch {
			case tc.size > 256<<10:
				require.JSONEq(t, `{"content":[{"type":"text","text":"[result omitted: too large]"}],"isError":`+strconv.FormatBool(tc.isError)+`}`, string(app.Result))
			case tc.noResult:
				require.JSONEq(t, `{"content":[],"isError":`+strconv.FormatBool(tc.isError)+`}`, string(app.Result))
			default:
				require.JSONEq(t, string(original.Result), string(app.Result))
			}
		})
	}
}

func TestMCPAppAttachmentMetadata(t *testing.T) {
	t.Parallel()
	app := codersdk.ChatMCPApp{ServerName: "test", ResourceURI: "ui://test/app", Result: json.RawMessage(`{"content":[],"isError":false}`)}
	attachment := chattool.AttachmentMetadata{FileID: uuid.New(), MediaType: "text/plain"}
	for _, appFirst := range []bool{true, false} {
		t.Run(strconv.FormatBool(appFirst), func(t *testing.T) {
			t.Parallel()
			response := fantasy.NewTextResponse("unchanged")
			if appFirst {
				response = chattool.WithAttachments(chattool.WithMCPApp(response, app), attachment)
			} else {
				response = chattool.WithMCPApp(chattool.WithAttachments(response, attachment), app)
			}
			got, err := chattool.MCPAppFromMetadata(response.Metadata)
			require.NoError(t, err)
			require.Equal(t, &app, got)
			attachments, err := chattool.AttachmentsFromMetadata(response.Metadata)
			require.NoError(t, err)
			require.Equal(t, []chattool.AttachmentMetadata{attachment}, attachments)
			require.Equal(t, "unchanged", response.Content)
		})
	}
}

func TestNewWorkspaceMCPTools_DisambiguatesCollidingNames(t *testing.T) {
	t.Parallel()

	var routed []string
	getConn := func(_ context.Context) (workspacesdk.AgentConn, error) {
		return &fakeAgentConn{
			callMCPToolFunc: func(_ context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
				routed = append(routed, req.ToolName)
				return workspacesdk.CallMCPToolResponse{}, nil
			},
		}, nil
	}

	// Both names sanitize to "foo_bar__echo"; the set builder must keep them
	// distinct for the model while routing each to its own original name.
	infos := []workspacesdk.MCPToolInfo{
		{Name: "foo.bar__echo"},
		{Name: "foo_bar__echo"},
	}

	tools := chattool.NewWorkspaceMCPTools(infos, getConn, nil, false)
	require.Len(t, tools, 2)

	names := []string{tools[0].Info().Name, tools[1].Info().Name}
	assert.NotEqual(t, names[0], names[1],
		"colliding model-facing names must be disambiguated")
	assert.ElementsMatch(t,
		[]string{"foo_bar__echo", "foo_bar__echo_2"}, names)

	// Each tool routes to its own original (unsanitized) name.
	for _, tl := range tools {
		_, err := tl.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
	}
	assert.ElementsMatch(t,
		[]string{"foo.bar__echo", "foo_bar__echo"}, routed)
}
