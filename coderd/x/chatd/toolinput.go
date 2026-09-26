package chatd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"

	"charm.land/fantasy"
	"charm.land/fantasy/schema"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/toolschema"
)

// partitionAmbiguousToolCalls separates the calls a consumer must not be asked
// to decide from the rest, returning synthetic error results for them. Callers
// reject before pre_tool_use so a hook consumer is never asked to authorize
// bytes whose meaning depends on which reader resolves them, and so input that
// cannot be carried in a hook payload fails as a retryable tool error instead
// of a dispatch failure. allowedIndexes maps allowed calls back to the input
// order without relying on duplicate-prone IDs.
func partitionAmbiguousToolCalls(
	prepared generationPrepared,
	toolCalls []fantasy.ToolCallContent,
) (allowed []fantasy.ToolCallContent, allowedIndexes []int, rejected []fantasy.ToolResultContent) {
	for i, toolCall := range toolCalls {
		if !json.Valid([]byte(toolCall.Input)) {
			rejected = append(rejected, malformedToolResult(toolCall))
			continue
		}
		if err := validateBuiltinToolInput(prepared, toolCall.ToolName, []byte(toolCall.Input)); err != nil {
			rejected = append(rejected, ambiguousToolResult(toolCall, err))
			continue
		}
		allowed = append(allowed, toolCall)
		allowedIndexes = append(allowedIndexes, i)
	}
	return allowed, allowedIndexes, rejected
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

// editFilesHookInputProperties is the schema of the grouped edit_files
// form, used to reject ambiguous overrides before they are decoded.
var editFilesHookInputProperties = schema.ToParameters(schema.Generate(reflect.TypeFor[chattool.EditFilesHookInput]()))

// presentHookToolInputs returns the calls to send to pre_tool_use and the
// model inputs it replaced, keyed by tool call ID. Hooks receive builtin
// edit_files input grouped by path, the shape hook policies were written
// for, instead of the model's flat edits list. Input that does not decode
// into the flat schema is sent unchanged; the tool rejects it without
// executing anything. Callers must have rejected duplicate tool call IDs.
func presentHookToolInputs(
	prepared generationPrepared,
	toolCalls []fantasy.ToolCallContent,
) ([]fantasy.ToolCallContent, map[string]string) {
	if !prepared.BuiltinToolNames[chattool.EditFilesName] {
		return toolCalls, nil
	}
	presented := slices.Clone(toolCalls)
	modelInputs := make(map[string]string)
	for i, toolCall := range presented {
		if toolCall.ToolName != chattool.EditFilesName {
			continue
		}
		var args chattool.EditFilesArgs
		if err := json.Unmarshal([]byte(toolCall.Input), &args); err != nil {
			continue
		}
		grouped, err := json.Marshal(chattool.NewEditFilesHookInput(args.Edits))
		if err != nil {
			continue
		}
		modelInputs[toolCall.ToolCallID] = toolCall.Input
		presented[i].Input = string(grouped)
	}
	return presented, modelInputs
}

// restoreHookToolInputs undoes presentHookToolInputs on the pre_tool_use
// result so persistence and execution see flat edit_files input: a call
// without an override gets its model input back, and an override, which
// must use the grouped form, is flattened. The model cannot fix a bad
// override, so one that is ambiguous or does not decode fails closed.
func restoreHookToolInputs(
	prepared generationPrepared,
	preflight *chathooks.PreToolUseExecutionResult,
	modelInputs map[string]string,
) error {
	if !prepared.BuiltinToolNames[chattool.EditFilesName] {
		return nil
	}
	for i, toolCall := range preflight.Allowed {
		if toolCall.ToolName != chattool.EditFilesName {
			continue
		}
		override, overridden := preflight.Overrides[toolCall.ToolCallID]
		if !overridden {
			if input, ok := modelInputs[toolCall.ToolCallID]; ok {
				preflight.Allowed[i].Input = input
			}
			continue
		}
		flat, err := flattenEditFilesOverride(override)
		if err != nil {
			return xerrors.Errorf("hook input override for tool %s: %w", toolCall.ToolName, err)
		}
		preflight.Allowed[i].Input = string(flat)
		preflight.Overrides[toolCall.ToolCallID] = flat
	}
	return nil
}

// flattenEditFilesOverride converts a grouped edit_files override into
// the flat tool input. The ambiguity check runs on the grouped bytes the
// consumer wrote, because flattening re-encodes every key canonically and
// would hide a case variant or repeated key from the later check.
func flattenEditFilesOverride(override json.RawMessage) (json.RawMessage, error) {
	if err := toolschema.ValidateUnambiguous(editFilesHookInputProperties, override); err != nil {
		return nil, err
	}
	var grouped chattool.EditFilesHookInput
	decoder := json.NewDecoder(bytes.NewReader(override))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&grouped); err != nil {
		return nil, xerrors.Errorf("decode grouped files form: %w", err)
	}
	if grouped.Files == nil {
		return nil, xerrors.New("decode grouped files form: files is required")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, xerrors.New("decode grouped files form: trailing JSON value")
	}
	flat, err := json.Marshal(chattool.EditFilesArgs{Edits: grouped.Edits()})
	if err != nil {
		return nil, xerrors.Errorf("encode flat edits: %w", err)
	}
	return flat, nil
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
