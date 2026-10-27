package chatloop

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/x/chatd/toolschema"
)

// fullSchemaTool is implemented by tools that can report their
// complete JSON Schema input schema, preserving keys such as $defs,
// additionalProperties, and combinators that fantasy.ToolInfo cannot
// carry.
type fullSchemaTool interface {
	FullInputSchema() map[string]any
}

// BuildToolDefinitions converts AgentTool definitions into the
// fantasy.Tool slice expected by fantasy.Call. When activeTools
// is non-empty, only function tools whose name appears in the
// list are included. Provider tool definitions are always
// appended unconditionally.
func BuildToolDefinitions(tools []fantasy.AgentTool, activeTools []string, providerTools []ProviderTool) []fantasy.Tool {
	prepared := make([]fantasy.Tool, 0, len(tools)+len(providerTools))
	for _, tool := range tools {
		info := tool.Info()
		if !isToolActive(info.Name, activeTools) {
			continue
		}
		prepared = append(prepared, fantasy.FunctionTool{
			Name:            info.Name,
			Description:     info.Description,
			InputSchema:     ToolInputSchema(tool),
			ProviderOptions: tool.ProviderOptions(),
		})
	}
	for _, pt := range providerTools {
		prepared = append(prepared, pt.Definition)
	}
	return prepared
}

// ToolInputSchema returns the input schema BuildToolDefinitions sends
// to the model for tool, so callers can bound its size.
func ToolInputSchema(tool fantasy.AgentTool) map[string]any {
	inputSchema := fullInputSchema(tool)
	if inputSchema == nil {
		info := tool.Info()
		// Substitute an empty object for nil properties so that a tool
		// with no parameters never serializes "properties" to null,
		// which OpenAI rejects.
		properties := info.Parameters
		if properties == nil {
			properties = map[string]any{}
		}
		inputSchema = map[string]any{
			"type":       "object",
			"properties": properties,
		}
		// Only include "required" when non-empty so that a nil slice
		// never serializes to null, which OpenAI rejects.
		if len(info.Required) > 0 {
			inputSchema["required"] = info.Required
		}
	}
	return enforceObjectRoot(toolschema.Normalize(inputSchema))
}

// fullInputSchema returns a shallow copy of the tool's full input schema, or
// nil when the tool does not provide one and the caller must
// reconstruct a schema from Info().
func fullInputSchema(tool fantasy.AgentTool) map[string]any {
	provider, ok := tool.(fullSchemaTool)
	if !ok {
		return nil
	}
	source := provider.FullInputSchema()
	if source == nil {
		return nil
	}
	inputSchema := make(map[string]any, len(source)+1)
	for key, value := range source {
		inputSchema[key] = value
	}
	// JSON-unmarshaled schemas carry "required" as []any, which some
	// provider drivers silently drop on a []string type assertion.
	// Coerce to []string, and drop the key when empty so it never
	// serializes to null, which OpenAI rejects.
	if raw, ok := inputSchema["required"]; ok {
		required := coerceRequiredStrings(raw)
		if len(required) > 0 {
			inputSchema["required"] = required
		} else {
			delete(inputSchema, "required")
		}
	}
	return inputSchema
}

// coerceRequiredStrings converts a schema "required" value to
// []string, accepting the []any shape JSON unmarshaling produces and
// skipping non-string entries.
func coerceRequiredStrings(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		required := make([]string, 0, len(typed))
		for _, entry := range typed {
			if str, ok := entry.(string); ok {
				required = append(required, str)
			}
		}
		return required
	default:
		return nil
	}
}

// bannedRootKeys are schema keywords providers reject at the root of a
// tool input schema. OpenAI fails the whole request with 400 "schema
// must have type 'object' and not have 'oneOf'/'anyOf'/'allOf'/'enum'/
// 'const'/'not' at the top level", which would stall every turn of a
// chat carrying such a tool. "$ref" is included because it defines the
// root shape elsewhere and would dangle once an object root is forced.
var bannedRootKeys = [...]string{"$ref", "anyOf", "oneOf", "allOf", "enum", "const", "not"}

// maxRootMergeSchemas bounds how many schemas root composition merges
// beyond one for each composition branch and reference in the schema,
// which also bounds how deeply it recurses. A reference is merged into
// each object shape once, but alternatives that share a reference each
// expand it again, so a small schema could otherwise take exponential
// work. Schemas past the bound are not merged, which only drops
// constraints.
const maxRootMergeSchemas = 1000

