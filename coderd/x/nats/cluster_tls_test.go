//nolint:testpackage
package nats

import (
	"context"
	"crypto/tls"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestCluster_TLS_RoundTrip(t *testing.T) {
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

	token := "tls-token"
	portA := freePort(t)
	portB := freePort(t)
	urlA := "tls://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "tls://127.0.0.1:" + strconv.Itoa(portB)

	a := buildClusterPubsub(t, "tls-a", portA, []Peer{{RouteURL: urlB}}, token, mkCfg())
	b := buildClusterPubsub(t, "tls-b", portB, []Peer{{RouteURL: urlA}}, token, mkCfg())

	waitForRoutes(t, a, 1)
	waitForRoutes(t, b, 1)

	crossPublish(t, a, b, "tls-evt", "tls-hello")
}

func TestCluster_TokenMismatch(t *testing.T) {
	t.Parallel()
	portA := freePort(t)
	portB := freePort(t)
	urlA := "nats://127.0.0.1:" + strconv.Itoa(portA)
	urlB := "nats://127.0.0.1:" + strconv.Itoa(portB)

	a := buildClusterPubsub(t, "auth-a", portA, []Peer{{RouteURL: urlB}}, "alpha", nil)
	b := buildClusterPubsub(t, "auth-b", portB, []Peer{{RouteURL: urlA}}, "beta", nil)

	// Routes may briefly count during handshake before auth rejection
	// closes them. Assert that within a bounded window NumRoutes settles
	// back to 0 on both nodes and stays there.
	require.Eventually(t, func() bool {
		return a.ns.NumRoutes() == 0 && b.ns.NumRoutes() == 0
	}, testutil.WaitMedium, testutil.IntervalFast,
		"expected 0 routes after auth rejection")
	tick := time.NewTicker(testutil.IntervalFast)
	defer tick.Stop()
	stable := time.After(testutil.IntervalMedium * 2)
stableLoop:
	for {
		select {
		case <-stable:
			break stableLoop
		case <-tick.C:
			require.Equal(t, 0, a.ns.NumRoutes())
			require.Equal(t, 0, b.ns.NumRoutes())
		}
	}

	// Cross-cluster delivery must not occur.
	got := make(chan []byte, 1)
	cancel, err := b.Subscribe("mismatch", func(_ context.Context, msg []byte) {
		got <- msg
	})
	require.NoError(t, err)
	defer cancel()
	require.NoError(t, a.Publish("mismatch", []byte("nope")))
	select {
	case <-got:
		t.Fatal("received message across mismatched-token cluster")
	case <-time.After(testutil.IntervalMedium):
	}

	// Each node still works locally.
	gotA := make(chan []byte, 1)
	cancelA, err := a.Subscribe("local-a", func(_ context.Context, msg []byte) {
		gotA <- msg
	})
	require.NoError(t, err)
	defer cancelA()
	require.NoError(t, a.Publish("local-a", []byte("ok-a")))
	select {
	case msg := <-gotA:
		require.Equal(t, "ok-a", string(msg))
	case <-time.After(testutil.WaitShort):
		t.Fatal("local A delivery failed")
	}

	gotB := make(chan []byte, 1)
	cancelB, err := b.Subscribe("local-b", func(_ context.Context, msg []byte) {
		gotB <- msg
	})
	require.NoError(t, err)
	defer cancelB()
	require.NoError(t, b.Publish("local-b", []byte("ok-b")))
	select {
	case msg := <-gotB:
		require.Equal(t, "ok-b", string(msg))
	case <-time.After(testutil.WaitShort):
		t.Fatal("local B delivery failed")
	}
}
