package chatloop

import (
	"encoding/json"
	"fmt"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/jsonschema-go/jsonschema"
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
// removed from the root after their properties and required names fold
// into it, with an object root enforced, dangling "required" entries
// filtered, and every other key (nested combinators, $defs,
// additionalProperties, description) preserved.
func TestBuildToolDefinitionsFullSchemaBannedRootKeywords(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"$ref":  "#/$defs/Missing",
		"oneOf": []any{map[string]any{"required": []any{"kept"}}},
		"anyOf": []any{map[string]any{"required": []any{"dangling"}}},
		"allOf": []any{map[string]any{"properties": map[string]any{"merged": map[string]any{"type": "string"}}}},
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
		"required":             []any{"kept", "merged"},
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
			"kept": {"oneOf": [{"$ref": "#/$defs/Kept"}, {"type": "integer"}]},
			"merged": {"type": "string"}
		},
		"required": ["kept", "merged"],
		"$defs": {"Kept": {"type": "string"}},
		"additionalProperties": false,
		"description": "root description"
	}`, string(raw))
	// The tool's own schema map keeps its top-level keys.
	require.Contains(t, source, "oneOf")
	require.Contains(t, source, "$ref")
	require.Equal(t, []any{"kept", "merged"}, source["required"])
}

// TestBuildToolDefinitionsFullSchemaRootComposition verifies that an
// object root built from "$ref", "allOf", and "anyOf" keeps its
// parameters once those keywords leave the root, and that references
// into the removed keywords still resolve.
func TestBuildToolDefinitionsFullSchemaRootComposition(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": "object",
		"$ref": "#/$defs/Base",
		"properties": map[string]any{
			"alias": map[string]any{"$ref": "#/allOf/0/properties/name"},
		},
		"allOf": []any{
			map[string]any{
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
				"required":   []any{"name"},
			},
		},
		"anyOf": []any{
			map[string]any{
				"properties": map[string]any{"email": map[string]any{"type": "string"}},
				"required":   []any{"email", "phone"},
			},
			map[string]any{
				"properties": map[string]any{
					"email": map[string]any{"type": "string", "format": "email"},
					"phone": map[string]any{"type": "string"},
				},
				"required": []any{"phone"},
			},
		},
		"$defs": map[string]any{
			"Base": map[string]any{
				"properties": map[string]any{"id": map[string]any{"type": "integer"}},
				"required":   []any{"id"},
			},
		},
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
			"alias": {"$ref": "#/$defs/coder_root_allOf_0_properties_name"},
			"id": {"type": "integer"},
			"name": {"type": "string"},
			"email": {"anyOf": [{"type": "string"}, {"type": "string", "format": "email"}]},
			"phone": {"anyOf": [{}, {"type": "string"}]}
		},
		"required": ["id", "name", "phone"],
		"$defs": {
			"Base": {
				"properties": {"id": {"type": "integer"}},
				"required": ["id"]
			},
			"coder_root_allOf_0_properties_name": {"type": "string"}
		}
	}`, string(raw))

	instance := `{"alias": "a", "id": 1, "name": "n", "phone": "p"}`
	require.NoError(t, validateSchemaInstance(t, source, instance), "the source schema must accept the instance")
	require.NoError(t, validateSchemaInstance(t, ft.InputSchema, instance))
	require.Error(t, validateSchemaInstance(t, source, `{"alias": 1, "id": 1, "name": "n", "phone": "p"}`))
	require.Error(t, validateSchemaInstance(t, ft.InputSchema, `{"alias": 1, "id": 1, "name": "n", "phone": "p"}`))
}

