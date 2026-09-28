package agenttoolcall

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// CancelHandler serves POST /api/v0/tool-calls/{id}/cancel, where {id} is
// the tool call UUID derived from the request's chat and tool call
// headers. It answers 204 once the tool call's outcome is final: for a
// tool call the agent never received, it stores a canceled marker so that
// a request still in transit does nothing; for a tool call with a record,
// it waits for the run and then for the work the run's cancel hook stops.
// The request has no body.
func (s *Store) CancelHandler() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		key, present, badRequest := toolCallFromRequest(r)
		if !present {
			badRequest = &codersdk.Response{Message: "Canceling a tool call requires tool call headers."}
		}
		if badRequest != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, *badRequest)
			return
		}
		// The ID is derived from the request's own chat, so a request
		// cannot address another chat's tool call.
		id := workspacesdk.ToolCallUUID(key.ChatID, key.MessageID, key.ToolName, key.ToolCallID).String()
		if chi.URLParam(r, "id") != id {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "The tool call ID in the path is not the ID of the tool call in the headers.",
			})
			return
		}

		// A done request context means the client is gone, so this
		// handler returns without writing wherever it sees one.
		rec := s.cancel(key)
		if rec == nil {
			rw.WriteHeader(http.StatusNoContent)
			return
		}
		if !waitDone(ctx, rec) {
			return
		}
		hook, mustRun := s.startHook(rec)
		if hook == nil {
			rw.WriteHeader(http.StatusNoContent)
			return
		}
		if mustRun {
			hook.run()
		}
		select {
		case <-hook.ran:
		case <-ctx.Done():
			return
		}
		if hook.err != nil && hook.running() {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to stop the tool call's work.",
				Detail:  hook.err.Error(),
			})
			return
		}
		select {
		case <-hook.stopped:
			rw.WriteHeader(http.StatusNoContent)
		case <-ctx.Done():
		}
	}
}
