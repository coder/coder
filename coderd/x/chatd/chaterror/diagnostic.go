package chaterror

import (
	"errors"
	"net/url"
	"strings"
)

// FormatDiagnosticDetail returns a bounded, single-line diagnostic string from
// err, suitable for surfacing to a user.
func FormatDiagnosticDetail(err error) string {
	if err == nil {
		return ""
	}
	return fallbackDiagnosticDetail("", err.Error(), err)
}

func fallbackDiagnosticDetail(structured, message string, err error) string {
	if strings.TrimSpace(structured) != "" {
		return structured
	}
	detail := strings.TrimSpace(message)
	if detail == "" {
		return ""
	}
	detail = redactDiagnosticURLError(detail, err)
	return normalizeClassificationDetail(strings.Join(strings.Fields(detail), " "))
}

func redactDiagnosticURLError(message string, err error) string {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) || urlErr == nil || urlErr.URL == "" {
		return message
	}
	redactedURL := redactDiagnosticURL(urlErr.URL)
	if redactedURL == urlErr.URL {
		return message
	}
	return strings.ReplaceAll(message, urlErr.URL, redactedURL)
}

func redactDiagnosticURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[REDACTED_URL]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}
