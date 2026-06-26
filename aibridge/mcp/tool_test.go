package mcp_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/mcp"
)

func TestSanitizeToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "AlreadyValid",
			input:    "my_tool-name123",
			expected: "my_tool-name123",
		},
		{
			name:     "DotsReplaced",
			input:    "awslabs.aws-documentation-mcp-server",
			expected: "awslabs_aws-documentation-mcp-server",
		},
		{
			name:     "MultipleDots",
			input:    "com.example.tool.v2",
			expected: "com_example_tool_v2",
		},
		{
			name:     "Spaces",
			input:    "my tool name",
			expected: "my_tool_name",
		},
		{
			name:     "SpecialCharacters",
			input:    "tool@v2#special!",
			expected: "tool_v2_special_",
		},
		{
			name:     "Empty",
			input:    "",
			expected: "",
		},
		{
			name:     "AllInvalid",
			input:    "...",
			expected: "___",
		},
		{
			name:     "Slashes",
			input:    "org/repo/tool",
			expected: "org_repo_tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mcp.SanitizeToolName(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestEncodeToolID_SanitizesComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   string
		tool     string
		expected string
	}{
		{
			name:     "ValidNames",
			server:   "my-server",
			tool:     "my_tool",
			expected: "bmcp_my-server_my_tool",
		},
		{
			name:     "DottedServerName",
			server:   "awslabs.aws-documentation-mcp-server",
			tool:     "read_documentation",
			expected: "bmcp_awslabs_aws-documentation-mcp-server_read_documentation",
		},
		{
			name:     "DottedToolName",
			server:   "server",
			tool:     "com.example.action",
			expected: "bmcp_server_com_example_action",
		},
		{
			name:     "BothDotted",
			server:   "org.server",
			tool:     "ns.action",
			expected: "bmcp_org_server_ns_action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mcp.EncodeToolID(tt.server, tt.tool)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestEncodeToolID_TruncatesLongNames(t *testing.T) {
	t.Parallel()

	// "bmcp_" prefix = 5 chars, "_" delimiter = 1 char, so
	// server + tool budget is MaxToolNameLen - 6.
	longServer := strings.Repeat("a", 40)
	longTool := strings.Repeat("b", 40)

	id := mcp.EncodeToolID(longServer, longTool)
	require.LessOrEqual(t, len(id), mcp.MaxToolNameLen,
		"encoded ID must not exceed MaxToolNameLen")
	assert.True(t, strings.HasPrefix(id, "bmcp_"))
}
