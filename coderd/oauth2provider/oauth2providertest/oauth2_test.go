package oauth2providertest_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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

// TestOAuth2PKCEFailureConsumesCode verifies that a code_verifier that fails
// the PKCE hash comparison consumes the authorization code (RFC 6749 §10.5:
// codes are single-use). Without this, a leaked code could be replayed with
// unlimited further code_verifier guesses for the rest of its lifetime.
func TestOAuth2PKCEFailureConsumesCode(t *testing.T) {
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

	// Attempt the exchange with a well-formed but wrong verifier. This fails
	// the PKCE hash comparison (invalid_grant) and must consume the code.
	failedParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		CodeVerifier: oauth2providertest.InvalidCodeVerifier,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}
	oauth2providertest.PerformTokenExchangeExpectingError(
		t, client.URL.String(), failedParams, oauth2providertest.OAuth2ErrorTypes.InvalidGrant,
	)

	// The correct verifier can no longer redeem the code: the failed PKCE
	// comparison above already consumed it.
	retryParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		CodeVerifier: codeVerifier,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}
	oauth2providertest.PerformTokenExchangeExpectingError(
		t, client.URL.String(), retryParams, oauth2providertest.OAuth2ErrorTypes.InvalidGrant,
	)
}

// TestOAuth2MalformedCodeVerifierIsRejected verifies that a code_verifier
// below the RFC 7636 §4.1 length floor is rejected as invalid_request,
// distinct from a well-formed verifier that fails the PKCE hash comparison
// (invalid_grant, covered by TestOAuth2InvalidPKCE).
func TestOAuth2MalformedCodeVerifierIsRejected(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	app, clientSecret := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	_, codeChallenge := oauth2providertest.GeneratePKCE(t)
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

	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		CodeVerifier: oauth2providertest.MalformedCodeVerifier,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}

	oauth2providertest.PerformTokenExchangeExpectingError(
		t, client.URL.String(), tokenParams, oauth2providertest.OAuth2ErrorTypes.InvalidRequest,
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

	oauth2providertest.AuthorizeOAuth2AppExpectingRedirectError(
		t, client, client.URL.String(), authParams, codersdk.OAuth2ErrorCodeInvalidRequest,
		"is required and cannot be empty",
	)
}

