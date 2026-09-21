package agentacp

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Routes serves session operations over the authenticated workspace connection.
func (m *Manager) Routes() http.Handler {
	r := chi.NewRouter()
	r.Route("/{organization}/{parent}", func(r chi.Router) {
		r.Post("/", m.serve)
		r.Get("/", m.serve)
		r.Route("/{session}", func(r chi.Router) {
			r.Get("/", m.serve)
			r.Get("/stream", m.serve)
			r.Get("/wait", m.serve)
			r.Post("/messages", m.serve)
			r.Post("/interrupt", m.serve)
		})
	})
	return r
}

func (m *Manager) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	organization, e1 := uuid.Parse(chi.URLParam(r, "organization"))
	parent, e2 := uuid.Parse(chi.URLParam(r, "parent"))
	if e1 != nil || e2 != nil {
		httpapi.Write(ctx, w, 400, codersdk.Response{Message: "Invalid parent identity."})
		return
	}
	fail := func(code int, err error) { httpapi.Write(ctx, w, code, codersdk.Response{Message: err.Error()}) }
	rawID := chi.URLParam(r, "session")
	if rawID == "" {
		if r.Method == http.MethodPost {
			var req codersdk.ACPSpawnRequest
			if !httpapi.Read(ctx, w, r, &req) {
				return
			}
			result, err := m.Spawn(ctx, parent, organization, req)
			if err != nil {
				fail(400, err)
				return
			}
			httpapi.Write(ctx, w, 201, result)
			return
		}
		all := m.list(parent, organization)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 10
		}
		limit = min(limit, 50)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		offset = min(max(offset, 0), len(all))
		end := min(offset+limit, len(all))
		httpapi.Write(ctx, w, 200, codersdk.ACPListResponse{Agents: all[offset:end], Total: len(all), HasMore: end < len(all)})
		return
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		fail(400, err)
		return
	}
	s, err := m.lookup(id, parent, organization)
	if err != nil {
		fail(404, err)
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/messages"):
		var req codersdk.ACPMessageRequest
		if !httpapi.Read(ctx, w, r, &req) {
			return
		}
		if err = s.message(req.Message, req.Interrupt); err != nil {
			fail(409, err)
			return
		}
	case strings.HasSuffix(r.URL.Path, "/interrupt"):
		if err = s.interrupt(); err != nil {
			fail(502, err)
			return
		}
	case strings.HasSuffix(r.URL.Path, "/wait"):
		seconds, _ := strconv.Atoi(r.URL.Query().Get("timeout_seconds"))
		if seconds <= 0 {
			seconds = 300
		}
		timer := m.clock.NewTimer(time.Duration(min(seconds, 3600))*time.Second, "acp", "wait")
		defer timer.Stop()
		for {
			s.mu.Lock()
			state := s.snapshotLocked()
			changed := s.changed
			s.mu.Unlock()
			if state.Status == "waiting" || state.Status == "error" {
				httpapi.Write(ctx, w, 200, state)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				httpapi.Write(ctx, w, 200, s.snapshot())
				return
			case <-changed:
			}
		}
	case strings.HasSuffix(r.URL.Path, "/stream"):
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx = conn.CloseRead(ctx)
		for {
			s.mu.Lock()
			state := s.snapshotLocked()
			changed := s.changed
			s.mu.Unlock()
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = wsjson.Write(writeCtx, conn, state)
			cancel()
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-m.ctx.Done():
				return
			case <-changed:
			}
		}
	}
	httpapi.Write(ctx, w, 200, s.snapshot())
}
