package agentsdk

import "strings"

// SanitizeLogOutput replaces invalid UTF-8 and NUL characters in log output.
// Invalid UTF-8 cannot be transported in protobuf string fields, and PostgreSQL
// rejects NUL bytes in text columns.
func SanitizeLogOutput(s string) string {
	s = strings.ToValidUTF8(s, "❌")
	return strings.ReplaceAll(s, "\x00", "❌")
}
