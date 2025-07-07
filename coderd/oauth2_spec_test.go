package coderd_test

import (
	"context"
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

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// OAuth2TestSetup contains common setup for OAuth2 tests
type OAuth2TestSetup struct {
	Client       *codersdk.Client
	Owner        codersdk.CreateFirstUserResponse
	Config       *oauth2.Config
	Metadata     codersdk.OAuth2AuthorizationServerMetadata
	Registration codersdk.OAuth2ClientRegistrationResponse
}

// setupOAuth2Test creates a common setup for OAuth2 tests
func setupOAuth2Test(t *testing.T) OAuth2TestSetup {
	t.Helper()

	cfg := coderdtest.DeploymentValues(t)
	cfg.Experiments = []string{"oauth2"}
	client := coderdtest.New(t, &coderdtest.Options{
		DeploymentValues: cfg,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Step 1: Discover OAuth2 authorization server metadata (RFC 8414)
	metadata, err := client.GetOAuth2AuthorizationServerMetadata(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, metadata.AuthorizationEndpoint)
	require.NotEmpty(t, metadata.TokenEndpoint)
	require.NotEmpty(t, metadata.DeviceAuthorizationEndpoint)

	// Step 2: Dynamically register OAuth2 client (RFC 7591)
	registrationReq := codersdk.OAuth2ClientRegistrationRequest{
		ClientName:    fmt.Sprintf("spec-test-%d", time.Now().UnixNano()%1000000),
		RedirectURIs:  []string{"http://localhost:8080/callback"},
		GrantTypes:    []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		ResponseTypes: []string{"code"},
	}

	registrationResp, err := client.PostOAuth2ClientRegistration(ctx, registrationReq)
	require.NoError(t, err)
	require.NotEmpty(t, registrationResp.ClientID)
	require.NotEmpty(t, registrationResp.ClientSecret)

	// Step 3: Create OAuth2 configuration using discovered endpoints
	config := &oauth2.Config{
		ClientID:     registrationResp.ClientID,
		ClientSecret: registrationResp.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:       metadata.AuthorizationEndpoint,
			TokenURL:      metadata.TokenEndpoint,
			DeviceAuthURL: metadata.DeviceAuthorizationEndpoint,
			AuthStyle:     oauth2.AuthStyleInParams,
		},
		RedirectURL: registrationResp.RedirectURIs[0],
		Scopes:      []string{},
	}

	return OAuth2TestSetup{
		Client:       client,
		Owner:        owner,
		Config:       config,
		Metadata:     metadata,
		Registration: registrationResp,
	}
}

// setupUserForOAuth2Test creates a user client for OAuth2 tests
func setupUserForOAuth2Test(t *testing.T, client *codersdk.Client, orgID uuid.UUID) (*codersdk.Client, codersdk.User) {
	t.Helper()
	return coderdtest.CreateAnotherUser(t, client, orgID)
}

func TestOAuth2AuthorizationCodeStandardFlow(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, user := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Step 1: Generate authorization URL
	state := uuid.NewString()
	authURL := setup.Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	require.Contains(t, authURL, "response_type=code")
	require.Contains(t, authURL, "client_id="+setup.Config.ClientID)
	require.Contains(t, authURL, "state="+state)

	// Step 2: User visits authorization URL and grants consent
	// In a real scenario, user would visit authURL in their browser
	// For testing, we programmatically authorize and extract the code
	code := authorizeCodeFlowForUser(t, userClient, authURL)
	require.NotEmpty(t, code)

	// Step 3: Exchange code for token using standard library
	token, err := setup.Config.Exchange(ctx, code)
	require.NoError(t, err)
	require.NotEmpty(t, token.AccessToken)
	require.NotEmpty(t, token.RefreshToken)
	require.Equal(t, "Bearer", token.TokenType)
	require.True(t, time.Now().Before(token.Expiry))

	// Step 4: Verify token works by making an authenticated API call
	verifyOAuth2Token(t, setup.Client, token, user.ID)
}

func TestOAuth2AuthorizationCodePKCEFlow(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, user := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Test PKCE (Proof Key for Code Exchange) - RFC 7636
	verifier := oauth2.GenerateVerifier()

	// Debug PKCE values
	t.Logf("PKCE Test Values:")
	t.Logf("  verifier:  %q", verifier)

	state := uuid.NewString()
	authURL := setup.Config.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier), // Pass verifier, not challenge
	)

	code := authorizeCodeFlowForUser(t, userClient, authURL)

	// Exchange with PKCE verifier
	t.Logf("Exchanging code with verifier: %q", verifier)
	token, err := setup.Config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	require.NoError(t, err)
	require.NotEmpty(t, token.AccessToken)

	verifyOAuth2Token(t, setup.Client, token, user.ID)
}

