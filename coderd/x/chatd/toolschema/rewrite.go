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

// IsKeyword reports whether value is a JSON Schema keyword.
func IsKeyword(value string) bool {
	return slices.Contains(keywords[:], value)
}

// Normalize returns a copy of a schema in the shapes provider drivers
// accept: draft 3 and draft 4 schemas are rewritten in draft 7 (see
// UpgradeLegacyDrafts),
// recursive references are replaced (see ReplaceRecursiveRefs), a
// "$schema" that names a published dialect is spelled as published, one
// that does not is removed, or names draft 7 when it is a draft before
// 2019-09 that the rest of the rewriting reads like draft 7, every type
// list is rewritten into an equivalent "anyOf", every array schema
// without "items" gets an empty one, an "enum" with an array or object
// member becomes an "anyOf" of "const" values, "$id" loses its fragment,
// which becomes an "$anchor" when draft 2020-12 allows its name, a
// reference that names the root resource by URI becomes a fragment
// reference, and a reference to an anchor that its resource declares
// once becomes a JSON Pointer reference into that resource. So does a
// "$dynamicRef" to such an anchor that no other schema declares as a
// dynamic anchor, since it then resolves like "$ref", and a
// "$dynamicRef" with a JSON Pointer becomes a "$ref". A reference that a
// schema with a "$ref" gains this way joins its "allOf". Instance data
// such as "const" and "enum" values keeps its exact shape. The source is
// not modified.
func Normalize(schema map[string]any) map[string]any {
	schema = ReplaceRecursiveRefs(UpgradeLegacyDrafts(schema))
	// Pointers are encoded only for the anchors references name.
	anchors := map[resourceAnchor]anchorPath{}
	duplicated := map[resourceAnchor]bool{}
	dynamicAnchors := map[string]int{}
	// resources maps the canonical URI of each embedded resource to the
	// keys of the subschemas that declare it.
	resources := map[string][]string{}
	keys := resourceKeys{}
	normalized := copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		// Read before normalizing, which drops a "$id" anchor when the
		// node also declares "$anchor".
		declared := declaredAnchors(node)
		if name, ok := node["$dynamicAnchor"].(string); ok {
			dynamicAnchors[name]++
		}
		resource := keys.of(scope.resourcePath)
		if _, ok := resourceID(node); ok && scope.nested && scope.base != nil {
			uri := canonicalURI(scope.base)
			resources[uri] = append(resources[uri], resource)
		}
		normalizeNode(node, scope)
		for _, name := range declared {
			key := resourceAnchor{resource: resource, name: name}
			if _, seen := anchors[key]; seen {
				duplicated[key] = true
			}
			anchors[key] = anchorPath{path: scope.path, resource: scope.resourcePath}
		}
	})
	// Some providers ignore anchors, and validators of drafts before
	// 2019-09 do not read "$anchor", but all of them follow a JSON
	// Pointer. Normalizing kept every subschema at its source path.
	refs := map[resourceAnchor][]anchorRef{}
	keys = resourceKeys{}
	addRef := func(node map[string]any, key string, scope refScope) {
		ref, _ := node[key].(string)
		prefix, fragment, _ := strings.Cut(ref, "#")
		name, ok := anchorFragment(fragment)
		if !ok {
			return
		}
		resource := keys.of(scope.resourcePath)
		if prefix != "" {
			uri, ok := scope.resolve(prefix)
			if !ok {
				return
			}
			if scope.rootBase != nil && canonicalURI(uri) == canonicalURI(scope.rootBase) {
				resource = ""
			} else if found := resources[canonicalURI(uri)]; len(found) == 1 {
				resource = found[0]
			} else {
				return
			}
		}
		anchor := resourceAnchor{resource: resource, name: name}
		if _, declared := anchors[anchor]; !declared || duplicated[anchor] {
			return
		}
		refs[anchor] = append(refs[anchor], anchorRef{node: node, key: key, prefix: prefix})
	}
	normalized = copyNode(normalized, rootScope(normalized), func(node map[string]any, scope refScope) {
		if _, ok := node["$ref"].(string); ok {
			addRef(node, "$ref", scope)
		}
		// Root folding follows only "$ref" into the keywords it removes.
		ref, ok := node["$dynamicRef"].(string)
		if !ok {
			return
		}
		_, fragment, _ := strings.Cut(ref, "#")
		if _, ok := decodePointer(fragment); ok {
			delete(node, "$dynamicRef")
			addReference(node, "$ref", ref)
			return
		}
		if anchor, ok := anchorFragment(fragment); ok && dynamicAnchors[anchor] > 1 {
			return
		}
		addRef(node, "$dynamicRef", scope)
	})
	// A pointer can be as long as the schema is deep, and as many
	// references as there are subschemas can name such anchors, so
	// pointers for all of them could make the schema quadratically
	// larger. Anchors are converted in order while their pointers fit a
	// budget, which a schema reaches only when the pointers alone would
	// take more than callers allow a tool schema.
	budget := maxAnchorPointerBytes
	lengths := map[*pathToken]int{nil: 0}
	for _, anchor := range slices.SortedFunc(maps.Keys(refs), compareResourceAnchors) {
		path := anchors[anchor]
		length := path.path.encodedLen(lengths) - path.resource.encodedLen(lengths) + len("#")
		if len(refs[anchor]) > budget/length {
			continue
		}
		budget -= length * len(refs[anchor])
		pointer := "#" + EncodePointer(path.path.tokens()[len(path.resource.tokens()):])
		for _, ref := range refs[anchor] {
			if ref.key == "$ref" {
				ref.node["$ref"] = ref.prefix + pointer
				continue
			}
			delete(ref.node, ref.key)
			addReference(ref.node, "$ref", ref.prefix+pointer)
		}
	}
	return normalized
}

