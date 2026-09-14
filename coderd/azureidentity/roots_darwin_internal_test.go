//go:build darwin

package azureidentity

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmbeddedRoots ensures the package's embedded root certificates parse
// successfully. The roots are used by Validate to avoid falling back to the
// platform's system verifier (notably Apple's Security framework on macOS),
// which previously caused TestValidate/regular to fail on macOS with
// `x509: "metadata.azure.com" certificate is not standards compliant`.
// See https://github.com/coder/coder/issues/12978.
func TestEmbeddedRoots(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, embeddedRoots, "embedded roots must not be empty")
	seen := map[string]bool{}
	for _, pemCert := range embeddedRoots {
		block, rest := pem.Decode([]byte(pemCert))
		require.NotNil(t, block, "PEM block should decode")
		require.Zero(t, len(rest), "no trailing data after PEM block")
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		// Each root must be self-signed (issuer == subject).
		require.Equal(t, cert.Issuer.String(), cert.Subject.String(),
			"root certificate must be self-signed: %s", cert.Subject.CommonName)
		require.False(t, seen[cert.Subject.CommonName],
			"duplicate embedded root: %s", cert.Subject.CommonName)
		seen[cert.Subject.CommonName] = true
	}
	// Verify the three roots Azure instance-identity chains ultimately
	// terminate at are all present.
	for _, name := range []string{
		"DigiCert Global Root G2",
		"DigiCert Global Root G3",
		"Baltimore CyberTrust Root",
	} {
		require.True(t, seen[name], "missing embedded root %q", name)
	}
}
