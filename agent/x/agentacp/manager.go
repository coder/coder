// Package agentacp manages workspace-local ACP harnesses.
package agentacp

import (
	"context"
	"sync"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// ErrManagerClosed is returned when the manager has been closed.
var ErrManagerClosed = xerrors.New("ACP manager closed")

// Options supplies the workspace execution environment and lifecycle.
type Options struct {
	Logger     slog.Logger
	Clock      quartz.Clock
	Filesystem afero.Fs
	EnvInfo    usershell.EnvInfoer
}

// Manager owns filesystem discovery and catalog snapshots.
type Manager struct {
	configHashes map[string][32]byte
	closedDone   chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	logger       slog.Logger
	clock        quartz.Clock
	fs           afero.Fs
	envInfo      usershell.EnvInfoer
	mu           sync.Mutex
	closed       bool
	configs      map[string]harnessConfig
	configDir    string
	catalog      []workspacesdk.ACPHarness
	onChange     func()
	wg           sync.WaitGroup
	reloadMu     sync.Mutex
	runOnce      sync.Once
}

// NewManager creates a manager; RunDiscovery starts filesystem discovery.
// Clock, Filesystem, and EnvInfo must be supplied.
func NewManager(ctx context.Context, opts Options) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{
		ctx:          ctx,
		cancel:       cancel,
		logger:       opts.Logger,
		clock:        opts.Clock,
		fs:           opts.Filesystem,
		envInfo:      opts.EnvInfo,
		closedDone:   make(chan struct{}),
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

// Close stops discovery and all manager-owned goroutines.
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