// resourceAnchor is an anchor name in the schema resource with the given
// resourceKeys key.
type resourceAnchor struct {
	resource string
	name     string
}

func compareResourceAnchors(a, b resourceAnchor) int {
	if c := strings.Compare(a.resource, b.resource); c != 0 {
		return c
	}
	return strings.Compare(a.name, b.name)
}

// anchorPath is where an anchor is declared: its path in the root and
// the path of its resource's root.
type anchorPath struct {
	path     *pathToken
	resource *pathToken
}

// resourceKeys names schema resources by the JSON Pointer of their root,
// which normalizing keeps, so walks of the source and of the normalized
// schema agree on them. It encodes each path once.
type resourceKeys map[*pathToken]string

func (k resourceKeys) of(path *pathToken) string {
	if path == nil {
		return ""
	}
	key, ok := k[path]
	if !ok {
		key = EncodePointer(path.tokens())
		k[path] = key
	}
	return key
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

// draft4UnknownKeywords are keywords that later drafts apply to
// instances but draft 4 does not define, so that a draft 4 validator
// ignores them.
var draft4UnknownKeywords = [...]string{
	"const", "contains", "minContains", "maxContains", "propertyNames",
	"if", "then", "else", "dependentRequired", "dependentSchemas",
	"prefixItems", "unevaluatedItems", "unevaluatedProperties", "$dynamicRef",
}

// draft3UnknownKeywords are keywords that draft 4 added, so that a draft
// 3 validator ignores them.
var draft3UnknownKeywords = [...]string{
	"allOf", "anyOf", "oneOf", "not", "multipleOf", "minProperties", "maxProperties",
}

// UpgradeLegacyDrafts returns a copy of a schema in which every schema
// read in draft 3 or draft 4 is rewritten in draft 7, which names
// resources with "$id" and has "const", as the rest of the rewriting and
// the providers expect. A schema is read in the dialect that its
// "$schema" names or, without one, that of the nearest schema around it
// that has one, so the root and the resources it embeds may differ, and
// every URI under a draft, such as that of its hyper-schema, names that
// draft. Draft 3 schemas are first rewritten in draft 4 (see
// upgradeDraft3). In draft 4 schemas "id" becomes "$id", a boolean
// "exclusiveMinimum" or "exclusiveMaximum" becomes the number that draft
// 7 expects, and "$schema" names draft 7. The keywords a draft ignores
// but later drafts apply are removed, except where a reference's JSON
// Pointer passes through one: that occurrence keeps its value under a
// key no draft defines, which validators also ignore, and the references
// follow it. Normalize may then add some back where they hold as in the
// source, such as "const" for an "enum". Schemas read in other dialects
// keep their keywords, and a schema without any read in draft 3 or
// draft 4 is returned as is. A reference to an object outside the
// positions that hold schemas first names a copy of it that the
// rewriting reaches (see pullReferencedData).
func UpgradeLegacyDrafts(schema map[string]any) map[string]any {
	schema = upgradeDraft3(pullReferencedData(schema))
	if !readsIn(schema, isDraft4) {
		return schema
	}
	upgraded := copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		if !isDraft4(scope.dialect) {
			return
		}
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
	return removeUnknownKeywords(upgraded, isDraft4, draft4UnknownKeywords[:], "x-draft4-", "http://json-schema.org/draft-07/schema#")
}

// pullReferencedData returns a copy of a schema in which each "$ref"
// whose JSON Pointer locates an object outside the positions that hold
// schemas, such as under a key no draft defines, in a resource read in
// draft 3 or draft 4, names a copy of that object added to the
// "definitions" of that resource, where the rewriting reaches it. Left where it is,
// the object would keep what its draft reads unlike draft 7, such as a
// "const" that draft 4 ignores. The object also stays, as instance data.
// An object that declares an identifier is not copied, since in
// "definitions" it would declare it, and neither is one whose resource
// has "definitions" that is not an object. The schema itself is returned
// when nothing is copied, or when a schema read in those drafts other
// than the root names a resource with "id", which the scopes read only
// once it becomes "$id".
func pullReferencedData(schema map[string]any) map[string]any {
	legacy := func(dialect string) bool { return isDraft3(dialect) || isDraft4(dialect) }
	if !readsIn(schema, legacy) {
		return schema
	}
	resources := map[string][]*pathToken{}
	// roots maps the encoded path of each resource root to its scope.
	roots := map[string]refScope{}
	unread := false
	walkNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		if id, ok := node["id"].(string); ok && scope.path != nil && legacy(scope.dialect) && !strings.HasPrefix(id, "#") {
			unread = true
		}
		if _, ok := resourceID(node); ok && scope.nested && scope.base != nil {
			uri := canonicalURI(scope.base)
			resources[uri] = append(resources[uri], scope.path)
		}
		if scope.path == scope.resourcePath {
			roots[EncodePointer(scope.path.tokens())] = scope
		}
	})
	if unread {
		return schema
	}
	// names maps the encoded path of each object copied to the name of
	// its copy, and added maps the encoded path of each resource root to
	// the objects copied into its "definitions" by name.
	names := map[string]string{}
	added := map[string]map[string]map[string]any{}
	var find visitFunc
	find = func(node map[string]any, scope refScope) {
		ref, _ := node["$ref"].(string)
		resource, tokens, ok := scope.resourcePointer(ref, resources)
		if !ok || len(tokens) == 0 {
			return
		}
		path := append(slices.Clone(resource), tokens...)
		if _, ok := names[EncodePointer(path)]; ok {
			return
		}
		if _, _, ok := schemaAt(schema, path); ok {
			return
		}
		value, _ := lookupPointer(schema, path)
		object, ok := value.(map[string]any)
		if !ok || declaresIdentifier(object) {
			return
		}
		root, dialect, ok := schemaAt(schema, resource)
		if !ok || !legacy(dialect) {
			return
		}
		definitions, ok := root["definitions"].(map[string]any)
		if _, exists := root["definitions"]; exists && !ok {
			return
		}
		key := EncodePointer(resource)
		copies := added[key]
		if copies == nil {
			copies = map[string]map[string]any{}
			added[key] = copies
		}
		base := tokens[len(tokens)-1]
		name := base
		for n := 2; ; n++ {
			_, taken := definitions[name]
			if _, given := copies[name]; !taken && !given {
				break
			}
			name = base + "-" + strconv.Itoa(n)
		}
		copies[name] = object
		names[EncodePointer(path)] = name
		walkNode(object, roots[key].at("definitions").at(name).enter(object), find)
	}
	walkNode(schema, rootScope(schema), find)
	if len(names) == 0 {
		return schema
	}
	var visit visitFunc
	visit = func(node map[string]any, scope refScope) {
		if copies, ok := added[EncodePointer(scope.path.tokens())]; ok && scope.path == scope.resourcePath {
			definitions, _ := node["definitions"].(map[string]any)
			if definitions == nil {
				definitions = make(map[string]any, len(copies))
			}
			for name, object := range copies {
				definitions[name] = copySchema(object, scope.at("definitions").at(name), visit)
			}
			node["definitions"] = definitions
		}
		ref, _ := node["$ref"].(string)
		resource, tokens, ok := scope.resourcePointer(ref, resources)
		if !ok {
			return
		}
		if name, ok := names[EncodePointer(append(resource, tokens...))]; ok {
			prefix, _, _ := strings.Cut(ref, "#")
			node["$ref"] = prefix + "#" + EncodePointer([]string{"definitions", name})
		}
	}
	return copyNode(schema, rootScope(schema), visit)
}

