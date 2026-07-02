package nats

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"time"

	"golang.org/x/xerrors"
)

// leafCertValidity is how long a replica's ephemeral cluster leaf
// certificate remains valid. Leaves are minted in memory at startup and
// die with the process, so this only needs to exceed the maximum
// expected process lifetime between restarts.
const leafCertValidity = 30 * 24 * time.Hour

// clusterTLSTimeout bounds the route TLS handshake. It replaces the
// NATS default of 2s, which is tight under CI load.
const clusterTLSTimeout = 10 * time.Second

// RFC 5280 limits certificate serial numbers to 20 octets (160 bits); 128 random bits stays well under that ceiling
// while still giving enough entropy to make collisions negligible.
const serialNumberBits = 128

// ClusterTLSOptions configures mutual TLS for the inter-replica NATS
// cluster route listener. The CA signs an ephemeral per-replica leaf
// certificate at startup; the leaf private key is never persisted.
type ClusterTLSOptions struct {
	// CACert is the deployment-wide cluster CA certificate. Peers are
	// verified against it in both directions of a route handshake.
	CACert *x509.Certificate

	// CAKey is the private key for CACert, used to sign this replica's
	// leaf certificate.
	CAKey crypto.Signer

	// SANIP is this replica's relay-URL host, embedded as an IP SAN
	// in the leaf certificate. It must be an IP address and must match
	// the address peers dial, or route TLS handshakes fail with a
	// hostname verification error.
	SANIP string
}

// ClusterTLSOptionsFromRelayURL derives ClusterTLSOptions from this replica's relay URL, whose host is the address
// peers dial for cluster routes and therefore the leaf certificate's IP SAN. The relay host must be an IP address.
// It validates eagerly by building a trial TLS config; errors from that step propagate directly.
func ClusterTLSOptionsFromRelayURL(relayURL *url.URL, caCert *x509.Certificate, caKey crypto.Signer) (*ClusterTLSOptions, error) {
	if relayURL == nil {
		return nil, xerrors.New("cluster TLS: relay URL is required")
	}
	opts := &ClusterTLSOptions{
		CACert: caCert,
		CAKey:  caKey,
		SANIP:  relayURL.Hostname(),
	}
	// Surface invalid options at construction time rather than at server startup.
	if _, err := BuildClusterTLSConfig(*opts); err != nil {
		return nil, err
	}
	return opts, nil
}

// BuildClusterTLSConfig mints an ephemeral ECDSA P-256 leaf certificate signed by the configured CA and returns a
// *tls.Config suitable for natsserver.ClusterOpts.TLSConfig. The same config serves both route roles: the NATS server
// uses it when accepting routes and clones it (setting ServerName from the dialed route URL) when soliciting them.
func BuildClusterTLSConfig(opts ClusterTLSOptions) (*tls.Config, error) {
	if opts.CACert == nil {
		return nil, xerrors.New("cluster TLS: CA certificate is required")
	}
	if opts.CAKey == nil {
		return nil, xerrors.New("cluster TLS: CA private key is required")
	}

	if !opts.CACert.IsCA {
		return nil, xerrors.New("cluster TLS: CA certificate does not have the CA basic constraint")
	}
	if opts.CACert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, xerrors.New("cluster TLS: CA certificate is missing KeyUsageCertSign")
	}

	ip := net.ParseIP(opts.SANIP)
	if ip == nil {
		return nil, xerrors.Errorf("cluster TLS: SAN host %q is not an IP address; coder NATS cluster TLS only supports IP-based SANs", opts.SANIP)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("generate leaf key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), serialNumberBits)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, xerrors.Errorf("cluster TLS: generate leaf serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: opts.SANIP,
		},
		IPAddresses: []net.IP{ip},
		NotBefore:   now.Add(-5 * time.Minute),
		NotAfter:    now.Add(leafCertValidity),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		// Both usages are required: on a route each server acts as the
		// TLS server when accepting and the TLS client when dialing.
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, template, opts.CACert, &leafKey.PublicKey, opts.CAKey)
	if err != nil {
		return nil, xerrors.Errorf("cluster TLS: sign leaf certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, xerrors.Errorf("cluster TLS: parse leaf certificate: %w", err)
	}

	// A pool rather than a single cert so multiple CA certificates can
	// coexist during a future CA rotation overlap window.
	caPool := x509.NewCertPool()
	caPool.AddCert(opts.CACert)

	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{leafDER},
			PrivateKey:  leafKey,
			Leaf:        leaf,
		}},
		// RootCAs verifies peers when dialing routes; ClientCAs
		// verifies the dialing peer's certificate when accepting them.
		RootCAs:    caPool,
		ClientCAs:  caPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}, nil
}
