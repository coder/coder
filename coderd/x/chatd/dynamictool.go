package chatd

import (
	"context"
	"encoding/json"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// dynamicTool wraps a codersdk.DynamicTool as a fantasy.AgentTool.
// These tools are presented to the LLM but never executed by the
// chatloop — when the LLM calls one, the chatloop exits with
// requires_action status and the client handles execution.
// The Run method should never be called; it returns an error if
// it is, as a safety net.
type dynamicTool struct {
	name        string
	description string
	parameters  map[string]any
	required    []string
	rawSchema   map[string]any
	opts        fantasy.ProviderOptions
}

// dynamicToolsFromSDK converts codersdk.DynamicTool definitions
// into fantasy.AgentTool implementations for inclusion in the LLM
// tool list.
func dynamicToolsFromSDK(logger slog.Logger, tools []codersdk.DynamicTool) []fantasy.AgentTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]fantasy.AgentTool, 0, len(tools))
	for _, t := range tools {
		dt := &dynamicTool{
			name:        t.Name,
			description: t.Description,
		}
		// InputSchema is a full JSON Schema object stored as
		// json.RawMessage. Extract the "properties" and
		// "required" fields that fantasy.ToolInfo expects, and
		// preserve the raw schema map for ToolInputSchemaProvider.
		if len(t.InputSchema) > 0 {
			var rawMap map[string]any
			if err := json.Unmarshal(t.InputSchema, &rawMap); err != nil {
				// Defensive: present the tool with no parameter
				// constraints rather than failing. The LLM may
				// hallucinate argument shapes, but the tool will
				// still appear in the tool list.
				logger.Warn(context.Background(), "failed to parse dynamic tool input schema",
					slog.F("tool_name", t.Name),
					slog.Error(err))
			} else {
				dt.rawSchema = rawMap
				if props, ok := rawMap["properties"].(map[string]any); ok {
					dt.parameters = props
				}
				if req, ok := rawMap["required"].([]any); ok {
					for _, r := range req {
						if str, ok := r.(string); ok {
							dt.required = append(dt.required, str)
						}
					}
				}
			}
		}
		result = append(result, dt)
	}
	return result
}

func (t *dynamicTool) InputSchema() map[string]any {
	if t.rawSchema == nil {
		return nil
	}
	clone := make(map[string]any, len(t.rawSchema))
	for k, v := range t.rawSchema {
		clone[k] = v
	}
	return clone
}

func (t *dynamicTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        t.name,
		Description: t.description,
		Parameters:  t.parameters,
		Required:    t.required,
	}
}

func (*dynamicTool) Run(_ context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
	// Dynamic tools are never executed by the chatloop. If this
	// method is called, it indicates a bug in the chatloop's
	// dynamic tool detection logic.
	return fantasy.NewTextErrorResponse(
		"dynamic tool called in chatloop — this is a bug; " +
			"dynamic tools should be handled by the client",
	), nil
}

func (t *dynamicTool) ProviderOptions() fantasy.ProviderOptions {
	return t.opts
}

func (t *dynamicTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.opts = opts
}