// declaresIdentifier reports whether a schema or one of its subschemas
// declares a resource or an anchor.
func declaresIdentifier(schema map[string]any) bool {
	found := false
	walkNode(schema, rootScope(schema), func(node map[string]any, _ refScope) {
		for _, key := range []string{"id", "$id", "$anchor", "$dynamicAnchor"} {
			if _, ok := node[key].(string); ok {
				found = true
			}
		}
	})
	return found
}

// upgradeDraft3 returns a copy of a schema in which every schema read in
// draft 3 is rewritten in draft 4, or the schema itself when none is.
// Draft 4 changed what some draft 3 keywords mean, and providers reject
// the draft 3 forms: a "required": true in a property's schema becomes
// the property's name in the "required" list of the schema holding
// "properties", "type" "any" goes, schemas in a "type" list become an
// "anyOf", "disallow" becomes a "not", "extends" becomes an "allOf",
// "divisibleBy" becomes "multipleOf", a "dependencies" property name
// becomes a list of one, and "$schema" names draft 4. A reference to a
// schema that moves follows it. The keywords draft 4 added are removed
// first, as UpgradeLegacyDrafts describes, so the rewriting adds only
// the ones that hold as in the source.
func upgradeDraft3(schema map[string]any) map[string]any {
	if !readsIn(schema, isDraft3) {
		return schema
	}
	schema = removeUnknownKeywords(schema, isDraft3, draft3UnknownKeywords[:], "x-draft3-", "")
	// moved maps each schema the rewriting moves, by its encoded path
	// through the draft 3 keyword of its rewritten holder, to its new
	// path.
	moved := map[string][]string{}
	var visit visitFunc
	visit = func(node map[string]any, scope refScope) {
		if !isDraft3(scope.dialect) {
			return
		}
		// removeUnknownKeywords did not reach the schemas that moved here
		// either.
		for _, key := range draft3UnknownKeywords {
			delete(node, key)
		}
		// Draft 3 holds schemas under "extends" and in "type" and
		// "disallow" lists, which the copy did not reach. move copies the
		// schemas found in a keyword's value to the paths, relative to
		// node, that to returns.
		move := func(key string, value any, schemas []any, to func(i int) []string) []any {
			_, isList := value.([]any)
			copied := make([]any, len(schemas))
			for i, schema := range schemas {
				from := append(scope.path.tokens(), key)
				if isList {
					from = append(from, strconv.Itoa(i))
				}
				target := scope
				for _, token := range to(i) {
					target = target.at(token)
				}
				moved[EncodePointer(from)] = target.path.tokens()
				copied[i] = copySchema(schema, target, visit)
			}
			return copied
		}
		at := func(tokens ...string) func(i int) []string {
			return func(i int) []string {
				return append(slices.Clone(tokens), strconv.Itoa(i))
			}
		}
		if value, ok := node["divisibleBy"]; ok {
			delete(node, "divisibleBy")
			node["multipleOf"] = value
		}
		if value, ok := node["extends"]; ok {
			delete(node, "extends")
			list, ok := value.([]any)
			if !ok {
				list = []any{value}
			}
			node["allOf"] = move("extends", value, list, at("allOf"))
		}
		if variants, ok := draft3Types(node["type"]); ok {
			if !slices.ContainsFunc(variants, isEmptySchema) {
				node["anyOf"] = move("type", node["type"], variants, at("anyOf"))
			}
			delete(node, "type")
		}
		if value, ok := node["disallow"]; ok {
			delete(node, "disallow")
			variants, ok := draft3Types(value)
			if !ok {
				variants = []any{map[string]any{"type": value}}
			}
			switch len(variants) {
			case 0:
			case 1:
				node["not"] = move("disallow", value, variants, func(int) []string { return []string{"not"} })[0]
			default:
				node["not"] = map[string]any{"anyOf": move("disallow", value, variants, at("not", "anyOf"))}
			}
		}
		if dependencies, ok := node["dependencies"].(map[string]any); ok {
			for name, dependency := range dependencies {
				if property, ok := dependency.(string); ok {
					dependencies[name] = []any{property}
				}
			}
		}
	}
	upgraded := copyNode(schema, rootScope(schema), visit)
	// A property's "required" is read before the property's own schema
	// replaces it with the names of its properties.
	walkNode(upgraded, rootScope(upgraded), func(node map[string]any, scope refScope) {
		if !isDraft3(scope.dialect) {
			return
		}
		if _, ok := node["$schema"].(string); ok {
			node["$schema"] = "http://json-schema.org/draft-04/schema#"
		}
		if _, ok := node["required"].(bool); ok {
			delete(node, "required")
		}
		properties, _ := node["properties"].(map[string]any)
		var names []string
		for name, property := range properties {
			if property, ok := property.(map[string]any); ok {
				if required, _ := property["required"].(bool); required {
					names = append(names, name)
				}
			}
		}
		if len(names) == 0 {
			return
		}
		slices.Sort(names)
		// A list is not draft 3, but it names required properties as
		// in draft 4.
		required, _ := node["required"].([]any)
		required = slices.Clone(required)
		for _, name := range names {
			if !slices.Contains(required, any(name)) {
				required = append(required, name)
			}
		}
		node["required"] = required
	})
	if len(moved) == 0 {
		return upgraded
	}
	// References through a moved schema follow it. The paths are
	// compared encoded, built a token at a time.
	return RewriteRefs(upgraded, func(tokens []string) ([]string, bool) {
		renamed := make([]string, 0, len(tokens))
		var path []byte
		for _, token := range tokens {
			renamed = append(renamed, token)
			path = append(path, EncodePointer([]string{token})...)
			if to, ok := moved[string(path)]; ok {
				renamed = slices.Clone(to)
				path = append(path[:0], EncodePointer(to)...)
			}
		}
		return renamed, !slices.Equal(renamed, tokens)
	})
}

