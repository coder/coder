package agenttoolcall

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ProcessStopper stops the process a tool call started, under the run-age
// rule: a running process whose run age is below stopIfRunAgeBelow is
// killed, and any other process is left as it is. found is false when no
// process has processID.
type ProcessStopper interface {
	StopToolCallProcess(ctx context.Context, processID string, stopIfRunAgeBelow time.Duration) (state workspacesdk.ToolCallProcess, found bool, err error)
}

// CancelHandler serves POST /api/v0/tool-calls/{id}/cancel, where {id} is
// the tool call UUID. For a tool call the agent never received, it records
// the tool call as canceled so that a request still in transit does
// nothing. For a tool call that ran, it waits for a run in progress and
// answers the recorded response, plus the state of the tool call's process
// after asking stopper to stop it. stopper may be nil for agents without
// processes.
func (s *Store) CancelHandler(stopper ProcessStopper) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		key, age, present, badRequest := toolCallFromRequest(r)
		if !present {
			badRequest = &codersdk.Response{Message: "Canceling a tool call requires tool call headers."}
		}
		if badRequest != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, *badRequest)
			return
		}
		// The ID is derived from the request's own chat, so a request
		// cannot address another chat's tool call.
		id := workspacesdk.ToolCallUUID(key.ChatID, key.MessageID, key.ToolCallID).String()
		if pathID := chi.URLParam(r, "id"); pathID != id {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "The tool call ID in the path is not the ID of the tool call in the headers.",
			})
			return
		}
		var req workspacesdk.CancelToolCallRequest
		if !httpapi.Read(ctx, rw, r, &req) {
			return
		}

		rec, received, err := s.cancel(key, age)
		if writeToolCallError(ctx, rw, err) {
			return
		}
		if !received || rec.canceled {
			httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.CancelToolCallResponse{})
			return
		}
		// A finished request context means the client is gone, so this
		// handler returns without writing wherever it sees one.
		resp, ok := rec.wait(ctx)
		if !ok {
			return
		}
		out := workspacesdk.CancelToolCallResponse{
			Started:     true,
			StatusCode:  resp.status,
			ContentType: resp.contentType,
			Body:        resp.body,
		}
		if stopper != nil {
			state, found, err := stopper.StopToolCallProcess(ctx, id, stopThreshold(req.StopIfRunAgeBelowMs))
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Failed to stop the tool call's process.",
					Detail:  err.Error(),
				})
				return
			}
			if found {
				out.Process = &state
			}
		}
		rw.Header().Set(workspacesdk.CoderToolCallRunAgeMsHeader, strconv.FormatInt(s.runAge(rec).Milliseconds(), 10))
		httpapi.Write(ctx, rw, http.StatusOK, out)
	}
}

// stopThreshold converts stop_if_run_age_below_ms to a Duration. A
// negative value never stops, like 0, and a huge one saturates instead of
// overflowing.
func stopThreshold(ms int64) time.Duration {
	return time.Duration(min(max(ms, 0), math.MaxInt64/int64(time.Millisecond))) * time.Millisecond
}
