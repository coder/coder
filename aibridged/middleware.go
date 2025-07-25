package aibridged

import (
	"bytes"
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpmw"
)

type ContextKeyBridgeAPIKey struct{}

// AuthMiddleware extracts and validates authorization tokens for AI bridge endpoints.
// It supports both Bearer tokens in Authorization headers and Coder session tokens
// from cookies/headers following the same patterns as existing Coder authentication.
func AuthMiddleware(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract token using the same pattern as the bridge
			token := extractAuthTokenForBridge(r)
			if token == "" {
				http.Error(rw, "Authorization token required", http.StatusUnauthorized)
				return
			}

			// Validate token using httpmw.APIKeyFromRequest
			_, _, ok := httpmw.APIKeyFromRequest(ctx, db, func(r *http.Request) string {
				return token
			}, &http.Request{})

			if !ok {
				http.Error(rw, "Invalid or expired session token", http.StatusUnauthorized)
				return
			}

			// Pass request with modify context including the request token.
			next.ServeHTTP(rw, r.WithContext(context.WithValue(ctx, ContextKeyBridgeAPIKey{}, token)))
		})
	}
}

// extractAuthTokenForBridge extracts authorization token from HTTP request using multiple sources.
// It checks Authorization header (Bearer token), X-Api-Key header, and Coder session headers and cookies.
func extractAuthTokenForBridge(r *http.Request) string {
	// 1. Check Authorization header for Bearer token
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		bearer := []byte("bearer ")
		hdr := []byte(authHeader)

		// Use case-insensitive comparison for Bearer token
		if len(hdr) >= len(bearer) && subtle.ConstantTimeCompare(bytes.ToLower(hdr[:len(bearer)]), bearer) == 1 {
			return string(hdr[len(bearer):])
		}
	}

	// 2. Check X-Api-Key header
	apiKeyHeader := r.Header.Get("X-Api-Key")
	if apiKeyHeader != "" {
		return apiKeyHeader
	}

	// 3. Fall back to Coder's standard token extraction
	return httpmw.APITokenFromRequest(r)
}
