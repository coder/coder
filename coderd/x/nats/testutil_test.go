//nolint:testpackage
package nats

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// freePort returns a port that was bindable on 127.0.0.1 at the moment
// of the call. There is an inherent TOCTOU race; tests should only use
// this when no other strategy is available.
func freePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// buildClusterPubsub constructs and starts a Pubsub configured for cluster
// mode with the supplied peers. The Pubsub is closed during test cleanup.
func buildClusterPubsub(t testing.TB, name string, port int, peers []Peer) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Named(name).Leveled(slog.LevelDebug)

	opts := Options{
		ServerName:       name,
		ClusterName:      "test-cluster",
		ClusterHost:      "127.0.0.1",
		ClusterPort:      port,
		ClusterAdvertise: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		PeerProvider:     StaticPeerProvider(peers),
		ReadyTimeout:     testutil.WaitMedium,
	}
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	p, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// waitForRoutes waits until the embedded server reports at least the
// expected number of established routes, or fails the test.
func waitForRoutes(t testing.TB, p *Pubsub, expected int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return p.ns.NumRoutes() >= expected
	}, testutil.WaitMedium, testutil.IntervalFast, "expected at least %d routes", expected)
}
