package nats

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func Test_parsePeerAddresses(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		ps := &Pubsub{}
		routes, err := ps.parsePeerAddresses([]string{
			"whatever://127.0.0.1:4222 ",
			"http://[::1]:7222",
			"nats://example.com:6222",
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"nats://127.0.0.1:4222",
			"nats://[::1]:7222",
			"nats://example.com:6222",
		}, routeStrings(routes))
	})

	// Test that when a pubsub is running with the default port, it assumes all peers are also using
	// the default port.
	t.Run("PrefersDefaultPort", func(t *testing.T) {
		t.Parallel()
		ps := &Pubsub{}
		ps.opts.ClusterPort = defaultClusterPort
		routes, err := ps.parsePeerAddresses([]string{
			"whatever://127.0.0.1:4222 ",
			"http://[::1]:7222",
			"nats://example.com:1234",
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"nats://127.0.0.1:6222",
			"nats://[::1]:6222",
			"nats://example.com:6222",
		}, routeStrings(routes))
	})

	// Regression: in production the relay URL host carries the coderd HTTP
	// port (e.g. 8080), and routes must be rewritten to the NATS cluster
	// port. This only works because New defaults ClusterPort to
	// defaultClusterPort; if it were left at the zero value the rewrite
	// would be skipped and routes would dial the HTTP port.
	t.Run("RewritesRelayHTTPPort", func(t *testing.T) {
		t.Parallel()
		ps := &Pubsub{}
		ps.opts.ClusterPort = defaultClusterPort
		routes, err := ps.parsePeerAddresses([]string{"http://10.0.0.7:8080"})
		require.NoError(t, err)
		require.Equal(t, []string{"nats://10.0.0.7:6222"}, routeStrings(routes))
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		ps := &Pubsub{}
		routes, err := ps.parsePeerAddresses(nil)
		require.NoError(t, err)
		require.Empty(t, routes)
	})

	t.Run("Dedupes", func(t *testing.T) {
		t.Parallel()
		ps := &Pubsub{}
		routes, err := ps.parsePeerAddresses([]string{
			"nats://b.example:6222",
			"nats://a.example:6222",
			"nats://b.example:6222",
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"nats://a.example:6222",
			"nats://b.example:6222",
		}, routeStrings(routes))
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		for _, address := range []string{
			"",
			"   ",
			"127.0.0.1:4222",
			"127.0.0.1",
			":4222",
			"127.0.0.1:0",
			"127.0.0.1:bad",
			"nats://127.0.0.1",
			"nats://:4222",
			"nats://127.0.0.1:0",
			"nats://127.0.0.1:bad",
			"nats://user@127.0.0.1:4222",
			"nats://127.0.0.1:4222/path",
			"nats://127.0.0.1:4222?x=1",
			"nats://127.0.0.1:4222#frag",
		} {
			t.Run(address, func(t *testing.T) {
				t.Parallel()
				ps := &Pubsub{}
				_, err := ps.parsePeerAddresses([]string{address})
				require.Error(t, err)
			})
		}
	})
}

func Test_filterSelfRoutes(t *testing.T) {
	t.Parallel()

	ps := &Pubsub{}
	routes, err := ps.parsePeerAddresses([]string{
		"nats://b.example:6222",
		"http://self.example:6222",
	})
	require.NoError(t, err)

	routes = filterSelfRoutes(routes, &url.URL{Scheme: "nats", Host: "self.example:6222"})
	require.Equal(t, []string{"nats://b.example:6222"}, routeStrings(routes))
}

