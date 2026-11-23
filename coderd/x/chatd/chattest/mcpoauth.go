package chattest

import (
	"fmt"
	"io"
	"net/http"
	"sync"
)

// The answer mcp.linear.app gave on 2026-11-18 to a request with an
// invalid bearer token. The chatd MCP client has no OAuth handler, so it
// never follows resource_metadata.
const (
	// RecordedUnauthorizedChallenge is the recorded WWW-Authenticate
	// header.
	RecordedUnauthorizedChallenge = `Bearer realm="OAuth", resource_metadata="https://mcp.linear.app/.well-known/oauth-protected-resource/mcp", error="invalid_token", scope="read write"`
	// RecordedUnauthorizedBody is the recorded JSON body.
	RecordedUnauthorizedBody = `{"error":"invalid_token","error_description":"Invalid access token"}`
)

// WriteRecordedUnauthorized answers a request with the recorded 401 and
// returns the error from writing the body.
func WriteRecordedUnauthorized(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", RecordedUnauthorizedChallenge)
	w.WriteHeader(http.StatusUnauthorized)
	_, err := io.WriteString(w, RecordedUnauthorizedBody)
	return err
}

// TokenGrant is one OAuth grant at a fake token endpoint that rotates
// refresh tokens. Each accepted refresh issues a new token pair,
// "<prefix>access-<n>" and "<prefix>refresh-<n>". It accepts the current
// refresh token and, with grace, also the one spent before it, like
// Cloudflare workers-oauth-provider. It is safe for concurrent use.
type TokenGrant struct {
	prefix string
	grace  bool

	mu       sync.Mutex
	current  string
	previous string
	issued   int
}

// NewTokenGrant returns a grant whose current refresh token is
// refreshToken.
func NewTokenGrant(prefix, refreshToken string, grace bool) *TokenGrant {
	return &TokenGrant{prefix: prefix, grace: grace, current: refreshToken}
}

// Rotate spends refreshToken and returns the new token pair, or false
// when the grant does not accept refreshToken.
func (g *TokenGrant) Rotate(refreshToken string) (accessToken, nextRefreshToken string, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	accepted := refreshToken != "" &&
		(refreshToken == g.current || (g.grace && refreshToken == g.previous))
	if !accepted {
		return "", "", false
	}
	g.issued++
	if g.grace {
		g.previous = refreshToken
	}
	g.current = fmt.Sprintf("%srefresh-%d", g.prefix, g.issued)
	return fmt.Sprintf("%saccess-%d", g.prefix, g.issued), g.current, true
}

// Current returns the refresh token the grant issued last, or the one it
// started with.
func (g *TokenGrant) Current() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.current
}
