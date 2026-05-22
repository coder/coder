package nats //nolint:testpackage // Exercises internal cluster helpers and state.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

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
			"nats://[::1]:7222",
			"nats://example.com:6222",
		})
		require.NoError(t, err)
		require.Equal(t, []string{
			"nats://127.0.0.1:4222",
			"nats://[::1]:7222",
			"nats://example.com:6222",
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

	t.Run("FiltersSortsAndDedupes", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
			"nats://b.example:6222",
			"nats://a.example:6222",
			"nats://b.example:6222",
			"nats://self.example:6222",
		}, "self.example:6222")
		require.NoError(t, err)
		require.Equal(t, []string{
			"nats://a.example:6222",
			"nats://b.example:6222",
		}, routeStrings(routes))
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

//nolint:paralleltest // Cluster tests bind free ports and reload shared route state.
func TestPubsubCluster(t *testing.T) {
	t.Run("LocalRoundTrip", func(t *testing.T) {
		ps := newClusterTestPubsub(t, newClusterTestOptions(t))
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

	t.Run("SetPeerAddressesReloadsConfiguredRoutes", func(t *testing.T) {
		a := newClusterTestPubsub(t, newClusterTestOptions(t))
		b := newClusterTestPubsub(t, newClusterTestOptions(t))
		c := newClusterTestPubsub(t, newClusterTestOptions(t))

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.SetPeerAddresses([]string{addrB}))
		waitForRoutes(t, a, 1)
		waitForRoutes(t, b, 1)

		eventC := uniqueSubject("add-c")
		gotC := make(chan []byte, 8)
		cancelC, err := c.Subscribe(eventC, func(_ context.Context, msg []byte) {
			gotC <- msg
		})
		require.NoError(t, err)
		defer cancelC()

		require.NoError(t, a.SetPeerAddresses([]string{addrC, addrB}))
		require.Equal(t, sortedRouteStrings(t, addrB, addrC), routeStrings(a.currentRoutes))
		waitForRoutes(t, a, 2)
		waitForRoutes(t, c, 1)
		publishUntilReceived(t, a, eventC, "from-a-to-c", gotC)

		require.Error(t, a.SetPeerAddresses([]string{"nats://127.0.0.1:not-a-port"}))
		require.Equal(t, sortedRouteStrings(t, addrB, addrC), routeStrings(a.currentRoutes))

		eventAC := uniqueSubject("remove-b")
		gotA := make(chan []byte, 8)
		cancelA, err := a.Subscribe(eventAC, func(_ context.Context, msg []byte) {
			gotA <- msg
		})
		require.NoError(t, err)
		defer cancelA()

		require.NoError(t, a.SetPeerAddresses([]string{addrC}))
		require.Equal(t, sortedRouteStrings(t, addrC), routeStrings(a.currentRoutes))
		publishUntilReceived(t, c, eventAC, "from-c-to-a", gotA)
	})

	t.Run("SetPeerAddressesStandaloneConfigError", func(t *testing.T) {
		ps := newClusterTestPubsub(t, Options{})
		err := ps.SetPeerAddresses(nil)
		require.ErrorContains(t, err, "not started with clustering enabled")
	})

	t.Run("SetPeerAddressesClosed", func(t *testing.T) {
		ps := newClusterTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.Close())
		err := ps.SetPeerAddresses(nil)
		require.True(t, errors.Is(err, errClosed), "got %v", err)
	})

	t.Run("SetPeerAddressesDropsSelfRoute", func(t *testing.T) {
		ps := newClusterTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.SetPeerAddresses([]string{clusterRouteAddress(t, ps)}))
		require.Empty(t, ps.currentRoutes)
	})
}

func newClusterTestOptions(t *testing.T) Options {
	t.Helper()
	return Options{ClusterPort: freeTCPPort(t)}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return addr.Port
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
	return routeStrings(mustParseRoutes(t, addresses...))
}

func uniqueSubject(prefix string) string {
	return fmt.Sprintf("cluster.%s.%d", prefix, time.Now().UnixNano())
}
