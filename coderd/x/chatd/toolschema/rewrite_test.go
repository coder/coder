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

func TestResolveLocalRef(t *testing.T) {
	t.Parallel()

	const schema = `{
		"$id": "https://example.com/root.json",
		"$defs": {
			"A": {"type": "string"},
			"Named": {"$anchor": "named", "type": "boolean"},
			"D1": {"$anchor": "dup", "type": "string"},
			"D2": {"$anchor": "dup", "type": "number"},
			"N": {"$id": "n.json", "properties": {"x": {"type": "integer"}}, "$defs": {"M": {"$anchor": "inner", "type": "number"}}}
		}
	}`
	var root map[string]any
	require.NoError(t, json.Unmarshal([]byte(schema), &root))
	for _, tc := range []struct {
		name         string
		ref          string
		want         string
		sameResource bool
	}{
		{name: "fragment", ref: "#/$defs/A", want: `{"type": "string"}`, sameResource: true},
		{name: "root $id", ref: "root.json#/$defs/A", want: `{"type": "string"}`, sameResource: true},
		{name: "fragment into an embedded resource", ref: "#/$defs/N/properties/x", want: `{"type": "integer"}`},
		{name: "embedded resource $id", ref: "https://example.com/n.json", want: `{"$id": "n.json", "properties": {"x": {"type": "integer"}}, "$defs": {"M": {"$anchor": "inner", "type": "number"}}}`},
		{name: "pointer in an embedded resource", ref: "n.json#/properties/x", want: `{"type": "integer"}`},
		{name: "anchor", ref: "#named", want: `{"$anchor": "named", "type": "boolean"}`, sameResource: true},
		{name: "root $id with an anchor", ref: "root.json#named", want: `{"$anchor": "named", "type": "boolean"}`, sameResource: true},
		{name: "anchor in an embedded resource", ref: "n.json#inner", want: `{"$anchor": "inner", "type": "number"}`},
		{name: "anchor of another resource", ref: "#inner"},
		{name: "unknown anchor", ref: "#missing"},
		{name: "duplicated anchor", ref: "#dup"},
		{name: "unknown resource", ref: "https://example.com/other.json#/$defs/A"},
		{name: "missing pointer target", ref: "#/$defs/B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			target, sameResource, ok := toolschema.ResolveLocalRef(root, tc.ref)
			if tc.want == "" {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			got, err := json.Marshal(target)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
			require.Equal(t, tc.sameResource, sameResource)
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
		{
			name: "equivalent spellings of the root URI become fragments",
			schema: `{
				"$id": "HTTPS://Example.COM:443/a/%7etool.json",
				"properties": {
					"case": {"$ref": "https://example.com/a/~tool.json#/$defs/A"},
					"dots": {"$ref": "https://example.com/a/b/%2E%2E/%7Etool.json#/$defs/A"},
					"relative": {"$ref": "./~tool.json#/$defs/A"},
					"path case": {"$ref": "https://example.com/A/~tool.json#/$defs/A"},
					"other port": {"$ref": "https://example.com:8443/a/~tool.json#/$defs/A"}
				},
				"$defs": {"A": {"type": "string"}}
			}`,
			want: `{
				"$id": "HTTPS://Example.COM:443/a/%7etool.json",
				"properties": {
					"case": {"$ref": "#/$defs/A"},
					"dots": {"$ref": "#/$defs/A"},
					"relative": {"$ref": "#/$defs/A"},
					"path case": {"$ref": "https://example.com/A/~tool.json#/$defs/A"},
					"other port": {"$ref": "https://example.com:8443/a/~tool.json#/$defs/A"}
				},
				"$defs": {"A": {"type": "string"}}
			}`,
		},
		{
			name: "an empty http path is the root path",
			schema: `{
				"$id": "https://example.com",
				"properties": {"a": {"$ref": "https://example.com/#/$defs/A"}},
				"$defs": {"A": {"type": "string"}}
			}`,
			want: `{
				"$id": "https://example.com",
				"properties": {"a": {"$ref": "#/$defs/A"}},
				"$defs": {"A": {"type": "string"}}
			}`,
		},
		{
			name: "a relative root $id names the root resource",
			schema: `{
				"$id": "tool.json",
				"properties": {
					"a": {"$ref": "tool.json#/$defs/A"},
					"b": {"$ref": "./tool.json#/$defs/A"},
					"other": {"$ref": "other.json#/$defs/A"}
				},
				"$defs": {"A": {"type": "string"}}
			}`,
			want: `{
				"$id": "tool.json",
				"properties": {
					"a": {"$ref": "#/$defs/A"},
					"b": {"$ref": "#/$defs/A"},
					"other": {"$ref": "other.json#/$defs/A"}
				},
				"$defs": {"A": {"type": "string"}}
			}`,
		},
		{
			// Providers validate tool schemas against draft 2020-12,
			// which forbids a non-empty fragment in "$id". Earlier
			// drafts used one to name an anchor, which 2020-12 spells
			// "$anchor".
			name: "fragments in $id become anchors",
			schema: `{
				"$id": "https://example.com/tool.json#root",
				"properties": {
					"a": {"$ref": "https://example.com/tool.json#/$defs/A"},
					"anchor": {"$id": "#anchor", "type": "string"},
					"to anchor": {"$ref": "https://example.com/tool.json#anchor"},
					"nested": {"$id": "nested.json#n", "type": "string"},
					"empty fragment": {"$id": "empty.json#", "type": "string"},
					"pointer": {"$id": "#/properties/pointer", "type": "string"},
					"kept anchor": {"$id": "#old", "$anchor": "new", "type": "string"}
				},
				"$defs": {"A": {"$id": "#A", "type": "string"}}
			}`,
			want: `{
				"$id": "https://example.com/tool.json",
				"$anchor": "root",
				"properties": {
					"a": {"$ref": "#/$defs/A"},
					"anchor": {"$anchor": "anchor", "type": "string"},
					"to anchor": {"$ref": "#/properties/anchor"},
					"nested": {"$id": "nested.json", "$anchor": "n", "type": "string"},
					"empty fragment": {"$id": "empty.json#", "type": "string"},
					"pointer": {"type": "string"},
					"kept anchor": {"$anchor": "new", "type": "string"}
				},
				"$defs": {"A": {"$anchor": "A", "type": "string"}}
			}`,
		},
		{
			name: "a root $id anchor beside $anchor stays referable",
			schema: `{
				"$id": "https://example.com/tool.json#root",
				"$anchor": "other",
				"properties": {
					"child": {"$ref": "#root"},
					"absolute": {"$ref": "https://example.com/tool.json#root"},
					"other": {"$ref": "#other"}
				}
			}`,
			want: `{
				"$id": "https://example.com/tool.json",
				"$anchor": "other",
				"properties": {
					"child": {"$ref": "#"},
					"absolute": {"$ref": "#"},
					"other": {"$ref": "#"}
				}
			}`,
		},
		{
			// Some providers do not resolve anchors, and a draft 7
			// validator does not read "$anchor", so a reference to an
			// anchor of the root resource becomes a JSON Pointer. A
			// reference in another resource, to an anchor of another
			// resource, or to a duplicated anchor stays.
			name: "references to root anchors become pointers",
			schema: `{
				"$anchor": "root",
				"properties": {
					"def": {"$ref": "#A"},
					"draft 7 beside $anchor": {"$ref": "#old"},
					"list item": {"$ref": "#item"},
					"escaped": {"$ref": "#slash"},
					"root": {"$ref": "#root"},
					"duplicate": {"$ref": "#dup"},
					"missing": {"$ref": "#missing"},
					"other resource": {"$ref": "#N"},
					"nested": {"$id": "nested.json", "$anchor": "N", "properties": {"self": {"$ref": "#N"}, "up": {"$ref": "#A"}}}
				},
				"$defs": {
					"A": {"$anchor": "A", "type": "string"},
					"Old": {"$id": "#old", "$anchor": "new", "type": "string"},
					"List": {"allOf": [{"type": "integer"}, {"$anchor": "item", "minimum": 1}]},
					"S/T": {"$anchor": "slash", "type": "boolean"},
					"D1": {"$anchor": "dup", "type": "string"},
					"D2": {"$id": "#dup", "type": "number"}
				}
			}`,
			want: `{
				"$anchor": "root",
				"properties": {
					"def": {"$ref": "#/$defs/A"},
					"draft 7 beside $anchor": {"$ref": "#/$defs/Old"},
					"list item": {"$ref": "#/$defs/List/allOf/1"},
					"escaped": {"$ref": "#/$defs/S~1T"},
					"root": {"$ref": "#"},
					"duplicate": {"$ref": "#dup"},
					"missing": {"$ref": "#missing"},
					"other resource": {"$ref": "#N"},
					"nested": {"$id": "nested.json", "$anchor": "N", "properties": {"self": {"$ref": "#N"}, "up": {"$ref": "#A"}}}
				},
				"$defs": {
					"A": {"$anchor": "A", "type": "string"},
					"Old": {"$anchor": "new", "type": "string"},
					"List": {"allOf": [{"type": "integer"}, {"$anchor": "item", "minimum": 1}]},
					"S/T": {"$anchor": "slash", "type": "boolean"},
					"D1": {"$anchor": "dup", "type": "string"},
					"D2": {"$anchor": "dup", "type": "number"}
				}
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
