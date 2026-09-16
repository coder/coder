package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/pprof"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/coder/coder/examples/mcp-apps/pprof-lab/lab"
)

//go:embed ui/index.html
var indexHTML []byte

// maxBodyBytes caps the JSON body accepted by mutating routes.
const maxBodyBytes = 64 << 10

// Handler serves the control panel, the JSON API, and net/http/pprof for one
// registry of scenarios.
type Handler struct {
	reg     *lab.Registry
	started time.Time
	mux     *http.ServeMux
}

// NewHandler builds the HTTP handler for reg. started is reported as the
// uptime origin in /api/status.
func NewHandler(reg *lab.Registry, started time.Time) *Handler {
	h := &Handler{reg: reg, started: started, mux: http.NewServeMux()}

	h.mux.HandleFunc("GET /{$}", h.serveIndex)
	h.mux.HandleFunc("GET /api/status", h.serveStatus)
	h.mux.Handle("POST /api/scenarios/{name}/start", mutating(h.startScenario))
	h.mux.Handle("POST /api/scenarios/{name}/stop", mutating(h.stopScenario))
	h.mux.Handle("POST /api/reset", mutating(h.reset))

	// Importing net/http/pprof registers its handlers on
	// http.DefaultServeMux, which this program never serves. The same
	// handlers are mounted here on the lab mux under their usual paths.
	// pprof.Index serves the index page and dispatches the named profiles
	// (heap, goroutine, allocs, block, mutex, threadcreate) under it.
	h.mux.HandleFunc("/debug/pprof/", pprof.Index)
	h.mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	h.mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	h.mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	h.mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// mutating wraps a state-changing handler. It rejects cross-site requests
// identified by the browser's Sec-Fetch-Site header with 403 and requests
// without a JSON content type with 415. A simple cross-origin form or
// fetch cannot set application/json without a CORS preflight, which this
// server never answers.
func mutating(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeError(w, http.StatusForbidden, "cross-site requests are not allowed")
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next(w, r)
	})
}

func (*Handler) serveIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}

// scenarioStatus is the JSON shape of one scenario in API responses.
type scenarioStatus struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Knobs       []lab.Knob     `json:"knobs"`
	Running     bool           `json:"running"`
	State       map[string]any `json:"state"`
}

// memStatus is the subset of runtime.MemStats reported by /api/status.
type memStatus struct {
	HeapAlloc   uint64 `json:"heap_alloc"`
	HeapInuse   uint64 `json:"heap_inuse"`
	HeapObjects uint64 `json:"heap_objects"`
	NumGC       uint32 `json:"num_gc"`
}

// statusResponse is the body of GET /api/status.
type statusResponse struct {
	Scenarios     []scenarioStatus `json:"scenarios"`
	NumGoroutine  int              `json:"num_goroutine"`
	Mem           memStatus        `json:"mem"`
	UptimeSeconds float64          `json:"uptime_seconds"`
	GoVersion     string           `json:"go_version"`
}

func (h *Handler) serveStatus(w http.ResponseWriter, _ *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	all := h.reg.All()
	resp := statusResponse{
		Scenarios:    make([]scenarioStatus, 0, len(all)),
		NumGoroutine: runtime.NumGoroutine(),
		Mem: memStatus{
			HeapAlloc:   ms.HeapAlloc,
			HeapInuse:   ms.HeapInuse,
			HeapObjects: ms.HeapObjects,
			NumGC:       ms.NumGC,
		},
		UptimeSeconds: time.Since(h.started).Seconds(),
		GoVersion:     runtime.Version(),
	}
	for _, s := range all {
		resp.Scenarios = append(resp.Scenarios, describe(s))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) startScenario(w http.ResponseWriter, r *http.Request) {
	s, ok := h.reg.Get(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown scenario")
		return
	}

	knobs := map[string]any{}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&knobs); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "body must be a JSON object of knobs: "+err.Error())
		return
	}

	// The run must outlive this request, so its context is not derived
	// from r.Context().
	if err := s.Start(context.Background(), knobs); err != nil {
		if _, ok := errors.AsType[*lab.KnobError](err); ok {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("started %s with knobs %v", s.Name(), knobs)
	writeJSON(w, http.StatusOK, describe(s))
}

func (h *Handler) stopScenario(w http.ResponseWriter, r *http.Request) {
	s, ok := h.reg.Get(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown scenario")
		return
	}
	s.Stop()
	log.Printf("stopped %s", s.Name())
	writeJSON(w, http.StatusOK, describe(s))
}

// reset stops every scenario and returns freed memory to the OS so the next
// heap profile reflects a clean baseline. debug.FreeOSMemory runs a full GC
// before releasing memory.
func (h *Handler) reset(w http.ResponseWriter, _ *http.Request) {
	h.reg.StopAll()
	debug.FreeOSMemory()
	log.Printf("reset: all scenarios stopped")
	statuses := make([]scenarioStatus, 0, len(h.reg.All()))
	for _, s := range h.reg.All() {
		statuses = append(statuses, describe(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"scenarios": statuses})
}

func describe(s lab.Scenario) scenarioStatus {
	return scenarioStatus{
		Name:        s.Name(),
		Description: s.Description(),
		Knobs:       s.Knobs(),
		Running:     s.Running(),
		State:       s.State(),
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
