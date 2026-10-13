// Package agentacp manages workspace-local ACP harnesses and sessions.
package agentacp

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// Errors classify agent API failures.
var (
	ErrInvalid     = xerrors.New("invalid ACP request")
	ErrNotFound    = xerrors.New("ACP session not found")
	ErrConflict    = xerrors.New("ACP request conflict")
	ErrUnavailable = xerrors.New("ACP harness unavailable")
)

const setupTimeout = 30 * time.Second

// Options supplies the workspace execution environment and lifecycle.
type Options struct {
	Logger     slog.Logger
	Clock      quartz.Clock
	Execer     agentexec.Execer
	Filesystem afero.Fs
	EnvInfo    usershell.EnvInfoer
	UpdateEnv  func([]string) ([]string, error)
	WorkingDir func() string
}

type initialization struct {
	request    workspacesdk.ACPCreateSessionRequest
	done       chan struct{}
	fsm        *SessionFSM
	err        error
	configHash [32]byte
	retryAfter time.Time
}

// Manager owns subprocesses, catalog snapshots, and in-memory transcripts.
type Manager struct {
	configHashes  map[string][32]byte
	reloadTrigger chan struct{}
	closedDone    chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	logger        slog.Logger
	clock         quartz.Clock
	execer        agentexec.Execer
	fs            afero.Fs
	envInfo       usershell.EnvInfoer
	updateEnv     func([]string) ([]string, error)
	workingDir    func() string
	mu            sync.Mutex
	closed        bool
	sessions      map[workspacesdk.ACPSessionID]*SessionFSM
	initializing  map[workspacesdk.ACPSessionID]*initialization
	instances     []*SessionFSM
	configs       map[string]harnessConfig
	retryAfter    map[string]time.Time
	configDir     string
	catalog       []workspacesdk.ACPHarness
	onChange      func()
	wg            sync.WaitGroup
	reloadMu      sync.Mutex
	runOnce       sync.Once
}

// NewManager creates a manager; RunDiscovery starts filesystem discovery.
// Clock, Execer, Filesystem, EnvInfo, and WorkingDir must be supplied.
// UpdateEnv is optional.
func NewManager(ctx context.Context, opts Options) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{ctx: ctx, cancel: cancel, logger: opts.Logger, clock: opts.Clock, execer: opts.Execer, fs: opts.Filesystem, envInfo: opts.EnvInfo, updateEnv: opts.UpdateEnv, workingDir: opts.WorkingDir, sessions: make(map[workspacesdk.ACPSessionID]*SessionFSM), closedDone: make(chan struct{}), reloadTrigger: make(chan struct{}, 1), initializing: make(map[workspacesdk.ACPSessionID]*initialization), configs: make(map[string]harnessConfig), configHashes: make(map[string][32]byte)}
}

// SetOnChange installs the callback invoked after discovery changes.
func (m *Manager) SetOnChange(fn func()) { m.mu.Lock(); defer m.mu.Unlock(); m.onChange = fn }

// run starts fn in a goroutine tracked by Close. It returns false if shutdown
// has begun. The manager lock makes starting work atomic with shutdown.
func (m *Manager) run(fn func()) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.wg.Go(fn)
	return true
}

func (m *Manager) timeout(ctx context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	timer := m.clock.AfterFunc(duration, cancel, "acp-timeout")
	return ctx, func() { timer.Stop(); cancel() }
}

func (m *Manager) validate(directory string) error {
	if directory == "" {
		return xerrors.New("workspace agent directory is unset")
	}
	if !filepath.IsAbs(directory) {
		return xerrors.New("workspace agent directory must be an absolute path")
	}
	info, err := m.fs.Stat(directory)
	if err != nil || !info.IsDir() {
		return xerrors.New("workspace agent directory is missing or is not a directory")
	}
	return nil
}

