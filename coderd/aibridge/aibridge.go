// Package aibridge provides utilities for the AI Bridge feature.
package aibridge

import (
	"net/http"
	"strings"
)

// HeaderCoderToken is a header set by clients opting into BYOK
// (Bring Your Own Key) mode. It carries the Coder token so
// that Authorization and X-Api-Key can carry the user's own LLM
// credentials. When present, AI Bridge forwards the user's LLM
// headers unchanged instead of injecting the centralized key.
//
// The AI Bridge proxy also sets this header automatically for clients
// that use per-user LLM credentials but cannot set custom headers.
const HeaderCoderToken = "X-Coder-AI-Governance-Token" //nolint:gosec // This is a header name, not a credential.

// HeaderCoderRequestID is a header set by aibridgeproxyd on each
// request forwarded to aibridged for cross-service log correlation.
const HeaderCoderRequestID = "X-Coder-AI-Governance-Request-Id"

// IsBYOK reports whether the request is using BYOK mode, determined
// by the presence of the X-Coder-AI-Governance-Token header.
func IsBYOK(header http.Header) bool {
	return strings.TrimSpace(header.Get(HeaderCoderToken)) != ""
}

// ExtractAuthToken extracts a token from HTTP headers.
// It checks the BYOK header first (set by clients opting into BYOK),
// then falls back to Authorization: Bearer and X-Api-Key for direct
// centralized mode. If none are present, an empty string is returned.
func ExtractAuthToken(header http.Header) string {
	if token := strings.TrimSpace(header.Get(HeaderCoderToken)); token != "" {
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
