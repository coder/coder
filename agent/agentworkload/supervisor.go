package agentworkload

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/websocket"
)

const (
	// exitedRetention is how long a finished workload's sandbox is kept, so
	// that its output can still be followed to the end.
	exitedRetention = 2 * time.Minute
	// outputTailBytes bounds how much of a workload's output is reported
	// to coderd when the workload is over.
	outputTailBytes = 16 << 10
)

// Policy is what the daemon allows a workload to ask for. The template sets
// it, so a sponsor chooses within what the template's author permits.
type Policy struct {
	// DefaultImage is used when a workload does not name an image. It is
	// always allowed.
	DefaultImage string
	// AllowedImages are patterns, as in path.Match, of other images a
	// workload may name.
	AllowedImages []string
	// DefaultNetwork is used when a workload does not name a network. Empty
	// means outbound.
	DefaultNetwork codersdk.WorkloadNetwork
	// AllowedNetworks are other networks a workload may name.
	AllowedNetworks []codersdk.WorkloadNetwork
	// AllowedMounts are directories of the workspace that may be shared
	// with a workload, together with everything below them. Without any,
	// a workload sees nothing of the workspace.
	AllowedMounts []string
	// Fixed means the sandbox exists before the workload does, as a pod
	// from a pool does. Its image and network are what they are: the
	// defaults describe them, and a workload cannot ask for others.
	Fixed bool
}

