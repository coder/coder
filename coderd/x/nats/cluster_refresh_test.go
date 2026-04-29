//nolint:testpackage
package nats

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// mutablePeerProvider is a PeerProvider whose peer set can be swapped at
// runtime. Peers returns a defensive copy.
type mutablePeerProvider struct {
	mu    sync.Mutex
	peers []Peer
}

func newMutablePeerProvider(peers []Peer) *mutablePeerProvider {
	m := &mutablePeerProvider{}
	m.set(peers)
	return m
}

func (m *mutablePeerProvider) Peers(_ context.Context) ([]Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Peer, len(m.peers))
	copy(out, m.peers)
	return out, nil
}

func (m *mutablePeerProvider) set(peers []Peer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]Peer, len(peers))
	copy(cp, peers)
	m.peers = cp
}

// buildClusterPubsubWithProvider mirrors buildClusterPubsub but allows
// using an arbitrary PeerProvider so tests can mutate the peer set after
// startup.
func buildClusterPubsubWithProvider(t *testing.T, name string, port int, provider PeerProvider, token string, tlsConfig *tls.Config) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Named(name).Leveled(slog.LevelDebug)

	opts := Options{
		ServerName:       name,
		ClusterName:      "test-cluster",
		ClusterToken:     token,
		ClusterHost:      "127.0.0.1",
		ClusterPort:      port,
		ClusterAdvertise: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		ClusterTLSConfig: tlsConfig,
		PeerProvider:     provider,
		ReadyTimeout:     testutil.WaitMedium,
	}
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	p, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestPubsubRefreshPeers_AddPeer(t *testing.T) {
	t.Parallel()
	token := "refresh-add"
	portA := freePort(t)
	portB := freePort(t)
	portC := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)
	urlC := "nats://127.0.0.1:" + strconv.Itoa(portC)

	provA := newMutablePeerProvider([]Peer{{RouteURL: urlB}})
	provB := newMutablePeerProvider([]Peer{{RouteURL: urlA}})

	a := buildClusterPubsubWithProvider(t, "node-a", portA, provA, token, nil)
	b := buildClusterPubsubWithProvider(t, "node-b", portB, provB, token, nil)
	waitForRoutes(t, a, 1)
	waitForRoutes(t, b, 1)

	// Bring up C clustered with A and B; A and B don't know about C yet.
	c := buildClusterPubsub(t, "node-c", portC, []Peer{{RouteURL: urlA}, {RouteURL: urlB}}, token, nil)
	waitForRoutes(t, c, 2)

	// Now hot-add C to A and B's providers and refresh.
	provA.set([]Peer{{RouteURL: urlB}, {RouteURL: urlC}})
	provB.set([]Peer{{RouteURL: urlA}, {RouteURL: urlC}})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	require.NoError(t, a.RefreshPeers(ctx))
	require.NoError(t, b.RefreshPeers(ctx))

	waitForRoutes(t, a, 2)
	waitForRoutes(t, b, 2)

	crossPublish(t, a, c, "evt-ac", "from-a-to-c")
	crossPublish(t, b, c, "evt-bc", "from-b-to-c")
}

func TestPubsubRefreshPeers_RemovePeer(t *testing.T) {
	t.Parallel()
	token := "refresh-remove"
	portA := freePort(t)
	portB := freePort(t)
	portC := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)
	urlC := "nats://127.0.0.1:" + strconv.Itoa(portC)

	provA := newMutablePeerProvider([]Peer{{RouteURL: urlB}, {RouteURL: urlC}})
	provB := newMutablePeerProvider([]Peer{{RouteURL: urlA}, {RouteURL: urlC}})

	a := buildClusterPubsubWithProvider(t, "node-a", portA, provA, token, nil)
	b := buildClusterPubsubWithProvider(t, "node-b", portB, provB, token, nil)
	c := buildClusterPubsub(t, "node-c", portC, []Peer{{RouteURL: urlA}, {RouteURL: urlB}}, token, nil)
	waitForRoutes(t, a, 2)
	waitForRoutes(t, b, 2)
	waitForRoutes(t, c, 2)

	// Drop C from A and B and refresh.
	provA.set([]Peer{{RouteURL: urlB}})
	provB.set([]Peer{{RouteURL: urlA}})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	require.NoError(t, a.RefreshPeers(ctx))
	require.NoError(t, b.RefreshPeers(ctx))

	// Eventually A and B no longer have a configured route to C. NATS
	// won't tear down already-established routes synchronously, so we
	// inspect the configured Routes via getOpts on the server.
	require.Eventually(t, func() bool {
		return !configuredHas(a, urlC) && !configuredHas(b, urlC)
	}, testutil.WaitMedium, testutil.IntervalFast,
		"expected C to be removed from A and B configured routes")
}