// enforceObjectRoot returns the tool input schema with the plain object
// root MCP mandates and every provider accepts: "type" is "object" and
// "properties" is never null. It runs after toolschema.Normalize, which
// can turn a root type array into a root "anyOf". Root
// keywords providers reject are removed, but their meaning is folded
// into the root first: properties and required names from "allOf" and
// "$ref" are merged in, and those from "anyOf" and "oneOf" are merged
// with a name required only when every alternative requires it. The
// keywords of merged "allOf" and "$ref" schemas that constrain the
// object as a whole, such as "additionalProperties" and "minProperties",
// apply at the root as well, and an alternative that no value satisfies
// is dropped. A schema declaring a draft before 2019-09 ignores every
// keyword beside "$ref", so only the reference is folded there. Removed
// content that a reference still points into moves under the root
// definitions, and those references follow it.
func enforceObjectRoot(inputSchema map[string]any) map[string]any {
	var banned []string
	for _, key := range bannedRootKeys {
		if _, ok := inputSchema[key]; ok {
			banned = append(banned, key)
		}
	}
	if len(banned) > 0 {
		inputSchema = foldRootComposition(inputSchema, banned)
	}
	if inputSchema["type"] != "object" {
		inputSchema["type"] = "object"
	}
	if properties, _ := inputSchema["properties"].(map[string]any); properties == nil {
		inputSchema["properties"] = map[string]any{}
	}
	return inputSchema
}

// keptBesideLegacyRootRef lists the root keywords a schema declaring a
// draft before 2019-09 keeps beside a root "$ref": the dialect and
// resource identity, the definitions references resolve into, and
// annotations. Every other keyword there is ignored, so it is removed.
var keptBesideLegacyRootRef = [...]string{"$schema", "$id", "$defs", "definitions", "$comment", "title", "description"}

