package chaterror

import "strings"

// FormatDiagnosticDetail returns a bounded, single-line diagnostic string from
// err.
func FormatDiagnosticDetail(err error) string {
	if err == nil {
		return ""
	}
	return formatDiagnosticDetailString(err.Error())
}

func fallbackDiagnosticDetail(structuredDetail string, message string) string {
	if strings.TrimSpace(structuredDetail) != "" {
		return structuredDetail
	}
	return formatDiagnosticDetailString(message)
}

func formatDiagnosticDetailString(message string) string {
	detail := strings.TrimSpace(message)
	if detail == "" {
		return ""
	}
	return normalizeClassificationDetail(strings.Join(strings.Fields(detail), " "))
}
