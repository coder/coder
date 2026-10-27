package toolschema

import (
	"maps"
	"net/url"
	"reflect"
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

// keywords lists the JSON Schema keywords of drafts 4 to 2020-12, and the
// OpenAPI "nullable" that some servers send.
var keywords = [...]string{
	"$anchor", "$comment", "$defs", "$dynamicAnchor", "$dynamicRef", "$id",
	"$recursiveAnchor", "$recursiveRef", "$ref", "$schema", "$vocabulary",
	"additionalItems", "additionalProperties", "allOf", "anyOf", "const",
	"contains", "contentEncoding", "contentMediaType", "contentSchema",
	"default", "definitions", "dependencies", "dependentRequired",
	"dependentSchemas", "deprecated", "description", "else", "enum",
	"examples", "exclusiveMaximum", "exclusiveMinimum", "format", "id", "if",
	"items", "maxContains", "maximum", "maxItems", "maxLength",
	"maxProperties", "minContains", "minimum", "minItems", "minLength",
	"minProperties", "multipleOf", "not", "nullable", "oneOf", "pattern",
	"patternProperties", "prefixItems", "properties", "propertyNames",
	"readOnly", "required", "then", "title", "type", "unevaluatedItems",
	"unevaluatedProperties", "uniqueItems", "writeOnly",
}

// InKeyword reports whether value is part of a JSON Schema keyword, so
// that replacing it in a schema's keys could rename a keyword.
func InKeyword(value string) bool {
	return slices.ContainsFunc(keywords[:], func(keyword string) bool {
		return strings.Contains(keyword, value)
	})
}

// Normalize returns a copy of a schema in the shapes provider drivers
// accept: a draft 4 schema is rewritten in draft 7 (see UpgradeDraft4),
// every type list is rewritten into an equivalent "anyOf", every
// array schema without "items" gets an empty one, an "enum" with an
// array or object member becomes an "anyOf" of "const" values, "$id"
// loses its fragment, which becomes an "$anchor" when it names one, a
// reference that names the root resource by URI becomes a fragment
// reference, and a reference to an anchor that the root resource declares
// once becomes a JSON Pointer reference. So does a "$dynamicRef" to such
// an anchor that no other schema declares as a dynamic anchor, since it
// then resolves like "$ref", and a "$dynamicRef" to the root resource by
// JSON Pointer becomes a "$ref". Instance data such as "const" and "enum"
// values keeps its exact shape. The source is not modified.
func Normalize(schema map[string]any) map[string]any {
	schema = UpgradeDraft4(schema)
	// Pointers are encoded only for the anchors references name.
	pointers := map[string]*pathToken{}
	duplicated := map[string]bool{}
	dynamicAnchors := map[string]int{}
	normalized := copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		// Read before normalizing, which drops a "$id" anchor when the
		// node also declares "$anchor".
		var anchors []string
		if !scope.nested {
			anchors = declaredAnchors(node)
		}
		if name, ok := node["$dynamicAnchor"].(string); ok {
			dynamicAnchors[name]++
		}
		normalizeNode(node, scope)
		for _, anchor := range anchors {
			if _, seen := pointers[anchor]; seen {
				duplicated[anchor] = true
			}
			pointers[anchor] = scope.path
		}
	})
	// Some providers ignore anchors, and validators of drafts before
	// 2019-09 do not read "$anchor", but all of them follow a JSON
	// Pointer. Normalizing kept every subschema at its source path.
	refs := map[string][]anchorRef{}
	addRef := func(node map[string]any, key string, scope refScope) {
		ref, _ := node[key].(string)
		fragment, ok := scope.rootFragment(ref)
		if !ok {
			return
		}
		anchor, ok := anchorFragment(fragment)
		if _, declared := pointers[anchor]; !ok || !declared || duplicated[anchor] {
			return
		}
		prefix, _, _ := strings.Cut(ref, "#")
		refs[anchor] = append(refs[anchor], anchorRef{node: node, key: key, prefix: prefix})
	}
	normalized = copyNode(normalized, rootScope(normalized), func(node map[string]any, scope refScope) {
		if _, ok := node["$ref"].(string); ok {
			addRef(node, "$ref", scope)
			return
		}
		// Root folding follows only "$ref" into the keywords it removes.
		ref, ok := node["$dynamicRef"].(string)
		if !ok {
			return
		}
		if _, ok := scope.rootPointer(ref); ok {
			node["$ref"] = ref
			delete(node, "$dynamicRef")
			return
		}
		_, fragment, _ := strings.Cut(ref, "#")
		if anchor, ok := anchorFragment(fragment); ok && dynamicAnchors[anchor] > 1 {
			return
		}
		addRef(node, "$dynamicRef", scope)
	})
	// A pointer can be as long as the schema is deep, and as many
	// references as there are subschemas can name such anchors, so
	// pointers for all of them could make the schema quadratically
	// larger. Anchors are converted in name order while their pointers
	// fit a budget, which a schema reaches only when the pointers alone
	// would take more than callers allow a tool schema.
	budget := maxAnchorPointerBytes
	lengths := map[*pathToken]int{nil: 0}
	for _, anchor := range slices.Sorted(maps.Keys(refs)) {
		path := pointers[anchor]
		length := path.encodedLen(lengths) + len("#")
		if len(refs[anchor]) > budget/length {
			continue
		}
		budget -= length * len(refs[anchor])
		pointer := "#" + EncodePointer(path.tokens())
		for _, ref := range refs[anchor] {
			ref.node["$ref"] = ref.prefix + pointer
			if ref.key != "$ref" {
				delete(ref.node, ref.key)
			}
		}
	}
	return normalized
}

