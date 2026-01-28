// Package aibridge provides utilities for the AI Bridge feature.
package aibridge

import (
	"net/http"
	"strings"
)

// HeaderCoderAuth is an internal header used to pass the Coder token
// from AI Proxy to AI Bridge for authentication. This header is stripped
// by AI Bridge before forwarding requests to upstream providers.
const HeaderCoderAuth = "X-Coder-Token"

// ExtractAuthToken extracts an authorization token from HTTP headers.
// It checks X-Coder-Token first (set by AI Proxy), then falls back
// to Authorization header (Bearer token) and X-Api-Key header, which represent
// the different ways clients authenticate against AI providers.
// If none are present, an empty string is returned.
func ExtractAuthToken(header http.Header) string {
	if token := strings.TrimSpace(header.Get(HeaderCoderAuth)); token != "" {
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
