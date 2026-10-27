package toolschema

import (
	"net/url"
	"regexp"
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

// Normalize returns a copy of a schema in the shapes provider drivers
// accept: every type list is rewritten into an equivalent "anyOf", every
// array schema without "items" gets an empty one, an "enum" with an
// array or object member becomes an "anyOf" of "const" values, "$id"
// loses its fragment, which becomes an "$anchor" when it names one, and
// a reference that names the root resource by URI becomes a fragment
// reference. Instance data such as "const" and "enum" values keeps its
// exact shape. The source is not modified.
func Normalize(schema map[string]any) map[string]any {
	return copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		normalizeNode(node, scope)
	})
}

func normalizeNode(node map[string]any, scope refScope) {
	if types, ok := node["type"].([]any); ok {
		variants := make([]any, 0, len(types))
		for _, typ := range types {
			variant := map[string]any{"type": typ}
			if typ == "array" {
				variant["items"] = map[string]any{}
			}
			variants = append(variants, variant)
		}
		delete(node, "type")
		requireAnyOf(node, variants)
	} else if node["type"] == "array" {
		if _, ok := node["items"]; !ok {
			node["items"] = map[string]any{}
		}
	}

	// OpenAI rejects a tool schema whose "enum" has an array or object
	// member, but accepts the same values as "const".
	if values, ok := node["enum"].([]any); ok && slices.ContainsFunc(values, isComposite) {
		variants := make([]any, 0, len(values))
		for _, value := range values {
			variants = append(variants, map[string]any{"const": value})
		}
		delete(node, "enum")
		requireAnyOf(node, variants)
	}

	// Providers validate tool schemas against draft 2020-12, which
	// forbids a fragment in "$id". Earlier drafts used one to name an
	// anchor, which 2020-12 spells "$anchor". The resource is the URI
	// without the fragment, and an "$id" that is only a fragment starts
	// no resource.
	if id, ok := node["$id"].(string); ok {
		if base, fragment, _ := strings.Cut(id, "#"); fragment != "" {
			if _, ok := node["$anchor"]; !ok && anchorName.MatchString(fragment) {
				node["$anchor"] = fragment
			}
			if base == "" {
				delete(node, "$id")
			} else {
				node["$id"] = base
			}
		}
	}

	// Some providers resolve only fragment references.
	if ref, ok := node["$ref"].(string); ok && !scope.nested && !strings.HasPrefix(ref, "#") {
		if fragment, ok := scope.rootFragment(ref); ok {
			if tokens, ok := decodePointer(fragment); ok {
				node["$ref"] = "#" + encodePointer(tokens)
			} else if anchorName.MatchString(fragment) {
				node["$ref"] = "#" + fragment
			}
		}
	}
}

// anchorName matches the plain-name fragments that draft 2020-12 allows
// as "$anchor" values.
var anchorName = regexp.MustCompile(`^[A-Za-z_][-A-Za-z0-9._]*$`)

// requireAnyOf requires one of variants to hold. An existing "anyOf"
// keeps its place, so references into it still resolve, and the
// variants join "allOf" instead.
func requireAnyOf(node map[string]any, variants []any) {
	if _, ok := node["anyOf"]; !ok {
		node["anyOf"] = variants
		return
	}
	allOf, _ := node["allOf"].([]any)
	node["allOf"] = append(slices.Clone(allOf), map[string]any{"anyOf": variants})
}

func isComposite(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
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

// HasRefs reports whether schema or one of its subschemas holds a
// reference keyword.
func HasRefs(schema map[string]any) bool {
	found := false
	copyNode(schema, refScope{}, func(node map[string]any, _ refScope) {
		for _, key := range []string{"$ref", "$dynamicRef", "$recursiveRef"} {
			if _, ok := node[key]; ok {
				found = true
			}
		}
	})
	return found
}

// ResolveLocalRef returns the value in root that a reference made from
// root's own resource points at. The reference may be a fragment such as
// "#/$defs/Filter", name root's "$id", or name the "$id" of a resource
// that root embeds. sameResource reports whether the value belongs to
// root's resource, so that references inside it resolve as they would
// at the root.
func ResolveLocalRef(root map[string]any, ref string) (target any, sameResource bool, ok bool) {
	scope := rootScope(root)
	prefix, fragment, _ := strings.Cut(ref, "#")
	tokens, ok := decodePointer(fragment)
	if !ok {
		return nil, false, false
	}
	current, sameResource := any(root), true
	if prefix != "" {
		uri, ok := scope.resolve(prefix)
		if !ok {
			return nil, false, false
		}
		if scope.rootBase == nil || canonicalURI(uri) != canonicalURI(scope.rootBase) {
			current, ok = embeddedResource(root, scope, uri)
			if !ok {
				return nil, false, false
			}
			sameResource = false
		}
	}
	for _, token := range tokens {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[token]
			if !ok {
				return nil, false, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false, false
			}
			current = typed[index]
		default:
			return nil, false, false
		}
		if node, ok := current.(map[string]any); ok {
			if _, ok := resourceID(node); ok {
				sameResource = false
			}
		}
	}
	return current, sameResource, true
}

