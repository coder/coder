package nats

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func numRoutes(ps *Pubsub) int {
	routez, err := ps.Server.Routez(&natsserver.RoutezOptions{})
	if err != nil {
		return 0
	}
	return len(routez.Routes)
}

func TestPubsub_ClusterTLS(t *testing.T) {
	t.Parallel()

	t.Run("Mesh", func(t *testing.T) {
		t.Parallel()

		caCert, caKey := generateTestCA(t)
		opts := clusterTestOptions(t)
		tls := &ClusterTLSOptions{
			CACert: caCert,
			CAKey:  caKey,
			SANIP:  "127.0.0.1",
		}

		a := newTestPubsub(t, opts, tls)
		b := newTestPubsub(t, opts, tls)
		c := newTestPubsub(t, opts, tls)

		// Form a direct full mesh, mirroring production where every
		// replica peers with every other (replicasync returns all peer
		// addresses). This avoids depending on multi-hop route gossip.
		addrA := clusterRouteAddress(t, a)
		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.setPeerAddresses([]string{addrB, addrC}))
		require.NoError(t, b.setPeerAddresses([]string{addrA, addrC}))
		require.NoError(t, c.setPeerAddresses([]string{addrA, addrB}))

		received := make(chan string, 4)
		cancelSub, err := c.Subscribe("tls-mesh", func(_ context.Context, msg []byte) {
			select {
			case received <- string(msg):
			default:
			}
		})
		require.NoError(t, err)
		defer cancelSub()

		// Publish until the message arrives over the TLS route: routes
		// and subscription interest propagate asynchronously after the
		// servers report ready, so retry rather than gate on a one-shot
		// route-state check.
		require.Eventually(t, func() bool {
			if err := b.Publish("tls-mesh", []byte("hello")); err != nil {
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
	})

	t.Run("WrongCA", func(t *testing.T) {
		t.Parallel()

		caCert, caKey := generateTestCA(t)
		otherCACert, otherCAKey := generateTestCA(t)

		optsA := clusterTestOptions(t)
		tlsA := &ClusterTLSOptions{
			CACert: caCert,
			CAKey:  caKey,
			SANIP:  "127.0.0.1",
		}
		a := newTestPubsub(t, optsA, tlsA)

		optsB := optsA
		tlsB := &ClusterTLSOptions{
			CACert: otherCACert,
			CAKey:  otherCAKey,
			SANIP:  "127.0.0.1",
		}
		b := newTestPubsub(t, optsB, tlsB)

		require.NoError(t, b.setPeerAddresses([]string{clusterRouteAddress(t, a)}))
		require.Never(t, func() bool {
			return numRoutes(a) > 0 || numRoutes(b) > 0
		}, testutil.WaitShort, testutil.IntervalMedium)
	})

	t.Run("SANMismatch", func(t *testing.T) {
		t.Parallel()

		caCert, caKey := generateTestCA(t)
		opts := clusterTestOptions(t)
		tls := &ClusterTLSOptions{
			CACert: caCert,
			CAKey:  caKey,
			SANIP:  "127.0.0.1",
		}
		a := newTestPubsub(t, opts, tls)

		// b's leaf is signed by the same CA but for a different IP. SAN
		// verification happens on the dialing side (the accept side
		// checks only the chain, as Go does not SAN-check client
		// certs), so a must dial b to hit b's mismatched SAN.
		optsB := opts
		tlsB := &ClusterTLSOptions{
			CACert: caCert,
			CAKey:  caKey,
			SANIP:  "10.99.99.99",
		}
		b := newTestPubsub(t, optsB, tlsB)

		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.Never(t, func() bool {
			return numRoutes(a) > 0 || numRoutes(b) > 0
		}, testutil.WaitShort, testutil.IntervalMedium)
	})

	t.Run("MixedTLSAndPlaintext", func(t *testing.T) {
		t.Parallel()

		caCert, caKey := generateTestCA(t)
		opts := clusterTestOptions(t)
		tls := &ClusterTLSOptions{
			CACert: caCert,
			CAKey:  caKey,
			SANIP:  "127.0.0.1",
		}
		a := newTestPubsub(t, opts, tls)
		b := newTestPubsub(t, opts, nil)

		// Routes cannot form in either direction; rollout must enable
		// TLS on every replica of a deployment.
		require.NoError(t, b.setPeerAddresses([]string{clusterRouteAddress(t, a)}))
		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.Never(t, func() bool {
			return numRoutes(a) > 0 || numRoutes(b) > 0
		}, testutil.WaitShort, testutil.IntervalMedium)
	})
}
