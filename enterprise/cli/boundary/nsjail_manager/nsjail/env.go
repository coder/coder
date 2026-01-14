//go:build linux

package nsjail

import (
	"os"

	"github.com/coder/coder/v2/enterprise/cli/boundary/util"
)

// Returns environment variables intended to be set on the child process,
// so they can later be inherited by the target process.
func getEnvsForTargetProcess(configDir string, caCertPath string) []string {
	e := os.Environ()

	e = util.MergeEnvs(e, map[string]string{
		// Set standard CA certificate environment variables for common tools
		// This makes tools like curl, git, etc. trust our dynamically generated CA
		"SSL_CERT_FILE":       caCertPath, // OpenSSL/LibreSSL-based tools
		"SSL_CERT_DIR":        configDir,  // OpenSSL certificate directory
		"CURL_CA_BUNDLE":      caCertPath, // curl
		"GIT_SSL_CAINFO":      caCertPath, // Git
		"REQUESTS_CA_BUNDLE":  caCertPath, // Python requests
		"NODE_EXTRA_CA_CERTS": caCertPath, // Node.js
	})

	return e
}
