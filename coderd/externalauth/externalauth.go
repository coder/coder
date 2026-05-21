package externalauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/go-github/v43/github"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/oauth2"
	xgithub "golang.org/x/oauth2/github"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth/gitprovider"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

const (
	// failureReasonLimit is the maximum text length of an error to be cached to the
	// database for a failed refresh token. In rare cases, the error could be a large
	// HTML payload.
	failureReasonLimit = 400

	// tokenRevocationTimeout timeout for requests to external oauth provider.
	tokenRevocationTimeout = 10 * time.Second
)

// Config is used for authentication for Git operations.
type Config struct {
	promoauth.InstrumentedOAuth2Config
	// ID is a unique identifier for the authenticator.
	ID string
	// Type is the type of provider.
	Type string

	ClientID     string
	ClientSecret string
	// DeviceAuth is set if the provider uses the device flow.
	DeviceAuth *DeviceAuth
	// DisplayName is the name of the provider to display to the user.
	DisplayName string
	// DisplayIcon is the path to an image that will be displayed to the user.
	DisplayIcon string

	// ExtraTokenKeys is a list of extra properties to
	// store in the database returned from the token endpoint.
	//
	// e.g. Slack returns `authed_user` in the token which is
	// a payload that contains information about the authenticated
	// user.
	ExtraTokenKeys []string

	// NoRefresh stops Coder from using the refresh token
	// to renew the access token.
	//
	// Some organizations have security policies that require
	// re-authentication for every token.
	NoRefresh bool
	// ValidateURL ensures an access token is valid before
	// returning it to the user. If omitted, tokens will
	// not be validated before being returned.
	ValidateURL string

	RevokeURL     string
	RevokeTimeout time.Duration

	// Regex is a Regexp matched against URLs for
	// a Git clone. e.g. "Username for 'https://github.com':"
	// The regex would be `github\.com`..
	Regex *regexp.Regexp
	// APIBaseURL is the base URL for provider REST API calls
	// (e.g., "https://api.github.com" for GitHub). Derived from
	// defaults when not explicitly configured.
	APIBaseURL string
	// AppInstallURL is for GitHub App's (and hopefully others eventually)
	// to provide a link to install the app. There's installation
	// of the application, and user authentication. It's possible
	// for the user to authenticate but the application to not.
	AppInstallURL string
	// AppInstallationsURL is an API endpoint that returns a list of
	// installations for the user. This is used for GitHub Apps.
	AppInstallationsURL string
	// Deprecated: Injected MCP in AI Bridge is deprecated and will be removed in a future release.
	//
	// MCPURL is the endpoint that clients must use to communicate with the associated
	// MCP server.
	MCPURL string
	// Deprecated: Injected MCP in AI Bridge is deprecated and will be removed in a future release.
	//
	// MCPToolAllowRegex is a [regexp.Regexp] to match tools which are explicitly allowed to be
	// injected into Coder AI Bridge upstream requests.
	// In the case of conflicts, [MCPToolDenylistPattern] overrides items evaluated by this list.
	// This field can be nil if unspecified in the config.
	MCPToolAllowRegex *regexp.Regexp
	// Deprecated: Injected MCP in AI Bridge is deprecated and will be removed in a future release.
	//
	// MCPToolDenyRegex is a [regexp.Regexp] to match tools which are explicitly NOT allowed to be
	// injected into Coder AI Bridge upstream requests.
	// In the case of conflicts, items evaluated by this list override [MCPToolAllowRegex].
	// This field can be nil if unspecified in the config.
	MCPToolDenyRegex              *regexp.Regexp
	CodeChallengeMethodsSupported []promoauth.Oauth2PKCEChallengeMethod
}

// Git returns a Provider for this config if the provider type
// is a supported git hosting provider. Returns nil for non-git
// providers (e.g. Slack, JFrog).
func (c *Config) Git(client *http.Client) gitprovider.Provider {
	norm := strings.ToLower(c.Type)
	if !codersdk.EnhancedExternalAuthProvider(norm).Git() {
		return nil
	}
	return gitprovider.New(norm, c.APIBaseURL, client)
}

// GenerateTokenExtra generates the extra token data to store in the database.
func (c *Config) GenerateTokenExtra(token *oauth2.Token) (pqtype.NullRawMessage, error) {
	if len(c.ExtraTokenKeys) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	extraMap := map[string]any{}
	for _, key := range c.ExtraTokenKeys {
		extraMap[key] = token.Extra(key)
	}
	data, err := json.Marshal(extraMap)
	if err != nil {
		return pqtype.NullRawMessage{}, err
	}
	return pqtype.NullRawMessage{
		RawMessage: data,
		Valid:      true,
	}, nil
}

// InvalidTokenError is a case where the "RefreshToken" failed to complete
// as a result of invalid credentials. Error contains the reason of the failure.
type InvalidTokenError string

func (e InvalidTokenError) Error() string {
	return string(e)
}

func IsInvalidTokenError(err error) bool {
	var invalidTokenError InvalidTokenError
	return xerrors.As(err, &invalidTokenError)
}

