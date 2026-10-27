package chatloop

import (
	"fmt"
	"reflect"
	"slices"
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

		inputSchema := fullInputSchema(tool)
		if inputSchema == nil {
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
		inputSchema = enforceObjectRoot(toolschema.Normalize(inputSchema))
		prepared = append(prepared, fantasy.FunctionTool{
			Name:            info.Name,
			Description:     info.Description,
			InputSchema:     inputSchema,
			ProviderOptions: tool.ProviderOptions(),
		})
	}
	for _, pt := range providerTools {
		prepared = append(prepared, pt.Definition)
	}
	return prepared
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

// maxRootMergeDepth bounds how deeply root composition is followed, so
// a reference cycle or a long chain cannot expand without limit.
const maxRootMergeDepth = 32

// enforceObjectRoot returns the tool input schema with the plain object
// root MCP mandates and every provider accepts: "type" is "object" and
// "properties" is never null. It runs after toolschema.Normalize, which
// can turn a root type array into a root "anyOf". Root
// keywords providers reject are removed, but their meaning is folded
// into the root first: properties and required names from "allOf" and
// "$ref" are merged in, and those from "anyOf" and "oneOf" are merged
// with a name required only when every alternative requires it. Removed
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

func foldRootComposition(inputSchema map[string]any, banned []string) map[string]any {
	folder := rootFolder{root: inputSchema, visiting: map[string]struct{}{}}
	shape := objectShape{properties: map[string]any{}}
	if properties, ok := inputSchema["properties"].(map[string]any); ok {
		for name, property := range properties {
			shape.properties[name] = property
		}
	}
	shape.addRequired(coerceRequiredStrings(inputSchema["required"]))
	if ref, ok := inputSchema["$ref"].(string); ok {
		folder.mergeRef(&shape, ref, 0)
	}
	if branches, ok := inputSchema["allOf"].([]any); ok {
		for _, branch := range branches {
			folder.mergeAll(&shape, branch, 0)
		}
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if branches, ok := inputSchema[key].([]any); ok {
			folder.mergeAlternatives(&shape, branches, 0)
		}
	}

	removed := make(map[string]any, len(banned))
	for _, key := range banned {
		removed[key] = inputSchema[key]
		delete(inputSchema, key)
	}
	inputSchema["properties"] = shape.properties
	required := make([]string, 0, len(shape.required))
	for _, name := range shape.required {
		if _, ok := shape.properties[name]; ok {
			required = append(required, name)
		}
	}
	if len(required) > 0 {
		inputSchema["required"] = required
	} else {
		delete(inputSchema, "required")
	}
	return relocateReferencedRootKeywords(inputSchema, removed)
}

// relocateReferencedRootKeywords keeps references into removed root
// keywords resolvable. A removed keyword that something still points
// into, such as "#/allOf/0/properties/a", moves into the root
// definitions as a schema holding just that keyword, and the
// references are rewritten to point there.
func relocateReferencedRootKeywords(inputSchema map[string]any, removed map[string]any) map[string]any {
	referenced := map[string]bool{}
	toolschema.RewriteRefs(inputSchema, func(tokens []string) ([]string, bool) {
		if len(tokens) > 0 {
			if _, ok := removed[tokens[0]]; ok {
				referenced[tokens[0]] = true
			}
		}
		return nil, false
	})
	if len(referenced) == 0 {
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
	names := make(map[string]string, len(referenced))
	for _, key := range bannedRootKeys {
		if !referenced[key] {
			continue
		}
		name := "coder_root_" + strings.TrimPrefix(key, "$")
		for i := 2; ; i++ {
			if _, taken := definitions[name]; !taken {
				break
			}
			name = fmt.Sprintf("coder_root_%s_%d", strings.TrimPrefix(key, "$"), i)
		}
		definitions[name] = map[string]any{key: removed[key]}
		names[key] = name
	}
	inputSchema[container] = definitions
	return toolschema.RewriteRefs(inputSchema, func(tokens []string) ([]string, bool) {
		if len(tokens) == 0 {
			return nil, false
		}
		name, ok := names[tokens[0]]
		if !ok {
			return nil, false
		}
		return append([]string{container, name}, tokens...), true
	})
}

// objectShape collects the properties and required names that root
// composition contributes to the object root.
type objectShape struct {
	properties map[string]any
	required   []string
	// closed reports that names outside properties are forbidden.
	closed bool
}

func (s *objectShape) addRequired(names []string) {
	for _, name := range names {
		if !slices.Contains(s.required, name) {
			s.required = append(s.required, name)
		}
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
}

// mergeAll merges a schema that must hold in addition to the root.
func (f *rootFolder) mergeAll(shape *objectShape, branch any, depth int) {
	node, ok := branch.(map[string]any)
	if !ok || depth > maxRootMergeDepth {
		return
	}
	if _, ok := node["$id"]; ok && toolschema.HasRefs(node) {
		// References in a separate resource resolve against its own
		// "$id", so its content cannot move into the root resource.
		return
	}
	if additional, ok := node["additionalProperties"].(bool); ok && !additional {
		if _, hasPatterns := node["patternProperties"]; !hasPatterns {
			shape.closed = true
		}
	}
	if ref, ok := node["$ref"].(string); ok {
		f.mergeRef(shape, ref, depth)
	}
	if properties, ok := node["properties"].(map[string]any); ok {
		for name, property := range properties {
			shape.addProperty(name, property)
		}
	}
	shape.addRequired(coerceRequiredStrings(node["required"]))
	if branches, ok := node["allOf"].([]any); ok {
		for _, nested := range branches {
			f.mergeAll(shape, nested, depth+1)
		}
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if branches, ok := node[key].([]any); ok {
			f.mergeAlternatives(shape, branches, depth+1)
		}
	}
}

func (f *rootFolder) mergeRef(shape *objectShape, ref string, depth int) {
	if _, ok := f.visiting[ref]; ok {
		return
	}
	target, sameResource, ok := toolschema.ResolveLocalRef(f.root, ref)
	if !ok {
		return
	}
	if node, ok := target.(map[string]any); ok && !sameResource && toolschema.HasRefs(node) {
		// As in mergeAll, references inside another resource resolve
		// against its own "$id".
		return
	}
	f.visiting[ref] = struct{}{}
	defer delete(f.visiting, ref)
	f.mergeAll(shape, target, depth+1)
}

// mergeAlternatives merges schemas of which at least one must hold. Each
// property becomes the "anyOf" of the alternatives' schemas for it, and a
// name is required only when every alternative requires it. An
// alternative that does not define a property allows any value for it,
// unless it forbids additional properties.
func (f *rootFolder) mergeAlternatives(shape *objectShape, branches []any, depth int) {
	if len(branches) == 0 {
		return
	}
	alternatives := make([]objectShape, len(branches))
	for i, branch := range branches {
		alternatives[i] = objectShape{properties: map[string]any{}}
		f.mergeAll(&alternatives[i], branch, depth+1)
	}

	var names []string
	for _, alternative := range alternatives {
		for name := range alternative.properties {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	for _, name := range names {
		var options []any
		for _, alternative := range alternatives {
			property, ok := alternative.properties[name]
			if !ok {
				if alternative.closed {
					continue
				}
				property = map[string]any{}
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

	for _, name := range alternatives[0].required {
		requiredByAll := true
		for _, alternative := range alternatives[1:] {
			if !slices.Contains(alternative.required, name) {
				requiredByAll = false
				break
			}
		}
		if requiredByAll {
			shape.addRequired([]string{name})
		}
	}
}