// configuredHas reports whether the *configured* route URLs of p contain
// a route whose host:port matches target.
func configuredHas(p *Pubsub, target string) bool {
	for _, u := range p.currentRoutes {
		if u == nil {
			continue
		}
		if u.Host == hostFromURL(target) {
			return true
		}
	}
	return false
}

func stripUserinfo(u *url.URL) string {
	if u == nil {
		return ""
	}
	cp := *u
	cp.User = nil
	return cp.String()
}

func hostFromURL(raw string) string {
	// Best-effort; tests pass nats://host:port URLs.
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func TestPubsubRefreshPeers_NoOp(t *testing.T) {
	t.Parallel()
	token := "refresh-noop"
	portA := freePort(t)
	portB := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)

	provA := newMutablePeerProvider([]Peer{{RouteURL: urlB}})
	a := buildClusterPubsubWithProvider(t, "node-a", portA, provA, token, nil)
	_ = buildClusterPubsub(t, "node-b", portB, []Peer{{RouteURL: urlA}}, token, nil)
	waitForRoutes(t, a, 1)

	// Re-set with the same single peer (order trivially same).
	provA.set([]Peer{{RouteURL: urlB}})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	require.NoError(t, a.RefreshPeers(ctx))

	// Refresh with the same single peer must be a no-op for
	// configured routes; the runtime route count may still be settling
	// route-pool connections, so we assert on configured route URLs
	// rather than NumRoutes.
	require.Len(t, a.currentRoutes, 1)
	require.Equal(t, urlB, stripUserinfo(a.currentRoutes[0]))
}

func TestPubsubRefreshPeers_NoOp_DifferentOrder(t *testing.T) {
	t.Parallel()
	token := "refresh-noop-order"
	portA := freePort(t)
	portB := freePort(t)
	portC := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)
	urlC := "nats://127.0.0.1:" + strconv.Itoa(portC)

	provA := newMutablePeerProvider([]Peer{{RouteURL: urlB}, {RouteURL: urlC}})
	a := buildClusterPubsubWithProvider(t, "node-a", portA, provA, token, nil)
	_ = buildClusterPubsub(t, "node-b", portB, []Peer{{RouteURL: urlA}, {RouteURL: urlC}}, token, nil)
	_ = buildClusterPubsub(t, "node-c", portC, []Peer{{RouteURL: urlA}, {RouteURL: urlB}}, token, nil)
	waitForRoutes(t, a, 2)

	// Reorder same set; refresh should be a no-op.
	provA.set([]Peer{{RouteURL: urlC}, {RouteURL: urlB}})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	require.NoError(t, a.RefreshPeers(ctx))
	require.Equal(t, 2, len(a.currentRoutes))
}

func TestPubsubRefreshPeers_NilProvider_ConfigError(t *testing.T) {
	t.Parallel()
	// New with no PeerProvider: server is up (cluster of 1), but
	// RefreshPeers cannot do anything because there is no provider to
	// query. Returns a config error, NOT ErrNoEmbeddedServer.
	p := newSoloPubsub(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	err := p.RefreshPeers(ctx)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrNoEmbeddedServer))
	require.Contains(t, err.Error(), "no PeerProvider")
}

func TestPubsubRefreshPeers_ZeroPeers_NoOp(t *testing.T) {
	t.Parallel()
	// New with a PeerProvider that returns zero peers: cluster of 1.
	// RefreshPeers must succeed (no-op): empty currentRoutes ==
	// empty refreshed set, no ReloadOptions call needed.
	p := newSoloPubsub(t, Options{
		PeerProvider: StaticPeerProvider(nil),
	})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	require.NoError(t, p.RefreshPeers(ctx))
	require.Empty(t, p.currentRoutes)
}

func TestPubsubRefreshPeers_NewFromConn_NoEmbeddedServer(t *testing.T) {
	t.Parallel()
	// NewFromConn does not own a server. RefreshPeers must return
	// ErrNoEmbeddedServer regardless of whether a provider could
	// theoretically be wired in.
	host := newSoloPubsub(t, Options{})
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	p, err := NewFromConn(logger, host.nc)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	err = p.RefreshPeers(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoEmbeddedServer))
}

