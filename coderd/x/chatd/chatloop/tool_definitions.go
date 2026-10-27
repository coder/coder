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

// maxRootFoldWork bounds the work root composition spends on the schemas
// it merges: copying their properties and required names, giving each
// property name the schema of every alternative, and applying
// "additionalProperties" and "patternProperties" to the names. Many
// alternatives that share a large schema through references would
// otherwise take work quadratic in the schema size. Work past the bound
// is skipped where that only drops constraints.
const maxRootFoldWork = 1 << 20

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
// apply at the root as well, the "additionalProperties" of each schema
// still holds for the names the others add, so do their conditionals,
// an object "const" requires its fields and no other name, and an
// alternative that no object satisfies is dropped. A schema declaring a
// draft before 2019-09 ignores every keyword beside "$ref", so only the
// reference is folded there. Removed content that a reference still
// points into moves under the root definitions, and those references
// follow it.
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
		resolver:          toolschema.NewResolver(inputSchema),
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
		if value, ok := inputSchema["const"].(map[string]any); ok {
			folder.mergeAll(&shape, objectConstSchema(value))
		}
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
	restricting := shape.merged
	if !folder.ignoreRefSiblings || inputSchema["$ref"] == nil {
		restricting = append(slices.Clone(restricting), maps.Clone(inputSchema))
	}
	folder.applyAdditionalProperties(&shape, restricting)
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
	folder.closeHoistedPatterns(inputSchema, restricting)
	hoistConditionals(inputSchema, shape.merged, removed)
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
	return strings.Contains(toolschema.Dialect(schema), "json-schema.org/draft-0")
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
// Patterns are tried in sorted order, so the budget a call spends, and
// with it the folded schema, does not vary between runs.
func (m *patternMatcher) match(patterns map[string]any, name string) ([]string, bool) {
	var matched []string
	for _, pattern := range slices.Sorted(maps.Keys(patterns)) {
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

// closeHoistedPatterns makes the "additionalProperties" of each of
// nodes, schemas folded into root, hold for the names that match a
// pattern hoisted from another schema, which no longer counts them as
// additional at the root. It is added to the pattern when the node
// lists no name the pattern matches and has no pattern that could match
// a name it does, so that it holds for every such name in the source.
// Otherwise it is left out there, which only drops a constraint.
func (f *rootFolder) closeHoistedPatterns(root map[string]any, nodes []map[string]any) {
	hoisted, _ := root["patternProperties"].(map[string]any)
	if len(hoisted) == 0 {
		return
	}
	closed := maps.Clone(hoisted)
	for _, node := range nodes {
		additional := node["additionalProperties"]
		if schema, ok := additional.(map[string]any); !isBooleanSchema(additional, false) && (!ok || len(schema) == 0) {
			continue
		}
		own, _ := node["patternProperties"].(map[string]any)
		listed, _ := node["properties"].(map[string]any)
		if !f.spend(len(hoisted) * (1 + len(listed) + len(own))) {
			return
		}
		for _, pattern := range slices.Sorted(maps.Keys(hoisted)) {
			if !disjointFromAll(pattern, own) {
				continue
			}
			only := map[string]any{pattern: hoisted[pattern]}
			if slices.ContainsFunc(slices.Sorted(maps.Keys(listed)), func(name string) bool {
				matched, known := f.patterns.match(only, name)
				return !known || len(matched) > 0
			}) {
				continue
			}
			closed[pattern] = conjoinSchemas(closed[pattern], additional)
		}
	}
	root["patternProperties"] = closed
}

// disjointFromAll reports whether no name can match both pattern and any
// of patterns. It knows only patterns that anchor a literal prefix, such
// as "^x-", and reports false when another pattern is involved.
func disjointFromAll(pattern string, patterns map[string]any) bool {
	prefix, ok := literalPrefix(pattern)
	if !ok && len(patterns) > 0 {
		return false
	}
	for other := range patterns {
		otherPrefix, ok := literalPrefix(other)
		if !ok || strings.HasPrefix(prefix, otherPrefix) || strings.HasPrefix(otherPrefix, prefix) {
			return false
		}
	}
	return true
}

// literalPrefix returns the text a pattern of the form "^" followed by
// characters without regular expression meaning requires names to start
// with, and false for other patterns.
func literalPrefix(pattern string) (string, bool) {
	prefix, ok := strings.CutPrefix(pattern, "^")
	return prefix, ok && !strings.ContainsAny(prefix, `\.+*?()|[]{}^$`)
}

// conditionalKeywords are the keywords of a JSON Schema conditional.
var conditionalKeywords = [...]string{"if", "then", "else"}

// hoistConditionals makes the conditionals of the schemas folded into
// the root hold at the root. The root has room for one, and providers
// reject a root "allOf" that would hold several, so several, the root's
// own among them, become the "allOf" of a "then" whose "if" always holds.
// The root's own conditional keywords then move to removed, which keeps
// the references into them resolving.
func hoistConditionals(root map[string]any, merged []map[string]any, removed map[string]any) {
	var conditionals []map[string]any
	for _, node := range merged {
		if conditional := conditionalOf(node); conditional != nil {
			conditionals = append(conditionals, conditional)
		}
	}
	if len(conditionals) == 0 {
		return
	}
	if own := conditionalOf(root); own != nil {
		conditionals = append([]map[string]any{own}, conditionals...)
	}
	for _, key := range conditionalKeywords {
		if value, ok := root[key]; ok {
			removed[key] = value
			delete(root, key)
		}
	}
	if len(conditionals) == 1 {
		maps.Copy(root, conditionals[0])
		return
	}
	all := make([]any, len(conditionals))
	for i, conditional := range conditionals {
		all[i] = conditional
	}
	root["if"] = map[string]any{}
	root["then"] = map[string]any{"allOf": all}
}

// conditionalOf returns the conditional keywords of node, or nil when
// they constrain nothing, since "then" and "else" apply only beside "if".
func conditionalOf(node map[string]any) map[string]any {
	if _, ok := node["if"]; !ok {
		return nil
	}
	conditional := map[string]any{}
	for _, key := range conditionalKeywords {
		if value, ok := node[key]; ok {
			conditional[key] = value
		}
	}
	if len(conditional) == 1 {
		return nil
	}
	return conditional
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
// An embedded resource that a reference names by URI moves the same way
// and keeps its "$id", so such references need no rewrite.
// Moved content can point into further removed content, which then
// moves as well. A schema inside another moved schema stays inside it,
// so that no anchor is declared twice. A keyword whose referenced
// content cannot stand alone, because a pointer reaches it through an
// embedded resource or it is not a schema, moves as a whole instead.
func relocateReferencedRootKeywords(inputSchema map[string]any, removed map[string]any) map[string]any {
	original := maps.Clone(inputSchema)
	maps.Copy(original, removed)
	resolver := toolschema.NewResolver(original)
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
	var tree pointerTree
	addTarget := func(pointer string, moved target) {
		targets[pointer] = moved
		tree.at(moved.tokens).target = true
	}
	// enclosed reports whether a target already holds the one at tokens.
	enclosed := func(tokens []string) bool {
		node, _ := tree.enclosing(tokens[:len(tokens)-1])
		return node != nil
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
			schema, sameResource, ok := resolver.Resolve("#" + pointer)
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
				addTarget(pointer, target{tokens: tokens, schema: typed})
				return nil, false
			case bool:
				addTarget(pointer, target{tokens: tokens, schema: typed})
				return nil, false
			}
			whole[tokens[0]] = true
			queue = append(queue, probe(tokens[0], removed[tokens[0]]))
			return nil, false
		})
		// An embedded resource keeps its "$id" when it moves, so the
		// references that name it by URI keep resolving without a
		// rewrite. One nested in another resource moves with the
		// outermost one, against which its "$id" may resolve.
		for _, tokens := range resolver.ResourceRefs(queue[0]) {
			if _, ok := removed[tokens[0]]; !ok || whole[tokens[0]] {
				continue
			}
			pointer := toolschema.EncodePointer(tokens)
			if _, seen := targets[pointer]; seen {
				continue
			}
			schema, _, _ := resolver.Resolve("#" + pointer)
			node, ok := schema.(map[string]any)
			if !ok {
				continue
			}
			if !enclosed(tokens) {
				queue = append(queue, probe("$defs", map[string]any{"target": node}))
			}
			addTarget(pointer, target{tokens: tokens, schema: node})
		}
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
	for _, pointer := range slices.Sorted(maps.Keys(targets)) {
		moved := targets[pointer]
		if whole[moved.tokens[0]] || enclosed(moved.tokens) {
			continue
		}
		tree.at(moved.tokens).name = define(moved.tokens, moved.schema)
	}
	inputSchema[container] = definitions
	return toolschema.RewriteRefs(inputSchema, func(tokens []string) ([]string, bool) {
		if len(tokens) == 0 {
			return nil, false
		}
		if name, ok := wholeNames[tokens[0]]; ok {
			return append([]string{container, name}, tokens...), true
		}
		// The shortest target holding the one at tokens is never enclosed,
		// so it was defined.
		if node, end := tree.enclosing(tokens); node != nil {
			return append([]string{container, node.name}, tokens[end:]...), true
		}
		return nil, false
	})
}

// pointerTree holds JSON Pointers by their tokens, so that the pointers
// that are prefixes of another are found in time linear in its length.
type pointerTree struct {
	children map[string]*pointerTree
	// target reports that a moved target ends here, and name is the
	// definition it moved to, once defined.
	target bool
	name   string
}

// at returns the node for tokens, adding the nodes missing on the way.
func (t *pointerTree) at(tokens []string) *pointerTree {
	for _, token := range tokens {
		next, ok := t.children[token]
		if !ok {
			if t.children == nil {
				t.children = map[string]*pointerTree{}
			}
			next = &pointerTree{}
			t.children[token] = next
		}
		t = next
	}
	return t
}

// enclosing returns the node of the shortest target that is a prefix of
// tokens, or tokens itself, and the number of tokens it holds.
func (t *pointerTree) enclosing(tokens []string) (*pointerTree, int) {
	for i, token := range tokens {
		next, ok := t.children[token]
		if !ok {
			return nil, 0
		}
		if next.target {
			return next, i + 1
		}
		t = next
	}
	return nil, 0
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
	case !ok || allowsAny(existing):
		s.properties[name] = property
	case isBooleanSchema(existing, false) || isBooleanSchema(property, false):
		s.properties[name] = false
	case !allowsAny(property) && !reflect.DeepEqual(existing, property):
		s.properties[name] = map[string]any{"allOf": []any{existing, property}}
	}
}

