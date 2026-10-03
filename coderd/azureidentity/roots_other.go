//go:build !darwin

package azureidentity

import "crypto/x509"

// rootCertPool returns the system cert pool on non-Apple platforms.
func rootCertPool() (*x509.CertPool, error) {
	return x509.SystemCertPool()
}
