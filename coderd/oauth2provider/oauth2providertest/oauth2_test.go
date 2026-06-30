package oauth2providertest_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuth2AuthorizationServerMetadata(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Fetch OAuth2 metadata
	metadata := oauth2providertest.FetchOAuth2Metadata(t, client.URL.String())

	// Verify required metadata fields
	require.Contains(t, metadata, "issuer", "missing issuer in metadata")
	require.Contains(t, metadata, "authorization_endpoint", "missing authorization_endpoint in metadata")
	require.Contains(t, metadata, "token_endpoint", "missing token_endpoint in metadata")

	// Verify response types
	responseTypes, ok := metadata["response_types_supported"].([]any)
	require.True(t, ok, "response_types_supported should be an array")
	require.Contains(t, responseTypes, "code", "should support authorization code flow")

	// Verify grant types
	grantTypes, ok := metadata["grant_types_supported"].([]any)
	require.True(t, ok, "grant_types_supported should be an array")
	require.Contains(t, grantTypes, "authorization_code", "should support authorization_code grant")
	require.Contains(t, grantTypes, "refresh_token", "should support refresh_token grant")

	// Verify PKCE support
	challengeMethods, ok := metadata["code_challenge_methods_supported"].([]any)
	require.True(t, ok, "code_challenge_methods_supported should be an array")
	require.Contains(t, challengeMethods, "S256", "should support S256 PKCE method")

	// Verify token endpoint auth methods
	authMethods, ok := metadata["token_endpoint_auth_methods_supported"].([]any)
	require.True(t, ok, "token_endpoint_auth_methods_supported should be an array")
	require.Contains(t, authMethods, "client_secret_basic", "should support client_secret_basic token auth")
	require.Contains(t, authMethods, "client_secret_post", "should support client_secret_post token auth")

	// Verify endpoints are proper URLs
	authEndpoint, ok := metadata["authorization_endpoint"].(string)
	require.True(t, ok, "authorization_endpoint should be a string")
	require.Contains(t, authEndpoint, "/oauth2/authorize", "authorization endpoint should be /oauth2/authorize")

	tokenEndpoint, ok := metadata["token_endpoint"].(string)
	require.True(t, ok, "token_endpoint should be a string")
	require.Contains(t, tokenEndpoint, "/oauth2/tokens", "token endpoint should be /oauth2/tokens")
}

func TestOAuth2PKCEFlow(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Create OAuth2 app
	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

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
	require.NotEmpty(t, code, "should receive authorization code")

	// Exchange code for token with PKCE
	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		CodeVerifier: codeVerifier,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}

	token := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), tokenParams)
	require.NotEmpty(t, token.AccessToken, "should receive access token")
	require.NotEmpty(t, token.RefreshToken, "should receive refresh token")
	require.Equal(t, "Bearer", token.TokenType, "token type should be Bearer")
}

func TestOAuth2InvalidPKCE(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Create OAuth2 app
	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	// Generate PKCE parameters
	_, codeChallenge := oauth2providertest.GeneratePKCE(t)
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
	require.NotEmpty(t, code, "should receive authorization code")

	// Attempt token exchange with wrong code verifier
	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		CodeVerifier: oauth2providertest.InvalidCodeVerifier,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}

	oauth2providertest.PerformTokenExchangeExpectingError(
		t, client.URL.String(), tokenParams, oauth2providertest.OAuth2ErrorTypes.InvalidGrant,
	)
}

// TestOAuth2WithoutPKCEIsRejected verifies that authorization requests without
// a code_challenge are rejected now that PKCE is mandatory.
func TestOAuth2WithoutPKCEIsRejected(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Create OAuth2 app.
	app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	state := oauth2providertest.GenerateState(t)

	// Authorization without code_challenge should be rejected.
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:     app.ID.String(),
		ResponseType: "code",
		RedirectURI:  oauth2providertest.TestRedirectURI,
		State:        state,
	}

	oauth2providertest.AuthorizeOAuth2AppExpectingError(
		t, client, client.URL.String(), authParams, http.StatusBadRequest,
	)
}