// draft3Types returns the schemas of the alternatives that a draft 3
// "type" or "disallow" value lists, with an empty schema for "any", and
// reports false when the value is a type name or list of type names that
// draft 4 reads the same way.
func draft3Types(value any) ([]any, bool) {
	list, ok := value.([]any)
	if !ok {
		list = []any{value}
	}
	plain := true
	variants := make([]any, len(list))
	for i, item := range list {
		switch item {
		case "any":
			plain = false
			variants[i] = map[string]any{}
		default:
			if _, ok := item.(map[string]any); ok {
				plain = false
				variants[i] = item
			} else {
				variants[i] = map[string]any{"type": item}
			}
		}
	}
	if plain {
		return nil, false
	}
	return variants, true
}

func isEmptySchema(value any) bool {
	node, ok := value.(map[string]any)
	return ok && len(node) == 0
}

// readsIn reports whether a schema or one of its subschemas is read in a
// dialect for which is returns true (see UpgradeLegacyDrafts).
func readsIn(schema map[string]any, is func(dialect string) bool) bool {
	found := false
	walkNode(schema, rootScope(schema), func(_ map[string]any, scope refScope) {
		found = found || is(scope.dialect)
	})
	return found
}

// isDraft3 reports whether a dialect that Dialect returned is a draft 3
// dialect.
func isDraft3(dialect string) bool {
	return strings.HasPrefix(draftPath(dialect), "/draft-03/")
}

// isDraft4 reports whether a dialect that Dialect returned is a draft 4
// dialect.
func isDraft4(dialect string) bool {
	return strings.HasPrefix(draftPath(dialect), "/draft-04/")
}

// isDraft201909 reports whether a dialect that Dialect returned is a
// draft 2019-09 dialect.
func isDraft201909(dialect string) bool {
	return strings.HasPrefix(draftPath(dialect), "/draft/2019-09/")
}

