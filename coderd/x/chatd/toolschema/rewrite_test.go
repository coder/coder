package toolschema_test

import (
	"encoding/json"
	"fmt"
	"strings"
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
			name:   "a dynamic reference by pointer resolves like a reference",
			schema: `{"properties": {"a": {"$dynamicRef": "#/b"}, "c": {"$dynamicRef": "#c"}}}`,
			want:   `{"properties": {"a": {"$dynamicRef": "#/moved/b"}, "c": {"$dynamicRef": "#c"}}}`,
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
			"R1": {"$id": "dup.json", "type": "string"},
			"R2": {"$id": "dup.json", "type": "number"},
			"N": {"$id": "n.json", "properties": {"x": {"type": "integer"}}, "$defs": {"M": {"$anchor": "inner", "type": "number"}, "D": {"$dynamicAnchor": "dynamic", "type": "null"}}}
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
		{name: "embedded resource $id", ref: "https://example.com/n.json", want: `{"$id": "n.json", "properties": {"x": {"type": "integer"}}, "$defs": {"M": {"$anchor": "inner", "type": "number"}, "D": {"$dynamicAnchor": "dynamic", "type": "null"}}}`},
		{name: "pointer in an embedded resource", ref: "n.json#/properties/x", want: `{"type": "integer"}`},
		{name: "anchor", ref: "#named", want: `{"$anchor": "named", "type": "boolean"}`, sameResource: true},
		{name: "percent-encoded anchor", ref: "#%6Eamed", want: `{"$anchor": "named", "type": "boolean"}`, sameResource: true},
		{name: "root $id with an anchor", ref: "root.json#named", want: `{"$anchor": "named", "type": "boolean"}`, sameResource: true},
		{name: "anchor in an embedded resource", ref: "n.json#inner", want: `{"$anchor": "inner", "type": "number"}`},
		{name: "dynamic anchor in an embedded resource", ref: "n.json#dynamic", want: `{"$dynamicAnchor": "dynamic", "type": "null"}`},
		{name: "anchor of another resource", ref: "#inner"},
		{name: "unknown anchor", ref: "#missing"},
		{name: "duplicated anchor", ref: "#dup"},
		{name: "duplicated resource", ref: "dup.json"},
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
			// anchor of its resource becomes a JSON Pointer into that
			// resource. A reference to an anchor its resource does not
			// declare, or declares twice, stays.
			name: "references to anchors become pointers",
			schema: `{
				"$anchor": "root",
				"properties": {
					"def": {"$ref": "#A"},
					"percent-encoded": {"$ref": "#%41"},
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
					"percent-encoded": {"$ref": "#/$defs/A"},
					"draft 7 beside $anchor": {"$ref": "#/$defs/Old"},
					"list item": {"$ref": "#/$defs/List/allOf/1"},
					"escaped": {"$ref": "#/$defs/S~1T"},
					"root": {"$ref": "#"},
					"duplicate": {"$ref": "#dup"},
					"missing": {"$ref": "#missing"},
					"other resource": {"$ref": "#N"},
					"nested": {"$id": "nested.json", "$anchor": "N", "properties": {"self": {"$ref": "#"}, "up": {"$ref": "#A"}}}
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
		{
			// A dynamic reference resolves like a reference when it is a
			// JSON Pointer or no other schema declares its dynamic anchor,
			// and root folding follows only references.
			name: "dynamic references to anchors declared once or by pointer become pointers",
			schema: `{
				"properties": {
					"tree": {"$dynamicRef": "#node"},
					"plain": {"$dynamicRef": "#leaf"},
					"extended": {"$dynamicRef": "#meta"},
					"pointer": {"$dynamicRef": "#/$defs/Meta"},
					"nested": {"$id": "nested.json", "$defs": {"M": {"$dynamicAnchor": "meta"}}}
				},
				"$defs": {
					"Node": {"$dynamicAnchor": "node", "items": {"$dynamicRef": "#node"}},
					"Leaf": {"$anchor": "leaf", "type": "string"},
					"Meta": {"$dynamicAnchor": "meta", "type": "object"}
				}
			}`,
			want: `{
				"properties": {
					"tree": {"$ref": "#/$defs/Node"},
					"plain": {"$ref": "#/$defs/Leaf"},
					"extended": {"$dynamicRef": "#meta"},
					"pointer": {"$ref": "#/$defs/Meta"},
					"nested": {"$id": "nested.json", "$defs": {"M": {"$dynamicAnchor": "meta"}}}
				},
				"$defs": {
					"Node": {"$dynamicAnchor": "node", "items": {"$ref": "#/$defs/Node"}},
					"Leaf": {"$anchor": "leaf", "type": "string"},
					"Meta": {"$dynamicAnchor": "meta", "type": "object"}
				}
			}`,
		},
		{
			// Both references apply, so one the schema gains beside its
			// "$ref" joins "allOf".
			name: "dynamic references beside references join allOf",
			schema: `{
				"properties": {
					"pointer": {"$ref": "#/$defs/A", "$dynamicRef": "#/$defs/B"},
					"anchor": {"$ref": "#/$defs/A", "$dynamicRef": "#leaf"},
					"both": {"$ref": "#leaf", "$dynamicRef": "#/$defs/B", "allOf": [{"required": ["x"]}]}
				},
				"$defs": {"A": {"minLength": 1}, "B": {"maxLength": 3}, "Leaf": {"$anchor": "leaf", "type": "string"}}
			}`,
			want: `{
				"properties": {
					"pointer": {"$ref": "#/$defs/A", "allOf": [{"$ref": "#/$defs/B"}]},
					"anchor": {"$ref": "#/$defs/A", "allOf": [{"$ref": "#/$defs/Leaf"}]},
					"both": {"$ref": "#/$defs/Leaf", "allOf": [{"required": ["x"]}, {"$ref": "#/$defs/B"}]}
				},
				"$defs": {"A": {"minLength": 1}, "B": {"maxLength": 3}, "Leaf": {"$anchor": "leaf", "type": "string"}}
			}`,
		},
		{
			// Drafts 6 and 7 allow colons in the plain name of a "$id"
			// fragment, which "$anchor" does not, so references to it
			// become pointers.
			name: "legacy anchors with colons become pointers",
			schema: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": {
					"filter": {"$ref": "#filter:string"},
					"encoded": {"$ref": "#filter%3Astring"},
					"plain": {"$ref": "#filter:number"},
					"escaped": {"$ref": "#filter%3Anumber"}
				},
				"definitions": {"F": {"$id": "#filter:string", "type": "string"}, "N": {"$id": "#filter%3Anumber", "type": "number"}}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": {
					"filter": {"$ref": "#/definitions/F"},
					"encoded": {"$ref": "#/definitions/F"},
					"plain": {"$ref": "#/definitions/N"},
					"escaped": {"$ref": "#/definitions/N"}
				},
				"definitions": {"F": {"type": "string"}, "N": {"type": "number"}}
			}`,
		},
		{
			// Draft 4 names resources and anchors with "id" and spells
			// exclusive bounds as booleans. "id" as a property name or in
			// instance data stays.
			name: "draft 4 is rewritten in draft 7",
			schema: `{
				"$schema": "http://json-schema.org/draft-04/schema#",
				"id": "https://example.com/tool.json",
				"properties": {
					"id": {"type": "string", "default": {"id": "kept"}},
					"low": {"type": "number", "minimum": 1, "exclusiveMinimum": true},
					"high": {"type": "number", "maximum": 9, "exclusiveMaximum": false},
					"args": {"$ref": "#args"},
					"kind": {"enum": [{"kind": "x"}]}
				},
				"definitions": {"Args": {"id": "#args", "type": "object"}}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id": "https://example.com/tool.json",
				"properties": {
					"id": {"type": "string", "default": {"id": "kept"}},
					"low": {"type": "number", "exclusiveMinimum": 1},
					"high": {"type": "number", "maximum": 9},
					"args": {"$ref": "#/definitions/Args"},
					"kind": {"anyOf": [{"const": {"kind": "x"}}]}
				},
				"definitions": {"Args": {"$anchor": "args", "type": "object"}}
			}`,
		},
		{
			// A draft 4 "id" fragment in an embedded resource becomes an
			// "$anchor", which draft 7 does not read, so references to
			// it, from inside the resource or by its URI, become pointers
			// into that resource. A "$dynamicAnchor" names a fragment
			// as well.
			name: "references to anchors of embedded resources become pointers",
			schema: `{
				"$schema": "http://json-schema.org/draft-04/schema#",
				"id": "https://example.com/tool.json",
				"properties": {
					"args": {"$ref": "child.json#args"},
					"meta": {"$ref": "https://example.com/child.json#meta"},
					"child": {
						"id": "child.json",
						"properties": {"self": {"$ref": "#args"}, "up": {"$ref": "tool.json#/properties/args"}},
						"definitions": {"Args": {"id": "#args", "type": "object"}, "Meta": {"$dynamicAnchor": "meta", "type": "string"}}
					}
				}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id": "https://example.com/tool.json",
				"properties": {
					"args": {"$ref": "child.json#/definitions/Args"},
					"meta": {"$ref": "https://example.com/child.json#/definitions/Meta"},
					"child": {
						"$id": "child.json",
						"properties": {"self": {"$ref": "#/definitions/Args"}, "up": {"$ref": "tool.json#/properties/args"}},
						"definitions": {"Args": {"$anchor": "args", "type": "object"}, "Meta": {"$dynamicAnchor": "meta", "type": "string"}}
					}
				}
			}`,
		},
		{
			// A dynamic reference resolves against the base URI first, so
			// it names the root resource with a fragment as a reference
			// does.
			name: "dynamic references to the root resource become fragments",
			schema: `{
				"$id": "https://example.com/tool.json",
				"properties": {
					"pointer": {"$dynamicRef": "https://example.com/tool.json#/$defs/Q"},
					"anchor": {"$dynamicRef": "tool.json#node"}
				},
				"$defs": {"Q": {"type": "string"}, "A": {"$dynamicAnchor": "node"}, "B": {"$id": "b.json", "$dynamicAnchor": "node"}}
			}`,
			want: `{
				"$id": "https://example.com/tool.json",
				"properties": {
					"pointer": {"$ref": "#/$defs/Q"},
					"anchor": {"$dynamicRef": "#node"}
				},
				"$defs": {"Q": {"type": "string"}, "A": {"$dynamicAnchor": "node"}, "B": {"$id": "b.json", "$dynamicAnchor": "node"}}
			}`,
		},
		{
			// Some providers check a schema against the dialect only when
			// "$schema" spells it as published. URIs compare their scheme
			// and host without case, and an empty fragment is no fragment.
			// Providers reject a "$schema" that is not spelled as
			// published. One under a legacy draft is read as that draft,
			// and one for another dialect goes, which reads as 2020-12.
			// Draft 3 is rewritten in draft 7.
			name: "published dialects are spelled as published",
			schema: `{
				"$schema": "HTTP://JSON-SCHEMA.ORG/draft-07/schema#",
				"properties": {
					"secure": {"$id": "a.json", "$schema": "https://json-schema.org/draft-07/schema"},
					"insecure": {"$id": "b.json", "$schema": "http://Json-Schema.org/draft/2020-12/schema#"},
					"path case": {"$id": "c.json", "$schema": "https://json-schema.org/DRAFT/2020-12/schema"},
					"other": {"$id": "d.json", "$schema": "https://example.com/meta"},
					"legacy": {"$id": "e.json", "$schema": "http://json-schema.org/draft-06/hyper-schema#"},
					"draft 3": {"$id": "f.json", "$schema": "http://json-schema.org/draft-03/schema#"}
				}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": {
					"secure": {"$id": "a.json", "$schema": "http://json-schema.org/draft-07/schema#"},
					"insecure": {"$id": "b.json", "$schema": "https://json-schema.org/draft/2020-12/schema"},
					"path case": {"$id": "c.json"},
					"other": {"$id": "d.json"},
					"legacy": {"$id": "e.json", "$schema": "http://json-schema.org/draft-07/schema#"},
					"draft 3": {"$id": "f.json", "$schema": "http://json-schema.org/draft-07/schema#"}
				}
			}`,
		},
		{
			// A draft 2019-09 recursive reference becomes a reference,
			// and "$recursiveAnchor", which providers reject, goes.
			name:   "recursive references are replaced",
			schema: `{"$schema": "https://json-schema.org/draft/2019-09/schema", "$recursiveAnchor": true, "properties": {"child": {"$recursiveRef": "#"}}}`,
			want:   `{"$schema": "https://json-schema.org/draft/2019-09/schema", "properties": {"child": {"$ref": "#"}}}`,
		},
		{
			// Draft 4 ignores the keywords later drafts define, so they
			// go. The one occurrence a reference's pointer passes through
			// keeps its value under a key that no draft defines, and a
			// property named like a keyword is only a name.
			name: "draft 4 drops keywords it ignores",
			schema: `{
				"$schema": "http://json-schema.org/draft-04/schema#",
				"properties": {
					"mode": {"type": "string", "const": "fixed", "if": {"minLength": 2}, "then": {"maxLength": 3}},
					"tags": {"type": "array", "contains": {"type": "string"}},
					"meta": {"type": "object", "propertyNames": {"maxLength": 3}, "dependentRequired": {"a": ["b"]}},
					"other": {"$ref": "#/definitions/Holder/const"},
					"const": {"type": "integer", "const": 7},
					"same": {"$ref": "#/properties/const"}
				},
				"definitions": {"Holder": {"const": {"type": "integer"}, "x-draft4-const": "taken"}}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": {
					"mode": {"type": "string"},
					"tags": {"type": "array", "items": {}},
					"meta": {"type": "object"},
					"other": {"$ref": "#/definitions/Holder/x-draft4-const-2"},
					"const": {"type": "integer"},
					"same": {"$ref": "#/properties/const"}
				},
				"definitions": {"Holder": {"x-draft4-const-2": {"type": "integer"}, "x-draft4-const": "taken"}}
			}`,
		},
		{
			// A pointer starts at the resource that the reference names
			// or is in.
			name: "draft 4 keeps keywords that pointers into embedded resources pass through",
			schema: `{
				"$schema": "http://json-schema.org/draft-04/schema#",
				"properties": {
					"tree": {"id": "tree.json", "properties": {"a": {"$ref": "#/definitions/T/if"}}, "definitions": {"T": {"if": {"type": "integer"}}}},
					"b": {"$ref": "tree.json#/definitions/T/if"}
				}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": {
					"tree": {"$id": "tree.json", "properties": {"a": {"$ref": "#/definitions/T/x-draft4-if"}}, "definitions": {"T": {"x-draft4-if": {"type": "integer"}}}},
					"b": {"$ref": "tree.json#/definitions/T/x-draft4-if"}
				}
			}`,
		},
		{
			// A resource read in another dialect keeps the keywords that
			// dialect applies, and so does a pointer into it, while a
			// draft 4 resource inside it is rewritten.
			name: "draft 4 rewriting skips resources in other dialects",
			schema: `{
				"$schema": "http://json-schema.org/draft-04/schema#",
				"properties": {
					"mode": {"type": "string", "const": "x"},
					"child": {
						"$id": "child.json",
						"$schema": "https://json-schema.org/draft/2020-12/schema",
						"unevaluatedProperties": false,
						"properties": {
							"k": {"const": 1},
							"legacy": {"$schema": "http://json-schema.org/draft-04/schema#", "id": "legacy.json", "type": "integer", "const": 2}
						}
					},
					"pin": {"$ref": "child.json#/properties/k/const"}
				}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": {
					"mode": {"type": "string"},
					"child": {
						"$id": "child.json",
						"$schema": "https://json-schema.org/draft/2020-12/schema",
						"unevaluatedProperties": false,
						"properties": {
							"k": {"const": 1},
							"legacy": {"$schema": "http://json-schema.org/draft-07/schema#", "$id": "legacy.json", "type": "integer"}
						}
					},
					"pin": {"$ref": "child.json#/properties/k/const"}
				}
			}`,
		},
		{
			// A draft 4 resource is rewritten in a root of another
			// dialect, and so is a schema under any draft 4 URI.
			name: "draft 4 resources and hyper-schemas are rewritten",
			schema: `{
				"properties": {
					"k": {"const": 1},
					"legacy": {"$schema": "http://json-schema.org/draft-04/schema#", "id": "legacy.json", "minimum": 0, "exclusiveMinimum": true, "const": 2},
					"hyper": {"$schema": "http://json-schema.org/draft-04/hyper-schema#", "id": "hyper.json", "maximum": 9, "exclusiveMaximum": true}
				}
			}`,
			want: `{
				"properties": {
					"k": {"const": 1},
					"legacy": {"$schema": "http://json-schema.org/draft-07/schema#", "$id": "legacy.json", "exclusiveMinimum": 0},
					"hyper": {"$schema": "http://json-schema.org/draft-07/schema#", "$id": "hyper.json", "exclusiveMaximum": 9}
				}
			}`,
		},
		{
			// Providers reject the draft 3 forms of keywords whose
			// meaning draft 4 changed. A "required": true moves to the
			// list of the schema holding "properties", after the names of
			// a list that the schema already has, and the schemas draft 3
			// holds under "extends", "type" and "disallow" are rewritten
			// too. Keywords draft 4 added go, since draft 3 ignores them,
			// and "type" "any" allows every value.
			name: "draft 3 is rewritten in draft 7",
			schema: `{
				"$schema": "http://json-schema.org/draft-03/schema#",
				"type": "object",
				"required": ["extra"],
				"properties": {
					"name": {"type": "string", "required": true},
					"count": {"type": "integer", "divisibleBy": 5, "minimum": 0, "exclusiveMinimum": true, "required": false},
					"child": {"type": "object", "required": true, "properties": {"leaf": {"type": "any", "required": true}}},
					"either": {"type": ["string", {"type": "object", "properties": {"x": {"type": "string", "required": true}}}]},
					"mode": {"type": "string", "disallow": ["null", {"enum": ["off"]}], "extends": {"maxLength": 9}},
					"link": {"extends": [{"minLength": 1}, {"$ref": "#/definitions/Short"}], "dependencies": {"a": "b", "c": ["d"]}},
					"plain": {"type": ["string", "null"], "disallow": "integer"},
					"loose": {"type": ["string", "any"], "anyOf": [{"type": "integer"}], "const": 3}
				},
				"definitions": {"Short": {"maxLength": 3}}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"required": ["extra", "child", "name"],
				"properties": {
					"name": {"type": "string"},
					"count": {"type": "integer", "multipleOf": 5, "exclusiveMinimum": 0},
					"child": {"type": "object", "required": ["leaf"], "properties": {"leaf": {}}},
					"either": {"anyOf": [{"type": "string"}, {"type": "object", "required": ["x"], "properties": {"x": {"type": "string"}}}]},
					"mode": {"type": "string", "not": {"anyOf": [{"type": "null"}, {"enum": ["off"]}]}, "allOf": [{"maxLength": 9}]},
					"link": {"allOf": [{"minLength": 1}, {"$ref": "#/definitions/Short"}], "dependencies": {"a": ["b"], "c": ["d"]}},
					"plain": {"anyOf": [{"type": "string"}, {"type": "null"}], "not": {"type": "integer"}},
					"loose": {}
				},
				"definitions": {"Short": {"maxLength": 3}}
			}`,
		},
		{
			// A pointer to a schema that draft 3 holds under "extends",
			// "type" or "disallow" follows it to its draft 7 keyword.
			name: "draft 3 references follow the schemas it moves",
			schema: `{
				"$schema": "http://json-schema.org/draft-03/schema#",
				"properties": {
					"a": {"$ref": "#/definitions/X/extends"},
					"b": {"$ref": "#/definitions/X/extends/properties/n/type/0"},
					"c": {"$ref": "#/definitions/Y/type/1"},
					"d": {"$ref": "#/definitions/Z/disallow/1"},
					"e": {"$ref": "#/definitions/V/disallow/0"},
					"f": {"$ref": "#/definitions/W/disallow"},
					"g": {"$ref": "#/definitions/L/extends/1"}
				},
				"definitions": {
					"X": {"extends": {"properties": {"n": {"type": [{"type": "string"}, "null"]}}}},
					"Y": {"type": ["null", {"minLength": 2}]},
					"Z": {"disallow": ["null", {"maxLength": 1}]},
					"V": {"disallow": [{"maxLength": 1}]},
					"W": {"disallow": {"enum": ["off"]}},
					"L": {"extends": [{"minLength": 1}, {"maxLength": 5}]}
				}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": {
					"a": {"$ref": "#/definitions/X/allOf/0"},
					"b": {"$ref": "#/definitions/X/allOf/0/properties/n/anyOf/0"},
					"c": {"$ref": "#/definitions/Y/anyOf/1"},
					"d": {"$ref": "#/definitions/Z/not/anyOf/1"},
					"e": {"$ref": "#/definitions/V/not"},
					"f": {"$ref": "#/definitions/W/not"},
					"g": {"$ref": "#/definitions/L/allOf/1"}
				},
				"definitions": {
					"X": {"allOf": [{"properties": {"n": {"anyOf": [{"type": "string"}, {"type": "null"}]}}}]},
					"Y": {"anyOf": [{"type": "null"}, {"minLength": 2}]},
					"Z": {"not": {"anyOf": [{"type": "null"}, {"maxLength": 1}]}},
					"V": {"not": {"maxLength": 1}},
					"W": {"not": {"enum": ["off"]}},
					"L": {"allOf": [{"minLength": 1}, {"maxLength": 5}]}
				}
			}`,
		},
		{
			// A pointer that passes through a keyword draft 4 added keeps
			// it under a key no draft defines, as in draft 4, and a draft 3
			// hyper-schema is draft 3.
			name: "draft 3 keeps keywords that pointers pass through",
			schema: `{
				"$schema": "http://json-schema.org/draft-03/hyper-schema#",
				"properties": {
					"a": {"allOf": [{"type": "string"}], "minProperties": 1, "required": true},
					"b": {"$ref": "#/definitions/D/oneOf/0"}
				},
				"definitions": {"D": {"oneOf": [{"type": "integer"}]}}
			}`,
			want: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"required": ["a"],
				"properties": {
					"a": {},
					"b": {"$ref": "#/definitions/D/x-draft3-oneOf/0"}
				},
				"definitions": {"D": {"x-draft3-oneOf": [{"type": "integer"}]}}
			}`,
		},
		{
			// A draft 3 resource is rewritten in a root of another
			// dialect, while a resource of another dialect inside it keeps
			// its keywords.
			name: "draft 3 rewriting skips resources in other dialects",
			schema: `{
				"properties": {
					"modern": {"required": ["y"], "anyOf": [{"type": "string"}]},
					"legacy": {
						"$schema": "http://json-schema.org/draft-03/schema#",
						"id": "legacy.json",
						"properties": {
							"x": {"type": "integer", "required": true},
							"inner": {"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "inner.json", "required": ["z"], "anyOf": [{"type": "object"}]}
						}
					}
				}
			}`,
			want: `{
				"properties": {
					"modern": {"required": ["y"], "anyOf": [{"type": "string"}]},
					"legacy": {
						"$schema": "http://json-schema.org/draft-07/schema#",
						"$id": "legacy.json",
						"required": ["x"],
						"properties": {
							"x": {"type": "integer"},
							"inner": {"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "inner.json", "required": ["z"], "anyOf": [{"type": "object"}]}
						}
					}
				}
			}`,
		},
		{
			// URIs compare their scheme and host without case.
			name:   "an uppercase draft 4 URI is draft 4",
			schema: `{"$schema": "HTTP://JSON-SCHEMA.ORG/draft-04/schema#", "properties": {"low": {"type": "number", "minimum": 1, "exclusiveMinimum": true}}}`,
			want:   `{"$schema": "http://json-schema.org/draft-07/schema#", "properties": {"low": {"type": "number", "exclusiveMinimum": 1}}}`,
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

// TestNormalizeBoundsAnchorPointers verifies that references to deeply
// nested anchors become JSON Pointers only while the pointers fit a
// budget, since a pointer for every reference would grow the schema
// quadratically, and that every other reference keeps naming its anchor.
func TestNormalizeBoundsAnchorPointers(t *testing.T) {
	t.Parallel()

	const depth = 1000
	var b strings.Builder
	for level := range depth {
		_, _ = fmt.Fprintf(&b, `{"$anchor": "a%d", "properties": {"r": {"$ref": "#a%d"}, "a": `, level, max(level-1, 0))
	}
	_, _ = b.WriteString(`{}`)
	_, _ = b.WriteString(strings.Repeat(`}}`, depth))
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(b.String()), &schema))

	normalized := toolschema.Normalize(schema)
	pointers, anchors, pointerBytes := 0, 0, 0
	node := normalized
	for level := range depth {
		properties, ok := node["properties"].(map[string]any)
		require.True(t, ok)
		r, ok := properties["r"].(map[string]any)
		require.True(t, ok)
		ref, ok := r["$ref"].(string)
		require.True(t, ok)
		target := max(level-1, 0)
		switch ref {
		case fmt.Sprintf("#a%d", target):
			anchors++
		case "#" + strings.Repeat("/properties/a", target):
			pointers++
			pointerBytes += len(ref)
		default:
			require.Failf(t, "unexpected reference", "level %d: %s", level, ref)
		}
		node, ok = properties["a"].(map[string]any)
		require.True(t, ok)
	}
	// Pointers for every reference would take 6.5 MB.
	require.NotZero(t, pointers)
	require.NotZero(t, anchors)
	require.LessOrEqual(t, pointerBytes, 64<<10)

	first, err := json.Marshal(normalized)
	require.NoError(t, err)
	again, err := json.Marshal(toolschema.Normalize(schema))
	require.NoError(t, err)
	require.Equal(t, string(first), string(again), "the same references must become pointers on every run")
}

func TestReplaceRecursiveRefs(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		schema string
		want   string
	}{
		{
			// The root is the outermost resource of every dynamic scope,
			// so a recursive reference in it names it, and one beside a
			// "$ref" joins "allOf".
			name:   "references in the root resource name the root",
			schema: `{"$schema": "https://json-schema.org/draft/2019-09/schema", "$recursiveAnchor": true, "properties": {"a": {"$recursiveRef": "#"}, "b": {"$ref": "#/$defs/K", "$recursiveRef": "#"}}, "$defs": {"K": {"required": ["q"]}}}`,
			want:   `{"$schema": "https://json-schema.org/draft/2019-09/schema", "properties": {"a": {"$ref": "#"}, "b": {"$ref": "#/$defs/K", "allOf": [{"$ref": "#"}]}}, "$defs": {"K": {"required": ["q"]}}}`,
		},
		{
			// Without "$recursiveAnchor" at its root, a resource is the
			// target of its own recursive references.
			name:   "references in an unanchored resource name it",
			schema: `{"$schema": "https://json-schema.org/draft/2019-09/schema", "$recursiveAnchor": true, "properties": {"tree": {"$id": "tree.json", "$recursiveAnchor": false, "items": {"$recursiveRef": "tree.json"}}}}`,
			want:   `{"$schema": "https://json-schema.org/draft/2019-09/schema", "properties": {"tree": {"$id": "tree.json", "items": {"$ref": "#"}}}}`,
		},
		{
			// In a resource whose root declares "$recursiveAnchor", the
			// outermost such resource of the dynamic scope is the target,
			// as for a "$dynamicRef" to a "$dynamicAnchor" each of them
			// declares. The name is one no anchor uses.
			name: "references in an anchored resource become dynamic",
			schema: `{
				"$schema": "https://json-schema.org/draft/2019-09/schema",
				"$recursiveAnchor": true,
				"properties": {
					"a": {"$ref": "#recursive"},
					"tree": {"$id": "tree.json", "$recursiveAnchor": true, "items": {"$recursiveRef": "#", "$dynamicRef": "#/items"}}
				},
				"$defs": {"R": {"$anchor": "recursive"}, "S": {"$id": "#recursive2"}}
			}`,
			want: `{
				"$schema": "https://json-schema.org/draft/2019-09/schema",
				"$dynamicAnchor": "recursive3",
				"properties": {
					"a": {"$ref": "#recursive"},
					"tree": {"$id": "tree.json", "$dynamicAnchor": "recursive3", "items": {"$dynamicRef": "#/items", "allOf": [{"$dynamicRef": "#recursive3"}]}}
				},
				"$defs": {"R": {"$anchor": "recursive"}, "S": {"$id": "#recursive2"}}
			}`,
		},
		{
			// An anchored resource root that declares a dynamic anchor
			// has no room for another, so the reference is left out,
			// which only drops a constraint.
			name:   "a dynamic anchor already declared leaves the reference out",
			schema: `{"$schema": "https://json-schema.org/draft/2019-09/schema", "properties": {"tree": {"$id": "tree.json", "$recursiveAnchor": true, "$dynamicAnchor": "node", "items": {"$recursiveRef": "#"}}, "leaf": {"$recursiveRef": "#"}}}`,
			want:   `{"$schema": "https://json-schema.org/draft/2019-09/schema", "properties": {"tree": {"$id": "tree.json", "$dynamicAnchor": "node", "items": {}}, "leaf": {"$ref": "#"}}}`,
		},
		{
			name:   "references to other targets stay",
			schema: `{"$schema": "https://json-schema.org/draft/2019-09/schema", "properties": {"a": {"$recursiveRef": "#/$defs/A"}, "b": {"$recursiveRef": "other.json"}}}`,
			want:   `{"$schema": "https://json-schema.org/draft/2019-09/schema", "properties": {"a": {"$recursiveRef": "#/$defs/A"}, "b": {"$recursiveRef": "other.json"}}}`,
		},
		{
			// Draft 7 and draft 2020-12 ignore both keywords.
			name:   "other dialects drop the keywords",
			schema: `{"$schema": "https://json-schema.org/draft/2020-12/schema", "$recursiveAnchor": true, "properties": {"a": {"$recursiveRef": "#"}}}`,
			want:   `{"$schema": "https://json-schema.org/draft/2020-12/schema", "properties": {"a": {}}}`,
		},
		{
			// MCP reads a schema without "$schema" as draft 2020-12.
			name:   "a schema without a dialect drops the keywords",
			schema: `{"$recursiveAnchor": true, "properties": {"a": {"$recursiveRef": "#"}}}`,
			want:   `{"properties": {"a": {}}}`,
		},
		{
			name:   "draft 7 drops the keywords",
			schema: `{"$schema": "http://json-schema.org/draft-07/schema#", "properties": {"a": {"$recursiveRef": "#"}}}`,
			want:   `{"$schema": "http://json-schema.org/draft-07/schema#", "properties": {"a": {}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var schema map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.schema), &schema))
			got, err := json.Marshal(toolschema.ReplaceRecursiveRefs(schema))
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
			source, err := json.Marshal(schema)
			require.NoError(t, err)
			require.JSONEq(t, tc.schema, string(source), "the source must not be modified")
		})
	}
}

func TestPointerKeywords(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pointer string
		want    []bool
	}{
		// A name in a map of schemas is only a name, even when it is
		// spelled like a keyword.
		{pointer: "/properties/description/description", want: []bool{true, false, true}},
		// Draft 7 "items" may hold a list of schemas, and instance data
		// holds no keywords.
		{pointer: "/$defs/A/items/0/const/properties", want: []bool{true, false, true, false, true, false}},
		{pointer: "/allOf/1/required/0", want: []bool{true, false, true, false}},
		{pointer: "/x-custom/properties", want: []bool{false, false}},
	} {
		t.Run(tc.pointer, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, toolschema.PointerKeywords(strings.Split(tc.pointer[1:], "/")))
		})
	}
}