func foldRootComposition(inputSchema map[string]any, banned []string) map[string]any {
	folder := rootFolder{
		root:              inputSchema,
		visiting:          map[string]struct{}{},
		maxMerged:         compositionEntries(inputSchema) + maxRootMergeSchemas,
		ignoreRefSiblings: ignoresRefSiblings(inputSchema),
	}
	shape := objectShape{properties: map[string]any{}}
	if ref, ok := inputSchema["$ref"].(string); ok && folder.ignoreRefSiblings {
		for key := range inputSchema {
			if !slices.Contains(keptBesideLegacyRootRef[:], key) && !slices.Contains(banned, key) {
				banned = append(banned, key)
			}
		}
		folder.mergeRef(&shape, ref)
	} else {
		if properties, ok := inputSchema["properties"].(map[string]any); ok {
			for name, property := range properties {
				shape.properties[name] = property
			}
		}
		shape.addRequired(coerceRequiredStrings(inputSchema["required"]))
		if ref, ok := inputSchema["$ref"].(string); ok {
			folder.mergeRef(&shape, ref)
		}
		if branches, ok := inputSchema["allOf"].([]any); ok {
			for _, branch := range branches {
				folder.mergeAll(&shape, branch)
			}
		}
		for _, key := range []string{"anyOf", "oneOf"} {
			if branches, ok := inputSchema[key].([]any); ok {
				folder.mergeAlternatives(&shape, branches)
			}
		}
	}
	// Folding can leave names that the source evaluated unevaluated at
	// the root, which "unevaluatedProperties" would then constrain.
	if _, ok := inputSchema["unevaluatedProperties"]; ok && (folder.opaque || slices.ContainsFunc(shape.merged, evaluatesUnhoisted)) {
		banned = append(banned, "unevaluatedProperties")
	}

	removed := make(map[string]any, len(banned))
	for _, key := range banned {
		removed[key] = inputSchema[key]
		delete(inputSchema, key)
	}
	hoistKeywords(inputSchema, shape.merged)
	// Some providers reject a required name missing from "properties".
	var undeclared []string
	for _, name := range shape.required {
		if _, ok := shape.properties[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}
	maps.Copy(shape.properties, folder.undeclaredRootProperties(inputSchema, undeclared))
	required := slices.DeleteFunc(shape.required, func(name string) bool {
		_, declared := shape.properties[name]
		return !declared
	})
	inputSchema["properties"] = shape.properties
	if len(required) > 0 {
		inputSchema["required"] = required
	} else {
		delete(inputSchema, "required")
	}
	return relocateReferencedRootKeywords(inputSchema, removed)
}

// ignoresRefSiblings reports whether a schema declares a draft before
// 2019-09, in which every keyword beside "$ref" is ignored.
func ignoresRefSiblings(schema map[string]any) bool {
	dialect, _ := schema["$schema"].(string)
	return strings.Contains(dialect, "json-schema.org/draft-0")
}

// maxPatternMatchCost bounds the regular expression work of matching the
// property names root folding gives a schema against
// "patternProperties", since a schema can pair many such names with many
// patterns. Matching a name costs the product of the name and pattern
// lengths, and compiling a pattern is charged as matching it against a
// 15 byte name, which costs about as much.
const maxPatternMatchCost = 1 << 12

// patternMatcher matches property names against "patternProperties"
// patterns within maxPatternMatchCost, compiling each pattern once.
type patternMatcher struct {
	cost int
	// compiled holds the patterns compiled so far, with nil for those
	// Go cannot compile.
	compiled map[string]*regexp.Regexp
}

// match returns the sorted patterns in patterns that name matches. It
// reports false when that is unknown: maxPatternMatchCost is spent, or a
// pattern Go cannot compile, such as one using lookahead, might match.
func (m *patternMatcher) match(patterns map[string]any, name string) ([]string, bool) {
	var matched []string
	for pattern := range patterns {
		if m.cost > maxPatternMatchCost {
			return nil, false
		}
		re, compiled := m.compiled[pattern]
		if !compiled {
			m.cost += 16 * (len(pattern) + 1)
			if m.compiled == nil {
				m.compiled = map[string]*regexp.Regexp{}
			}
			var err error
			if re, err = regexp.Compile(pattern); err != nil {
				m.compiled[pattern] = nil
				return nil, false
			}
			m.compiled[pattern] = re
		}
		if re == nil {
			return nil, false
		}
		m.cost += (len(name) + 1) * (len(pattern) + 1)
		if m.cost > maxPatternMatchCost {
			return nil, false
		}
		if re.MatchString(name) {
			matched = append(matched, pattern)
		}
	}
	slices.Sort(matched)
	return matched, true
}

// undeclaredRootProperties returns property schemas for required names
// the root does not list in "properties" that keep the root's
// constraints on them: any value when a "patternProperties" pattern
// matches the name, since the pattern keeps applying, and otherwise the
// "additionalProperties" schema. A name that "additionalProperties"
// forbids is left out, since no argument can then hold it.
func (f *rootFolder) undeclaredRootProperties(root map[string]any, names []string) map[string]any {
	properties := make(map[string]any, len(names))
	additional, restricted := root["additionalProperties"].(map[string]any)
	if allowed, ok := root["additionalProperties"].(bool); ok && !allowed {
		restricted = true
	}
	patterns, _ := root["patternProperties"].(map[string]any)
	for _, name := range names {
		if !restricted {
			properties[name] = map[string]any{}
			continue
		}
		matched, known := f.patterns.match(patterns, name)
		switch {
		case !known || len(matched) > 0:
			properties[name] = map[string]any{}
		case additional != nil:
			properties[name] = additional
		}
	}
	return properties
}

// hoistKeywords adds to the root the keywords of the schemas folded into
// it that constrain the object as a whole. "patternProperties" and
// "additionalProperties" still hold for every name they constrained
// before, since the root's "properties" holds every name a folded schema
// lists and the root's "patternProperties" every pattern. Other keywords
// mean the same at the root.
func hoistKeywords(root map[string]any, merged []map[string]any) {
	for _, node := range merged {
		for key, value := range node {
			switch key {
			case "patternProperties", "dependentSchemas", "dependentRequired", "dependencies":
				entries, ok := value.(map[string]any)
				if !ok || len(entries) == 0 {
					continue
				}
				existing, _ := root[key].(map[string]any)
				combined := maps.Clone(existing)
				if combined == nil {
					combined = make(map[string]any, len(entries))
				}
				for name, entry := range entries {
					combined[name] = conjoinEntries(combined[name], entry)
				}
				root[key] = combined
			case "additionalProperties", "propertyNames":
				if schema, ok := value.(map[string]any); isBooleanSchema(value, false) || (ok && len(schema) > 0) {
					root[key] = conjoinSchemas(root[key], value)
				}
			case "minProperties", "maxProperties":
				bound, ok := schemaNumber(value)
				if !ok {
					continue
				}
				if existing, ok := schemaNumber(root[key]); ok {
					stricter := bound > existing
					if key == "maxProperties" {
						stricter = bound < existing
					}
					if !stricter {
						continue
					}
				}
				root[key] = value
			}
		}
	}
}

// conjoinEntries combines two entries of a keyword that maps property
// names to schemas or to lists of names, such as "dependencies". A nil
// entry is absent.
func conjoinEntries(existing, entry any) any {
	existingNames, existingIsList := existing.([]any)
	names, isList := entry.([]any)
	switch {
	case existing == nil:
		return entry
	case existingIsList && isList:
		combined := slices.Clone(existingNames)
		for _, name := range names {
			if !slices.Contains(combined, name) {
				combined = append(combined, name)
			}
		}
		return combined
	case existingIsList:
		existing = map[string]any{"required": existingNames}
	case isList:
		entry = map[string]any{"required": names}
	}
	return conjoinSchemas(existing, entry)
}

// schemaNumber returns the value of a number in a schema, which JSON
// decoding leaves as a float64, a json.Number or a json.RawMessage.
func schemaNumber(value any) (float64, bool) {
	var text string
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case json.Number:
		text = typed.String()
	case json.RawMessage:
		text = string(typed)
	default:
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// conjoinSchemas returns a schema that holds when both a and b hold,
// where a nil schema is absent and holds for every value.
func conjoinSchemas(a, b any) any {
	switch {
	case a == nil || isBooleanSchema(a, true) || reflect.DeepEqual(a, b):
		return b
	case b == nil || isBooleanSchema(b, true):
		return a
	case isBooleanSchema(a, false) || isBooleanSchema(b, false):
		return false
	default:
		return map[string]any{"allOf": []any{a, b}}
	}
}

// isBooleanSchema reports whether schema is the boolean schema holds.
func isBooleanSchema(schema any, holds bool) bool {
	value, ok := schema.(bool)
	return ok && value == holds
}

// evaluatesUnhoisted reports whether node, a schema folded into the root
// that must hold, evaluates property names with a keyword the root does
// not take on, which matters to "unevaluatedProperties".
func evaluatesUnhoisted(node map[string]any) bool {
	for _, key := range []string{"unevaluatedProperties", "if", "$dynamicRef", "$recursiveRef"} {
		if _, ok := node[key]; ok {
			return true
		}
	}
	additional, ok := node["additionalProperties"].(map[string]any)
	return isBooleanSchema(node["additionalProperties"], true) || (ok && len(additional) == 0)
}

// alternativeEvaluatesUnfolded reports whether node, a schema in a
// folded alternative, evaluates property names other than those it lists
// in "properties", which the root does not keep.
func alternativeEvaluatesUnfolded(node map[string]any) bool {
	if evaluatesUnhoisted(node) {
		return true
	}
	if patterns, ok := node["patternProperties"].(map[string]any); ok && len(patterns) > 0 {
		return true
	}
	if _, ok := node["dependentSchemas"]; ok {
		return true
	}
	additional, ok := node["additionalProperties"]
	return ok && !isBooleanSchema(additional, false)
}

// compositionEntries counts the "allOf", "anyOf" and "oneOf" branches
// and the "$ref" keywords in value. Root composition merges a schema for
// each one it reaches, so this many merges cover every schema unless a
// reference is expanded again.
func compositionEntries(value any) int {
	count := 0
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			switch key {
			case "allOf", "anyOf", "oneOf":
				if branches, ok := item.([]any); ok {
					count += len(branches)
				}
			case "$ref":
				if _, ok := item.(string); ok {
					count++
				}
			}
			count += compositionEntries(item)
		}
	case []any:
		for _, item := range typed {
			count += compositionEntries(item)
		}
	}
	return count
}

