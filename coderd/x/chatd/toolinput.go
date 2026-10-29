package chatd

import (
	"encoding/json"

	"charm.land/fantasy"
	"github.com/google/jsonschema-go/jsonschema"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/toolschema"
)

// partitionInvalidToolCalls rejects malformed JSON and invalid builtin inputs
// before pre_tool_use or execution. Synthetic tool errors let the model correct
// its arguments without failing the turn. allowedIndexes maps allowed calls
// back to the input order without relying on duplicate-prone IDs.
func partitionInvalidToolCalls(
	prepared turnEnvironment,
	toolCalls []fantasy.ToolCallContent,
) (allowed []fantasy.ToolCallContent, allowedIndexes []int, rejected []fantasy.ToolResultContent) {
	for i, toolCall := range toolCalls {
		if !json.Valid([]byte(toolCall.Input)) {
			rejected = append(rejected, malformedToolResult(toolCall))
			continue
		}
		if err := validateBuiltinToolInput(prepared, toolCall.ToolName, []byte(toolCall.Input)); err != nil {
			rejected = append(rejected, invalidToolInputResult(toolCall, err))
			continue
		}
		allowed = append(allowed, toolCall)
		allowedIndexes = append(allowedIndexes, i)
	}
	return allowed, allowedIndexes, rejected
}

// validateOverriddenToolInputs rechecks the inputs a pre_tool_use consumer
// replaced. The model cannot fix an invalid override, so the turn fails
// closed instead of executing it.
func validateOverriddenToolInputs(prepared turnEnvironment, preflight chathooks.PreToolUseExecutionResult) error {
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
func validateBuiltinToolInput(prepared turnEnvironment, toolName string, input []byte) error {
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
		if err := toolschema.ValidateUnambiguous(info.Parameters, input); err != nil {
			return err
		}
		schemaJSON, err := json.Marshal(info.Parameters)
		if err != nil {
			return xerrors.Errorf("marshal tool schema: %w", err)
		}
		schema := jsonschema.Schema{Type: "object", Required: info.Required}
		if err := json.Unmarshal(schemaJSON, &schema.Properties); err != nil {
			return xerrors.Errorf("decode tool schema: %w", err)
		}
		normalizeToolMapSchemas(&schema)
		compiled, err := schema.Resolve(nil)
		if err != nil {
			return xerrors.Errorf("compile tool schema: %w", err)
		}
		var value any
		if err := json.Unmarshal(input, &value); err != nil {
			return xerrors.Errorf("decode tool input: %w", err)
		}
		return compiled.Validate(value)
	}
	return nil
}

// normalizeToolMapSchemas translates Fantasy's map[string]T wildcard property
// into JSON Schema's additionalProperties, leaving the tool metadata intact.
func normalizeToolMapSchemas(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	for _, property := range schema.Properties {
		normalizeToolMapSchemas(property)
	}
	normalizeToolMapSchemas(schema.Items)
	normalizeToolMapSchemas(schema.AdditionalProperties)
	for _, branches := range [][]*jsonschema.Schema{schema.AnyOf, schema.OneOf, schema.AllOf} {
		for _, branch := range branches {
			normalizeToolMapSchemas(branch)
		}
	}
	if value, wildcard := schema.Properties["*"]; wildcard && len(schema.Properties) == 1 && schema.AdditionalProperties == nil {
		schema.AdditionalProperties = value
		schema.Properties = nil
	}
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

func invalidToolInputResult(toolCall fantasy.ToolCallContent, err error) fantasy.ToolResultContent {
	message := "This tool call was not executed because its input is invalid: " + err.Error() +
		". Retry with a JSON object matching the tool schema, including all required properties and their declared types. Use the exact property names, each key used once."
	return fantasy.ToolResultContent{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
		Result: fantasy.ToolResultOutputContentError{
			Error: xerrors.New(message),
		},
	}
}
