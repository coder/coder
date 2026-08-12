package agentcontext

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// SourceResponse is the on-wire representation of a Source.
// Matches the path-only RFC schema; future additions (tags,
// labels) can land additively without breaking clients.
type SourceResponse struct {
	Path string `json:"path"`
}

// SourceRequest is the request body for POST /sources.
type SourceRequest struct {
	Path string `json:"path"`
}

// SnapshotResource is the on-wire representation of a Resource.
// Payloads are omitted; clients that need the bytes go through
// the drpc PushContextState path.
type SnapshotResource struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	SourcePath  string `json:"source_path,omitempty"`
	ContentHash string `json:"content_hash"`
	SizeBytes   uint64 `json:"size_bytes"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// SnapshotResponse is the on-wire representation of a Snapshot
// returned by the resync endpoint.
type SnapshotResponse struct {
	Version       uint64             `json:"version"`
	AggregateHash string             `json:"aggregate_hash"`
	Resources     []SnapshotResource `json:"resources"`
	PayloadBytes  uint64             `json:"payload_bytes"`
	SnapshotError string             `json:"snapshot_error,omitempty"`
}

// API exposes the Manager over HTTP. The routes match the RFC:
//
//	GET    /api/v0/context/sources
//	POST   /api/v0/context/sources         { path }
//	GET    /api/v0/context/sources/{path}
//	DELETE /api/v0/context/sources/{path}
//	POST   /api/v0/context/resync
//
// {path} is URL-encoded canonical path. Callers pass either the
// canonical or original path; the handler canonicalizes before
// matching.
type API struct {
	manager *Manager
}

// NewAPI wraps the supplied Manager.
func NewAPI(m *Manager) *API {
	return &API{manager: m}
}

// Routes returns the chi handler for /api/v0/context/*. Mount
// it at "/api/v0/context".
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Route("/sources", func(r chi.Router) {
		r.Get("/", a.handleListSources)
		r.Post("/", a.handleAddSource)
		r.Get("/{path}", a.handleGetSource)
		r.Delete("/{path}", a.handleRemoveSource)
	})
	r.Post("/resync", a.handleResync)
	return r
}

func (a *API) handleListSources(rw http.ResponseWriter, r *http.Request) {
	sources := a.manager.Sources()
	out := make([]SourceResponse, 0, len(sources))
	for _, s := range sources {
		out = append(out, SourceResponse(s))
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, out)
}

func (a *API) handleAddSource(rw http.ResponseWriter, r *http.Request) {
	var req SourceRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	s, err := a.manager.AddSource(Source(req))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Could not add context source.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusCreated, SourceResponse(s))
}

func (a *API) handleGetSource(rw http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "path")
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid context source path.",
			Detail:  err.Error(),
		})
		return
	}
	canonical, ok := a.manager.HasSource(decoded)
	if !ok {
		httpapi.Write(r.Context(), rw, http.StatusNotFound, codersdk.Response{
			Message: "Context source not found.",
			Detail:  "No source registered for path " + strconv.Quote(decoded) + ".",
		})
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, SourceResponse{Path: canonical})
}

func (a *API) handleRemoveSource(rw http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "path")
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid context source path.",
			Detail:  err.Error(),
		})
		return
	}
	if err := a.manager.RemoveSource(decoded); err != nil {
		if errors.Is(err, ErrSourceNotFound) {
			httpapi.Write(r.Context(), rw, http.StatusNotFound, codersdk.Response{
				Message: "Context source not found.",
				Detail:  err.Error(),
			})
			return
		}
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Could not remove context source.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

func (a *API) handleResync(rw http.ResponseWriter, r *http.Request) {
	snap, err := a.manager.Resync(r.Context())
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		httpapi.Write(r.Context(), rw, status, codersdk.Response{
			Message: "Resync failed.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, snapshotResponse(snap))
}

// snapshotResponse converts a Snapshot to the JSON form returned by
// the resync endpoint. Payloads are omitted; the per-resource
// payload bytes ship via the drpc PushContextState path. Keep the
// per-resource field mapping in sync with contextSnapshotToProto in
// agent/agentsocket/service.go.
func snapshotResponse(s Snapshot) SnapshotResponse {
	out := SnapshotResponse{
		Version:       s.Version,
		AggregateHash: hex.EncodeToString(s.AggregateHash[:]),
		Resources:     make([]SnapshotResource, 0, len(s.Resources)),
		PayloadBytes:  s.PayloadBytes,
		SnapshotError: s.SnapshotError,
	}
	for _, r := range s.Resources {
		out.Resources = append(out.Resources, SnapshotResource{
			ID:          r.ID,
			Kind:        r.Kind.String(),
			Source:      r.Source,
			SourcePath:  r.SourcePath,
			ContentHash: hex.EncodeToString(r.ContentHash[:]),
			SizeBytes:   r.SizeBytes,
			Status:      r.Status.String(),
			Error:       r.Error,
			Name:        r.Name,
			Description: r.Description,
		})
	}
	return out
}