// relocateReferencedRootKeywords keeps references into removed root
// keywords resolvable. Each schema that something still points at, such
// as "#/allOf/0/properties/a", moves into the root definitions under a
// name of its own, and the references are rewritten to point there.
// Anthropic models send a JSON string for an object parameter whose
// reference points below a top-level definition, so the moved schemas
// become top-level definitions instead of staying under the keyword they
// came from.
// Moved content can point into further removed content, which then
// moves as well. A schema inside another moved schema stays inside it,
// so that no anchor is declared twice. A keyword whose referenced
// content cannot stand alone, because it belongs to an embedded
// resource or is not a schema, moves as a whole instead.
func relocateReferencedRootKeywords(inputSchema map[string]any, removed map[string]any) map[string]any {
	original := maps.Clone(inputSchema)
	maps.Copy(original, removed)
	// probe places content found under the removed keywords where its
	// references resolve as they did at the root.
	probe := func(key string, value any) map[string]any {
		placed := map[string]any{key: value}
		if id, ok := inputSchema["$id"]; ok {
			placed["$id"] = id
		}
		return placed
	}
	type target struct {
		tokens []string
		schema any
	}
	targets := map[string]target{}
	// enclosed reports whether a target already holds the one at tokens.
	enclosed := func(tokens []string) bool {
		for end := 1; end < len(tokens); end++ {
			if _, ok := targets[toolschema.EncodePointer(tokens[:end])]; ok {
				return true
			}
		}
		return false
	}
	whole := map[string]bool{}
	queue := []map[string]any{inputSchema}
	for len(queue) > 0 {
		toolschema.RewriteRefs(queue[0], func(tokens []string) ([]string, bool) {
			if len(tokens) == 0 || whole[tokens[0]] {
				return nil, false
			}
			if _, ok := removed[tokens[0]]; !ok {
				return nil, false
			}
			pointer := toolschema.EncodePointer(tokens)
			if _, seen := targets[pointer]; seen {
				return nil, false
			}
			schema, sameResource, ok := toolschema.ResolveLocalRef(original, "#"+pointer)
			if !ok {
				return nil, false
			}
			switch typed := schema.(type) {
			case map[string]any:
				if !sameResource {
					break
				}
				if !enclosed(tokens) {
					queue = append(queue, probe("$defs", map[string]any{"target": typed}))
				}
				targets[pointer] = target{tokens: tokens, schema: typed}
				return nil, false
			case bool:
				targets[pointer] = target{tokens: tokens, schema: typed}
				return nil, false
			}
			whole[tokens[0]] = true
			queue = append(queue, probe(tokens[0], removed[tokens[0]]))
			return nil, false
		})
		queue = queue[1:]
	}
	if len(targets) == 0 && len(whole) == 0 {
		return inputSchema
	}

	container := "$defs"
	if _, hasDefs := inputSchema["$defs"]; !hasDefs {
		if _, hasDefinitions := inputSchema["definitions"].(map[string]any); hasDefinitions {
			container = "definitions"
		}
	}
	definitions := map[string]any{}
	if existing, ok := inputSchema[container].(map[string]any); ok {
		for name, definition := range existing {
			definitions[name] = definition
		}
	}
	define := func(tokens []string, definition any) string {
		parts := make([]string, len(tokens))
		for i, token := range tokens {
			parts[i] = strings.Map(func(r rune) rune {
				if r == '_' || r == '-' || r == '.' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
					return r
				}
				return '_'
			}, strings.TrimPrefix(token, "$"))
		}
		base := "coder_root_" + strings.Join(parts, "_")
		name := base
		for i := 2; ; i++ {
			if _, taken := definitions[name]; !taken {
				break
			}
			name = fmt.Sprintf("%s_%d", base, i)
		}
		definitions[name] = definition
		return name
	}

	wholeNames := map[string]string{}
	for _, key := range slices.Sorted(maps.Keys(whole)) {
		wholeNames[key] = define([]string{key}, map[string]any{key: removed[key]})
	}
	names := map[string]string{}
	for _, pointer := range slices.Sorted(maps.Keys(targets)) {
		moved := targets[pointer]
		if whole[moved.tokens[0]] || enclosed(moved.tokens) {
			continue
		}
		names[pointer] = define(moved.tokens, moved.schema)
	}
	inputSchema[container] = definitions
	return toolschema.RewriteRefs(inputSchema, func(tokens []string) ([]string, bool) {
		if len(tokens) == 0 {
			return nil, false
		}
		if name, ok := wholeNames[tokens[0]]; ok {
			return append([]string{container, name}, tokens...), true
		}
		for end := 1; end <= len(tokens); end++ {
			if name, ok := names[toolschema.EncodePointer(tokens[:end])]; ok {
				return append([]string{container, name}, tokens[end:]...), true
			}
		}
		return nil, false
	})
}