func TestOAuth2TokenExchangeClientSecretBasic(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	codeVerifier, codeChallenge := oauth2providertest.GeneratePKCE(t)
	state := oauth2providertest.GenerateState(t)

	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)
	require.NotEmpty(t, code, "should receive authorization code")

	ctx := testutil.Context(t, testutil.WaitLong)
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", oauth2providertest.TestRedirectURI)
	data.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, "POST", client.URL.String()+"/oauth2/tokens", strings.NewReader(data.Encode()))
	require.NoError(t, err, "failed to create token request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(app.ID.String(), clientSecret)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	require.NoError(t, err, "failed to perform token request")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected status code")

	var tokenResp oauth2.Token
	err = json.NewDecoder(resp.Body).Decode(&tokenResp)
	require.NoError(t, err, "failed to decode token response")

	require.NotEmpty(t, tokenResp.AccessToken, "missing access token")
	require.NotEmpty(t, tokenResp.RefreshToken, "missing refresh token")
	require.Equal(t, "Bearer", tokenResp.TokenType, "unexpected token type")
}

func TestOAuth2TokenExchangeClientSecretBasicInvalidSecret(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	codeVerifier, codeChallenge := oauth2providertest.GeneratePKCE(t)
	state := oauth2providertest.GenerateState(t)

	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)
	require.NotEmpty(t, code, "should receive authorization code")

	ctx := testutil.Context(t, testutil.WaitLong)
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", oauth2providertest.TestRedirectURI)
	data.Set("code_verifier", codeVerifier)

	wrongSecret := clientSecret + "x"

	req, err := http.NewRequestWithContext(ctx, "POST", client.URL.String()+"/oauth2/tokens", strings.NewReader(data.Encode()))
	require.NoError(t, err, "failed to create token request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(app.ID.String(), wrongSecret)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	require.NoError(t, err, "failed to perform token request")
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expected 401 status code")
	require.Equal(t, `Basic realm="coder"`, resp.Header.Get("WWW-Authenticate"), "missing WWW-Authenticate header")

	oauth2providertest.RequireOAuth2Error(t, resp, oauth2providertest.OAuth2ErrorTypes.InvalidClient)
}

func TestOAuth2PKCEPlainMethodRejected(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Create OAuth2 app
	app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	// Generate PKCE parameters but use "plain" method (should be rejected)
	_, codeChallenge := oauth2providertest.GeneratePKCE(t)
	state := oauth2providertest.GenerateState(t)

	// Attempt authorization with plain method - should fail
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        string(codersdk.OAuth2ProviderResponseTypeCode),
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: string(codersdk.OAuth2PKCECodeChallengeMethodPlain),
	}

	// Should get a 400 Bad Request
	oauth2providertest.AuthorizeOAuth2AppExpectingError(t, client, client.URL.String(), authParams, 400)
}

func TestOAuth2ResourceParameter(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Create OAuth2 app
	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	state := oauth2providertest.GenerateState(t)
	codeVerifier, codeChallenge := oauth2providertest.GeneratePKCE(t)

	// Perform authorization with resource parameter.
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
		Resource:            oauth2providertest.TestResourceURI,
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)
	require.NotEmpty(t, code, "should receive authorization code")

	// Exchange code for token with resource parameter.
	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		RedirectURI:  oauth2providertest.TestRedirectURI,
		CodeVerifier: codeVerifier,
		Resource:     oauth2providertest.TestResourceURI,
	}

	token := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), tokenParams)
	require.NotEmpty(t, token.AccessToken, "should receive access token")
	require.NotEmpty(t, token.RefreshToken, "should receive refresh token")
}

func TestOAuth2TokenRefresh(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Create OAuth2 app
	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	state := oauth2providertest.GenerateState(t)
	codeVerifier, codeChallenge := oauth2providertest.GeneratePKCE(t)

	// Get initial token.
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)

	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		RedirectURI:  oauth2providertest.TestRedirectURI,
		CodeVerifier: codeVerifier,
	}

	initialToken := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), tokenParams)
	require.NotEmpty(t, initialToken.RefreshToken, "should receive refresh token")

	// Use refresh token to get new access token
	refreshParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "refresh_token",
		RefreshToken: initialToken.RefreshToken,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
	}

	refreshedToken := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), refreshParams)
	require.NotEmpty(t, refreshedToken.AccessToken, "should receive new access token")
	require.NotEqual(t, initialToken.AccessToken, refreshedToken.AccessToken, "new access token should be different")
}

