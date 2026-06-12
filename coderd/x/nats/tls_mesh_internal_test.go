package nats

import (
	"context"
	"testing"

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
		opts.ClusterTLS = &ClusterTLSOptions{
			CACert:  caCert,
			CAKey:   caKey,
			SANHost: "127.0.0.1",
		}

		a := newTestPubsub(t, opts)
		b := newTestPubsub(t, opts)
		c := newTestPubsub(t, opts)

		addrA := clusterRouteAddress(t, a)
		require.NoError(t, b.setPeerAddresses([]string{addrA}))
		require.NoError(t, c.setPeerAddresses([]string{addrA}))

		ctx := testutil.Context(t, testutil.WaitLong)
		received := make(chan string, 4)
		cancelSub, err := c.Subscribe("tls-mesh", func(_ context.Context, msg []byte) {
			select {
			case received <- string(msg):
			default:
			}
		})
		require.NoError(t, err)
		defer cancelSub()

		// b -> a -> c crosses two TLS route hops (gossip meshes b and c
		// through a).
		waitForRouteSubscription(t, b, "tls-mesh")
		require.NoError(t, b.Publish("tls-mesh", []byte("hello")))
		require.Equal(t, "hello", testutil.TryReceive(ctx, t, received))
	})

	t.Run("WrongCA", func(t *testing.T) {
		t.Parallel()

		caCert, caKey := generateTestCA(t)
		otherCACert, otherCAKey := generateTestCA(t)

		optsA := clusterTestOptions(t)
		optsA.ClusterTLS = &ClusterTLSOptions{
			CACert:  caCert,
			CAKey:   caKey,
			SANHost: "127.0.0.1",
		}
		a := newTestPubsub(t, optsA)

		optsB := optsA
		optsB.ClusterTLS = &ClusterTLSOptions{
			CACert:  otherCACert,
			CAKey:   otherCAKey,
			SANHost: "127.0.0.1",
		}
		b := newTestPubsub(t, optsB)

		require.NoError(t, b.setPeerAddresses([]string{clusterRouteAddress(t, a)}))
		require.Never(t, func() bool {
			return numRoutes(a) > 0 || numRoutes(b) > 0
		}, testutil.WaitShort, testutil.IntervalMedium)
	})

	t.Run("SANMismatch", func(t *testing.T) {
		t.Parallel()

		caCert, caKey := generateTestCA(t)
		opts := clusterTestOptions(t)
		opts.ClusterTLS = &ClusterTLSOptions{
			CACert:  caCert,
			CAKey:   caKey,
			SANHost: "127.0.0.1",
		}
		a := newTestPubsub(t, opts)

		// b's leaf is signed by the same CA but for a different IP. SAN
		// verification happens on the dialing side (the accept side
		// checks only the chain, as Go does not SAN-check client
		// certs), so a must dial b to hit b's mismatched SAN.
		optsB := opts
		optsB.ClusterTLS = &ClusterTLSOptions{
			CACert:  caCert,
			CAKey:   caKey,
			SANHost: "10.99.99.99",
		}
		b := newTestPubsub(t, optsB)

		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.Never(t, func() bool {
			return numRoutes(a) > 0 || numRoutes(b) > 0
		}, testutil.WaitShort, testutil.IntervalMedium)
	})

	t.Run("MixedTLSAndPlaintext", func(t *testing.T) {
		t.Parallel()

		caCert, caKey := generateTestCA(t)
		optsTLS := clusterTestOptions(t)
		optsTLS.ClusterTLS = &ClusterTLSOptions{
			CACert:  caCert,
			CAKey:   caKey,
			SANHost: "127.0.0.1",
		}
		a := newTestPubsub(t, optsTLS)

		optsPlain := optsTLS
		optsPlain.ClusterTLS = nil
		b := newTestPubsub(t, optsPlain)

		// Routes cannot form in either direction; rollout must enable
		// TLS on every replica of a deployment.
		require.NoError(t, b.setPeerAddresses([]string{clusterRouteAddress(t, a)}))
		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.Never(t, func() bool {
			return numRoutes(a) > 0 || numRoutes(b) > 0
		}, testutil.WaitShort, testutil.IntervalMedium)
	})
}
