package chaterror

import "strings"

// FormatDiagnosticDetail returns a bounded, single-line diagnostic string from
// err, suitable for surfacing to a user.
func FormatDiagnosticDetail(err error) string {
	if err == nil {
		return ""
	}
	return resolveDiagnosticDetail("", err.Error())
}

// resolveDiagnosticDetail picks the detail string to surface: structured
// provider detail always wins, otherwise the raw message is used as a fallback
// after dropping any URL-bearing text and bounding its length.
func resolveDiagnosticDetail(structured, message string) string {
	if strings.TrimSpace(structured) != "" {
		return structured
	}
	detail := strings.TrimSpace(message)
	if detail == "" {
		return ""
	}
	if strings.Contains(detail, "://") {
		return ""
	}
	return normalizeClassificationDetail(strings.Join(strings.Fields(detail), " "))
}
