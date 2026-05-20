//nolint:testpackage
package nats

import (
	"context"
	"strconv"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// newSoloPubsub spins up a "cluster of 1" embedded Pubsub: cluster mode
// is enabled but no peers are configured.
func newSoloPubsub(t *testing.T, opts Options) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	p, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestCluster_PeerProviderEmpty_ClusterOfOne(t *testing.T) {
	t.Parallel()
	p := newSoloPubsub(t, Options{
		PeerProvider: StaticPeerProvider(nil),
	})
	require.Equal(t, 0, p.ns.NumRoutes())
	// Cluster listener is bound even with zero peers.
	require.NotNil(t, p.ns.ClusterAddr())
}

func TestCluster_PeerProviderNil_ClusterOfOne(t *testing.T) {
	t.Parallel()
	p := newSoloPubsub(t, Options{})
	require.Equal(t, 0, p.ns.NumRoutes())
	require.NotNil(t, p.ns.ClusterAddr())
}

func TestCluster_RoutePoolSizePinned(t *testing.T) {
	t.Parallel()
	peers := []Peer{{RouteURL: "nats://127.0.0.1:6222"}}

	// Default (zero) → DefaultRoutePoolSize, ClusterPort 0 → RANDOM_PORT.
	got, err := buildServerOptions(Options{}, peers)
	require.NoError(t, err)
	require.Equal(t, DefaultRoutePoolSize, got.Cluster.PoolSize)
	require.Equal(t, natsserver.RANDOM_PORT, got.Cluster.Port)

	// Override.
	got, err = buildServerOptions(Options{RoutePoolSize: 7, ClusterPort: 12345}, peers)
	require.NoError(t, err)
	require.Equal(t, 7, got.Cluster.PoolSize)
	require.Equal(t, 12345, got.Cluster.Port)
}

func TestCluster_BuildOptions_ClientListener(t *testing.T) {
	t.Parallel()
	got, err := buildServerOptions(
		Options{},
		[]Peer{{RouteURL: "nats://127.0.0.1:6222"}},
	)
	require.NoError(t, err)
	require.False(t, got.DontListen)
	require.Equal(t, "127.0.0.1", got.Host)
	require.Equal(t, natsserver.RANDOM_PORT, got.Port)

	// Cluster of 1 also binds the client listener (no DontListen).
	got, err = buildServerOptions(Options{}, nil)
	require.NoError(t, err)
	require.False(t, got.DontListen)
	require.Equal(t, "127.0.0.1", got.Host)
	require.Equal(t, natsserver.RANDOM_PORT, got.Port)
}

// twoNodeCluster brings up two clustered Pubsubs that seed each other.
func twoNodeCluster(t *testing.T) (a, b *Pubsub) {
	t.Helper()
	portA := freePort(t)
	portB := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)

	a = buildClusterPubsub(t, "node-a", portA, []Peer{{RouteURL: urlB}})
	b = buildClusterPubsub(t, "node-b", portB, []Peer{{RouteURL: urlA}})
	waitForRoutes(t, a, 1)
	waitForRoutes(t, b, 1)
	return a, b
}

func crossPublish(t *testing.T, sender, receiver *Pubsub, event, payload string) {
	t.Helper()
	got := make(chan []byte, 1)
	cancel, err := receiver.Subscribe(event, func(_ context.Context, msg []byte) {
		select {
		case got <- msg:
		default:
		}
	})
	require.NoError(t, err)
	defer cancel()

	// Interest propagation across routes is async; retry publish until
	// the subscriber observes a message or the deadline fires.
	deadline := time.Now().Add(testutil.WaitMedium)
	for time.Now().Before(deadline) {
		require.NoError(t, sender.Publish(event, []byte(payload)))
		select {
		case msg := <-got:
			require.Equal(t, payload, string(msg))
			return
		case <-time.After(testutil.IntervalFast):
		}
	}
	t.Fatalf("did not receive cross-cluster message %q in time", payload)
}

