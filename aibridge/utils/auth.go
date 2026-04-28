package utils

import "strings"

// ExtractBearerToken extracts the token from a "Bearer <token>" authorization header.
func ExtractBearerToken(auth string) string {
	if auth := strings.TrimSpace(auth); auth != "" {
		fields := strings.Fields(auth)
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			return fields[1]
		}
	}
	return ""
}
