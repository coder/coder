package nats

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// TestPubsub_ClusterTLS_RealCA stands up a three-node TLS mesh whose trust
// root is a real CA fetched from cryptokeys (create-at-fetch against a real
// DB), then verifies a cross-route publish/subscribe round-trip. This
// exercises the integration seam between the cryptokeys CA and the x/nats
// cluster TLS reload path, including the real PEM/x509 round-trip that the
// synthetic generateTestCA helper does not cover.
//
// TLS is installed via the peer-route reload path (setClusterTLS arms the
// provider and applies it with an empty-routes reload), matching production
// where the enterprise provider mints the leaf from the CA cache. Nodes form a
// direct full mesh to avoid depending on multi-hop route gossip.
func TestPubsub_ClusterTLS_RealCA(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Seed an active nats_ca crypto key, mirroring the row the key rotator
	// mints in production (FetchNATSCA is read-only and never creates).
	dbgen.CryptoKey(t, db, database.CryptoKey{
		Feature:  database.CryptoKeyFeatureNATSCA,
		Sequence: 1,
		StartsAt: time.Now().UTC().Add(-time.Hour),
	})

	// Real CA from the cryptokeys accessor: parses the seeded secret into a
	// cert+key the same way production reads it.
	ca, err := cryptokeys.FetchNATSCA(ctx, testutil.Logger(t), db)
	require.NoError(t, err)

	// Nodes mesh on loopback, so the leaf IP-SAN must be 127.0.0.1. Driving
	// it through ClusterTLSOptionsFromRelayURL also exercises the production
	// seam (relay URL host -> SANIP).
	relayURL, err := url.Parse("nats://127.0.0.1:6222")
	require.NoError(t, err)
	tlsOpts, err := ClusterTLSOptionsFromRelayURL(relayURL, ca.Cert, ca.Key)
	require.NoError(t, err)

	opts := clusterTestOptions(t)
	a := newTestPubsub(t, opts)
	b := newTestPubsub(t, opts)
	c := newTestPubsub(t, opts)
	setClusterTLS(t, a, *tlsOpts)
	setClusterTLS(t, b, *tlsOpts)
	setClusterTLS(t, c, *tlsOpts)

	// Form a direct full mesh, mirroring production where every replica peers
	// with every other. This avoids depending on multi-hop route gossip.
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

	// Publish until the message arrives over the TLS route: routes and
	// subscription interest propagate asynchronously after the servers report
	// ready, so retry rather than gate on a one-shot route-state check.
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
