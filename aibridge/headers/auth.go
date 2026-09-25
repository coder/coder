package headers

import "strings"

// Auth header names shared by providers and request handlers.
const (
	// AuthHeaderXAPIKey carries an API key.
	AuthHeaderXAPIKey = "X-Api-Key" //nolint:gosec // HTTP header name, not a credential.
	// AuthHeaderAuthorization carries an authorization credential.
	AuthHeaderAuthorization = "Authorization"
)

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
