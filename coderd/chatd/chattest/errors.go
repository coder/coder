package chattest

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse describes an HTTP error that a test server should return
// instead of a normal streaming or JSON response.
type ErrorResponse struct {
	StatusCode int
	Type       string
	Message    string
}

// writeErrorResponse writes a JSON error response matching the common
// provider error format used by both Anthropic and OpenAI.
func writeErrorResponse(w http.ResponseWriter, errResp *ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(errResp.StatusCode)
	body := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errResp.Type,
			"message": errResp.Message,
		},
	}
	_ = json.NewEncoder(w).Encode(body)
}

// AnthropicErrorResponse returns an AnthropicResponse that causes the
// test server to respond with the given HTTP status code and error.
// This simulates provider errors like 529 Overloaded or 429 Rate Limited.
func AnthropicErrorResponse(statusCode int, errorType, message string) AnthropicResponse {
	return AnthropicResponse{
		Error: &ErrorResponse{
			StatusCode: statusCode,
			Type:       errorType,
			Message:    message,
		},
	}
}

// AnthropicOverloadedResponse returns a 529 "overloaded" error matching
// Anthropic's overloaded response format.
func AnthropicOverloadedResponse() AnthropicResponse {
	return AnthropicErrorResponse(529, "overloaded_error", "Overloaded")
}

// AnthropicRateLimitResponse returns a 429 rate limit error.
func AnthropicRateLimitResponse() AnthropicResponse {
	return AnthropicErrorResponse(http.StatusTooManyRequests, "rate_limit_error", "Rate limited")
}

// OpenAIErrorResponse returns an OpenAIResponse that causes the
// test server to respond with the given HTTP status code and error.
func OpenAIErrorResponse(statusCode int, errorType, message string) OpenAIResponse {
	return OpenAIResponse{
		Error: &ErrorResponse{
			StatusCode: statusCode,
			Type:       errorType,
			Message:    message,
		},
	}
}

// OpenAIRateLimitResponse returns a 429 rate limit error.
func OpenAIRateLimitResponse() OpenAIResponse {
	return OpenAIErrorResponse(http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded")
}

// OpenAIServerErrorResponse returns a 500 internal server error.
func OpenAIServerErrorResponse() OpenAIResponse {
	return OpenAIErrorResponse(http.StatusInternalServerError, "server_error", "Internal server error")
}
