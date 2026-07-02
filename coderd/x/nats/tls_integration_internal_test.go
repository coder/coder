package nats

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestPubsub_ClusterTLS_RealCA stands up a three-node TLS mesh whose trust root
// is a real CA served by the cryptokeys signing cache against a real DB, then
// verifies a cross-route publish/subscribe round-trip. This exercises the
// integration seam between the cryptokeys CA cache and the x/nats cluster TLS
// callbacks, including the real PEM/x509 round-trip that the synthetic
// generateTestCA helper does not cover. Nodes form a direct full mesh to avoid
// depending on multi-hop route gossip.
func TestPubsub_ClusterTLS_RealCA(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Seed an active nats_ca crypto key, mirroring the row the key rotator
	// mints in production. The signing cache decodes the PEM secret into a
	// *cryptokeys.NATSCA the same way production reads it.
	dbgen.CryptoKey(t, db, database.CryptoKey{
		Feature:  database.CryptoKeyFeatureNATSCA,
		Sequence: 1,
		StartsAt: time.Now().UTC().Add(-time.Hour),
	})

	newNode := func() *Pubsub {
		// A real signing cache per node, as each replica builds in coderd.New.
		cache, err := cryptokeys.NewSigningCache(ctx, slogtest.Make(t, nil), &cryptokeys.DBFetcher{DB: db}, codersdk.CryptoKeyFeatureNATSCA)
		require.NoError(t, err)
		t.Cleanup(func() { _ = cache.Close() })
		// Nodes mesh on loopback, so the leaf IP SAN must be 127.0.0.1.
		return newTLSPubsub(t, cache, net.IPv4(127, 0, 0, 1))
	}

	a := newNode()
	b := newNode()
	c := newNode()

	addrA := clusterRouteAddress(t, a)
	addrB := clusterRouteAddress(t, b)
	addrC := clusterRouteAddress(t, c)
	require.NoError(t, a.setPeerAddresses([]string{addrB, addrC}))
	require.NoError(t, b.setPeerAddresses([]string{addrA, addrC}))
	require.NoError(t, c.setPeerAddresses([]string{addrA, addrB}))

	received := make(chan string, 4)
	cancelSub, err := c.Subscribe("tls-realca", func(_ context.Context, msg []byte) {
		select {
		case received <- string(msg):
		default:
		}
	})
	require.NoError(t, err)
	defer cancelSub()

	// Routes and subscription interest propagate asynchronously after the
	// servers report ready, so retry rather than gate on a one-shot check.
	require.Eventually(t, func() bool {
		if err := b.Publish("tls-realca", []byte("hello")); err != nil {
			return false
		}
		select {
		case msg := <-received:
			require.Equal(t, "hello", msg)
			return true
		case <-time.After(testutil.IntervalMedium):
			return false
		}
	}, testutil.WaitLong, testutil.IntervalFast)
}
