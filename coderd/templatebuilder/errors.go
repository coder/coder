package templatebuilder

import "strings"

// networkErrorPatterns are substrings found in provisioner job output when
// the Terraform registry or provider endpoints are unreachable. These are
// intentionally not augmented with diagnostic context because the canned
// message is self-explanatory and the underlying logs rarely add value.
var networkErrorPatterns = []string{
	"no such host",
	"connection refused",
	"i/o timeout",
	"dial tcp: lookup",
	"network is unreachable",
	"no route to host",
	"tls handshake timeout",
}

// authErrorPatterns are substrings that indicate cloud provider
// authentication or credential failures. Matching is case-insensitive
// (the combined text is lowercased before comparison).
var authErrorPatterns = []string{
	"no valid credential sources found",
	"could not find default credentials",
	"unauthorizedaccess",
	"authorizationfailed",
	"authfailure",
	"accessdenied",
	"invalidclienttokenid",
	"expiredtoken",
	"signaturedoesnotmatch",
	"not authorized to perform",
}

// maxLogContextLines caps how many relevant log lines are included in
// the error detail to avoid returning excessive output.
const maxLogContextLines = 20

// ProvisionerErrorCategory names the failure mode of a provisioner import
// job. It carries no error text, so it can be reported as telemetry to
// explain why a build failed without leaking deployment detail.
type ProvisionerErrorCategory string

const (
	// ProvisionerErrorNetwork means the Terraform registry or a provider
	// endpoint was unreachable from the provisioner.
	ProvisionerErrorNetwork ProvisionerErrorCategory = "network"
	// ProvisionerErrorAuth means the provisioner's cloud credentials were
	// missing, expired, or insufficient.
	ProvisionerErrorAuth ProvisionerErrorCategory = "auth"
	// ProvisionerErrorUnknown means the failure matched no known pattern.
	ProvisionerErrorUnknown ProvisionerErrorCategory = "unknown"
)

// ClassifyProvisionerErrorCategory inspects a provisioner job error and its
// log lines and reports which known failure mode they match. Network
// patterns are checked first: a credential lookup that fails because the
// metadata endpoint is unreachable is a network problem.
func ClassifyProvisionerErrorCategory(jobError string, logs []string) ProvisionerErrorCategory {
	combined := strings.ToLower(jobError + "\n" + strings.Join(logs, "\n"))

	for _, pattern := range networkErrorPatterns {
		if strings.Contains(combined, pattern) {
			return ProvisionerErrorNetwork
		}
	}

	for _, pattern := range authErrorPatterns {
		if strings.Contains(combined, pattern) {
			return ProvisionerErrorAuth
		}
	}

	return ProvisionerErrorUnknown
}

// ClassifyProvisionerError inspects a provisioner job error and its log
// lines, returning a user-friendly message for known failure modes.
// For unrecognized errors, the raw jobError is returned with relevant
// log context appended so the user can diagnose the failure. The message
// is derived from ClassifyProvisionerErrorCategory so that what a user
// reads and what telemetry records cannot disagree.
func ClassifyProvisionerError(jobError string, logs []string) string {
	switch ClassifyProvisionerErrorCategory(jobError, logs) {
	case ProvisionerErrorNetwork:
		return "The Terraform registry is unreachable from your provisioner. " +
			"Check network configuration and ensure registry.terraform.io " +
			"is accessible."
	case ProvisionerErrorAuth:
		msg := "Cloud provider authentication failed. " +
			"Check that valid credentials are configured for the provisioner."
		if diag := extractDiagnostics(logs); diag != "" {
			msg += "\n\n" + diag
		}
		return msg
	default:
		// For unrecognized errors, include relevant Terraform output so the
		// user can diagnose the failure.
		if diag := extractDiagnostics(logs); diag != "" {
			return jobError + "\n\n" + diag
		}
		return jobError
	}
}

// diagnosticPrefixes identify Terraform diagnostic output lines worth
// surfacing. Terraform formats errors as blocks starting with "Error:"
// or "Warning:" followed by indented detail lines.
var diagnosticPrefixes = []string{
	"Error:",
	"Warning:",
	"error:",
	"warning:",
}

// isDiagnosticStart returns true if the line begins a Terraform
// diagnostic block.
func isDiagnosticStart(line string) bool {
	for _, prefix := range diagnosticPrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// extractDiagnostics pulls Terraform diagnostic blocks from provisioner
// log output. It keeps lines that start a diagnostic (Error:/Warning:)
// and subsequent indented or context lines, up to maxLogContextLines.
func extractDiagnostics(logs []string) string {
	var lines []string
	inBlock := false

	for _, line := range logs {
		if len(lines) >= maxLogContextLines {
			lines = append(lines, "... (truncated)")
			break
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inBlock {
				// Blank line inside a block: keep it to preserve
				// readability, but end the block.
				lines = append(lines, "")
				inBlock = false
			}
			continue
		}

		if isDiagnosticStart(trimmed) {
			inBlock = true
			lines = append(lines, trimmed)
			continue
		}

		if inBlock {
			lines = append(lines, trimmed)
			continue
		}

		// Capture "on <file> line <N>" references that appear
		// outside the main diagnostic block in some Terraform versions.
		if strings.HasPrefix(trimmed, "on ") && strings.Contains(trimmed, " line ") {
			lines = append(lines, trimmed)
		}
	}

	return strings.Join(lines, "\n")
}
