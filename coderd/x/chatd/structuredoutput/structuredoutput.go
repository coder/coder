// Package structuredoutput implements server-validated structured
// final output for chat turns.
//
// A caller opts in by sending a response_format of type json_schema
// on CreateChatRequest or CreateChatMessageRequest. chatd then runs
// the normal agent loop but injects a server-owned finalizer tool
// (ToolName) that the model must call to end the turn. The tool
// validates the model's arguments against the caller's JSON schema;
// the turn only finishes successfully once a validated result
// exists. The finalizer is an implementation detail: the API
// guarantee is server-validated output, not provider-native
// constrained decoding.
package structuredoutput

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/kaptinlin/jsonschema"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ToolName is the reserved name of the server-owned finalizer tool.
// Dynamic tools must never use this name; chatd rejects it at the
// HTTP layer and enforces builtin precedence at generation time.
const ToolName = "coder_structured_output"

// MaxSchemaBytes caps the caller-provided JSON schema size.
const MaxSchemaBytes = 16 * 1024

// outputProperty is the single top-level argument of the finalizer
// tool. The caller schema is nested under it because fantasy tool
// definitions treat ToolInfo.Parameters as a property map of an
// implicit root object schema.
const outputProperty = "output"

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidationError describes a request-time response_format
// rejection. Field is the JSON path of the offending request field.
type ValidationError struct {
	Field  string
	Detail string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Detail)
}

// Request is a normalized, validated structured output request
// active for one assistant turn.
type Request struct {
	Name        string
	Description string
	Schema      json.RawMessage

	// schemaMap is the decoded Schema object, parsed once during
	// NewRequest so tool definitions never reparse the raw bytes.
	schemaMap map[string]any
	compiled  *jsonschema.Schema
}

// Validate checks a request-level response_format. A nil format is
// valid (structured output not requested). It returns a
// *ValidationError so HTTP handlers can produce field-specific 400s.
func Validate(format *codersdk.ChatResponseFormat) *ValidationError {
	_, err := NewRequest(format)
	return err
}

// NewRequest validates format and compiles its schema. It returns
// (nil, nil) when format is nil or explicitly "text" with no schema
// payload, i.e. when the turn has no structured output request.
func NewRequest(format *codersdk.ChatResponseFormat) (*Request, *ValidationError) {
	if format == nil {
		return nil, nil
	}
	switch format.Type {
	case codersdk.ChatResponseFormatTypeText:
		if format.JSONSchema != nil {
			return nil, &ValidationError{
				Field:  "response_format.json_schema",
				Detail: `must not be set when type is "text".`,
			}
		}
		return nil, nil
	case codersdk.ChatResponseFormatTypeJSONSchema:
		// Validated below.
	case "":
		return nil, &ValidationError{
			Field:  "response_format.type",
			Detail: `is required; must be "text" or "json_schema".`,
		}
	default:
		return nil, &ValidationError{
			Field:  "response_format.type",
			Detail: fmt.Sprintf(`unsupported value %q; must be "text" or "json_schema".`, format.Type),
		}
	}

	js := format.JSONSchema
	if js == nil {
		return nil, &ValidationError{
			Field:  "response_format.json_schema",
			Detail: `is required when type is "json_schema".`,
		}
	}
	if !namePattern.MatchString(js.Name) {
		return nil, &ValidationError{
			Field:  "response_format.json_schema.name",
			Detail: "must match ^[A-Za-z0-9_-]{1,64}$.",
		}
	}
	if js.Strict != nil && !*js.Strict {
		return nil, &ValidationError{
			Field:  "response_format.json_schema.strict",
			Detail: "strict=false is not supported yet; omit strict or set it to true.",
		}
	}
	if len(js.Schema) == 0 {
		return nil, &ValidationError{
			Field:  "response_format.json_schema.schema",
			Detail: "is required.",
		}
	}
	if len(js.Schema) > MaxSchemaBytes {
		return nil, &ValidationError{
			Field:  "response_format.json_schema.schema",
			Detail: fmt.Sprintf("exceeds the maximum size of %d bytes.", MaxSchemaBytes),
		}
	}

	var root map[string]any
	if err := json.Unmarshal(js.Schema, &root); err != nil {
		return nil, &ValidationError{
			Field:  "response_format.json_schema.schema",
			Detail: "must be a JSON object.",
		}
	}
	if rootType, _ := root["type"].(string); rootType != "object" {
		return nil, &ValidationError{
			Field:  "response_format.json_schema.schema",
			Detail: `root must declare "type":"object"; wrap arrays or primitives in an object property.`,
		}
	}
	if refErr := validateFragmentOnlyRefs(root); refErr != nil {
		return nil, refErr
	}

	compiled, err := compileSchema(js.Schema)
	if err != nil {
		return nil, &ValidationError{
			Field:  "response_format.json_schema.schema",
			Detail: fmt.Sprintf("failed to compile: %v.", err),
		}
	}

	return &Request{
		Name:        js.Name,
		Description: js.Description,
		Schema:      js.Schema,
		schemaMap:   root,
		compiled:    compiled,
	}, nil
}

