package nats

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

const (
	minTCPPort int32 = 1
	maxTCPPort int32 = 65535
)

func Test_parsePeerAddresses(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
			"nats://127.0.0.1:4222 ",
			"nats://[::1]:7222",
			"nats://example.com:6222",
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{
			"nats://127.0.0.1:4222",
			"nats://[::1]:7222",
			"nats://example.com:6222",
		}, routeStrings(routes))
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses(nil)
		require.NoError(t, err)
		require.Empty(t, routes)
	})

	t.Run("Dedupes", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
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
			"whatever://127.0.0.1:4222 ",
			"http://[::1]:7222",
		} {
			t.Run(address, func(t *testing.T) {
				t.Parallel()
				_, err := parsePeerAddresses([]string{address})
				require.Error(t, err)
			})
		}
	})
}

func Test_filterSelfRoutes(t *testing.T) {
	t.Parallel()

	routes, err := parsePeerAddresses([]string{
		"nats://b.example:6222",
		"nats://self.example:6222",
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
		require.GreaterOrEqual(t, fetcher.port, minTCPPort)
		require.LessOrEqual(t, fetcher.port, maxTCPPort)

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
		fetcher := &testPeerFetcher{addresses: routes}

		expectedRoutes := routesWithAuth(mustParsePeerAddresses(t, fetcher.addresses...), opts.ClusterAuthToken)

		a.SetPeerFetcher(fetcher)
		require.GreaterOrEqual(t, fetcher.port, minTCPPort)
		require.LessOrEqual(t, fetcher.port, maxTCPPort)
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
	port      int32
}

func (f *testPeerFetcher) SetSelfNATSPort(port int32) {
	f.port = port
}

func (f *testPeerFetcher) FetchNATSPeers() []string {
	return f.addresses
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