// maxAnchorPointerBytes caps the JSON Pointer text that Normalize writes
// in place of anchor references. Callers reject a normalized tool schema
// larger than 64 KiB.
const maxAnchorPointerBytes = 64 << 10

// anchorRef is a reference keyword in a normalized schema that names an
// anchor of the root resource.
type anchorRef struct {
	node map[string]any
	key  string
	// prefix is the reference's URI part before "#", which names the root
	// resource or is empty.
	prefix string
}

// UpgradeDraft4 returns a copy of a schema that declares draft 4,
// rewritten in draft 7, which names resources with "$id" and has
// "const", as the rest of the rewriting and the providers expect. "id"
// becomes "$id", and a boolean "exclusiveMinimum" or "exclusiveMaximum"
// becomes the number that draft 7 expects. Other schemas are returned
// as is.
func UpgradeDraft4(schema map[string]any) map[string]any {
	if !strings.Contains(Dialect(schema), "json-schema.org/draft-04/schema") {
		return schema
	}
	upgraded := copyNode(schema, refScope{}, func(node map[string]any, _ refScope) {
		if id, ok := node["id"].(string); ok {
			node["$id"] = id
			delete(node, "id")
		}
		for exclusive, inclusive := range map[string]string{"exclusiveMinimum": "minimum", "exclusiveMaximum": "maximum"} {
			isExclusive, ok := node[exclusive].(bool)
			if !ok {
				continue
			}
			delete(node, exclusive)
			if bound, ok := node[inclusive]; ok && isExclusive {
				node[exclusive] = bound
				delete(node, inclusive)
			}
		}
	})
	upgraded["$schema"] = "http://json-schema.org/draft-07/schema#"
	return upgraded
}

// Dialect returns the "$schema" URI that schema declares, with the
// scheme and host in lowercase since URIs compare them without case, or
// "" when it declares none.
func Dialect(schema map[string]any) string {
	dialect, _ := schema["$schema"].(string)
	uri, err := parseURI(dialect)
	if err != nil {
		return dialect
	}
	return canonicalURI(uri)
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
				node["$ref"] = "#" + EncodePointer(tokens)
			} else if anchor, ok := anchorFragment(fragment); ok {
				node["$ref"] = "#" + anchor
			}
		}
	}
}

// anchorName matches the plain-name fragments that draft 2020-12 allows
// as "$anchor" values.
var anchorName = regexp.MustCompile(`^[A-Za-z_][-A-Za-z0-9._]*$`)

// anchorFragment returns the anchor name that a URI fragment holds,
// which may be percent-encoded like the rest of the fragment.
func anchorFragment(fragment string) (string, bool) {
	name, err := url.PathUnescape(fragment)
	if err != nil || !anchorName.MatchString(name) {
		return "", false
	}
	return name, true
}

// declaredAnchors returns the plain-name anchors node declares, with
// "$anchor" or "$dynamicAnchor" or, as drafts before 2019-09 did, with a
// "$id" fragment.
func declaredAnchors(node map[string]any) []string {
	var anchors []string
	for _, key := range []string{"$anchor", "$dynamicAnchor"} {
		if anchor, ok := node[key].(string); ok && anchorName.MatchString(anchor) && !slices.Contains(anchors, anchor) {
			anchors = append(anchors, anchor)
		}
	}
	if id, ok := node["$id"].(string); ok {
		if _, fragment, _ := strings.Cut(id, "#"); anchorName.MatchString(fragment) && !slices.Contains(anchors, fragment) {
			anchors = append(anchors, fragment)
		}
	}
	return anchors
}

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