// allowsAny reports whether schema is one that every value satisfies:
// true or {}.
func allowsAny(schema any) bool {
	node, ok := schema.(map[string]any)
	return isBooleanSchema(schema, true) || (ok && len(node) == 0)
}

type rootFolder struct {
	resolver *toolschema.Resolver
	// visiting holds the references being followed on the current path.
	visiting map[string]struct{}
	// merged counts the schemas merged so far, and maxMerged bounds it.
	merged    int
	maxMerged int
	// work counts the work spent so far against maxRootFoldWork.
	work int
	// ignoreRefSiblings reports that the schema declares a draft before
	// 2019-09, which ignores every keyword beside "$ref".
	ignoreRefSiblings bool
	// opaque reports that the fold left out how a schema evaluates
	// property names, which "unevaluatedProperties" depends on.
	opaque bool
	// skipped counts the schemas the fold did not merge where they
	// appear: those it left out, and references already merged or being
	// followed.
	skipped    int
	patterns   patternMatcher
	refScanner toolschema.RefScanner
}

// leaveOut records that the fold left a schema out.
func (f *rootFolder) leaveOut() {
	f.opaque = true
	f.skipped++
}

// spend charges cost against maxRootFoldWork and reports whether the
// work may go ahead. Work it refuses leaves constraints out of the fold.
func (f *rootFolder) spend(cost int) bool {
	f.work += cost
	if f.work > maxRootFoldWork {
		f.leaveOut()
		return false
	}
	return true
}

