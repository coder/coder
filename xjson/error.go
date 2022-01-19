package xjson

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/coder/coder/srverr"
)

const (
	// codeVerbose indicates a details object with a 'verbose' field
	// exists in the error response.
	codeVerbose srverr.Code = "verbose"
	// codeEmpty indicates that no details object exists.
	codeEmpty srverr.Code = "empty"
	// codeSolution indicates the details field has a payload for the
	// error and has a solution to resolve the error.
	codeSolution srverr.Code = "solution"
)

// WriteBadRequestWithCode writes a 400 to the response using a custom code, msg, and json marshaled details
func WriteBadRequestWithCode(w http.ResponseWriter, code srverr.Code, humanMsg string, details interface{}) {
	Write(w, http.StatusBadRequest, errorResponse{
		Error: errorPayload{
			Msg:     humanMsg,
			Code:    code,
			Details: details,
		},
	})
}

// WriteBadRequest writes a 400 to the response.
func WriteBadRequest(w http.ResponseWriter, humanMsg string) {
	WriteError(w, http.StatusBadRequest, humanMsg, nil)
}

// WriteUnauthorized writes a 401 to the response.
func WriteUnauthorized(w http.ResponseWriter, humanMsg string) {
	WriteError(w, http.StatusUnauthorized, humanMsg, nil)
}

// WriteForbidden writes a 403 to the response.
func WriteForbidden(w http.ResponseWriter, humanMsg string) {
	WriteError(w, http.StatusForbidden, humanMsg, nil)
}

// WriteConflict writes a 409 to the response.
func WriteConflict(w http.ResponseWriter, humanMsg string) {
	WriteError(w, http.StatusConflict, humanMsg, nil)
}

// WritePreconditionFailed writes a 412 to the response. If the err is non-nil
// a verbose field is written with the contents of the error.
func WritePreconditionFailed(w http.ResponseWriter, humanMsg string, err error) {
	WriteError(w, http.StatusPreconditionFailed, humanMsg, err)
}

func WriteErrorWithSolution(w http.ResponseWriter, statusCode int, humanMsg string, solution string, err error) {
	Write(w, statusCode, errorResponse{
		Error: errorPayload{
			Msg:  humanMsg,
			Code: codeSolution,
			Details: detailsPrecondition{
				Message:  humanMsg,
				Error:    err.Error(),
				Solution: solution,
				Verbose:  err.Error(), //nolint:deprecated
			},
		},
	})
}

// WriteFieldedPreconditionFailed writes a 412 to the response and the
// proper json fielded payload for decoding the error + solution
func WriteFieldedPreconditionFailed(w http.ResponseWriter, humanMsg string, solution string, err error) {
	WriteErrorWithSolution(w, http.StatusPreconditionFailed, humanMsg, solution, err)
}

// WriteNotFound writes a 404 to the response. It returns a generic public
// message such as "Environment not found." using the provided resource.
func WriteNotFound(w http.ResponseWriter, resource string) {
	WriteError(w, http.StatusNotFound, fmt.Sprintf("%s not found.", resource), nil)
}

// WriteCustomNotFound writes a 400 to the response.
func WriteCustomNotFound(w http.ResponseWriter, humanMsg string) {
	WriteError(w, http.StatusNotFound, humanMsg, nil)
}

// WriteInternalServerError writes a 500 to the response. It uses a generic
// message as the public message and writes the error the 'verbose' field
// in 'details' if it is non-nil.
func WriteInternalServerError(w http.ResponseWriter, err error) {
	WriteCustomInternalServerError(w, "An internal server error occurred.", err)
}

// WriteCustomInternalServerError writes a 500 to the response. Instead of the
// generic "An internal server error" occurred, the provided humanMsg is used.
func WriteCustomInternalServerError(w http.ResponseWriter, humanMsg string, err error) {
	WriteError(w, http.StatusInternalServerError, humanMsg, err)
}

// WriteError is a generic endpoint for writing error responses. If err is non-nil
// a 'verbose' field is written to the 'details' object.
func WriteError(w http.ResponseWriter, status int, humanMsg string, err error) {
	Write(w, status, defaultErrorParams{
		msg:     humanMsg,
		verbose: err,
	})
}

// defaultErrorParams contains common parameters across most error responses.
// Since the nature of the error payload is nested this type exists to allow
// assigning the values to a friendly, flat type.
type defaultErrorParams struct {
	msg     string
	verbose error
}

// MarshalJSON marshals the default error parameters into our structured error
// response.
func (d defaultErrorParams) MarshalJSON() ([]byte, error) {
	payload := errorResponse{
		Error: errorPayload{
			Msg:  d.msg,
			Code: codeEmpty,
		},
	}
	if d.verbose != nil {
		payload.Error.Code = codeVerbose
		payload.Error.Details = detailsVerbose{
			Verbose: d.verbose.Error(),
		}
	}

	return json.Marshal(payload)
}

// detailsVerbose is a simple object that can be assigned to the 'details'
// field of an erro response. It contains a more verbose explanation of the
// error. It tends to be the raw output of err.Error().
type detailsVerbose struct {
	Verbose string `json:"verbose,omitempty"`
}

// detailsPrecondition is a details object that should be paired with 412 status
// codes. It contains the Go error, a human message, and a solution note.
type detailsPrecondition struct {
	// Error is err.Error() and from Go
	Error string `json:"error"`
	// Message is the human readable error message
	Message string `json:"message"`
	// Solution is a helpful hint on how to solve the error
	Solution string `json:"solution"`

	// Verbose is a copy of Error.
	// Deprecated: Should remove this field, but the ui expects 'verbose' messages
	// 		still and have not been moved to use the new fields for this error type.
	Verbose string `json:"verbose,omitempty"`
}

// errorResponse is the root of the error payload we send for status codes 400
// and above.
type errorResponse struct {
	Error errorPayload `json:"error"`
}

// errorPayload contains the contents of an error response.
type errorPayload struct {
	// Msg is a human-readable message.
	Msg string `json:"msg"`
	// Code dictates the structure of the details field.
	Code srverr.Code `json:"code"`
	// Details is an arbitrary object containing extra information
	// on a particular error. Its structure is dictated by Code.
	Details interface{} `json:"details,omitempty"`
}

// HTTPError represents an error from the Coder API.
type HTTPError struct {
	*http.Response
	// we can't read the body lazily when Error is invoked
	// so this must be populated at construction
	Body []byte
}

var _ error = &HTTPError{}

// Error implements error.
func (e *HTTPError) Error() string {
	var msg errorResponse
	// Try to decode the payload as an error, if it fails or if there is no error message,
	// return the response URL with the status.
	if err := json.Unmarshal(e.Body, &msg); err != nil || msg.Error.Msg == "" {
		return fmt.Sprintf("%s: %d %s", e.Request.URL, e.StatusCode, e.Status)
	}

	// If the payload was a in the expected error format with a message, include it.
	return msg.Error.Msg
}

func BodyError(resp *http.Response) *HTTPError {
	body, _ := io.ReadAll(resp.Body)
	return &HTTPError{Response: resp, Body: body}
}