func (m *Manager) validateSessionRequest(slug, directory string) error {
	if slug == "" {
		return xerrors.Errorf("%w: harness slug is required", ErrInvalid)
	}
	if directory == "" || !filepath.IsAbs(directory) {
		return xerrors.Errorf("%w: working_directory must be an explicit absolute path", ErrInvalid)
	}
	info, err := m.fs.Stat(directory)
	if err != nil || !info.IsDir() {
		return xerrors.Errorf("%w: working_directory must be an existing directory", ErrInvalid)
	}
	return nil
}

// SessionManager returns the initialization and recovery interface.
func (m *Manager) SessionManager() SessionManager { return sessions{m} }

// SessionManager initializes sessions and replaces failed adapter runtimes.
// Existing failed instances remain readable until the manager closes.
type SessionManager interface {
	// Create returns a new initialized FSM in idle.
	Create(context.Context, workspacesdk.ACPCreateSessionRequest) (*SessionFSM, error)
	// Get restores absent sessions and returns existing errors without recovery.
	Get(context.Context, workspacesdk.ACPSessionID) (*SessionFSM, error)
	// Recover replaces a failed instance or returns an already recovered one.
	Recover(context.Context, workspacesdk.ACPSessionID) (*SessionFSM, error)
	// Close prevents initialization and cleans up owned runtimes.
	Close() error
}

// sessions separates initialization from the user-facing Manager operations.
type sessions struct{ manager *Manager }

type sessionAccess uint8

const (
	readSession sessionAccess = iota
	recoverSession
)

var _ SessionManager = sessions{}

func (sm sessions) Create(ctx context.Context, req workspacesdk.ACPCreateSessionRequest) (*SessionFSM, error) {
	m := sm.manager
	if err := m.validateSessionRequest(req.HarnessSlug, req.WorkingDirectory); err != nil {
		return nil, err
	}
	req.Config = maps.Clone(req.Config)
	if len(req.Config) == 0 {
		req.Config = nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrUnavailable
	}
	attempt := &initialization{request: req, done: make(chan struct{})}
	m.startInitialization(attempt, workspacesdk.ACPSessionID{}, nil)
	m.mu.Unlock()
	return awaitInitialization(ctx, attempt)
}

// Create initializes a dedicated idle adapter.
func (m *Manager) Create(ctx context.Context, req workspacesdk.ACPCreateSessionRequest) (workspacesdk.ACPSession, error) {
	s, err := (sessions{m}).Create(ctx, req)
	if err != nil {
		return workspacesdk.ACPSession{}, err
	}
	r, err := s.Read(ctx, nil)
	return r.Session, err
}

func awaitInitialization(ctx context.Context, attempt *initialization) (*SessionFSM, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-attempt.done:
		return attempt.fsm, attempt.err
	}
}

func (sm sessions) Get(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	return sm.obtain(ctx, id, readSession)
}

func (sm sessions) Recover(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	return sm.obtain(ctx, id, recoverSession)
}

func (sm sessions) obtain(ctx context.Context, id workspacesdk.ACPSessionID, access sessionAccess) (*SessionFSM, error) {
	m := sm.manager
	if id.SessionID == "" || id.HarnessSlug == "" || !filepath.IsAbs(id.WorkingDirectory) {
		return nil, ErrInvalid
	}
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, ErrUnavailable
		}
		s := m.sessions[id]
		// Reads retain the failed instance while recovery runs.
		if s != nil && access == readSession {
			m.mu.Unlock()
			return s, nil
		}
		attempt := m.initializing[id]
		if attempt != nil {
			select {
			case <-attempt.done:
				cfg := m.configs[id.HarnessSlug]
				if attempt.err != nil && attempt.configHash == cfg.hash && m.clock.Now().Before(attempt.retryAfter) {
					m.mu.Unlock()
					return awaitInitialization(ctx, attempt)
				}
			default:
				m.mu.Unlock()
				return awaitInitialization(ctx, attempt)
			}
		}
		m.mu.Unlock()
		if s != nil {
			r, err := s.Read(ctx, nil)
			if err != nil {
				return nil, err
			}
			if r.Session.Status != workspacesdk.ACPSessionStatusError {
				return s, nil
			}
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, ErrUnavailable
		}
		if m.sessions[id] != s || m.initializing[id] != attempt {
			m.mu.Unlock()
			continue
		}
		req := workspacesdk.ACPCreateSessionRequest{HarnessSlug: id.HarnessSlug, WorkingDirectory: id.WorkingDirectory}
		if s != nil {
			req = s.request
		}
		attempt = &initialization{request: req, done: make(chan struct{}), configHash: m.configs[id.HarnessSlug].hash}
		m.initializing[id] = attempt
		m.startInitialization(attempt, id, s)
		m.mu.Unlock()
		return awaitInitialization(ctx, attempt)
	}
}