// embeddedResource returns the subschema of root that declares the
// resource uri with its own "$id".
func embeddedResource(root map[string]any, scope refScope, uri *url.URL) (map[string]any, bool) {
	want := canonicalURI(uri)
	var found map[string]any
	copyNode(root, scope, func(node map[string]any, scope refScope) {
		if _, ok := resourceID(node); ok && scope.nested && scope.base != nil &&
			found == nil && canonicalURI(scope.base) == want {
			found = node
		}
	})
	return found, found != nil
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

// rootScope returns the scope of root. The URI root was retrieved from
// is unknown, so a relative root "$id" resolves against an empty base,
// the same way as the references compared with it.
func rootScope(root map[string]any) refScope {
	scope := refScope{base: &url.URL{}}
	if id, ok := resourceID(root); ok {
		if id == nil {
			scope.base = nil
		} else {
			scope.base = scope.base.ResolveReference(id)
			scope.rootBase = scope.base
		}
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
	case id.IsAbs():
		s.base = (&url.URL{}).ResolveReference(id)
	}
	return s
}

// resolve returns the absolute form of a reference's URI part, or false
// when it is relative to an unknown base.
func (s refScope) resolve(prefix string) (*url.URL, bool) {
	target, err := parseURI(prefix)
	if err != nil {
		return nil, false
	}
	switch {
	case s.base != nil:
		return s.base.ResolveReference(target), true
	case target.IsAbs():
		return (&url.URL{}).ResolveReference(target), true
	default:
		return nil, false
	}
}

// rootPointer returns the decoded JSON Pointer tokens of ref when its
// target lies in the root resource.
func (s refScope) rootPointer(ref string) ([]string, bool) {
	fragment, ok := s.rootFragment(ref)
	if !ok {
		return nil, false
	}
	return decodePointer(fragment)
}

// rootFragment returns the fragment of ref when its target lies in the
// root resource.
func (s refScope) rootFragment(ref string) (string, bool) {
	prefix, fragment, _ := strings.Cut(ref, "#")
	if prefix == "" {
		return fragment, !s.nested
	}
	if s.rootBase == nil {
		return "", false
	}
	target, ok := s.resolve(prefix)
	if !ok || canonicalURI(target) != canonicalURI(s.rootBase) {
		return "", false
	}
	return fragment, true
}

// resourceID reports whether node declares a schema resource with "$id"
// and returns its URI without a fragment, or nil when it does not parse.
// A fragment-only "$id" is a draft 7 anchor and starts no resource.
func resourceID(node map[string]any) (*url.URL, bool) {
	id, ok := node["$id"].(string)
	if !ok || id == "" || strings.HasPrefix(id, "#") {
		return nil, false
	}
	parsed, err := parseURI(id)
	if err != nil {
		return nil, true
	}
	parsed.Fragment, parsed.RawFragment = "", ""
	return parsed, true
}

// parseURI parses a URI reference after the percent-encoding
// normalization of RFC 3986 section 6.2.2.2: escaped unreserved
// characters are decoded and other escapes use uppercase hex digits.
// Escaped dot segments then resolve like plain ones.
func parseURI(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "%") {
		return url.Parse(raw)
	}
	normalized := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '%' && i+2 < len(raw) {
			if octet, err := strconv.ParseUint(raw[i+1:i+3], 16, 8); err == nil {
				if isUnreserved(byte(octet)) {
					normalized = append(normalized, byte(octet))
				} else {
					normalized = append(normalized, '%')
					normalized = append(normalized, strings.ToUpper(raw[i+1:i+3])...)
				}
				i += 2
				continue
			}
		}
		normalized = append(normalized, raw[i])
	}
	return url.Parse(string(normalized))
}

func isUnreserved(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' ||
		c == '-' || c == '.' || c == '_' || c == '~'
}

// canonicalURI returns a resolved URI in the form used to compare
// resource URIs: the scheme and host are lowercase, and for http and
// https the default port is dropped and an empty path is "/". Parsing
// with parseURI and resolving have already normalized percent-encoding
// and dot segments.
func canonicalURI(uri *url.URL) string {
	canonical := *uri
	canonical.Host = strings.ToLower(canonical.Host)
	if defaultPort, ok := httpDefaultPorts[canonical.Scheme]; ok {
		port := canonical.Port()
		host := strings.TrimSuffix(strings.TrimSuffix(canonical.Host, port), ":")
		if number, err := strconv.Atoi(port); err == nil {
			port = strconv.Itoa(number)
		}
		if port != "" && port != defaultPort {
			host += ":" + port
		}
		canonical.Host = host
		if canonical.Host != "" && canonical.Path == "" {
			canonical.Path = "/"
		}
	}
	return canonical.String()
}

var httpDefaultPorts = map[string]string{"http": "80", "https": "443"}

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
