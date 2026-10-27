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

func TestNormalizeKeepsExistingAnyOf(t *testing.T) {
	t.Parallel()

	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"type": ["string", "array"], "anyOf": [{"minLength": 1}]}`), &schema))
	got, err := json.Marshal(toolschema.Normalize(schema))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"anyOf": [{"type": "string"}, {"type": "array", "items": {}}],
		"allOf": [{"anyOf": [{"minLength": 1}]}]
	}`, string(got))
}