func TestCluster_TwoServer_RoundTrip_AtoB(t *testing.T) {
	t.Parallel()
	a, b := twoNodeCluster(t)
	crossPublish(t, a, b, "evt-ab", "hello-from-a")
}

func TestCluster_TwoServer_RoundTrip_BtoA(t *testing.T) {
	t.Parallel()
	a, b := twoNodeCluster(t)
	crossPublish(t, b, a, "evt-ba", "hello-from-b")
}

func TestCluster_ThreeServer_RoundTrip(t *testing.T) {
	t.Parallel()
	portA := freePort(t)
	portB := freePort(t)
	portC := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)
	urlC := "nats://127.0.0.1:" + strconv.Itoa(portC)

	a := buildClusterPubsub(t, "node-a", portA, []Peer{{RouteURL: urlB}})
	b := buildClusterPubsub(t, "node-b", portB, []Peer{{RouteURL: urlA}, {RouteURL: urlC}})
	c := buildClusterPubsub(t, "node-c", portC, []Peer{{RouteURL: urlB}})

	waitForRoutes(t, a, 1)
	waitForRoutes(t, b, 2)
	waitForRoutes(t, c, 1)

	crossPublish(t, a, c, "evt-ac", "from-a-to-c")
	crossPublish(t, b, a, "evt-ba", "from-b-to-a")
}

// Ensure local pub/sub still works on a clustered node so we know
// cluster mode hasn't broken single-node semantics.
func TestCluster_LocalRoundTrip(t *testing.T) {
	t.Parallel()
	a, _ := twoNodeCluster(t)
	got := make(chan []byte, 1)
	cancel, err := a.Subscribe("local", func(_ context.Context, msg []byte) {
		got <- msg
	})
	require.NoError(t, err)
	defer cancel()
	require.NoError(t, a.Publish("local", []byte("hi")))
	select {
	case msg := <-got:
		require.Equal(t, "hi", string(msg))
	case <-time.After(testutil.WaitShort):
		t.Fatal("local publish not delivered")
	}
}

func TestNormalizePeers_RouteURLValidation(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		cases := []string{
			"nats://127.0.0.1:6222",
			"nats://nats-1.internal:6222",
			"nats://host",
			"nats://[::1]:6222",
			"nats://[2001:db8::1]:6222",
		}
		for _, raw := range cases {
			raw := raw
			t.Run(raw, func(t *testing.T) {
				t.Parallel()
				out, err := normalizePeers([]Peer{{RouteURL: raw}})
				require.NoError(t, err)
				require.Len(t, out, 1)
				require.Equal(t, raw, out[0].RouteURL)
			})
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		t.Parallel()
		out, err := normalizePeers([]Peer{{RouteURL: "  nats://127.0.0.1:6222  "}})
		require.NoError(t, err)
		require.Equal(t, "nats://127.0.0.1:6222", out[0].RouteURL)
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			url  string
		}{
			{"empty", ""},
			{"whitespace only", "   "},
			{"unsupported scheme tls", "tls://127.0.0.1:6222"},
			{"unsupported scheme http", "http://127.0.0.1:6222"},
			{"missing scheme", "127.0.0.1:6222"},
			{"userinfo", "nats://user:pass@127.0.0.1:6222"},
			{"userinfo no password", "nats://user@127.0.0.1:6222"},
			{"empty host", "nats://"},
			{"trailing slash path", "nats://127.0.0.1:6222/"},
			{"non-empty path", "nats://127.0.0.1:6222/route"},
			{"query", "nats://127.0.0.1:6222?foo=bar"},
			{"empty query", "nats://127.0.0.1:6222?"},
			{"fragment", "nats://127.0.0.1:6222#frag"},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := normalizePeers([]Peer{{RouteURL: tc.url}})
				require.Error(t, err, "expected error for %q", tc.url)
			})
		}
	})
}
