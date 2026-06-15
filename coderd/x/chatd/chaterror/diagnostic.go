package chaterror

import (
	"errors"
	"net/url"
	"strings"
)

// FormatDiagnosticDetail returns a bounded, single-line diagnostic string from
// err, suitable for surfacing to a user.
func FormatDiagnosticDetail(err error) string {
	return resolveDiagnosticDetail("", err)
}

// resolveDiagnosticDetail picks the detail string to surface: structured
// provider detail always wins, otherwise the raw error string is used as a
// fallback after redacting any URL preserved in a typed *url.Error and bounding
// its length.
func resolveDiagnosticDetail(structured string, err error) string {
	if strings.TrimSpace(structured) != "" {
		return structured
	}
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return ""
	}
	detail = redactTypedTransportURL(detail, err)
	return normalizeClassificationDetail(strings.Join(strings.Fields(detail), " "))
}

func redactTypedTransportURL(message string, err error) string {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || urlErr == nil || urlErr.URL == "" {
		return message
	}
	redactedURL, changed := redactDiagnosticURL(urlErr.URL)
	if !changed {
		return message
	}
	return strings.ReplaceAll(message, urlErr.URL, redactedURL)
}

func redactDiagnosticURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[REDACTED_URL]", true
	}
	redacted := *parsed
	redacted.User = nil
	redacted.RawQuery = ""
	redacted.ForceQuery = false
	redacted.Fragment = ""
	redactedURL := redacted.String()
	return redactedURL, redactedURL != rawURL
}
