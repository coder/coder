package aibridged

import (
	"net/http"
	"strings"
)

// extractAuthToken extracts authorization token from HTTP request using multiple sources.
// These sources represent the different ways clients authenticate against AI providers.
// It checks the Authorization header (Bearer token) and X-Api-Key header.
// If neither are present, an empty string is returned.
func extractAuthToken(r *http.Request) string {
	// 1. Check Authorization header for Bearer token.
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		segs := strings.Split(authHeader, " ")
		if len(segs) > 1 {
			if strings.ToLower(segs[0]) == "bearer" {
				return strings.Join(segs[1:], "")
			}
		}
	}

	// 2. Check X-Api-Key header.
	apiKeyHeader := r.Header.Get("X-Api-Key")
	if apiKeyHeader != "" {
		return apiKeyHeader
	}

	return ""
}
