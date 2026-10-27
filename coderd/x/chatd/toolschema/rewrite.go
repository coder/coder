package toolschema

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Keywords whose values are subschemas. The walkers below visit only
// these positions, so instance data such as "const", "enum", "default",
// and "examples" values, and annotations, are never mistaken for schemas.
var (
	subschemaKeywords = map[string]struct{}{
		"additionalItems":       {},
		"additionalProperties":  {},
		"contains":              {},
		"contentSchema":         {},
		"else":                  {},
		"if":                    {},
		"not":                   {},
		"propertyNames":         {},
		"then":                  {},
		"unevaluatedItems":      {},
		"unevaluatedProperties": {},
	}
	subschemaListKeywords = map[string]struct{}{
		"allOf":       {},
		"anyOf":       {},
		"oneOf":       {},
		"prefixItems": {},
	}
	subschemaMapKeywords = map[string]struct{}{
		"$defs":             {},
		"definitions":       {},
		"dependentSchemas":  {},
		"patternProperties": {},
		"properties":        {},
	}
)

// Normalize returns a copy of a schema in which every type list is
// rewritten into an equivalent "anyOf" and every array schema without
// "items" gets an empty one, the shapes provider drivers accept. Only
// schema positions are rewritten, so instance data such as "const" and
// "enum" values keeps its exact shape. The source is not modified.
func Normalize(schema map[string]any) map[string]any {
	return copyNode(schema, refScope{}, func(node map[string]any, _ refScope) {
		normalizeNode(node)
	})
}

func normalizeNode(node map[string]any) {
	types, ok := node["type"].([]any)
	if !ok {
		if node["type"] == "array" {
			if _, ok := node["items"]; !ok {
				node["items"] = map[string]any{}
			}
		}
		return
	}
	variants := make([]any, 0, len(types))
	for _, typ := range types {
		variant := map[string]any{"type": typ}
		if typ == "array" {
			variant["items"] = map[string]any{}
		}
		variants = append(variants, variant)
	}
	if existing, ok := node["anyOf"]; ok {
		// Both constraints apply, so keep the original alternatives.
		allOf, _ := node["allOf"].([]any)
		node["allOf"] = append(slices.Clone(allOf), map[string]any{"anyOf": existing})
	}
	delete(node, "type")
	node["anyOf"] = variants
}

// RewriteRefs returns a copy of a schema in which every "$ref" that
// locates its target in the root schema resource by JSON Pointer is
// passed to rewrite as the decoded pointer tokens. When rewrite reports a
// change, the reference is re-encoded with the returned tokens. Anchor
// references and references into other resources, including subschemas
// that declare their own "$id", are kept. The source is not modified.
func RewriteRefs(schema map[string]any, rewrite func(tokens []string) ([]string, bool)) map[string]any {
	return copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		ref, ok := node["$ref"].(string)
		if !ok {
			return
		}
		tokens, ok := scope.rootPointer(ref)
		if !ok {
			return
		}
		tokens, changed := rewrite(tokens)
		if !changed {
			return
		}
		prefix, _, _ := strings.Cut(ref, "#")
		node["$ref"] = prefix + "#" + encodePointer(tokens)
	})
}

