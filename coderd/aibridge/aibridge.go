// Package aibridge provides utilities for the AI Bridge feature.
package aibridge

import (
	"net/http"
	"strings"
)

// HeaderCoderSessionAuth is an internal header used to pass the Coder session
// token from AI Proxy to AI Bridge for authentication. This header is
// stripped by AI Bridge before forwarding requests to upstream providers.
const HeaderCoderSessionAuth = "X-Coder-Session-Token"

// ExtractAuthToken extracts an authorization token from HTTP headers.
// It checks X-Coder-Session-Token first (set by AI Proxy), then falls back
// to Authorization header (Bearer token) and X-Api-Key header, which represent
// the different ways clients authenticate against AI providers.
// If none are present, an empty string is returned.
func ExtractAuthToken(header http.Header) string {
	if token := strings.TrimSpace(header.Get(HeaderCoderSessionAuth)); token != "" {
		return token
	}
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