func TestPubsub_RefreshPeers(t *testing.T) {
	t.Parallel()

	t.Run("PeersFetchedOnStartup", func(t *testing.T) {
		t.Parallel()

		// Supplying PeerFetcher in Options should be enough to seed routes.
		// Callers should not need a separate SetPeerFetcher or RefreshPeers call
		// after New returns.
		fetcher := &testPeerFetcher{addresses: []string{"nats://127.0.0.1:1234"}}
		opts := clusterTestOptions(t)
		opts.PeerFetcher = fetcher
		a := newTestPubsub(t, opts)

		require.Eventually(t, func() bool {
			routes := currentRouteURLs(a)
			return sortedURLsEqual(routes, sortRouteURLs(mustParsePeerAddresses(t,
				addrWithAuth(t, "nats://127.0.0.1:1234", opts.ClusterAuthToken),
			)))
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("SetPeerFetcher", func(t *testing.T) {
		t.Parallel()
		opts := clusterTestOptions(t)
		a := newTestPubsub(t, opts)

		routes := []string{
			"nats://127.0.0.1:1234",
			"nats://127.0.0.1:1235",
		}
		fetcher := &testPeerFetcher{routes}

		expectedRoutes := routesWithAuth(mustParsePeerAddresses(t, fetcher.addresses...), opts.ClusterAuthToken)

		a.SetPeerFetcher(fetcher)
		require.Eventually(t, func() bool {
			return sortedURLsEqual(currentRouteURLs(a), sortRouteURLs(expectedRoutes))
		}, testutil.WaitShort, testutil.IntervalFast)

		a.SetPeerFetcher(nil)
		require.Eventually(t, func() bool {
			return sortedURLsEqual(currentRouteURLs(a), nil)
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

func mustParsePeerAddresses(t *testing.T, addresses ...string) []*url.URL {
	t.Helper()
	routes := make([]*url.URL, 0, len(addresses))
	for _, address := range addresses {
		route, err := url.Parse(address)
		require.NoError(t, err)
		routes = append(routes, route)
	}
	return routes
}

func currentRouteURLs(ps *Pubsub) []*url.URL {
	ps.clusterMu.Lock()
	defer ps.clusterMu.Unlock()
	return cloneRouteURLs(ps.currentRoutes)
}

type testPeerFetcher struct {
	addresses []string
}

func (f *testPeerFetcher) PrimaryPeerAddresses() []string {
	return f.addresses
}

// TestPubsub_New_DefaultsClusterPort guards the production wiring: New
// must persist the default cluster port onto opts so the peer route
// rewrite in parsePeerAddresses recognizes prod and forces routes to the
// NATS port. The cli constructs Options without a ClusterPort, so leaving
// it at the zero value made every replica dial peers at the relay URL's
// HTTP port instead of the NATS route port.
func TestPubsub_New_DefaultsClusterPort(t *testing.T) {
	t.Parallel()
	// defaultTestOptions disables clustering (no fixed-port listener to
	// collide with parallel tests) and leaves ClusterPort unset.
	ps := newTestPubsub(t, defaultTestOptions())
	require.Equal(t, defaultClusterPort, ps.opts.ClusterPort)
}

func TestPubsub_setPeerAddresses(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		opts := clusterTestOptions(t)
		a := newTestPubsub(t, opts)
		b := newTestPubsub(t, opts)
		c := newTestPubsub(t, opts)

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.setPeerAddresses([]string{addrC, addrB}))
		requireRoutesEqual(t, a.currentRoutes,
			addrWithAuth(t, addrB, opts.ClusterAuthToken),
			addrWithAuth(t, addrC, opts.ClusterAuthToken),
		)

		require.NoError(t, a.setPeerAddresses([]string{addrB, addrC}))
		requireRoutesEqual(t, a.currentRoutes,
			addrWithAuth(t, addrB, opts.ClusterAuthToken),
			addrWithAuth(t, addrC, opts.ClusterAuthToken),
		)

		require.NoError(t, a.setPeerAddresses(nil))
		require.Empty(t, a.currentRoutes)
		require.Empty(t, a.serverOpts.Routes)
	})

	t.Run("StandaloneConfigError", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, defaultTestOptions())
		err := ps.setPeerAddresses(nil)
		require.ErrorContains(t, err, "not started with clustering enabled")
	})

	t.Run("Closed", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, clusterTestOptions(t))
		require.NoError(t, ps.Close())
		err := ps.setPeerAddresses(nil)
		require.True(t, errors.Is(err, errClosed), "got %v", err)
	})

	t.Run("DropsSelfRoute", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, clusterTestOptions(t))
		require.NoError(t, ps.setPeerAddresses([]string{clusterRouteAddress(t, ps)}))
		require.Empty(t, ps.currentRoutes)
	})
}