// TestOAuth2MalformedCodeChallengeIsRejected verifies that a code_challenge
// below the RFC 7636 §4.1 length floor is rejected at the authorization
// request, rather than being stored and only failing once a client attempts
// to exchange the resulting code.
func TestOAuth2MalformedCodeChallengeIsRejected(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
	t.Cleanup(func() {
		oauth2providertest.CleanupOAuth2App(t, client, app.ID)
	})

	state := oauth2providertest.GenerateState(t)

	authParams := oauth2providertest.AuthorizeParams{
		ClientID:            app.ID.String(),
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       "too-short",
		CodeChallengeMethod: "S256",
	}

	oauth2providertest.AuthorizeOAuth2AppExpectingRedirectError(
		t, client, client.URL.String(), authParams, codersdk.OAuth2ErrorCodeInvalidRequest,
		"must be 43 to 128 characters",
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

// TestOAuth2RegisterPublicClient exercises the RegisterPublicClient helper
// end-to-end against a real server: registering with token_endpoint_auth_
// method "none" issues no client secret. A bug in the helper's request
// shape or assertions would otherwise ride uncaught until a later PR's test
// happened to call it.
func TestOAuth2RegisterPublicClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	oauth2providertest.EnableDCR(t, client)

	resp := oauth2providertest.RegisterPublicClient(t, client, "test-public-client", "https://example.com/callback")
	require.NotEmpty(t, resp.ClientID)
}

// RFC 7591 clients may register several redirect_uris and use any of them.
// Cursor registers a desktop deep link and a web callback, then uses the second.
func TestOAuth2MultipleRegisteredRedirectURIs(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	oauth2providertest.EnableDCR(t, client)

	const (
		desktopRedirectURI = "cursor://anysphere.cursor-mcp/oauth/callback"
		webRedirectURI     = "https://www.cursor.com/agents/mcp/oauth/callback"
		unregisteredURI    = "https://www.cursor.com/agents/mcp/oauth/callback/other"
	)

	register := func(t *testing.T) codersdk.OAuth2ClientRegistrationResponse {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		registration, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs:            []string{desktopRedirectURI, webRedirectURI},
			ClientName:              "cursor-" + testutil.MustRandString(t, 10),
			TokenEndpointAuthMethod: codersdk.OAuth2TokenEndpointAuthMethodNone,
		})
		require.NoError(t, err)
		return registration
	}

	// The package helpers only POST and always send redirect_uri.
	sendAuthorize := func(t *testing.T, method, clientID, redirectURI string) *http.Response {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, codeChallenge := oauth2providertest.GeneratePKCE(t)
		authURL, err := url.Parse(client.URL.String() + "/oauth2/authorize")
		require.NoError(t, err)
		query := url.Values{}
		query.Set("client_id", clientID)
		query.Set("response_type", "code")
		query.Set("state", oauth2providertest.GenerateState(t))
		query.Set("code_challenge", codeChallenge)
		query.Set("code_challenge_method", "S256")
		if redirectURI != "" {
			query.Set("redirect_uri", redirectURI)
		}
		authURL.RawQuery = query.Encode()
		req, err := http.NewRequestWithContext(ctx, method, authURL.String(), nil)
		require.NoError(t, err)
		req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
		resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}).Do(req)
		require.NoError(t, err)
		return resp
	}

	authorize := func(t *testing.T, clientID, redirectURI string) (code, verifier string) {
		t.Helper()
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code = oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), oauth2providertest.AuthorizeParams{
			ClientID:            clientID,
			ResponseType:        "code",
			RedirectURI:         redirectURI,
			State:               oauth2providertest.GenerateState(t),
			CodeChallenge:       challenge,
			CodeChallengeMethod: "S256",
		})
		return code, verifier
	}

	t.Run("SecondRegisteredURIAccepted", func(t *testing.T) {
		t.Parallel()
		registration := register(t)

		getResp := sendAuthorize(t, http.MethodGet, registration.ClientID, webRedirectURI)
		defer getResp.Body.Close()
		require.Equal(t, http.StatusOK, getResp.StatusCode, "consent page must render for a non-primary registered URI")

		code, verifier := authorize(t, registration.ClientID, webRedirectURI)
		token := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			Code:         code,
			ClientID:     registration.ClientID,
			CodeVerifier: verifier,
			RedirectURI:  webRedirectURI,
		})
		require.NotEmpty(t, token.AccessToken)
	})

	t.Run("CodeDeliveredToPresentedURI", func(t *testing.T) {
		t.Parallel()
		registration := register(t)

		resp := sendAuthorize(t, http.MethodPost, registration.ClientID, webRedirectURI)
		defer resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		require.True(t, strings.HasPrefix(resp.Header.Get("Location"), webRedirectURI+"?"),
			"the browser must land on the URI it presented, not the primary: %s", resp.Header.Get("Location"))
	})

	t.Run("UnregisteredURIRejected", func(t *testing.T) {
		t.Parallel()
		registration := register(t)

		getResp := sendAuthorize(t, http.MethodGet, registration.ClientID, unregisteredURI)
		defer getResp.Body.Close()
		require.Equal(t, http.StatusBadRequest, getResp.StatusCode)

		resp := sendAuthorize(t, http.MethodPost, registration.ClientID, unregisteredURI)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var oauthErr oauth2providertest.OAuth2Error
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&oauthErr))
		require.Equal(t, "invalid_request", oauthErr.Error)
		require.Contains(t, oauthErr.ErrorDescription, "registered redirect URIs",
			"the rejection must not point the client at the primary URI only")
	})

	// RFC 6749 §3.1.2.3.
	t.Run("OmittedURIRejectedWhenSeveralRegistered", func(t *testing.T) {
		t.Parallel()
		registration := register(t)

		getResp := sendAuthorize(t, http.MethodGet, registration.ClientID, "")
		defer getResp.Body.Close()
		require.Equal(t, http.StatusBadRequest, getResp.StatusCode)

		resp := sendAuthorize(t, http.MethodPost, registration.ClientID, "")
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var oauthErr oauth2providertest.OAuth2Error
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&oauthErr))
		require.Equal(t, "invalid_request", oauthErr.Error)
		require.Contains(t, oauthErr.ErrorDescription, "more than one redirect URI")
	})

	// RFC 6749 §4.1.3.
	t.Run("ExchangeWithOtherRegisteredURIRejected", func(t *testing.T) {
		t.Parallel()
		registration := register(t)

		code, verifier := authorize(t, registration.ClientID, webRedirectURI)
		oauth2providertest.PerformTokenExchangeExpectingError(t, client.URL.String(), oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			Code:         code,
			ClientID:     registration.ClientID,
			CodeVerifier: verifier,
			RedirectURI:  desktopRedirectURI,
		}, "invalid_grant")
	})

	t.Run("ExchangeWithUnregisteredURIRejected", func(t *testing.T) {
		t.Parallel()
		registration := register(t)

		code, verifier := authorize(t, registration.ClientID, webRedirectURI)
		oauth2providertest.PerformTokenExchangeExpectingError(t, client.URL.String(), oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			Code:         code,
			ClientID:     registration.ClientID,
			CodeVerifier: verifier,
			RedirectURI:  unregisteredURI,
		}, "invalid_request")
	})

	t.Run("AdminCallbackEditRevokesOtherURIs", func(t *testing.T) {
		t.Parallel()
		registration := register(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		appID, err := uuid.Parse(registration.ClientID)
		require.NoError(t, err)
		_, err = client.PutOAuth2ProviderApp(ctx, appID, codersdk.PutOAuth2ProviderAppRequest{
			Name:        "cursor-" + testutil.MustRandString(t, 10),
			CallbackURL: webRedirectURI,
		})
		require.NoError(t, err)

		getResp := sendAuthorize(t, http.MethodGet, registration.ClientID, desktopRedirectURI)
		defer getResp.Body.Close()
		require.Equal(t, http.StatusBadRequest, getResp.StatusCode, "the replaced URI must no longer be accepted")

		code, verifier := authorize(t, registration.ClientID, webRedirectURI)
		token := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			Code:         code,
			ClientID:     registration.ClientID,
			CodeVerifier: verifier,
			RedirectURI:  webRedirectURI,
		})
		require.NotEmpty(t, token.AccessToken, "the new callback must keep working")
	})
}
