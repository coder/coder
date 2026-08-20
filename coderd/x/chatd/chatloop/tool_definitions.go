package chatloop

import (
	"charm.land/fantasy"
	"charm.land/fantasy/schema"
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
		schema.Normalize(inputSchema)
		enforceObjectRoot(inputSchema)
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

// fullInputSchema returns a copy of the tool's full input schema, or
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

// enforceObjectRoot forces a tool input schema into the plain object
// root shape MCP mandates and every provider accepts: banned root
// keywords are dropped, "type" becomes "object" (schema.Normalize can
// rewrite a root type array into a root "anyOf", so this must run
// after it), and "properties" is materialized so it never serializes
// to null. Nested combinators, $defs, additionalProperties, and root
// descriptions all survive, so spec-compliant schemas pass unchanged.
func enforceObjectRoot(inputSchema map[string]any) {
	stripped := false
	for _, key := range bannedRootKeys {
		if _, ok := inputSchema[key]; ok {
			delete(inputSchema, key)
			stripped = true
		}
	}
	if inputSchema["type"] != "object" {
		inputSchema["type"] = "object"
	}
	properties, _ := inputSchema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		inputSchema["properties"] = properties
	}
	if !stripped {
		return
	}
	// Required entries whose property schemas lived inside a dropped
	// keyword (e.g. an allOf branch) would dangle; filter them out.
	if required, ok := inputSchema["required"].([]string); ok {
		filtered := make([]string, 0, len(required))
		for _, name := range required {
			if _, ok := properties[name]; ok {
				filtered = append(filtered, name)
			}
		}
		if len(filtered) > 0 {
			inputSchema["required"] = filtered
		} else {
			delete(inputSchema, "required")
		}
	}
}