// RewriteRefs returns a copy of a schema in which every "$ref" or
// "$dynamicRef" that locates its target in the root schema resource by
// JSON Pointer is passed to rewrite as the decoded pointer tokens. A
// "$dynamicRef" with a JSON Pointer resolves like "$ref". When rewrite
// reports a change, the reference is re-encoded with the returned
// tokens. A "$recursiveRef" in the root resource, which names the root,
// is passed as no tokens and becomes a "$ref" when rewrite reports a
// change, since it cannot hold a pointer. Anchor references and
// references into other resources, including subschemas that declare
// their own "$id", are kept. The source is not modified.
func RewriteRefs(schema map[string]any, rewrite func(tokens []string) ([]string, bool)) map[string]any {
	return copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		for _, key := range []string{"$ref", "$dynamicRef"} {
			ref, ok := node[key].(string)
			if !ok {
				continue
			}
			tokens, ok := scope.rootPointer(ref)
			if !ok {
				continue
			}
			tokens, changed := rewrite(tokens)
			if !changed {
				continue
			}
			prefix, _, _ := strings.Cut(ref, "#")
			node[key] = prefix + "#" + EncodePointer(tokens)
		}
		ref, ok := node["$recursiveRef"].(string)
		if !ok {
			return
		}
		tokens, ok := scope.rootPointer(ref)
		if !ok || len(tokens) > 0 {
			return
		}
		tokens, changed := rewrite(tokens)
		if !changed {
			return
		}
		pointer := "#" + EncodePointer(tokens)
		delete(node, "$recursiveRef")
		if _, ok := node["$ref"]; ok {
			allOf, _ := node["allOf"].([]any)
			node["allOf"] = append(slices.Clone(allOf), map[string]any{"$ref": pointer})
		} else {
			node["$ref"] = pointer
		}
	})
}

// RefScanner reports whether schemas hold reference keywords. It
// remembers the answer for every schema object it scans, so a subschema
// that is nested in or shared by the schemas asked about is scanned once.
// Schema objects are told apart by address, so they must stay reachable
// while the scanner is in use. The zero value is ready to use.
type RefScanner struct {
	scanned map[uintptr]bool
}

// HasRefs reports whether schema or one of its subschemas holds a
// reference keyword.
func (s *RefScanner) HasRefs(schema map[string]any) bool {
	address := reflect.ValueOf(schema).Pointer()
	if found, ok := s.scanned[address]; ok {
		return found
	}
	found := false
	for _, key := range []string{"$ref", "$dynamicRef", "$recursiveRef"} {
		if _, ok := schema[key]; ok {
			found = true
		}
	}
	for key, value := range schema {
		if found {
			break
		}
		forEachSubschema(key, value, refScope{}, func(node map[string]any, _ refScope) {
			found = found || s.HasRefs(node)
		})
	}
	if s.scanned == nil {
		s.scanned = map[uintptr]bool{}
	}
	s.scanned[address] = found
	return found
}

// ResolveLocalRef returns the value in root that a reference made from
// root's own resource points at, as a Resolver for root does.
func ResolveLocalRef(root map[string]any, ref string) (target any, sameResource bool, ok bool) {
	return NewResolver(root).Resolve(ref)
}

// Resolver resolves references made from a root schema's own resource.
// The first reference that names an embedded resource or an anchor
// indexes them all, so resolving many references walks the root once.
// The root must not change while the resolver is in use.
type Resolver struct {
	root  map[string]any
	scope refScope
	// resources maps the canonical URI of each embedded resource to the
	// subschemas that declare it, or is nil before indexing.
	resources map[string][]embeddedResource
	// anchors maps each resource, by its canonical URI or "" for the
	// root resource, and each "$anchor" in it to the subschemas declaring
	// that anchor.
	anchors map[string]map[string][]map[string]any
}

type embeddedResource struct {
	node map[string]any
	// outer is the path to the resource, or to the outermost embedded
	// resource holding it.
	outer *pathToken
}

// NewResolver returns a Resolver for references made from root's own
// resource.
func NewResolver(root map[string]any) *Resolver {
	return &Resolver{root: root, scope: rootScope(root)}
}

