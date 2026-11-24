// Package mcpoauth refreshes, serializes and revokes the OAuth2 tokens
// that users store for MCP servers. Every function that calls a provider
// takes a non-nil *http.Client, normally the caller's guarded MCP HTTP
// client.
package mcpoauth

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/safedial"
)

// tokenRequestTimeout caps one token endpoint or revocation endpoint
// request so a stalled provider cannot block an MCP connect. It matches
// the MCP server connect timeout.
const tokenRequestTimeout = 10 * time.Second

// refreshResult is the token endpoint's answer to a refresh. Any
// successful answer is persisted, even one with an unchanged access
// token, to keep a rotated refresh token.
type refreshResult struct {
	// AccessToken is the new (or unchanged) access token.
	AccessToken string
	// RefreshToken is the new (or preserved original) refresh
	// token. Providers that don't rotate refresh tokens return
	// an empty value; in that case the original is kept.
	RefreshToken string
	// TokenType is the token type (usually "Bearer").
	TokenType string
	// Expiry is the new token expiry. Zero value means no expiry
	// was provided by the provider.
	Expiry time.Time
}

// refreshFailureReasonLimit caps the error text persisted to
// mcp_server_user_tokens.oauth_refresh_failure_reason, matching the
// external auth failure reason limit.
const refreshFailureReasonLimit = 400

// isPermanentRefreshError reports whether an OAuth2 token refresh
// error means the user's grant is permanently unusable (for example
// the upstream grant was revoked) rather than a transient provider
// failure. Only error codes tied to the grant itself count: client
// or config problems (invalid_client, unauthorized_client, ...)
// affect every user of the server and cannot be fixed by the user
// reconnecting, so they are treated as transient here. See RFC 6749
// section 5.2.
func isPermanentRefreshError(err error) bool {
	var oauthErr *oauth2.RetrieveError
	if !xerrors.As(err, &oauthErr) {
		return false
	}
	switch oauthErr.ErrorCode {
	case "invalid_grant", // RFC 6749: grant invalid, expired, or revoked
		"bad_refresh_token": // GitHub's equivalent, returned with HTTP 200
		return true
	}
	return false
}

// RefreshFailureReason converts a refresh error into a bounded string
// safe to persist as oauth_refresh_failure_reason. It is stored for
// operator debugging only and is never returned through the API.
func RefreshFailureReason(err error) string {
	reason := err.Error()
	if len(reason) > refreshFailureReasonLimit {
		reason = reason[:refreshFailureReasonLimit]
	}
	return reason
}

// oauth2TokenFromRow converts a stored MCP user token for expiry checks.
func oauth2TokenFromRow(tok database.MCPServerUserToken) *oauth2.Token {
	oauthToken := &oauth2.Token{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
	}
	if tok.Expiry.Valid {
		oauthToken.Expiry = tok.Expiry.Time
	}
	return oauthToken
}

// refreshOAuth2Token refreshes an MCP user token using the OAuth2
// credentials from the server config, sending the config's RFC 8707
// resource indicator when set. A token without a refresh token fails
// without a network call. RefreshOAuth2TokenWithLease calls it under the
// token's refresh lease and persists the result.
func refreshOAuth2Token(
	ctx context.Context,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (refreshResult, error) {
	if tok.RefreshToken == "" {
		return refreshResult{}, xerrors.New("refresh oauth2 token: refresh token is not set")
	}
	return requestOAuth2Refresh(ctx, httpClient, cfg, tok)
}

// requestOAuth2Refresh spends tok's refresh token at the token endpoint.
func requestOAuth2Refresh(
	ctx context.Context,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (refreshResult, error) {
	params := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
	}
	if cfg.OAuth2Resource != "" {
		params.Set("resource", cfg.OAuth2Resource)
	}
	// oauth2.Config cannot add parameters to refresh requests. The
	// clientcredentials token source sends EndpointParams verbatim,
	// allows overriding grant_type, and shares oauth2.Config's token
	// request code: auth style detection, *oauth2.RetrieveError, and
	// keeping the request's refresh_token when the response omits it.
	refreshCfg := &clientcredentials.Config{
		ClientID:       cfg.OAuth2ClientID,
		ClientSecret:   cfg.OAuth2ClientSecret,
		TokenURL:       cfg.OAuth2TokenURL,
		EndpointParams: params,
	}

	// Cap the refresh HTTP call so a stalled token endpoint
	// cannot block the entire MCP connection phase.
	refreshCtx, cancel := context.WithTimeout(ctx, tokenRequestTimeout)
	defer cancel()
	refreshClient := *httpClient
	refreshClient.CheckRedirect = safedial.CheckSameOriginRedirect
	refreshCtx = context.WithValue(refreshCtx, oauth2.HTTPClient, &refreshClient)

	newToken, err := refreshCfg.Token(refreshCtx)
	if err != nil {
		return refreshResult{}, xerrors.Errorf("refresh oauth2 token: %w", err)
	}

	// Preserve the old refresh token when the provider doesn't
	// rotate (returns empty).
	refreshToken := cmp.Or(newToken.RefreshToken, tok.RefreshToken)

	return refreshResult{
		AccessToken:  newToken.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    newToken.TokenType,
		Expiry:       newToken.Expiry,
	}, nil
}
