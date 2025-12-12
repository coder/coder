// Package aibridge provides utilities for the AI Bridge feature.
package aibridge

import (
	"net/http"
	"strings"
)

// ExtractAuthToken extracts an authorization token from HTTP headers.
// It checks the Authorization header (Bearer token) and X-Api-Key header,
// which represent the different ways clients authenticate against AI providers.
// If neither are present, an empty string is returned.
func ExtractAuthToken(header http.Header) string {
	if auth := strings.TrimSpace(header.Get("Authorization")); auth != "" {
		fields := strings.Fields(auth)
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			return fields[1]
		}
	}
	if apiKey := strings.TrimSpace(header.Get("X-Api-Key")); apiKey != "" {
		return apiKey
	}
	return ""
}