// TestBuildToolDefinitionsFullSchemaRootCompositionAbsoluteRefs verifies
// that root composition follows references written as the root "$id"
// URI, not only fragment references, and that other such references
// reach the wire as fragment references.
func TestBuildToolDefinitionsFullSchemaRootCompositionAbsoluteRefs(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"$id":  "https://example.com/tool.json",
		"$ref": "https://example.com/tool.json#/$defs/Base",
		"allOf": []any{
			map[string]any{"$ref": "tool.json#/$defs/Named"},
		},
		"properties": map[string]any{
			"alias": map[string]any{"$ref": "https://example.com/tool.json#/$defs/Named/properties/name"},
		},
		"$defs": map[string]any{
			"Base": map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "integer"}},
				"required":   []any{"id"},
			},
			"Named": map[string]any{
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
				"required":   []any{"name"},
			},
		},
	}
	defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"$id": "https://example.com/tool.json",
		"type": "object",
		"properties": {
			"alias": {"$ref": "#/$defs/Named/properties/name"},
			"id": {"type": "integer"},
			"name": {"type": "string"}
		},
		"required": ["id", "name"],
		"$defs": {
			"Base": {
				"type": "object",
				"properties": {"id": {"type": "integer"}},
				"required": ["id"]
			},
			"Named": {
				"properties": {"name": {"type": "string"}},
				"required": ["name"]
			}
		}
	}`, string(raw))
}

// TestBuildToolDefinitionsFullSchemaRootCompositionTransitiveRefs
// verifies that removed root content stays resolvable when only other
// relocated content points into it, and that each relocated schema gets
// a definition of its own, since Anthropic models send a JSON string for
// an object parameter behind a reference nested deeper.
func TestBuildToolDefinitionsFullSchemaRootCompositionTransitiveRefs(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filter": map[string]any{"$ref": "#/allOf/0/$defs/Filter"},
		},
		"allOf": []any{
			map[string]any{
				"$defs": map[string]any{
					"Filter": map[string]any{"$ref": "#/anyOf/0/$defs/Term"},
				},
			},
		},
		"anyOf": []any{
			map[string]any{
				"$defs": map[string]any{
					"Term": map[string]any{"type": "string", "minLength": 3},
				},
			},
		},
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
			"filter": {"$ref": "#/$defs/coder_root_allOf_0_defs_Filter"}
		},
		"$defs": {
			"coder_root_allOf_0_defs_Filter": {"$ref": "#/$defs/coder_root_anyOf_0_defs_Term"},
			"coder_root_anyOf_0_defs_Term": {"type": "string", "minLength": 3}
		}
	}`, string(raw))
	require.NoError(t, validateSchemaInstance(t, ft.InputSchema, `{"filter": "abc"}`))
	require.Error(t, validateSchemaInstance(t, ft.InputSchema, `{"filter": "ab"}`))
}

