package agentworkload

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// exitedRetention is how long a finished workload's sandbox is kept, so that
// its output and exit code can still be read.
const exitedRetention = 2 * time.Minute

// Options configures a Supervisor.
type Options struct {
	Logger  slog.Logger
	Runtime Runtime
	// CoderURL is where the daemon reaches coderd.
	CoderURL *url.URL
	// SessionToken returns the daemon's own credential. It is added to
	// forwarded requests and never reaches a sandbox.
	SessionToken func() string
	// DefaultImage is used when a workload does not name an image.
	DefaultImage string
}

// Supervisor runs workloads for one workspace daemon.
type Supervisor struct {
	opts Options

	mu        sync.Mutex
	workloads map[uuid.UUID]*workload

	// closed ends the goroutines that watch workloads.
	closed    chan struct{}
	closeOnce sync.Once
	watchers  sync.WaitGroup
}

type workload struct {
	spec      Spec
	sandbox   Sandbox
	startedAt time.Time
	// done is closed when the command exits; exitCode is valid after that.
	done     chan struct{}
	exitCode int
}

// New returns a supervisor. It removes sandboxes an earlier daemon process
// left behind: their channels died with that process.
func New(ctx context.Context, opts Options) (*Supervisor, error) {
	if opts.Runtime == nil || opts.CoderURL == nil || opts.SessionToken == nil {
		return nil, xerrors.New("a runtime, a Coder URL and a session token are required")
	}
	if err := opts.Runtime.Sweep(ctx); err != nil {
		return nil, xerrors.Errorf("sweep %s sandboxes: %w", opts.Runtime.Name(), err)
	}
	return &Supervisor{opts: opts, workloads: make(map[uuid.UUID]*workload), closed: make(chan struct{})}, nil
}

// channel returns the handler that is the workload's only way to Coder. It
// forwards requests to coderd as that workload: the daemon's credential
// authenticates the daemon, and the header names the identity it vouches
// for. Nothing the workload sends can change either.
func (s *Supervisor) channel(id uuid.UUID) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(s.opts.CoderURL)
			pr.Out.Host = s.opts.CoderURL.Host
			// coderd reads credentials from all of these. A cookie or
			// query parameter would take precedence over the header set
			// below, so the workload's own are removed.
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
			query := pr.Out.URL.Query()
			query.Del(codersdk.SessionTokenCookie)
			query.Del("access_token")
			pr.Out.URL.RawQuery = query.Encode()
			pr.Out.Header.Set(codersdk.SessionTokenHeader, s.opts.SessionToken())
			pr.Out.Header.Set(codersdk.WorkloadIdentityHeader, id.String())
		},
		FlushInterval: -1,
	}
}

// Start runs a workload in a new sandbox.
func (s *Supervisor) Start(ctx context.Context, spec Spec) (codersdk.AgentWorkloadState, error) {
	if spec.ID == uuid.Nil || spec.Command == "" {
		return codersdk.AgentWorkloadState{}, xerrors.New("a workload needs an identity and a command")
	}
	if spec.Image == "" {
		spec.Image = s.opts.DefaultImage
	}
	s.mu.Lock()
	if _, exists := s.workloads[spec.ID]; exists {
		s.mu.Unlock()
		return codersdk.AgentWorkloadState{}, xerrors.Errorf("workload %s already runs here", spec.ID)
	}
	// Reserve the identity so a concurrent request cannot start a second
	// sandbox for it.
	w := &workload{spec: spec, startedAt: time.Now(), done: make(chan struct{})}
	s.workloads[spec.ID] = w
	s.mu.Unlock()

	sandbox, err := s.opts.Runtime.Start(ctx, spec, s.channel(spec.ID))
	if err != nil {
		s.mu.Lock()
		delete(s.workloads, spec.ID)
		s.mu.Unlock()
		return codersdk.AgentWorkloadState{}, err
	}
	s.mu.Lock()
	w.sandbox = sandbox
	s.mu.Unlock()
	s.opts.Logger.Info(ctx, "started workload",
		slog.F("workload_id", spec.ID), slog.F("name", spec.Name), slog.F("sandbox", sandbox.ID()))

	s.watchers.Add(1)
	go func() {
		defer s.watchers.Done()
		ctx := context.Background()
		code, err := sandbox.Wait(ctx)
		if err != nil {
			s.opts.Logger.Warn(ctx, "wait for workload", slog.F("workload_id", spec.ID), slog.Error(err))
		}
		s.mu.Lock()
		w.exitCode = code
		s.mu.Unlock()
		close(w.done)
		s.opts.Logger.Info(ctx, "workload command exited", slog.F("workload_id", spec.ID), slog.F("exit_code", code))

		select {
		case <-time.After(exitedRetention):
			_ = s.Remove(ctx, spec.ID)
		case <-s.closed:
		}
	}()
	return s.state(w), nil
}

