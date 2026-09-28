// Package agenttoolcall runs each chat tool call request at most once and
// cancels tool calls.
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

// entryMaxAge is how long a tool call is remembered. A retry after this
// runs the tool call again.
const entryMaxAge = time.Hour

type key struct {
	chatID uuid.UUID
	id     uuid.UUID
}

type entry struct {
	added    time.Time
	canceled bool
	// done is closed when the run finishes and resp is saved. It is nil
	// when a cancel arrived before any request.
	done chan struct{}
	resp workspacesdk.CancelToolCallResponse
}

// Table remembers tool calls by chat ID and tool call ID.
type Table struct {
	clock   quartz.Clock
	kill    func(ctx context.Context, id uuid.UUID)
	mu      sync.Mutex
	entries map[key]*entry
}

// New returns a Table. kill kills the process of a canceled tool call,
// if it has one.
func New(clock quartz.Clock, kill func(ctx context.Context, id uuid.UUID)) *Table {
	return &Table{clock: clock, kill: kill, entries: make(map[key]*entry)}
}

// add stores e under k and drops entries older than entryMaxAge.
// t.mu must be held.
func (t *Table) add(k key, e *entry) {
	e.added = t.clock.Now()
	for k, old := range t.entries {
		if e.added.Sub(old.added) > entryMaxAge {
			delete(t.entries, k)
		}
	}
	t.entries[k] = e
}

// Middleware runs a request whose chat context has a tool call ID once
// per chat and tool call ID, and answers repeats with the saved
// response. It refuses a canceled tool call with 409. Other requests
// pass through.
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
		if !found {
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
			rec := httptest.NewRecorder()
			next.ServeHTTP(rec, r)
			e.resp = workspacesdk.CancelToolCallResponse{
				Received:    true,
				Status:      rec.Code,
				ContentType: rec.Header().Get("Content-Type"),
				Body:        rec.Body.Bytes(),
			}
			close(e.done)
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

// Routes returns the HTTP handler for tool call routes.
func (t *Table) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/{id}/cancel", t.handleCancel)
	return r
}

// handleCancel cancels tool call {id}. A tool call the agent never
// received is refused from now on. Otherwise the cancel waits for the
// run to finish, kills its process, and answers the saved response.
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
	} else {
		e = &entry{canceled: true}
		t.add(k, e)
	}
	t.mu.Unlock()

	if e.done == nil {
		httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.CancelToolCallResponse{})
		return
	}
	// Kill after the run so a process that is still starting is killed.
	select {
	case <-e.done:
	case <-ctx.Done():
		return
	}
	t.kill(ctx, id)
	httpapi.Write(ctx, rw, http.StatusOK, e.resp)
}