// TestBuildToolDefinitionsFullSchemaRootCompositionNestedTargets
// verifies that a referenced schema inside another one stays inside it,
// so that the content is not sent twice, and that a removed keyword
// whose referenced content belongs to an embedded resource moves as a
// whole, so that references in that resource keep resolving against it.
func TestBuildToolDefinitionsFullSchemaRootCompositionNestedTargets(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"outer":  map[string]any{"$ref": "#/allOf/0/$defs/Outer"},
			"inner":  map[string]any{"$ref": "#/allOf/0/$defs/Outer/properties/v"},
			"remote": map[string]any{"$ref": "#/anyOf/0/$defs/R/$defs/S"},
		},
		"allOf": []any{
			map[string]any{"$defs": map[string]any{
				"Outer": map[string]any{
					"type":       "object",
					"properties": map[string]any{"v": map[string]any{"type": "string", "minLength": 2}},
				},
			}},
		},
		"anyOf": []any{
			map[string]any{"$defs": map[string]any{
				"R": map[string]any{
					"$id": "https://example.com/r.json",
					"$defs": map[string]any{
						"S": map[string]any{"$ref": "#/$defs/T"},
						"T": map[string]any{"type": "integer"},
					},
				},
			}},
		},
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
			"outer": {"$ref": "#/$defs/coder_root_allOf_0_defs_Outer"},
			"inner": {"$ref": "#/$defs/coder_root_allOf_0_defs_Outer/properties/v"},
			"remote": {"$ref": "#/$defs/coder_root_anyOf/anyOf/0/$defs/R/$defs/S"}
		},
		"$defs": {
			"coder_root_allOf_0_defs_Outer": {
				"type": "object",
				"properties": {"v": {"type": "string", "minLength": 2}}
			},
			"coder_root_anyOf": {
				"anyOf": [{"$defs": {"R": {
					"$id": "https://example.com/r.json",
					"$defs": {"S": {"$ref": "#/$defs/T"}, "T": {"type": "integer"}}
				}}}]
			}
		}
	}`, string(raw))
	require.NoError(t, validateSchemaInstance(t, ft.InputSchema, `{"outer": {"v": "ab"}, "inner": "ab", "remote": 1}`))
	require.Error(t, validateSchemaInstance(t, ft.InputSchema, `{"inner": "a"}`))
	require.Error(t, validateSchemaInstance(t, ft.InputSchema, `{"remote": "x"}`))
}

// TestBuildToolDefinitionsFullSchemaRootCompositionResourceBranches
// verifies that a root "allOf" branch declaring its own "$id" still
// contributes its parameters when it holds no references, which would
// resolve differently once its content leaves that resource.
func TestBuildToolDefinitionsFullSchemaRootCompositionResourceBranches(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": "object",
		"allOf": []any{
			map[string]any{
				"$id":        "https://example.com/plain.json",
				"properties": map[string]any{"a": map[string]any{"type": "string"}},
				"required":   []any{"a"},
			},
			map[string]any{
				"$id":        "https://example.com/linked.json",
				"properties": map[string]any{"c": map[string]any{"$ref": "#/$defs/C"}},
				"required":   []any{"c"},
				"$defs":      map[string]any{"C": map[string]any{"type": "integer"}},
			},
			map[string]any{
				"properties": map[string]any{"b": map[string]any{"type": "integer"}},
				"required":   []any{"b"},
			},
		},
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
			"a": {"type": "string"},
			"b": {"type": "integer"}
		},
		"required": ["a", "b"]
	}`, string(raw))
}

// TestBuildToolDefinitionsFullSchemaRootRefTargets verifies that a root
// "$ref" contributes its parameters however it names its target: through
// a relative or differently spelled root "$id", or through the "$id" of a
// resource the schema embeds, which merges only when it holds no
// references of its own.
func TestBuildToolDefinitionsFullSchemaRootRefTargets(t *testing.T) {
	t.Parallel()

	params := `{"properties": {"p": {"type": "string"}}, "required": ["p"]}`
	for _, tc := range []struct {
		name   string
		schema string
		want   string
	}{
		{
			name:   "relative root $id",
			schema: `{"$id": "tool.json", "$ref": "tool.json#/$defs/X", "$defs": {"X": ` + params + `}}`,
			want:   params,
		},
		{
			name:   "root $id with a different host case",
			schema: `{"$id": "https://Example.COM/t.json", "$ref": "https://example.com/t.json#/$defs/X", "$defs": {"X": ` + params + `}}`,
			want:   params,
		},
		{
			name: "embedded resource named by its $id",
			schema: `{
				"$id": "https://example.com/root.json",
				"$ref": "https://example.com/other.json",
				"$defs": {"O": {"$id": "other.json", "properties": {"p": {"type": "string"}}, "required": ["p"]}}
			}`,
			want: params,
		},
		{
			name: "embedded resource without a root $id",
			schema: `{
				"$ref": "https://example.com/other.json#/$defs/X",
				"$defs": {"O": {"$id": "https://example.com/other.json", "$defs": {"X": ` + params + `}}}
			}`,
			want: params,
		},
		{
			// The reference inside resolves against "linked.json", so
			// its content cannot move into the root resource.
			name: "embedded resource with references",
			schema: `{
				"$ref": "https://example.com/linked.json#/$defs/X",
				"$defs": {"L": {
					"$id": "https://example.com/linked.json",
					"$defs": {"X": {"properties": {"p": {"$ref": "#/$defs/S"}}}, "S": {"type": "string"}}
				}}
			}`,
			want: `{"properties": {}}`,
		},
		{
			name:   "anchor",
			schema: `{"$ref": "#args", "$defs": {"Args": {"$anchor": "args", "properties": {"p": {"type": "string"}}, "required": ["p"]}}}`,
			want:   params,
		},
		{
			name:   "root $id with an anchor",
			schema: `{"$id": "https://example.com/t.json", "$ref": "https://example.com/t.json#args", "$defs": {"Args": {"$anchor": "args", "properties": {"p": {"type": "string"}}, "required": ["p"]}}}`,
			want:   params,
		},
		{
			name:   "draft 7 anchor",
			schema: `{"$ref": "#args", "definitions": {"Args": {"$id": "#args", "properties": {"p": {"type": "string"}}, "required": ["p"]}}}`,
			want:   params,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var source map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.schema), &source))
			defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
			require.Len(t, defs, 1)
			ft, ok := defs[0].(fantasy.FunctionTool)
			require.True(t, ok)
			got := map[string]any{"properties": ft.InputSchema["properties"]}
			if required, ok := ft.InputSchema["required"]; ok {
				got["required"] = required
			}
			raw, err := json.Marshal(got)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(raw))
		})
	}
}

