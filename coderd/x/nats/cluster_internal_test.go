package nats //nolint:testpackage // Exercises internal cluster helpers and state.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func Test_parsePeerAddresses(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
			" nats://127.0.0.1:4222 ",
			"nats://example.com:6222",
			"nats://[::1]:7222",
		})
		require.NoError(t, err)
		require.Equal(t, []string{
			"nats://127.0.0.1:4222",
			"nats://example.com:6222",
			"nats://[::1]:7222",
		}, routeStrings(routes))

		routes[0].Host = "mutated:4222"
		routes2, err := parsePeerAddresses([]string{"nats://127.0.0.1:4222"})
		require.NoError(t, err)
		require.Equal(t, "nats://127.0.0.1:4222", routes2[0].String())
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses(nil)
		require.NoError(t, err)
		require.Empty(t, routes)
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		for _, address := range []string{
			"",
			"   ",
			"http://127.0.0.1:4222",
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
				_, err := parsePeerAddresses([]string{address})
				require.Error(t, err)
			})
		}
	})
}

func Test_routeURLHelpers(t *testing.T) {
	t.Parallel()

	routes := mustParseRoutes(t, "nats://b.example:6222", "nats://a.example:6222")
	sortRouteURLs(routes)
	require.Equal(t, []string{"nats://a.example:6222", "nats://b.example:6222"}, routeStrings(routes))

	clone := cloneRouteURLs(routes)
	require.True(t, routeURLsEqual(routes, clone))
	clone[0].Host = "changed:6222"
	require.False(t, routeURLsEqual(routes, clone))
	require.Equal(t, "nats://a.example:6222", routes[0].String())

	filtered := filterSelfRoutes(routes, "a.example:6222")
	require.Equal(t, []string{"nats://b.example:6222"}, routeStrings(filtered))

	withDuplicate := mustParseRoutes(t, "nats://a.example:6222", "nats://a.example:6222")
	deduped := dedupeRouteURLs(withDuplicate)
	require.Equal(t, []string{"nats://a.example:6222"}, routeStrings(deduped))
	withDuplicate[0].Host = "changed:6222"
	require.Equal(t, []string{"nats://a.example:6222"}, routeStrings(deduped))
}

func Test_clusterEnabled(t *testing.T) {
	t.Parallel()

	require.False(t, clusterEnabled(Options{}))
	for _, opts := range []Options{
		{ClusterName: "test"},
		{ClusterHost: "127.0.0.1"},
		{ClusterPort: 6222},
		{ClusterAdvertise: "127.0.0.1:6222"},
		{RoutePoolSize: 1},
		{PeerAddresses: []string{"nats://127.0.0.1:6222"}},
	} {
		require.True(t, clusterEnabled(opts), "%+v", opts)
	}
}

func Test_buildServerOptionsCluster(t *testing.T) {
	t.Parallel()

	t.Run("StandaloneDefault", func(t *testing.T) {
		t.Parallel()
		sopts, err := buildServerOptions(Options{})
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1", sopts.Host)
		require.Equal(t, natsserver.RANDOM_PORT, sopts.Port)
		require.Zero(t, sopts.Cluster.Port)
		require.Empty(t, sopts.Routes)
	})

	t.Run("ClusterDefaults", func(t *testing.T) {
		t.Parallel()
		sopts, err := buildServerOptions(Options{ClusterName: "test"})
		require.NoError(t, err)
		require.Equal(t, "test", sopts.Cluster.Name)
		require.Equal(t, "127.0.0.1", sopts.Cluster.Host)
		require.Equal(t, natsserver.RANDOM_PORT, sopts.Cluster.Port)
		require.Equal(t, DefaultRoutePoolSize, sopts.Cluster.PoolSize)
	})

	t.Run("ClusterOverridesAndRoutes", func(t *testing.T) {
		t.Parallel()
		sopts, err := buildServerOptions(Options{
			ClusterName:      "override",
			ClusterHost:      "127.0.0.1",
			ClusterPort:      6222,
			ClusterAdvertise: "cluster.example:6222",
			RoutePoolSize:    2,
			PeerAddresses: []string{
				"nats://b.example:6222",
				"nats://a.example:6222",
			},
		})
		require.NoError(t, err)
		require.Equal(t, "override", sopts.Cluster.Name)
		require.Equal(t, "127.0.0.1", sopts.Cluster.Host)
		require.Equal(t, 6222, sopts.Cluster.Port)
		require.Equal(t, "cluster.example:6222", sopts.Cluster.Advertise)
		require.Equal(t, 2, sopts.Cluster.PoolSize)
		require.Equal(t, []string{"nats://a.example:6222", "nats://b.example:6222"}, routeStrings(sopts.Routes))
	})
}

