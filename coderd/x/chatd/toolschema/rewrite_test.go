package toolschema_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/toolschema"
)

func TestRewriteRefs(t *testing.T) {
	t.Parallel()

	prefix := func(tokens []string) ([]string, bool) {
		return append([]string{"moved"}, tokens...), true
	}
	for _, tc := range []struct {
		name   string
		schema string
		want   string
	}{
		{
			name:   "anchors and instance values stay",
			schema: `{"properties": {"a": {"$ref": "#name"}}, "enum": [{"$ref": "#/x"}], "x-extension": {"$ref": "#/x"}}`,
			want:   `{"properties": {"a": {"$ref": "#name"}}, "enum": [{"$ref": "#/x"}], "x-extension": {"$ref": "#/x"}}`,
		},
		{
			name:   "draft 7 tuple items and dependencies are schemas",
			schema: `{"items": [{"$ref": "#/a"}], "dependencies": {"b": {"$ref": "#/a"}, "c": ["d"]}}`,
			want:   `{"items": [{"$ref": "#/moved/a"}], "dependencies": {"b": {"$ref": "#/moved/a"}, "c": ["d"]}}`,
		},
		{
			name:   "a fragment-only $id is an anchor, not a resource",
			schema: `{"properties": {"a": {"$id": "#a", "$ref": "#/b"}}}`,
			want:   `{"properties": {"a": {"$id": "#a", "$ref": "#/moved/b"}}}`,
		},
		{
			name:   "a nested resource without a root $id keeps its pointers",
			schema: `{"properties": {"a": {"$id": "sub.json", "properties": {"b": {"$ref": "#/c"}}}}}`,
			want:   `{"properties": {"a": {"$id": "sub.json", "properties": {"b": {"$ref": "#/c"}}}}}`,
		},
		{
			name:   "tokens are unescaped and escaped again",
			schema: `{"properties": {"a": {"$ref": "#/a~1b/c~0d/%25"}}}`,
			want:   `{"properties": {"a": {"$ref": "#/moved/a~1b/c~0d/%25"}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var schema map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.schema), &schema))
			got, err := json.Marshal(toolschema.RewriteRefs(schema, prefix))
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
			source, err := json.Marshal(schema)
			require.NoError(t, err)
			require.JSONEq(t, tc.schema, string(source), "the source must not be modified")
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		schema string
		want   string
	}{
		{
			// References into the existing "anyOf" keep resolving.
			name:   "an existing anyOf keeps its place",
			schema: `{"type": ["string", "array"], "anyOf": [{"minLength": 1}], "allOf": [{"maxLength": 9}]}`,
			want: `{
				"anyOf": [{"minLength": 1}],
				"allOf": [{"maxLength": 9}, {"anyOf": [{"type": "string"}, {"type": "array", "items": {}}]}]
			}`,
		},
		{
			// OpenAI rejects an "enum" with array or object members but
			// accepts the same values as "const".
			name:   "an enum with composite members becomes consts",
			schema: `{"enum": [["a", "b"], {"type": ["string", "null"]}, "plain", 1.5, null]}`,
			want: `{"anyOf": [
				{"const": ["a", "b"]},
				{"const": {"type": ["string", "null"]}},
				{"const": "plain"},
				{"const": 1.5},
				{"const": null}
			]}`,
		},
		{
			name:   "an enum of scalars stays",
			schema: `{"type": "string", "enum": ["a", "b", null]}`,
			want:   `{"type": "string", "enum": ["a", "b", null]}`,
		},
		{
			name:   "a type list and a composite enum both apply",
			schema: `{"type": ["array", "null"], "enum": [["a"], null]}`,
			want: `{
				"anyOf": [{"type": "array", "items": {}}, {"type": "null"}],
				"allOf": [{"anyOf": [{"const": ["a"]}, {"const": null}]}]
			}`,
		},
		{
			// Some providers resolve only fragment references, so a
			// reference that names the root resource by its URI is
			// written as a fragment. A nested resource keeps the URI,
			// because a fragment there would resolve against itself.
			name: "references to the root resource become fragments",
			schema: `{
				"$id": "https://example.com/tool.json",
				"properties": {
					"absolute": {"$ref": "https://example.com/tool.json#/$defs/A"},
					"relative": {"$ref": "tool.json#/$defs/A"},
					"other": {"$ref": "https://example.com/other.json#/$defs/A"},
					"nested": {
						"$id": "nested.json",
						"properties": {"back": {"$ref": "https://example.com/tool.json#/$defs/A"}}
					}
				},
				"$defs": {"A": {"type": "string"}}
			}`,
			want: `{
				"$id": "https://example.com/tool.json",
				"properties": {
					"absolute": {"$ref": "#/$defs/A"},
					"relative": {"$ref": "#/$defs/A"},
					"other": {"$ref": "https://example.com/other.json#/$defs/A"},
					"nested": {
						"$id": "nested.json",
						"properties": {"back": {"$ref": "https://example.com/tool.json#/$defs/A"}}
					}
				},
				"$defs": {"A": {"type": "string"}}
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var schema map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.schema), &schema))
			got, err := json.Marshal(toolschema.Normalize(schema))
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
			source, err := json.Marshal(schema)
			require.NoError(t, err)
			require.JSONEq(t, tc.schema, string(source), "the source must not be modified")
		})
	}
}