// mounts checks the directories a workload asks to see. Symbolic links are
// resolved first, so that a link cannot lead out of an allowed directory.
func (p Policy) mounts(requested []codersdk.WorkloadMount) ([]codersdk.WorkloadMount, error) {
	mounts := make([]codersdk.WorkloadMount, 0, len(requested))
	for _, mount := range requested {
		if !filepath.IsAbs(mount.Path) {
			return nil, xerrors.Errorf("the directory %q to share with the workload must be an absolute path", mount.Path)
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(mount.Path))
		if err != nil {
			return nil, xerrors.Errorf("the directory %q to share with the workload does not exist here", mount.Path)
		}
		allowed := false
		for _, root := range p.AllowedMounts {
			if root == "" {
				continue
			}
			root, err := filepath.EvalSymlinks(filepath.Clean(root))
			if err != nil {
				continue
			}
			if resolved == root || strings.HasPrefix(resolved, strings.TrimSuffix(root, "/")+"/") {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, xerrors.Errorf("this workspace's template does not allow sharing %q with workloads", mount.Path)
		}
		mounts = append(mounts, codersdk.WorkloadMount{Path: resolved, Writable: mount.Writable})
	}
	return mounts, nil
}

func (p Policy) image(requested string) (string, error) {
	if p.Fixed {
		if requested != "" && requested != p.DefaultImage {
			return "", xerrors.Errorf("the sandbox of this workload exists already and its image is %q; choose another template for another image", p.DefaultImage)
		}
		return p.DefaultImage, nil
	}
	if requested == "" || requested == p.DefaultImage {
		if p.DefaultImage == "" {
			return "", xerrors.New("this workspace's template sets no image for workloads")
		}
		return p.DefaultImage, nil
	}
	for _, pattern := range p.AllowedImages {
		if pattern == "" {
			continue
		}
		if ok, _ := path.Match(pattern, requested); ok {
			return requested, nil
		}
	}
	return "", xerrors.Errorf("this workspace's template does not allow the image %q for workloads", requested)
}

func (p Policy) network(requested codersdk.WorkloadNetwork) (codersdk.WorkloadNetwork, error) {
	def := p.DefaultNetwork
	if def == "" {
		def = codersdk.WorkloadNetworkOutbound
	}
	if requested == "" || requested == def {
		return def, nil
	}
	if p.Fixed {
		return "", xerrors.Errorf("the sandbox of this workload exists already and its network is %q; choose another template for another network", def)
	}
	if requested != codersdk.WorkloadNetworkNone && requested != codersdk.WorkloadNetworkOutbound {
		return "", xerrors.Errorf("unknown workload network %q", requested)
	}
	// Asking for less than the default is always fine.
	if requested == codersdk.WorkloadNetworkNone || slices.Contains(p.AllowedNetworks, requested) {
		return requested, nil
	}
	return "", xerrors.Errorf("this workspace's template does not allow the network %q for workloads", requested)
}

// Options configures a Supervisor.
type Options struct {
	Logger  slog.Logger
	Runtime Runtime
	Policy  Policy
	// CoderURL is where the daemon reaches coderd.
	CoderURL *url.URL
	// SessionToken returns the daemon's own credential. It is added to
	// forwarded requests and never reaches a sandbox.
	SessionToken func() string
	// Pending returns the workloads coderd has given this daemon and that
	// the daemon has not started, and waits for one when there is none.
	// The daemon asks coderd, and not the other way around, so that a
	// workload can start before coderd has a connection to the daemon,
	// which takes a daemon that has just started a moment to set up. Nil
	// leaves starting workloads to the daemon's own API.
	Pending func(ctx context.Context) ([]codersdk.AgentWorkloadSpec, error)
	// ReportStart tells coderd whether a workload from Pending runs.
	ReportStart func(ctx context.Context, id uuid.UUID, start codersdk.AgentWorkloadStart) error
	// ReportExit tells coderd that a workload is over. coderd records it
	// and revokes the workload's identity. Nil reports nothing; coderd then
	// finds out the next time it asks what runs.
	ReportExit func(ctx context.Context, id uuid.UUID, exit codersdk.AgentWorkloadExit) error
}

// Supervisor runs workloads for one workspace daemon.
type Supervisor struct {
	opts Options

	mu        sync.Mutex
	workloads map[uuid.UUID]*workload
	// given holds the workloads from coderd that are being started, or
	// that could not be, with the reason.
	given map[uuid.UUID]string

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
	// announced is closed once coderd knows that the workload runs. Its
	// end is reported after that, so that coderd hears of things in order.
	announced chan struct{}
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
	s := &Supervisor{
		opts:      opts,
		workloads: make(map[uuid.UUID]*workload),
		given:     make(map[uuid.UUID]string),
		closed:    make(chan struct{}),
	}
	if opts.Pending != nil && opts.ReportStart != nil {
		s.watchers.Add(1)
		go s.pull()
	}
	return s, nil
}

// pull starts the workloads coderd gives this daemon.
func (s *Supervisor) pull() {
	defer s.watchers.Done()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.closed:
			cancel()
		case <-ctx.Done():
		}
	}()
	pause := func(d time.Duration) {
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
	}
	for ctx.Err() == nil {
		specs, err := s.opts.Pending(ctx)
		if err != nil {
			// A daemon that waits to be claimed is not somebody yet, and
			// must notice at once when it becomes somebody. Asking again
			// is that wait: its credential is not handed out before.
			var refused *codersdk.Error
			if xerrors.As(err, &refused) && refused.StatusCode() == http.StatusUnauthorized {
				pause(50 * time.Millisecond)
				continue
			}
			// coderd is away.
			s.opts.Logger.Warn(ctx, "ask coderd for workloads", slog.Error(err))
			pause(time.Second)
			continue
		}
		if len(specs) > 0 {
			s.opts.Logger.Info(ctx, "coderd gave this daemon workloads", slog.F("count", len(specs)))
		}
		var starting sync.WaitGroup
		fresh := 0
		for _, spec := range specs {
			s.mu.Lock()
			reason, known := s.given[spec.ID]
			if !known {
				s.given[spec.ID] = ""
			}
			s.mu.Unlock()
			switch {
			case !known:
				fresh++
				starting.Add(1)
				go func() {
					defer starting.Done()
					s.startGiven(ctx, spec)
				}()
			case reason != "":
				// coderd has not heard that it could not be started.
				_ = s.opts.ReportStart(ctx, spec.ID, codersdk.AgentWorkloadStart{Error: reason})
			default:
			}
		}
		// A workload stays pending until coderd has heard of its start.
		// Give that a moment before asking again, and do not spin while a
		// slow start is under way.
		started := make(chan struct{})
		go func() {
			starting.Wait()
			close(started)
		}()
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
		}
		if len(specs) > 0 && fresh == 0 {
			pause(time.Second)
		}
	}
}