//nolint:paralleltest // Cluster tests bind free ports and reload shared route state.
func TestPubsubCluster(t *testing.T) {
	t.Run("ExplicitClusterOfOneWithEmptyPeers", func(t *testing.T) {
		ps := newClusterTestPubsub(t, Options{ClusterName: "one"})
		require.NotNil(t, ps.ns.ClusterAddr())
		require.Empty(t, ps.currentRoutes)
	})

	t.Run("LocalRoundTrip", func(t *testing.T) {
		ps := newClusterTestPubsub(t, Options{ClusterName: "local"})
		event := uniqueSubject("local")
		got := make(chan []byte, 1)
		cancel, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish(event, []byte("hello")))
		require.NoError(t, ps.Flush())
		require.Equal(t, "hello", string(receiveMessage(t, got)))
	})

	t.Run("TwoNodeSetPeerAddresses", func(t *testing.T) {
		a := newClusterTestPubsub(t, Options{ClusterName: "two"})
		b := newClusterTestPubsub(t, Options{ClusterName: "two"})
		require.NoError(t, a.SetPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.NoError(t, b.SetPeerAddresses([]string{clusterRouteAddress(t, a)}))
		waitForRoutes(t, a, 1)
		waitForRoutes(t, b, 1)

		eventAB := uniqueSubject("ab")
		gotB := make(chan []byte, 8)
		cancelB, err := b.Subscribe(eventAB, func(_ context.Context, msg []byte) {
			gotB <- msg
		})
		require.NoError(t, err)
		defer cancelB()
		publishUntilReceived(t, a, eventAB, "from-a", gotB)

		eventBA := uniqueSubject("ba")
		gotA := make(chan []byte, 8)
		cancelA, err := a.Subscribe(eventBA, func(_ context.Context, msg []byte) {
			gotA <- msg
		})
		require.NoError(t, err)
		defer cancelA()
		publishUntilReceived(t, b, eventBA, "from-b", gotA)
	})

	t.Run("SetPeerAddressesReloadsConfiguredRoutes", func(t *testing.T) {
		a := newClusterTestPubsub(t, Options{ClusterName: "reload"})
		b := newClusterTestPubsub(t, Options{ClusterName: "reload"})
		c := newClusterTestPubsub(t, Options{ClusterName: "reload"})

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.SetPeerAddresses([]string{addrC, addrB}))
		require.Equal(t, sortedRouteStrings(t, addrB, addrC), routeStrings(a.currentRoutes))

		require.NoError(t, a.SetPeerAddresses([]string{addrB, addrC}))
		require.Equal(t, sortedRouteStrings(t, addrB, addrC), routeStrings(a.currentRoutes))

		require.Error(t, a.SetPeerAddresses([]string{"nats://127.0.0.1:not-a-port"}))
		require.Equal(t, sortedRouteStrings(t, addrB, addrC), routeStrings(a.currentRoutes))

		require.NoError(t, a.SetPeerAddresses([]string{addrB}))
		require.Equal(t, sortedRouteStrings(t, addrB), routeStrings(a.currentRoutes))

		require.NoError(t, a.SetPeerAddresses(nil))
		require.Empty(t, a.currentRoutes)
	})

	t.Run("SetPeerAddressesStandaloneConfigError", func(t *testing.T) {
		ps := newClusterTestPubsub(t, Options{})
		err := ps.SetPeerAddresses(nil)
		require.ErrorContains(t, err, "not started with clustering enabled")
	})

	t.Run("SetPeerAddressesClosed", func(t *testing.T) {
		ps := newClusterTestPubsub(t, Options{ClusterName: "closed"})
		require.NoError(t, ps.Close())
		err := ps.SetPeerAddresses(nil)
		require.True(t, errors.Is(err, errClosed), "got %v", err)
	})

	t.Run("SetPeerAddressesDropsSelfRoute", func(t *testing.T) {
		ps := newClusterTestPubsub(t, Options{ClusterName: "self"})
		require.NoError(t, ps.SetPeerAddresses([]string{clusterRouteAddress(t, ps)}))
		require.Empty(t, ps.currentRoutes)
	})
}

func newClusterTestPubsub(t *testing.T, opts Options) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithCancel(context.Background())
	ps, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
		cancel()
	})
	return ps
}

func clusterRouteAddress(t *testing.T, ps *Pubsub) string {
	t.Helper()
	addr := ps.ns.ClusterAddr()
	require.NotNil(t, addr)
	return "nats://" + addr.String()
}

func waitForRoutes(t *testing.T, ps *Pubsub, minRoutes int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return ps.ns.NumRoutes() >= minRoutes
	}, testutil.WaitLong, testutil.IntervalFast)
}

func publishUntilReceived(t *testing.T, ps *Pubsub, event, want string, got <-chan []byte) {
	t.Helper()
	ticker := time.NewTicker(testutil.IntervalFast)
	defer ticker.Stop()
	ctx := testutil.Context(t, testutil.WaitLong)
	for {
		require.NoError(t, ps.Publish(event, []byte(want)))
		require.NoError(t, ps.Flush())
		select {
		case msg := <-got:
			assert.Equal(t, want, string(msg))
			return
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %q: %v", want, ctx.Err())
		}
	}
}

func receiveMessage(t *testing.T, got <-chan []byte) []byte {
	t.Helper()
	select {
	case msg := <-got:
		return msg
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for message")
		return nil
	}
}

func mustParseRoutes(t *testing.T, addresses ...string) []*url.URL {
	t.Helper()
	routes, err := parsePeerAddresses(addresses)
	require.NoError(t, err)
	return routes
}

func routeStrings(routes []*url.URL) []string {
	strings := make([]string, 0, len(routes))
	for _, route := range routes {
		strings = append(strings, route.String())
	}
	return strings
}

func sortedRouteStrings(t *testing.T, addresses ...string) []string {
	t.Helper()
	routes := mustParseRoutes(t, addresses...)
	sortRouteURLs(routes)
	return routeStrings(routes)
}

func uniqueSubject(prefix string) string {
	return fmt.Sprintf("cluster.%s.%d", prefix, time.Now().UnixNano())
}
