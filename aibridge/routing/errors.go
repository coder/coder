package routing

import (
	"net/http"
	"strings"

	"github.com/coder/coder/v2/coderd/httpapi"
)

// ErrorCodeBudgetExceeded is the code written in the response body when the
// user's AI budget is exceeded. Paired with HTTP 403.
const ErrorCodeBudgetExceeded = "budget_exceeded"

// errorTypePermission is the error type both Anthropic and OpenAI use for
// HTTP 403 responses.
const errorTypePermission = "permission_error"

type providerErrorObject struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// anthropicErrorBody is the Anthropic error envelope. Anthropic error
// objects do not define a code field; clients ignore it, and Coder reads it.
type anthropicErrorBody struct {
	Type  string              `json:"type"`
	Error providerErrorObject `json:"error"`
}

// openAIErrorBody is the OpenAI error envelope.
type openAIErrorBody struct {
	Error providerErrorObject `json:"error"`
}

// WriteForbiddenError writes a 403 JSON error with a stable code in the
// format of the provider API that r targets, so AI clients parse it as a
// provider error instead of an authentication failure. Requests to the
// Anthropic Messages API, whatever the provider, get the Anthropic format.
// Every other request gets the OpenAI format.
func WriteForbiddenError(w http.ResponseWriter, r *http.Request, code, message string) {
	obj := providerErrorObject{Type: errorTypePermission, Message: message, Code: code}

	var body any = openAIErrorBody{Error: obj}
	if strings.Contains(r.URL.Path, "/v1/messages") {
		body = anthropicErrorBody{Type: "error", Error: obj}
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	httpapi.Write(r.Context(), w, http.StatusForbidden, body)
}