func TestOAuth2AuthorizationCodeInvalidRedirectURI(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, _ := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	invalidConfig := *setup.Config
	invalidConfig.RedirectURL = "http://evil.com/callback"

	state := uuid.NewString()
	authURL := invalidConfig.AuthCodeURL(state)

	// Parse the authorization URL to extract parameters
	parsedURL, err := url.Parse(authURL)
	require.NoError(t, err)
	query := parsedURL.Query()

	// Filter out access_type parameter
	query.Del("access_type")

	// Make direct request to test invalid redirect URI error
	serverAuthURL := userClient.URL.String() + "/oauth2/authorize?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", serverAuthURL, nil)
	require.NoError(t, err)
	req.Header.Set("Coder-Session-Token", userClient.SessionToken())

	// Don't follow redirects
	userClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := userClient.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Expect HTTP 400 due to invalid redirect URI
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "Server should reject invalid redirect URI")
}

func TestOAuth2AuthorizationCodeInvalidClientSecret(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, _ := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	invalidConfig := *setup.Config
	invalidConfig.ClientSecret = "invalid_secret"

	state := uuid.NewString()
	authURL := setup.Config.AuthCodeURL(state) // Use valid config for auth
	code := authorizeCodeFlowForUser(t, userClient, authURL)

	// Exchange should fail with invalid client secret
	_, err := invalidConfig.Exchange(ctx, code)
	require.Error(t, err)
	var oauth2Err *oauth2.RetrieveError
	require.ErrorAs(t, err, &oauth2Err)
	require.Equal(t, http.StatusUnauthorized, oauth2Err.Response.StatusCode)
}

func TestOAuth2DeviceCodeStandardFlow(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, user := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Step 1: Request device authorization using standard library
	deviceAuth, err := setup.Config.DeviceAuth(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, deviceAuth.DeviceCode)
	require.NotEmpty(t, deviceAuth.UserCode)
	require.NotEmpty(t, deviceAuth.VerificationURI)
	require.Greater(t, deviceAuth.Interval, time.Duration(0))

	// Verify device code format matches our implementation
	require.True(t, strings.HasPrefix(deviceAuth.DeviceCode, "cdr_device_"))

	// Step 2: Simulate user visiting verification URI and authorizing device
	// In a real scenario, user would visit deviceAuth.VerificationURI
	// For testing, we programmatically authorize using the server API
	authorizeDeviceCodeForUser(t, userClient, deviceAuth.UserCode)

	// Step 3: Poll for token using standard library
	token, err := setup.Config.DeviceAccessToken(ctx, deviceAuth)
	require.NoError(t, err)
	require.NotEmpty(t, token.AccessToken)
	require.Equal(t, "Bearer", token.TokenType)

	// Step 4: Verify token works by making an authenticated API call
	verifyOAuth2Token(t, setup.Client, token, user.ID)
}

func TestOAuth2DeviceCodeAuthorizationPending(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Test that polling before user authorization returns authorization_pending
	deviceAuth, err := setup.Config.DeviceAuth(ctx)
	require.NoError(t, err)

	// Try to get token before authorization - should timeout with authorization_pending
	pollCtx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow)
	defer cancel()

	_, err = setup.Config.DeviceAccessToken(pollCtx, deviceAuth)
	require.Error(t, err)
	// The oauth2 library should return context deadline exceeded when polling times out
	require.Contains(t, err.Error(), "context deadline exceeded")
}