// RefreshToken automatically refreshes the token if expired and permitted.
func (c *Config) RefreshToken(ctx context.Context, db database.Store, externalAuthLink database.ExternalAuthLink) (database.ExternalAuthLink, error) {
	// If the token is expired and refresh is disabled, we prompt
	// the user to authenticate again.
	if c.NoRefresh &&
		// If the time is set to 0, then it should never expire.
		// This is true for github, which has no expiry.
		!externalAuthLink.OAuthExpiry.IsZero() &&
		externalAuthLink.OAuthExpiry.Before(dbtime.Now()) {
		return externalAuthLink, InvalidTokenError("token expired, refreshing is either disabled or refreshing failed and will not be retried")
	}

	refreshToken := externalAuthLink.OAuthRefreshToken

	// This is additional defensive programming. Because TokenSource is an interface,
	// we cannot be sure that the implementation will treat an 'IsZero' time
	// as "not-expired". The default implementation does, but a custom implementation
	// might not. Removing the refreshToken will guarantee a refresh will fail.
	if c.NoRefresh {
		refreshToken = ""
	}

	existingToken := &oauth2.Token{
		AccessToken:  externalAuthLink.OAuthAccessToken,
		RefreshToken: refreshToken,
		Expiry:       externalAuthLink.OAuthExpiry,
	}

	// Note: The TokenSource(...) method will make no remote HTTP requests if the
	// token is expired and no refresh token is set. This is important to prevent
	// spamming the API, consuming rate limits, when the token is known to fail.
	token, err := c.TokenSource(ctx, existingToken).Token()
	if err != nil {
		// TokenSource can fail for numerous reasons. If it fails because of
		// a bad refresh token, then the refresh token is invalid, and we should
		// get rid of it. Keeping it around will cause additional refresh
		// attempts that will fail and cost us api rate limits.
		//
		// The error message is saved for debugging purposes.
		if isFailedRefresh(existingToken, err) {
			// Before caching the failure, re-read the external auth link
			// from the database. A concurrent request may have already
			// refreshed the token successfully, consuming the single-use
			// refresh token (e.g., GitHub App tokens). In that case our
			// "bad_refresh_token" error is a false positive from losing
			// the race, and we should use the winner's updated token
			// instead of poisoning the database with a cached failure.
			currentLink, readErr := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
				ProviderID: externalAuthLink.ProviderID,
				UserID:     externalAuthLink.UserID,
			})
			if readErr == nil && currentLink.OAuthRefreshToken != externalAuthLink.OAuthRefreshToken {
				// Another caller won the refresh race and stored a new
				// refresh token. Return their updated link instead of
				// caching a failure.
				return currentLink, nil
			}

			reason := err.Error()
			if len(reason) > failureReasonLimit {
				// Limit the length of the error message to prevent
				// spamming the database with long error messages.
				reason = reason[:failureReasonLimit]
			}
			dbExecErr := db.UpdateExternalAuthLinkRefreshToken(ctx, database.UpdateExternalAuthLinkRefreshTokenParams{
				// Adding a reason will prevent further attempts to try and refresh the token.
				OauthRefreshFailureReason: reason,
				// Remove the invalid refresh token so it is never used again. The cached
				// `reason` can be used to know why this field was zeroed out.
				OAuthRefreshToken:      "",
				OAuthRefreshTokenKeyID: externalAuthLink.OAuthRefreshTokenKeyID.String,
				UpdatedAt:              dbtime.Now(),
				ProviderID:             externalAuthLink.ProviderID,
				UserID:                 externalAuthLink.UserID,
				// Optimistic lock: only clear the token if it hasn't been
				// updated by a concurrent caller that won the refresh race.
				OldOauthRefreshToken: externalAuthLink.OAuthRefreshToken,
			})
			if dbExecErr != nil {
				// This error should be rare.
				return externalAuthLink, InvalidTokenError(fmt.Sprintf("refresh token failed: %q, then removing refresh token failed: %q", err.Error(), dbExecErr.Error()))
			}
			// The refresh token was cleared
			externalAuthLink.OAuthRefreshToken = ""
			externalAuthLink.UpdatedAt = dbtime.Now()
		}

		// Unfortunately have to match exactly on the error message string.
		// Improve the error message to account refresh tokens are deleted if
		// invalid on our end.
		//
		// This error messages comes from the oauth2 package on our client side.
		// So this check is not against a server generated error message.
		// Error source: https://github.com/golang/oauth2/blob/master/oauth2.go#L277
		if err.Error() == "oauth2: token expired and refresh token is not set" {
			if externalAuthLink.OauthRefreshFailureReason != "" {
				// A cached refresh failure error exists. So the refresh token was set, but was invalid, and zeroed out.
				// Return this cached error for the original refresh attempt.
				return externalAuthLink, InvalidTokenError(fmt.Sprintf("token expired and refreshing failed %s with: %s",
					// Do not return the exact time, because then we have to know what timezone the
					// user is in. This approximate time is good enough.
					humanize.Time(externalAuthLink.UpdatedAt),
					externalAuthLink.OauthRefreshFailureReason,
				))
			}

			return externalAuthLink, InvalidTokenError("token expired, refreshing is either disabled or refreshing failed and will not be retried")
		}

		// TokenSource(...).Token() will always return the current token if the token is not expired.
		// So this error is only returned if a refresh of the token failed.
		return externalAuthLink, InvalidTokenError(fmt.Sprintf("refresh token: %s", err.Error()))
	}

	extra, err := c.GenerateTokenExtra(token)
	if err != nil {
		return externalAuthLink, xerrors.Errorf("generate token extra: %w", err)
	}

	// Persist the refreshed token to the DB before validation. GitHub
	// rotates refresh tokens on every use, so the old refresh token is
	// already invalid on the IDP side. If we validated first and the
	// validation endpoint was unavailable (e.g. rate-limited 403), the
	// new token would be silently lost and the user would be forced to
	// re-authenticate manually.
	// Use a detached context for the DB write only. The IDP already
	// consumed the old refresh token, so if the caller's request
	// context is canceled mid-save, the new token would be lost.
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer persistCancel()

	originalAccessToken := externalAuthLink.OAuthAccessToken
	if token.AccessToken != originalAccessToken {
		updatedAuthLink, err := db.UpdateExternalAuthLink(persistCtx, database.UpdateExternalAuthLinkParams{
			ProviderID:             c.ID,
			UserID:                 externalAuthLink.UserID,
			UpdatedAt:              dbtime.Now(),
			OAuthAccessToken:       token.AccessToken,
			OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
			OAuthRefreshToken:      token.RefreshToken,
			OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
			OAuthExpiry:            token.Expiry,
			OAuthExtra:             extra,
		})
		if err != nil {
			return updatedAuthLink, xerrors.Errorf("persist refreshed token: %w", err)
		}
		externalAuthLink = updatedAuthLink
	}

	r := retry.New(50*time.Millisecond, 200*time.Millisecond)
	// See the comment below why the retry and cancel is required.
	retryCtx, retryCtxCancel := context.WithTimeout(ctx, time.Second)
	defer retryCtxCancel()
