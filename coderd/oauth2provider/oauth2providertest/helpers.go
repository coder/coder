// Package oauth2providertest provides comprehensive testing utilities for OAuth2 identity provider functionality.
// It includes helpers for creating OAuth2 apps, performing authorization flows, token exchanges,
// PKCE challenge generation and verification, and testing error scenarios.
package oauth2providertest

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// AuthorizeParams contains parameters for OAuth2 authorization
type AuthorizeParams struct {
	ClientID            string
	ResponseType        string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
	Scope               string
}

// TokenExchangeParams contains parameters for token exchange
type TokenExchangeParams struct {
	GrantType    string
	Code         string
	ClientID     string
	ClientSecret string
	CodeVerifier string
	RedirectURI  string
	RefreshToken string
	Resource     string
}

// OAuth2Error represents an OAuth2 error response
type OAuth2Error struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// CreateTestOAuth2App creates an OAuth2 app for testing and returns the app and client secret
func CreateTestOAuth2App(t *testing.T, client *codersdk.Client) (*codersdk.OAuth2ProviderApp, string) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Create unique app name with random suffix
	appName := fmt.Sprintf("test-oauth2-app-%s", testutil.MustRandString(t, 10))

	req := codersdk.PostOAuth2ProviderAppRequest{
		Name:        appName,
		CallbackURL: TestRedirectURI,
	}

	app, err := client.PostOAuth2ProviderApp(ctx, req)
	require.NoError(t, err, "failed to create OAuth2 app")

	// Create client secret
	secret, err := client.PostOAuth2ProviderAppSecret(ctx, app.ID)
	require.NoError(t, err, "failed to create OAuth2 app secret")

	return &app, secret.ClientSecretFull
}

// GeneratePKCE generates a random PKCE code verifier and challenge
func GeneratePKCE(t *testing.T) (verifier, challenge string) {
	t.Helper()

	// Generate 32 random bytes for verifier
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	require.NoError(t, err, "failed to generate random bytes")

	// Create code verifier (base64url encoding without padding)
	verifier = base64.RawURLEncoding.EncodeToString(bytes)

	// Create code challenge using S256 method
	challenge = GenerateCodeChallenge(verifier)

	return verifier, challenge
}

// GenerateState generates a random state parameter
func GenerateState(t *testing.T) string {
	t.Helper()

	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	require.NoError(t, err, "failed to generate random bytes")

	return base64.RawURLEncoding.EncodeToString(bytes)
}

// AuthorizeOAuth2App performs the OAuth2 authorization flow and returns the authorization code
func AuthorizeOAuth2App(t *testing.T, client *codersdk.Client, baseURL string, params AuthorizeParams) string {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Build authorization URL
	authURL, err := url.Parse(baseURL + "/oauth2/authorize")
	require.NoError(t, err, "failed to parse authorization URL")

	query := url.Values{}
	query.Set("client_id", params.ClientID)
	query.Set("response_type", params.ResponseType)
	query.Set("redirect_uri", params.RedirectURI)
	query.Set("state", params.State)

	if params.CodeChallenge != "" {
		query.Set("code_challenge", params.CodeChallenge)
		query.Set("code_challenge_method", params.CodeChallengeMethod)
	}
	if params.Resource != "" {
		query.Set("resource", params.Resource)
	}
	if params.Scope != "" {
		query.Set("scope", params.Scope)
	}

	authURL.RawQuery = query.Encode()

	// Create POST request to authorize endpoint (simulating user clicking "Allow")
	req, err := http.NewRequestWithContext(ctx, "POST", authURL.String(), nil)
	require.NoError(t, err, "failed to create authorization request")

	// Add session token
	req.Header.Set("Coder-Session-Token", client.SessionToken())

	// Perform request
	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Don't follow redirects, we want to capture the redirect URL
			return http.ErrUseLastResponse
		},
	}

	resp, err := httpClient.Do(req)
	require.NoError(t, err, "failed to perform authorization request")
	defer resp.Body.Close()

	// Should get a redirect response (either 302 Found or 307 Temporary Redirect)
	require.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect,
		"expected redirect response, got %d", resp.StatusCode)

	// Extract redirect URL
	location := resp.Header.Get("Location")
	require.NotEmpty(t, location, "missing Location header in redirect response")

	// Parse redirect URL to extract authorization code
	redirectURL, err := url.Parse(location)
	require.NoError(t, err, "failed to parse redirect URL")

	code := redirectURL.Query().Get("code")
	require.NotEmpty(t, code, "missing authorization code in redirect URL")

	// Verify state parameter
	returnedState := redirectURL.Query().Get("state")
	require.Equal(t, params.State, returnedState, "state parameter mismatch")

	return code
}

