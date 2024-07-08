package derpmesh

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"sync"

	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet"
)

// New constructs a new mesh for DERP servers.
func New(logger slog.Logger, server *derp.Server, tlsConfig *tls.Config) *Mesh {
	return &Mesh{
		logger:    logger,
		server:    server,
		tlsConfig: tlsConfig,
		ctx:       context.Background(),
		closed:    make(chan struct{}),
		active:    make(map[string]context.CancelFunc),
	}
}

type Mesh struct {
	logger    slog.Logger
	server    *derp.Server
	ctx       context.Context
	tlsConfig *tls.Config

	mutex  sync.Mutex
	closed chan struct{}
	active map[string]context.CancelFunc
}

// SetAddresses performs a diff of the incoming addresses and adds
// or removes DERP clients from the mesh.
//
// Connect is only used for testing to ensure DERPs are meshed before
// exchanging messages.
// nolint:revive
func (m *Mesh) SetAddresses(addresses []string, connect bool) {
	total := make(map[string]struct{}, 0)
	for _, address := range addresses {
		addressURL, err := url.Parse(address)
		if err != nil {
			m.logger.Error(m.ctx, "unable to parse DERP address", slog.F("address", address), slog.Error(err))
			continue
		}
		derpURL, err := addressURL.Parse("/derp")
		if err != nil {
			m.logger.Error(m.ctx, "unable to parse DERP address with /derp", slog.F("address", addressURL.String()), slog.Error(err))
			continue
		}
		address = derpURL.String()

		total[address] = struct{}{}
		added, err := m.addAddress(address, connect)
		if err != nil {
			m.logger.Error(m.ctx, "failed to add address", slog.F("address", address), slog.Error(err))
			continue
		}
		if added {
			m.logger.Debug(m.ctx, "added mesh address", slog.F("address", address))
		}
	}

	m.mutex.Lock()
	for address := range m.active {
		_, found := total[address]
		if found {
			continue
		}
		removed := m.removeAddress(address)
		if removed {
			m.logger.Debug(m.ctx, "removed mesh address", slog.F("address", address))
		}
	}
	m.mutex.Unlock()
}

// addAddress begins meshing with a new address. It returns false if the address is already being meshed with.
// It's expected that this is a full HTTP address with a path.
// e.g. http://127.0.0.1:8080/derp
// nolint:revive
func (m *Mesh) addAddress(address string, connect bool) (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.isClosed() {
		return false, nil
	}
	_, isActive := m.active[address]
	if isActive {
		return false, nil
	}
	client, err := derphttp.NewClient(m.server.PrivateKey(), address, tailnet.Logger(m.logger.Named("client")))
	if err != nil {
		return false, xerrors.Errorf("create derp client: %w", err)
	}
	client.TLSConfig = m.tlsConfig
	client.MeshKey = m.server.MeshKey()
	client.SetURLDialer(func(ctx context.Context, network, addr string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, addr)
	})
	if connect {
		_ = client.Connect(m.ctx)
	}
	ctx, cancelFunc := context.WithCancel(m.ctx)
	closed := make(chan struct{})
	closeFunc := func() {
		cancelFunc()
		_ = client.Close()
		<-closed
	}
	m.active[address] = closeFunc
	go func() {
		defer close(closed)
		client.RunWatchConnectionLoop(ctx, m.server.PublicKey(), tailnet.Logger(m.logger.Named("loop")), func(np key.NodePublic) {
			m.server.AddPacketForwarder(np, client)
		}, func(np key.NodePublic) {
			m.server.RemovePacketForwarder(np, client)
		})
	}()
	return true, nil
}

// removeAddress stops meshing with a given address.
func (m *Mesh) removeAddress(address string) bool {
	cancelFunc, isActive := m.active[address]
	if isActive {
		cancelFunc()
		delete(m.active, address)
	}
	return isActive
}

// Close ends all active meshes with the DERP server.
func (m *Mesh) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.isClosed() {
		return nil
	}
	close(m.closed)
	for _, cancelFunc := range m.active {
		cancelFunc()
	}
	return nil
}

func (m *Mesh) isClosed() bool {
	select {
	case <-m.closed:
		return true
	default:
	}
	return false
}