// TestBuildToolDefinitionsFullSchemaKeepsInstanceValues verifies that
// normalization rewrites type lists only where they are schemas, so
// "const", "enum", and "default" values that look like schemas reach
// the provider unchanged, and that the tool's schema is not modified.
func TestBuildToolDefinitionsFullSchemaKeepsInstanceValues(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"payload": map[string]any{"$ref": "#/$defs/Payload"},
			// A property named like a keyword is still a schema.
			"const": map[string]any{"type": []any{"string", "null"}},
		},
		"$defs": map[string]any{
			"Payload": map[string]any{
				"type":    "object",
				"const":   map[string]any{"type": []any{"string", "number"}},
				"enum":    []any{map[string]any{"type": []any{"boolean"}}},
				"default": map[string]any{"type": "array"},
			},
		},
	}
	before, err := json.Marshal(source)
	require.NoError(t, err)

	defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {
			"payload": {"$ref": "#/$defs/Payload"},
			"const": {"anyOf": [{"type": "string"}, {"type": "null"}]}
		},
		"$defs": {
			"Payload": {
				"type": "object",
				"const": {"type": ["string", "number"]},
				"anyOf": [{"const": {"type": ["boolean"]}}],
				"default": {"type": "array"}
			}
		}
	}`, string(raw))

	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "the tool's schema must not be modified")
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

// TestBuildToolDefinitionsFullSchemaRootAlternativesMissingProperty
// verifies that a root alternative leaving a property unconstrained
// keeps it unconstrained in the fold, while an alternative that forbids
// additional properties contributes nothing for it.
func TestBuildToolDefinitionsFullSchemaRootAlternativesMissingProperty(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		source   map[string]any
		want     string
		accepted []string
	}{
		{
			name: "OpenAlternative",
			source: map[string]any{
				"anyOf": []any{
					map[string]any{
						"properties": map[string]any{"a": map[string]any{"type": "string"}},
					},
					map[string]any{
						"properties": map[string]any{"b": map[string]any{"type": "number"}},
						"required":   []any{"b"},
					},
				},
			},
			want: `{
				"type": "object",
				"properties": {
					"a": {"anyOf": [{"type": "string"}, {}]},
					"b": {"anyOf": [{}, {"type": "number"}]}
				}
			}`,
			accepted: []string{`{"a": 1, "b": 2}`, `{"a": "x", "b": "y"}`},
		},
		{
			name: "PatternAlternative",
			source: map[string]any{
				"anyOf": []any{
					map[string]any{
						"properties": map[string]any{"a": map[string]any{"type": "string"}},
					},
					map[string]any{
						"patternProperties":    map[string]any{"^a$": map[string]any{"type": "number"}},
						"additionalProperties": false,
					},
				},
			},
			want: `{
				"type": "object",
				"properties": {"a": {"anyOf": [{"type": "string"}, {}]}}
			}`,
			accepted: []string{`{"a": 1}`},
		},
		{
			// A false alternative never holds, so the other one decides.
			name: "FalseAlternative",
			source: map[string]any{
				"anyOf": []any{
					false,
					map[string]any{
						"properties": map[string]any{"q": map[string]any{"type": "string"}},
						"required":   []any{"q"},
					},
				},
			},
			want: `{
				"type": "object",
				"properties": {"q": {"type": "string"}},
				"required": ["q"]
			}`,
			accepted: []string{`{"q": "x"}`},
		},
		{
			name: "ClosedAlternatives",
			source: map[string]any{
				"oneOf": []any{
					map[string]any{
						"properties": map[string]any{
							"kind": map[string]any{"const": "x"},
							"x":    map[string]any{"type": "string"},
						},
						"required":             []any{"kind"},
						"additionalProperties": false,
					},
					map[string]any{
						"$ref": "#/$defs/Y",
					},
				},
				"$defs": map[string]any{
					"Y": map[string]any{
						"properties": map[string]any{
							"kind": map[string]any{"const": "y"},
							"y":    map[string]any{"type": "integer"},
						},
						"required":             []any{"kind"},
						"additionalProperties": false,
					},
				},
			},
			want: `{
				"type": "object",
				"properties": {
					"kind": {"anyOf": [{"const": "x"}, {"const": "y"}]},
					"x": {"type": "string"},
					"y": {"type": "integer"}
				},
				"required": ["kind"],
				"$defs": {
					"Y": {
						"properties": {
							"kind": {"const": "y"},
							"y": {"type": "integer"}
						},
						"required": ["kind"],
						"additionalProperties": false
					}
				}
			}`,
			accepted: []string{`{"kind": "x", "x": "a"}`, `{"kind": "y", "y": 1}`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: tc.source}}, nil, nil)
			require.Len(t, defs, 1)
			ft, ok := defs[0].(fantasy.FunctionTool)
			require.True(t, ok)
			raw, err := json.Marshal(ft.InputSchema)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(raw))
			for _, instance := range tc.accepted {
				require.NoError(t, validateSchemaInstance(t, tc.source, instance), "the source schema must accept %s", instance)
				require.NoError(t, validateSchemaInstance(t, ft.InputSchema, instance))
			}
		})
	}
}

// TestBuildToolDefinitionsFullSchemaRootFoldUndeclaredRequired verifies
// that root folding keeps a required name that has no property schema,
// and gives it the schema that governs it, since some providers reject
// a required name missing from "properties".
func TestBuildToolDefinitionsFullSchemaRootFoldUndeclaredRequired(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		source   string
		want     string
		accepted []string
		rejected []string
	}{
		{
			name:     "AnyValue",
			source:   `{"type": "object", "required": ["query"], "allOf": [{}]}`,
			want:     `{"type": "object", "properties": {"query": {}}, "required": ["query"]}`,
			accepted: []string{`{"query": 1}`},
			rejected: []string{`{}`},
		},
		{
			name:     "AdditionalProperties",
			source:   `{"type": "object", "required": ["query"], "additionalProperties": {"type": "string"}, "allOf": [{}]}`,
			want:     `{"type": "object", "properties": {"query": {"type": "string"}}, "required": ["query"], "additionalProperties": {"type": "string"}}`,
			accepted: []string{`{"query": "x"}`},
			rejected: []string{`{}`, `{"query": 1}`},
		},
		{
			// The pattern keeps applying beside the property.
			name:     "PatternProperties",
			source:   `{"type": "object", "required": ["x-id"], "patternProperties": {"^x-": {"type": "integer"}}, "additionalProperties": false, "allOf": [{}]}`,
			want:     `{"type": "object", "properties": {"x-id": {}}, "required": ["x-id"], "patternProperties": {"^x-": {"type": "integer"}}, "additionalProperties": false}`,
			accepted: []string{`{"x-id": 1}`},
			rejected: []string{`{}`, `{"x-id": "a"}`},
		},
		{
			name:     "RequiredByEveryAlternative",
			source:   `{"type": "object", "oneOf": [{"required": ["id"]}, {"required": ["id", "name"]}]}`,
			want:     `{"type": "object", "properties": {"id": {}}, "required": ["id"]}`,
			accepted: []string{`{"id": 1}`},
			rejected: []string{`{"name": "x"}`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var source map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.source), &source))
			defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
			require.Len(t, defs, 1)
			ft, ok := defs[0].(fantasy.FunctionTool)
			require.True(t, ok)
			raw, err := json.Marshal(ft.InputSchema)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(raw))
			for _, instance := range tc.accepted {
				require.NoError(t, validateSchemaInstance(t, ft.InputSchema, instance))
			}
			for _, instance := range tc.rejected {
				require.Error(t, validateSchemaInstance(t, source, instance), "the source schema must reject %s", instance)
				require.Error(t, validateSchemaInstance(t, ft.InputSchema, instance))
			}
		})
	}
}

// TestBuildToolDefinitionsFullSchemaRootFoldBoundsWork verifies that
// root folding stays cheap when many branches reference the same
// schemas, which it would otherwise expand again on every path.
func TestBuildToolDefinitionsFullSchemaRootFoldBoundsWork(t *testing.T) {
	t.Parallel()

	const levels, fanOut = 8, 10
	refs := func(level int) []any {
		branches := make([]any, fanOut)
		for i := range branches {
			branches[i] = map[string]any{"$ref": fmt.Sprintf("#/$defs/L%d", level)}
		}
		return branches
	}
	defs := map[string]any{}
	want := map[string]any{}
	for level := 0; level <= levels; level++ {
		name := fmt.Sprintf("p%d", level)
		def := map[string]any{"properties": map[string]any{name: map[string]any{"type": "string"}}}
		// The last level points back to the first, so no expansion is
		// complete enough to reuse.
		def["allOf"] = refs((level + 1) % (levels + 1))
		defs[fmt.Sprintf("L%d", level)] = def
		want[name] = map[string]any{"type": "string"}
	}
	source := map[string]any{"type": "object", "allOf": refs(0), "$defs": defs}

	inputSchema := ToolInputSchema(fullSchemaTestTool{schema: source})
	require.Equal(t, want, inputSchema["properties"])
}

// TestBuildToolDefinitionsFullSchemaRootFoldBoundsPatternWork verifies
// that root folding stays cheap when many required names the root does
// not declare meet many "patternProperties" patterns, and that names it
// leaves unchecked stay required with any value allowed.
func TestBuildToolDefinitionsFullSchemaRootFoldBoundsPatternWork(t *testing.T) {
	t.Parallel()

	const names, patterns = 2000, 2000
	required := make([]any, names)
	want := map[string]any{}
	for i := range required {
		name := fmt.Sprintf("n%d", i)
		required[i] = name
		want[name] = map[string]any{}
	}
	patternProperties := map[string]any{}
	for i := range patterns {
		patternProperties[fmt.Sprintf("^p%d$", i)] = map[string]any{"type": "string"}
	}
	source := map[string]any{
		"type":                 "object",
		"required":             required,
		"patternProperties":    patternProperties,
		"additionalProperties": false,
		"allOf":                []any{map[string]any{}},
	}

	inputSchema := ToolInputSchema(fullSchemaTestTool{schema: source})
	require.Equal(t, want, inputSchema["properties"])
	require.Len(t, inputSchema["required"], names)
}

// TestBuildToolDefinitionsFullSchemaRootFoldLegacyRef verifies that root
// folding ignores the keywords beside "$ref" in a schema declaring draft
// 7, as that draft does, so the folded root accepts what the source
// accepts. Without a declared draft they hold beside the reference.
func TestBuildToolDefinitionsFullSchemaRootFoldLegacyRef(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		source   string
		want     string
		accepted []string
		rejected []string
	}{
		{
			name:     "Branch",
			source:   `{"$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "definitions": {"N": {"properties": {"q": {"type": "number"}}}}, "allOf": [{"$ref": "#/definitions/N", "properties": {"q": {"type": "string"}}, "required": ["q"]}]}`,
			want:     `{"$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "definitions": {"N": {"properties": {"q": {"type": "number"}}}}, "properties": {"q": {"type": "number"}}}`,
			accepted: []string{`{"q": 1}`, `{}`},
			rejected: []string{`{"q": "a"}`},
		},
		{
			// A reference into an ignored keyword still resolves.
			name:     "Root",
			source:   `{"$schema": "http://json-schema.org/draft-07/schema#", "$ref": "#/definitions/A", "type": "object", "properties": {"x": {"type": "string"}}, "required": ["x"], "additionalProperties": false, "definitions": {"A": {"properties": {"y": {"type": "integer"}, "z": {"$ref": "#/properties/x"}}, "required": ["y"]}}}`,
			want:     `{"$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "properties": {"y": {"type": "integer"}, "z": {"$ref": "#/definitions/coder_root_properties_x"}}, "required": ["y"], "definitions": {"A": {"properties": {"y": {"type": "integer"}, "z": {"$ref": "#/definitions/coder_root_properties_x"}}, "required": ["y"]}, "coder_root_properties_x": {"type": "string"}}}`,
			accepted: []string{`{"y": 1}`, `{"y": 1, "x": 1}`, `{"y": 1, "z": "a"}`},
			rejected: []string{`{"x": "a"}`, `{"y": "a"}`, `{"y": 1, "z": 1}`},
		},
		{
			name:     "LaterDraft",
			source:   `{"type": "object", "$defs": {"N": {"properties": {"q": {"type": "number"}}}}, "allOf": [{"$ref": "#/$defs/N", "properties": {"q": {"type": "string"}}, "required": ["q"]}]}`,
			want:     `{"type": "object", "$defs": {"N": {"properties": {"q": {"type": "number"}}}}, "properties": {"q": {"allOf": [{"type": "number"}, {"type": "string"}]}}, "required": ["q"]}`,
			rejected: []string{`{"q": 1}`, `{"q": "a"}`, `{}`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var source map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.source), &source))
			inputSchema := ToolInputSchema(fullSchemaTestTool{schema: source})
			raw, err := json.Marshal(inputSchema)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(raw))
			for _, instance := range tc.accepted {
				require.NoError(t, validateSchemaInstance(t, source, instance), "the source schema must accept %s", instance)
				require.NoError(t, validateSchemaInstance(t, inputSchema, instance))
			}
			for _, instance := range tc.rejected {
				require.Error(t, validateSchemaInstance(t, source, instance), "the source schema must reject %s", instance)
				require.Error(t, validateSchemaInstance(t, inputSchema, instance))
			}
		})
	}
}

