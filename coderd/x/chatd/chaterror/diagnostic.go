package chaterror

import "strings"

// FormatDiagnosticDetail returns a bounded, single-line diagnostic string from
// err, suitable for surfacing to a user.
func FormatDiagnosticDetail(err error) string {
	if err == nil {
		return ""
	}
	return fallbackDiagnosticDetail("", err.Error())
}

func fallbackDiagnosticDetail(structured, message string) string {
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
