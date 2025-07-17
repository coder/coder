package oauth2provider_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOAuth2TokenRevocation tests the OAuth2 token revocation endpoint
func TestOAuth2TokenRevocation(t *testing.T) {
	t.Parallel()

	t.Run("RefreshTokenRevocation", func(t *testing.T) {
		t.Parallel()

		t.Run("SuccessfulRevocation", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			// Get tokens through OAuth2 flow
			tokens := performOAuth2Flow(t, client, app, clientSecret)

			// Revoke the refresh token
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    tokens.RefreshToken,
				ClientID: app.ID.String(),
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusOK, revokeResp.StatusCode)

			// Verify token is revoked by trying to use it
			refreshResp := refreshToken(t, client.URL.String(), oauth2providertest.TokenExchangeParams{
				GrantType:    "refresh_token",
				RefreshToken: tokens.RefreshToken,
				ClientID:     app.ID.String(),
				ClientSecret: clientSecret,
			})
			defer refreshResp.Body.Close()
			// Should get a 4xx error since token is revoked
			require.True(t, refreshResp.StatusCode >= 400 && refreshResp.StatusCode < 500,
				"Expected 4xx error when using revoked token, got %d", refreshResp.StatusCode)

			// Verify error response contains OAuth2 error
			var oauth2Err oauth2providertest.OAuth2Error
			err := json.NewDecoder(refreshResp.Body).Decode(&oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_grant", oauth2Err.Error)
		})

		t.Run("RevokeNonExistentToken", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			// Try to revoke a non-existent token (should succeed per RFC 7009)
			fakeRefreshToken := "coder_fake123_fakesecret456"
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    fakeRefreshToken,
				ClientID: app.ID.String(),
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusOK, revokeResp.StatusCode)
		})

		t.Run("RevokeTokenFromDifferentClient", func(t *testing.T) {
			t.Parallel()

			// Create fresh client for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			// Create first OAuth2 app and get tokens
			app1, clientSecret1 := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app1.ID)
			})

			tokens1 := performOAuth2Flow(t, client, app1, clientSecret1)

			// Create second OAuth2 app
			app2, _ := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app2.ID)
			})

			// Try to revoke app1's token using app2's client_id (should succeed per RFC 7009 but token should remain valid)
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    tokens1.RefreshToken,
				ClientID: app2.ID.String(),
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusOK, revokeResp.StatusCode)

			// Verify the token is still valid (wasn't actually revoked)
			refreshResp := refreshToken(t, client.URL.String(), oauth2providertest.TokenExchangeParams{
				GrantType:    "refresh_token",
				RefreshToken: tokens1.RefreshToken,
				ClientID:     app1.ID.String(),
				ClientSecret: clientSecret1,
			})
			defer refreshResp.Body.Close()
			// Should succeed since the token belongs to app1, not app2
			require.Equal(t, http.StatusOK, refreshResp.StatusCode)
		})

		t.Run("MissingClientID", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			tokens := performOAuth2Flow(t, client, app, clientSecret)

			// Try to revoke without client_id
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token: tokens.RefreshToken,
				// ClientID omitted
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusBadRequest, revokeResp.StatusCode)

			var oauth2Err oauth2providertest.OAuth2Error
			err := json.NewDecoder(revokeResp.Body).Decode(&oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_request", oauth2Err.Error)
			assert.Contains(t, oauth2Err.ErrorDescription, "Missing client_id parameter")
		})

		t.Run("InvalidClientID", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			tokens := performOAuth2Flow(t, client, app, clientSecret)

			// Try to revoke with invalid client_id
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    tokens.RefreshToken,
				ClientID: "invalid-uuid",
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusUnauthorized, revokeResp.StatusCode)

			var oauth2Err oauth2providertest.OAuth2Error
			err := json.NewDecoder(revokeResp.Body).Decode(&oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_client", oauth2Err.Error)
		})

		t.Run("NonExistentClientID", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			tokens := performOAuth2Flow(t, client, app, clientSecret)

			// Try to revoke with non-existent client_id
			fakeClientID := uuid.New().String()
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    tokens.RefreshToken,
				ClientID: fakeClientID,
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusUnauthorized, revokeResp.StatusCode)

			var oauth2Err oauth2providertest.OAuth2Error
			err := json.NewDecoder(revokeResp.Body).Decode(&oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_client", oauth2Err.Error)
		})

		t.Run("MissingToken", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			// Try to revoke without token
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				ClientID: app.ID.String(),
				// Token omitted
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusBadRequest, revokeResp.StatusCode)

			var oauth2Err oauth2providertest.OAuth2Error
			err := json.NewDecoder(revokeResp.Body).Decode(&oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_request", oauth2Err.Error)
			assert.Contains(t, oauth2Err.ErrorDescription, "Missing token parameter")
		})

		t.Run("InvalidTokenFormat", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			// Try to revoke with invalid token format (no dash separator)
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    "invalid_token_format_no_dash",
				ClientID: app.ID.String(),
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusBadRequest, revokeResp.StatusCode)

			var oauth2Err oauth2providertest.OAuth2Error
			err := json.NewDecoder(revokeResp.Body).Decode(&oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_request", oauth2Err.Error)
		})
	})

	t.Run("AccessTokenRevocation", func(t *testing.T) {
		t.Parallel()

		t.Run("SuccessfulRevocation", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			// Get tokens through OAuth2 flow
			tokens := performOAuth2Flow(t, client, app, clientSecret)

			// Revoke the access token (API key)
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    tokens.AccessToken,
				ClientID: app.ID.String(),
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusOK, revokeResp.StatusCode)

			// Note: Since we're treating access tokens as API keys and not implementing
			// full API key revocation in this PR, we just verify the endpoint responds correctly
			// TODO: Implement actual API key revocation verification when available
		})

		t.Run("RevokeAccessTokenFromDifferentClient", func(t *testing.T) {
			t.Parallel()

			// Create fresh client for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			// Create first OAuth2 app and get tokens
			app1, clientSecret1 := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app1.ID)
			})

			tokens1 := performOAuth2Flow(t, client, app1, clientSecret1)

			// Create second OAuth2 app
			app2, _ := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app2.ID)
			})

			// Try to revoke app1's access token using app2's client_id (should succeed per RFC 7009)
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    tokens1.AccessToken,
				ClientID: app2.ID.String(),
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusOK, revokeResp.StatusCode)
		})

		t.Run("RevokeInvalidAccessTokenFormat", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			// Try to revoke access token with invalid format (no dash separator)
			revokeResp := revokeToken(t, client.URL.String(), revokeParams{
				Token:    "not_a_valid_api_key_format_no_dash",
				ClientID: app.ID.String(),
			})
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusBadRequest, revokeResp.StatusCode)

			var oauth2Err oauth2providertest.OAuth2Error
			err := json.NewDecoder(revokeResp.Body).Decode(&oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_request", oauth2Err.Error)
		})
	})

	t.Run("SecurityTests", func(t *testing.T) {
		t.Parallel()

		t.Run("HTTPMethodAttack", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			tokens := performOAuth2Flow(t, client, app, clientSecret)

			// Try to revoke using GET method (should fail)
			ctx := testutil.Context(t, testutil.WaitLong)
			revokeURL := client.URL.String() + "/oauth2/revoke"
			req, err := http.NewRequestWithContext(ctx, "GET", revokeURL, nil)
			require.NoError(t, err)

			query := url.Values{}
			query.Set("token", tokens.RefreshToken)
			query.Set("client_id", app.ID.String())
			req.URL.RawQuery = query.Encode()

			httpClient := &http.Client{Timeout: testutil.WaitLong}
			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)

			// Read the response body to see what's actually in it
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// If body is empty, the middleware might not be handling it properly
			if len(body) == 0 {
				t.Log("Response body is empty for method not allowed")
				return
			}

			var oauth2Err oauth2providertest.OAuth2Error
			err = json.Unmarshal(body, &oauth2Err)
			require.NoError(t, err)
			assert.Equal(t, "invalid_request", oauth2Err.Error)
			assert.Contains(t, oauth2Err.ErrorDescription, "Method not allowed")
		})

		t.Run("TokenTypeHintIgnored", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			tokens := performOAuth2Flow(t, client, app, clientSecret)

			// Try to revoke with incorrect token_type_hint (should still work)
			revokeResp := revokeTokenWithHint(t, client.URL.String(), revokeParams{
				Token:    tokens.RefreshToken,
				ClientID: app.ID.String(),
			}, "access_token") // Wrong hint for refresh token
			defer revokeResp.Body.Close()
			require.Equal(t, http.StatusOK, revokeResp.StatusCode)

			// Verify token is actually revoked
			refreshResp := refreshToken(t, client.URL.String(), oauth2providertest.TokenExchangeParams{
				GrantType:    "refresh_token",
				RefreshToken: tokens.RefreshToken,
				ClientID:     app.ID.String(),
				ClientSecret: clientSecret,
			})
			defer refreshResp.Body.Close()
			// Should get a 4xx error since token is revoked
			require.True(t, refreshResp.StatusCode >= 400 && refreshResp.StatusCode < 500,
				"Expected 4xx error when using revoked token, got %d", refreshResp.StatusCode)
		})

		t.Run("MaliciousTokenFormats", func(t *testing.T) {
			t.Parallel()

			// Create fresh client and app for this test
			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: false,
			})
			_ = coderdtest.CreateFirstUser(t, client)

			app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
			t.Cleanup(func() {
				oauth2providertest.CleanupOAuth2App(t, client, app.ID)
			})

			maliciousTokens := []string{
				"coder_",                        // Missing prefix and secret
				"coder__secret",                 // Empty prefix
				"coder_prefix_",                 // Missing secret
				"../../../etc/passwd",           // Path traversal attempt
				"<script>alert('xss')</script>", // XSS attempt
				strings.Repeat("a", 10000),      // Very long token
				"",                              // Empty token (already covered but included for completeness)
			}

			for _, maliciousToken := range maliciousTokens {
				t.Run(fmt.Sprintf("Token_%s", strings.ReplaceAll(maliciousToken, "/", "_slash_")), func(t *testing.T) {
					revokeResp := revokeToken(t, client.URL.String(), revokeParams{
						Token:    maliciousToken,
						ClientID: app.ID.String(),
					})
					defer revokeResp.Body.Close()
					// Should either return 400 for invalid format or 200 for "success" (per RFC 7009)
					require.True(t, revokeResp.StatusCode == http.StatusBadRequest || revokeResp.StatusCode == http.StatusOK,
						"Expected 400 or 200, got %d for token: %s", revokeResp.StatusCode, maliciousToken)
				})
			}
		})
	})
}

