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

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// ErrManagerClosed is returned when the manager has been closed.
var ErrManagerClosed = xerrors.New("ACP manager closed")

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
	// UpdateEnv optionally enriches the inherited environment with workspace
	// variables and secrets. Discovery calls it once per reload and uses the
	// result for both cache invalidation and harness subprocesses. Session
	// creation calls it again to use the current environment.
	UpdateEnv  func([]string) ([]string, error)
	WorkingDir func() string
}

// Manager owns harness discovery and the session registry.
// Each SessionFSM owns its process, tasks, operations, status, and transcript.
type Manager struct {
	configHashes map[string][32]byte
	closedDone   chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	logger       slog.Logger
	clock        quartz.Clock
	execer       agentexec.Execer
	fs           afero.Fs
	envInfo      usershell.EnvInfoer
	updateEnv    func([]string) ([]string, error)
	workingDir   func() string
	mu           sync.Mutex
	closed       bool
	sessions     map[workspacesdk.ACPSessionID]*SessionFSM
	initializing map[workspacesdk.ACPSessionID]*initialization
	configs      map[string]harnessConfig
	configDir    string
	discoveryDir string
	catalog      []workspacesdk.ACPHarness
	catalogEnv   []string
	onChange     func()
	wg           sync.WaitGroup
	reloadMu     sync.Mutex
	runOnce      sync.Once
}

// NewManager creates a manager; RunDiscovery starts filesystem discovery.
// Clock, Execer, Filesystem, EnvInfo, and WorkingDir must be supplied.
// UpdateEnv is optional and runs once per discovery reload or session launch.
func NewManager(ctx context.Context, opts Options) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{
		ctx:          ctx,
		cancel:       cancel,
		logger:       opts.Logger,
		clock:        opts.Clock,
		execer:       opts.Execer,
		fs:           opts.Filesystem,
		envInfo:      opts.EnvInfo,
		updateEnv:    opts.UpdateEnv,
		workingDir:   opts.WorkingDir,
		sessions:     make(map[workspacesdk.ACPSessionID]*SessionFSM),
		closedDone:   make(chan struct{}),
		initializing: make(map[workspacesdk.ACPSessionID]*initialization),
		configs:      make(map[string]harnessConfig),
		configHashes: make(map[string][32]byte),
	}
}

// SetOnChange installs the callback invoked after discovery changes.
func (m *Manager) SetOnChange(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

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

func timeout(ctx context.Context, clock quartz.Clock, duration time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	timer := clock.AfterFunc(duration, cancel, "acp-timeout")
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

// Create initializes a dedicated idle adapter.
func (m *Manager) Create(ctx context.Context, req workspacesdk.ACPCreateSessionRequest) (workspacesdk.ACPSession, error) {
	s, err := m.createSession(ctx, req)
	if err != nil {
		return workspacesdk.ACPSession{}, err
	}
	r, err := s.Read(ctx, nil)
	if err != nil {
		m.discardSession(s)
		return workspacesdk.ACPSession{}, err
	}
	return r.Session, nil
}

// Message submits a prompt to an existing idle session.
// If the session failed, Message attempts to recover it before submitting the prompt.
func (m *Manager) Message(ctx context.Context, id workspacesdk.ACPSessionID, req workspacesdk.ACPMessageRequest) (workspacesdk.ACPMessageResponse, error) {
	if strings.TrimSpace(req.Text) == "" {
		return workspacesdk.ACPMessageResponse{}, ErrInvalid
	}
	s, err := m.recover(ctx, id)
	if err != nil {
		return workspacesdk.ACPMessageResponse{}, err
	}
	return s.Message(ctx, req)
}

// Interrupt obtains a session without recovering failures, then writes cancel.
func (m *Manager) Interrupt(ctx context.Context, id workspacesdk.ACPSessionID) (workspacesdk.ACPSession, error) {
	s, err := m.get(ctx, id)
	if err != nil {
		return workspacesdk.ACPSession{}, err
	}
	return s.Interrupt(ctx)
}

// Read preserves failed instances and their partial output.
func (m *Manager) Read(ctx context.Context, id workspacesdk.ACPSessionID, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, error) {
	s, err := m.get(ctx, id)
	if err != nil {
		return workspacesdk.ACPSessionResponse{}, err
	}
	return s.Read(ctx, after)
}

// Close prevents initialization and stops all session runtimes.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		<-m.closedDone
		return nil
	}
	m.closed = true
	m.cancel()
	sessions := slices.Collect(maps.Values(m.sessions))
	m.sessions = nil
	m.mu.Unlock()
	for _, s := range sessions {
		s.Close()
	}
	m.wg.Wait()
	close(m.closedDone)
	return nil
}

// newHarness keeps config_options an empty JSON array instead of null, including
// when discovery or probing returns an error.
func newHarness(slug, displayName string) workspacesdk.ACPHarness {
	return workspacesdk.ACPHarness{
		Slug:          slug,
		DisplayName:   displayName,
		ConfigOptions: []workspacesdk.ACPConfigOption{},
	}
}

func harnessInfo(cfg harnessConfig, init acp.InitializeResponse) workspacesdk.ACPHarness {
	h := newHarness(cfg.slug, cfg.displayName)
	h.LoadSession = init.AgentCapabilities.LoadSession
	h.ResumeSession = init.AgentCapabilities.SessionCapabilities.Resume != nil
	return h
}

func configOptions(options []acp.SessionConfigOption) []workspacesdk.ACPConfigOption {
	out := []workspacesdk.ACPConfigOption{}
	for _, option := range options {
		s := option.Select
		if s == nil {
			continue
		}
		opt := workspacesdk.ACPConfigOption{
			ID:           string(s.Id),
			Name:         s.Name,
			CurrentValue: string(s.CurrentValue),
			Values:       []workspacesdk.ACPConfigValue{},
		}
		if s.Category != nil {
			opt.Category = string(*s.Category)
		}
		if s.Description != nil {
			opt.Description = *s.Description
		}
		add := func(v acp.SessionConfigSelectOption, group, groupName string) {
			value := workspacesdk.ACPConfigValue{
				ID:        string(v.Value),
				Name:      v.Name,
				Group:     group,
				GroupName: groupName,
			}
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

// Watch atomically returns a snapshot and a stream of subsequent events.
// The stream stays open across turns. Callers must cancel ctx when finished.
func (m *Manager) Watch(ctx context.Context, id workspacesdk.ACPSessionID, after *workspacesdk.ACPCursor) (workspacesdk.ACPSessionResponse, <-chan workspacesdk.ACPEvent, error) {
	s, err := m.get(ctx, id)
	if err != nil {
		return workspacesdk.ACPSessionResponse{}, nil, err
	}
	return s.Watch(ctx, after)
}
