package gitauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/google/go-github/v43/github"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

type OAuth2Config interface {
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
	TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource
}

// Config is used for authentication for Git operations.
type Config struct {
	OAuth2Config
	// ID is a unique identifier for the authenticator.
	ID string
	// Type is the type of provider.
	Type codersdk.ExternalAuthProvider
	// DeviceAuth is set if the provider uses the device flow.
	DeviceAuth *DeviceAuth

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

// RefreshToken automatically refreshes the token if expired and permitted.
// It returns the token and a bool indicating if the token is valid.
func (c *Config) RefreshToken(ctx context.Context, db database.Store, gitAuthLink database.ExternalAuthLink) (database.ExternalAuthLink, bool, error) {
	// If the token is expired and refresh is disabled, we prompt
	// the user to authenticate again.
	if c.NoRefresh &&
		// If the time is set to 0, then it should never expire.
		// This is true for github, which has no expiry.
		!gitAuthLink.OAuthExpiry.IsZero() &&
		gitAuthLink.OAuthExpiry.Before(dbtime.Now()) {
		return gitAuthLink, false, nil
	}

	// This is additional defensive programming. Because TokenSource is an interface,
	// we cannot be sure that the implementation will treat an 'IsZero' time
	// as "not-expired". The default implementation does, but a custom implementation
	// might not. Removing the refreshToken will guarantee a refresh will fail.
	refreshToken := gitAuthLink.OAuthRefreshToken
	if c.NoRefresh {
		refreshToken = ""
	}

	token, err := c.TokenSource(ctx, &oauth2.Token{
		AccessToken:  gitAuthLink.OAuthAccessToken,
		RefreshToken: refreshToken,
		Expiry:       gitAuthLink.OAuthExpiry,
	}).Token()
	if err != nil {
		// Even if the token fails to be obtained, we still return false because
		// we aren't trying to surface an error, we're just trying to obtain a valid token.
		return gitAuthLink, false, nil
	}
	r := retry.New(50*time.Millisecond, 200*time.Millisecond)
	// See the comment below why the retry and cancel is required.
	retryCtx, retryCtxCancel := context.WithTimeout(ctx, time.Second)
	defer retryCtxCancel()
validate:
	valid, _, err := c.ValidateToken(ctx, token.AccessToken)
	if err != nil {
		return gitAuthLink, false, xerrors.Errorf("validate git auth token: %w", err)
	}
	if !valid {
		// A customer using GitHub in Australia reported that validating immediately
		// after refreshing the token would intermittently fail with a 401. Waiting
		// a few milliseconds with the exact same token on the exact same request
		// would resolve the issue. It seems likely that the write is not propagating
		// to the read replica in time.
		//
		// We do an exponential backoff here to give the write time to propagate.
		if c.Type == codersdk.ExternalAuthProviderGitHub && r.Wait(retryCtx) {
			goto validate
		}
		// The token is no longer valid!
		return gitAuthLink, false, nil
	}

	if token.AccessToken != gitAuthLink.OAuthAccessToken {
		// Update it
		gitAuthLink, err = db.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
			ProviderID:        c.ID,
			UserID:            gitAuthLink.UserID,
			UpdatedAt:         dbtime.Now(),
			OAuthAccessToken:  token.AccessToken,
			OAuthRefreshToken: token.RefreshToken,
			OAuthExpiry:       token.Expiry,
		})
		if err != nil {
			return gitAuthLink, false, xerrors.Errorf("update git auth link: %w", err)
		}
	}
	return gitAuthLink, true, nil
}