func (r *Resolver) index() {
	if r.resources != nil {
		return
	}
	r.resources = map[string][]embeddedResource{}
	r.anchors = map[string]map[string][]map[string]any{}
	walkNode(r.root, r.scope, func(node map[string]any, scope refScope) {
		resource := ""
		if scope.nested {
			if scope.base == nil {
				// No reference can name this resource.
				return
			}
			resource = canonicalURI(scope.base)
			if _, ok := resourceID(node); ok {
				r.resources[resource] = append(r.resources[resource], embeddedResource{node: node, outer: scope.outer})
			}
		}
		if anchor, ok := node["$anchor"].(string); ok {
			if r.anchors[resource] == nil {
				r.anchors[resource] = map[string][]map[string]any{}
			}
			r.anchors[resource][anchor] = append(r.anchors[resource][anchor], node)
		}
	})
}

// isRoot reports whether uri names the root resource.
func (r *Resolver) isRoot(uri *url.URL) bool {
	return r.scope.rootBase != nil && canonicalURI(uri) == canonicalURI(r.scope.rootBase)
}

// embedded returns the embedded resource that uri names. A resource URI
// that more than one subschema declares names none of them.
func (r *Resolver) embedded(uri *url.URL) (embeddedResource, bool) {
	r.index()
	found := r.resources[canonicalURI(uri)]
	if len(found) != 1 {
		return embeddedResource{}, false
	}
	return found[0], true
}

// Resolve returns the value in the root that ref points at. The reference
// may be a fragment such as "#/$defs/Filter" or an "$anchor" name such as
// "#filter", and may name the root's "$id" or the "$id" of a resource
// that the root embeds. sameResource reports whether the value belongs to
// the root's resource, so that references inside it resolve as they
// would at the root. An anchor declared more than once in a resource
// names no subschema.
func (r *Resolver) Resolve(ref string) (target any, sameResource bool, ok bool) {
	prefix, fragment, _ := strings.Cut(ref, "#")
	tokens, isPointer := decodePointer(fragment)
	anchor, isAnchor := anchorFragment(fragment)
	if !isPointer && !isAnchor {
		return nil, false, false
	}
	resource, resourceKey, sameResource := r.root, "", true
	if prefix != "" {
		uri, ok := r.scope.resolve(prefix)
		if !ok {
			return nil, false, false
		}
		if !r.isRoot(uri) {
			embedded, ok := r.embedded(uri)
			if !ok {
				return nil, false, false
			}
			resource, resourceKey, sameResource = embedded.node, canonicalURI(uri), false
		}
	}
	if !isPointer {
		r.index()
		found := r.anchors[resourceKey][anchor]
		if len(found) != 1 {
			return nil, false, false
		}
		return found[0], sameResource, true
	}
	current := any(resource)
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

// ResourceRefs returns the JSON Pointer tokens, in the root, of the
// embedded resources that references in schema name by URI. A resource
// inside another embedded resource is reported as the outermost one,
// since its "$id" may resolve against the enclosing one. schema is read
// as a schema that declares the root's "$id", so it may be a part of the
// root placed where its references resolve as they do in the root.
func (r *Resolver) ResourceRefs(schema map[string]any) [][]string {
	var found [][]string
	walkNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		for _, key := range []string{"$ref", "$dynamicRef"} {
			ref, ok := node[key].(string)
			if !ok {
				continue
			}
			prefix, _, _ := strings.Cut(ref, "#")
			if prefix == "" {
				continue
			}
			uri, ok := scope.resolve(prefix)
			if !ok || r.isRoot(uri) {
				continue
			}
			if embedded, ok := r.embedded(uri); ok {
				found = append(found, embedded.outer.tokens())
			}
		}
	})
	return found
}

// visitFunc is called on schema objects with the resource scope each one
// belongs to.
type visitFunc func(node map[string]any, scope refScope)

// copyNode copies node and its subschemas, calling visit on each copy
// after its subschemas were copied. The values of other keywords are
// shared with the source and must not be modified.
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
		return copySchema(value, scope.at(key), visit)
	}
	if _, ok := subschemaListKeywords[key]; ok {
		return copySchemaList(value, scope.at(key), visit)
	}
	if _, ok := subschemaMapKeywords[key]; ok {
		return copySchemaMap(value, scope.at(key), visit)
	}
	switch key {
	case "items":
		// A schema, or a list of schemas before draft 2020-12.
		if _, ok := value.([]any); ok {
			return copySchemaList(value, scope.at(key), visit)
		}
		return copySchema(value, scope.at(key), visit)
	case "dependencies":
		// Draft 7 maps each name to a schema or to a list of names, and
		// copySchema keeps the lists as is.
		return copySchemaMap(value, scope.at(key), visit)
	default:
		return value
	}
}