// keywordEntries counts node's keywords and the entries of those whose
// values are maps or lists, which merging node copies or scans.
func keywordEntries(node map[string]any) int {
	count := len(node)
	for _, value := range node {
		switch typed := value.(type) {
		case map[string]any:
			count += len(typed)
		case []any:
			count += len(typed)
		case []string:
			count += len(typed)
		}
	}
	return count
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
	if f.merged >= f.maxMerged || !f.spend(keywordEntries(node)) {
		f.leaveOut()
		return
	}
	f.merged++
	if ref, ok := node["$ref"].(string); ok && f.ignoreRefSiblings {
		f.mergeRef(shape, ref)
		return
	}
	if _, ok := node["$id"]; ok && f.refScanner.HasRefs(node) {
		// References in a separate resource resolve against its own
		// "$id", so its content cannot move into the root resource.
		f.leaveOut()
		return
	}
	if excludesObjects(node) {
		// The root accepts only objects.
		shape.impossible = true
		return
	}
	shape.merged = append(shape.merged, node)
	start, skipped := len(shape.merged)-1, f.skipped
	if value, ok := node["const"].(map[string]any); ok {
		f.mergeAll(shape, objectConstSchema(value))
	}
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
	if f.skipped == skipped && !f.ignoreRefSiblings {
		if restriction := unevaluatedRestriction(node, shape.merged[start:]); restriction != nil {
			shape.merged = append(shape.merged, restriction)
		}
	}
}

