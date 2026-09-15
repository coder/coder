package chattool_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"charm.land/fantasy"
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

func TestWorkspaceMCPTool_Errors(t *testing.T) {
	t.Parallel()

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewWorkspaceMCPTools(
			[]workspacesdk.MCPToolInfo{{
				Name:        "test__echo",
				Description: "test tool",
			}},
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
		)[0]

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError, "response should be an error")
	})

	t.Run("BadGateway", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewWorkspaceMCPTools(
			[]workspacesdk.MCPToolInfo{{
				Name:        "test__echo",
				Description: "test tool",
			}},
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
		)[0]

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
	})

	t.Run("ToolError", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewWorkspaceMCPTools(
			[]workspacesdk.MCPToolInfo{{
				Name:        "test__echo",
				Description: "test tool",
			}},
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
		)[0]

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
	})
}

func TestWorkspaceMCPTool_ConvertsMixedContent(t *testing.T) {
	t.Parallel()

	image := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	audio := []byte("wav-bytes")
	encoded := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

	tests := []struct {
		name          string
		resp          workspacesdk.CallMCPToolResponse
		wantType      string
		wantData      []byte
		wantMediaType string
		wantContent   string
		wantIsError   bool
	}{
		{
			name: "TextThenImage",
			resp: workspacesdk.CallMCPToolResponse{Content: []workspacesdk.MCPToolContent{
				{Type: "text", Text: "Ran Playwright code"},
				{Type: "image", Data: encoded(image), MediaType: "image/png"},
			}},
			wantType:      "image",
			wantData:      image,
			wantMediaType: "image/png",
			wantContent:   "Ran Playwright code",
		},
		{
			name: "ImageThenText",
			resp: workspacesdk.CallMCPToolResponse{Content: []workspacesdk.MCPToolContent{
				{Type: "image", Data: encoded(image), MediaType: "image/png"},
				{Type: "text", Text: "first"},
				{Type: "text", Text: "second"},
			}},
			wantType:      "image",
			wantData:      image,
			wantMediaType: "image/png",
			wantContent:   "first\nsecond",
		},
		{
			name: "AudioWithTextIsMedia",
			resp: workspacesdk.CallMCPToolResponse{Content: []workspacesdk.MCPToolContent{
				{Type: "text", Text: "transcript"},
				{Type: "audio", Data: encoded(audio), MediaType: "audio/wav"},
			}},
			wantType:      "media",
			wantData:      audio,
			wantMediaType: "audio/wav",
			wantContent:   "transcript",
		},
		{
			name: "FirstImageKept",
			resp: workspacesdk.CallMCPToolResponse{Content: []workspacesdk.MCPToolContent{
				{Type: "image", Data: encoded(image), MediaType: "image/png"},
				{Type: "image", Data: encoded(audio), MediaType: "image/jpeg"},
				{Type: "text", Text: "two shots"},
			}},
			wantType:      "image",
			wantData:      image,
			wantMediaType: "image/png",
			wantContent:   "two shots",
		},
		{
			name: "ImageWithoutMediaTypeStaysText",
			resp: workspacesdk.CallMCPToolResponse{Content: []workspacesdk.MCPToolContent{
				{Type: "text", Text: "captured"},
				{Type: "image", Data: encoded(image)},
			}},
			wantType:    "text",
			wantContent: "captured",
		},
		{
			name: "MixedErrorKeepsFlag",
			resp: workspacesdk.CallMCPToolResponse{IsError: true, Content: []workspacesdk.MCPToolContent{
				{Type: "text", Text: "boom"},
				{Type: "image", Data: encoded(image), MediaType: "image/png"},
			}},
			wantType:      "image",
			wantData:      image,
			wantMediaType: "image/png",
			wantContent:   "boom",
			wantIsError:   true,
		},
		{
			name: "TextOnly",
			resp: workspacesdk.CallMCPToolResponse{Content: []workspacesdk.MCPToolContent{
				{Type: "text", Text: "plain"},
			}},
			wantType:    "text",
			wantContent: "plain",
		},
		{
			name: "ImageOnly",
			resp: workspacesdk.CallMCPToolResponse{Content: []workspacesdk.MCPToolContent{
				{Type: "image", Data: encoded(image), MediaType: "image/png"},
			}},
			wantType:      "image",
			wantData:      image,
			wantMediaType: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := chattool.NewWorkspaceMCPTools(
				[]workspacesdk.MCPToolInfo{{Name: "browser__take_screenshot"}},
				func(context.Context) (workspacesdk.AgentConn, error) {
					return &fakeAgentConn{
						callMCPToolFunc: func(context.Context, workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
							return tt.resp, nil
						},
					}, nil
				},
			)[0]

			resp, err := tool.Run(context.Background(), fantasy.ToolCall{Input: "{}"})
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, resp.Type)
			assert.Equal(t, tt.wantData, resp.Data)
			assert.Equal(t, tt.wantMediaType, resp.MediaType)
			assert.Equal(t, tt.wantContent, resp.Content)
			assert.Equal(t, tt.wantIsError, resp.IsError)
		})
	}
}

func TestWorkspaceMCPTool_SanitizesModelNameKeepsRoutingName(t *testing.T) {
	t.Parallel()

	t.Run("InvalidCharsSanitizedForModelOriginalForRouting", func(t *testing.T) {
		t.Parallel()

		var gotToolName string
		tool := chattool.NewWorkspaceMCPTools(
			[]workspacesdk.MCPToolInfo{{
				// "@" is outside the provider's allowed tool-name set; the
				// model must never see it or the whole request is rejected.
				Name:        "weather@home__get_forecast",
				Description: "test tool",
			}},
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
		)[0]

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

		tool := chattool.NewWorkspaceMCPTools(
			[]workspacesdk.MCPToolInfo{{
				Name:        "github__create_issue",
				Description: "test tool",
			}},
			func(_ context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, nil
					},
				}, nil
			},
		)[0]

		// A name already within the allowed set is left untouched.
		assert.Equal(t, "github__create_issue", tool.Info().Name)
	})

	t.Run("LongNameTruncatedForModel", func(t *testing.T) {
		t.Parallel()

		// A name longer than the provider limit is truncated. "srv__" plus a
		// 64-char tool name exceeds the 64-char cap.
		longName := "srv__" + strings.Repeat("a", 64)
		tool := chattool.NewWorkspaceMCPTools(
			[]workspacesdk.MCPToolInfo{{
				Name:        longName,
				Description: "test tool",
			}},
			func(_ context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, nil
					},
				}, nil
			},
		)[0]

		// The model-facing name is capped at the strictest provider limit.
		assert.LessOrEqual(t, len(tool.Info().Name), 64)
	})
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

	tools := chattool.NewWorkspaceMCPTools(infos, getConn)
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