func TestOAuth2DeviceCodeAccessDenied(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, _ := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Test user denying authorization
	deviceAuth, err := setup.Config.DeviceAuth(ctx)
	require.NoError(t, err)

	// Simulate user denying authorization
	denyDeviceCodeForUser(t, userClient, deviceAuth.UserCode)

	// Poll should return access_denied
	_, err = setup.Config.DeviceAccessToken(ctx, deviceAuth)
	require.Error(t, err)
	var oauth2Err *oauth2.RetrieveError
	require.ErrorAs(t, err, &oauth2Err)

	var errorResp struct {
		Error string `json:"error"`
	}
	err = json.Unmarshal(oauth2Err.Body, &errorResp)
	require.NoError(t, err)
	require.Equal(t, "access_denied", errorResp.Error)
}

func TestOAuth2DeviceCodeInvalidDeviceCode(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Test with invalid device code
	invalidDeviceAuth := &oauth2.DeviceAuthResponse{
		DeviceCode:      "invalid_device_code",
		UserCode:        "INVALID",
		VerificationURI: setup.Config.Endpoint.DeviceAuthURL,
		Interval:        5,
	}

	_, err := setup.Config.DeviceAccessToken(ctx, invalidDeviceAuth)
	require.Error(t, err)
	var oauth2Err *oauth2.RetrieveError
	require.ErrorAs(t, err, &oauth2Err)
	require.Equal(t, http.StatusBadRequest, oauth2Err.Response.StatusCode)
}

func TestOAuth2RefreshTokenValidRefresh(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, user := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Get initial token through authorization code flow
	state := uuid.NewString()
	authURL := setup.Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	code := authorizeCodeFlowForUser(t, userClient, authURL)

	originalToken, err := setup.Config.Exchange(ctx, code)
	require.NoError(t, err)
	require.NotEmpty(t, originalToken.RefreshToken)

	// Force token to be expired to trigger refresh
	expiredToken := *originalToken
	expiredToken.Expiry = time.Now().Add(-time.Hour) // Make token expired

	// Use standard library to refresh token
	tokenSource := setup.Config.TokenSource(ctx, &expiredToken)
	newToken, err := tokenSource.Token()
	require.NoError(t, err)
	require.NotEmpty(t, newToken.AccessToken)
	require.NotEqual(t, originalToken.AccessToken, newToken.AccessToken)

	// Verify new token works
	verifyOAuth2Token(t, setup.Client, newToken, user.ID)
}

func TestOAuth2RefreshTokenInvalidRefreshToken(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	userClient, _ := setupUserForOAuth2Test(t, setup.Client, setup.Owner.OrganizationID)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Get initial token through authorization code flow
	state := uuid.NewString()
	authURL := setup.Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	code := authorizeCodeFlowForUser(t, userClient, authURL)

	originalToken, err := setup.Config.Exchange(ctx, code)
	require.NoError(t, err)
	require.NotEmpty(t, originalToken.RefreshToken)

	invalidToken := *originalToken
	invalidToken.RefreshToken = "invalid_refresh_token"
	invalidToken.Expiry = time.Now().Add(-time.Hour) // Make token expired to force refresh

	tokenSource := setup.Config.TokenSource(ctx, &invalidToken)
	_, err = tokenSource.Token()
	require.Error(t, err)
	var oauth2Err *oauth2.RetrieveError
	require.ErrorAs(t, err, &oauth2Err)
	require.Equal(t, http.StatusBadRequest, oauth2Err.Response.StatusCode)
}

func TestOAuth2ErrorHandlingInvalidClientID(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	invalidConfig := *setup.Config
	invalidConfig.ClientID = uuid.NewString()

	_, err := invalidConfig.DeviceAuth(ctx)
	require.Error(t, err)
	var oauth2Err *oauth2.RetrieveError
	require.ErrorAs(t, err, &oauth2Err)
	require.Equal(t, http.StatusBadRequest, oauth2Err.Response.StatusCode)

	var errorResp struct {
		Error string `json:"error"`
	}
	err = json.Unmarshal(oauth2Err.Body, &errorResp)
	require.NoError(t, err)
	require.Equal(t, "invalid_client", errorResp.Error)
}

