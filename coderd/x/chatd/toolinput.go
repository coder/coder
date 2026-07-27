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

// validateBuiltinToolInput only guards builtin tools, whose input coderd
// decodes itself. Dynamic calls are executed by the client and MCP calls by
// their own server, and a dynamic tool cannot shadow a builtin name.
func validateBuiltinToolInput(prepared generationPrepared, toolName string, input []byte) error {
	// Execution resolves a deprecated alias to its canonical tool, so
	// skipping that here would let the old name bypass validation.
	if canonical, aliased := subagentToolNameAliases[toolName]; aliased {
		toolName = canonical
	}
	if !prepared.BuiltinToolNames[toolName] {
		return nil
	}
	for _, tool := range prepared.Tools {
		info := tool.Info()
		if info.Name != toolName {
			continue
		}
		return toolschema.ValidateUnambiguous(info.Parameters, input)
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
