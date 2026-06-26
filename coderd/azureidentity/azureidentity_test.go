package azureidentity_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"runtime"
	"testing"
	"time"

	"github.com/smallstep/pkcs7"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/azureidentity"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	mustTime := func(layout string, value string) time.Time {
		ti, err := time.Parse(layout, value)
		require.NoError(t, err)
		return ti
	}

	for _, tc := range []struct {
		date    time.Time
		name    string
		payload string
		vmID    string
	}{{
		name:    "regular",
		payload: "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",
		vmID:    "dc18de58-292f-4794-9a5d-a4192bad0039",
		// This cert uses intermediates:
		//  1. Microsoft TLS G2 RSA CA OCSP 02 (expires 2029-06-03T20:03:00Z)
		//  2. Microsoft TLS RSA Root G2       (expires 2029-06-19T23:59:59Z)
		// It uses root:
		//  DigiCert Global Root G2            (expires 2038-01-15T12:00:00Z)
		// So this test should be good until 2038 provided that we don't remove the above intermediates, and the
		// root doesn't get removed from OS trust stores (would be very surprising and a huge security deal).
		date: mustTime(time.RFC3339, "2026-06-25T00:00:00Z"),
	}, {
		name:    "govcloud",
		payload: "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",
		vmID:    "990878d4-068a-4ac4-9ee9-1231d2218ef2",
		// This cert uses intermediate:
		//  Microsoft Azure TLS Issuing CA 05  (expires 2024-06-27T23:59:59Z)
		// It uses root:
		//  DigiCert Global Root G2            (expires 2038-01-15T12:00:00Z)
		// So this test should be good until 2038 provided that we don't remove the above intermediates, and the
		// root doesn't get removed from OS trust stores (would be very surprising and a huge security deal).
		date: mustTime(time.RFC3339, "2023-04-01T00:00:00Z"),
	}, {
		name:    "rsa",
		payload: "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",
		vmID:    "960a4b4a-dab2-44ef-9b73-7753043b4f16",
		// This cert uses intermediate:
		//  Microsoft Azure RSA TLS Issuing CA 08  (expires 2026-08-25T23:59:59Z)
		// It uses root:
		//  DigiCert Global Root G2                (expires 2038-01-15T12:00:00Z)
		// So this test should be good until 2038 provided that we don't remove the above intermediates, and the
		// root doesn't get removed from OS trust stores (would be very surprising and a huge security deal).
		date: mustTime(time.RFC3339, "2024-04-22T17:32:44Z"),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vm, err := azureidentity.Validate(context.Background(), tc.payload, azureidentity.Options{
				CurrentTime: tc.date,
				Offline:     true,
			})
			require.NoError(t, err)
			require.Equal(t, tc.vmID, vm)
		})
	}
}

func TestIsAllowedCertificateURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		url     string
		allowed bool
	}{
		{"microsoft http", "http://www.microsoft.com/pki/mscorp/cert.crt", true},
		{"microsoft https", "https://www.microsoft.com/pkiops/certs/cert.crt", true},
		{"digicert http", "http://cacerts.digicert.com/DigiCertGlobalRootG2.crt", true},
		{"digicert https", "https://cacerts.digicert.com/DigiCertGlobalRootG3.crt", true},
		{"evil domain", "http://evil.example.com/cert.crt", false},
		{"metadata endpoint", "http://169.254.169.254/latest/meta-data/", false},
		{"localhost", "http://localhost/secret", false},
		{"subdomain trick", "http://www.microsoft.com.evil.com/cert.crt", false},
		{"empty string", "", false},
		{"ftp scheme", "ftp://www.microsoft.com/cert.crt", false},
		{"no scheme", "www.microsoft.com/cert.crt", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"microsoft with path", "http://www.microsoft.com/pkiops/certs/cert.crt", true},
		{"microsoft explicit port 80", "http://www.microsoft.com:80/cert.crt", true},
		{"microsoft explicit port 443", "https://www.microsoft.com:443/cert.crt", true},
		{"microsoft non-standard port", "http://www.microsoft.com:8080/cert.crt", false},
		{"microsoft port 22", "http://www.microsoft.com:22/cert.crt", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := azureidentity.IsAllowedCertificateURL(tc.url)
			require.Equal(t, tc.allowed, result, "URL: %s", tc.url)
		})
	}
}

// testCertChain holds a three-level certificate hierarchy (Root CA,
// Intermediate CA, Signing/leaf) together with their private keys.
type testCertChain struct {
	RootCert         *x509.Certificate
	RootKey          *rsa.PrivateKey
	IntermediateCert *x509.Certificate
	IntermediateKey  *rsa.PrivateKey
	SigningCert      *x509.Certificate
	SigningKey       *rsa.PrivateKey
}

