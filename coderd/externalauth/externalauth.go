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

	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/google/go-github/v43/github"
	"github.com/sqlc-dev/pqtype"
	xgithub "golang.org/x/oauth2/github"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

// Config is used for authentication for Git operations.
type Config struct {
	promoauth.InstrumentedOAuth2Config
	// ID is a unique identifier for the authenticator.
	ID string
	// Type is the type of provider.
	Type string
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

	// Regex is a Regexp matched against URLs for
	// a Git clone. e.g. "Username for 'https://github.com':"
	// The regex would be `github\.com`..
	Regex *regexp.Regexp
	// AppInstallURL is for GitHub App's (and hopefully others eventually)
	// to provide a link to install the app. There's installation
	// of the application, and user authentication. It's possible
	// for the user to authenticate but the application to not.
	AppInstallURL string
	// AppInstallationsURL is an API endpoint that returns a list of
	// installations for the user. This is used for GitHub Apps.
	AppInstallationsURL string
}

// GenerateTokenExtra generates the extra token data to store in the database.
func (c *Config) GenerateTokenExtra(token *oauth2.Token) (pqtype.NullRawMessage, error) {
	if len(c.ExtraTokenKeys) == 0 {
		return pqtype.NullRawMessage{}, nil
	}
	extraMap := map[string]interface{}{}
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
// If an error is returned, the token is either invalid, or an error occurred.
// Use 'IsInvalidTokenError(err)' to determine the difference.
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

	// This is additional defensive programming. Because TokenSource is an interface,
	// we cannot be sure that the implementation will treat an 'IsZero' time
	// as "not-expired". The default implementation does, but a custom implementation
	// might not. Removing the refreshToken will guarantee a refresh will fail.
	refreshToken := externalAuthLink.OAuthRefreshToken
	if c.NoRefresh {
		refreshToken = ""
	}

	existingToken := &oauth2.Token{
		AccessToken:  externalAuthLink.OAuthAccessToken,
		RefreshToken: refreshToken,
		Expiry:       externalAuthLink.OAuthExpiry,
	}

	token, err := c.TokenSource(ctx, existingToken).Token()
	if err != nil {
		// TokenSource can fail for numerous reasons. If it fails because of
		// a bad refresh token, then the refresh token is invalid, and we should
		// get rid of it. Keeping it around will cause additional refresh
		// attempts that will fail and cost us api rate limits.
		if isFailedRefresh(existingToken, err) {
			dbExecErr := db.UpdateExternalAuthLinkRefreshToken(ctx, database.UpdateExternalAuthLinkRefreshTokenParams{
				OAuthRefreshToken:      "", // It is better to clear the refresh token than to keep retrying.
				OAuthRefreshTokenKeyID: externalAuthLink.OAuthRefreshTokenKeyID.String,
				UpdatedAt:              dbtime.Now(),
				ProviderID:             externalAuthLink.ProviderID,
				UserID:                 externalAuthLink.UserID,
			})
			if dbExecErr != nil {
				// This error should be rare.
				return externalAuthLink, InvalidTokenError(fmt.Sprintf("refresh token failed: %q, then removing refresh token failed: %q", err.Error(), dbExecErr.Error()))
			}
			// The refresh token was cleared
			externalAuthLink.OAuthRefreshToken = ""
		}

		// Unfortunately have to match exactly on the error message string.
		// Improve the error message to account refresh tokens are deleted if
		// invalid on our end.
		if err.Error() == "oauth2: token expired and refresh token is not set" {
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

	if token.AccessToken != externalAuthLink.OAuthAccessToken {
		updatedAuthLink, err := db.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
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
			return updatedAuthLink, xerrors.Errorf("update external auth link: %w", err)
		}
		externalAuthLink = updatedAuthLink

		// Update the associated users github.com username if the token is for github.com.
		if IsGithubDotComURL(c.AuthCodeURL("")) && user != nil {
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
	}

	return externalAuthLink, nil
}

// ValidateToken ensures the Git token provided is valid!
// The user is optionally returned if the provider supports it.
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
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		// The token is no longer valid!
		return false, nil, nil
	}
	if res.StatusCode != http.StatusOK {
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

		cfg := &Config{
			InstrumentedOAuth2Config: instrumented,
			ID:                       entry.ID,
			Regex:                    regex,
			Type:                     entry.Type,
			NoRefresh:                entry.NoRefresh,
			ValidateURL:              entry.ValidateURL,
			AppInstallationsURL:      entry.AppInstallationsURL,
			AppInstallURL:            entry.AppInstallURL,
			DisplayName:              entry.DisplayName,
			DisplayIcon:              entry.DisplayIcon,
			ExtraTokenKeys:           entry.ExtraTokenKeys,
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
	configType := codersdk.EnhancedExternalAuthProvider(config.Type)
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
	switch codersdk.EnhancedExternalAuthProvider(config.Type) {
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
		// No defaults for this type. We still want to run this apply with
		// an empty set of defaults.
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
}

func bitbucketServerDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	defaults := codersdk.ExternalAuthConfig{
		DisplayName: "Bitbucket Server",
		Scopes:      []string{"PUBLIC_REPOS", "REPO_READ", "REPO_WRITE"},
		DisplayIcon: "/icon/bitbucket.svg",
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
		AuthURL:     "https://gitlab.com/oauth/authorize",
		TokenURL:    "https://gitlab.com/oauth/token",
		ValidateURL: "https://gitlab.com/oauth/token/info",
		DisplayName: "GitLab",
		DisplayIcon: "/icon/gitlab.svg",
		Regex:       `^(https?://)?gitlab\.com(/.*)?$`,
		Scopes:      []string{"write_repository"},
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
		DisplayName: cloud.DisplayName,
		Scopes:      cloud.Scopes,
		DisplayIcon: cloud.DisplayIcon,
		AuthURL:     au.ResolveReference(&url.URL{Path: "/oauth/authorize"}).String(),
		TokenURL:    au.ResolveReference(&url.URL{Path: "/oauth/token"}).String(),
		ValidateURL: au.ResolveReference(&url.URL{Path: "/oauth/token/info"}).String(),
		Regex:       fmt.Sprintf(`^(https?://)?%s(/.*)?$`, strings.ReplaceAll(au.Host, ".", `\.`)),
	}
}

func jfrogArtifactoryDefaults(config *codersdk.ExternalAuthConfig) codersdk.ExternalAuthConfig {
	defaults := codersdk.ExternalAuthConfig{
		DisplayName: "JFrog Artifactory",
		Scopes:      []string{"applied-permissions/user"},
		DisplayIcon: "/icon/jfrog.svg",
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
		DisplayName: "Gitea",
		Scopes:      []string{"read:repository", " write:repository", "read:user"},
		DisplayIcon: "/icon/gitea.svg",
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
	},
	codersdk.EnhancedExternalAuthProviderBitBucketCloud: {
		AuthURL:     "https://bitbucket.org/site/oauth2/authorize",
		TokenURL:    "https://bitbucket.org/site/oauth2/access_token",
		ValidateURL: "https://api.bitbucket.org/2.0/user",
		DisplayName: "BitBucket",
		DisplayIcon: "/icon/bitbucket.svg",
		Regex:       `^(https?://)?bitbucket\.org(/.*)?$`,
		Scopes:      []string{"account", "repository:write"},
	},
	codersdk.EnhancedExternalAuthProviderGitHub: {
		AuthURL:     xgithub.Endpoint.AuthURL,
		TokenURL:    xgithub.Endpoint.TokenURL,
		ValidateURL: "https://api.github.com/user",
		DisplayName: "GitHub",
		DisplayIcon: "/icon/github.svg",
		Regex:       `^(https?://)?github\.com(/.*)?$`,
		// "workflow" is required for managing GitHub Actions in a repository.
		Scopes:              []string{"repo", "workflow"},
		DeviceCodeURL:       "https://github.com/login/device/code",
		AppInstallationsURL: "https://api.github.com/user/installations",
	},
	codersdk.EnhancedExternalAuthProviderSlack: {
		AuthURL:     "https://slack.com/oauth/v2/authorize",
		TokenURL:    "https://slack.com/api/oauth.v2.access",
		DisplayName: "Slack",
		DisplayIcon: "/icon/slack.svg",
		// See: https://api.slack.com/authentication/oauth-v2#exchanging
		ExtraTokenKeys: []string{"authed_user"},
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
			"invalid_grant",          // Gitlab & Spec
			"unauthorized_client",    // Gitea & Spec
			"unsupported_grant_type": // Spec, refresh not supported
			return true
		}

		switch oauthErr.Response.StatusCode {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusOK:
			// Status codes that indicate the request was processed, and rejected.
			return true
		case http.StatusInternalServerError, http.StatusTooManyRequests:
			// These do not indicate a failed refresh, but could be a temporary issue.
			return false
		}
	}

	return false
}