func TestPubsubRefreshPeers_TLSAndToken(t *testing.T) {
	t.Parallel()
	pool, cert := genTestCert(t, []string{"localhost"})
	mkCfg := func() *tls.Config {
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      pool,
			ClientCAs:    pool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			MinVersion:   tls.VersionTLS12,
			ServerName:   "localhost",
		}
	}

	token := "tls-refresh-token" //nolint:gosec // test-only shared cluster token, not a real credential
	portA := freePort(t)
	portB := freePort(t)
	portC := freePort(t)
	urlA := "tls://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "tls://127.0.0.1:" + strconv.Itoa(portB)
	urlC := "tls://127.0.0.1:" + strconv.Itoa(portC)

	provA := newMutablePeerProvider([]Peer{{RouteURL: urlB}})
	a := buildClusterPubsubWithProvider(t, "tls-a", portA, provA, token, mkCfg())
	_ = buildClusterPubsub(t, "tls-b", portB, []Peer{{RouteURL: urlA}, {RouteURL: urlC}}, token, mkCfg())
	c := buildClusterPubsub(t, "tls-c", portC, []Peer{{RouteURL: urlA}, {RouteURL: urlB}}, token, mkCfg())
	waitForRoutes(t, a, 1)

	// Add C and refresh.
	provA.set([]Peer{{RouteURL: urlB}, {RouteURL: urlC}})
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	require.NoError(t, a.RefreshPeers(ctx))

	// Verify currentRoutes preserve tls scheme + coder:<token> userinfo.
	require.Len(t, a.currentRoutes, 2)
	for _, u := range a.currentRoutes {
		require.Equal(t, "tls", u.Scheme)
		require.NotNil(t, u.User)
		require.Equal(t, "coder", u.User.Username())
		pw, set := u.User.Password()
		require.True(t, set)
		require.Equal(t, token, pw)
	}

	waitForRoutes(t, a, 2)
	crossPublish(t, a, c, "tls-evt", "tls-refresh-hello")
}

// TestPubsub_SetPeerProvider_DeferredInstallEnablesRefresh exercises the
// deferred-provider wiring used by the enterprise xreplicasync glue: a
// Pubsub is constructed without a PeerProvider, the caller installs one
// later via SetPeerProvider, and RefreshPeers then succeeds. This is a
// regression test against the production bug where SetPeerProvider did
// not exist and RefreshPeers errored continuously with "no PeerProvider
// configured" because cli/server.go could not supply a provider that
// depends on enterprise-only subsystems.
func TestPubsub_SetPeerProvider_DeferredInstallEnablesRefresh(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Construct two Pubsubs without PeerProviders, mirroring the
	// production sequence in cli/server.go where the provider depends on
	// enterprise-only subsystems built later.
	token := "deferred-provider-token" //nolint:gosec
	portA := freePort(t)
	portB := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)
	a := buildClusterPubsubWithProvider(t, "node-a", portA, nil, token, nil)
	b := buildClusterPubsubWithProvider(t, "node-b", portB, nil, token, nil)

	// RefreshPeers must fail before SetPeerProvider with the exact
	// production error string the deployed scaletest reported.
	err := a.RefreshPeers(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no PeerProvider")

	// Install the providers post-construction.
	provA := newMutablePeerProvider([]Peer{{RouteURL: urlB}})
	provB := newMutablePeerProvider([]Peer{{RouteURL: urlA}})
	require.NoError(t, a.SetPeerProvider(provA))
	require.NoError(t, b.SetPeerProvider(provB))

	// Set-once: a second SetPeerProvider must error.
	err = a.SetPeerProvider(provA)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already configured")

	// nil provider is rejected.
	err = b.SetPeerProvider(nil)
	require.Error(t, err)

	// RefreshPeers now establishes routes.
	require.NoError(t, a.RefreshPeers(ctx))
	require.NoError(t, b.RefreshPeers(ctx))
	waitForRoutes(t, a, 1)
	waitForRoutes(t, b, 1)
	crossPublish(t, a, b, "deferred-evt", "deferred-hello")
}

// TestPubsub_SetPeerProvider_RejectsWhenAlreadySetAtConstruction confirms
// that SetPeerProvider refuses to clobber a provider supplied via
// Options.PeerProvider at New time.
func TestPubsub_SetPeerProvider_RejectsWhenAlreadySetAtConstruction(t *testing.T) {
	t.Parallel()
	port := freePort(t)
	p := buildClusterPubsubWithProvider(t, "preset", port, StaticPeerProvider(nil), "preset-token", nil) //nolint:gosec
	err := p.SetPeerProvider(StaticPeerProvider(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "already configured")
}