// forEachSubschema calls visit with each schema object in the value of
// key, the positions copyKeyword copies as schemas, and its scope, given
// the scope of the schema holding key.
func forEachSubschema(key string, value any, scope refScope, visit visitFunc) {
	visitAt := func(item any, scope refScope) {
		if node, ok := item.(map[string]any); ok {
			visit(node, scope.enter(node))
		}
	}
	_, isSchema := subschemaKeywords[key]
	_, isList := subschemaListKeywords[key]
	_, isMap := subschemaMapKeywords[key]
	switch list, listed := value.([]any); {
	case isSchema || (key == "items" && !listed):
		visitAt(value, scope.at(key))
	case isList || (key == "items" && listed):
		for i, item := range list {
			visitAt(item, scope.at(key).at(strconv.Itoa(i)))
		}
	case isMap || key == "dependencies":
		named, _ := value.(map[string]any)
		for name, item := range named {
			visitAt(item, scope.at(key).at(name))
		}
	}
}

// walkNode calls visit on node and each of its subschemas, without
// copying them, before visiting their subschemas.
func walkNode(node map[string]any, scope refScope, visit visitFunc) {
	visit(node, scope)
	for key, value := range node {
		forEachSubschema(key, value, scope, func(child map[string]any, scope refScope) {
			walkNode(child, scope, visit)
		})
	}
}

func copySchemaList(value any, scope refScope, visit visitFunc) any {
	list, ok := value.([]any)
	if !ok {
		return value
	}
	out := make([]any, len(list))
	for i, item := range list {
		out[i] = copySchema(item, scope.at(strconv.Itoa(i)), visit)
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
		out[name] = copySchema(item, scope.at(name), visit)
	}
	return out
}

// refScope tracks where a subschema lies: its location in the root and
// the schema resource it belongs to. A "$id" starts a new resource, and
// JSON Schema resolves fragment-only references against the resource
// that contains them.
type refScope struct {
	// rootBase is the root resource's base URI, or nil when the root
	// declares no usable "$id".
	rootBase *url.URL
	// base is the current resource's base URI, or nil when unknown.
	base *url.URL
	// nested reports that a resource other than the root was entered.
	nested bool
	// path holds the last JSON Pointer token from the root to the
	// subschema, or nil at the root.
	path *pathToken
	// outer is the path of the outermost resource other than the root
	// that was entered.
	outer *pathToken
}

// pathToken is a JSON Pointer token linked to the tokens before it, so a
// scope is extended without copying its path, which would make walking
// deeply nested schemas quadratic.
type pathToken struct {
	parent *pathToken
	token  string
}

// at returns the scope of the value found under token.
func (s refScope) at(token string) refScope {
	s.path = &pathToken{parent: s.path, token: token}
	return s
}

// tokens returns the JSON Pointer tokens from the root to the subschema.
func (p *pathToken) tokens() []string {
	var tokens []string
	for ; p != nil; p = p.parent {
		tokens = append(tokens, p.token)
	}
	slices.Reverse(tokens)
	return tokens
}

// encodedLen returns the length of EncodePointer(p.tokens()). lengths
// remembers it for p and the paths before it, so the paths of many
// subschemas cost no more than the tokens they hold, and must map nil
// to 0.
func (p *pathToken) encodedLen(lengths map[*pathToken]int) int {
	var pending []*pathToken
	for ; ; p = p.parent {
		if _, ok := lengths[p]; ok {
			break
		}
		pending = append(pending, p)
	}
	length := lengths[p]
	for _, token := range slices.Backward(pending) {
		// Escaping is per byte, so a pointer's length is the sum of its
		// tokens' lengths.
		length += len(EncodePointer([]string{token.token}))
		lengths[token] = length
	}
	return length
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
	if !s.nested {
		s.outer = s.path
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

// EncodePointer returns the URI fragment, without the leading "#", that
// holds the JSON Pointer with the given unescaped tokens.
func EncodePointer(tokens []string) string {
	var pointer strings.Builder
	for _, token := range tokens {
		_, _ = pointer.WriteString("/")
		_, _ = pointer.WriteString(strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1"))
	}
	return (&url.URL{Fragment: pointer.String()}).EscapedFragment()
}
