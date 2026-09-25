package agentfiles

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agenttoolcall"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// fileResult is the HTTP response of an edit or write. With tool call
// headers it is recorded, replayed to repeated requests, and returned by
// cancel.
type fileResult struct {
	status int
	body   any
}

// readToolCall parses the tool call headers of r. found is false when r
// has none. ok is false when readToolCall wrote a 400: the headers are
// malformed, or they are present without chat context.
func readToolCall(ctx context.Context, rw http.ResponseWriter, r *http.Request) (key agenttoolcall.Key, age time.Duration, found, ok bool) {
	toolCall, found, err := workspacesdk.ToolCallFromHeaders(r.Header)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid tool call headers.",
			Detail:  err.Error(),
		})
		return agenttoolcall.Key{}, 0, false, false
	}
	if !found {
		return agenttoolcall.Key{}, 0, false, true
	}
	chatContext, hasChat := agentchat.FromContext(ctx)
	if !hasChat {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Tool call headers require the %s header.", workspacesdk.CoderChatIDHeader),
		})
		return agenttoolcall.Key{}, 0, false, false
	}
	key = agenttoolcall.Key{ChatID: chatContext.ID, MessageID: toolCall.MessageID, ToolCallID: toolCall.ID}
	return key, toolCall.Age, true, true
}

// runToolCall runs apply at most once for the tool call and writes its
// recorded response, or the 409 that refuses the request.
func (api *API) runToolCall(ctx context.Context, rw http.ResponseWriter, key agenttoolcall.Key, age time.Duration, input [sha256.Size]byte, apply func() fileResult) {
	res, err := api.toolCalls.Start(ctx, key, age, input, func() (fileResult, error) {
		return apply(), nil
	})
	if writeToolCallError(ctx, rw, err) {
		return
	}
	if err != nil {
		// The request ended while another request applied the tool call,
		// or apply panicked.
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to apply the tool call.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, res.status, res.body)
}

// handleCancelToolCall cancels an edit_files or write_file tool call. An
// edit or write in progress cannot be stopped, so it waits for it and
// returns the recorded response. A tool call the agent never received is
// recorded as canceled so that a request still in transit does nothing.
func (api *API) handleCancelToolCall(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	key, age, found, ok := readToolCall(ctx, rw, r)
	if !ok {
		return
	}
	if !found {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Canceling a tool call requires tool call headers.",
		})
		return
	}
	if want := workspacesdk.ToolCallUUID(key.ChatID, key.MessageID, key.ToolCallID).String(); id != want {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("ID %q is not the ID of the tool call in the headers.", id),
		})
		return
	}

	res, started, err := api.toolCalls.Cancel(ctx, key, age)
	// Cancel reports an aborted wait for an edit in progress on the same
	// path as a recorded failure. The client is gone, so nothing is
	// written.
	if err != nil && ctx.Err() != nil {
		return
	}
	if writeToolCallError(ctx, rw, err) {
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "The tool call failed without a response.",
			Detail:  err.Error(),
		})
		return
	}
	if !started {
		httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.CancelFileToolCallResponse{})
		return
	}
	body, err := json.Marshal(res.body)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to encode the recorded response.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.CancelFileToolCallResponse{
		Started:    true,
		StatusCode: res.status,
		Body:       body,
	})
}

// writeToolCallError writes the HTTP 409 for an agenttoolcall decision
// error and reports whether err was one.
func writeToolCallError(ctx context.Context, rw http.ResponseWriter, err error) bool {
	code, ok := agenttoolcall.ErrorCode(err)
	if !ok {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusConflict, workspacesdk.ToolCallError{
		Response: codersdk.Response{Message: err.Error()},
		Code:     code,
	})
	return true
}