// objectShape collects the properties and required names that root
// composition contributes to the object root.
type objectShape struct {
	properties map[string]any
	required   []string
	// requiredSet holds the names in required.
	requiredSet map[string]struct{}
	// merged holds the schemas merged into the shape, whose keywords
	// beside composition, "properties" and "required" still hold.
	merged []map[string]any
	// impossible reports that a merged schema allows no value.
	impossible bool
	// refs holds the references already merged into the shape. Merging
	// one again adds nothing, so it is skipped.
	refs map[string]struct{}
}

func (s *objectShape) addRequired(names []string) {
	for _, name := range names {
		if _, ok := s.requiredSet[name]; ok {
			continue
		}
		if s.requiredSet == nil {
			s.requiredSet = map[string]struct{}{}
		}
		s.requiredSet[name] = struct{}{}
		s.required = append(s.required, name)
	}
}

// addProperty adds a property that must hold together with any schema
// the shape already has for that name.
func (s *objectShape) addProperty(name string, property any) {
	existing, ok := s.properties[name]
	switch {
	case !ok:
		s.properties[name] = property
	case !reflect.DeepEqual(existing, property):
		s.properties[name] = map[string]any{"allOf": []any{existing, property}}
	}
}

type rootFolder struct {
	root map[string]any
	// visiting holds the references being followed on the current path.
	visiting map[string]struct{}
	// merged counts the schemas merged so far, and maxMerged bounds it.
	merged    int
	maxMerged int
	// ignoreRefSiblings reports that the schema declares a draft before
	// 2019-09, which ignores every keyword beside "$ref".
	ignoreRefSiblings bool
	// opaque reports that the fold left out how a schema evaluates
	// property names, which "unevaluatedProperties" depends on.
	opaque   bool
	patterns patternMatcher
}

