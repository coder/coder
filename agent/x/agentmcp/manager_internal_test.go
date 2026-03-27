package agentmcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestSplitToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantServer string
		wantTool   string
		wantErr    bool
	}{
		{
			name:       "Valid",
			input:      "server__tool",
			wantServer: "server",
			wantTool:   "tool",
		},
		{
			name:       "ValidWithUnderscoresInTool",
			input:      "server__my_tool",
			wantServer: "server",
			wantTool:   "my_tool",
		},
		{
			name:    "MissingSeparator",
			input:   "servertool",
			wantErr: true,
		},
		{
			name:    "EmptyServer",
			input:   "__tool",
			wantErr: true,
		},
		{
			name:    "EmptyTool",
			input:   "server__",
			wantErr: true,
		},
		{
			name:    "JustSeparator",
			input:   "__",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, tool, err := splitToolName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidToolName)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantServer, server)
			assert.Equal(t, tt.wantTool, tool)
		})
	}
}

func TestConvertResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// input is a pointer so we can test nil.
		input *mcp.CallToolResult
		want  workspacesdk.CallMCPToolResponse
	}{
		{
			name:  "NilInput",
			input: nil,
			want:  workspacesdk.CallMCPToolResponse{},
		},
		{
			name: "TextContent",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: "hello"},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "text", Text: "hello"},
				},
			},
		},
		{
			name: "ImageContent",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.ImageContent{
						Type:     "image",
						Data:     "base64data",
						MIMEType: "image/png",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "image", Data: "base64data", MediaType: "image/png"},
				},
			},
		},
		{
			name: "AudioContent",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.AudioContent{
						Type:     "audio",
						Data:     "base64audio",
						MIMEType: "audio/mp3",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "audio", Data: "base64audio", MediaType: "audio/mp3"},
				},
			},
		},
		{
			name: "IsErrorPropagation",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: "fail"},
				},
				IsError: true,
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "text", Text: "fail"},
				},
				IsError: true,
			},
		},
		{
			name: "MultipleContentItems",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: "caption"},
					mcp.ImageContent{
						Type:     "image",
						Data:     "imgdata",
						MIMEType: "image/jpeg",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "text", Text: "caption"},
					{Type: "image", Data: "imgdata", MediaType: "image/jpeg"},
				},
			},
		},
		{
			name: "ResourceLink",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.ResourceLink{
						Type: "resource_link",
						URI:  "file:///tmp/test.txt",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "resource", Text: "[resource link: file:///tmp/test.txt]"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := convertResult(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
