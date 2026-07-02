package cryptokeys

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestCASecretRoundTrip(t *testing.T) {
	t.Parallel()

	// The certificate's NotAfter must track the supplied keyDuration, not a
	// hardcoded default, so a CA stays valid for as long as it can be the
	// active signer plus the longest leaf it signs.
	for _, keyDuration := range []time.Duration{DefaultKeyDuration, DefaultKeyDuration * 3, time.Hour} {
		now := time.Now().UTC().Truncate(time.Second)
		secret, err := generateCASecret(now, keyDuration)
		require.NoError(t, err)

		cert, signer, err := parseCASecret(secret)
		require.NoError(t, err)

		require.True(t, cert.IsCA)
		require.True(t, cert.BasicConstraintsValid)
		require.True(t, cert.MaxPathLenZero)
		require.Equal(t, x509.KeyUsageCertSign, cert.KeyUsage)
		require.Equal(t, now.Add(-clockSkewTolerance), cert.NotBefore)
		require.Equal(t, now.Add(keyDuration+NATSCALeafValidity+clockSkewTolerance), cert.NotAfter)
		require.Equal(t, cert.PublicKey, signer.Public())

		// The cert must outlive its active-signer window so leaves signed at
		// the end of that window still chain to a valid CA.
		require.True(t, cert.NotAfter.After(now.Add(keyDuration)),
			"cert must remain valid past the end of its active-signer window")

		// The cert must be able to verify itself as a trust root.
		pool := x509.NewCertPool()
		pool.AddCert(cert)
		_, err = cert.Verify(x509.VerifyOptions{Roots: pool})
		require.NoError(t, err)
	}
}

func TestParseCASecretErrors(t *testing.T) {
	t.Parallel()

	now := time.Now()
	secretA, err := generateCASecret(now, DefaultKeyDuration)
	require.NoError(t, err)
	secretB, err := generateCASecret(now, DefaultKeyDuration)
	require.NoError(t, err)

	certA, keyA := splitCAPEM(t, secretA)
	_, keyB := splitCAPEM(t, secretB)

	nonCACert, nonCAKey := generateNonCAPEM(t, now)

	cases := []struct {
		name    string
		secret  string
		errText string
	}{
		{"Empty", "", "no certificate"},
		{"NotPEM", "not pem at all", "no certificate"},
		{"CertOnly", string(certA), "no private key"},
		{"KeyCertMismatch", string(certA) + string(keyB), "does not match certificate"},
		{"MultipleCertificates", string(certA) + string(certA) + string(keyA), "multiple certificates"},
		{"MultiplePrivateKeys", string(certA) + string(keyA) + string(keyA), "multiple private keys"},
		{"UnexpectedBlockType", string(pemBlock("RSA PRIVATE KEY", []byte("x"))), "unexpected PEM block type"},
		{"BadCertificateBytes", string(pemBlock(caCertPEMBlockType, []byte("garbage"))), "parse certificate"},
		{"BadPrivateKeyBytes", string(certA) + string(pemBlock(caKeyPEMBlockType, []byte("garbage"))), "parse private key"},
		{"NotASigningCA", string(nonCACert) + string(nonCAKey), "not a valid signing CA"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := parseCASecret(tc.secret)
			require.ErrorContains(t, err, tc.errText)
		})
	}
}

// splitCAPEM splits a CA secret bundle into its certificate and private key
// PEM blocks so tests can recombine them into malformed bundles.
func splitCAPEM(t *testing.T, secret string) (certPEM, keyPEM []byte) {
	t.Helper()
	rest := []byte(secret)
	for {
		block, r := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = r
		switch block.Type {
		case caCertPEMBlockType:
			certPEM = pem.EncodeToMemory(block)
		case caKeyPEMBlockType:
			keyPEM = pem.EncodeToMemory(block)
		}
	}
	require.NotNil(t, certPEM)
	require.NotNil(t, keyPEM)
	return certPEM, keyPEM
}

func pemBlock(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

// generateNonCAPEM produces a structurally valid cert+key bundle whose
// certificate is not a CA (no IsCA, no KeyUsageCertSign). The key matches the
// cert, so it passes every parseCASecret check except the signing-CA check.
func generateNonCAPEM(t *testing.T, now time.Time) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "not-a-ca"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return pemBlock(caCertPEMBlockType, der), pemBlock(caKeyPEMBlockType, keyDER)
}

