package chattest

import (
	"fmt"
	"net/http"
	"strings"
)

// ValidateResponsesAPIInput inspects the Responses API `input` array and
// returns a non-nil ErrorResponse mimicking the live OpenAI validation
// rules that chatd needs to satisfy:
//
//  1. Any `item_reference` to a `web_search_call` (ID prefix `ws_`) MUST
//     be immediately preceded by an `item_reference` to a `reasoning`
//     item (ID prefix `rs_`). Live OpenAI rejects the request with:
//     "Item 'ws_xxx' of type 'web_search_call' was provided without
//     its required 'reasoning' item: 'rs_xxx'."
//  2. Any `function_call` MUST be followed by a `function_call_output`
//     with a matching `call_id`. Live OpenAI rejects with:
//     "No tool output found for function call call_xxx."
//
// Item-reference shape is detected leniently. fantasy currently emits
// item_reference entries with only an "id" field (the type field is
// `omitzero` in the openai-go SDK), so the validator falls back to
// treating any otherwise-bare item with a known ID prefix as a
// reference of the inferred kind.
//
// Returns nil when the input passes validation. Mirrors the OpenAI
// 400 error envelope. Caller decides whether to return the error.
func ValidateResponsesAPIInput(items []interface{}) *ErrorResponse {
	prevReasoningID := ""
	functionCalls := make(map[string]bool)   // call_id -> seen function_call
	functionOutputs := make(map[string]bool) // call_id -> seen function_call_output

	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		switch classifyResponsesInputItem(item) {
		case responsesInputKindItemReference:
			id, _ := item["id"].(string)
			switch {
			case strings.HasPrefix(id, "ws_"):
				if !strings.HasPrefix(prevReasoningID, "rs_") {
					return &ErrorResponse{
						StatusCode: http.StatusBadRequest,
						Type:       "invalid_request_error",
						Message: fmt.Sprintf(
							"Item '%s' of type 'web_search_call' was "+
								"provided without its required 'reasoning' "+
								"item: 'rs_*'.",
							id),
					}
				}
				prevReasoningID = ""
			case strings.HasPrefix(id, "rs_"):
				prevReasoningID = id
			default:
				prevReasoningID = ""
			}
		case responsesInputKindFunctionCall:
			callID, _ := item["call_id"].(string)
			if callID == "" {
				callID, _ = item["id"].(string)
			}
			if callID != "" {
				functionCalls[callID] = true
			}
			prevReasoningID = ""
		case responsesInputKindFunctionCallOutput:
			callID, _ := item["call_id"].(string)
			if callID != "" {
				functionOutputs[callID] = true
			}
			prevReasoningID = ""
		default:
			prevReasoningID = ""
		}
	}

	for callID := range functionCalls {
		if !functionOutputs[callID] {
			return &ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Type:       "invalid_request_error",
				Message: fmt.Sprintf(
					"No tool output found for function call %s.", callID),
			}
		}
	}

	return nil
}

type responsesInputKind int

const (
	responsesInputKindUnknown responsesInputKind = iota
	responsesInputKindMessage
	responsesInputKindItemReference
	responsesInputKindFunctionCall
	responsesInputKindFunctionCallOutput
)

// classifyResponsesInputItem identifies the kind of Responses API input
// item from its JSON shape. It first honors an explicit "type" field,
// then falls back to shape-based heuristics for the openai-go SDK's
// item_reference shape (only "id" set).
func classifyResponsesInputItem(item map[string]interface{}) responsesInputKind {
	if t, ok := item["type"].(string); ok && t != "" {
		switch t {
		case "item_reference":
			return responsesInputKindItemReference
		case "function_call":
			return responsesInputKindFunctionCall
		case "function_call_output":
			return responsesInputKindFunctionCallOutput
		case "message":
			return responsesInputKindMessage
		default:
			return responsesInputKindUnknown
		}
	}
	// Implicit message: EasyInputMessage shape with role and content.
	if _, ok := item["role"]; ok {
		return responsesInputKindMessage
	}
	// Implicit function_call_output: has call_id + output but no role.
	if _, ok := item["call_id"]; ok {
		if _, hasOutput := item["output"]; hasOutput {
			return responsesInputKindFunctionCallOutput
		}
		return responsesInputKindFunctionCall
	}
	// Implicit item_reference: bare {id} (the openai-go SDK omits the
	// type field as it is `omitzero`). Only treat as reference if the
	// ID looks like an OpenAI item ID.
	if id, ok := item["id"].(string); ok && id != "" {
		if strings.HasPrefix(id, "ws_") ||
			strings.HasPrefix(id, "rs_") ||
			strings.HasPrefix(id, "msg_") ||
			strings.HasPrefix(id, "fc_") {
			return responsesInputKindItemReference
		}
	}
	return responsesInputKindUnknown
}
