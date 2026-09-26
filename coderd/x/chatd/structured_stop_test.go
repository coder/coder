package chatd_test

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

func TestStructuredStopHooks(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object","properties":{"v":{"type":"string"}},"required":["v"]}`)
	finalize := func(v string) []chattest.OpenAIToolCall {
		return []chattest.OpenAIToolCall{finalizerToolCall(chatstructured.FinalizerToolName, `{"output":{"v":"`+v+`"}}`)}
	}

	// A continuing stop hook invalidates the candidate with its model-only
	// context row, so the text answer after the nudge is a rejection and
	// only a new finalizer call closes the request.
	t.Run("ContinuingHookInvalidates", func(t *testing.T) {
		t.Parallel()
		var stops atomic.Int32
		run := runStructuredHookTurn(t, func(request agenthooks.Request) (int, string) {
			if request.Type == agenthooks.EventStop && stops.Add(1) == 1 {
				return http.StatusOK, `{"model_context":"continue please"}`
			}
			return 0, ""
		}, schema, 0, finalize("OLD"), nil, nil, finalize("NEW"))
		require.Equal(t, database.ChatStatusWaiting, run.status)
		require.Len(t, run.requests, 5)
		require.Equal(t, []chatstructured.Control{{RequestID: run.state.Request.RequestID, Kind: chatstructured.ControlInvalidation}}, run.hidden)
		require.Equal(t, 1, run.state.Rejections)
		require.Len(t, run.receipts, 1)
		require.JSONEq(t, `{"v":"NEW"}`, string(run.receipts[0].Value))
		for _, msg := range run.client {
			require.NotEmpty(t, msg.Content, "no client message is empty")
		}
	})

	// A failing post_tool_use hook ends the turn with an error that also
	// closes the open request.
	t.Run("HookErrorClosesRequest", func(t *testing.T) {
		t.Parallel()
		run := runStructuredHookTurn(t, func(request agenthooks.Request) (int, string) {
			if request.Type == agenthooks.EventPostToolUse {
				return http.StatusInternalServerError, ""
			}
			return 0, ""
		}, schema, 0, finalize("OLD"))
		require.Equal(t, database.ChatStatusError, run.status)
		require.Len(t, run.receipts, 1)
		require.Equal(t, codersdk.ChatStructuredOutputErrorCodeGenerationFailed, run.receipts[0].Error.Code)
		require.Equal(t, chatstructured.ControlCandidate, run.controls[0].Kind, "the step commits before the error")
	})
}
