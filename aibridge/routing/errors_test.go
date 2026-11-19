package routing_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/routing"
)

func TestWriteForbiddenError(t *testing.T) {
	t.Parallel()

	type errorObject struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	type errorBody struct {
		Type  string      `json:"type"`
		Error errorObject `json:"error"`
	}
	want := errorObject{Type: "permission_error", Message: "budget gone", Code: routing.ErrorCodeBudgetExceeded}

	cases := []struct {
		name string
		path string
		// wantType is the top-level type field, which only the Anthropic
		// format has.
		wantType string
	}{
		{name: "AnthropicMessages", path: "/anthropic/v1/messages", wantType: "error"},
		{name: "CopilotMessages", path: "/copilot/v1/messages", wantType: "error"},
		{name: "OpenAIChatCompletions", path: "/openai/v1/chat/completions"},
		{name: "OpenAIResponses", path: "/openai/v1/responses"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			routing.WriteForbiddenError(rec, req, routing.ErrorCodeBudgetExceeded, "budget gone")

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			var got errorBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tc.wantType, got.Type)
			require.Equal(t, want, got.Error)
		})
	}
}