// TestBuildToolDefinitionsFullSchemaRootFoldSharedRefs verifies that
// root folding keeps every parameter when branches share references
// many times over or chain them deeply, which a bound on the work done
// would otherwise cut short.
func TestBuildToolDefinitionsFullSchemaRootFoldSharedRefs(t *testing.T) {
	t.Parallel()

	leaf := map[string]any{
		"properties": map[string]any{"q": map[string]any{"type": "string"}},
		"required":   []any{"q"},
	}
	sibling := map[string]any{
		"properties": map[string]any{"r": map[string]any{"type": "string", "minLength": 3}},
		"required":   []any{"r"},
	}
	for _, tc := range []struct {
		name   string
		levels int
		fanOut int
	}{
		// Every level references the next twice, which expands to about
		// a thousand merges when each path is followed again.
		{name: "SharedReferences", levels: 9, fanOut: 2},
		{name: "LongChain", levels: 40, fanOut: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			defs := map[string]any{fmt.Sprintf("D%d", tc.levels): leaf}
			for level := range tc.levels {
				branches := make([]any, tc.fanOut)
				for i := range branches {
					branches[i] = map[string]any{"$ref": fmt.Sprintf("#/$defs/D%d", level+1)}
				}
				defs[fmt.Sprintf("D%d", level)] = map[string]any{"allOf": branches}
			}
			source := map[string]any{
				"type":  "object",
				"allOf": []any{map[string]any{"$ref": "#/$defs/D0"}, sibling},
				"$defs": defs,
			}

			inputSchema := ToolInputSchema(fullSchemaTestTool{schema: source})
			require.Equal(t, map[string]any{
				"q": map[string]any{"type": "string"},
				"r": map[string]any{"type": "string", "minLength": 3},
			}, inputSchema["properties"])
			require.Equal(t, []string{"q", "r"}, inputSchema["required"])
		})
	}
}

