// Package agentacp manages workspace-local ACP harnesses.
package agentacp

import (
	"context"
	"path/filepath"
	"slices"
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

// Errors classify harness discovery failures.
var (
	ErrInvalid     = xerrors.New("invalid ACP request")
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

// Manager owns discovery subprocesses and catalog snapshots.
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
	configs       map[string]harnessConfig
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
	return &Manager{ctx: ctx, cancel: cancel, logger: opts.Logger, clock: opts.Clock, execer: opts.Execer, fs: opts.Filesystem, envInfo: opts.EnvInfo, updateEnv: opts.UpdateEnv, workingDir: opts.WorkingDir, closedDone: make(chan struct{}), reloadTrigger: make(chan struct{}, 1), configs: make(map[string]harnessConfig), configHashes: make(map[string][32]byte)}
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

func (m *Manager) validate(slug, directory string) error {
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

// Close stops discovery subprocesses and all manager-owned goroutines.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		<-m.closedDone
		return nil
	}
	m.closed = true
	m.cancel()
	m.mu.Unlock()
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

func cloneHarness(h workspacesdk.ACPHarness) workspacesdk.ACPHarness {
	h.ConfigOptions = slices.Clone(h.ConfigOptions)
	for i := range h.ConfigOptions {
		h.ConfigOptions[i].Values = slices.Clone(h.ConfigOptions[i].Values)
	}
	return h
}