// compileSchema compiles schema bytes with a compiler that cannot
// load remote documents. Fragment-only $ref enforcement happens
// before compilation; stripping the loaders is defense in depth.
func compileSchema(schemaBytes []byte) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	for scheme := range compiler.Loaders {
		delete(compiler.Loaders, scheme)
	}
	return compiler.Compile(schemaBytes)
}

// refKeywords are schema keywords whose string values reference
// other schemas. Each must stay inside the caller's document.
var refKeywords = map[string]struct{}{
	"$ref":          {},
	"$dynamicRef":   {},
	"$recursiveRef": {},
}

// validateFragmentOnlyRefs walks the schema and rejects any
// reference value that does not start with "#". This keeps schema
// resolution local to the submitted document so no network or file
// lookups can be triggered.
func validateFragmentOnlyRefs(node any) *ValidationError {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if _, isRef := refKeywords[key]; isRef {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return &ValidationError{
						Field:  "response_format.json_schema.schema",
						Detail: fmt.Sprintf(`%s values must be fragment-local (start with "#"); got %v.`, key, child),
					}
				}
				continue
			}
			if err := validateFragmentOnlyRefs(child); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := validateFragmentOnlyRefs(item); err != nil {
				return err
			}
		}
	}
	return nil
}

// Tool returns the server-owned finalizer fantasy.AgentTool for req.
func Tool(req *Request) fantasy.AgentTool {
	return &finalizerTool{req: req}
}

type finalizerTool struct {
	req  *Request
	opts fantasy.ProviderOptions
}

func (t *finalizerTool) Info() fantasy.ToolInfo {
	description := "Submit the final structured answer for this task. " +
		"Call this tool exactly once, alone, after all other work is done. " +
		"The output argument must satisfy the required JSON schema."
	if t.req.Description != "" {
		description += " Output description: " + t.req.Description
	}

	return fantasy.ToolInfo{
		Name:        ToolName,
		Description: description,
		Parameters:  map[string]any{outputProperty: t.req.schemaMap},
		Required:    []string{outputProperty},
		Parallel:    false,
	}
}

func (t *finalizerTool) Run(_ context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	canonical, err := t.req.ValidateOutput([]byte(call.Input))
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return fantasy.NewTextResponse(string(canonical)), nil
}

func (t *finalizerTool) ProviderOptions() fantasy.ProviderOptions {
	return t.opts
}

func (t *finalizerTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.opts = opts
}

// ValidateOutput parses finalizer tool args, validates the "output"
// value against the compiled schema, and returns its canonical JSON
// encoding. Errors are stable, model-actionable strings surfaced as
// retryable tool errors.
func (r *Request) ValidateOutput(args []byte) (json.RawMessage, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(args, &parsed); err != nil {
		return nil, xerrors.New(`invalid arguments: expected a JSON object of the form {"output": <value matching the schema>}.`)
	}
	rawOutput, ok := parsed[outputProperty]
	if !ok {
		return nil, xerrors.New(`missing required "output" argument: pass the final answer as {"output": <value matching the schema>}.`)
	}

	var outputValue any
	if err := json.Unmarshal(rawOutput, &outputValue); err != nil {
		return nil, xerrors.New(`invalid "output" argument: not valid JSON.`)
	}
	result := r.compiled.Validate(outputValue)
	if !result.IsValid() {
		return nil, xerrors.Errorf(`"output" does not satisfy the required schema: %s. Fix the listed fields and call %s again.`, formatValidationErrors(result), ToolName)
	}

	// Re-marshal for a canonical encoding (stable whitespace,
	// escaped strings) independent of the model's formatting.
	canonical, err := json.Marshal(outputValue)
	if err != nil {
		return nil, xerrors.Errorf("encode validated output: %w", err)
	}
	return canonical, nil
}

// formatValidationErrors flattens an evaluation result into a
// compact, deterministic one-line summary for the model.
func formatValidationErrors(result *jsonschema.EvaluationResult) string {
	list := result.ToList(false)
	if list == nil {
		return "schema validation failed"
	}
	var sb strings.Builder
	appendErrors(&sb, *list)
	if sb.Len() == 0 {
		return "schema validation failed"
	}
	return sb.String()
}

func appendErrors(sb *strings.Builder, list jsonschema.List) {
	location := list.InstanceLocation
	if location == "" {
		location = "(root)"
	}
	keys := make([]string, 0, len(list.Errors))
	for key := range list.Errors {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if sb.Len() > 0 {
			_, _ = sb.WriteString("; ")
		}
		_, _ = fmt.Fprintf(sb, "%s: %s", location, list.Errors[key])
	}
	for _, detail := range list.Details {
		appendErrors(sb, detail)
	}
}
