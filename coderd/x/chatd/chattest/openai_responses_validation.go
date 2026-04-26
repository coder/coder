package chattest

import (
	"fmt"
	"net/http"
	"strings"
)

// ValidateResponsesAPIInput validates the Responses API item relationships
// that OpenAI enforces but the fake test server would otherwise miss.
func ValidateResponsesAPIInput(items []interface{}) *ErrorResponse {
	if err := validateResponsesWebSearchReasoning(items); err != nil {
		return err
	}
	return validateResponsesFunctionCallOutputs(items)
}

type responsesInputKind int

const (
	responsesInputOther responsesInputKind = iota
	responsesInputReasoning
	responsesInputWebSearch
	responsesInputFunctionCall
	responsesInputFunctionCallOutput
)

type responsesInputItem struct {
	kind   responsesInputKind
	id     string
	callID string
}

func validateResponsesWebSearchReasoning(items []interface{}) *ErrorResponse {
	previousKind := responsesInputOther
	for _, raw := range items {
		item := classifyResponsesInputItem(raw)
		if item.kind == responsesInputWebSearch && previousKind != responsesInputReasoning {
			return openAIResponsesValidationError(fmt.Sprintf(
				"Item %q of type 'web_search_call' was provided without its required 'reasoning' item.",
				item.id,
			))
		}
		previousKind = item.kind
	}
	return nil
}

func validateResponsesFunctionCallOutputs(items []interface{}) *ErrorResponse {
	type callState struct {
		calls       int
		outputs     int
		firstCall   int
		firstOutput int
	}
	states := make(map[string]*callState)
	var callIDs []string
	var outputCallIDs []string

	stateFor := func(callID string) *callState {
		state, ok := states[callID]
		if ok {
			return state
		}
		state = &callState{firstCall: -1, firstOutput: -1}
		states[callID] = state
		return state
	}

	for index, raw := range items {
		item := classifyResponsesInputItem(raw)
		switch item.kind {
		case responsesInputFunctionCall:
			if item.callID == "" {
				continue
			}
			state := stateFor(item.callID)
			if state.calls == 0 {
				callIDs = append(callIDs, item.callID)
				state.firstCall = index
			}
			state.calls++
		case responsesInputFunctionCallOutput:
			if item.callID == "" {
				continue
			}
			state := stateFor(item.callID)
			if state.outputs == 0 {
				outputCallIDs = append(outputCallIDs, item.callID)
				state.firstOutput = index
			}
			state.outputs++
		}
	}

	for _, callID := range callIDs {
		state := states[callID]
		if state.calls > 1 {
			return openAIResponsesValidationError(fmt.Sprintf(
				"Duplicate function call found for call_id %s.", callID,
			))
		}
	}
	for _, callID := range outputCallIDs {
		state := states[callID]
		if state.outputs > 1 {
			return openAIResponsesValidationError(fmt.Sprintf(
				"Duplicate tool output found for function call %s.", callID,
			))
		}
	}
	for _, callID := range outputCallIDs {
		state := states[callID]
		if state.calls == 0 || state.firstOutput < state.firstCall {
			return openAIResponsesValidationError(fmt.Sprintf(
				"Tool output found without preceding function call %s.", callID,
			))
		}
	}
	for _, callID := range callIDs {
		state := states[callID]
		if state.outputs == 0 {
			return openAIResponsesValidationError(fmt.Sprintf(
				"No tool output found for function call %s.", callID,
			))
		}
	}

	return nil
}

func classifyResponsesInputItem(raw interface{}) responsesInputItem {
	itemMap, ok := raw.(map[string]interface{})
	if !ok {
		return responsesInputItem{kind: responsesInputOther}
	}

	itemType := StringResponseField(itemMap, "type")
	id := StringResponseField(itemMap, "id")
	callID := StringResponseField(itemMap, "call_id")

	switch itemType {
	case "reasoning":
		return responsesInputItem{kind: responsesInputReasoning, id: id}
	case "web_search_call":
		return responsesInputItem{kind: responsesInputWebSearch, id: id}
	case "function_call":
		return responsesInputItem{kind: responsesInputFunctionCall, callID: callID}
	case "function_call_output":
		return responsesInputItem{kind: responsesInputFunctionCallOutput, callID: callID}
	case "item_reference":
		switch {
		case strings.HasPrefix(id, "rs_"):
			return responsesInputItem{kind: responsesInputReasoning, id: id}
		case strings.HasPrefix(id, "ws_"):
			return responsesInputItem{kind: responsesInputWebSearch, id: id}
		default:
			return responsesInputItem{kind: responsesInputOther, id: id}
		}
	}

	// Some SDK encoders omit the type field for item references. Fall
	// back to stable OpenAI item ID prefixes so tests still catch an
	// invalid prompt shape.
	switch {
	case strings.HasPrefix(id, "rs_"):
		return responsesInputItem{kind: responsesInputReasoning, id: id}
	case strings.HasPrefix(id, "ws_"):
		return responsesInputItem{kind: responsesInputWebSearch, id: id}
	default:
		return responsesInputItem{kind: responsesInputOther, id: id, callID: callID}
	}
}

// StringResponseField returns the string value for key from a decoded
// Responses API item, or an empty string when the field is absent or not a
// string.
func StringResponseField(values map[string]interface{}, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func openAIResponsesValidationError(message string) *ErrorResponse {
	return &ErrorResponse{
		StatusCode: http.StatusBadRequest,
		Type:       "invalid_request_error",
		Message:    message,
	}
}