validate:
	valid, user, err := c.ValidateToken(ctx, token)
	if err != nil {
		return externalAuthLink, xerrors.Errorf("validate external auth token: %w", err)
	}
	if !valid {
		// A customer using GitHub in Australia reported that validating immediately
		// after refreshing the token would intermittently fail with a 401. Waiting
		// a few milliseconds with the exact same token on the exact same request
		// would resolve the issue. It seems likely that the write is not propagating
		// to the read replica in time.
		//
		// We do an exponential backoff here to give the write time to propagate.
		if c.Type == string(codersdk.EnhancedExternalAuthProviderGitHub) && r.Wait(retryCtx) {
			goto validate
		}
		// The token is no longer valid!
		return externalAuthLink, InvalidTokenError("token failed to validate")
	}

	// Update the associated user's github.com user ID if the token
	// is for github.com and validation returned user info.
	if token.AccessToken != originalAccessToken && IsGithubDotComURL(c.AuthCodeURL("")) && user != nil {
		err = db.UpdateUserGithubComUserID(ctx, database.UpdateUserGithubComUserIDParams{
			ID: externalAuthLink.UserID,
			GithubComUserID: sql.NullInt64{
				Int64: user.ID,
				Valid: true,
			},
		})
		if err != nil {
			return externalAuthLink, xerrors.Errorf("update user github com user id: %w", err)
		}
	}

	return externalAuthLink, nil
}

