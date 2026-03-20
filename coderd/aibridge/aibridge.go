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

// HeaderCoderBYOKToken is a header set by clients opting into BYOK
// (Bring Your Own Key) mode. It carries the Coder session token so
// that Authorization and X-Api-Key can carry the user's own LLM
// credentials. When present, AI Bridge forwards the user's LLM
// headers unchanged instead of injecting the centralized key.
const HeaderCoderBYOKToken = "X-Coder-AI-Governance-BYOK-Token" //nolint:gosec // This is a header name, not a credential.

// IsBYOK reports whether the request is using BYOK mode, determined
// by the presence of the X-Coder-AI-Governance-BYOK-Token header.
func IsBYOK(header http.Header) bool {
	return strings.TrimSpace(header.Get(HeaderCoderBYOKToken)) != ""
}

// ExtractAuthToken extracts the Coder session token from HTTP headers.
// It checks the BYOK header first (set by clients opting into BYOK),
// then X-Coder-Token (set by AI Proxy), then falls back to
// Authorization: Bearer and X-Api-Key for direct centralized mode.
// If none are present, an empty string is returned.
func ExtractAuthToken(header http.Header) string {
	if token := strings.TrimSpace(header.Get(HeaderCoderBYOKToken)); token != "" {
		return token
	}
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
