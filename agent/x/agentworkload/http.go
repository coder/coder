package agentworkload

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ServeHTTP handles the private /info, /stats, /history, /workloads,
// /workloads/{uuid}, and /drain routes. All requests require the token.
func (s *Supervisor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	hash := sha256.Sum256([]byte(token))
	if !ok || subtle.ConstantTimeCompare(hash[:], s.tokenHash[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "invalid supervisor credential")
		return
	}
	switch {
	case r.URL.Path == "/stats" && r.Method == http.MethodGet:
		s.statsHTTP(w)
	case r.URL.Path == "/history" && r.Method == http.MethodGet:
		s.historyHTTP(w, r)
	case r.URL.Path == "/info" && r.Method == http.MethodGet:
		s.mu.Lock()
		info := s.infoLocked()
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, info)
	case r.URL.Path == "/drain" && r.Method == http.MethodPost:
		s.mu.Lock()
		s.state.Draining = true
		err := s.persistLocked()
		s.mu.Unlock()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/workloads" && r.Method == http.MethodPost:
		s.startHTTP(w, r)
	case strings.HasPrefix(r.URL.Path, "/workloads/"):
		path := strings.TrimPrefix(r.URL.Path, "/workloads/")
		idPath, input := strings.CutSuffix(path, "/input")
		id, err := uuid.Parse(idPath)
		if err != nil || id == uuid.Nil {
			writeError(w, http.StatusBadRequest, "invalid workload ID")
			return
		}
		if input {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.inputHTTP(w, r, id)
			return
		}
		s.workloadHTTP(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (s *Supervisor) startHTTP(w http.ResponseWriter, r *http.Request) {
	var req codersdk.StartHostWorkloadRequest
	if err := decodeJSON(http.MaxBytesReader(w, r.Body, 1<<20), &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid workload request JSON")
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.OrganizationID == uuid.Nil || req.OwnerID == uuid.Nil || req.GenerationID == "" {
		writeError(w, http.StatusBadRequest, "organization_id, owner_id and generation_id are required")
		return
	}
	workload, status, err := s.admit(req)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, status, workload)
}

func (s *Supervisor) admit(req codersdk.StartHostWorkloadRequest) (codersdk.Workload, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Idempotent observation stays available while draining or failed closed.
	if previous, exists := s.state.Runs[req.ID]; exists {
		if !reflect.DeepEqual(previous.Request, req) {
			return codersdk.Workload{}, http.StatusConflict, xerrors.New("workload ID already exists with a different request")
		}
		return previous.Workload, http.StatusOK, nil
	}
	if s.state.Draining || s.failed || s.closed {
		return codersdk.Workload{}, http.StatusServiceUnavailable, xerrors.New("workload supervisor is draining or unavailable")
	}
	if req.GenerationID != s.generation.ID {
		return codersdk.Workload{}, http.StatusConflict, xerrors.New("prepared generation does not match")
	}
	if len(s.state.Runs) >= maxHistory {
		return codersdk.Workload{}, http.StatusServiceUnavailable, xerrors.New("workload history limit reached; idempotency keys cannot be evicted")
	}
	info := s.infoLocked()
	// Subtract overhead first to avoid overflow on hostile memory quantities.
	if req.CPU > info.AvailableCPU || req.MemoryBytes > info.AvailableMemoryBytes-runtimeOverhead {
		return codersdk.Workload{}, http.StatusTooManyRequests, xerrors.New("insufficient allocatable workload capacity including runtime overhead")
	}
	// Own the environment retained in history and read by the runtime.
	req.Env = maps.Clone(req.Env)
	run := &record{
		Request: req, Reserved: true, cancel: make(chan struct{}),
		Workload: codersdk.Workload{
			ID: req.ID, OrganizationID: req.OrganizationID, OwnerID: req.OwnerID,
			TemplateID: req.TemplateID, Generation: s.generation, CPU: req.CPU,
			MemoryBytes: req.MemoryBytes, State: "requested", CreatedAt: time.Now().UTC(),
			CleanupPending: true,
		},
		output: retainedOutput{Events: []codersdk.WorkloadEvent{}},
	}
	run.inputCtx, run.inputCancel = context.WithCancel(s.ctx)
	s.state.Runs[req.ID] = run
	// Commit the idempotency key and reservation before any Docker operation.
	// An ambiguous fsync retains the key in memory and fails admission closed.
	if err := s.persistLocked(); err != nil {
		s.unknownLocked(run, "workload admission could not be persisted; operator reconciliation required; command will not be replayed")
		return codersdk.Workload{}, http.StatusServiceUnavailable, err
	}
	s.wg.Add(1)
	go s.execute(run)
	return run.Workload, http.StatusCreated, nil
}

func (s *Supervisor) workloadHTTP(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if r.Method == http.MethodGet {
		var offset int64
		if raw := r.URL.Query().Get("offset"); raw != "" {
			var err error
			offset, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || offset < 0 {
				writeError(w, http.StatusBadRequest, "offset must be a nonnegative event offset")
				return
			}
		}
		if r.URL.Query().Get("follow") == "true" {
			s.streamOutput(w, r, id, offset)
			return
		}
		output, status, err := s.output(id, offset)
		if err != nil {
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, status, output)
		return
	}
	if r.Method == http.MethodDelete {
		status, err := s.cancelWorkload(id)
		if err != nil {
			writeError(w, status, err.Error())
			return
		}
		w.WriteHeader(status)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Supervisor) output(id uuid.UUID, offset int64) (codersdk.WorkloadOutput, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outputLocked(id, offset)
}

func (s *Supervisor) outputLocked(id uuid.UUID, offset int64) (codersdk.WorkloadOutput, int, error) {
	run, exists := s.state.Runs[id]
	if !exists {
		return codersdk.WorkloadOutput{}, http.StatusNotFound, xerrors.New("workload not found")
	}
	retained := run.output
	if run.outputOnDisk {
		if s.closed {
			return codersdk.WorkloadOutput{}, http.StatusServiceUnavailable, xerrors.New("workload supervisor is detached")
		}
		if err := s.store.read(outputName(id), 8<<20, &retained); err != nil || !retained.valid() {
			s.failed = true
			return codersdk.WorkloadOutput{}, http.StatusServiceUnavailable, xerrors.New("retained workload output is unavailable; operator reconciliation required")
		}
	}
	if offset < 0 || offset > retained.BaseOffset+int64(len(retained.Events)) {
		return codersdk.WorkloadOutput{}, http.StatusBadRequest, xerrors.New("offset exceeds retained output")
	}
	truncated := retained.Truncated || offset < retained.BaseOffset
	start := int(max(offset, retained.BaseOffset) - retained.BaseOffset)
	end, size := start, 0
	for end < len(retained.Events) && end-start < maxBatchEvents {
		n := len(retained.Events[end].Data)
		if size+n > maxBatchBytes {
			break
		}
		size += n
		end++
	}
	return codersdk.WorkloadOutput{
		Workload:   run.Workload,
		Events:     append([]codersdk.WorkloadEvent{}, retained.Events[start:end]...),
		NextOffset: retained.BaseOffset + int64(end), Truncated: truncated,
	}, http.StatusOK, nil
}

func (s *Supervisor) cancelWorkload(id uuid.UUID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.state.Runs[id]
	if !exists {
		return http.StatusNotFound, xerrors.New("workload not found")
	}
	if run.Workload.State == "unknown" || (run.Workload.Terminal() && run.Reserved) {
		return http.StatusConflict, xerrors.New("workload requires operator reconciliation; automatic retries are disabled")
	}
	if s.closed || s.failed {
		return http.StatusServiceUnavailable, xerrors.New("workload supervisor is detached or unavailable")
	}
	if !run.Workload.Terminal() && !run.CancelRequested {
		run.CancelRequested = true
		if err := s.persistLocked(); err != nil {
			return http.StatusServiceUnavailable, err
		}
		close(run.cancel)
		run.inputCancel()
	}
	return http.StatusNoContent, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	//nolint:errcheck // Headers are committed; an encoding failure cannot be sent.
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, codersdk.Response{Message: message})
}
