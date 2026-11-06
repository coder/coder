package chatloop

import (
	"charm.land/fantasy"
	"charm.land/fantasy/schema"
)

// ToolInputSchemaProvider is an optional interface that fantasy.AgentTool
// implementations may satisfy to supply an explicit input schema, preserving
// top-level metadata such as "$schema", "$defs", or custom validation
// constraints that would otherwise be discarded when synthesizing a schema
// purely from ToolInfo.Parameters and ToolInfo.Required.
type ToolInputSchemaProvider interface {
	InputSchema() map[string]any
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

		var inputSchema map[string]any
		if sp, ok := tool.(ToolInputSchemaProvider); ok {
			inputSchema = sp.InputSchema()
		}

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
		} else {
			inputSchema = cloneSchemaMap(inputSchema)
			if inputSchema["properties"] == nil {
				inputSchema["properties"] = map[string]any{}
			}
			if _, ok := inputSchema["type"]; !ok {
				inputSchema["type"] = "object"
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

func cloneSchemaMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	clone := make(map[string]any, len(m))
	for k, v := range m {
		clone[k] = cloneSchemaValue(v)
	}
	return clone
}

func cloneSchemaValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cloneSchemaMap(val)
	case []any:
		s := make([]any, len(val))
		for i, item := range val {
			s[i] = cloneSchemaValue(item)
		}
		return s
	default:
		return val
	}
}