// newTestCertChain creates a fresh three-level certificate chain for
// testing. All certificates are valid at time.Now().
func newTestCertChain(t *testing.T) testCertChain {
	t.Helper()

	// Smaller key sizes are fine for tests; keeps them fast.
	const keyBits = 2048

	// ---- Root CA ----
	rootKey, err := rsa.GenerateKey(rand.Reader, keyBits)
	require.NoError(t, err)
	rootTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	require.NoError(t, err)
	rootCert, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	// ---- Intermediate CA ----
	intermediateKey, err := rsa.GenerateKey(rand.Reader, keyBits)
	require.NoError(t, err)
	intermediateTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Test Intermediate CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	intermediateDER, err := x509.CreateCertificate(rand.Reader, intermediateTmpl, rootCert, &intermediateKey.PublicKey, rootKey)
	require.NoError(t, err)
	intermediateCert, err := x509.ParseCertificate(intermediateDER)
	require.NoError(t, err)

	// ---- Signing (leaf) certificate ----
	signingKey, err := rsa.GenerateKey(rand.Reader, keyBits)
	require.NoError(t, err)
	signingTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "metadata.azure.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	signingDER, err := x509.CreateCertificate(rand.Reader, signingTmpl, intermediateCert, &signingKey.PublicKey, intermediateKey)
	require.NoError(t, err)
	signingCert, err := x509.ParseCertificate(signingDER)
	require.NoError(t, err)

	return testCertChain{
		RootCert:         rootCert,
		RootKey:          rootKey,
		IntermediateCert: intermediateCert,
		IntermediateKey:  intermediateKey,
		SigningCert:      signingCert,
		SigningKey:       signingKey,
	}
}

// createSignedPKCS7 produces a base64-encoded PKCS7 SignedData
// envelope over content, signed by the chain's leaf certificate.
func (tc *testCertChain) createSignedPKCS7(t *testing.T, content []byte) string {
	t.Helper()

	sd, err := pkcs7.NewSignedData(content)
	require.NoError(t, err)
	err = sd.AddSignerChain(tc.SigningCert, tc.SigningKey, []*x509.Certificate{tc.IntermediateCert}, pkcs7.SignerInfoConfig{})
	require.NoError(t, err)
	der, err := sd.Finish()
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(der)
}

// validationOptions returns azureidentity.Options that trust only this
// chain's Root CA.
func (tc *testCertChain) validationOptions() azureidentity.Options {
	roots := x509.NewCertPool()
	roots.AddCert(tc.RootCert)
	return azureidentity.Options{
		Roots:         roots,
		Intermediates: []*x509.Certificate{tc.IntermediateCert},
		Offline:       true,
	}
}

func TestValidate_TamperedContent(t *testing.T) {
	t.Parallel()

	chain := newTestCertChain(t)

	// Build a valid PKCS7 envelope.
	original := []byte(`{"vmId":"tamper-test-vm"}`)
	signed := chain.createSignedPKCS7(t, original)

	// Decode, tamper with the content, re-encode.
	raw, err := base64.StdEncoding.DecodeString(signed)
	require.NoError(t, err)
	tampered := bytes.Replace(raw, []byte("tamper-test-vm"), []byte("tampered!!!!!!"), 1)
	require.NotEqual(t, raw, tampered, "payload should have changed")
	tamperedB64 := base64.StdEncoding.EncodeToString(tampered)

	opts := chain.validationOptions()
	_, err = azureidentity.Validate(context.Background(), tamperedB64, opts)
	require.Error(t, err, "tampered content must not pass validation")
}

func TestValidate_UntrustedCertWithValidSignature(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("pkcs7 signing uses SHA1 which may be restricted on macOS")
	}

	chain := newTestCertChain(t)

	content := []byte(`{"vmId":"untrusted-test-vm"}`)
	signed := chain.createSignedPKCS7(t, content)

	// Build options that trust a DIFFERENT root, so the chain
	// should not verify.
	otherRoot, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	otherRootTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(99),
		Subject:               pkix.Name{CommonName: "Other Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	otherRootDER, err := x509.CreateCertificate(rand.Reader, otherRootTmpl, otherRootTmpl, &otherRoot.PublicKey, otherRoot)
	require.NoError(t, err)
	otherRootCert, err := x509.ParseCertificate(otherRootDER)
	require.NoError(t, err)

	untrustedRoots := x509.NewCertPool()
	untrustedRoots.AddCert(otherRootCert)
	opts := azureidentity.Options{
		Roots:         untrustedRoots,
		Intermediates: []*x509.Certificate{chain.IntermediateCert},
		Offline:       true,
	}

	_, err = azureidentity.Validate(context.Background(), signed, opts)
	require.Error(t, err, "signature from untrusted CA must not pass validation")
}