// ResolveLocalRef returns the value that a reference made from root's
// own resource points at in root. This covers fragment references such
// as "#/$defs/Filter" and, when root declares "$id", references that
// name that URI, such as "https://example.com/tool.json#/$defs/Filter".
func ResolveLocalRef(root map[string]any, ref string) (any, bool) {
	tokens, ok := rootScope(root).rootPointer(ref)
	if !ok {
		return nil, false
	}
	current := any(root)
	for _, token := range tokens {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[token]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

// visitFunc is called on every copied schema object after its subschemas
// were copied, with the resource scope the object belongs to.
type visitFunc func(node map[string]any, scope refScope)

// copyNode copies node and its subschemas. The values of other keywords
// are shared with the source and must not be modified.
func copyNode(node map[string]any, scope refScope, visit visitFunc) map[string]any {
	out := make(map[string]any, len(node))
	for key, value := range node {
		out[key] = copyKeyword(key, value, scope, visit)
	}
	visit(out, scope)
	return out
}

func copySchema(value any, scope refScope, visit visitFunc) any {
	node, ok := value.(map[string]any)
	if !ok {
		// Boolean schemas, or malformed values that are kept as is.
		return value
	}
	return copyNode(node, scope.enter(node), visit)
}

func copyKeyword(key string, value any, scope refScope, visit visitFunc) any {
	if _, ok := subschemaKeywords[key]; ok {
		return copySchema(value, scope, visit)
	}
	if _, ok := subschemaListKeywords[key]; ok {
		return copySchemaList(value, scope, visit)
	}
	if _, ok := subschemaMapKeywords[key]; ok {
		return copySchemaMap(value, scope, visit)
	}
	switch key {
	case "items":
		// A schema, or a list of schemas before draft 2020-12.
		if _, ok := value.([]any); ok {
			return copySchemaList(value, scope, visit)
		}
		return copySchema(value, scope, visit)
	case "dependencies":
		// Draft 7 maps each name to a schema or to a list of names, and
		// copySchema keeps the lists as is.
		return copySchemaMap(value, scope, visit)
	default:
		return value
	}
}

func copySchemaList(value any, scope refScope, visit visitFunc) any {
	list, ok := value.([]any)
	if !ok {
		return value
	}
	out := make([]any, len(list))
	for i, item := range list {
		out[i] = copySchema(item, scope, visit)
	}
	return out
}

func copySchemaMap(value any, scope refScope, visit visitFunc) any {
	named, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(named))
	for name, item := range named {
		out[name] = copySchema(item, scope, visit)
	}
	return out
}

// refScope tracks the schema resource a subschema belongs to. A "$id"
// starts a new resource, and JSON Schema resolves fragment-only
// references against the resource that contains them.
type refScope struct {
	// rootBase is the root resource's base URI, or nil when the root
	// declares no usable "$id".
	rootBase *url.URL
	// base is the current resource's base URI, or nil when unknown.
	base *url.URL
	// nested reports that a resource other than the root was entered.
	nested bool
}

func rootScope(root map[string]any) refScope {
	var scope refScope
	if id, ok := resourceID(root); ok {
		scope.rootBase = id
		scope.base = id
	}
	return scope
}

func (s refScope) enter(node map[string]any) refScope {
	id, ok := resourceID(node)
	if !ok {
		return s
	}
	s.nested = true
	switch {
	case id == nil:
		s.base = nil
	case s.base != nil:
		s.base = s.base.ResolveReference(id)
	default:
		s.base = id
	}
	return s
}

// rootPointer returns the decoded JSON Pointer tokens of ref when its
// target lies in the root resource.
func (s refScope) rootPointer(ref string) ([]string, bool) {
	prefix, fragment, _ := strings.Cut(ref, "#")
	if prefix == "" {
		if s.nested {
			return nil, false
		}
		return decodePointer(fragment)
	}
	if s.rootBase == nil {
		return nil, false
	}
	target, err := url.Parse(prefix)
	if err != nil {
		return nil, false
	}
	if s.base != nil {
		target = s.base.ResolveReference(target)
	}
	if target.String() != s.rootBase.String() {
		return nil, false
	}
	return decodePointer(fragment)
}

// resourceID reports whether node declares a schema resource with "$id"
// and returns its URI without a fragment, or nil when it does not parse.
// A fragment-only "$id" is a draft 7 anchor and starts no resource.
func resourceID(node map[string]any) (*url.URL, bool) {
	id, ok := node["$id"].(string)
	if !ok || id == "" || strings.HasPrefix(id, "#") {
		return nil, false
	}
	parsed, err := url.Parse(id)
	if err != nil {
		return nil, true
	}
	parsed.Fragment, parsed.RawFragment = "", ""
	return parsed, true
}

// decodePointer splits a URI fragment holding a JSON Pointer into its
// unescaped tokens. It rejects anchors and malformed escapes.
func decodePointer(fragment string) ([]string, bool) {
	pointer, err := url.PathUnescape(fragment)
	if err != nil {
		return nil, false
	}
	if pointer == "" {
		return []string{}, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	tokens := strings.Split(pointer[1:], "/")
	for i, token := range tokens {
		tokens[i] = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
	}
	return tokens, true
}

func encodePointer(tokens []string) string {
	pointer := ""
	for _, token := range tokens {
		pointer += "/" + strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	}
	return (&url.URL{Fragment: pointer}).EscapedFragment()
}