// unevaluatedRestriction returns a schema whose "additionalProperties"
// is the "unevaluatedProperties" of node, for the names that node and
// the schemas merged from it, subtree, list or match. It holds where
// node's does in the source when those schemas evaluate names only
// through "properties" and "patternProperties". It returns nil
// otherwise, or when node's constrains nothing.
func unevaluatedRestriction(node map[string]any, subtree []map[string]any) map[string]any {
	unevaluated, ok := node["unevaluatedProperties"]
	if schema, isSchema := unevaluated.(map[string]any); !ok || (!isBooleanSchema(unevaluated, false) && (!isSchema || len(schema) == 0)) {
		return nil
	}
	listed := map[string]any{}
	patterns := map[string]any{}
	for i, merged := range subtree {
		for _, key := range evaluatingKeywords {
			if _, ok := merged[key]; ok && (key != "unevaluatedProperties" || i > 0) {
				return nil
			}
		}
		properties, _ := merged["properties"].(map[string]any)
		for name := range properties {
			listed[name] = true
		}
		matched, _ := merged["patternProperties"].(map[string]any)
		for pattern := range matched {
			patterns[pattern] = true
		}
	}
	restriction := map[string]any{"properties": listed, "additionalProperties": unevaluated}
	if len(patterns) > 0 {
		restriction["patternProperties"] = patterns
	}
	return restriction
}

// evaluatingKeywords are the keywords beside "properties" and
// "patternProperties" through which a schema can evaluate property
// names.
var evaluatingKeywords = [...]string{
	"additionalProperties", "unevaluatedProperties", "anyOf", "oneOf", "if",
	"dependentSchemas", "dependencies", "$dynamicRef", "$recursiveRef",
}

