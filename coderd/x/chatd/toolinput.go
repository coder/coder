package chatd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"

	"charm.land/fantasy"
	fantasyschema "charm.land/fantasy/schema"
	"github.com/google/jsonschema-go/jsonschema"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
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

// editFilesHookInputProperties is the schema of the grouped edit_files
// form, used to reject ambiguous overrides before they are decoded.
var editFilesHookInputProperties = fantasyschema.ToParameters(fantasyschema.Generate(reflect.TypeFor[chattool.EditFilesHookInput]()))

// presentHookToolInputs returns the calls to send to pre_tool_use and the
// model inputs it replaced, keyed by tool call ID. Hooks receive builtin
// edit_files input grouped by path, the shape edit_files used before the
// flat list, with paths normalized the way the tool uses them. Decoding
// drops keys the flat schema does not declare. Input that does not
// decode is sent unchanged; the tool rejects it without executing
// anything. Callers must have rejected duplicate tool call IDs.
func presentHookToolInputs(
	prepared turnEnvironment,
	toolCalls []fantasy.ToolCallContent,
) ([]fantasy.ToolCallContent, map[string]string) {
	if !prepared.BuiltinToolNames[chattool.EditFilesToolName] {
		return toolCalls, nil
	}
	presented := slices.Clone(toolCalls)
	modelInputs := make(map[string]string)
	for i, toolCall := range presented {
		if toolCall.ToolName != chattool.EditFilesToolName {
			continue
		}
		var args chattool.EditFilesArgs
		if err := json.Unmarshal([]byte(toolCall.Input), &args); err != nil {
			continue
		}
		grouped, err := json.Marshal(chattool.NewEditFilesHookInput(chattool.NormalizeEditPaths(args.Edits)))
		if err != nil {
			continue
		}
		modelInputs[toolCall.ToolCallID] = toolCall.Input
		presented[i].Input = string(grouped)
	}
	return presented, modelInputs
}

// restoreHookToolInputs undoes presentHookToolInputs so persistence and
// execution see flat edit_files input. A call without an override gets
// its model input back. A grouped override is flattened into
// preflight.Overrides, which persistence reads, and into
// preflight.Allowed for later readers (currently only override
// validation). The model cannot fix a bad override, so one that is
// ambiguous or does not decode fails closed.
func restoreHookToolInputs(
	prepared turnEnvironment,
	preflight *chathooks.PreToolUseExecutionResult,
	modelInputs map[string]string,
) error {
	if !prepared.BuiltinToolNames[chattool.EditFilesToolName] {
		return nil
	}
	for i, toolCall := range preflight.Allowed {
		if toolCall.ToolName != chattool.EditFilesToolName {
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
	// The flat input is stored and replayed to the model, so text is kept
	// as written instead of HTML-escaped.
	var flat bytes.Buffer
	encoder := json.NewEncoder(&flat)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(chattool.EditFilesArgs{Edits: grouped.Edits()}); err != nil {
		return nil, xerrors.Errorf("encode flat edits: %w", err)
	}
	return bytes.TrimSuffix(flat.Bytes(), []byte("\n")), nil
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
