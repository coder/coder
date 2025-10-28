package agentsocket

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

// Protocol version for the agent socket API
const ProtocolVersion = "1.0"

// Request represents an incoming request to the agent socket
type Request struct {
	Version string          `json:"version"`
	Method  string          `json:"method"`
	ID      string          `json:"id,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a response from the agent socket
type Response struct {
	Version string          `json:"version"`
	ID      string          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error represents an error in the response
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard error codes
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// NewError creates a new error response
func NewError(code int, message string, data any) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// NewResponse creates a successful response
func NewResponse(id string, result any) (*Response, error) {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, xerrors.Errorf("marshal result: %w", err)
	}

	return &Response{
		Version: ProtocolVersion,
		ID:      id,
		Result:  resultBytes,
	}, nil
}

// NewErrorResponse creates an error response
func NewErrorResponse(id string, err *Error) *Response {
	return &Response{
		Version: ProtocolVersion,
		ID:      id,
		Error:   err,
	}
}

// Handler represents a function that can handle a request
type Handler func(ctx Context, req *Request) (*Response, error)

// Context provides context for request handling
type Context struct {
	// Additional context can be added here in the future
	// For now, this is a placeholder for future auth context, etc.
}