func (sm sessions) Close() error { return sm.manager.Close() }

func (m *Manager) get(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	return (sessions{m}).Get(ctx, id)
}

func (m *Manager) recover(ctx context.Context, id workspacesdk.ACPSessionID) (*SessionFSM, error) {
	return (sessions{m}).Recover(ctx, id)
}

// startInitialization is called with m.mu held. Only initialized FSMs enter
// the registry; concurrent callers share the attempt, including its failure.
func (m *Manager) startInitialization(attempt *initialization, id workspacesdk.ACPSessionID, retired *SessionFSM) {
	m.wg.Go(func() {
		if retired != nil {
			retired.proc.close()
		}
		s, err := m.initialize(attempt.request, id.SessionID)
		m.mu.Lock()
		if err == nil {
			switch {
			case m.closed:
				err = ErrUnavailable
			case id.SessionID == "" && m.sessions[s.id] != nil:
				err = ErrConflict
			default:
				m.sessions[s.id] = s
				m.instances = append(m.instances, s)
				m.wg.Go(s.loop)
			}
		}
		m.mu.Unlock()
		if err == nil {
			if !m.run(func() { m.watchProcess(s) }) {
				err = ErrUnavailable
			}
		}
		if err != nil && s != nil {
			s.cancel()
			s.proc.close()
		}
		attempt.fsm, attempt.err = s, err
		if err != nil {
			attempt.fsm = nil
		}
		attempt.retryAfter = m.clock.Now().Add(discoveryInterval)
		close(attempt.done)
	})
}