// IsLegacyDialect reports whether a dialect that Dialect returned is a
// draft before 2019-09, in which a "$ref" makes its schema ignore every
// other keyword.
func IsLegacyDialect(dialect string) bool {
	return strings.HasPrefix(draftPath(dialect), "/draft-0")
}

// draftPath returns the path of a dialect that Dialect returned when it
// is an http or https URI on json-schema.org, which publishes the JSON
// Schema drafts, or "" for any other dialect.
func draftPath(dialect string) string {
	uri, err := url.Parse(dialect)
	if err != nil || (uri.Scheme != "http" && uri.Scheme != "https") || uri.Host != "json-schema.org" {
		return ""
	}
	return uri.Path
}

// removeUnknownKeywords returns a copy of a schema without the unknown
// keywords in the schemas read in a dialect that legacy reports, keeping
// the occurrences that references pass through under a key that starts
// with prefix, as UpgradeLegacyDrafts describes. When target is not
// empty, their "$schema" becomes target.
func removeUnknownKeywords(schema map[string]any, legacy func(dialect string) bool, unknown []string, prefix, target string) map[string]any {
	// kept maps the encoded path of each occurrence a reference passes
	// through to the key that keeps its value.
	kept := map[string]string{}
	resources := map[string][]*pathToken{}
	walkNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		if _, ok := resourceID(node); ok && scope.nested && scope.base != nil {
			uri := canonicalURI(scope.base)
			resources[uri] = append(resources[uri], scope.path)
		}
	})
	walkNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		ref, _ := node["$ref"].(string)
		resource, tokens, ok := scope.resourcePointer(ref, resources)
		if !ok {
			return
		}
		keywords := PointerKeywords(tokens)
		for i, token := range tokens {
			if !keywords[i] || !slices.Contains(unknown, token) {
				continue
			}
			path := append(slices.Clone(resource), tokens[:i+1]...)
			holder, dialect, ok := schemaAt(schema, path[:len(path)-1])
			if !ok || !legacy(dialect) {
				continue
			}
			if _, ok := holder[token]; !ok {
				continue
			}
			name := prefix + token
			for n := 2; ; n++ {
				if _, taken := holder[name]; !taken {
					break
				}
				name = prefix + token + "-" + strconv.Itoa(n)
			}
			kept[EncodePointer(path)] = name
		}
	})
	return copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		if legacy(scope.dialect) {
			if _, ok := node["$schema"].(string); ok && target != "" {
				node["$schema"] = target
			}
			var path []string
			for _, key := range unknown {
				value, ok := node[key]
				if !ok {
					continue
				}
				delete(node, key)
				if len(kept) == 0 {
					continue
				}
				if path == nil {
					path = append(scope.path.tokens(), "")
				}
				path[len(path)-1] = key
				if name, ok := kept[EncodePointer(path)]; ok {
					node[name] = value
				}
			}
		}
		ref, _ := node["$ref"].(string)
		resource, tokens, ok := scope.resourcePointer(ref, resources)
		if !ok || len(kept) == 0 {
			return
		}
		renamed := slices.Clone(tokens)
		for i := range tokens {
			if name, ok := kept[EncodePointer(append(slices.Clone(resource), tokens[:i+1]...))]; ok {
				renamed[i] = name
			}
		}
		if !slices.Equal(renamed, tokens) {
			prefix, _, _ := strings.Cut(ref, "#")
			node["$ref"] = prefix + "#" + EncodePointer(renamed)
		}
	})
}

// schemaAt returns the schema object that JSON Pointer tokens locate in
// root, passing only through the positions that hold schemas, and the
// dialect it is read in (see UpgradeLegacyDrafts). It reports false when the
// tokens locate no such schema.
func schemaAt(root map[string]any, tokens []string) (map[string]any, string, bool) {
	node, dialect := root, Dialect(root)
	for i := 0; i < len(tokens); i++ {
		value, ok := node[tokens[i]]
		if !ok {
			return nil, "", false
		}
		kind := KeywordSubschemas(tokens[i], value)
		if isDraft3(dialect) && (tokens[i] == "extends" || tokens[i] == "type" || tokens[i] == "disallow") {
			// Draft 3 holds a schema or a list of them under "extends",
			// and upgradeDraft3 reads "type" and "disallow" the same way,
			// where a list may also name types.
			switch value.(type) {
			case []any:
				kind = SubschemaList
			case map[string]any:
				kind = SingleSubschema
			}
		}
		switch kind {
		case SingleSubschema:
		case SubschemaList, SubschemaMap:
			i++
			if i == len(tokens) {
				return nil, "", false
			}
			if value, ok = lookupPointer(value, tokens[i:i+1]); !ok {
				return nil, "", false
			}
		default:
			return nil, "", false
		}
		if node, ok = value.(map[string]any); !ok {
			return nil, "", false
		}
		if _, ok := node["$schema"].(string); ok {
			dialect = Dialect(node)
		}
	}
	return node, dialect, true
}

