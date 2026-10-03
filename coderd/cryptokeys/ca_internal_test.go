package cryptokeys

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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
		require.Equal(t, now.Add(keyDuration+NATSCAOverlap), cert.NotAfter)
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

// TestNATSCASigningCache exercises the nats_ca feature through the generic
// signing key cache: the PEM secret decodes into a *NATSCA, SigningKey serves
// the active CA, VerifyingKey serves a specific CA by sequence, and a rotation
// is picked up on the next refresh.
func TestNATSCASigningCache(t *testing.T) {
	t.Parallel()

	t.Run("ActiveAndVerifyingByID", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		now := time.Now().UTC()

		current := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-time.Hour),
		})

		cache, err := NewSigningCache(ctx, testutil.Logger(t), &DBFetcher{DB: db}, codersdk.CryptoKeyFeatureNATSCA)
		require.NoError(t, err)
		defer cache.Close()

		id, key, err := cache.SigningKey(ctx)
		require.NoError(t, err)

		ca, ok := key.(*NATSCA)
		require.True(t, ok, "signing key should decode to *NATSCA, got %T", key)
		require.Equal(t, current.Sequence, ca.Sequence)
		require.NotNil(t, ca.Cert)
		require.NotNil(t, ca.Key)

		currentCert, _, err := parseCASecret(current.Secret.String)
		require.NoError(t, err)
		require.Equal(t, currentCert.Raw, ca.Cert.Raw)

		// VerifyingKey looks the CA up by the sequence embedded in id, which is
		// how a peer leaf minted under this CA is verified.
		verifying, err := cache.VerifyingKey(ctx, id)
		require.NoError(t, err)
		vca, ok := verifying.(*NATSCA)
		require.True(t, ok, "verifying key should decode to *NATSCA, got %T", verifying)
		require.Equal(t, currentCert.Raw, vca.Cert.Raw)
	})

	t.Run("RefreshesOnRotation", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		now := dbtime.Now()
		clock.Set(now)

		first := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-time.Hour),
		})

		cache, err := NewSigningCache(ctx, testutil.Logger(t), &DBFetcher{DB: db}, codersdk.CryptoKeyFeatureNATSCA, WithCacheClock(clock))
		require.NoError(t, err)
		defer cache.Close()

		_, key, err := cache.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, first.Sequence, key.(*NATSCA).Sequence)

		// Simulate a rotation by inserting a higher-sequence active CA. The old
		// CA stays valid for verification by its sequence.
		second := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 2,
			StartsAt: now.Add(-time.Minute),
		})

		// Fire the background refresher; the active CA advances to the new row.
		clock.Advance(refreshInterval).MustWait(ctx)

		_, key, err = cache.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, second.Sequence, key.(*NATSCA).Sequence)

		oldVerifying, err := cache.VerifyingKey(ctx, "1")
		require.NoError(t, err)
		require.Equal(t, first.Sequence, oldVerifying.(*NATSCA).Sequence)
	})
}

func TestNoopSigningKeycache(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	var cache SigningKeycache = NoopSigningKeycache{}

	_, _, err := cache.SigningKey(ctx)
	require.ErrorIs(t, err, ErrKeyNotFound)

	_, err = cache.VerifyingKey(ctx, "1")
	require.ErrorIs(t, err, ErrKeyNotFound)

	require.NoError(t, cache.Close())
}