// startGiven starts a workload from coderd and reports how that went.
func (s *Supervisor) startGiven(ctx context.Context, spec codersdk.AgentWorkloadSpec) {
	announced := make(chan struct{})
	defer close(announced)
	start := codersdk.AgentWorkloadStart{}
	state, err := s.start(ctx, Spec{
		ID: spec.ID, Name: spec.Name, Command: spec.Command,
		Image: spec.Image, Network: spec.Network, Mounts: spec.Mounts, Interactive: spec.Interactive,
	}, announced)
	if err != nil {
		start.Error = err.Error()
		s.mu.Lock()
		s.given[spec.ID] = start.Error
		s.mu.Unlock()
	} else {
		start.State = &state
	}
	const attempts = 3
	for attempt := 1; attempt <= attempts; attempt++ {
		reportCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := s.opts.ReportStart(reportCtx, spec.ID, start)
		cancel()
		if err == nil {
			break
		}
		s.opts.Logger.Warn(ctx, "report workload start",
			slog.F("workload_id", spec.ID), slog.F("attempt", attempt), slog.Error(err))
		if attempt == attempts {
			break
		}
		select {
		case <-time.After(time.Second):
		case <-s.closed:
		}
	}
	if start.State != nil {
		// coderd no longer offers it. Should it be offered again, a start
		// report was lost and it is started anew, which is refused.
		s.mu.Lock()
		delete(s.given, spec.ID)
		s.mu.Unlock()
	}
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
	announced := make(chan struct{})
	close(announced)
	return s.start(ctx, spec, announced)
}

func (s *Supervisor) start(ctx context.Context, spec Spec, announced chan struct{}) (codersdk.AgentWorkloadState, error) {
	if spec.ID == uuid.Nil || spec.Command == "" {
		return codersdk.AgentWorkloadState{}, xerrors.New("a workload needs an identity and a command")
	}
	var err error
	if spec.Image, err = s.opts.Policy.image(spec.Image); err != nil {
		return codersdk.AgentWorkloadState{}, err
	}
	if spec.Network, err = s.opts.Policy.network(spec.Network); err != nil {
		return codersdk.AgentWorkloadState{}, err
	}
	if spec.Mounts, err = s.opts.Policy.mounts(spec.Mounts); err != nil {
		return codersdk.AgentWorkloadState{}, err
	}
	s.mu.Lock()
	if _, exists := s.workloads[spec.ID]; exists {
		s.mu.Unlock()
		return codersdk.AgentWorkloadState{}, xerrors.Errorf("workload %s already runs here", spec.ID)
	}
	// Reserve the identity so a concurrent request cannot start a second
	// sandbox for it.
	w := &workload{spec: spec, startedAt: time.Now(), done: make(chan struct{}), announced: announced}
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
		slog.F("workload_id", spec.ID), slog.F("name", spec.Name), slog.F("sandbox", sandbox.ID()),
		slog.F("image", spec.Image), slog.F("network", spec.Network))

	s.watchers.Add(1)
	go s.watch(w)
	return s.state(w), nil
}