// TestBuildToolDefinitionsFullSchemaAnchorRefs verifies that a
// reference to an anchor reaches the provider as a JSON Pointer to the
// anchored schema, which providers and draft 7 validators follow even
// where they ignore anchors, so the constraints it names keep holding.
func TestBuildToolDefinitionsFullSchemaAnchorRefs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		source  map[string]any
		wantRef string
	}{
		{
			name: "Draft7Anchor",
			source: map[string]any{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type":    "object",
				"properties": map[string]any{
					"code": map[string]any{"$ref": "#code"},
				},
				"definitions": map[string]any{
					"Code": map[string]any{"$id": "#code", "type": "string", "minLength": 3},
				},
			},
			wantRef: "#/definitions/Code",
		},
		{
			// The anchored schema moves out of the removed root "anyOf"
			// into a definition of its own, and the reference follows it.
			name: "AnchorInRemovedRootKeyword",
			source: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{"$ref": "#code"},
				},
				"anyOf": []any{
					map[string]any{
						"$defs": map[string]any{
							"Code": map[string]any{"$anchor": "code", "type": "string", "minLength": 3},
						},
					},
				},
			},
			wantRef: "#/$defs/coder_root_anyOf_0_defs_Code",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			defs := BuildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: tc.source}}, nil, nil)
			require.Len(t, defs, 1)
			ft, ok := defs[0].(fantasy.FunctionTool)
			require.True(t, ok)
			properties, ok := ft.InputSchema["properties"].(map[string]any)
			require.True(t, ok)
			code, ok := properties["code"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, tc.wantRef, code["$ref"])
			require.NoError(t, validateSchemaInstance(t, ft.InputSchema, `{"code": "abc"}`))
			require.Error(t, validateSchemaInstance(t, ft.InputSchema, `{"code": "ab"}`))
		})
	}
}

// validateSchemaInstance resolves schema with a JSON Schema validator,
// which fails on references that point at nothing, and validates the
// JSON instance against it.
func validateSchemaInstance(t *testing.T, schema map[string]any, instance string) error {
	t.Helper()
	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	var parsed jsonschema.Schema
	require.NoError(t, json.Unmarshal(raw, &parsed))
	resolved, err := parsed.Resolve(nil)
	require.NoError(t, err)
	var value any
	require.NoError(t, json.Unmarshal([]byte(instance), &value))
	return resolved.Validate(value)
}