// ValidateToken checks if the Git token provided is valid.
// The user is optionally returned if the provider supports it.
// Returns valid=true when: the provider confirmed the token,
// no ValidateURL is configured, or the validation endpoint
// returned a rate-limited response (403 with rate-limit headers
// or 429).
func (c *Config) ValidateToken(ctx context.Context, link *oauth2.Token) (bool, *codersdk.ExternalAuthUser, error) {
	if link == nil {
		return false, nil, xerrors.New("validate external auth token: token is nil")
	}
	if !link.Expiry.IsZero() && link.Expiry.Before(dbtime.Now()) {
		return false, nil, nil
	}

	if c.ValidateURL == "" {
		// Default that the token is valid if no validation URL is provided.
		return true, nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ValidateURL, nil)
	if err != nil {
		return false, nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", link.AccessToken))
	res, err := c.InstrumentedOAuth2Config.Do(ctx, promoauth.SourceValidateToken, req)
	if err != nil {
		return false, nil, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusUnauthorized:
		// The token is no longer valid!
		return false, nil, nil

	case http.StatusForbidden:
		// Some providers (notably GitHub) use 403 for both "token
		// revoked" and "rate limit exceeded." If standard rate-limit
		// headers are present, the token may still be valid and the
		// validation endpoint is rejecting for a transient reason.
		// Treat it as optimistically valid rather than discarding
		// the token.
		if isRateLimited(res) {
			return true, nil, nil
		}
		// No rate-limit headers: genuine token revocation or
		// permission error.
		return false, nil, nil

	case http.StatusTooManyRequests:
		// GitHub can return either 403 or 429 for rate limits.
		// Treat 429 the same as a rate-limited 403: optimistically
		// valid. The token was likely just issued by the IDP; the
		// validation endpoint is transiently overloaded.
		return true, nil, nil

	case http.StatusOK:
		// Success, handled below.

	default:
		data, _ := io.ReadAll(res.Body)
		return false, nil, xerrors.Errorf("status %d: body: %s", res.StatusCode, data)
	}

	var user *codersdk.ExternalAuthUser
	if c.Type == string(codersdk.EnhancedExternalAuthProviderGitHub) {
		var ghUser github.User
		err = json.NewDecoder(res.Body).Decode(&ghUser)
		if err == nil {
			user = &codersdk.ExternalAuthUser{
				ID:         ghUser.GetID(),
				Login:      ghUser.GetLogin(),
				AvatarURL:  ghUser.GetAvatarURL(),
				ProfileURL: ghUser.GetHTMLURL(),
				Name:       ghUser.GetName(),
			}
		}
	}

	return true, user, nil
}

type AppInstallation struct {
	ID int
	// Login is the username of the installation.
	Login string
	// URL is a link to configure the app install.
	URL string
}

// AppInstallations returns a list of app installations for the given token.
// If the provider does not support app installations, it returns nil.
func (c *Config) AppInstallations(ctx context.Context, token string) ([]codersdk.ExternalAuthAppInstallation, bool, error) {
	if c.AppInstallationsURL == "" {
		return nil, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.AppInstallationsURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	res, err := c.InstrumentedOAuth2Config.Do(ctx, promoauth.SourceAppInstallations, req)
	if err != nil {
		return nil, false, err
	}
	defer res.Body.Close()
	// It's possible the installation URL is misconfigured, so we don't
	// want to return an error here.
	if res.StatusCode != http.StatusOK {
		return nil, false, nil
	}
	installs := []codersdk.ExternalAuthAppInstallation{}
	if c.Type == string(codersdk.EnhancedExternalAuthProviderGitHub) {
		var ghInstalls struct {
			Installations []*github.Installation `json:"installations"`
		}
		err = json.NewDecoder(res.Body).Decode(&ghInstalls)
		if err != nil {
			return nil, false, err
		}
		for _, installation := range ghInstalls.Installations {
			account := installation.GetAccount()
			if account == nil {
				continue
			}
			installs = append(installs, codersdk.ExternalAuthAppInstallation{
				ID:           int(installation.GetID()),
				ConfigureURL: installation.GetHTMLURL(),
				Account: codersdk.ExternalAuthUser{
					ID:         account.GetID(),
					Login:      account.GetLogin(),
					AvatarURL:  account.GetAvatarURL(),
					ProfileURL: account.GetHTMLURL(),
					Name:       account.GetName(),
				},
			})
		}
	}
	return installs, true, nil
}

func (c *Config) RevokeToken(ctx context.Context, link database.ExternalAuthLink) (bool, error) {
	if c.RevokeURL == "" {
		return false, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.RevokeTimeout)
	defer cancel()
	req, err := c.TokenRevocationRequest(reqCtx, link)
	if err != nil {
		return false, err
	}

	res, err := c.InstrumentedOAuth2Config.Do(ctx, promoauth.SourceRevoke, req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return false, err
	}

	if c.TokenRevocationResponseOk(res) {
		return true, nil
	}
	return false, xerrors.Errorf("failed to revoke token: %d %s", res.StatusCode, string(body))
}

func (c *Config) TokenRevocationRequest(ctx context.Context, link database.ExternalAuthLink) (*http.Request, error) {
	if c.Type == codersdk.EnhancedExternalAuthProviderGitHub.String() {
		return c.TokenRevocationRequestGitHub(ctx, link)
	}
	return c.TokenRevocationRequestRFC7009(ctx, link)
}

func (c *Config) TokenRevocationRequestRFC7009(ctx context.Context, link database.ExternalAuthLink) (*http.Request, error) {
	p := url.Values{}
	p.Add("client_id", c.ClientID)
	p.Add("client_secret", c.ClientSecret)
	if link.OAuthRefreshToken != "" {
		p.Add("token_type_hint", "refresh_token")
		p.Add("token", link.OAuthRefreshToken)
	} else {
		p.Add("token_type_hint", "access_token")
		p.Add("token", link.OAuthAccessToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.RevokeURL, strings.NewReader(p.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", link.OAuthAccessToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}

func (c *Config) TokenRevocationRequestGitHub(ctx context.Context, link database.ExternalAuthLink) (*http.Request, error) {
	// GitHub doesn't follow RFC spec
	// https://docs.github.com/en/rest/apps/oauth-applications?apiVersion=2022-11-28#delete-an-app-authorization
	body := fmt.Sprintf("{\"access_token\":%q}", link.OAuthAccessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.RevokeURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/vnd.github+json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")
	req.SetBasicAuth(c.ClientID, c.ClientSecret)
	return req, nil
}

func (c *Config) TokenRevocationResponseOk(res *http.Response) bool {
	// RFC spec on successful revocation returns 200, GitHub 204
	if c.Type == codersdk.EnhancedExternalAuthProviderGitHub.String() {
		return res.StatusCode == http.StatusNoContent
	}
	return res.StatusCode == http.StatusOK
}

type DeviceAuth struct {
	// Config is provided for the http client method.
	Config   promoauth.InstrumentedOAuth2Config
	ClientID string
	TokenURL string
	Scopes   []string
	CodeURL  string
}

// AuthorizeDevice begins the device authorization flow.
// See: https://tools.ietf.org/html/rfc8628#section-3.1
func (c *DeviceAuth) AuthorizeDevice(ctx context.Context) (*codersdk.ExternalAuthDevice, error) {
	if c.CodeURL == "" {
		return nil, xerrors.New("oauth2: device code URL not set")
	}
	codeURL, err := c.formatDeviceCodeURL()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codeURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	do := http.DefaultClient.Do
	if c.Config != nil {
		// The cfg can be nil in unit tests.
		do = func(req *http.Request) (*http.Response, error) {
			return c.Config.Do(ctx, promoauth.SourceAuthorizeDevice, req)
		}
	}

	resp, err := do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		codersdk.ExternalAuthDevice
		ErrorDescription string `json:"error_description"`
	}
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if err != nil {
			mediaType = "unknown"
		}

		// If the json fails to decode, do a best effort to return a better error.
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			retryIn := "please try again later"
			resetIn := resp.Header.Get("x-ratelimit-reset")
			if resetIn != "" {
				// Best effort to tell the user exactly how long they need
				// to wait for.
				unix, err := strconv.ParseInt(resetIn, 10, 64)
				if err == nil {
					waitFor := time.Unix(unix, 0).Sub(time.Now().Truncate(time.Second))
					retryIn = fmt.Sprintf(" retry after %s", waitFor.Truncate(time.Second))
				}
			}
			// 429 returns a plaintext payload with a message.
			return nil, xerrors.New(fmt.Sprintf("rate limit hit, unable to authorize device. %s", retryIn))
		case mediaType == "application/x-www-form-urlencoded":
			return nil, xerrors.Errorf("status_code=%d, payload response is form-url encoded, expected a json payload", resp.StatusCode)
		default:
			return nil, xerrors.Errorf("status_code=%d, mediaType=%s: %w", resp.StatusCode, mediaType, err)
		}
	}
	if r.ErrorDescription != "" {
		return nil, xerrors.New(r.ErrorDescription)
	}
	return &r.ExternalAuthDevice, nil
}

type ExchangeDeviceCodeResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ExchangeDeviceCode exchanges a device code for an access token.
// The boolean returned indicates whether the device code is still pending
// and the caller should try again.
func (c *DeviceAuth) ExchangeDeviceCode(ctx context.Context, deviceCode string) (*oauth2.Token, error) {
	if c.TokenURL == "" {
		return nil, xerrors.New("oauth2: token URL not set")
	}
	tokenURL, err := c.formatDeviceTokenURL(deviceCode)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, codersdk.ReadBodyAsError(resp)
	}
	var body ExchangeDeviceCodeResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	if err != nil {
		return nil, err
	}
	if body.Error != "" {
		return nil, xerrors.New(body.Error)
	}
	// If expiresIn is 0, then the token never expires.
	expires := dbtime.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	if body.ExpiresIn == 0 {
		expires = time.Time{}
	}
	return &oauth2.Token{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		Expiry:       expires,
	}, nil
}

func (c *DeviceAuth) formatDeviceTokenURL(deviceCode string) (string, error) {
	tok, err := url.Parse(c.TokenURL)
	if err != nil {
		return "", err
	}
	tok.RawQuery = url.Values{
		"client_id":   {c.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}.Encode()
	return tok.String(), nil
}

func (c *DeviceAuth) formatDeviceCodeURL() (string, error) {
	cod, err := url.Parse(c.CodeURL)
	if err != nil {
		return "", err
	}
	cod.RawQuery = url.Values{
		"client_id": {c.ClientID},
		"scope":     c.Scopes,
	}.Encode()
	return cod.String(), nil
}

// ConvertConfig converts the SDK configuration entry format
// to the parsed and ready-to-consume in coderd provider type.
func ConvertConfig(instrument *promoauth.Factory, entries []codersdk.ExternalAuthConfig, accessURL *url.URL) ([]*Config, error) {
	ids := map[string]struct{}{}
	configs := []*Config{}
	for _, entry := range entries {
		// Applies defaults to the config entry.
		// This allows users to very simply state that they type is "GitHub",
		// apply their client secret and ID, and have the UI appear nicely.
		applyDefaultsToConfig(&entry)

		valid := codersdk.NameValid(entry.ID)
		if valid != nil {
			return nil, xerrors.Errorf("external auth provider %q doesn't have a valid id: %w", entry.ID, valid)
		}
		if entry.ClientID == "" {
			return nil, xerrors.Errorf("%q external auth provider: client_id must be provided", entry.ID)
		}

		_, exists := ids[entry.ID]
		if exists {
			if entry.ID == entry.Type {
				return nil, xerrors.Errorf("multiple %s external auth providers provided. you must specify a unique id for each", entry.Type)
			}
			return nil, xerrors.Errorf("multiple external auth providers exist with the id %q. specify a unique id for each", entry.ID)
		}
		ids[entry.ID] = struct{}{}

		authRedirect, err := accessURL.Parse(fmt.Sprintf("/external-auth/%s/callback", entry.ID))
		if err != nil {
			return nil, xerrors.Errorf("parse external auth callback url: %w", err)
		}

		var regex *regexp.Regexp
		if entry.Regex != "" {
			regex, err = regexp.Compile(entry.Regex)
			if err != nil {
				return nil, xerrors.Errorf("compile regex for external auth provider %q: %w", entry.ID, entry.Regex)
			}
		}

		oc := &oauth2.Config{
			ClientID:     entry.ClientID,
			ClientSecret: entry.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  entry.AuthURL,
				TokenURL: entry.TokenURL,
			},
			RedirectURL: authRedirect.String(),
			Scopes:      entry.Scopes,
		}

		var oauthConfig promoauth.OAuth2Config = oc
		// Azure DevOps uses JWT token authentication!
		if entry.Type == string(codersdk.EnhancedExternalAuthProviderAzureDevops) {
			oauthConfig = &jwtConfig{oc}
		}
		if entry.Type == string(codersdk.EnhancedExternalAuthProviderAzureDevopsEntra) {
			oauthConfig = &entraV1Oauth{oc}
		}
		if entry.Type == string(codersdk.EnhancedExternalAuthProviderJFrog) {
			oauthConfig = &exchangeWithClientSecret{oc}
		}

		instrumented := instrument.New(entry.ID, oauthConfig)
		if strings.EqualFold(entry.Type, string(codersdk.EnhancedExternalAuthProviderGitHub)) {
			instrumented = instrument.NewGithub(entry.ID, oauthConfig)
		}

		var mcpToolAllow *regexp.Regexp
		var mcpToolDeny *regexp.Regexp
		if entry.MCPToolAllowRegex != "" {
			mcpToolAllow, err = regexp.Compile(entry.MCPToolAllowRegex)
			if err != nil {
				return nil, xerrors.Errorf("compile MCP tool allow regex for external auth provider %q: %w", entry.ID, entry.MCPToolAllowRegex)
			}
		}
		if entry.MCPToolDenyRegex != "" {
			mcpToolDeny, err = regexp.Compile(entry.MCPToolDenyRegex)
			if err != nil {
				return nil, xerrors.Errorf("compile MCP tool deny regex for external auth provider %q: %w", entry.ID, entry.MCPToolDenyRegex)
			}
		}

		cfg := &Config{
			InstrumentedOAuth2Config:      instrumented,
			ID:                            entry.ID,
			ClientID:                      entry.ClientID,
			ClientSecret:                  entry.ClientSecret,
			Regex:                         regex,
			APIBaseURL:                    entry.APIBaseURL,
			Type:                          entry.Type,
			NoRefresh:                     entry.NoRefresh,
			ValidateURL:                   entry.ValidateURL,
			RevokeURL:                     entry.RevokeURL,
			RevokeTimeout:                 tokenRevocationTimeout,
			AppInstallationsURL:           entry.AppInstallationsURL,
			AppInstallURL:                 entry.AppInstallURL,
			DisplayName:                   entry.DisplayName,
			DisplayIcon:                   entry.DisplayIcon,
			ExtraTokenKeys:                entry.ExtraTokenKeys,
			MCPURL:                        entry.MCPURL,
			MCPToolAllowRegex:             mcpToolAllow,
			MCPToolDenyRegex:              mcpToolDeny,
			CodeChallengeMethodsSupported: slice.StringEnums[promoauth.Oauth2PKCEChallengeMethod](entry.CodeChallengeMethodsSupported),
		}

		if entry.DeviceFlow {
			if entry.DeviceCodeURL == "" {
				return nil, xerrors.Errorf("external auth provider %q: device auth url must be provided", entry.ID)
			}
			cfg.DeviceAuth = &DeviceAuth{
				Config:   cfg,
				ClientID: entry.ClientID,
				TokenURL: oc.Endpoint.TokenURL,
				Scopes:   entry.Scopes,
				CodeURL:  entry.DeviceCodeURL,
			}
		}

		configs = append(configs, cfg)
	}
	return configs, nil
}

// applyDefaultsToConfig applies defaults to the config entry.
func applyDefaultsToConfig(config *codersdk.ExternalAuthConfig) {
	configType := codersdk.EnhancedExternalAuthProvider(strings.ToLower(config.Type))
	if configType == "bitbucket" {
		// For backwards compatibility, we need to support the "bitbucket" string.
		configType = codersdk.EnhancedExternalAuthProviderBitBucketCloud
		defer func() {
			// The config type determines the config ID (if unset). So change the legacy
			// type to the correct new type after the defaults have been configured.
			config.Type = string(codersdk.EnhancedExternalAuthProviderBitBucketCloud)
		}()
	}
	// If static defaults exist, apply them.
	if defaults, ok := staticDefaults[configType]; ok {
		copyDefaultSettings(config, defaults)
		return
	}

	// Dynamic defaults
	switch configType {
	case codersdk.EnhancedExternalAuthProviderGitHub:
		copyDefaultSettings(config, gitHubDefaults(config))
		return
	case codersdk.EnhancedExternalAuthProviderGitLab:
		copyDefaultSettings(config, gitlabDefaults(config))
		return
	case codersdk.EnhancedExternalAuthProviderBitBucketServer:
		copyDefaultSettings(config, bitbucketServerDefaults(config))
		return
	case codersdk.EnhancedExternalAuthProviderJFrog:
		copyDefaultSettings(config, jfrogArtifactoryDefaults(config))
		return
	case codersdk.EnhancedExternalAuthProviderGitea:
		copyDefaultSettings(config, giteaDefaults(config))
		return
	case codersdk.EnhancedExternalAuthProviderAzureDevopsEntra:
		copyDefaultSettings(config, azureDevopsEntraDefaults(config))
		return
	default:
		// Global defaults are specified at the end of the `copyDefaultSettings` function.
		copyDefaultSettings(config, codersdk.ExternalAuthConfig{})
		return
	}
}

func copyDefaultSettings(config *codersdk.ExternalAuthConfig, defaults codersdk.ExternalAuthConfig) {
	if config.AuthURL == "" {
		config.AuthURL = defaults.AuthURL
	}
	if config.TokenURL == "" {
		config.TokenURL = defaults.TokenURL
	}
	if config.ValidateURL == "" {
		config.ValidateURL = defaults.ValidateURL
	}
	if config.RevokeURL == "" {
		config.RevokeURL = defaults.RevokeURL
	}
	if config.AppInstallURL == "" {
		config.AppInstallURL = defaults.AppInstallURL
	}
	if config.AppInstallationsURL == "" {
		config.AppInstallationsURL = defaults.AppInstallationsURL
	}
	if config.Regex == "" {
		config.Regex = defaults.Regex
	}
	if len(config.Scopes) == 0 {
		config.Scopes = defaults.Scopes
	}
	if config.DeviceCodeURL == "" {
		config.DeviceCodeURL = defaults.DeviceCodeURL
	}
	if config.DisplayName == "" {
		config.DisplayName = defaults.DisplayName
	}
	if config.DisplayIcon == "" {
		config.DisplayIcon = defaults.DisplayIcon
	}
	if len(config.ExtraTokenKeys) == 0 {
		config.ExtraTokenKeys = defaults.ExtraTokenKeys
	}
	if config.CodeChallengeMethodsSupported == nil {
		config.CodeChallengeMethodsSupported = defaults.CodeChallengeMethodsSupported
	}

	// Apply defaults if it's still empty...
	if config.ID == "" {
		config.ID = config.Type
	}
	if config.DisplayName == "" {
		config.DisplayName = config.Type
	}
	if config.DisplayIcon == "" {
		// This is a key emoji.
		config.DisplayIcon = "/emojis/1f511.png"
	}
	if config.CodeChallengeMethodsSupported == nil {
		config.CodeChallengeMethodsSupported = []string{string(promoauth.PKCEChallengeMethodSha256)}
	}

	// Set default API base URL for providers that need one.
	if config.APIBaseURL == "" {
		normType := strings.ToLower(config.Type)
		switch codersdk.EnhancedExternalAuthProvider(normType) {
		case codersdk.EnhancedExternalAuthProviderGitHub:
			config.APIBaseURL = "https://api.github.com"
		case codersdk.EnhancedExternalAuthProviderGitLab:
			config.APIBaseURL = "https://gitlab.com/api/v4"
		case codersdk.EnhancedExternalAuthProviderGitea:
			config.APIBaseURL = "https://gitea.com/api/v1"
		}
	}
}

// gitHubDefaults returns default config values for GitHub.
// The only dynamic value is the revocation URL which depends on client ID.
func gitHubDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	defaults := codersdk.ExternalAuthConfig{
		AuthURL:     xgithub.Endpoint.AuthURL,
		TokenURL:    xgithub.Endpoint.TokenURL,
		ValidateURL: "https://api.github.com/user",
		DisplayName: "GitHub",
		DisplayIcon: "/icon/github.svg",
		Regex:       `^(https?://)?github\.com(/.*)?$`,
		// "workflow" is required for managing GitHub Actions in a repository.
		Scopes:                        []string{"repo", "workflow"},
		DeviceCodeURL:                 "https://github.com/login/device/code",
		AppInstallationsURL:           "https://api.github.com/user/installations",
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodSha256)},
	}

	if config.RevokeURL == "" && config.ClientID != "" {
		defaults.RevokeURL = fmt.Sprintf("https://api.github.com/applications/%s/grant", config.ClientID)
	}

	return defaults
}

func bitbucketServerDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	defaults := codersdk.ExternalAuthConfig{
		DisplayName: "Bitbucket Server",
		Scopes:      []string{"PUBLIC_REPOS", "REPO_READ", "REPO_WRITE"},
		DisplayIcon: "/icon/bitbucket.svg",
		// TODO: Investigate if 'S256' is accepted and PKCE is supported
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodNone)},
	}
	// Bitbucket servers will have some base url, e.g. https://bitbucket.coder.com.
	// We will grab this from the Auth URL. This choice is a bit arbitrary,
	// but we need to require at least 1 field to be populated.
	if config.AuthURL == "" {
		// No auth url, means we cannot guess the urls.
		return defaults
	}

	auth, err := url.Parse(config.AuthURL)
	if err != nil {
		// We need a valid URL to continue with.
		return defaults
	}

	// Populate Regex, ValidateURL, and TokenURL.
	// Default regex should be anything using the same host as the auth url.
	defaults.Regex = fmt.Sprintf(`^(https?://)?%s(/.*)?$`, strings.ReplaceAll(auth.Host, ".", `\.`))

	tokenURL := auth.ResolveReference(&url.URL{Path: "/rest/oauth2/latest/token"})
	defaults.TokenURL = tokenURL.String()

	// validate needs to return a 200 when logged in and a 401 when unauthenticated.
	// This endpoint returns the count of the number of PR's in the authenticated
	// user's inbox. Which will work perfectly for our use case.
	validate := auth.ResolveReference(&url.URL{Path: "/rest/api/latest/inbox/pull-requests/count"})
	defaults.ValidateURL = validate.String()

	return defaults
}