// Remove stops a workload and removes its sandbox.
func (s *Supervisor) Remove(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	w, ok := s.workloads[id]
	delete(s.workloads, id)
	s.mu.Unlock()
	if !ok || w.sandbox == nil {
		return nil
	}
	return w.sandbox.Close(ctx)
}

// Close removes every workload.
func (s *Supervisor) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	defer s.watchers.Wait()
	s.mu.Lock()
	ids := make([]uuid.UUID, 0, len(s.workloads))
	for id := range s.workloads {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.Remove(context.Background(), id)
	}
	return nil
}

func (s *Supervisor) state(w *workload) codersdk.AgentWorkloadState {
	state := codersdk.AgentWorkloadState{
		ID:        w.spec.ID,
		Name:      w.spec.Name,
		Sandbox:   codersdk.WorkloadSandbox{Runtime: s.opts.Runtime.Name()},
		Running:   true,
		StartedAt: w.startedAt,
	}
	if w.sandbox != nil {
		state.Sandbox.ID = w.sandbox.ID()
	}
	select {
	case <-w.done:
		code := w.exitCode
		state.Running, state.ExitCode = false, &code
	default:
	}
	return state
}

func (s *Supervisor) get(id uuid.UUID) (*workload, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workloads[id]
	return w, ok && w.sandbox != nil
}

// Routes is the daemon's workload API. It is served on the agent's HTTP
// API, which only coderd and users who may connect to the workspace reach.
func (s *Supervisor) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/", s.handleStart)
	r.Get("/", s.handleList)
	r.Get("/{workload}/logs", s.handleLogs)
	r.Delete("/{workload}", s.handleRemove)
	return r
}

func (s *Supervisor) handleStart(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.AgentWorkloadSpec
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	state, err := s.Start(ctx, Spec{ID: req.ID, Name: req.Name, Command: req.Command, Image: req.Image})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "The workload could not be started.",
			Detail:  err.Error(),
		})
		return
	}
	httpapi.Write(ctx, rw, http.StatusCreated, state)
}

func (s *Supervisor) handleList(rw http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	states := make([]codersdk.AgentWorkloadState, 0, len(s.workloads))
	for _, w := range s.workloads {
		states = append(states, s.state(w))
	}
	s.mu.Unlock()
	httpapi.Write(r.Context(), rw, http.StatusOK, states)
}

func (s *Supervisor) workloadParam(rw http.ResponseWriter, r *http.Request) (*workload, uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: "The workload ID must be a UUID."})
		return nil, uuid.Nil, false
	}
	w, ok := s.get(id)
	if !ok {
		httpapi.ResourceNotFound(rw)
		return nil, id, false
	}
	return w, id, true
}

// handleLogs streams the workload's output as newline-delimited events and
// ends with its exit code.
func (s *Supervisor) handleLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w, _, ok := s.workloadParam(rw, r)
	if !ok {
		return
	}
	rw.Header().Set("Content-Type", "application/x-ndjson")
	rw.WriteHeader(http.StatusOK)
	flusher, _ := rw.(http.Flusher)
	encoder := json.NewEncoder(rw)
	send := func(event codersdk.WorkloadLogEvent) {
		_ = encoder.Encode(event)
		if flusher != nil {
			flusher.Flush()
		}
	}

	reader, writer := io.Pipe()
	go func() {
		_ = writer.CloseWithError(w.sandbox.Logs(ctx, writer))
	}()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		send(codersdk.WorkloadLogEvent{Output: scanner.Text()})
	}
	_ = reader.Close()

	select {
	case <-w.done:
		s.mu.Lock()
		code := w.exitCode
		s.mu.Unlock()
		send(codersdk.WorkloadLogEvent{ExitCode: &code})
	case <-ctx.Done():
	}
}

func (s *Supervisor) handleRemove(rw http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: "The workload ID must be a UUID."})
		return
	}
	if err := s.Remove(r.Context(), id); err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}