// mergeAll merges a schema that must hold in addition to the root.
func (f *rootFolder) mergeAll(shape *objectShape, branch any) {
	if holds, ok := branch.(bool); ok {
		shape.impossible = shape.impossible || !holds
		return
	}
	node, ok := branch.(map[string]any)
	if !ok {
		return
	}
	if f.merged >= f.maxMerged {
		f.opaque = true
		return
	}
	f.merged++
	if ref, ok := node["$ref"].(string); ok && f.ignoreRefSiblings {
		f.mergeRef(shape, ref)
		return
	}
	if _, ok := node["$id"]; ok && toolschema.HasRefs(node) {
		// References in a separate resource resolve against its own
		// "$id", so its content cannot move into the root resource.
		f.opaque = true
		return
	}
	shape.merged = append(shape.merged, node)
	if ref, ok := node["$ref"].(string); ok {
		f.mergeRef(shape, ref)
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		for name, property := range properties {
			shape.addProperty(name, property)
		}
	}
	shape.addRequired(coerceRequiredStrings(node["required"]))
	if branches, ok := node["allOf"].([]any); ok {
		for _, nested := range branches {
			f.mergeAll(shape, nested)
		}
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if branches, ok := node[key].([]any); ok {
			f.mergeAlternatives(shape, branches)
		}
	}
}

func (f *rootFolder) mergeRef(shape *objectShape, ref string) {
	if _, ok := f.visiting[ref]; ok {
		return
	}
	if _, ok := shape.refs[ref]; ok {
		return
	}
	target, sameResource, ok := toolschema.ResolveLocalRef(f.root, ref)
	if !ok {
		f.opaque = true
		return
	}
	if node, ok := target.(map[string]any); ok && !sameResource && toolschema.HasRefs(node) {
		// As in mergeAll, references inside another resource resolve
		// against its own "$id".
		f.opaque = true
		return
	}
	if shape.refs == nil {
		shape.refs = map[string]struct{}{}
	}
	shape.refs[ref] = struct{}{}
	f.visiting[ref] = struct{}{}
	defer delete(f.visiting, ref)
	f.mergeAll(shape, target)
}

