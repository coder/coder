// This file defines the local finalizer tool through which a model submits
// its structured output. The definition is meant to be sent as a provider
// tool definition, bypassing the chat loop's schema normalization, so the
// model sees the caller's schema exactly as written. Arguments go through
// one validation path that checks the envelope, then validates the exact
// output bytes the model sent.

package chatstructured

import (
	"context"
	"encoding/json"
	"errors"

	"charm.land/fantasy"
	"golang.org/x/xerrors"
)

// FinalizerToolName is the reserved name of the structured output tool.
const FinalizerToolName = "coder_structured_output"

const (
	finalizerDescription = "Submit your final answer exactly once as the output property. " +
		"The output must match the JSON Schema of the output property."
	finalizerAcknowledgment = "Structured output accepted."
	finalizerRetry          = ". Call " + FinalizerToolName + " again with a corrected output."
	finalizerUnchecked      = "structured output could not be checked"
)

// ErrInvalidFinalizerArguments rejects finalizer arguments that are not a
// JSON object whose only key is "output".
var ErrInvalidFinalizerArguments = xerrors.New("finalizer arguments must be an object with only the output property")

// finalizerSentinels are the fixed-text rejections the runner may show the
// model. Any other error is reported with finalizerUnchecked.
var finalizerSentinels = []error{
	ErrInvalidFinalizerArguments, ErrTooLarge, ErrInvalidUTF8, ErrMalformed, ErrTrailingData, ErrTooDeep,
	ErrTooManyNodes, ErrArrayTooLong, ErrDuplicateKey, ErrNullCharacter, ErrUnpairedSurrogate,
	ErrNumberTooLong, ErrExponentTooLarge, ErrNumberOutOfRange,
}

// FinalizerDefinition returns a fresh tool definition whose output property
// is a deep copy of the caller's schema without its root $schema, so a
// caller or provider adapter mutating it cannot affect this Schema.
func (s *Schema) FinalizerDefinition() fantasy.FunctionTool {
	return fantasy.FunctionTool{
		Name:        FinalizerToolName,
		Description: finalizerDescription,
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"output": s.outputSchema()},
			"required":             []string{"output"},
			"additionalProperties": false,
		},
	}
}

func (s *Schema) outputSchema() map[string]any {
	out, _ := copyJSON(s.document).(map[string]any)
	delete(out, "$schema")
	return out
}

// copyJSON deep-copies a parsed JSON value; scalars are immutable.
func copyJSON(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = copyJSON(e)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = copyJSON(e)
		}
		return out
	}
	return v
}

// CheckFinalizerArguments parses raw finalizer arguments under the envelope
// caps, requires exactly the output property, and validates the output's
// original bytes. Re-encoding the parsed value could change its size, for
// example by escaping U+2028, and so change which caps apply.
func (s *Schema) CheckFinalizerArguments(raw []byte) (any, error) {
	envelope, err := ParseFinalizerArguments(raw)
	if err != nil {
		return nil, err
	}
	obj, ok := envelope.(map[string]any)
	if _, has := obj["output"]; !ok || !has || len(obj) != 1 {
		return nil, ErrInvalidFinalizerArguments
	}
	// With the key set verified, encoding/json's case-insensitive field
	// matching can only select the exact output key.
	var args struct {
		Output json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, xerrors.Errorf("extract finalizer output: %w", err)
	}
	return s.Validate(args.Output)
}

// FinalizerRunner returns the local executor for the finalizer tool. Its
// only mutable state is its provider options.
func (s *Schema) FinalizerRunner() fantasy.AgentTool {
	return &finalizerRunner{schema: s}
}

type finalizerRunner struct {
	schema *Schema
	opts   fantasy.ProviderOptions
}

func (r *finalizerRunner) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        FinalizerToolName,
		Description: finalizerDescription,
		Parameters:  map[string]any{"output": r.schema.outputSchema()},
		Required:    []string{"output"},
	}
}

// Run acknowledges valid arguments. Rejections are model-visible tool
// errors, never Go errors, with at most 1024 bytes of feedback that holds
// validation paths and fixed text but no argument values.
func (r *finalizerRunner) Run(_ context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	_, err := r.schema.CheckFinalizerArguments([]byte(call.Input))
	if err == nil {
		return fantasy.NewTextResponse(finalizerAcknowledgment), nil
	}
	feedback := finalizerUnchecked
	var verr *ValidationError
	if errors.As(err, &verr) {
		feedback = verr.Error()
	} else {
		for _, sentinel := range finalizerSentinels {
			if errors.Is(err, sentinel) {
				feedback = sentinel.Error()
				break
			}
		}
	}
	return fantasy.NewTextErrorResponse(truncateUTF8(feedback+finalizerRetry, maxValidationErrorBytes)), nil
}

func (r *finalizerRunner) ProviderOptions() fantasy.ProviderOptions { return r.opts }

func (r *finalizerRunner) SetProviderOptions(opts fantasy.ProviderOptions) { r.opts = opts }
