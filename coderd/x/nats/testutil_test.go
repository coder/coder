//nolint:testpackage
package nats

import (
	"context"
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

// genTestCert generates a self-signed leaf cert valid for the given DNS
// names and 127.0.0.1. Returns a CA-equivalent root pool (containing
// the self-signed cert) and the server cert.
func genTestCert(t *testing.T, dnsNames []string) (*x509.CertPool, tls.Certificate) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "coder-nats-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(parsed)

	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
		Leaf:        parsed,
	}
	return pool, cert
}

// buildClusterPubsub constructs and starts a Pubsub configured for cluster
// mode with the supplied peers and optional TLS. The Pubsub is closed
// during test cleanup.
func buildClusterPubsub(t testing.TB, name string, port int, peers []Peer, token string, tlsConfig *tls.Config) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Named(name).Leveled(slog.LevelDebug)

	opts := Options{
		ServerName:       name,
		ClusterName:      "test-cluster",
		ClusterToken:     token,
		ClusterHost:      "127.0.0.1",
		ClusterPort:      port,
		ClusterAdvertise: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		ClusterTLSConfig: tlsConfig,
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