// initialize restores the referenced conversation, never a replacement.
// History replay is consumed before the idle FSM is published.
func (m *Manager) initialize(req workspacesdk.ACPCreateSessionRequest, nativeID string) (*SessionFSM, error) {
	if err := m.validateSessionRequest(req.HarnessSlug, req.WorkingDirectory); err != nil {
		return nil, err
	}
	m.mu.Lock()
	cfg, found := m.configs[req.HarnessSlug]
	m.mu.Unlock()
	if !found {
		return nil, xerrors.Errorf("%w: harness is missing or disabled", ErrUnavailable)
	}
	cfg.directory = req.WorkingDirectory
	handler := &client{}
	p, err := m.launch(m.ctx, cfg, handler)
	if err != nil {
		return nil, xerrors.Errorf("%w: %v", ErrUnavailable, err)
	}
	ok := false
	defer func() {
		if !ok {
			p.close()
		}
	}()
	ctx, cancel := m.timeout(m.ctx, setupTimeout)
	defer cancel()
	init, err := p.conn.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber, ClientInfo: &acp.Implementation{Name: "coder-agent", Version: buildinfo.Version()}})
	if err != nil {
		return nil, xerrors.Errorf("%w: initialize harness: %v", ErrUnavailable, m.harnessFailure(p, err, cfg))
	}
	if init.ProtocolVersion != acp.ProtocolVersionNumber {
		return nil, xerrors.Errorf("%w: unsupported ACP protocol version", ErrUnavailable)
	}
	creating := nativeID == ""
	harness := harnessInfo(cfg, init)
	var options []acp.SessionConfigOption
	switch {
	case nativeID == "":
		created, err := p.conn.NewSession(ctx, acp.NewSessionRequest{Cwd: req.WorkingDirectory, McpServers: []acp.McpServer{}})
		if err != nil {
			return nil, xerrors.Errorf("%w: create ACP session: %v", ErrUnavailable, err)
		}
		if created.SessionId == "" {
			return nil, xerrors.Errorf("%w: harness returned an empty session id", ErrUnavailable)
		}
		nativeID, options = string(created.SessionId), created.ConfigOptions
	case harness.LoadSession:
		handler.beginReplay(nativeID)
		loaded, err := p.conn.LoadSession(ctx, acp.LoadSessionRequest{SessionId: acp.SessionId(nativeID), Cwd: req.WorkingDirectory, McpServers: []acp.McpServer{}})
		if err != nil {
			return nil, xerrors.Errorf("%w: load ACP session: %v", ErrUnavailable, err)
		}
		options = loaded.ConfigOptions
	case harness.ResumeSession:
		resumed, err := p.conn.ResumeSession(ctx, acp.ResumeSessionRequest{SessionId: acp.SessionId(nativeID), Cwd: req.WorkingDirectory, McpServers: []acp.McpServer{}})
		if err != nil {
			return nil, xerrors.Errorf("%w: resume ACP session: %v", ErrUnavailable, err)
		}
		options = resumed.ConfigOptions
	default:
		return nil, xerrors.Errorf("%w: harness does not support restoration", ErrUnavailable)
	}
	for _, id := range slices.Sorted(maps.Keys(req.Config)) {
		value := req.Config[id]
		if !validConfig(options, id, value) {
			return nil, xerrors.Errorf("%w: unsupported configuration %q=%q", ErrInvalid, id, value)
		}
		updated, err := p.conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{ValueId: &acp.SetSessionConfigOptionValueId{SessionId: acp.SessionId(nativeID), ConfigId: acp.SessionConfigId(id), Value: acp.SessionConfigValueId(value)}})
		if err != nil {
			return nil, xerrors.Errorf("%w: configure ACP session: %v", ErrInvalid, err)
		}
		options = updated.ConfigOptions
	}
	id := workspacesdk.ACPSessionID{HarnessSlug: req.HarnessSlug, WorkingDirectory: req.WorkingDirectory, SessionID: nativeID}
	info := workspacesdk.ACPSession{ID: id, Config: maps.Clone(req.Config), Status: workspacesdk.ACPSessionStatusIdle, Cursor: workspacesdk.ACPCursor{Epoch: uuid.New()}, HistoryComplete: creating || handler.replaying()}
	info.Harness, info.HarnessDisplayName = harness, cfg.displayName
	info.Harness.ConfigOptions = configOptions(options)
	s := newSessionFSM(m.ctx, req, info, p, m.run)
	handler.bind(s)
	ok = true
	return s, nil
}

func (m *Manager) watchProcess(s *SessionFSM) {
	p := s.proc
	select {
	case <-p.done:
	case <-p.conn.Done():
	case <-m.ctx.Done():
	}
	p.close()
	// TODO: The ACP SDK closes Done before queued notifications finish and
	// exposes no signal for their completion. Disconnect can put the FSM in
	// error before final updates arrive, causing those updates to be rejected.
	err := p.err
	if err == nil {
		err = xerrors.New("harness disconnected")
	}
	_ = s.adapterError(m.ctx, m.harnessFailure(p, err, harnessConfig{slug: s.id.HarnessSlug}))
}

func (m *Manager) harnessFailure(p *process, err error, cfg harnessConfig) error {
	m.logger.Warn(m.ctx, "acp harness failed", slog.F("harness", cfg.slug), slog.Error(err), slog.F("stderr", p.stderr.String()))
	return xerrors.New("harness failed; see workspace agent logs for details")
}

