package cryptokeys

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"golang.org/x/xerrors"
)

const (
	caCertPEMBlockType = "CERTIFICATE"
	caKeyPEMBlockType  = "EC PRIVATE KEY"

	// clockSkewTolerance backdates the CA certificate's NotBefore and extends
	// its NotAfter so that replicas with mildly skewed clocks still accept it.
	clockSkewTolerance = time.Hour
)

// NATSCA is the decoded form of a single nats_ca crypto key row, produced by
// the generic crypto key cache (see idSecret). The CA signs the ephemeral leaf
// certificates that replicas use for NATS cluster mTLS.
//
// The active CA is served by a SigningKeycache.SigningKey call for the nats_ca
// feature; a specific historical CA (for verifying a peer leaf minted under an
// earlier CA during a rotation overlap) is served by VerifyingKey with that
// row's sequence.
type NATSCA struct {
	// Sequence is the crypto_keys sequence of the row this CA came from.
	Sequence int32
	// Cert is the CA certificate used to sign or verify leaf certificates.
	Cert *x509.Certificate
	// Key is the CA private key, used to sign leaves.
	Key crypto.Signer
}

// generateCASecret generates a new self-signed CA certificate and private key
// for signing NATS cluster leaf certificates, PEM-encoded into a single
// bundle for storage in the crypto_keys secret column.
//
// anchorTime is the key row's starts_at (which may be in the future for a
// rotated-in key). keyDuration is the rotator's key duration: the row stays the
// active signer for that long. The certificate stays valid for NATSCAOverlap
// past that window so that, once the next CA becomes the active signer, this CA
// is still valid while replicas' key caches refresh onto the new one. Leaves
// are separately clamped to expire before this NotAfter (see coderd/x/nats
// mintLeaf), so the overlap only needs to cover the cache-refresh transition.
func generateCASecret(anchorTime time.Time, keyDuration time.Duration) (string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", xerrors.Errorf("generate key: %w", err)
	}

	// 128-bit random serial per CA/Browser Forum conventions.
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", xerrors.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "coder-nats-ca",
		},
		NotBefore:             anchorTime.Add(-clockSkewTolerance),
		NotAfter:              anchorTime.Add(keyDuration + NATSCAOverlap),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return "", xerrors.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", xerrors.Errorf("marshal private key: %w", err)
	}

	var secret []byte
	secret = append(secret, pem.EncodeToMemory(&pem.Block{Type: caCertPEMBlockType, Bytes: der})...)
	secret = append(secret, pem.EncodeToMemory(&pem.Block{Type: caKeyPEMBlockType, Bytes: keyDER})...)
	return string(secret), nil
}

// parseCASecret parses a PEM bundle produced by generateCASecret back into
// the CA certificate and private key.
func parseCASecret(secret string) (*x509.Certificate, crypto.Signer, error) {
	var (
		cert *x509.Certificate
		key  *ecdsa.PrivateKey
	)
	rest := []byte(secret)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case caCertPEMBlockType:
			if cert != nil {
				return nil, nil, xerrors.New("multiple certificates in CA secret")
			}
			var err error
			cert, err = x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, nil, xerrors.Errorf("parse certificate: %w", err)
			}
		case caKeyPEMBlockType:
			if key != nil {
				return nil, nil, xerrors.New("multiple private keys in CA secret")
			}
			var err error
			key, err = x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				return nil, nil, xerrors.Errorf("parse private key: %w", err)
			}
		default:
			return nil, nil, xerrors.Errorf("unexpected PEM block type: %q", block.Type)
		}
	}
	if cert == nil {
		return nil, nil, xerrors.New("no certificate in CA secret")
	}
	if key == nil {
		return nil, nil, xerrors.New("no private key in CA secret")
	}
	if !key.PublicKey.Equal(cert.PublicKey) {
		return nil, nil, xerrors.New("private key does not match certificate")
	}
	// Reject a structurally valid bundle whose certificate cannot act as a
	// signing CA. Without this, a corrupted secret could yield a non-CA cert
	// that silently becomes the active signer; leaves signed under it would
	// then fail x509 verification on every replica.
	if !cert.IsCA || !cert.BasicConstraintsValid || cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, nil, xerrors.New("certificate is not a valid signing CA")
	}
	return cert, key, nil
}