// ExchangeCodeForToken exchanges an authorization code for tokens
func ExchangeCodeForToken(t *testing.T, baseURL string, params TokenExchangeParams) *oauth2.Token {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Prepare form data
	data := url.Values{}
	data.Set("grant_type", params.GrantType)

	if params.Code != "" {
		data.Set("code", params.Code)
	}
	if params.ClientID != "" {
		data.Set("client_id", params.ClientID)
	}
	if params.ClientSecret != "" {
		data.Set("client_secret", params.ClientSecret)
	}
	if params.CodeVerifier != "" {
		data.Set("code_verifier", params.CodeVerifier)
	}
	if params.RedirectURI != "" {
		data.Set("redirect_uri", params.RedirectURI)
	}
	if params.RefreshToken != "" {
		data.Set("refresh_token", params.RefreshToken)
	}
	if params.Resource != "" {
		data.Set("resource", params.Resource)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/oauth2/tokens", strings.NewReader(data.Encode()))
	require.NoError(t, err, "failed to create token request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Perform request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "failed to perform token request")
	defer resp.Body.Close()

	// Parse response
	var tokenResp oauth2.Token
	err = json.NewDecoder(resp.Body).Decode(&tokenResp)
	require.NoError(t, err, "failed to decode token response")

	require.NotEmpty(t, tokenResp.AccessToken, "missing access token")
	require.Equal(t, "Bearer", tokenResp.TokenType, "unexpected token type")

	return &tokenResp
}

// RequireOAuth2Error checks that the HTTP response contains an expected OAuth2 error
func RequireOAuth2Error(t *testing.T, resp *http.Response, expectedError string) {
	t.Helper()

	var errorResp OAuth2Error
	err := json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "failed to decode error response")

	require.Equal(t, expectedError, errorResp.Error, "unexpected OAuth2 error code")
	require.NotEmpty(t, errorResp.ErrorDescription, "missing error description")
}

// PerformTokenExchangeExpectingError performs a token exchange expecting an OAuth2 error
func PerformTokenExchangeExpectingError(t *testing.T, baseURL string, params TokenExchangeParams, expectedError string) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Prepare form data
	data := url.Values{}
	data.Set("grant_type", params.GrantType)

	if params.Code != "" {
		data.Set("code", params.Code)
	}
	if params.ClientID != "" {
		data.Set("client_id", params.ClientID)
	}
	if params.ClientSecret != "" {
		data.Set("client_secret", params.ClientSecret)
	}
	if params.CodeVerifier != "" {
		data.Set("code_verifier", params.CodeVerifier)
	}
	if params.RedirectURI != "" {
		data.Set("redirect_uri", params.RedirectURI)
	}
	if params.RefreshToken != "" {
		data.Set("refresh_token", params.RefreshToken)
	}
	if params.Resource != "" {
		data.Set("resource", params.Resource)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/oauth2/tokens", strings.NewReader(data.Encode()))
	require.NoError(t, err, "failed to create token request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Perform request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "failed to perform token request")
	defer resp.Body.Close()

	// Should be a 4xx error
	require.True(t, resp.StatusCode >= 400 && resp.StatusCode < 500, "expected 4xx status code, got %d", resp.StatusCode)

	// Check OAuth2 error
	RequireOAuth2Error(t, resp, expectedError)
}

// FetchOAuth2Metadata fetches and returns OAuth2 authorization server metadata
func FetchOAuth2Metadata(t *testing.T, baseURL string) map[string]any {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/.well-known/oauth-authorization-server", nil)
	require.NoError(t, err, "failed to create metadata request")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "failed to fetch metadata")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected metadata response status")

	var metadata map[string]any
	err = json.NewDecoder(resp.Body).Decode(&metadata)
	require.NoError(t, err, "failed to decode metadata response")

	return metadata
}

// CleanupOAuth2App deletes an OAuth2 app (helper for test cleanup)
func CleanupOAuth2App(t *testing.T, client *codersdk.Client, appID uuid.UUID) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	err := client.DeleteOAuth2ProviderApp(ctx, appID)
	if err != nil {
		t.Logf("Warning: failed to cleanup OAuth2 app %s: %v", appID, err)
	}
}