func TestOAuth2ErrorHandlingUnsupportedGrantType(t *testing.T) {
	t.Parallel()

	setup := setupOAuth2Test(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Test unsupported grant type by making raw request
	data := url.Values{}
	data.Set("grant_type", "unsupported_grant")
	data.Set("client_id", setup.Config.ClientID)
	data.Set("client_secret", setup.Config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", setup.Config.Endpoint.TokenURL,
		strings.NewReader(data.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errorResp struct {
		Error string `json:"error"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)
	require.Equal(t, "unsupported_grant_type", errorResp.Error)
}

// Helper functions that exclusively use the oauth2 library as requested

// authorizeCodeFlowForUser handles the server-side authorization for the authorization code flow
// This simulates the user visiting the authorization URL and granting consent
func authorizeCodeFlowForUser(t *testing.T, userClient *codersdk.Client, authURL string) string {
	ctx := testutil.Context(t, testutil.WaitLong)

	// Parse the authorization URL to extract parameters
	parsedURL, err := url.Parse(authURL)
	require.NoError(t, err)
	query := parsedURL.Query()
	state := query.Get("state")

	// Note: access_type=offline parameter is now supported by Coder's OAuth2 provider
	// No need to filter it out as it's properly handled according to OAuth2 specification

	// Set up client to not follow redirects automatically so we can capture the authorization code
	userClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Use the fixed OAuth2 authorize endpoint path as per the working oauth2providertest helpers
	// The discovery metadata points to this same endpoint, but we use it directly for simplicity
	serverAuthURL := userClient.URL.String() + "/oauth2/authorize?" + query.Encode()

	// Simulate the user authorizing the request (POST to the authorization endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", serverAuthURL, nil)
	require.NoError(t, err)

	// Use the correct session token header format (matching oauth2providertest helpers)
	req.Header.Set("Coder-Session-Token", userClient.SessionToken())

	resp, err := userClient.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Accept both 302 (Found) and 307 (Temporary Redirect) as valid redirect status codes
	require.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect,
		"Expected redirect after authorization, got %d", resp.StatusCode)

	// Extract the authorization code from the redirect location
	location := resp.Header.Get("Location")
	redirectURL, err := url.Parse(location)
	require.NoError(t, err)

	code := redirectURL.Query().Get("code")
	require.NotEmpty(t, code, "Authorization code should be present in redirect URL")

	// Verify state parameter is preserved
	if state != "" {
		require.Equal(t, state, redirectURL.Query().Get("state"), "State parameter should be preserved")
	}

	return code
}

// authorizeDeviceCodeForUser handles the server-side authorization for the device code flow
// This simulates the user visiting the verification URI and authorizing the device
func authorizeDeviceCodeForUser(t *testing.T, userClient *codersdk.Client, userCode string) {
	ctx := testutil.Context(t, testutil.WaitLong)

	// Use the client SDK method for device verification
	req := codersdk.OAuth2DeviceVerificationRequest{
		UserCode: userCode,
	}

	err := userClient.PostOAuth2DeviceVerification(ctx, req, "authorize")
	require.NoError(t, err, "Device authorization should succeed")
}

// denyDeviceCodeForUser handles the server-side denial for the device code flow
// This simulates the user visiting the verification URI and denying the device
func denyDeviceCodeForUser(t *testing.T, userClient *codersdk.Client, userCode string) {
	ctx := testutil.Context(t, testutil.WaitLong)

	// Use the client SDK method for device verification
	req := codersdk.OAuth2DeviceVerificationRequest{
		UserCode: userCode,
	}

	err := userClient.PostOAuth2DeviceVerification(ctx, req, "deny")
	require.NoError(t, err, "Device denial should succeed")
}

// verifyOAuth2Token verifies that an OAuth2 token works by making an authenticated API call
// This uses the standard oauth2.Token type and verifies it grants access to the expected user
func verifyOAuth2Token(t *testing.T, baseClient *codersdk.Client, token *oauth2.Token, expectedUserID uuid.UUID) {
	ctx := testutil.Context(t, testutil.WaitLong)

	// Create a new client with the OAuth2 token
	tokenClient := codersdk.New(baseClient.URL)
	tokenClient.SetSessionToken(token.AccessToken)

	// Verify token works by making an API call
	user, err := tokenClient.User(ctx, codersdk.Me)
	require.NoError(t, err, "Token should allow API access")
	require.Equal(t, expectedUserID, user.ID, "Token should grant access to the expected user")

	// Additional verification: ensure token type and expiry are set correctly
	require.Equal(t, "Bearer", token.TokenType, "Token type should be Bearer")
	require.True(t, time.Now().Before(token.Expiry), "Token should not be expired")
}
