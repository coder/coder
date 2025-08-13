package portforward

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/agent/agentssh"
)

// Spec represents a port forwarding specification.
type Spec struct {
	Network    string     // tcp, udp
	ListenHost netip.Addr // Local address to bind to
	ListenPort uint16     // Local port to listen on
	DialPort   uint16     // Remote port to connect to
}

// Forwarder handles a single port forward.
type Forwarder interface {
	// Start begins the port forwarding operation.
	Start(ctx context.Context) error
	// Stop stops the port forwarding operation.
	Stop() error
	// IsActive returns true if the forwarder is currently active.
	IsActive() bool
	// Spec returns the port forwarding specification.
	Spec() Spec
}

// Manager manages multiple port forwards.
type Manager interface {
	// Add adds a new port forward.
	Add(spec Spec) (Forwarder, error)
	// Remove removes an existing port forward.
	Remove(spec Spec) error
	// List returns all active port forwards.
	List() []Forwarder
	// Start starts all port forwards.
	Start(ctx context.Context) error
	// Stop stops all port forwards.
	Stop() error
}

// Dialer provides network dialing capabilities.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Listener provides network listening capabilities.
type Listener interface {
	Listen(network, address string) (net.Listener, error)
}

// Options configures port forwarding behavior.
type Options struct {
	Logger   slog.Logger
	Dialer   Dialer
	Listener Listener
}

// LocalForwarder implements a single port forward from local to remote.
type LocalForwarder struct {
	spec     Spec
	opts     Options
	listener net.Listener
	active   atomic.Bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewLocal creates a new local port forwarder.
func NewLocal(spec Spec, opts Options) *LocalForwarder {
	return &LocalForwarder{
		spec: spec,
		opts: opts,
	}
}

func (f *LocalForwarder) Start(ctx context.Context) error {
	if f.active.Load() {
		return xerrors.New("forwarder is already active")
	}

	ctx, cancel := context.WithCancel(ctx)
	f.cancel = cancel

	logger := f.opts.Logger.With(
		slog.F("network", f.spec.Network),
		slog.F("listen_host", f.spec.ListenHost),
		slog.F("listen_port", f.spec.ListenPort),
	)

	listenAddress := netip.AddrPortFrom(f.spec.ListenHost, f.spec.ListenPort)
	dialAddress := fmt.Sprintf("127.0.0.1:%d", f.spec.DialPort)

	l, err := f.opts.Listener.Listen(f.spec.Network, listenAddress.String())
	if err != nil {
		cancel()
		return xerrors.Errorf("listen '%s://%s': %w", f.spec.Network, listenAddress.String(), err)
	}
	f.listener = l
	logger.Debug(ctx, "listening")

	f.active.Store(true)

	f.wg.Add(1)
	go func() {
		defer func() {
			f.wg.Done()
			f.active.Store(false)
		}()

		for {
			netConn, err := l.Accept()
			if err != nil {
				// Silently ignore net.ErrClosed errors.
				if xerrors.Is(err, net.ErrClosed) {
					logger.Debug(ctx, "listener closed")
					return
				}
				logger.Error(ctx, "error accepting connection",
					slog.F("listen_address", listenAddress.String()),
					slog.Error(err))
				return
			}
			logger.Debug(ctx, "accepted connection",
				slog.F("remote_addr", netConn.RemoteAddr()))

			go func(netConn net.Conn) {
				defer netConn.Close()
				remoteConn, err := f.opts.Dialer.DialContext(ctx, f.spec.Network, dialAddress)
				if err != nil {
					logger.Error(ctx, "failed to dial remote",
						slog.F("dial_address", dialAddress),
						slog.Error(err))
					return
				}
				defer remoteConn.Close()
				logger.Debug(ctx, "dialed remote",
					slog.F("remote_addr", netConn.RemoteAddr()))

				agentssh.Bicopy(ctx, netConn, remoteConn)
				logger.Debug(ctx, "connection closing",
					slog.F("remote_addr", netConn.RemoteAddr()))
			}(netConn)
		}
	}()

	return nil
}

func (f *LocalForwarder) Stop() error {
	if !f.active.Load() {
		return nil
	}

	if f.cancel != nil {
		f.cancel()
	}
	if f.listener != nil {
		_ = f.listener.Close()
	}
	f.wg.Wait()
	return nil
}

func (f *LocalForwarder) IsActive() bool {
	return f.active.Load()
}

func (f *LocalForwarder) Spec() Spec {
	return f.spec
}

// manager implements the Manager interface.
type manager struct {
	forwarders map[string]Forwarder
	opts       Options
	mu         sync.RWMutex
}

// NewManager creates a new port forwarding manager.
func NewManager(opts Options) Manager {
	return &manager{
		forwarders: make(map[string]Forwarder),
		opts:       opts,
	}
}

func (m *manager) Add(spec Spec) (Forwarder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%d", spec.Network, spec.ListenHost, spec.ListenPort)
	if _, exists := m.forwarders[key]; exists {
		return nil, xerrors.Errorf("forwarder already exists for %s", key)
	}

	// Test if we can actually bind to the port before adding the forwarder
	listenAddress := netip.AddrPortFrom(spec.ListenHost, spec.ListenPort)
	testListener, err := m.opts.Listener.Listen(spec.Network, listenAddress.String())
	if err != nil {
		return nil, xerrors.Errorf("cannot bind to '%s://%s': %w", spec.Network, listenAddress.String(), err)
	}
	// Close the test listener immediately since we just wanted to verify we can bind
	_ = testListener.Close()

	forwarder := NewLocal(spec, m.opts)
	m.forwarders[key] = forwarder
	return forwarder, nil
}

func (m *manager) Remove(spec Spec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%d", spec.Network, spec.ListenHost, spec.ListenPort)
	forwarder, exists := m.forwarders[key]
	if !exists {
		return xerrors.Errorf("forwarder not found for %s", key)
	}

	err := forwarder.Stop()
	delete(m.forwarders, key)
	return err
}

func (m *manager) List() []Forwarder {
	m.mu.RLock()
	defer m.mu.RUnlock()

	forwarders := make([]Forwarder, 0, len(m.forwarders))
	for _, f := range m.forwarders {
		forwarders = append(forwarders, f)
	}
	return forwarders
}

func (m *manager) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, forwarder := range m.forwarders {
		if !forwarder.IsActive() {
			if err := forwarder.Start(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *manager) Stop() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	for _, forwarder := range m.forwarders {
		if err := forwarder.Stop(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
