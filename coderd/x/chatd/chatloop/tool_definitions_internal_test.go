package chatloop

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"
)

// staticParametersTool returns a fixed ToolInfo, letting a test control
// Parameters directly. fantasy.NewAgentTool always generates a non-nil
// Parameters map, so it cannot reproduce the nil Parameters that MCP tool
// wrappers report for a schema with an empty "properties" object.
type staticParametersTool struct {
	fantasy.AgentTool
	info            fantasy.ToolInfo
	providerOptions fantasy.ProviderOptions
}

func (t staticParametersTool) Info() fantasy.ToolInfo { return t.info }

func (t staticParametersTool) ProviderOptions() fantasy.ProviderOptions { return t.providerOptions }

// TestBuildToolDefinitionsNilPropertiesBecomesEmptyObject verifies that a
// tool whose input schema has no properties (for example an MCP tool
// reporting {"type": "object", "properties": {}}) still serializes
// "properties" as an empty JSON object. A nil Parameters map would serialize
// to null, which OpenAI rejects with "Invalid schema for function ... None is
// not of type 'object'".
func TestBuildToolDefinitionsNilPropertiesBecomesEmptyObject(t *testing.T) {
	t.Parallel()

	tool := staticParametersTool{
		info: fantasy.ToolInfo{
			Name:        "document_graphql_schema",
			Description: "Run a GraphQL query",
			Parameters:  nil,
		},
	}

	defs := BuildToolDefinitions([]fantasy.AgentTool{tool}, nil, nil)
	require.Len(t, defs, 1)

	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok, "expected a fantasy.FunctionTool")

	properties, ok := ft.InputSchema["properties"].(map[string]any)
	require.True(t, ok, "properties must be a map, got %T", ft.InputSchema["properties"])
	require.NotNil(t, properties, "properties must not be nil")

	// Verify it serializes to {} not null.
	bs, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"object","properties":{}}`, string(bs))
}

func TestBuildToolDefinitionsFiltersInactiveToolsAndAppendsProviderTools(t *testing.T) {
	t.Parallel()

	active := staticParametersTool{info: fantasy.ToolInfo{Name: "active"}}
	inactive := staticParametersTool{info: fantasy.ToolInfo{Name: "inactive"}}
	provider := fantasy.ProviderDefinedTool{ID: "web_search", Name: "web_search"}

	defs := BuildToolDefinitions(
		[]fantasy.AgentTool{active, inactive},
		[]string{"active"},
		[]ProviderTool{{Definition: provider}},
	)

	require.Len(t, defs, 2)
	function, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	require.Equal(t, "active", function.Name)
	require.Equal(t, provider, defs[1])
}

func TestBuildToolDefinitionsPreservesFunctionToolOptions(t *testing.T) {
	t.Parallel()

	providerOptions := fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ProviderOptions{},
	}
	tool := staticParametersTool{
		info:            fantasy.ToolInfo{Name: "test_tool"},
		providerOptions: providerOptions,
	}

	defs := BuildToolDefinitions([]fantasy.AgentTool{tool}, nil, nil)
	require.Len(t, defs, 1)
	function, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	require.Equal(t, "test_tool", function.Name)
	require.Equal(t, providerOptions, function.ProviderOptions)
}