// lookupPointer returns the value that JSON Pointer tokens locate in
// root.
func lookupPointer(root any, tokens []string) (any, bool) {
	current := root
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

// ReplaceRecursiveRefs returns a copy of a schema without the draft
// 2019-09 keywords "$recursiveAnchor" and "$recursiveRef", which draft
// 2020-12 replaced and providers reject or ignore. A "$recursiveRef"
// names the root of its resource, so it becomes a "$ref" to it, except
// in a resource other than the root whose root declares
// "$recursiveAnchor": there the dynamic scope picks the target, so it
// becomes a "$dynamicRef" to a "$dynamicAnchor" that each such resource
// root declares, which resolves the same way. It is left out when a name
// for that anchor cannot be added, which only drops a constraint. Other
// values are kept. Only a schema read in draft 2019-09 (see
// UpgradeLegacyDrafts) has these keywords: other dialects ignore them,
// and so does a schema read without "$schema", which MCP reads as draft
// 2020-12, so there they are only removed. Other schemas are returned as
// is.
func ReplaceRecursiveRefs(schema map[string]any) map[string]any {
	found, dynamic, clash := false, false, false
	used := map[string]bool{}
	walkNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		_, hasRef := node["$recursiveRef"]
		_, hasAnchor := node["$recursiveAnchor"]
		found = found || hasRef || hasAnchor
		for _, name := range declaredAnchors(node) {
			used[name] = true
		}
		for _, key := range []string{"$ref", "$dynamicRef"} {
			ref, _ := node[key].(string)
			_, fragment, _ := strings.Cut(ref, "#")
			if name, ok := anchorFragment(fragment); ok {
				used[name] = true
			}
		}
		if !isDraft201909(scope.dialect) {
			return
		}
		if _, ok := node["$dynamicAnchor"]; ok && recursiveAnchor(node) && scope.resourcePath == scope.path {
			clash = true
		}
		if ref, ok := node["$recursiveRef"].(string); ok && scope.nested && recursiveAnchor(scope.resource) && scope.namesResource(ref) {
			dynamic = true
		}
	})
	if !found {
		return schema
	}
	name := "recursive"
	for i := 2; used[name]; i++ {
		name = "recursive" + strconv.Itoa(i)
	}
	return copyNode(schema, rootScope(schema), func(node map[string]any, scope refScope) {
		anchored := recursiveAnchor(node)
		delete(node, "$recursiveAnchor")
		if !isDraft201909(scope.dialect) {
			delete(node, "$recursiveRef")
			return
		}
		if anchored && dynamic && !clash && scope.resourcePath == scope.path {
			node["$dynamicAnchor"] = name
		}
		ref, ok := node["$recursiveRef"].(string)
		if !ok || !scope.namesResource(ref) {
			return
		}
		delete(node, "$recursiveRef")
		switch {
		case !scope.nested || !recursiveAnchor(scope.resource):
			addReference(node, "$ref", "#")
		case !clash:
			addReference(node, "$dynamicRef", "#"+name)
		}
	})
}

// recursiveAnchor reports whether node declares "$recursiveAnchor": true.
func recursiveAnchor(node map[string]any) bool {
	anchored, _ := node["$recursiveAnchor"].(bool)
	return anchored
}

// addReference adds a reference keyword to node, in its "allOf" when
// node already has that keyword.
func addReference(node map[string]any, key, ref string) {
	if _, ok := node[key]; !ok {
		node[key] = ref
		return
	}
	allOf, _ := node["allOf"].([]any)
	node["allOf"] = append(slices.Clone(allOf), map[string]any{key: ref})
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

	// Some providers resolve only fragment references. A "$dynamicRef"
	// resolves against the same base before the dynamic scope applies.
	for _, key := range []string{"$ref", "$dynamicRef"} {
		ref, ok := node[key].(string)
		if !ok || scope.nested || strings.HasPrefix(ref, "#") {
			continue
		}
		if fragment, ok := scope.rootFragment(ref); ok {
			if tokens, ok := decodePointer(fragment); ok {
				node[key] = "#" + EncodePointer(tokens)
			} else if anchor, ok := anchorFragment(fragment); ok {
				node[key] = "#" + anchor
			}
		}
	}

	// Some providers check a schema against the dialect "$schema" names
	// only when it is spelled exactly as published, and reject a schema
	// whose "$schema" names any other dialect. Without "$schema" they
	// read draft 2020-12, which MCP specifies as the default. A URI under
	// a draft before 2019-09 is read as that draft, so it names draft 7,
	// whose "$ref" ignores its siblings in the same way.
	if dialect, ok := node["$schema"].(string); ok {
		switch published, ok := PublishedDialect(dialect); {
		case ok:
			node["$schema"] = published
		case IsLegacyDialect(Dialect(node)):
			node["$schema"] = "http://json-schema.org/draft-07/schema#"
		default:
			delete(node, "$schema")
		}
	}
}

// publishedDialects are the "$schema" URIs of the published JSON Schema
// dialects that Normalize keeps. Draft 3 is not among them:
// UpgradeLegacyDrafts rewrites draft 3 schemas in draft 7.
var publishedDialects = [...]string{
	"http://json-schema.org/draft-04/schema#",
	"http://json-schema.org/draft-06/schema#",
	"http://json-schema.org/draft-07/schema#",
	"https://json-schema.org/draft/2019-09/schema",
	"https://json-schema.org/draft/2020-12/schema",
}

