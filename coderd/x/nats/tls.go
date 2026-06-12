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

	// SANHost is this replica's relay-URL host, embedded as an IP SAN
	// in the leaf certificate. It must be an IP address and must match
	// the address peers dial, or route TLS handshakes fail with a
	// hostname verification error.
	SANHost string
}

// buildClusterTLSConfig mints an ephemeral ECDSA P-256 leaf certificate
// signed by the configured CA and returns a *tls.Config suitable for
// natsserver.ClusterOpts.TLSConfig. The same config serves both route
// roles: the NATS server uses it when accepting routes and clones it
// (setting ServerName from the dialed route URL) when soliciting them.
func buildClusterTLSConfig(opts ClusterTLSOptions) (*tls.Config, error) {
	if opts.CACert == nil {
		return nil, xerrors.New("cluster TLS: CA certificate is required")
	}
	if opts.CAKey == nil {
		return nil, xerrors.New("cluster TLS: CA private key is required")
	}
	ip := net.ParseIP(opts.SANHost)
	if ip == nil {
		return nil, xerrors.Errorf("cluster TLS: SAN host %q is not an IP address; NATS cluster TLS requires an IP-based relay URL", opts.SANHost)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, xerrors.Errorf("generate leaf key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, xerrors.Errorf("generate leaf serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: opts.SANHost,
		},
		IPAddresses: []net.IP{ip},
		NotBefore:   now,
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
		return nil, xerrors.Errorf("sign leaf certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, xerrors.Errorf("parse leaf certificate: %w", err)
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
		MinVersion: tls.VersionTLS12,
	}, nil
}
