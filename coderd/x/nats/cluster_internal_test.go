package nats

import (
	"errors"
	"net/url"
	"sync"
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

	t.Run("SetPeerFetcherRefreshes", func(t *testing.T) {
		t.Parallel()
		opts := clusterTestOptions(t)
		b := newTestPubsub(t, opts)
		fetcher := &testPeerFetcher{addresses: []string{clusterRouteAddress(t, b)}}
		aOpts := clusterTestOptions(t)
		aOpts.PeerFetcher = fetcher
		a := newTestPubsub(t, aOpts)

		require.Eventually(t, func() bool {
			routes := currentRouteURLs(a)
			return sortedURLsEqual(routes, sortRouteURLs(mustParsePeerAddresses(t,
				addrWithAuth(t, clusterRouteAddress(t, b), aOpts.ClusterAuthToken),
			)))
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("RefreshUsesLatestFetcher", func(t *testing.T) {
		t.Parallel()
		opts := clusterTestOptions(t)
		a := newTestPubsub(t, opts)
		b := newTestPubsub(t, opts)
		c := newTestPubsub(t, opts)
		fetcher := &testPeerFetcher{}
		a.SetPeerFetcher(fetcher)

		fetcher.set(clusterRouteAddress(t, b))
		a.RefreshPeers()
		require.Eventually(t, func() bool {
			return sortedURLsEqual(currentRouteURLs(a), sortRouteURLs(mustParsePeerAddresses(t,
				addrWithAuth(t, clusterRouteAddress(t, b), opts.ClusterAuthToken),
			)))
		}, testutil.WaitShort, testutil.IntervalFast)

		fetcher.set(clusterRouteAddress(t, c))
		a.RefreshPeers()
		require.Eventually(t, func() bool {
			return sortedURLsEqual(currentRouteURLs(a), sortRouteURLs(mustParsePeerAddresses(t,
				addrWithAuth(t, clusterRouteAddress(t, c), opts.ClusterAuthToken),
			)))
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("RefreshCoalescesPendingSignals", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, clusterTestOptions(t))
		fetcher := &blockingPeerFetcher{
			entered: make(chan struct{}),
			release: make(chan struct{}),
		}
		ps.SetPeerFetcher(fetcher)
		testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, fetcher.entered)

		for range 5 {
			ps.RefreshPeers()
		}
		require.Len(t, ps.peerRefresh, 1)
		close(fetcher.release)
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
	mu        sync.Mutex
	addresses []string
}

func (f *testPeerFetcher) set(addresses ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addresses = append([]string(nil), addresses...)
}

func (f *testPeerFetcher) PrimaryPeerAddresses() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.addresses...)
}

type blockingPeerFetcher struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (f *blockingPeerFetcher) PrimaryPeerAddresses() []string {
	f.once.Do(func() { f.entered <- struct{}{} })
	<-f.release
	return nil
}

// Cluster tests bind free ports and reload shared route state.
func TestPubsub_SetPeerAddresses(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		opts := clusterTestOptions(t)
		a := newTestPubsub(t, opts)
		b := newTestPubsub(t, opts)
		c := newTestPubsub(t, opts)

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.SetPeerAddresses([]string{addrC, addrB}))
		requireRoutesEqual(t, a.currentRoutes,
			addrWithAuth(t, addrB, opts.ClusterAuthToken),
			addrWithAuth(t, addrC, opts.ClusterAuthToken),
		)

		require.NoError(t, a.SetPeerAddresses([]string{addrB, addrC}))
		requireRoutesEqual(t, a.currentRoutes,
			addrWithAuth(t, addrB, opts.ClusterAuthToken),
			addrWithAuth(t, addrC, opts.ClusterAuthToken),
		)

		require.NoError(t, a.SetPeerAddresses(nil))
		require.Empty(t, a.currentRoutes)
		require.Empty(t, a.serverOpts.Routes)
	})

	t.Run("StandaloneConfigError", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, defaultTestOptions())
		err := ps.SetPeerAddresses(nil)
		require.ErrorContains(t, err, "not started with clustering enabled")
	})

	t.Run("Closed", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, clusterTestOptions(t))
		require.NoError(t, ps.Close())
		err := ps.SetPeerAddresses(nil)
		require.True(t, errors.Is(err, errClosed), "got %v", err)
	})

	t.Run("DropsSelfRoute", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, clusterTestOptions(t))
		require.NoError(t, ps.SetPeerAddresses([]string{clusterRouteAddress(t, ps)}))
		require.Empty(t, ps.currentRoutes)
	})
}
