package gitauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

// Config is used for authentication for Git operations.
type Config struct {
	httpmw.OAuth2Config
	// ID is a unique identifier for the authenticator.
	ID string
	// Regex is a regexp that URLs will match against.
	Regex *regexp.Regexp
	// Type is the type of provider.
	Type codersdk.GitProvider
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
}

// RefreshToken automatically refreshes the token if expired and permitted.
// It returns the token and a bool indicating if the token was refreshed.
func (c *Config) RefreshToken(ctx context.Context, db database.Store, gitAuthLink database.GitAuthLink) (database.GitAuthLink, bool, error) {
	// If the token is expired and refresh is disabled, we prompt
	// the user to authenticate again.
	if c.NoRefresh && gitAuthLink.OAuthExpiry.Before(database.Now()) {
		return gitAuthLink, false, nil
	}

	token, err := c.TokenSource(ctx, &oauth2.Token{
		AccessToken:  gitAuthLink.OAuthAccessToken,
		RefreshToken: gitAuthLink.OAuthRefreshToken,
		Expiry:       gitAuthLink.OAuthExpiry,
	}).Token()
	if err != nil {
		// Even if the token fails to be obtained, we still return false because
		// we aren't trying to surface an error, we're just trying to obtain a valid token.
		return gitAuthLink, false, nil
	}

	if c.ValidateURL != "" {
		valid, err := c.ValidateToken(ctx, token.AccessToken)
		if err != nil {
			return gitAuthLink, false, xerrors.Errorf("validate git auth token: %w", err)
		}
		if !valid {
			// The token is no longer valid!
			return gitAuthLink, false, nil
		}
	}

	if token.AccessToken != gitAuthLink.OAuthAccessToken {
		// Update it
		gitAuthLink, err = db.UpdateGitAuthLink(ctx, database.UpdateGitAuthLinkParams{
			ProviderID:        c.ID,
			UserID:            gitAuthLink.UserID,
			UpdatedAt:         database.Now(),
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
func (c *Config) ValidateToken(ctx context.Context, token string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ValidateURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		// The token is no longer valid!
		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(res.Body)
		return false, xerrors.Errorf("status %d: body: %s", res.StatusCode, data)
	}
	return true, nil
}

// ConvertConfig converts the SDK configuration entry format
// to the parsed and ready-to-consume in coderd provider type.
func ConvertConfig(entries []codersdk.GitAuthConfig, accessURL *url.URL) ([]*Config, error) {
	ids := map[string]struct{}{}
	configs := []*Config{}
	for _, entry := range entries {
		var typ codersdk.GitProvider
		switch codersdk.GitProvider(entry.Type) {
		case codersdk.GitProviderAzureDevops:
			typ = codersdk.GitProviderAzureDevops
		case codersdk.GitProviderBitBucket:
			typ = codersdk.GitProviderBitBucket
		case codersdk.GitProviderGitHub:
			typ = codersdk.GitProviderGitHub
		case codersdk.GitProviderGitLab:
			typ = codersdk.GitProviderGitLab
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
		if entry.ClientSecret == "" {
			return nil, xerrors.Errorf("%q git auth provider: client_secret must be provided", entry.ID)
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

		oauth2Config := &oauth2.Config{
			ClientID:     entry.ClientID,
			ClientSecret: entry.ClientSecret,
			Endpoint:     endpoint[typ],
			RedirectURL:  authRedirect.String(),
			Scopes:       scope[typ],
		}

		if entry.AuthURL != "" {
			oauth2Config.Endpoint.AuthURL = entry.AuthURL
		}
		if entry.TokenURL != "" {
			oauth2Config.Endpoint.TokenURL = entry.TokenURL
		}
		if entry.Scopes != nil && len(entry.Scopes) > 0 {
			oauth2Config.Scopes = entry.Scopes
		}
		if entry.ValidateURL == "" {
			entry.ValidateURL = validateURL[typ]
		}

		var oauthConfig httpmw.OAuth2Config = oauth2Config
		// Azure DevOps uses JWT token authentication!
		if typ == codersdk.GitProviderAzureDevops {
			oauthConfig = newJWTOAuthConfig(oauth2Config)
		}

		configs = append(configs, &Config{
			OAuth2Config: oauthConfig,
			ID:           entry.ID,
			Regex:        regex,
			Type:         typ,
			NoRefresh:    entry.NoRefresh,
			ValidateURL:  entry.ValidateURL,
		})
	}
	return configs, nil
}
