package templatebuilder

import "strings"

// networkErrorPatterns are substrings found in provisioner job output when
// the Terraform registry or provider endpoints are unreachable.
var networkErrorPatterns = []string{
	"no such host",
	"connection refused",
	"i/o timeout",
	"dial tcp: lookup",
	"network is unreachable",
	"no route to host",
	"TLS handshake timeout",
}

// ClassifyProvisionerError inspects a provisioner job error and its log
// lines, returning a user-friendly message for known failure modes.
// If the error is not recognized, the raw jobError is returned unchanged.
func ClassifyProvisionerError(jobError string, logs []string) string {
	combined := jobError + "\n" + strings.Join(logs, "\n")

	for _, pattern := range networkErrorPatterns {
		if strings.Contains(combined, pattern) {
			return "The Terraform registry is unreachable from your provisioner. " +
				"Check network configuration and ensure registry.terraform.io " +
				"is accessible."
		}
	}

	return jobError
}
