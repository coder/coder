package mcpclient

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// TestFullInputSchemaModelIntentObjectArguments verifies that the
// model_intent wrapper gives the tool's arguments the object shape of an
// unwrapped root even when the server's input schema lacks it. MCP
// requires an object schema, but not every server enforces it (the SDK
// test server does, hence an internal test). Models fill an untyped
// argument with a JSON string, and providers reject null members. The
// required list is that of the rewritten schema, which holds the draft
// 3 "required": true of each property.
func TestFullInputSchemaModelIntentObjectArguments(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		inputSchema map[string]any
		want        string
	}{
		{
			name: "no type",
			inputSchema: map[string]any{
				"$id":  "https://example.com/tool.json",
				"$ref": "https://example.com/other.json#/$defs/Args",
			},
			want: `{"type": "object", "properties": {}, "$ref": "https://example.com/other.json#/$defs/Args"}`,
		},
		{
			name: "type list",
			inputSchema: map[string]any{
				"type":       []any{"object", "null"},
				"properties": map[string]any{"a": map[string]any{"type": "string"}},
				"required":   []any{"a"},
			},
			want: `{"type": "object", "properties": {"a": {"type": "string"}}, "required": ["a"]}`,
		},
		{
			name:        "null members",
			inputSchema: map[string]any{"type": "object", "properties": nil, "required": nil},
			want:        `{"type": "object", "properties": {}}`,
		},
		{
			name: "draft 3 required",
			inputSchema: map[string]any{
				"$schema":    "http://json-schema.org/draft-03/schema#",
				"type":       "object",
				"properties": map[string]any{"a": map[string]any{"type": "string", "required": true}},
			},
			want: `{"type": "object", "properties": {"a": {"type": "string"}}, "required": ["a"]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tool := newMCPTool(uuid.Nil, "srv", &mcp.Tool{Name: "tool", InputSchema: tc.inputSchema}, nil, true, secretRedactor{}, 0)
			wrapped, ok := tool.FullInputSchema()["properties"].(map[string]any)
			require.True(t, ok)
			arguments, err := json.Marshal(wrapped["properties"])
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(arguments))
		})
	}
}