func validConfig(options []acp.SessionConfigOption, id, value string) bool {
	for _, opt := range configOptions(options) {
		if opt.ID == id {
			for _, v := range opt.Values {
				if v.ID == value {
					return true
				}
			}
		}
	}
	return false
}

// Message recovers failed sessions before submitting an FSM operation.
func (m *Manager) Message(ctx context.Context, id workspacesdk.ACPSessionID, req workspacesdk.ACPMessageRequest) (workspacesdk.ACPMessageResponse, error) {
	if req.ID == uuid.Nil || strings.TrimSpace(req.Text) == "" {
		return workspacesdk.ACPMessageResponse{}, ErrInvalid
	}
	s, err := m.get(ctx, id)
	if err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	r, err := s.Read(ctx, nil)
	if err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	if r.Session.Status == workspacesdk.ACPSessionStatusError {
		s, err = m.recover(ctx, id)
		if err != nil {
			return workspacesdk.ACPMessageResponse{}, err
		}
	}
	reply, err := s.Message(ctx, req)
	if err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	select {
	case <-ctx.Done():
		return workspacesdk.ACPMessageResponse{}, ctx.Err()
	case result := <-reply:
		return result.Message, result.Error
	}
}

// Interrupt obtains a session without recovering failures, then writes cancel.
func (m *Manager) Interrupt(ctx context.Context, id workspacesdk.ACPSessionID) (workspacesdk.ACPSession, error) {
	s, err := m.get(ctx, id)
	if err != nil {
		return workspacesdk.ACPSession{}, err
	}
	reply, err := s.Interrupt(ctx)
	if err != nil {
		return workspacesdk.ACPSession{}, err
	}
	select {
	case <-ctx.Done():
		return workspacesdk.ACPSession{}, ctx.Err()
	case result := <-reply:
		return result.Session, result.Error
	}
}

// Read preserves failed instances and their partial output.
func (m *Manager) Read(ctx context.Context, id workspacesdk.ACPSessionID, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, error) {
	s, err := m.get(ctx, id)
	if err != nil {
		return workspacesdk.ACPSessionResponse{}, err
	}
	return s.Read(ctx, after)
}

// List returns atomic snapshots of initialized runtime sessions.
func (m *Manager) List() []workspacesdk.ACPSession {
	m.mu.Lock()
	instances := slices.Collect(maps.Values(m.sessions))
	m.mu.Unlock()
	out := []workspacesdk.ACPSession{}
	for _, s := range instances {
		r, err := s.Read(m.ctx, nil)
		if err == nil {
			out = append(out, r.Session)
		}
	}
	slices.SortFunc(out, func(a, b workspacesdk.ACPSession) int {
		if a.ID.HarnessSlug != b.ID.HarnessSlug {
			return strings.Compare(a.ID.HarnessSlug, b.ID.HarnessSlug)
		}
		if a.ID.WorkingDirectory != b.ID.WorkingDirectory {
			return strings.Compare(a.ID.WorkingDirectory, b.ID.WorkingDirectory)
		}
		return strings.Compare(a.ID.SessionID, b.ID.SessionID)
	})
	return out
}

