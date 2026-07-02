package cryptokeys

import (
	"context"
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

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

const (
	caCertPEMBlockType = "CERTIFICATE"
	caKeyPEMBlockType  = "EC PRIVATE KEY"

	// clockSkewTolerance backdates the CA certificate's NotBefore and extends
	// its NotAfter so that replicas with mildly skewed clocks still accept it.
	clockSkewTolerance = time.Hour
)

// NATSCA is the parsed state of the nats_ca crypto key feature at one point in
// time. The CA signs the ephemeral leaf certificates that replicas use for
// NATS cluster mTLS.
//
// Callers that need to react to CA rotation (re-minting leaves and reloading
// the NATS server config) should poll FetchNATSCA and compare Sequence to
// detect when the active CA has changed.
type NATSCA struct {
	// Sequence is the crypto_keys sequence of the active row.
	Sequence int32
	// Cert is the active CA certificate used to sign leaf certificates.
	Cert *x509.Certificate
	// Key is the active CA private key.
	Key crypto.Signer
	// TrustBundle contains the certificates of all currently valid CA rows,
	// including Cert. During a rotation overlap window it has two entries;
	// installing the full bundle as the trust root lets replicas on either
	// side of a rotation verify each other.
	TrustBundle []*x509.Certificate
}

// ErrNATSCANotFound is returned by FetchNATSCA when no active nats_ca CA row
// exists yet. On a fresh deployment this resolves once the key rotator mints
// the initial CA, so callers should treat it as a transient condition and
// retry rather than a permanent failure.
var ErrNATSCANotFound = xerrors.New("no active NATS CA found")

// FetchNATSCA returns the current NATS cluster CA. It is read-only: the key
// rotator is the sole creator of nats_ca rows, so this never inserts. Before
// the rotator has minted the initial CA (or during a transient gap) it returns
// ErrNATSCANotFound; callers should retry.
func FetchNATSCA(ctx context.Context, logger slog.Logger, db database.Store) (*NATSCA, error) {
	//nolint:gocritic // The CA accessor requires the same crypto key read access as the cache.
	ctx = dbauthz.AsKeyReader(ctx)

	now := dbtime.Now()

	keys, err := db.GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureNATSCA)
	if err != nil {
		return nil, xerrors.Errorf("get crypto keys by feature: %w", err)
	}

	ca, ok, err := parseNATSCAKeys(ctx, logger, keys, now)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNATSCANotFound
	}
	return ca, nil
}

// parseNATSCAKeys builds a NATSCA from the database rows for the nats_ca
// feature. Rows must be ordered by sequence descending (the order returned by
// GetCryptoKeysByFeature). The active CA is the newest row that is usable for
// signing; the trust bundle contains the certificates of every row that is
// still valid for verification. The boolean reports whether a row could act
// as the active CA.
//
// A corrupt secret on the active (newest signable) key is fatal: there is no
// usable CA to sign leaves with. A corrupt secret on a non-active verify-only
// key is logged and skipped: it has already lost its usefulness as a trust
// root, and failing the whole fetch over it would needlessly take down the
// NATS mTLS subsystem.
func parseNATSCAKeys(ctx context.Context, logger slog.Logger, keys []database.CryptoKey, now time.Time) (*NATSCA, bool, error) {
	ca := &NATSCA{}
	for _, key := range keys {
		if !key.CanVerify(now) {
			continue
		}
		// The newest signable key we have not yet claimed becomes the active
		// CA. Its secret must parse; everything else is a best-effort trust
		// root.
		isActiveCandidate := ca.Cert == nil && key.CanSign(now)
		cert, signer, err := parseCASecret(key.Secret.String)
		if err != nil {
			if isActiveCandidate {
				return nil, false, xerrors.Errorf("parse active CA secret for sequence %d: %w", key.Sequence, err)
			}
			logger.Warn(ctx, "skipping corrupt non-active NATS CA key",
				slog.F("sequence", key.Sequence), slog.Error(err))
			continue
		}
		ca.TrustBundle = append(ca.TrustBundle, cert)
		if isActiveCandidate {
			ca.Sequence = key.Sequence
			ca.Cert = cert
			ca.Key = signer
		}
	}
	if ca.Cert == nil {
		return nil, false, nil
	}
	return ca, true, nil
}

// generateCASecret generates a new self-signed CA certificate and private key
// for signing NATS cluster leaf certificates, PEM-encoded into a single
// bundle for storage in the crypto_keys secret column.
//
// anchorTime is the key row's starts_at (which may be in the future for a
// rotated-in key). keyDuration is the rotator's key duration: the row stays the
// active signer for that long. The certificate must outlive that window plus
// the longest leaf it could sign (NATSCALeafValidity) plus clock-skew slack, so
// leaves minted just before rotation still chain to a valid CA.
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
		NotAfter:              anchorTime.Add(keyDuration + NATSCALeafValidity + clockSkewTolerance),
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
