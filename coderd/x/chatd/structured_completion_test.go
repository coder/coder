package chatd_test

import (
	"cmp"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

// A turn with an open structured output request finishes only with its
// candidate as a succeeded receipt; text answers are repaired and the
// repair budget ends the turn with a failed receipt.
func TestStructuredCompletion(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","properties":{"SCHEMA_MARKER":{"type":"string"}},"required":["SCHEMA_MARKER"]}`)
	valid := []chattest.OpenAIToolCall{finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"SCHEMA_MARKER":"CANDIDATE"}}`)}
	invalid := []chattest.OpenAIToolCall{finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"SCHEMA_MARKER":1}}`)}
	for _, tt := range []struct {
		name       string
		schema     json.RawMessage
		failStatus int
		steps      [][]chattest.OpenAIToolCall
		status     database.ChatStatus
		calls      int // model calls, or 0 to skip the check
		rejections int
		code       codersdk.ChatStructuredOutputErrorCode // of a failed receipt
		corrective bool                                   // every repair request ends with the corrective text
	}{
		{name: "Succeeded", schema: schema, steps: [][]chattest.OpenAIToolCall{valid}, calls: 2},
		{name: "NotProduced", schema: schema, calls: 3, rejections: 3, code: codersdk.ChatStructuredOutputErrorCodeNotProduced, corrective: true},
		{name: "Repaired", schema: schema, steps: [][]chattest.OpenAIToolCall{invalid, invalid, valid}, calls: 4, rejections: 2},
		{name: "ValidationExhausted", schema: schema, steps: [][]chattest.OpenAIToolCall{invalid, invalid, invalid}, calls: 3, rejections: 3, code: codersdk.ChatStructuredOutputErrorCodeValidationExhausted},
		{name: "RetriedTransport", schema: schema, failStatus: http.StatusInternalServerError, steps: [][]chattest.OpenAIToolCall{valid}},
		{name: "GenerationFailed", schema: schema, failStatus: http.StatusBadRequest, status: database.ChatStatusError, calls: 1, code: codersdk.ChatStructuredOutputErrorCodeGenerationFailed},
		{name: "Ordinary", calls: 1},
		{name: "OrdinaryError", failStatus: http.StatusBadRequest, status: database.ChatStatusError, calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := runStructuredTurn(t, tt.schema, tt.failStatus, tt.steps...)
			require.Equal(t, cmp.Or(tt.status, database.ChatStatusWaiting), run.status)
			if tt.calls != 0 {
				require.Len(t, run.requests, tt.calls)
			}
			for i, messages := range run.requests {
				last := messages[len(messages)-1]
				corrective := last.Role == "user" && strings.Contains(last.Content, chatstructured.FinalizerToolName)
				require.Equal(t, tt.corrective && i > 0, corrective)
				require.False(t, corrective && strings.Contains(last.Content, "SCHEMA_MARKER"), "the corrective text quotes no schema")
			}
			// Corrective rows are model-only.
			for _, msg := range run.client {
				for _, part := range msg.Content {
					require.NotContains(t, part.Text, chatstructured.FinalizerToolName)
				}
			}
			require.Equal(t, tt.rejections, run.state.Rejections)
			if tt.schema == nil {
				require.Empty(t, run.receipts)
				require.Empty(t, run.controls)
				return
			}
			require.Len(t, run.receipts, 1)
			require.True(t, run.state.Closed)
			got := run.receipts[0]
			if tt.code == "" {
				require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, got.Status)
				require.JSONEq(t, `{"SCHEMA_MARKER":"CANDIDATE"}`, string(got.Value))
				// Clients without outcome support read the value as text.
				last := run.client[len(run.client)-1]
				require.JSONEq(t, string(got.Value), last.Content[0].Text)
				return
			}
			require.Equal(t, codersdk.ChatStructuredOutputStatusFailed, got.Status)
			require.Equal(t, tt.code, got.Error.Code)
			require.NotEmpty(t, got.Error.Message)
			require.NotContains(t, got.Error.Message, "PROVIDER_BODY_MARKER")
		})
	}
}
