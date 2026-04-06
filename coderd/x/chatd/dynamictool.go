package chatd

import (
	"context"
	"encoding/json"

	"charm.land/fantasy"

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
	opts        fantasy.ProviderOptions
}

// dynamicToolsFromSDK converts codersdk.DynamicTool definitions
// into fantasy.AgentTool implementations for inclusion in the
// LLM tool list.
func dynamicToolsFromSDK(tools []codersdk.DynamicTool) []fantasy.AgentTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]fantasy.AgentTool, 0, len(tools))
	for _, t := range tools {
		dt := &dynamicTool{
			name:        t.Name,
			description: t.Description,
		}
		// Parse the JSON schema parameters into the map format
		// that fantasy.ToolInfo expects.
		if len(t.Parameters) > 0 {
			var schema map[string]any
			if err := json.Unmarshal(t.Parameters, &schema); err == nil {
				// Extract "properties" and "required" from the
				// JSON Schema object to match ToolInfo's format.
				if props, ok := schema["properties"].(map[string]any); ok {
					dt.parameters = props
				}
				if req, ok := schema["required"].([]any); ok {
					for _, r := range req {
						if s, ok := r.(string); ok {
							dt.required = append(dt.required, s)
						}
					}
				}
			}
		}
		result = append(result, dt)
	}
	return result
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
