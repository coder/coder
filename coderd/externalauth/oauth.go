package externalauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

// endpoint contains default SaaS URLs for each Git provider.
var endpoint = map[codersdk.ExternalAuthProvider]oauth2.Endpoint{
	codersdk.ExternalAuthProviderAzureDevops: {
		AuthURL:  "https://app.vssps.visualstudio.com/oauth2/authorize",
		TokenURL: "https://app.vssps.visualstudio.com/oauth2/token",
	},
	codersdk.ExternalAuthProviderBitBucket: {
		AuthURL:  "https://bitbucket.org/site/oauth2/authorize",
		TokenURL: "https://bitbucket.org/site/oauth2/access_token",
	},
	codersdk.ExternalAuthProviderGitLab: {
		AuthURL:  "https://gitlab.com/oauth/authorize",
		TokenURL: "https://gitlab.com/oauth/token",
	},
	codersdk.ExternalAuthProviderGitHub: github.Endpoint,
}

// validateURL contains defaults for each provider.
var validateURL = map[codersdk.ExternalAuthProvider]string{
	codersdk.ExternalAuthProviderGitHub:    "https://api.github.com/user",
	codersdk.ExternalAuthProviderGitLab:    "https://gitlab.com/oauth/token/info",
	codersdk.ExternalAuthProviderBitBucket: "https://api.bitbucket.org/2.0/user",
}

var deviceAuthURL = map[codersdk.ExternalAuthProvider]string{
	codersdk.ExternalAuthProviderGitHub: "https://github.com/login/device/code",
}

var appInstallationsURL = map[codersdk.ExternalAuthProvider]string{
	codersdk.ExternalAuthProviderGitHub: "https://api.github.com/user/installations",
}

// scope contains defaults for each Git provider.
var scope = map[codersdk.ExternalAuthProvider][]string{
	codersdk.ExternalAuthProviderAzureDevops: {"vso.code_write"},
	codersdk.ExternalAuthProviderBitBucket:   {"account", "repository:write"},
	codersdk.ExternalAuthProviderGitLab:      {"write_repository"},
	// "workflow" is required for managing GitHub Actions in a repository.
	codersdk.ExternalAuthProviderGitHub: {"repo", "workflow"},
}

// regex provides defaults for each Git provider to match their SaaS host URL.
// This is configurable by each provider.
var regex = map[codersdk.ExternalAuthProvider]*regexp.Regexp{
	codersdk.ExternalAuthProviderAzureDevops: regexp.MustCompile(`^(https?://)?dev\.azure\.com(/.*)?$`),
	codersdk.ExternalAuthProviderBitBucket:   regexp.MustCompile(`^(https?://)?bitbucket\.org(/.*)?$`),
	codersdk.ExternalAuthProviderGitLab:      regexp.MustCompile(`^(https?://)?gitlab\.com(/.*)?$`),
	codersdk.ExternalAuthProviderGitHub:      regexp.MustCompile(`^(https?://)?github\.com(/.*)?$`),
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

type DeviceAuth struct {
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
	resp, err := http.DefaultClient.Do(req)
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
		return nil, err
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
	return &oauth2.Token{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		Expiry:       dbtime.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
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
