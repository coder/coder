package chatd

import (
	"encoding/json"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/toolschema"
)

// partitionAmbiguousToolCalls separates the calls a consumer must not be asked
// to decide from the rest, returning synthetic error results for them. Callers
// reject before pre_tool_use so a hook consumer is never asked to authorize
// bytes whose meaning depends on which reader resolves them, and so input that
// cannot be carried in a hook payload fails as a retryable tool error instead
// of a dispatch failure.
func partitionAmbiguousToolCalls(
	prepared generationPrepared,
	toolCalls []fantasy.ToolCallContent,
) ([]fantasy.ToolCallContent, []fantasy.ToolResultContent) {
	var (
		allowed  []fantasy.ToolCallContent
		rejected []fantasy.ToolResultContent
	)
	for _, toolCall := range toolCalls {
		if !json.Valid([]byte(toolCall.Input)) {
			rejected = append(rejected, malformedToolResult(toolCall))
			continue
		}
		if err := validateBuiltinToolInput(prepared, toolCall.ToolName, []byte(toolCall.Input)); err != nil {
			rejected = append(rejected, ambiguousToolResult(toolCall, err))
			continue
		}
		allowed = append(allowed, toolCall)
	}
	return allowed, rejected
}

// validateOverriddenToolInputs rechecks the inputs a pre_tool_use consumer
// replaced. The model cannot fix an ambiguous override, so the turn fails
// closed instead of executing it.
func validateOverriddenToolInputs(prepared generationPrepared, preflight chathooks.PreToolUseExecutionResult) error {
	for _, toolCall := range preflight.Allowed {
		if _, overridden := preflight.Overrides[toolCall.ToolCallID]; !overridden {
			continue
		}
		if err := validateBuiltinToolInput(prepared, toolCall.ToolName, []byte(toolCall.Input)); err != nil {
			return xerrors.Errorf("hook input override for tool %s: %w", toolCall.ToolName, err)
		}
	}
	return nil
}

// validateBuiltinToolInput only guards tools whose input coderd decodes
// itself: builtin tools and provider tools with a local runner. Dynamic
// calls are executed by the client and MCP calls by their own server, and
// a dynamic tool cannot shadow a builtin name.
func validateBuiltinToolInput(prepared generationPrepared, toolName string, input []byte) error {
	// Execution resolves a deprecated alias to its canonical tool, so
	// skipping that here would let the old name bypass validation.
	if canonical, aliased := subagentToolNameAliases[toolName]; aliased {
		toolName = canonical
	}
	if prepared.BuiltinToolNames[toolName] {
		for _, tool := range prepared.Tools {
			info := tool.Info()
			if info.Name != toolName {
				continue
			}
			return toolschema.ValidateUnambiguous(info.Parameters, input)
		}
		return nil
	}
	for _, providerTool := range prepared.ProviderTools {
		runner := providerTool.Runner
		if runner == nil || runner.Info().Name != toolName {
			continue
		}
		return toolschema.ValidateUnambiguous(localProviderToolProperties(runner), input)
	}
	return nil
}

// computerUseProperties lists every input key the computer-use runners
// decode: the anthropic parser's flat action struct and the OpenAI batch
// envelope, including the keys of each batched action. The provider
// defines the tool schema server-side, so the runner advertises no local
// parameters and the ambiguity guard needs its own property set; without
// one, a case variant such as ACTION is invisible to a pre_tool_use
// consumer reading case-sensitively while encoding/json folds it into the
// executed action.
var computerUseProperties = map[string]any{
	"action":           nil,
	"coordinate":       nil,
	"start_coordinate": nil,
	"text":             nil,
	"scroll_direction": nil,
	"scroll_amount":    nil,
	"duration":         nil,
	"region":           nil,
	"call_id":          nil,
	"actions": map[string]any{
		"items": map[string]any{
			"properties": map[string]any{
				"type":     nil,
				"button":   nil,
				"keys":     nil,
				"text":     nil,
				"x":        nil,
				"y":        nil,
				"scroll_x": nil,
				"scroll_y": nil,
				"path": map[string]any{
					"items": map[string]any{
						"properties": map[string]any{
							"x": nil,
							"y": nil,
						},
					},
				},
			},
		},
	},
}

// localProviderToolProperties returns the property set to validate a
// provider tool executed by a local runner. The runner's advertised
// parameters win when present; the computer-use runner advertises none, so
// it falls back to the keys its decoders read.
func localProviderToolProperties(runner fantasy.AgentTool) map[string]any {
	info := runner.Info()
	if len(info.Parameters) > 0 {
		return info.Parameters
	}
	if info.Name == "computer" {
		return computerUseProperties
	}
	return nil
}

// malformedToolResult reports input the tool decoder would reject anyway. It
// is produced here because a hook payload carries the input as JSON, so
// invalid bytes would otherwise surface as a dispatch failure and end the
// turn instead of letting the model correct the call.
func malformedToolResult(toolCall fantasy.ToolCallContent) fantasy.ToolResultContent {
	return fantasy.ToolResultContent{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
		Result: fantasy.ToolResultOutputContentError{
			Error: xerrors.New("This tool call was not executed because its input is not valid JSON. Retry with a well-formed JSON object matching the tool schema."),
		},
	}
}

func ambiguousToolResult(toolCall fantasy.ToolCallContent, err error) fantasy.ToolResultContent {
	message := "This tool call was not executed because its input is ambiguous: " + err.Error() +
		". Retry with the exact property names from the tool schema, each key used once."
	return fantasy.ToolResultContent{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
		Result: fantasy.ToolResultOutputContentError{
			Error: xerrors.New(message),
		},
	}
}
