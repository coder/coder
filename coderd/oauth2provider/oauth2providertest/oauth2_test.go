package oauth2providertest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
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

func TestOAuth2WithoutPKCE(t *testing.T) {
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

	// Perform authorization without PKCE
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:     app.ID.String(),
		ResponseType: "code",
		RedirectURI:  oauth2providertest.TestRedirectURI,
		State:        state,
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)
	require.NotEmpty(t, code, "should receive authorization code")

	// Exchange code for token without PKCE
	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		RedirectURI:  oauth2providertest.TestRedirectURI,
	}

	token := oauth2providertest.ExchangeCodeForToken(t, client.URL.String(), tokenParams)
	require.NotEmpty(t, token.AccessToken, "should receive access token")
	require.NotEmpty(t, token.RefreshToken, "should receive refresh token")
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

	// Perform authorization with resource parameter
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:     app.ID.String(),
		ResponseType: "code",
		RedirectURI:  oauth2providertest.TestRedirectURI,
		State:        state,
		Resource:     oauth2providertest.TestResourceURI,
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)
	require.NotEmpty(t, code, "should receive authorization code")

	// Exchange code for token with resource parameter
	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		RedirectURI:  oauth2providertest.TestRedirectURI,
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

	// Get initial token
	authParams := oauth2providertest.AuthorizeParams{
		ClientID:     app.ID.String(),
		ResponseType: "code",
		RedirectURI:  oauth2providertest.TestRedirectURI,
		State:        state,
	}

	code := oauth2providertest.AuthorizeOAuth2App(t, client, client.URL.String(), authParams)

	tokenParams := oauth2providertest.TokenExchangeParams{
		GrantType:    "authorization_code",
		Code:         code,
		ClientID:     app.ID.String(),
		ClientSecret: clientSecret,
		RedirectURI:  oauth2providertest.TestRedirectURI,
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
