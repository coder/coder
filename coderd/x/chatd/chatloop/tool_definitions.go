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

// fullInputSchema returns a defensively normalized copy of the tool's
// full input schema, or nil when the tool does not provide one and the
// caller must reconstruct a schema from Info().
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
	// MCP requires object roots, so a missing "type" on a root without
	// combinators is an omission rather than intent.
	if _, ok := inputSchema["type"]; !ok && !hasCombinatorRoot(inputSchema) {
		inputSchema["type"] = "object"
	}
	if inputSchema["type"] == "object" {
		if properties, _ := inputSchema["properties"].(map[string]any); properties == nil {
			// A nil map serializes "properties" to null, which some
			// providers reject as an invalid schema.
			inputSchema["properties"] = map[string]any{}
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

func hasCombinatorRoot(inputSchema map[string]any) bool {
	for _, key := range []string{"$ref", "anyOf", "oneOf", "allOf"} {
		if _, ok := inputSchema[key]; ok {
			return true
		}
	}
	return false
}
