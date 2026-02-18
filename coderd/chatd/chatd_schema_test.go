package chatd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/require"
)

func TestSchemaMap_NormalizesRequiredArrays(t *testing.T) {
	t.Parallel()

	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"workspace": {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {Type: "string"},
					"files": {
						Type: "array",
						Items: &jsonschema.Schema{
							Type: "object",
							Properties: map[string]*jsonschema.Schema{
								"path":    {Type: "string"},
								"content": {Type: "string"},
							},
							Required: []string{"path", "content"},
						},
					},
				},
				Required: []string{"name", "files"},
			},
		},
		Required: []string{"workspace"},
	}

	mapped := schemaMap(schema)
	assertRequiredArraysUseStringSlices(t, mapped, "$")

	properties := mapValue(t, mapped["properties"], "$.properties")
	workspace := mapValue(t, properties["workspace"], "$.properties.workspace")
	workspaceProperties := mapValue(t, workspace["properties"], "$.properties.workspace.properties")
	files := mapValue(t, workspaceProperties["files"], "$.properties.workspace.properties.files")
	items := mapValue(t, files["items"], "$.properties.workspace.properties.files.items")

	require.Equal(t, []string{"workspace"}, requiredStrings(t, mapped, "$"))
	require.Equal(t, []string{"name", "files"}, requiredStrings(t, workspace, "$.properties.workspace"))
	require.Equal(t, []string{"path", "content"}, requiredStrings(t, items, "$.properties.workspace.properties.files.items"))
}

func TestNormalizeRequiredArrays_ConvertsEmptyRequiredArray(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"type":     "object",
		"required": []any{},
		"properties": map[string]any{
			"nested": map[string]any{
				"type":     "object",
				"required": []any{"name"},
			},
		},
	}

	normalizeRequiredArrays(input)

	require.Equal(t, []string{}, requiredStrings(t, input, "$"))

	properties := mapValue(t, input["properties"], "$.properties")
	nested := mapValue(t, properties["nested"], "$.properties.nested")
	require.Equal(t, []string{"name"}, requiredStrings(t, nested, "$.properties.nested"))
}

func assertRequiredArraysUseStringSlices(t *testing.T, value any, path string) {
	t.Helper()

	switch v := value.(type) {
	case map[string]any:
		if required, ok := v["required"]; ok {
			_, isStringSlice := required.([]string)
			require.Truef(t, isStringSlice, "required at %s has type %T", path, required)
		}
		for key, child := range v {
			assertRequiredArraysUseStringSlices(t, child, path+"."+key)
		}
	case []any:
		for i, child := range v {
			assertRequiredArraysUseStringSlices(t, child, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

func mapValue(t *testing.T, value any, path string) map[string]any {
	t.Helper()

	m, ok := value.(map[string]any)
	require.True(t, ok, "value at %s has unexpected type %T", path, value)
	return m
}

func requiredStrings(t *testing.T, schema map[string]any, path string) []string {
	t.Helper()

	required, ok := schema["required"].([]string)
	require.True(t, ok, "required at %s has unexpected type %T", path, schema["required"])
	return required
}

func schemaMap(schema *jsonschema.Schema) map[string]any {
	data, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{}
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	normalizeRequiredArrays(out)
	return out
}

func normalizeRequiredArrays(value any) {
	switch v := value.(type) {
	case map[string]any:
		normalizeMap(v)
	case []any:
		for _, item := range v {
			normalizeRequiredArrays(item)
		}
	}
}

func normalizeMap(m map[string]any) {
	if req, ok := m["required"]; ok {
		if arr, ok := req.([]any); ok {
			converted := make([]string, 0, len(arr))
			for _, item := range arr {
				s, isString := item.(string)
				if !isString {
					converted = nil
					break
				}
				converted = append(converted, s)
			}
			if converted != nil {
				m["required"] = converted
			}
		}
	}
	for _, v := range m {
		normalizeRequiredArrays(v)
	}
}