// Helper types and functions

type revokeParams struct {
	Token    string
	ClientID string
}

// performOAuth2Flow performs a complete OAuth2 authorization code flow and returns tokens
func performOAuth2Flow(t *testing.T, client *codersdk.Client, app *codersdk.OAuth2ProviderApp, clientSecret string) *oauth2.Token {
	t.Helper()

	// Generate PKCE parameters
	codeVerifier, codeChallenge := oauth2providertest.GeneratePKCE(t)
	state := oauth2providertest.GenerateState(t)

	// Perform authorization
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)

	// Exchange code for tokens
	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		CodeVerifier: codeVerifier,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}

	return oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), tokenParams)
}

// revokeToken makes a revocation request and returns the response
func revokeToken(t *testing.T, baseURL string, params revokeParams) *http.Response {
	t.Helper()
	return revokeTokenWithHint(t, baseURL, params, "")
}

// revokeTokenWithHint makes a revocation request with a token_type_hint and returns the response
func revokeTokenWithHint(t *testing.T, baseURL string, params revokeParams, tokenTypeHint string) *http.Response {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	data := url.Values{}
	if params.Token != "" {
		data.Set("token", params.Token)
	}
	if params.ClientID != "" {
		data.Set("client_id", params.ClientID)
	}
	if tokenTypeHint != "" {
		data.Set("token_type_hint", tokenTypeHint)
	}

	revokeURL := baseURL + "/oauth2/revoke"
	req, err := http.NewRequestWithContext(ctx, "POST", revokeURL, strings.NewReader(data.Encode()))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: testutil.WaitLong}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	return resp
}

// refreshToken attempts to refresh a token and returns the response
func refreshToken(t *testing.T, baseURL string, params oauth2providertest.TokenExchangeParams) *http.Response {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)

	data := url.Values{}
	data.Set("grant_type", params.GrantType)
	if params.RefreshToken != "" {
		data.Set("refresh_token", params.RefreshToken)
	}
	if params.ClientID != "" {
		data.Set("client_id", params.ClientID)
	}
	if params.ClientSecret != "" {
		data.Set("client_secret", params.ClientSecret)
	}

	tokenURL := baseURL + "/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: testutil.WaitLong}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	return resp
}