// PublishedDialect returns the published spelling of a "$schema" URI that
// differs from a published dialect's only in case where URIs ignore it,
// in http against https, or in an empty fragment.
func PublishedDialect(dialect string) (string, bool) {
	key, ok := dialectKey(dialect)
	if !ok {
		return "", false
	}
	for _, published := range publishedDialects {
		if publishedKey, _ := dialectKey(published); publishedKey == key {
			return published, true
		}
	}
	return "", false
}

// dialectKey returns an http or https URI in canonical form without its
// scheme or empty fragment.
func dialectKey(dialect string) (string, bool) {
	uri, err := parseURI(dialect)
	if err != nil || (uri.Scheme != "http" && uri.Scheme != "https") || uri.Fragment != "" {
		return "", false
	}
	return strings.TrimPrefix(canonicalURI(uri), uri.Scheme+":"), true
}

var (
	// anchorName matches the plain-name fragments that draft 2020-12
	// allows as "$anchor" values.
	anchorName = regexp.MustCompile(`^[A-Za-z_][-A-Za-z0-9._]*$`)
	// legacyAnchorName matches those and the plain-name fragments that
	// drafts 6 and 7 allow in "$id", which may also hold colons.
	legacyAnchorName = regexp.MustCompile(`^[A-Za-z_][-A-Za-z0-9._:]*$`)
)

// anchorFragment returns the anchor name that a URI fragment holds,
// which may be percent-encoded like the rest of the fragment.
func anchorFragment(fragment string) (string, bool) {
	name, err := url.PathUnescape(fragment)
	if err != nil || !legacyAnchorName.MatchString(name) {
		return "", false
	}
	return name, true
}

// declaredAnchors returns the plain-name anchors node declares, with
// "$anchor" or "$dynamicAnchor" or, as drafts before 2019-09 did, with a
// "$id" fragment, which may be percent-encoded.
func declaredAnchors(node map[string]any) []string {
	anchors := plainAnchors(node)
	if id, ok := node["$id"].(string); ok {
		_, fragment, _ := strings.Cut(id, "#")
		if name, ok := anchorFragment(fragment); ok && !slices.Contains(anchors, name) {
			anchors = append(anchors, name)
		}
	}
	return anchors
}

// plainAnchors returns the plain-name anchors node declares with
// "$anchor" or "$dynamicAnchor", which both name it in a fragment.
func plainAnchors(node map[string]any) []string {
	var anchors []string
	for _, key := range []string{"$anchor", "$dynamicAnchor"} {
		if anchor, ok := node[key].(string); ok && anchorName.MatchString(anchor) && !slices.Contains(anchors, anchor) {
			anchors = append(anchors, anchor)
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
// tokens. Anchor references and references into other resources,
// including subschemas that declare their own "$id", are kept. The
// source is not modified.
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
	})
}

// RefScanner reports whether schemas hold reference keywords. It
// remembers the answer for every schema object it scans, so a subschema
// that is nested in or shared by the schemas asked about is scanned once.
// Schema objects are told apart by address, so they must stay reachable
// while the scanner is in use. The zero value is ready to use.
type RefScanner struct {
	scanner keywordScanner
}

// HasRefs reports whether schema or one of its subschemas holds a
// reference keyword.
func (s *RefScanner) HasRefs(schema map[string]any) bool {
	return s.scanner.holds(schema, []string{"$ref", "$dynamicRef", "$recursiveRef"})
}

// IdentifierScanner reports whether schemas hold identifier keywords,
// remembering the answers like RefScanner. The zero value is ready to
// use.
type IdentifierScanner struct {
	scanner keywordScanner
}

// HasNestedIdentifiers reports whether a subschema of schema, other than
// schema itself, declares a resource or an anchor through "$id",
// "$anchor" or "$dynamicAnchor".
func (s *IdentifierScanner) HasNestedIdentifiers(schema map[string]any) bool {
	found := false
	for key, value := range schema {
		forEachSubschema(key, value, refScope{}, func(node map[string]any, _ refScope) {
			found = found || s.scanner.holds(node, []string{"$id", "$anchor", "$dynamicAnchor"})
		})
		if found {
			return true
		}
	}
	return false
}

// keywordScanner reports whether schemas or their subschemas hold any of
// a set of keywords, which must be the same on every call.
type keywordScanner struct {
	scanned map[uintptr]bool
}

