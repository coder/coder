package nats

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fakeCACache is an in-memory cryptokeys.SigningKeycache for tests. active is returned
// by SigningKey (the CA this replica mints leaves under); byID is consulted by
// VerifyingKey (the CAs this replica trusts when verifying peers).
type fakeCACache struct {
	active *cryptokeys.NATSCA
	byID   map[string]*cryptokeys.NATSCA
}

func (f *fakeCACache) SigningKey(context.Context) (string, interface{}, error) {
	if f.active == nil {
		return "", nil, cryptokeys.ErrKeyNotFound
	}
	return strconv.FormatInt(int64(f.active.Sequence), 10), f.active, nil
}

func (f *fakeCACache) VerifyingKey(_ context.Context, id string) (interface{}, error) {
	ca, ok := f.byID[id]
	if !ok {
		return nil, cryptokeys.ErrKeyNotFound
	}
	return ca, nil
}

func (*fakeCACache) Close() error { return nil }

func generateTestCA(t *testing.T, sequence int32) *cryptokeys.NATSCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(int64(sequence)),
		Subject:               pkix.Name{CommonName: "coder-nats-ca-test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(72 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return &cryptokeys.NATSCA{Sequence: sequence, Cert: cert, Key: crypto.Signer(key)}
}

// newTLSPubsub builds a clustered pubsub whose route listener requires mTLS,
// using the supplied CA cache. ip is this node's cluster host: the route
// listener bind host and the leaf IP SAN. Peers dial each other on that host
// (clusterRouteAddress), so ip must be 127.0.0.1 for routes to form.
func newTLSPubsub(t *testing.T, ca cryptokeys.SigningKeycache, ip net.IP) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, nil)
	ctx := testutil.Context(t, testutil.WaitLong)
	ps, err := New(ctx, logger, Options{
		ClusterHost:       ip.String(),
		ClusterPort:       natsserver.RANDOM_PORT,
		disableCluster:    false,
		ClusterCA:         ca,
		clusterTLSTimeout: testClusterTLSTimeout,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

// setLeafSAN overrides a node's leaf IP SAN after construction, for tests that
// need the minted SAN to differ from the loopback address the node binds and
// connects on (which are otherwise both the node's ClusterHost). Clearing the
// cached leaf forces the next handshake to re-mint under the new SAN.
func setLeafSAN(ps *Pubsub, ip net.IP) {
	ps.clusterTLS.mu.Lock()
	defer ps.clusterTLS.mu.Unlock()
	ps.clusterTLS.ip = ip
	ps.clusterTLS.leaf = nil
}

func numRoutes(t *testing.T, ps *Pubsub) int {
	t.Helper()
	routes, err := ps.Server.Routez(&natsserver.RoutezOptions{})
	require.NoError(t, err)
	return routes.NumRoutes
}

// TestPubsub_ClusterTLS validates that the embedded NATS server honors the
// tls.Config callbacks on cluster routes: leaves minted from the CA cache form
// a verified mesh, peers under unrelated CAs are rejected, and peers on either
// side of a CA rotation still verify each other.
func TestPubsub_ClusterTLS(t *testing.T) {
	t.Parallel()

	t.Run("Mesh", func(t *testing.T) {
		t.Parallel()

		ca := generateTestCA(t, 1)
		cache := func() *fakeCACache {
			return &fakeCACache{active: ca, byID: map[string]*cryptokeys.NATSCA{"1": ca}}
		}
		a := newTLSPubsub(t, cache(), net.IPv4(127, 0, 0, 1))
		b := newTLSPubsub(t, cache(), net.IPv4(127, 0, 0, 1))
		c := newTLSPubsub(t, cache(), net.IPv4(127, 0, 0, 1))

		addrA := clusterRouteAddress(t, a)
		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		// Full symmetric mesh: every node must know a peer to accept a route
		// from it (accept-side membership), so each is given the other two.
		// Drive peers through fetchers, as production does: a fetcher re-applies
		// the same peers on every refresh, so the startup refresh (which runs
		// with the boot-time noop fetcher) cannot race a manual call and wipe
		// the route/known-peer set.
		a.SetPeerFetcher(&testPeerFetcher{addresses: []string{addrB, addrC}})
		b.SetPeerFetcher(&testPeerFetcher{addresses: []string{addrA, addrC}})
		c.SetPeerFetcher(&testPeerFetcher{addresses: []string{addrA, addrB}})

		event := "tls-mesh"
		got := make(chan []byte, 8)
		cancel, err := c.Subscribe(event, func(_ context.Context, msg []byte) { got <- msg })
		require.NoError(t, err)
		defer cancel()

		// Retry publishes until the route subscription has propagated.
		require.Eventually(t, func() bool {
			if err := b.Publish(event, []byte("hello")); err != nil {
				return false
			}
			if err := b.Flush(); err != nil {
				return false
			}
			select {
			case msg := <-got:
				return string(msg) == "hello"
			case <-time.After(testutil.IntervalMedium):
				return false
			}
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("WrongCARejected", func(t *testing.T) {
		t.Parallel()

		caX := generateTestCA(t, 1)
		caY := generateTestCA(t, 1)
		a := newTLSPubsub(t, &fakeCACache{active: caX, byID: map[string]*cryptokeys.NATSCA{"1": caX}}, net.IPv4(127, 0, 0, 1))
		b := newTLSPubsub(t, &fakeCACache{active: caY, byID: map[string]*cryptokeys.NATSCA{"1": caY}}, net.IPv4(127, 0, 0, 1))

		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))

		// Each side only trusts its own CA, so the route handshake never
		// completes and no route is established.
		require.Never(t, func() bool {
			return numRoutes(t, a) > 0 || numRoutes(t, b) > 0
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("RotationOverlap", func(t *testing.T) {
		t.Parallel()

		ca1 := generateTestCA(t, 1)
		ca2 := generateTestCA(t, 2)
		bundle := map[string]*cryptokeys.NATSCA{"1": ca1, "2": ca2}
		// a still mints under the old CA; b has already rotated to the new CA.
		// Both trust both CAs, so the mesh forms across the rotation overlap.
		a := newTLSPubsub(t, &fakeCACache{active: ca1, byID: bundle}, net.IPv4(127, 0, 0, 1))
		b := newTLSPubsub(t, &fakeCACache{active: ca2, byID: bundle}, net.IPv4(127, 0, 0, 1))

		// Symmetric peers so each side accepts a route from the other, driven
		// through fetchers (see Mesh) so the startup noop refresh cannot race a
		// manual call and wipe the route/known-peer set.
		a.SetPeerFetcher(&testPeerFetcher{addresses: []string{clusterRouteAddress(t, b)}})
		b.SetPeerFetcher(&testPeerFetcher{addresses: []string{clusterRouteAddress(t, a)}})

		require.Eventually(t, func() bool {
			return numRoutes(t, a) > 0 && numRoutes(t, b) > 0
		}, testutil.WaitLong, testutil.IntervalFast)
	})

	t.Run("SANMismatch", func(t *testing.T) {
		t.Parallel()

		ca := generateTestCA(t, 1)
		cache := func() *fakeCACache {
			return &fakeCACache{active: ca, byID: map[string]*cryptokeys.NATSCA{"1": ca}}
		}
		// Both nodes bind and connect on loopback and know each other as peers,
		// so the CA and source-membership checks pass. But both mint their leaf
		// with a SAN that does not match the loopback address they connect from,
		// so every handshake is rejected on the SAN binding alone and no route
		// forms. This isolates the SAN check: a valid CA-signed leaf presented
		// from a known replica is still rejected when the cert is not bound to
		// the address it connects from (e.g. a stolen or mis-minted leaf).
		a := newTLSPubsub(t, cache(), net.IPv4(127, 0, 0, 1))
		b := newTLSPubsub(t, cache(), net.IPv4(127, 0, 0, 1))
		setLeafSAN(a, net.IPv4(10, 99, 99, 99))
		setLeafSAN(b, net.IPv4(10, 99, 99, 99))

		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.NoError(t, b.setPeerAddresses([]string{clusterRouteAddress(t, a)}))

		require.Never(t, func() bool {
			return numRoutes(t, a) > 0 || numRoutes(t, b) > 0
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("MixedTLSAndPlaintext", func(t *testing.T) {
		t.Parallel()

		ca := generateTestCA(t, 1)
		// a requires mTLS on its route listener; b is a plaintext node
		// (newTestPubsub leaves ClusterCA nil). Routes must not form in either
		// direction: a rollout has to enable TLS on every replica at once.
		a := newTLSPubsub(t, &fakeCACache{active: ca, byID: map[string]*cryptokeys.NATSCA{"1": ca}}, net.IPv4(127, 0, 0, 1))
		b := newTestPubsub(t, clusterTestOptions(t))

		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.NoError(t, b.setPeerAddresses([]string{clusterRouteAddress(t, a)}))

		require.Never(t, func() bool {
			return numRoutes(t, a) > 0 || numRoutes(t, b) > 0
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}

// TestPubsub_ClusterTLS_CacheSwap covers the Part C optional-mTLS model: a node
// that boots with the noop CA cache forms no route, and swapping in a real cache
// via SetCACache lets routes form over mTLS with no server restart.
func TestPubsub_ClusterTLS_CacheSwap(t *testing.T) {
	t.Parallel()

	t.Run("NoopFormsNoRoute", func(t *testing.T) {
		t.Parallel()

		ca := generateTestCA(t, 1)
		// a boots with the noop cache (production default); b has a real cache.
		// a cannot mint a leaf, so its route handshakes fail and no route forms.
		a := newTLSPubsub(t, cryptokeys.NoopSigningKeycache{}, net.IPv4(127, 0, 0, 1))
		b := newTLSPubsub(t, &fakeCACache{active: ca, byID: map[string]*cryptokeys.NATSCA{"1": ca}}, net.IPv4(127, 0, 0, 1))

		require.NoError(t, a.setPeerAddresses([]string{clusterRouteAddress(t, b)}))
		require.NoError(t, b.setPeerAddresses([]string{clusterRouteAddress(t, a)}))

		require.Never(t, func() bool {
			return numRoutes(t, a) > 0 || numRoutes(t, b) > 0
		}, testutil.WaitShort, testutil.IntervalFast)
	})

	t.Run("SwapToRealFormsRoute", func(t *testing.T) {
		t.Parallel()

		ca := generateTestCA(t, 1)
		realCache := func() *fakeCACache {
			return &fakeCACache{active: ca, byID: map[string]*cryptokeys.NATSCA{"1": ca}}
		}
		// Both boot with the noop cache, then both get the real cache swapped in
		// (mirroring the enterprise HA enable path) without a server restart.
		a := newTLSPubsub(t, cryptokeys.NoopSigningKeycache{}, net.IPv4(127, 0, 0, 1))
		b := newTLSPubsub(t, cryptokeys.NoopSigningKeycache{}, net.IPv4(127, 0, 0, 1))

		// Drive peers through fetchers, as production does, rather than calling
		// setPeerAddresses directly: SetCACache and SetPeerFetcher both trigger
		// a peer refresh that reads the current fetcher, so routes converge on
		// the fetcher's addresses without racing a manual call.
		a.SetCACache(realCache())
		b.SetCACache(realCache())

		a.SetPeerFetcher(&testPeerFetcher{addresses: []string{clusterRouteAddress(t, b)}})
		b.SetPeerFetcher(&testPeerFetcher{addresses: []string{clusterRouteAddress(t, a)}})

		require.Eventually(t, func() bool {
			return numRoutes(t, a) > 0 && numRoutes(t, b) > 0
		}, testutil.WaitLong, testutil.IntervalFast)
	})
}

// TestClusterTLS_configForClient_RequiresSourceIP asserts the accept side fails
// closed when it cannot determine the peer's source IP, rather than skipping the
// source-binding check.
func TestClusterTLS_configForClient_RequiresSourceIP(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ca := generateTestCA(t, 1)
	cache := &fakeCACache{active: ca, byID: map[string]*cryptokeys.NATSCA{"1": ca}}
	ct := newClusterTLS(ctx, slogtest.Make(t, nil), nil, cache, net.IPv4(127, 0, 0, 1))

	// No underlying connection: the source IP cannot be determined, so the
	// accept-side config is refused and the handshake aborts.
	_, err := ct.configForClient(&tls.ClientHelloInfo{})
	require.Error(t, err)
}

// TestClusterTLS_verify unit-tests the verifier directly, isolating chain
// verification and source-IP binding that the mesh tests exercise only
// indirectly.
func TestClusterTLS_verify(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ca := generateTestCA(t, 1)
	cache := &fakeCACache{active: ca, byID: map[string]*cryptokeys.NATSCA{"1": ca}}

	leafIP := net.IPv4(10, 0, 0, 5)
	ct := newClusterTLS(ctx, slogtest.Make(t, nil), nil, cache, net.IPv4(10, 0, 0, 1))

	// A leaf bound to leafIP, signed by the trusted CA.
	leafCert, err := mintLeaf(ca, leafIP, time.Now())
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(leafCert.Certificate[0])
	require.NoError(t, err)
	cs := tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}

	t.Run("DialSideChainOnly", func(t *testing.T) {
		t.Parallel()
		// No source IP (dial side): only the chain is verified.
		require.NoError(t, ct.verify(cs, nil))
	})

	t.Run("AcceptSideSourceMatches", func(t *testing.T) {
		t.Parallel()
		// Source IP equals the leaf SAN: accepted.
		require.NoError(t, ct.verify(cs, leafIP))
	})

	t.Run("AcceptSideSourceMismatch", func(t *testing.T) {
		t.Parallel()
		// The leaf is bound to leafIP, so a connection from a different source
		// is rejected even though the chain is valid.
		err := ct.verify(cs, net.IPv4(10, 0, 0, 1))
		require.ErrorContains(t, err, "do not match source IP")
	})

	t.Run("UntrustedCARejected", func(t *testing.T) {
		t.Parallel()
		otherCA := generateTestCA(t, 9)
		strangerCert, err := mintLeaf(otherCA, leafIP, time.Now())
		require.NoError(t, err)
		stranger, err := x509.ParseCertificate(strangerCert.Certificate[0])
		require.NoError(t, err)
		// The stamped sequence (9) is not in the cache, so the CA lookup fails.
		err = ct.verify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{stranger}}, nil)
		require.Error(t, err)
	})
}

// TestClusterTLS_verifyPool asserts the verify-pool cache reuses a pool for a
// given CA sequence and prunes entries whose CA cert has expired, so the map
// does not grow unbounded across rotations.
func TestClusterTLS_verifyPool(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	clock.Set(time.Now())

	ca1 := generateTestCA(t, 1)
	ca2 := generateTestCA(t, 2)
	cache := &fakeCACache{byID: map[string]*cryptokeys.NATSCA{"1": ca1, "2": ca2}}
	ct := newClusterTLS(ctx, slogtest.Make(t, nil), clock, cache, net.IPv4(10, 0, 0, 1))

	// First build for seq 1 caches the pool; a second call returns the same one.
	p1 := ct.verifyPool("1", ca1.Cert)
	require.Same(t, p1, ct.verifyPool("1", ca1.Cert))
	require.Len(t, ct.verifyPools, 1)

	// Advance past ca1's NotAfter. Building a pool for a new sequence prunes the
	// now-expired seq 1 entry, leaving only seq 2.
	clock.Set(ca1.Cert.NotAfter.Add(time.Minute))
	ct.verifyPool("2", ca2.Cert)
	require.Len(t, ct.verifyPools, 1)
	_, ok := ct.verifyPools["1"]
	require.False(t, ok, "expired seq 1 pool should be pruned")
	_, ok = ct.verifyPools["2"]
	require.True(t, ok)
}

// generateTestCAWithValidity is like generateTestCA but lets a test control the
// CA certificate's NotAfter, so leaf-clamp behavior near CA expiry is testable.
func generateTestCAWithValidity(t *testing.T, sequence int32, notAfter time.Time) *cryptokeys.NATSCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(int64(sequence)),
		Subject:               pkix.Name{CommonName: "coder-nats-ca-test"},
		NotBefore:             notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return &cryptokeys.NATSCA{Sequence: sequence, Cert: cert, Key: crypto.Signer(key)}
}

// TestMintLeaf asserts a leaf's NotAfter is exactly its signing CA's NotAfter
// (a leaf carries no independent lifetime), and that minting against an
// already-expired CA fails rather than emitting a dead leaf.
func TestMintLeaf(t *testing.T) {
	t.Parallel()

	ip := net.IPv4(127, 0, 0, 1)

	t.Run("MatchesCAValidity", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		// generateTestCA mints a 72h CA; the leaf's NotAfter tracks it exactly.
		ca := generateTestCA(t, 1)
		leaf, err := mintLeaf(ca, ip, now)
		require.NoError(t, err)
		require.WithinDuration(t, ca.Cert.NotAfter, leaf.Leaf.NotAfter, time.Second)
		require.WithinDuration(t, now.Add(-clockSkewToleranceTLS), leaf.Leaf.NotBefore, time.Second)
	})

	t.Run("ErrorsWhenCAExpired", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		// CA's NotAfter is already in the past: no usable leaf can be minted.
		ca := generateTestCAWithValidity(t, 1, now.Add(-time.Minute))
		_, err := mintLeaf(ca, ip, now)
		require.Error(t, err)
		require.ErrorContains(t, err, "expired")
	})
}

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
	// Drive peers through fetchers, as production does, so the startup noop
	// refresh cannot race a manual call and wipe the route/known-peer set.
	a.SetPeerFetcher(&testPeerFetcher{addresses: []string{addrB, addrC}})
	b.SetPeerFetcher(&testPeerFetcher{addresses: []string{addrA, addrC}})
	c.SetPeerFetcher(&testPeerFetcher{addresses: []string{addrA, addrB}})

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
