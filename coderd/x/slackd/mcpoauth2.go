package slackd

// This file duplicates the MCP OAuth2 discovery and Dynamic Client
// Registration flow from coderd/mcp.go. The logic is duplicated to
// avoid importing coderd from slackd (coderd imports slackd); keep the
// two in sync.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/xerrors"
)

// mcpOAuth2Discovery holds the result of MCP OAuth2 auto-discovery
// and Dynamic Client Registration.
type mcpOAuth2Discovery struct {
	clientID     string
	clientSecret string
	authURL      string
	tokenURL     string
	scopes       string // space-separated
}

// protectedResourceMetadata represents the response from a
// Protected Resource Metadata endpoint per RFC 9728 §2.
type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported,omitempty"`
}

// authServerMetadata represents the response from an Authorization
// Server Metadata endpoint per RFC 8414 §2.
type authServerMetadata struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint,omitempty"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
}

func mcpOAuth2HTTPClient(httpClient *http.Client) *http.Client {
	if httpClient != nil {
		return httpClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// fetchJSON performs a GET request to the given URL with the
// standard MCP OAuth2 discovery headers and decodes the JSON
// response into dest.
func fetchJSON(ctx context.Context, httpClient *http.Client, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return xerrors.Errorf("create request for %s: %w", rawURL, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.LATEST_PROTOCOL_VERSION)

	resp, err := httpClient.Do(req)
	if err != nil {
		return xerrors.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return xerrors.Errorf("GET %s returned HTTP %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return xerrors.Errorf("read response from %s: %w", rawURL, err)
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return xerrors.Errorf("decode JSON from %s: %w", rawURL, err)
	}

	return nil
}

// discoverProtectedResource discovers the Protected Resource
// Metadata for the given MCP server per RFC 9728 §3.1. It tries the
// path-aware well-known URL first, then falls back to the root-level
// URL.
func discoverProtectedResource(
	ctx context.Context, httpClient *http.Client, origin, path string,
) (*protectedResourceMetadata, error) {
	var urls []string

	// Per RFC 9728 §3.1, when the resource URL contains a path
	// component, the well-known URI is constructed by inserting the
	// well-known prefix before the path.
	if path != "" && path != "/" {
		urls = append(urls, origin+"/.well-known/oauth-protected-resource"+path)
	}
	// Always try the root-level URL as a fallback.
	urls = append(urls, origin+"/.well-known/oauth-protected-resource")

	var lastErr error
	for _, u := range urls {
		var meta protectedResourceMetadata
		if err := fetchJSON(ctx, httpClient, u, &meta); err != nil {
			lastErr = err
			continue
		}
		if len(meta.AuthorizationServers) == 0 {
			lastErr = xerrors.Errorf("protected resource metadata at %s has no authorization_servers", u)
			continue
		}
		return &meta, nil
	}

	return nil, xerrors.Errorf("discover protected resource metadata: %w", lastErr)
}

// discoverAuthServerMetadata discovers the Authorization Server
// Metadata per RFC 8414 §3.1, falling back to root-level and OpenID
// Connect discovery.
func discoverAuthServerMetadata(
	ctx context.Context, httpClient *http.Client, authServerURL string,
) (*authServerMetadata, error) {
	parsed, err := url.Parse(authServerURL)
	if err != nil {
		return nil, xerrors.Errorf("parse auth server URL: %w", err)
	}
	asOrigin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	asPath := parsed.Path

	var urls []string

	// Per RFC 8414 §3.1, if the issuer URL has a path, insert the
	// well-known prefix before the path.
	if asPath != "" && asPath != "/" {
		urls = append(urls, asOrigin+"/.well-known/oauth-authorization-server"+asPath)
	}
	// Root-level fallback.
	urls = append(urls, asOrigin+"/.well-known/oauth-authorization-server")
	// OpenID Connect discovery as a last resort. Per OpenID Connect
	// Discovery 1.0 §4, the well-known URL is formed by appending to
	// the full issuer (including path), not just the origin.
	urls = append(urls, strings.TrimRight(authServerURL, "/")+"/.well-known/openid-configuration")

	var lastErr error
	for _, u := range urls {
		var meta authServerMetadata
		if err := fetchJSON(ctx, httpClient, u, &meta); err != nil {
			lastErr = err
			continue
		}
		if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
			lastErr = xerrors.Errorf("auth server metadata at %s missing required endpoints", u)
			continue
		}
		return &meta, nil
	}

	return nil, xerrors.Errorf("discover auth server metadata: %w", lastErr)
}

// registerOAuth2Client performs Dynamic Client Registration per
// RFC 7591 by POSTing client metadata to the registration endpoint and
// returning the assigned client_id and optional client_secret.
func registerOAuth2Client(
	ctx context.Context, httpClient *http.Client,
	registrationEndpoint, callbackURL, clientName string,
) (clientID string, clientSecret string, err error) {
	payload := map[string]any{
		"client_name":                clientName,
		"redirect_uris":              []string{callbackURL},
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", xerrors.Errorf("marshal registration request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", xerrors.Errorf("create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", xerrors.Errorf("POST %s: %w", registrationEndpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", xerrors.Errorf("read registration response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Truncate to avoid leaking verbose upstream errors through
		// the API.
		const maxErrBody = 512
		errMsg := string(respBody)
		if len(errMsg) > maxErrBody {
			errMsg = errMsg[:maxErrBody] + "..."
		}
		return "", "", xerrors.Errorf("registration endpoint returned HTTP %d: %s", resp.StatusCode, errMsg)
	}

	var result struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", xerrors.Errorf("decode registration response: %w", err)
	}
	if result.ClientID == "" {
		return "", "", xerrors.New("registration response missing client_id")
	}

	return result.ClientID, result.ClientSecret, nil
}

func discoverMCPOAuth2Metadata(ctx context.Context, httpClient *http.Client, mcpServerURL string) (*authServerMetadata, error) {
	parsed, err := url.Parse(mcpServerURL)
	if err != nil {
		return nil, xerrors.Errorf("parse MCP server URL: %w", err)
	}
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	path := parsed.Path

	prm, err := discoverProtectedResource(ctx, httpClient, origin, path)
	if err != nil {
		return nil, xerrors.Errorf("protected resource discovery: %w", err)
	}

	asMeta, err := discoverAuthServerMetadata(ctx, httpClient, prm.AuthorizationServers[0])
	if err != nil {
		return nil, xerrors.Errorf("auth server metadata discovery: %w", err)
	}

	if asMeta.RegistrationEndpoint == "" {
		return nil, xerrors.New("authorization server does not advertise a registration_endpoint (dynamic client registration may not be supported)")
	}
	return asMeta, nil
}

// ValidateMCPServerOAuth2Discovery verifies that an MCP server's OAuth2
// metadata supports automatic configuration. It performs discovery only and
// does not register an OAuth2 client.
func ValidateMCPServerOAuth2Discovery(ctx context.Context, httpClient *http.Client, mcpServerURL string) error {
	_, err := discoverMCPOAuth2Metadata(ctx, mcpOAuth2HTTPClient(httpClient), mcpServerURL)
	return err
}

// discoverAndRegisterMCPOAuth2 performs MCP OAuth2 discovery and Dynamic
// Client Registration, returning the discovered endpoints and credentials.
func discoverAndRegisterMCPOAuth2(ctx context.Context, httpClient *http.Client, mcpServerURL, callbackURL string) (*mcpOAuth2Discovery, error) {
	asMeta, err := discoverMCPOAuth2Metadata(ctx, httpClient, mcpServerURL)
	if err != nil {
		return nil, err
	}

	clientID, clientSecret, err := registerOAuth2Client(ctx, httpClient, asMeta.RegistrationEndpoint, callbackURL, "Coder")
	if err != nil {
		return nil, xerrors.Errorf("dynamic client registration: %w", err)
	}

	scopes := strings.Join(asMeta.ScopesSupported, " ")

	return &mcpOAuth2Discovery{
		clientID:     clientID,
		clientSecret: clientSecret,
		authURL:      asMeta.AuthorizationEndpoint,
		tokenURL:     asMeta.TokenEndpoint,
		scopes:       scopes,
	}, nil
}
