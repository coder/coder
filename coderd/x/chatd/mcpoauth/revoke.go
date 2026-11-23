package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/safedial"
)

// RevokeOAuth2Token revokes the user's token at the provider's RFC 7009
// endpoint. It prefers the refresh token, retrying with the access token
// only on unsupported_token_type; other failures do not fall back, since
// an access-token success would hide a possibly live refresh token.
// Returns false without error when there is no revocation endpoint or no
// stored token. Errors carry only the HTTP status because provider
// bodies may echo secrets.
func RevokeOAuth2Token(
	ctx context.Context,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	tok database.MCPServerUserToken,
) (bool, error) {
	if cfg.OAuth2RevocationURL == "" {
		return false, nil
	}
	if tok.RefreshToken == "" && tok.AccessToken == "" {
		return false, nil
	}
	if err := ValidateRevocationEndpoint(cfg.OAuth2RevocationURL); err != nil {
		return false, err
	}

	// Copy so CheckRedirect does not leak into the shared client.
	redirectSafe := *httpClient
	redirectSafe.CheckRedirect = safedial.CheckSameOriginRedirect
	httpClient = &redirectSafe

	token, hint := tok.AccessToken, "access_token"
	if tok.RefreshToken != "" {
		token, hint = tok.RefreshToken, "refresh_token"
	}
	status, errorCode, err := postTokenRevocation(ctx, httpClient, cfg, token, hint)
	if err != nil {
		return false, err
	}
	if isRevocationSuccessStatus(status) {
		return true, nil
	}

	if hint == "refresh_token" && tok.AccessToken != "" && errorCode == "unsupported_token_type" {
		fbStatus, _, fbErr := postTokenRevocation(ctx, httpClient, cfg, tok.AccessToken, "access_token")
		if fbErr != nil {
			return false, fbErr
		}
		if isRevocationSuccessStatus(fbStatus) {
			return true, nil
		}
		return false, xerrors.Errorf(
			"revocation endpoint returned HTTP %d for the refresh token and HTTP %d for the access token",
			status, fbStatus,
		)
	}
	return false, xerrors.Errorf(
		"revocation endpoint returned HTTP %d", status,
	)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ValidateRevocationEndpoint enforces the RFC 7009 HTTPS requirement;
// the request carries token material and the client secret. Plain HTTP
// is allowed only for loopback hosts.
func ValidateRevocationEndpoint(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return xerrors.Errorf("parse revocation URL: %w", err)
	}
	// url.Parse accepts hostless forms like "https:/revoke" that can
	// never be POSTed to.
	if parsed.Hostname() == "" {
		return xerrors.Errorf(
			"revocation endpoint %q has no host", parsed.Redacted(),
		)
	}
	if !isAllowedRevocationScheme(parsed) {
		return xerrors.Errorf(
			"revocation endpoint %q must use https", parsed.Redacted(),
		)
	}
	return nil
}

func isAllowedRevocationScheme(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && isLoopbackHost(u.Hostname())
}

func isRevocationSuccessStatus(status int) bool {
	return status == http.StatusOK || status == http.StatusNoContent
}

// postTokenRevocation returns the HTTP status and the RFC 6749 error
// code from the body; the raw body never propagates.
func postTokenRevocation(
	ctx context.Context,
	httpClient *http.Client,
	cfg database.MCPServerConfig,
	token, tokenTypeHint string,
) (int, string, error) {
	form := url.Values{}
	form.Set("token", token)
	form.Set("token_type_hint", tokenTypeHint)
	// Only public clients send client_id in the body; mixing it with
	// Basic auth is malformed per RFC 6749 section 2.3.1.
	if cfg.OAuth2ClientSecret == "" {
		form.Set("client_id", cfg.OAuth2ClientID)
	}

	revokeCtx, cancel := context.WithTimeout(ctx, tokenRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		revokeCtx, http.MethodPost,
		cfg.OAuth2RevocationURL, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return 0, "", xerrors.Errorf("create revocation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Credentials are form-encoded per RFC 6749 section 2.3.1
	// (mirrors x/oauth2).
	if cfg.OAuth2ClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(cfg.OAuth2ClientID), url.QueryEscape(cfg.OAuth2ClientSecret))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, "", xerrors.Errorf("revoke oauth2 token: %w", err)
	}
	defer resp.Body.Close()

	if isRevocationSuccessStatus(resp.StatusCode) {
		_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // The drain only lets the connection be reused.
		return resp.StatusCode, "", nil
	}
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&errBody) //nolint:errcheck // A body without an RFC 6749 error object has no error code.
	_, _ = io.Copy(io.Discard, resp.Body)                                 //nolint:errcheck // The drain only lets the connection be reused.
	return resp.StatusCode, errBody.Error, nil
}
