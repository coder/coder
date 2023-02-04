package gitauth

import (
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/oauth2"

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

// ConvertConfig converts the YAML configuration entry to the
// parsed and ready-to-consume provider type.
func ConvertConfig(entries []codersdk.GitAuthConfig, accessURL *url.URL) ([]*Config, error) {
	ids := map[string]struct{}{}
	configs := []*Config{}
	for _, entry := range entries {
		var typ codersdk.GitProvider
		switch entry.Type {
		case codersdk.GitProviderAzureDevops:
			typ = codersdk.GitProviderAzureDevops
		case codersdk.GitProviderBitBucket:
			typ = codersdk.GitProviderBitBucket
		case codersdk.GitProviderGitHub:
			typ = codersdk.GitProviderGitHub
		case codersdk.GitProviderGitLab:
			typ = codersdk.GitProviderGitLab
		default:
			return nil, fmt.Errorf("unknown git provider type: %q", entry.Type)
		}
		if entry.ID == "" {
			// Default to the type.
			entry.ID = string(typ)
		}
		if valid := httpapi.NameValid(entry.ID); valid != nil {
			return nil, fmt.Errorf("git auth provider %q doesn't have a valid id: %w", entry.ID, valid)
		}

		_, exists := ids[entry.ID]
		if exists {
			if entry.ID == string(typ) {
				return nil, fmt.Errorf("multiple %s git auth providers provided. you must specify a unique id for each", typ)
			}
			return nil, fmt.Errorf("multiple git providers exist with the id %q. specify a unique id for each", entry.ID)
		}
		ids[entry.ID] = struct{}{}

		if entry.ClientID == "" {
			return nil, fmt.Errorf("%q git auth provider: client_id must be provided", entry.ID)
		}
		if entry.ClientSecret == "" {
			return nil, fmt.Errorf("%q git auth provider: client_secret must be provided", entry.ID)
		}
		authRedirect, err := accessURL.Parse(fmt.Sprintf("/gitauth/%s/callback", entry.ID))
		if err != nil {
			return nil, fmt.Errorf("parse gitauth callback url: %w", err)
		}
		regex := regex[typ]
		if entry.Regex != "" {
			regex, err = regexp.Compile(entry.Regex)
			if err != nil {
				return nil, fmt.Errorf("compile regex for git auth provider %v %q: %w", entry.ID, entry.Regex, err)
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
