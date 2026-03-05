// Package aibridge provides utilities for the AI Bridge feature.
package aibridge

import (
	"net/http"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// HeaderCoderAuth is an internal header used to pass the Coder token
// from AI Proxy to AI Bridge for authentication. This header is stripped
// by AI Bridge before forwarding requests to upstream providers.
const HeaderCoderAuth = "X-Coder-Token"

// ExtractAuthToken extracts an authorization token from an HTTP request.
// It checks X-Coder-Token first (set by AI Proxy), then falls back to
// Authorization (Bearer token), X-Api-Key, Coder-Session-Token header, and
// coder_session_token cookie, in that order. If none are present, an empty
// string is returned.
func ExtractAuthToken(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get(HeaderCoderAuth)); token != "" {
		return token
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); auth != "" {
		fields := strings.Fields(auth)
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			return fields[1]
		}
	}
	if apiKey := strings.TrimSpace(r.Header.Get("X-Api-Key")); apiKey != "" {
		return apiKey
	}
	if sessionToken := strings.TrimSpace(r.Header.Get(codersdk.SessionTokenHeader)); sessionToken != "" {
		return sessionToken
	}
	if cookie, err := r.Cookie(codersdk.SessionTokenCookie); err == nil {
		if sessionToken := strings.TrimSpace(cookie.Value); sessionToken != "" {
			return sessionToken
		}
	}
	return ""
}