// gitlabDefaults returns a static config if using the gitlab cloud offering.
// The values are dynamic if using a self-hosted gitlab.
// When the decision is not obvious, just defer to the cloud defaults.
// Any user specific fields will override this if provided.
func gitlabDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	cloud := codersdk.ExternalAuthConfig{
		AuthURL:                       "https://gitlab.com/oauth/authorize",
		TokenURL:                      "https://gitlab.com/oauth/token",
		ValidateURL:                   "https://gitlab.com/oauth/token/info",
		RevokeURL:                     "https://gitlab.com/oauth/revoke",
		DisplayName:                   "GitLab",
		DisplayIcon:                   "/icon/gitlab.svg",
		Regex:                         `^(https?://)?gitlab\.com(/.*)?$`,
		Scopes:                        []string{"write_repository"},
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodSha256)},
	}

	if config.AuthURL == "" || config.AuthURL == cloud.AuthURL {
		return cloud
	}

	au, err := url.Parse(config.AuthURL)
	if err != nil || au.Host == "gitlab.com" {
		// If the AuthURL is not a valid URL or is using the cloud,
		// use the cloud static defaults.
		return cloud
	}

	// At this point, assume it is self-hosted and use the AuthURL
	return codersdk.ExternalAuthConfig{
		DisplayName:                   cloud.DisplayName,
		Scopes:                        cloud.Scopes,
		DisplayIcon:                   cloud.DisplayIcon,
		AuthURL:                       au.ResolveReference(&url.URL{Path: "/oauth/authorize"}).String(),
		TokenURL:                      au.ResolveReference(&url.URL{Path: "/oauth/token"}).String(),
		ValidateURL:                   au.ResolveReference(&url.URL{Path: "/oauth/token/info"}).String(),
		RevokeURL:                     au.ResolveReference(&url.URL{Path: "/oauth/revoke"}).String(),
		Regex:                         fmt.Sprintf(`^(https?://)?%s(/.*)?$`, strings.ReplaceAll(au.Host, ".", `\.`)),
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodSha256)},
	}
}

func jfrogArtifactoryDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	defaults := codersdk.ExternalAuthConfig{
		DisplayName: "JFrog Artifactory",
		Scopes:      []string{"applied-permissions/user"},
		DisplayIcon: "/icon/jfrog.svg",
		// TODO: Investigate if 'S256' is accepted and PKCE is supported
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodNone)},
	}
	// Artifactory servers will have some base url, e.g. https://jfrog.coder.com.
	// We will grab this from the Auth URL. This choice is not arbitrary. It is a
	// static string for all integrations on the same artifactory.
	if config.AuthURL == "" {
		// No auth url, means we cannot guess the urls.
		return defaults
	}

	auth, err := url.Parse(config.AuthURL)
	if err != nil {
		// We need a valid URL to continue with.
		return defaults
	}

	if config.ClientID == "" {
		return defaults
	}

	tokenURL := auth.ResolveReference(&url.URL{Path: fmt.Sprintf("/access/api/v1/integrations/%s/token", config.ClientID)})
	defaults.TokenURL = tokenURL.String()

	// validate needs to return a 200 when logged in and a 401 when unauthenticated.
	validate := auth.ResolveReference(&url.URL{Path: "/access/api/v1/system/ping"})
	defaults.ValidateURL = validate.String()

	// Some options omitted:
	// - Regex: Artifactory can span pretty much all domains (git, docker, etc).
	//          I do not think we can intelligently guess this as a default.

	return defaults
}

func giteaDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	defaults := codersdk.ExternalAuthConfig{
		DisplayName:                   "Gitea",
		Scopes:                        []string{"read:repository", " write:repository", "read:user"},
		DisplayIcon:                   "/icon/gitea.svg",
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodSha256)},
	}
	// Gitea's servers will have some base url, e.g: https://gitea.coder.com.
	// If an auth url is not set, we will assume they are using the default
	// public Gitea.
	if config.AuthURL == "" {
		config.AuthURL = "https://gitea.com/login/oauth/authorize"
	}

	auth, err := url.Parse(config.AuthURL)
	if err != nil {
		// We need a valid URL to continue with.
		return defaults
	}

	// Default regex should be anything using the same host as the auth url.
	defaults.Regex = fmt.Sprintf(`^(https?://)?%s(/.*)?$`, strings.ReplaceAll(auth.Host, ".", `\.`))

	tokenURL := auth.ResolveReference(&url.URL{Path: "/login/oauth/access_token"})
	defaults.TokenURL = tokenURL.String()

	validate := auth.ResolveReference(&url.URL{Path: "/login/oauth/userinfo"})
	defaults.ValidateURL = validate.String()

	return defaults
}

func azureDevopsEntraDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	defaults := codersdk.ExternalAuthConfig{
		DisplayName: "Azure DevOps (Entra)",
		DisplayIcon: "/icon/azure-devops.svg",
		Regex:       `^(https?://)?dev\.azure\.com(/.*)?$`,
		// TODO: Investigate if 'S256' is accepted and PKCE is supported
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodNone)},
	}
	// The tenant ID is required for urls and is in the auth url.
	if config.AuthURL == "" {
		// No auth url, means we cannot guess the urls.
		return defaults
	}

	auth, err := url.Parse(config.AuthURL)
	if err != nil {
		// We need a valid URL to continue with.
		return defaults
	}

	// Only extract the tenant ID if the path is what we expect.
	// The path should be /{tenantId}/oauth2/authorize.
	parts := strings.Split(auth.Path, "/")
	if len(parts) < 4 && parts[2] != "oauth2" || parts[3] != "authorize" {
		// Not sure what this path is, abort.
		return defaults
	}
	tenantID := parts[1]

	tokenURL := auth.ResolveReference(&url.URL{Path: fmt.Sprintf("/%s/oauth2/token", tenantID)})
	defaults.TokenURL = tokenURL.String()

	// TODO: Discover a validate url for Azure DevOps.

	return defaults
}

var staticDefaults = map[codersdk.EnhancedExternalAuthProvider]codersdk.ExternalAuthConfig{
	codersdk.EnhancedExternalAuthProviderAzureDevops: {
		AuthURL:     "https://app.vssps.visualstudio.com/oauth2/authorize",
		TokenURL:    "https://app.vssps.visualstudio.com/oauth2/token",
		DisplayName: "Azure DevOps",
		DisplayIcon: "/icon/azure-devops.svg",
		Regex:       `^(https?://)?dev\.azure\.com(/.*)?$`,
		Scopes:      []string{"vso.code_write"},
		// TODO: Investigate if 'S256' is accepted and PKCE is supported
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodNone)},
	},
	codersdk.EnhancedExternalAuthProviderBitBucketCloud: {
		AuthURL:     "https://bitbucket.org/site/oauth2/authorize",
		TokenURL:    "https://bitbucket.org/site/oauth2/access_token",
		ValidateURL: "https://api.bitbucket.org/2.0/user",
		DisplayName: "BitBucket",
		DisplayIcon: "/icon/bitbucket.svg",
		Regex:       `^(https?://)?bitbucket\.org(/.*)?$`,
		Scopes:      []string{"account", "repository:write"},
		// TODO: Investigate if 'S256' is accepted and PKCE is supported
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodNone)},
	},
	codersdk.EnhancedExternalAuthProviderSlack: {
		AuthURL:     "https://slack.com/oauth/v2/authorize",
		TokenURL:    "https://slack.com/api/oauth.v2.access",
		RevokeURL:   "https://slack.com/api/auth.revoke",
		DisplayName: "Slack",
		DisplayIcon: "/icon/slack.svg",
		// See: https://api.slack.com/authentication/oauth-v2#exchanging
		ExtraTokenKeys: []string{"authed_user"},
		// TODO: Investigate if 'S256' is accepted and PKCE is supported
		CodeChallengeMethodsSupported: []string{string(promoauth.PKCEChallengeMethodNone)},
	},
}

