package aibridged

import (
	"bytes"
	"context"
	"crypto/subtle"
	"net/http"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

// AuthMiddleware extracts and validates authorization tokens for AI bridge endpoints.
// It supports both Bearer tokens in Authorization headers and Coder session tokens
// from cookies/headers following the same patterns as existing Coder authentication.
func AuthMiddleware(db database.Store, logger slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			token := extractAuthToken(r)
			if token == "" {
				http.Error(rw, "Authorization token required", http.StatusUnauthorized)
				return
			}

			// Validate token using httpmw.APIKeyFromRequest
			key, _, ok := httpmw.APIKeyFromRequest(ctx, db, func(*http.Request) string {
				return token
			}, &http.Request{})

			if !ok {
				http.Error(rw, "Invalid or expired session token", http.StatusUnauthorized)
				return
			}

			// Inject the initiator's RBAC subject into the scope so all actions occur on their behalf.
			actor, _, err := httpmw.UserRBACSubject(ctx, db, key.UserID, rbac.ScopeAll)
			if err != nil {
				logger.Error(ctx, "failed to setup user RBAC context", slog.Error(err), slog.F("user_id", key.UserID), slog.F("key_id", key.ID))
				http.Error(rw, "internal server error", http.StatusInternalServerError) // Don't leak reason as this might have security implications.
				return
			}
			ctx = dbauthz.As(ctx, actor)

			// TODO: I'd prefer if we didn't have to do this, or at least in this fashion.
			// Inject the API key into the context to later be used to authenticate against the Coder MCP server.
			ctx = context.WithValue(ctx, ContextKeyBridgeAPIKey{}, token)

			// Pass request with modify context including the request token.
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// extractAuthToken extracts authorization token from HTTP request using multiple sources.
// These sources represent the different ways clients authenticate against AI providers.
// It checks Authorization header (Bearer token), X-Api-Key header, and Coder session headers and cookies.
func extractAuthToken(r *http.Request) string {
	// 1. Check Authorization header for Bearer token.
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		bearer := []byte("bearer ")
		hdr := []byte(authHeader)

		// Use case-insensitive comparison for Bearer token.
		if len(hdr) >= len(bearer) && subtle.ConstantTimeCompare(bytes.ToLower(hdr[:len(bearer)]), bearer) == 1 {
			return string(hdr[len(bearer):])
		}
	}

	// 2. Check X-Api-Key header.
	apiKeyHeader := r.Header.Get("X-Api-Key")
	if apiKeyHeader != "" {
		return apiKeyHeader
	}

	// 3. Fall back to Coder's standard token extraction.
	return httpmw.APITokenFromRequest(r)
}
