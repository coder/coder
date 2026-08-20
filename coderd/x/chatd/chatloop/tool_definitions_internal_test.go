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

// fullSchemaTestTool models a tool that reports its complete input
// schema alongside a flattened Info() view.
type fullSchemaTestTool struct {
	fantasy.AgentTool
	schema map[string]any
}

func (fullSchemaTestTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        "full_schema_tool",
		Description: "keeps schemas whole",
		Parameters:  map[string]any{"fallback": map[string]any{"type": "string"}},
	}
}

func (t fullSchemaTestTool) FullInputSchema() map[string]any { return t.schema }

func (fullSchemaTestTool) ProviderOptions() fantasy.ProviderOptions { return nil }

// TestBuildToolDefinitionsFullSchema verifies that a tool exposing a
// full input schema reaches the wire whole, with "required" coerced
// from the []any shape JSON unmarshaling produces to []string so
// provider drivers that assert []string do not drop it.
func TestBuildToolDefinitionsFullSchema(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filter": map[string]any{"$ref": "#/$defs/Filter"},
		},
		"required":             []any{"filter"},
		"additionalProperties": false,
		"$defs": map[string]any{
			"Filter": map[string]any{"type": "string"},
		},
	}
	defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	require.Equal(t, []string{"filter"}, ft.InputSchema["required"])
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {"filter": {"$ref": "#/$defs/Filter"}},
		"required": ["filter"],
		"additionalProperties": false,
		"$defs": {"Filter": {"type": "string"}}
	}`, string(raw))
	// The tool's own schema map is not mutated by the coercion.
	require.Equal(t, []any{"filter"}, source["required"])
}

// TestBuildToolDefinitionsFullSchemaNormalization verifies the
// defensive guards on the full-schema path: a plain object root gets
// "type" defaulted and a non-nil properties object, and an empty
// "required" is dropped so it never serializes to null.
func TestBuildToolDefinitionsFullSchemaNormalization(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"description": "no explicit type",
		"required":    []any{},
	}
	defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"object","properties":{},"description":"no explicit type"}`, string(raw))
}

// TestBuildToolDefinitionsFullSchemaNilFallsBack verifies that a tool
// whose FullInputSchema returns nil produces the same reconstruction
// from Info() as tools without one.
func TestBuildToolDefinitionsFullSchemaNilFallsBack(t *testing.T) {
	t.Parallel()

	defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: nil}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"object","properties":{"fallback":{"type":"string"}}}`, string(raw))
}

// TestBuildToolDefinitionsFullSchemaBannedRootKeywords verifies that
// schema keywords OpenAI rejects at the top level of a tool schema are
// stripped from the root, with an object root enforced, dangling
// "required" entries filtered, and every other key (nested
// combinators, $defs, additionalProperties, description) preserved.
func TestBuildToolDefinitionsFullSchemaBannedRootKeywords(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"$ref":  "#/$defs/Root",
		"oneOf": []any{map[string]any{"required": []any{"kept"}}},
		"anyOf": []any{map[string]any{"required": []any{"lost"}}},
		"allOf": []any{map[string]any{"properties": map[string]any{"lost": map[string]any{"type": "string"}}}},
		"enum":  []any{"a", "b"},
		"const": "a",
		"not":   map[string]any{"type": "string"},
		"properties": map[string]any{
			"kept": map[string]any{
				"oneOf": []any{
					map[string]any{"$ref": "#/$defs/Kept"},
					map[string]any{"type": "integer"},
				},
			},
		},
		"required":             []any{"kept", "lost"},
		"$defs":                map[string]any{"Kept": map[string]any{"type": "string"}},
		"additionalProperties": false,
		"description":          "root description",
	}
	defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {
			"kept": {"oneOf": [{"$ref": "#/$defs/Kept"}, {"type": "integer"}]}
		},
		"required": ["kept"],
		"$defs": {"Kept": {"type": "string"}},
		"additionalProperties": false,
		"description": "root description"
	}`, string(raw))
	// The tool's own schema map keeps its top-level keys.
	require.Contains(t, source, "oneOf")
	require.Contains(t, source, "$ref")
	require.Equal(t, []any{"kept", "lost"}, source["required"])
}

// TestBuildToolDefinitionsFullSchemaRootTypeArray verifies that a root
// type array, which schema.Normalize rewrites into a root "anyOf",
// still reaches the wire as a plain object root instead of the
// top-level combinator OpenAI rejects.
func TestBuildToolDefinitionsFullSchemaRootTypeArray(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": []any{"object", "null"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}
	defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {"name": {"type": "string"}},
		"required": ["name"]
	}`, string(raw))
}