// jwtConfig is a new OAuth2 config that uses a custom
// assertion method that works with Azure Devops. See:
// https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/oauth?view=azure-devops
type jwtConfig struct {
	*oauth2.Config
}

func (c *jwtConfig) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return c.Config.AuthCodeURL(state, append(opts, oauth2.SetAuthURLParam("response_type", "Assertion"))...)
}

func (c *jwtConfig) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	v := url.Values{
		"client_assertion_type": {},
		"client_assertion":      {c.ClientSecret},
		"assertion":             {code},
		"grant_type":            {},
	}
	if c.RedirectURL != "" {
		v.Set("redirect_uri", c.RedirectURL)
	}
	return c.Config.Exchange(ctx, code,
		append(opts,
			oauth2.SetAuthURLParam("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"),
			oauth2.SetAuthURLParam("client_assertion", c.ClientSecret),
			oauth2.SetAuthURLParam("assertion", code),
			oauth2.SetAuthURLParam("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer"),
			oauth2.SetAuthURLParam("code", ""),
		)...,
	)
}

// When authenticating via Entra ID ADO only supports v1 tokens that requires the 'resource' rather than scopes
// When ADO gets support for V2 Entra ID tokens this struct and functions can be removed
type entraV1Oauth struct {
	*oauth2.Config
}

const azureDevOpsAppID = "499b84ac-1321-427f-aa17-267ca6975798"

func (c *entraV1Oauth) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return c.Config.AuthCodeURL(state, append(opts, oauth2.SetAuthURLParam("resource", azureDevOpsAppID))...)
}

func (c *entraV1Oauth) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return c.Config.Exchange(ctx, code,
		append(opts,
			oauth2.SetAuthURLParam("resource", azureDevOpsAppID),
		)...,
	)
}

// exchangeWithClientSecret wraps an OAuth config and adds the client secret
// to the Exchange request as a Bearer header. This is used by JFrog Artifactory.
type exchangeWithClientSecret struct {
	*oauth2.Config
}

func (e *exchangeWithClientSecret) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	httpClient, ok := ctx.Value(oauth2.HTTPClient).(*http.Client)
	if httpClient == nil || !ok {
		httpClient = http.DefaultClient
	}
	oldTransport := httpClient.Transport
	if oldTransport == nil {
		oldTransport = http.DefaultTransport
	}
	httpClient.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
		req.Header.Set("Authorization", "Bearer "+e.ClientSecret)
		return oldTransport.RoundTrip(req)
	})
	return e.Config.Exchange(context.WithValue(ctx, oauth2.HTTPClient, httpClient), code, opts...)
}

type roundTripper func(req *http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

// IsGithubDotComURL returns true if the given URL is a github.com URL.
func IsGithubDotComURL(str string) bool {
	str = strings.ToLower(str)
	ghURL, err := url.Parse(str)
	if err != nil {
		return false
	}
	return ghURL.Host == "github.com"
}

// isRateLimited checks whether an HTTP response indicates a rate
// limit rather than a genuine authorization failure. It returns
// true if either X-RateLimit-Remaining is "0" (primary) or
// Retry-After is present (secondary). OR logic is intentional:
// GitHub secondary limits can include Retry-After without
// X-RateLimit-Remaining: 0 (the remaining count tracks the
// primary quota, not secondary).
//
// Does not catch every secondary rate limit. GitHub can return
// 403 with positive X-RateLimit-Remaining and no Retry-After.
// Reliable detection of those requires response body inspection.
// Missing them is not a regression since all 403s were previously
// treated as invalid.
func isRateLimited(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return false
}

// isFailedRefresh returns true if the error returned by the TokenSource.Token()
// is due to a failed refresh. The failure being the refresh token itself.
// If this returns true, no amount of retries will fix the issue.
//
// Notes: Provider responses are not uniform. Here are some examples:
// Github
//   - Returns a 200 with Code "bad_refresh_token" and Description "The refresh token passed is incorrect or expired."
//
// Gitea [TODO: get an expired refresh token]
//   - [Bad JWT] Returns 400 with Code "unauthorized_client" and Description "unable to parse refresh token"
//
// Gitlab
//   - Returns 400 with Code "invalid_grant" and Description "The provided authorization grant is invalid, expired, revoked, does not match the redirection URI used in the authorization request, or was issued to another client."
func isFailedRefresh(existingToken *oauth2.Token, err error) bool {
	if existingToken.RefreshToken == "" {
		return false // No refresh token, so this cannot be refreshed
	}

	if existingToken.Valid() {
		return false // Valid tokens are not refreshed
	}

	var oauthErr *oauth2.RetrieveError
	if xerrors.As(err, &oauthErr) {
		switch oauthErr.ErrorCode {
		// Known error codes that indicate a failed refresh.
		// 'Spec' means the code is defined in the spec.
		case "bad_refresh_token", // Github
			"invalid_grant",                // Gitlab & Spec
			"unauthorized_client",          // Gitea & Spec
			"unsupported_grant_type",       // Spec, refresh not supported
			"incorrect_client_credentials", // GitHub, wrong client_id/secret (HTTP 200)
			"invalid_client":               // RFC 6749 Section 5.2, client auth failed
			return true
		}

		switch oauthErr.Response.StatusCode {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusOK:
			// Status codes that indicate the request was processed
			// and rejected. 403 is intentionally excluded: no known
			// provider returns 403 from the token endpoint, and the
			// previous 403 case caused token destruction on
			// rate-limited refresh attempts.
			return true
		case http.StatusInternalServerError, http.StatusTooManyRequests:
			// These do not indicate a failed refresh, but could be a temporary issue.
			return false
		}
	}

	return false
}