// Subscribe replays an atomic baseline and streams changes outside the FSM.
// Slow subscribers reconnect using their last cursor.
func (m *Manager) Subscribe(ctx context.Context, id workspacesdk.ACPSessionID, after *workspacesdk.ACPCursor) ([]workspacesdk.ACPEvent, <-chan workspacesdk.ACPEvent, func(), error) {
	s, err := m.get(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	r, changed, err := s.Watch(ctx, after)
	if err != nil {
		return nil, nil, nil, err
	}
	info := r.Session
	r.Events = append(r.Events, workspacesdk.ACPEvent{Kind: workspacesdk.ACPEventKindSnapshot, Cursor: info.Cursor, Session: &info})
	watchCtx, cancel := context.WithCancel(ctx)
	ch := make(chan workspacesdk.ACPEvent, 64)
	if !m.run(func() {
		defer close(ch)
		defer cancel()
		cursor := info.Cursor
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-m.ctx.Done():
				return
			case <-changed:
			}
			snapshot, next, err := s.Watch(watchCtx, &cursor)
			if xerrors.Is(err, ErrConflict) {
				snapshot, next, err = s.Watch(watchCtx, nil)
			}
			if err != nil {
				return
			}
			for _, event := range snapshot.Events {
				select {
				case ch <- event:
				default:
					return
				}
			}
			cursor, changed = snapshot.Session.Cursor, next
		}
	}) {
		cancel()
		return nil, nil, nil, ErrUnavailable
	}
	return r.Events, ch, cancel, nil
}

// Close prevents initialization and stops all current and retired runtimes.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		<-m.closedDone
		return nil
	}
	m.closed = true
	m.cancel()
	instances := slices.Clone(m.instances)
	m.mu.Unlock()
	for _, s := range instances {
		s.proc.close()
	}
	m.wg.Wait()
	close(m.closedDone)
	return nil
}

func harnessInfo(cfg harnessConfig, init acp.InitializeResponse) workspacesdk.ACPHarness {
	steering, _ := init.Meta["steering"].(map[string]any)
	supported, _ := steering["supported"].(bool)
	return workspacesdk.ACPHarness{Slug: cfg.slug, DisplayName: cfg.displayName, LoadSession: init.AgentCapabilities.LoadSession, ResumeSession: init.AgentCapabilities.SessionCapabilities.Resume != nil, Steering: supported, ConfigOptions: []workspacesdk.ACPConfigOption{}}
}

func configOptions(options []acp.SessionConfigOption) []workspacesdk.ACPConfigOption {
	out := []workspacesdk.ACPConfigOption{}
	for _, option := range options {
		s := option.Select
		if s == nil {
			continue
		}
		opt := workspacesdk.ACPConfigOption{ID: string(s.Id), Name: s.Name, CurrentValue: string(s.CurrentValue), Values: []workspacesdk.ACPConfigValue{}}
		if s.Category != nil {
			opt.Category = string(*s.Category)
		}
		if s.Description != nil {
			opt.Description = *s.Description
		}
		add := func(v acp.SessionConfigSelectOption, group, groupName string) {
			value := workspacesdk.ACPConfigValue{ID: string(v.Value), Name: v.Name, Group: group, GroupName: groupName}
			if v.Description != nil {
				value.Description = *v.Description
			}
			opt.Values = append(opt.Values, value)
		}
		if s.Options.Ungrouped != nil {
			for _, v := range *s.Options.Ungrouped {
				add(v, "", "")
			}
		}
		if s.Options.Grouped != nil {
			for _, group := range *s.Options.Grouped {
				for _, v := range group.Options {
					add(v, string(group.Group), group.Name)
				}
			}
		}
		out = append(out, opt)
	}
	return out
}

func cloneEvent(e workspacesdk.ACPEvent) workspacesdk.ACPEvent {
	e.Update = slices.Clone(e.Update)
	if e.MessageID != nil {
		id := *e.MessageID
		e.MessageID = &id
	}
	if e.Session != nil {
		info := *e.Session
		info.Config = maps.Clone(info.Config)
		info.Harness = cloneHarness(info.Harness)
		e.Session = &info
	}
	return e
}

func cloneHarness(h workspacesdk.ACPHarness) workspacesdk.ACPHarness {
	h.ConfigOptions = slices.Clone(h.ConfigOptions)
	for i := range h.ConfigOptions {
		h.ConfigOptions[i].Values = slices.Clone(h.ConfigOptions[i].Values)
	}
	return h
}