// watch waits for a workload to end, tells coderd, and removes the sandbox
// once its output has had time to be read.
func (s *Supervisor) watch(w *workload) {
	defer s.watchers.Done()
	ctx := context.Background()
	id := w.spec.ID

	code, err := w.sandbox.Wait(ctx)
	exit := codersdk.AgentWorkloadExit{Reason: codersdk.WorkloadEndReasonExited, ExitCode: &code}
	select {
	case <-s.closed:
		// The daemon is shutting down and took the workload with it.
		exit = codersdk.AgentWorkloadExit{Reason: codersdk.WorkloadEndReasonStopped}
	default:
		if err != nil {
			// The sandbox is gone and nobody knows how its command ended.
			s.opts.Logger.Warn(ctx, "wait for workload", slog.F("workload_id", id), slog.Error(err))
			exit = codersdk.AgentWorkloadExit{Reason: codersdk.WorkloadEndReasonLost}
		}
	}
	s.mu.Lock()
	w.exitCode = code
	s.mu.Unlock()
	close(w.done)
	s.opts.Logger.Info(ctx, "workload is over",
		slog.F("workload_id", id), slog.F("reason", exit.Reason), slog.F("exit_code", code))

	exit.OutputTail = s.outputTail(w.sandbox)
	select {
	case <-w.announced:
	case <-time.After(30 * time.Second):
	case <-s.closed:
	}
	s.report(id, exit)

	select {
	case <-time.After(exitedRetention):
		_ = s.Remove(ctx, id)
	case <-s.closed:
	}
}

// outputTail returns the end of a finished workload's output.
func (*Supervisor) outputTail(sandbox Sandbox) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tail := &tailWriter{limit: outputTailBytes}
	_ = sandbox.Logs(ctx, tail)
	return tail.String()
}

// report tells coderd that a workload is over. It is best effort: coderd
// also notices by itself when it next asks the daemon what runs.
func (s *Supervisor) report(id uuid.UUID, exit codersdk.AgentWorkloadExit) {
	if s.opts.ReportExit == nil {
		return
	}
	const attempts = 3
	for attempt := 1; attempt <= attempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := s.opts.ReportExit(ctx, id, exit)
		cancel()
		if err == nil {
			return
		}
		s.opts.Logger.Warn(context.Background(), "report workload exit",
			slog.F("workload_id", id), slog.F("attempt", attempt), slog.Error(err))
		if attempt == attempts {
			return
		}
		select {
		case <-time.After(2 * time.Second):
		case <-s.closed:
			// The daemon is going away: retry without waiting.
		}
	}
}

// tailWriter keeps the last limit bytes written to it.
type tailWriter struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.limit {
		t.buf = t.buf[len(t.buf)-t.limit:]
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
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

// Close removes every workload. It tells coderd that they stopped with the
// daemon.
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
		Image:     w.spec.Image,
		Network:   w.spec.Network,
		Mounts:    w.spec.Mounts,
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
	r.Get("/{workload}/attach", s.handleAttach)
	r.Delete("/{workload}", s.handleRemove)
	return r
}

func (s *Supervisor) handleStart(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.AgentWorkloadSpec
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	state, err := s.Start(ctx, Spec{
		ID: req.ID, Name: req.Name, Command: req.Command,
		Image: req.Image, Network: req.Network, Mounts: req.Mounts, Interactive: req.Interactive,
	})
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

func (s *Supervisor) workloadParam(rw http.ResponseWriter, r *http.Request) (*workload, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "workload"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{Message: "The workload ID must be a UUID."})
		return nil, false
	}
	w, ok := s.get(id)
	if !ok {
		httpapi.ResourceNotFound(rw)
		return nil, false
	}
	return w, true
}

// handleLogs streams the workload's output as newline-delimited events and
// ends with its exit code.
func (s *Supervisor) handleLogs(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w, ok := s.workloadParam(rw, r)
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

// handleAttach connects the caller to the terminal of an interactive
// workload. It speaks the reconnecting PTY protocol over a websocket.
func (s *Supervisor) handleAttach(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w, ok := s.workloadParam(rw, r)
	if !ok {
		return
	}
	if !w.spec.Interactive {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "The workload was not started with a terminal.",
		})
		return
	}
	height, _ := strconv.ParseUint(r.URL.Query().Get("height"), 10, 16)
	width, _ := strconv.ParseUint(r.URL.Query().Get("width"), 10, 16)

	ws, err := websocket.Accept(rw, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	defer conn.Close()

	//nolint:gosec // A terminal's size fits.
	_ = w.sandbox.Attach(ctx, conn, uint16(height), uint16(width))
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