func TestFetchNATSCA(t *testing.T) {
	t.Parallel()

	t.Run("NotFoundWhenMissing", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// FetchNATSCA never creates: with no nats_ca rows it reports the
		// transient not-found condition the rotator resolves.
		_, err := FetchNATSCA(ctx, testutil.Logger(t), db)
		require.ErrorIs(t, err, ErrNATSCANotFound)

		keys, err := db.GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureNATSCA)
		require.NoError(t, err)
		require.Empty(t, keys)
	})

	t.Run("ReturnsActive", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		now := time.Now().UTC()

		current := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-time.Hour),
		})

		ca, err := FetchNATSCA(ctx, testutil.Logger(t), db)
		require.NoError(t, err)
		require.Equal(t, current.Sequence, ca.Sequence)
		require.NotNil(t, ca.Cert)
		require.NotNil(t, ca.Key)
		require.Len(t, ca.TrustBundle, 1)

		currentCert, _, err := parseCASecret(current.Secret.String)
		require.NoError(t, err)
		require.Equal(t, currentCert.Raw, ca.Cert.Raw)
	})

	t.Run("SkipsCorruptNonActiveKey", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		now := time.Now().UTC()

		// Newest signable key is the active CA.
		active := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 2,
			StartsAt: now.Add(-time.Hour),
		})
		// Older still-verifiable key with a corrupt secret: it must be skipped
		// rather than failing the whole fetch.
		dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-2 * time.Hour),
			Secret:   sql.NullString{String: "not a pem", Valid: true},
		})

		ca, err := FetchNATSCA(ctx, testutil.Logger(t), db)
		require.NoError(t, err)
		require.Equal(t, active.Sequence, ca.Sequence)

		// The corrupt key is excluded from the trust bundle.
		activeCert, _, err := parseCASecret(active.Secret.String)
		require.NoError(t, err)
		require.Len(t, ca.TrustBundle, 1)
		require.Equal(t, activeCert.Raw, ca.TrustBundle[0].Raw)
	})

	t.Run("CorruptActiveKeyIsFatal", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		now := time.Now().UTC()

		// The active (newest signable) key is corrupt: there is no usable CA to
		// sign leaves with, so the fetch must fail loudly.
		dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-time.Hour),
			Secret:   sql.NullString{String: "not a pem", Valid: true},
		})

		_, err := FetchNATSCA(ctx, testutil.Logger(t), db)
		require.Error(t, err)
		require.Contains(t, err.Error(), "active CA secret for sequence 1")
	})

	t.Run("RotationOverlap", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		now := time.Now().UTC()

		// Old CA scheduled for deletion in the future: still a trust root,
		// no longer the active signer.
		oldKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:   database.CryptoKeyFeatureNATSCA,
			Sequence:  1,
			StartsAt:  now.Add(-2 * time.Hour),
			DeletesAt: sql.NullTime{Time: now.Add(time.Hour), Valid: true},
		})
		newKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 2,
			StartsAt: now.Add(-time.Hour),
		})
		// Deleted key: excluded entirely.
		deletedKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:   database.CryptoKeyFeatureNATSCA,
			Sequence:  3,
			StartsAt:  now.Add(-3 * time.Hour),
			DeletesAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
		})

		ca, err := FetchNATSCA(ctx, testutil.Logger(t), db)
		require.NoError(t, err)
		require.Equal(t, newKey.Sequence, ca.Sequence)

		newCert, _, err := parseCASecret(newKey.Secret.String)
		require.NoError(t, err)
		oldCert, _, err := parseCASecret(oldKey.Secret.String)
		require.NoError(t, err)
		deletedCert, _, err := parseCASecret(deletedKey.Secret.String)
		require.NoError(t, err)

		require.Equal(t, newCert.Raw, ca.Cert.Raw)
		require.Len(t, ca.TrustBundle, 2)
		bundle := [][]byte{ca.TrustBundle[0].Raw, ca.TrustBundle[1].Raw}
		require.Contains(t, bundle, newCert.Raw)
		require.Contains(t, bundle, oldCert.Raw)
		require.NotContains(t, bundle, deletedCert.Raw)
	})

	t.Run("FutureKeyNotActive", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		now := time.Now().UTC()

		current := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-time.Hour),
		})
		// A rotated-in key that hasn't started yet must not be the active
		// signer, but its cert belongs in the trust bundle.
		future := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 2,
			StartsAt: now.Add(time.Hour),
		})

		ca, err := FetchNATSCA(ctx, testutil.Logger(t), db)
		require.NoError(t, err)
		require.Equal(t, current.Sequence, ca.Sequence)

		currentCert, _, err := parseCASecret(current.Secret.String)
		require.NoError(t, err)
		futureCert, _, err := parseCASecret(future.Secret.String)
		require.NoError(t, err)

		require.Equal(t, currentCert.Raw, ca.Cert.Raw)
		require.Len(t, ca.TrustBundle, 2)
		bundle := [][]byte{ca.TrustBundle[0].Raw, ca.TrustBundle[1].Raw}
		require.Contains(t, bundle, currentCert.Raw)
		require.Contains(t, bundle, futureCert.Raw)
		require.NotEqual(t, currentCert.Raw, futureCert.Raw)
	})
}