func (f *rootFolder) mergeRef(shape *objectShape, ref string) {
	if _, ok := f.visiting[ref]; ok {
		f.skipped++
		return
	}
	if _, ok := shape.refs[ref]; ok {
		f.skipped++
		return
	}
	target, sameResource, ok := f.resolver.Resolve(ref)
	if !ok {
		f.leaveOut()
		return
	}
	if node, ok := target.(map[string]any); ok && !sameResource && f.refScanner.HasRefs(node) {
		// As in mergeAll, references inside another resource resolve
		// against its own "$id".
		f.leaveOut()
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
// property, and each name an alternative requires, becomes the "anyOf" of
// the alternatives' schemas for it, and a name is required only when
// every alternative requires it. An alternative that does not define a
// property gives it the schema its "patternProperties" or
// "additionalProperties" does, and an alternative that allows no value is
// left out. When the work budget runs out, the names left are not merged,
// and neither is the restriction of names no alternative lists, which
// could otherwise forbid them.
func (f *rootFolder) mergeAlternatives(shape *objectShape, branches []any) {
	alternatives := make([]objectShape, 0, len(branches))
	for _, branch := range branches {
		alternative := objectShape{properties: map[string]any{}}
		f.mergeAll(&alternative, branch)
		f.applyAdditionalProperties(&alternative, alternative.merged)
		f.applyPatternProperties(&alternative)
		if alternative.impossible || f.requiresForbidden(&alternative) {
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
		for _, name := range alternative.required {
			names[name] = struct{}{}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		options, ok := f.alternativeOptions(alternatives, name)
		if !ok {
			return
		}
		switch len(options) {
		case 0:
			// Every alternative forbids the name, so no argument holds it.
			delete(names, name)
		case 1:
			shape.addProperty(name, options[0])
		default:
			shape.addProperty(name, map[string]any{"anyOf": options})
		}
	}
	if restriction := f.alternativesRestriction(alternatives); restriction != nil {
		listed := make(map[string]any, len(names))
		for name := range names {
			listed[name] = map[string]any{}
		}
		restriction["properties"] = listed
		shape.merged = append(shape.merged, restriction)
	}

	for _, name := range alternatives[0].required {
		if !f.spend(len(alternatives)) {
			return
		}
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

// alternativeOptions returns the distinct schemas that alternatives give
// name, or false when the work budget runs out.
func (f *rootFolder) alternativeOptions(alternatives []objectShape, name string) ([]any, bool) {
	var options []any
	for i := range alternatives {
		if !f.spend(1 + len(options) + len(alternatives[i].merged)) {
			return nil, false
		}
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
	return options, true
}

// requiresForbidden reports whether shape, an alternative, requires a
// name that it allows no value for, so that no object satisfies it. It
// reports false when the work budget runs out.
func (f *rootFolder) requiresForbidden(shape *objectShape) bool {
	if !f.spend(len(shape.required) * (1 + len(shape.merged))) {
		return false
	}
	for _, name := range shape.required {
		property, ok := shape.properties[name]
		if !ok {
			property, ok = f.undeclaredSchema(shape, name)
		}
		if !ok || isBooleanSchema(property, false) {
			return true
		}
	}
	return false
}

// applyAdditionalProperties makes the "additionalProperties" of each of
// nodes hold for the properties of shape that the node neither lists in
// "properties" nor matches with "patternProperties", as it does in the
// source. A name it forbids leaves the shape, which becomes impossible
// when the name is required.
func (f *rootFolder) applyAdditionalProperties(shape *objectShape, nodes []map[string]any) {
	for _, node := range nodes {
		additional := node["additionalProperties"]
		forbids := isBooleanSchema(additional, false)
		if schema, ok := additional.(map[string]any); !forbids && (!ok || len(schema) == 0) {
			continue
		}
		if !f.spend(len(shape.properties)) {
			return
		}
		listed, _ := node["properties"].(map[string]any)
		patterns, _ := node["patternProperties"].(map[string]any)
		for _, name := range slices.Sorted(maps.Keys(shape.properties)) {
			if _, ok := listed[name]; ok {
				continue
			}
			if matched, known := f.patterns.match(patterns, name); !known || len(matched) > 0 {
				continue
			}
			if !forbids {
				shape.properties[name] = conjoinSchemas(shape.properties[name], additional)
				continue
			}
			delete(shape.properties, name)
			if _, required := shape.requiredSet[name]; required {
				shape.impossible = true
			}
		}
	}
}

// applyPatternProperties makes the "patternProperties" of the schemas
// merged into shape, an alternative, hold for the properties it lists
// whose names they match, as they do in the source, since only the
// properties of an alternative reach the root. A name a pattern forbids
// leaves the shape, which becomes impossible when the name is required.
func (f *rootFolder) applyPatternProperties(shape *objectShape) {
	for _, node := range shape.merged {
		patterns, _ := node["patternProperties"].(map[string]any)
		if len(patterns) == 0 {
			continue
		}
		if !f.spend(len(shape.properties)) {
			return
		}
		for _, name := range slices.Sorted(maps.Keys(shape.properties)) {
			matched, known := f.patterns.match(patterns, name)
			if !known {
				continue
			}
			property := shape.properties[name]
			for _, pattern := range matched {
				property = conjoinSchemas(property, patterns[pattern])
			}
			if !isBooleanSchema(property, false) {
				shape.properties[name] = property
				continue
			}
			delete(shape.properties, name)
			if _, required := shape.requiredSet[name]; required {
				shape.impossible = true
			}
		}
	}
}

// objectConstSchema returns the schema of the one object that equals
// value: each of its names with that value, and no other name.
func objectConstSchema(value map[string]any) map[string]any {
	properties := make(map[string]any, len(value))
	for name, field := range value {
		properties[name] = map[string]any{"const": field}
	}
	return map[string]any{
		"properties":           properties,
		"required":             slices.Sorted(maps.Keys(value)),
		"additionalProperties": false,
	}
}

// excludesObjects reports whether node allows no object, through a
// "type", "const" or "enum" that names none.
func excludesObjects(node map[string]any) bool {
	if kind, ok := node["type"].(string); ok && kind != "object" {
		return true
	}
	if value, ok := node["const"]; ok {
		if _, isObject := value.(map[string]any); !isObject {
			return true
		}
	}
	values, ok := node["enum"].([]any)
	return ok && !slices.ContainsFunc(values, func(value any) bool {
		_, isObject := value.(map[string]any)
		return isObject
	})
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
// when such names are unconstrained, the constraint depends on the name
// through "patternProperties", or the work budget runs out.
func (f *rootFolder) alternativesRestriction(alternatives []objectShape) map[string]any {
	var options []any
	for _, alternative := range alternatives {
		if !f.spend(1 + len(alternative.merged) + len(options)) {
			return nil
		}
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