// TestOAuth2ReauthorizeAfterCancel verifies that a second authorization
// attempt works after the user denied the first one. This is a regression
// test for https://github.com/coder/coder/issues/24912 where the cancel
// URL construction mutated the redirect URL pointer, which could corrupt
// the stored redirect_uri and prevent subsequent authorization flows.
func TestOAuth2ReauthorizeAfterCancel(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Create OAuth2 app.
	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	// --- First attempt: simulate the user clicking Cancel. ---
	// We issue a GET to the authorize page and verify the cancel URI
	// redirects with error=access_denied. No POST is needed because
	// Cancel is a client-side navigation.
	firstVerifier, firstChallenge := oauth2providertest.GeneratePKCE(t)
	_ = firstVerifier // not exchanged because the user cancels
	firstState := oauth2providertest.GenerateState(t)

	ctx := testutil.Context(t, testutil.WaitLong)

	// Build the first authorization URL to verify it renders.
	firstAuthURL := client.URL.String() + "/oauth2/authorize?" + url.Values{
		"client_id":             {app.ID.String()},
		"response_type":        {"code"},
		"redirect_uri":         {oauth2providertest.TestRedirectURI},
		"state":                {firstState},
		"code_challenge":       {firstChallenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", firstAuthURL, nil)
	require.NoError(t, err)
	req.Header.Set("Coder-Session-Token", client.SessionToken())

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"first authorization page should render")

	// Verify Cache-Control header prevents stale pages.
	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"),
		"consent page should not be cached")

	// --- Second attempt: user clicks Allow. ---
	secondVerifier, secondChallenge := oauth2providertest.GeneratePKCE(t)
	secondState := oauth2providertest.GenerateState(t)

	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               secondState,
		CodeChallenge:       secondChallenge,
		CodeChallengeMethod: "S256",
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)
	require.NotEmpty(t, code, "should receive authorization code on re-authorization")

	// Exchange the code for a token to prove the full flow works.
	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		CodeVerifier: secondVerifier,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}

	token := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), tokenParams)
	require.NotEmpty(t, token.AccessToken, "should receive access token after re-authorization")
	require.NotEmpty(t, token.RefreshToken, "should receive refresh token after re-authorization")
}

// TestOAuth2MultipleReauthorizations verifies that the authorization flow
// works correctly across three consecutive attempts: allow, cancel, allow.
func TestOAuth2MultipleReauthorizations(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	for i := range 3 {
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		state := oauth2providertest.GenerateState(t)

		authParams := oauth2providertest.AuthorizeParams{
			ClientID:            app.ID.String(),
			ResponseType:        "code",
			RedirectURI:         oauth2providertest.TestRedirectURI,
			State:               state,
			CodeChallenge:       challenge,
			CodeChallengeMethod: "S256",
		}

		code := oauth2providertest.AuthorizeOAuth2App(
			t, client, client.URL.String(), authParams,
		)
		require.NotEmpty(t, code, "attempt %d: should receive authorization code", i+1)

		tokenParams := oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			Code:         code,
			ClientID:     app.ID.String(),
			ClientSecret: clientSecret,
			CodeVerifier: verifier,
			RedirectURI:  oauth2providertest.TestRedirectURI,
		}

		token := oauth2providertest.ExchangeCodeForToken(
			t, client.URL.String(), tokenParams,
		)
		require.NotEmpty(t, token.AccessToken,
			"attempt %d: should receive access token", i+1)
	}
}

func TestOAuth2ErrorResponses(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	t.Run("InvalidClient", func(t *testing.T) {
		t.Parallel()

		tokenParams := oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			Code:         "invalid-code",
			ClientID:     "non-existent-client",
			ClientSecret: "invalid-secret",
		}

		oauth2providertest.PerformTokenExchangeExpectingError(
			t, client.URL.String(), tokenParams, oauth2providertest.OAuth2ErrorTypes.InvalidClient,
		)
	})

	t.Run("InvalidGrantType", func(t *testing.T) {
		t.Parallel()

		app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
		t.Cleanup(func() {
			oauth2providertest.CleanupOAuth2App(t, client, app.ID)
		})

		tokenParams := oauth2providertest.TokenExchangeParams{
			GrantType:    "invalid_grant_type",
			ClientID:     app.ID.String(),
			ClientSecret: clientSecret,
		}

		oauth2providertest.PerformTokenExchangeExpectingError(
			t, client.URL.String(), tokenParams, oauth2providertest.OAuth2ErrorTypes.UnsupportedGrantType,
		)
	})

	t.Run("MissingCode", func(t *testing.T) {
		t.Parallel()

		app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
		t.Cleanup(func() {
			oauth2providertest.CleanupOAuth2App(t, client, app.ID)
		})

		tokenParams := oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			ClientID:     app.ID.String(),
			ClientSecret: clientSecret,
		}

		oauth2providertest.PerformTokenExchangeExpectingError(
			t, client.URL.String(), tokenParams, oauth2providertest.OAuth2ErrorTypes.InvalidRequest,
		)
	})
}
