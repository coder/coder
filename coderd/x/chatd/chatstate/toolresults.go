package chatstate

import "encoding/json"

// ValidateToolResults returns the first validation error for results
// submitted against pending dynamic tool calls.
func ValidateToolResults(results []ToolResultInput, pending map[string]string) *ToolResultValidationError {
	submitted := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, dup := submitted[result.ToolCallID]; dup {
			return &ToolResultValidationError{Cause: ErrToolResultDuplicate, ToolCallID: result.ToolCallID}
		}
		if !json.Valid(result.Output) {
			return &ToolResultValidationError{Cause: ErrToolResultInvalidJSON, ToolCallID: result.ToolCallID}
		}
		submitted[result.ToolCallID] = struct{}{}
	}
	for toolCallID := range pending {
		if _, ok := submitted[toolCallID]; !ok {
			return &ToolResultValidationError{Cause: ErrToolResultMissing, ToolCallID: toolCallID}
		}
	}
	for toolCallID := range submitted {
		if _, ok := pending[toolCallID]; !ok {
			return &ToolResultValidationError{Cause: ErrToolResultUnexpected, ToolCallID: toolCallID}
		}
	}
	return nil
}