func (s *keywordScanner) holds(schema map[string]any, keywords []string) bool {
	address := reflect.ValueOf(schema).Pointer()
	if found, ok := s.scanned[address]; ok {
		return found
	}
	found := false
	for _, key := range keywords {
		if _, ok := schema[key]; ok {
			found = true
		}
	}
	for key, value := range schema {
		if found {
			break
		}
		forEachSubschema(key, value, refScope{}, func(node map[string]any, _ refScope) {
			found = found || s.holds(node, keywords)
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
	// root resource, and each "$anchor" or "$dynamicAnchor" name in it to
	// the subschemas declaring that anchor.
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
		for _, anchor := range plainAnchors(node) {
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
// may be a fragment such as "#/$defs/Filter" or an "$anchor" or
// "$dynamicAnchor" name such as "#filter", and may name the root's "$id"
// or the "$id" of a resource that the root embeds. sameResource reports
// whether the value belongs to the root's resource, so that references
// inside it resolve as they would at the root. An anchor declared more
// than once in a resource names no subschema.
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
	switch KeywordSubschemas(key, value) {
	case SingleSubschema:
		return copySchema(value, scope.at(key), visit)
	case SubschemaList:
		return copySchemaList(value, scope.at(key), visit)
	case SubschemaMap:
		return copySchemaMap(value, scope.at(key), visit)
	default:
		return value
	}
}

// SubschemaKind is how the value of a keyword in a schema object holds
// subschemas.
type SubschemaKind int

const (
	// NoSubschema is the kind of a value that is not a schema, such as
	// instance data, an annotation, or a list of property names.
	NoSubschema SubschemaKind = iota
	// SingleSubschema is the kind of a value that is a schema.
	SingleSubschema
	// SubschemaList is the kind of a list of schemas.
	SubschemaList
	// SubschemaMap is the kind of a map from names to schemas. Draft 7
	// "dependencies" may map a name to a list of names instead.
	SubschemaMap
)

// KeywordSubschemas returns how the value of key, a keyword in a schema
// object, holds subschemas.
func KeywordSubschemas(key string, value any) SubschemaKind {
	if _, ok := subschemaKeywords[key]; ok {
		return SingleSubschema
	}
	if _, ok := subschemaListKeywords[key]; ok {
		return SubschemaList
	}
	if _, ok := subschemaMapKeywords[key]; ok {
		return SubschemaMap
	}
	switch key {
	case "items":
		// A schema, or a list of schemas before draft 2020-12.
		if _, ok := value.([]any); ok {
			return SubschemaList
		}
		return SingleSubschema
	case "dependencies":
		return SubschemaMap
	default:
		return NoSubschema
	}
}

// PointerKeywords reports which tokens of a JSON Pointer that starts at
// a schema are keys of the schema objects it passes through. The other
// tokens are names in a map of schemas such as "properties", indexes in
// a list of schemas, or locations in instance data, and a token spelled
// like a keyword there is only a name.
func PointerKeywords(tokens []string) []bool {
	const (
		schemaKey = iota
		member
		data
	)
	keywords := make([]bool, len(tokens))
	role := schemaKey
	for i, token := range tokens {
		switch role {
		case member:
			role = schemaKey
		case schemaKey:
			if !IsKeyword(token) {
				role = data
				continue
			}
			keywords[i] = true
			switch KeywordSubschemas(token, nil) {
			case SingleSubschema:
				// "items" holds a list of schemas before draft 2020-12.
				if token == "items" && i+1 < len(tokens) && isIndex(tokens[i+1]) {
					role = member
				}
			case SubschemaList, SubschemaMap:
				role = member
			default:
				role = data
			}
		}
	}
	return keywords
}

func isIndex(token string) bool {
	return token != "" && strings.Trim(token, "0123456789") == ""
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
	switch KeywordSubschemas(key, value) {
	case SingleSubschema:
		visitAt(value, scope.at(key))
	case SubschemaList:
		list, _ := value.([]any)
		for i, item := range list {
			visitAt(item, scope.at(key).at(strconv.Itoa(i)))
		}
	case SubschemaMap:
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
	// resource is the source schema of the current resource's root, and
	// resourcePath its path, which is nil for the root resource.
	resource     map[string]any
	resourcePath *pathToken
	// dialect is the dialect the subschema is read in (see
	// UpgradeLegacyDrafts), as Dialect returns it.
	dialect string
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
	scope := refScope{base: &url.URL{}, resource: root, dialect: Dialect(root)}
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
	if _, ok := node["$schema"].(string); ok {
		s.dialect = Dialect(node)
	}
	id, ok := resourceID(node)
	if !ok {
		return s
	}
	if !s.nested {
		s.outer = s.path
	}
	s.nested = true
	s.resource, s.resourcePath = node, s.path
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

// resourcePointer returns the decoded JSON Pointer tokens of ref and the
// path in the root of the resource root the pointer starts at. resources
// maps the canonical URI of each embedded resource to the paths of the
// subschemas that declare it, and a reference that names a URI more than
// one of them declares names none.
func (s refScope) resourcePointer(ref string, resources map[string][]*pathToken) (resource, tokens []string, ok bool) {
	prefix, fragment, hasFragment := strings.Cut(ref, "#")
	switch {
	case !hasFragment:
		return nil, nil, false
	case prefix == "":
		resource = s.resourcePath.tokens()
	default:
		uri, ok := s.resolve(prefix)
		if !ok {
			return nil, nil, false
		}
		if s.rootBase == nil || canonicalURI(uri) != canonicalURI(s.rootBase) {
			found := resources[canonicalURI(uri)]
			if len(found) != 1 {
				return nil, nil, false
			}
			resource = found[0].tokens()
		}
	}
	tokens, ok = decodePointer(fragment)
	return resource, tokens, ok
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

// namesResource reports whether ref names the root of the resource s
// belongs to.
func (s refScope) namesResource(ref string) bool {
	prefix, fragment, _ := strings.Cut(ref, "#")
	if fragment != "" {
		return false
	}
	if prefix == "" {
		return true
	}
	if s.base == nil {
		return false
	}
	target, ok := s.resolve(prefix)
	return ok && canonicalURI(target) == canonicalURI(s.base)
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