// mergeAlternatives merges schemas of which at least one must hold. Each
// property becomes the "anyOf" of the alternatives' schemas for it, and a
// name is required only when every alternative requires it. An
// alternative that does not define a property gives it the schema its
// "patternProperties" or "additionalProperties" does, and an alternative
// that allows no value is left out.
func (f *rootFolder) mergeAlternatives(shape *objectShape, branches []any) {
	alternatives := make([]objectShape, 0, len(branches))
	for _, branch := range branches {
		alternative := objectShape{properties: map[string]any{}}
		f.mergeAll(&alternative, branch)
		if alternative.impossible {
			continue
		}
		f.opaque = f.opaque || slices.ContainsFunc(alternative.merged, alternativeEvaluatesUnfolded)
		alternatives = append(alternatives, alternative)
	}
	if len(alternatives) == 0 {
		shape.impossible = shape.impossible || len(branches) > 0
		return
	}

	names := map[string]struct{}{}
	for _, alternative := range alternatives {
		for name := range alternative.properties {
			names[name] = struct{}{}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		var options []any
		for i := range alternatives {
			property, ok := alternatives[i].properties[name]
			if !ok {
				property, ok = f.undeclaredSchema(&alternatives[i], name)
				if !ok {
					continue
				}
			}
			if slices.ContainsFunc(options, func(option any) bool {
				return reflect.DeepEqual(option, property)
			}) {
				continue
			}
			options = append(options, property)
		}
		if len(options) == 1 {
			shape.addProperty(name, options[0])
		} else {
			shape.addProperty(name, map[string]any{"anyOf": options})
		}
	}
	if restriction := alternativesRestriction(alternatives); restriction != nil {
		shape.merged = append(shape.merged, restriction)
	}

	for _, name := range alternatives[0].required {
		requiredByAll := true
		for _, alternative := range alternatives[1:] {
			if _, ok := alternative.requiredSet[name]; !ok {
				requiredByAll = false
				break
			}
		}
		if requiredByAll {
			shape.addRequired([]string{name})
		}
	}
}

// undeclaredSchema returns the schema that the schemas merged into shape
// give a name none of them lists in "properties": the schemas of the
// "patternProperties" patterns it matches, or else the
// "additionalProperties" schema. It reports false when they forbid the
// name. A schema whose matching patterns are unknown allows any value.
func (f *rootFolder) undeclaredSchema(shape *objectShape, name string) (any, bool) {
	var schemas []any
	for _, node := range shape.merged {
		patterns, _ := node["patternProperties"].(map[string]any)
		matched, known := f.patterns.match(patterns, name)
		if !known {
			continue
		}
		var applied []any
		if len(matched) > 0 {
			for _, pattern := range matched {
				applied = append(applied, patterns[pattern])
			}
		} else if additional, ok := node["additionalProperties"]; ok {
			applied = append(applied, additional)
		}
		for _, schema := range applied {
			switch typed := schema.(type) {
			case bool:
				if !typed {
					return nil, false
				}
			case map[string]any:
				if len(typed) > 0 {
					schemas = append(schemas, typed)
				}
			}
		}
	}
	switch len(schemas) {
	case 0:
		return map[string]any{}, true
	case 1:
		return schemas[0], true
	default:
		return map[string]any{"allOf": schemas}, true
	}
}

// alternativesRestriction returns a schema whose "additionalProperties"
// holds for every name that no alternative lists in "properties", or nil
// when such names are unconstrained or the constraint depends on the name
// through "patternProperties".
func alternativesRestriction(alternatives []objectShape) map[string]any {
	var options []any
	for _, alternative := range alternatives {
		forbidden := false
		var schemas []any
		for _, node := range alternative.merged {
			if patterns, ok := node["patternProperties"].(map[string]any); ok && len(patterns) > 0 {
				return nil
			}
			switch additional := node["additionalProperties"].(type) {
			case bool:
				forbidden = forbidden || !additional
			case map[string]any:
				if len(additional) > 0 {
					schemas = append(schemas, additional)
				}
			}
		}
		var option any
		switch {
		case forbidden:
			continue
		case len(schemas) == 0:
			return nil
		case len(schemas) == 1:
			option = schemas[0]
		default:
			option = map[string]any{"allOf": schemas}
		}
		if !slices.ContainsFunc(options, func(existing any) bool {
			return reflect.DeepEqual(existing, option)
		}) {
			options = append(options, option)
		}
	}
	switch len(options) {
	case 0:
		return map[string]any{"additionalProperties": false}
	case 1:
		return map[string]any{"additionalProperties": options[0]}
	default:
		return map[string]any{"additionalProperties": map[string]any{"anyOf": options}}
	}
}
