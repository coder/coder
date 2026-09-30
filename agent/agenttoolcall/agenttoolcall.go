// Package agenttoolcall deduplicates chat tool call requests and cancels
// tool calls.
package agenttoolcall

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// forgetAfter is how long an entry is kept after its last request or
// cancel. chatd resends unresolved tool calls on every retry, which keeps
// their entries.
const forgetAfter = time.Hour

type key struct {
	chatID uuid.UUID
	id     uuid.UUID
}

type entry struct {
	expiresAt time.Time
	canceled  bool
	done      chan struct{} // closed once resp is set; nil if a cancel created the entry
	resp      workspacesdk.CancelToolCallResponse
}

// Table remembers tool calls by chat ID and tool call ID.
type Table struct {
	clock   quartz.Clock
	cancel  func(ctx context.Context, chatID, id uuid.UUID)
	mu      sync.Mutex
	entries map[key]*entry
}

// New returns a Table that calls cancel to stop work a canceled tool call
// left running, such as a process.
func New(clock quartz.Clock, cancel func(ctx context.Context, chatID, id uuid.UUID)) *Table {
	return &Table{clock: clock, cancel: cancel, entries: make(map[key]*entry)}
}

// add stores e under k and deletes expired entries. t.mu must be held.
func (t *Table) add(k key, e *entry) {
	now := t.clock.Now()
	e.expiresAt = now.Add(forgetAfter)
	for k, old := range t.entries {
		if now.After(old.expiresAt) {
			delete(t.entries, k)
		}
	}
	t.entries[k] = e
}

// Middleware runs each tool call request once and responds to repeats
// with the saved response, or with 409 if the tool call was canceled.
func (t *Table) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		chat, _ := agentchat.FromContext(ctx)
		if chat.ToolCallID == uuid.Nil {
			next.ServeHTTP(rw, r)
			return
		}
		k := key{chatID: chat.ID, id: chat.ToolCallID}

		t.mu.Lock()
		e, found := t.entries[k]
		canceled := found && e.canceled
		if found {
			e.expiresAt = t.clock.Now().Add(forgetAfter)
		} else {
			e = &entry{done: make(chan struct{})}
			t.add(k, e)
		}
		t.mu.Unlock()

		if canceled {
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "Tool call was canceled.",
			})
			return
		}
		if !found {
			run(e, next, r)
		}
		select {
		case <-e.done:
		case <-ctx.Done():
			return
		}
		rw.Header().Set("Content-Type", e.resp.ContentType)
		rw.WriteHeader(e.resp.Status)
		_, _ = rw.Write(e.resp.Body)
	})
}

// run runs next and saves its response in e. If next panics, run saves a
// 500 so that waiters are released.
func run(e *entry, next http.Handler, r *http.Request) {
	rec := httptest.NewRecorder()
	defer func() {
		if e.resp.Status == 0 {
			rec = httptest.NewRecorder()
			httpapi.Write(r.Context(), rec, http.StatusInternalServerError, codersdk.Response{
				Message: "Tool call failed with an internal error. Its outcome is unknown.",
			})
			e.resp = savedResponse(rec)
		}
		close(e.done)
	}()
	next.ServeHTTP(rec, r)
	e.resp = savedResponse(rec)
}

func savedResponse(rec *httptest.ResponseRecorder) workspacesdk.CancelToolCallResponse {
	return workspacesdk.CancelToolCallResponse{
		Received:    true,
		Status:      rec.Code,
		ContentType: rec.Header().Get("Content-Type"),
		Body:        rec.Body.Bytes(),
	}
}

// Routes returns the handler for POST /{id}/cancel.
func (t *Table) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/{id}/cancel", t.handleCancel)
	return r
}

// handleCancel marks the tool call canceled, waits for its request, stops
// its work, and responds with the saved response. It calls t.cancel even
// without an entry, because a process can outlive its entry.
func (t *Table) handleCancel(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid tool call ID.",
			Detail:  err.Error(),
		})
		return
	}
	chat, _ := agentchat.FromContext(ctx)
	k := key{chatID: chat.ID, id: id}

	t.mu.Lock()
	e, found := t.entries[k]
	if found {
		e.canceled = true
		e.expiresAt = t.clock.Now().Add(forgetAfter)
	} else {
		e = &entry{canceled: true}
		t.add(k, e)
	}
	t.mu.Unlock()

	if e.done == nil {
		t.cancel(ctx, chat.ID, id)
		httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.CancelToolCallResponse{})
		return
	}
	// Wait so that t.cancel also stops a process the request is still
	// starting.
	select {
	case <-e.done:
	case <-ctx.Done():
		return
	}
	t.cancel(ctx, chat.ID, id)
	httpapi.Write(ctx, rw, http.StatusOK, e.resp)
}