// ValidateToken ensures the Git token provided is valid!
// The user is optionally returned if the provider supports it.
func (c *Config) ValidateToken(ctx context.Context, token string) (bool, *codersdk.GitAuthUser, error) {
	if c.ValidateURL == "" {
		// Default that the token is valid if no validation URL is provided.
		return true, nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ValidateURL, nil)
	if err != nil {
		return false, nil, err
	}

	cli := http.DefaultClient
	if v, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		cli = v
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	res, err := cli.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		// The token is no longer valid!
		return false, nil, nil
	}
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(res.Body)
		return false, nil, xerrors.Errorf("status %d: body: %s", res.StatusCode, data)
	}

	var user *codersdk.GitAuthUser
	if c.Type == codersdk.ExternalAuthProviderGitHub {
		var ghUser github.User
		err = json.NewDecoder(res.Body).Decode(&ghUser)
		if err == nil {
			user = &codersdk.GitAuthUser{
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
func (c *Config) AppInstallations(ctx context.Context, token string) ([]codersdk.GitAuthAppInstallation, bool, error) {
	if c.AppInstallationsURL == "" {
		return nil, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.AppInstallationsURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer res.Body.Close()
	// It's possible the installation URL is misconfigured, so we don't
	// want to return an error here.
	if res.StatusCode != http.StatusOK {
		return nil, false, nil
	}
	installs := []codersdk.GitAuthAppInstallation{}
	if c.Type == codersdk.ExternalAuthProviderGitHub {
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
			installs = append(installs, codersdk.GitAuthAppInstallation{
				ID:           int(installation.GetID()),
				ConfigureURL: installation.GetHTMLURL(),
				Account: codersdk.GitAuthUser{
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

// ConvertConfig converts the SDK configuration entry format
// to the parsed and ready-to-consume in coderd provider type.
func ConvertConfig(entries []codersdk.GitAuthConfig, accessURL *url.URL) ([]*Config, error) {
	ids := map[string]struct{}{}
	configs := []*Config{}
	for _, entry := range entries {
		var typ codersdk.ExternalAuthProvider
		switch codersdk.ExternalAuthProvider(entry.Type) {
		case codersdk.ExternalAuthProviderAzureDevops:
			typ = codersdk.ExternalAuthProviderAzureDevops
		case codersdk.ExternalAuthProviderBitBucket:
			typ = codersdk.ExternalAuthProviderBitBucket
		case codersdk.ExternalAuthProviderGitHub:
			typ = codersdk.ExternalAuthProviderGitHub
		case codersdk.ExternalAuthProviderGitLab:
			typ = codersdk.ExternalAuthProviderGitLab
		default:
			return nil, xerrors.Errorf("unknown git provider type: %q", entry.Type)
		}
		if entry.ID == "" {
			// Default to the type.
			entry.ID = string(typ)
		}
		if valid := httpapi.NameValid(entry.ID); valid != nil {
			return nil, xerrors.Errorf("git auth provider %q doesn't have a valid id: %w", entry.ID, valid)
		}

		_, exists := ids[entry.ID]
		if exists {
			if entry.ID == string(typ) {
				return nil, xerrors.Errorf("multiple %s git auth providers provided. you must specify a unique id for each", typ)
			}
			return nil, xerrors.Errorf("multiple git providers exist with the id %q. specify a unique id for each", entry.ID)
		}
		ids[entry.ID] = struct{}{}

		if entry.ClientID == "" {
			return nil, xerrors.Errorf("%q git auth provider: client_id must be provided", entry.ID)
		}
		authRedirect, err := accessURL.Parse(fmt.Sprintf("/gitauth/%s/callback", entry.ID))
		if err != nil {
			return nil, xerrors.Errorf("parse gitauth callback url: %w", err)
		}
		regex := regex[typ]
		if entry.Regex != "" {
			regex, err = regexp.Compile(entry.Regex)
			if err != nil {
				return nil, xerrors.Errorf("compile regex for git auth provider %q: %w", entry.ID, entry.Regex)
			}
		}

		oc := &oauth2.Config{
			ClientID:     entry.ClientID,
			ClientSecret: entry.ClientSecret,
			Endpoint:     endpoint[typ],
			RedirectURL:  authRedirect.String(),
			Scopes:       scope[typ],
		}

		if entry.AuthURL != "" {
			oc.Endpoint.AuthURL = entry.AuthURL
		}
		if entry.TokenURL != "" {
			oc.Endpoint.TokenURL = entry.TokenURL
		}
		if entry.Scopes != nil && len(entry.Scopes) > 0 {
			oc.Scopes = entry.Scopes
		}
		if entry.ValidateURL == "" {
			entry.ValidateURL = validateURL[typ]
		}
		if entry.AppInstallationsURL == "" {
			entry.AppInstallationsURL = appInstallationsURL[typ]
		}

		var oauthConfig OAuth2Config = oc
		// Azure DevOps uses JWT token authentication!
		if typ == codersdk.ExternalAuthProviderAzureDevops {
			oauthConfig = &jwtConfig{oc}
		}

		cfg := &Config{
			OAuth2Config:        oauthConfig,
			ID:                  entry.ID,
			Regex:               regex,
			Type:                typ,
			NoRefresh:           entry.NoRefresh,
			ValidateURL:         entry.ValidateURL,
			AppInstallationsURL: entry.AppInstallationsURL,
			AppInstallURL:       entry.AppInstallURL,
		}

		if entry.DeviceFlow {
			if entry.DeviceCodeURL == "" {
				entry.DeviceCodeURL = deviceAuthURL[typ]
			}
			if entry.DeviceCodeURL == "" {
				return nil, xerrors.Errorf("git auth provider %q: device auth url must be provided", entry.ID)
			}
			cfg.DeviceAuth = &DeviceAuth{
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
