package oauth2provider_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOAuth2ClientMetadataValidation tests enhanced metadata validation per RFC 7591
func TestOAuth2ClientMetadataValidation(t *testing.T) {
	t.Parallel()

	t.Run("RedirectURIValidation", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name          string
			redirectURIs  []string
			expectError   bool
			errorContains string
		}{
			{
				name:         "ValidHTTPS",
				redirectURIs: []string{"https://example.com/callback"},
				expectError:  false,
			},
			{
				name:         "ValidLocalhost",
				redirectURIs: []string{"http://localhost:8080/callback"},
				expectError:  false,
			},
			{
				name:         "ValidLocalhostIP",
				redirectURIs: []string{"http://127.0.0.1:8080/callback"},
				expectError:  false,
			},
			{
				name:         "ValidCustomScheme",
				redirectURIs: []string{"com.example.myapp://auth/callback"},
				expectError:  false,
			},
			{
				name:          "InvalidHTTPNonLocalhost",
				redirectURIs:  []string{"http://example.com/callback"},
				expectError:   true,
				errorContains: "redirect_uri",
			},
			{
				name:          "InvalidWithFragment",
				redirectURIs:  []string{"https://example.com/callback#fragment"},
				expectError:   true,
				errorContains: "fragment",
			},
			{
				name:          "InvalidJavaScriptScheme",
				redirectURIs:  []string{"javascript:alert('xss')"},
				expectError:   true,
				errorContains: "dangerous scheme",
			},
			{
				name:          "InvalidDataScheme",
				redirectURIs:  []string{"data:text/html,<script>alert('xss')</script>"},
				expectError:   true,
				errorContains: "dangerous scheme",
			},
			{
				name:          "InvalidFileScheme",
				redirectURIs:  []string{"file:///etc/passwd"},
				expectError:   true,
				errorContains: "dangerous scheme",
			},
			{
				name:          "EmptyString",
				redirectURIs:  []string{""},
				expectError:   true,
				errorContains: "redirect_uri",
			},
			{
				name:          "RelativeURL",
				redirectURIs:  []string{"/callback"},
				expectError:   true,
				errorContains: "redirect_uri",
			},
			{
				name:         "MultipleValid",
				redirectURIs: []string{"https://example.com/callback", "com.example.app://auth"},
				expectError:  false,
			},
			{
				name:          "MixedValidInvalid",
				redirectURIs:  []string{"https://example.com/callback", "http://example.com/callback"},
				expectError:   true,
				errorContains: "redirect_uri",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				req := codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs: test.redirectURIs,
					ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
				}

				_, err := client.PostOAuth2ClientRegistration(ctx, req)

				if test.expectError {
					require.Error(t, err)
					if test.errorContains != "" {
						require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(test.errorContains))
					}
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("ClientURIValidation", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name        string
			clientURI   string
			expectError bool
		}{
			{
				name:        "ValidHTTPS",
				clientURI:   "https://example.com",
				expectError: false,
			},
			{
				name:        "ValidHTTPLocalhost",
				clientURI:   "http://localhost:8080",
				expectError: false,
			},
			{
				name:        "ValidWithPath",
				clientURI:   "https://example.com/app",
				expectError: false,
			},
			{
				name:        "ValidWithQuery",
				clientURI:   "https://example.com/app?param=value",
				expectError: false,
			},
			{
				name:        "InvalidNotURL",
				clientURI:   "not-a-url",
				expectError: true,
			},
			{
				name:        "ValidWithFragment",
				clientURI:   "https://example.com#fragment",
				expectError: false, // Fragments are allowed in client_uri, unlike redirect_uri
			},
			{
				name:        "InvalidJavaScript",
				clientURI:   "javascript:alert('xss')",
				expectError: true, // Only http/https allowed for client_uri
			},
			{
				name:        "InvalidFTP",
				clientURI:   "ftp://example.com",
				expectError: true, // Only http/https allowed for client_uri
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				req := codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs: []string{"https://example.com/callback"},
					ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
					ClientURI:    test.clientURI,
				}

				_, err := client.PostOAuth2ClientRegistration(ctx, req)

				if test.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("LogoURIValidation", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name        string
			logoURI     string
			expectError bool
		}{
			{
				name:        "ValidHTTPS",
				logoURI:     "https://example.com/logo.png",
				expectError: false,
			},
			{
				name:        "ValidHTTPLocalhost",
				logoURI:     "http://localhost:8080/logo.png",
				expectError: false,
			},
			{
				name:        "ValidWithQuery",
				logoURI:     "https://example.com/logo.png?size=large",
				expectError: false,
			},
			{
				name:        "InvalidNotURL",
				logoURI:     "not-a-url",
				expectError: true,
			},
			{
				name:        "ValidWithFragment",
				logoURI:     "https://example.com/logo.png#fragment",
				expectError: false, // Fragments are allowed in logo_uri
			},
			{
				name:        "InvalidJavaScript",
				logoURI:     "javascript:alert('xss')",
				expectError: true, // Only http/https allowed for logo_uri
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				req := codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs: []string{"https://example.com/callback"},
					ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
					LogoURI:      test.logoURI,
				}

				_, err := client.PostOAuth2ClientRegistration(ctx, req)

				if test.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("GrantTypeValidation", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name        string
			grantTypes  []codersdk.OAuth2ProviderGrantType
			expectError bool
		}{
			{
				name:        "DefaultEmpty",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{},
				expectError: false,
			},
			{
				name:        "ValidAuthorizationCode",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeAuthorizationCode},
				expectError: false,
			},
			{
				name:        "InvalidRefreshTokenAlone",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeRefreshToken},
				expectError: true, // refresh_token requires authorization_code to be present
			},
			{
				name:        "ValidMultiple",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeAuthorizationCode, codersdk.OAuth2ProviderGrantTypeRefreshToken},
				expectError: false,
			},
			{
				name:        "InvalidUnsupported",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{"client_credentials"},
				expectError: true,
			},
			{
				name:        "InvalidPassword",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{"password"},
				expectError: true,
			},
			{
				name:        "InvalidImplicit",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{"implicit"},
				expectError: true,
			},
			{
				name:        "MixedValidInvalid",
				grantTypes:  []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeAuthorizationCode, "client_credentials"},
				expectError: true,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				req := codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs: []string{"https://example.com/callback"},
					ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
					GrantTypes:   test.grantTypes,
				}

				_, err := client.PostOAuth2ClientRegistration(ctx, req)

				if test.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("ResponseTypeValidation", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name          string
			responseTypes []codersdk.OAuth2ProviderResponseType
			expectError   bool
		}{
			{
				name:          "DefaultEmpty",
				responseTypes: []codersdk.OAuth2ProviderResponseType{},
				expectError:   false,
			},
			{
				name:          "ValidCode",
				responseTypes: []codersdk.OAuth2ProviderResponseType{codersdk.OAuth2ProviderResponseTypeCode},
				expectError:   false,
			},
			{
				name:          "InvalidToken",
				responseTypes: []codersdk.OAuth2ProviderResponseType{"token"},
				expectError:   true,
			},
			{
				name:          "InvalidImplicit",
				responseTypes: []codersdk.OAuth2ProviderResponseType{"id_token"},
				expectError:   true,
			},
			{
				name:          "InvalidMultiple",
				responseTypes: []codersdk.OAuth2ProviderResponseType{codersdk.OAuth2ProviderResponseTypeCode, "token"},
				expectError:   true,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				req := codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs:  []string{"https://example.com/callback"},
					ClientName:    fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
					ResponseTypes: test.responseTypes,
				}

				_, err := client.PostOAuth2ClientRegistration(ctx, req)

				if test.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("TokenEndpointAuthMethodValidation", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		tests := []struct {
			name        string
			authMethod  codersdk.OAuth2TokenEndpointAuthMethod
			expectError bool
		}{
			{
				name:        "DefaultEmpty",
				authMethod:  "",
				expectError: false,
			},
			{
				name:        "ValidClientSecretBasic",
				authMethod:  codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic,
				expectError: false,
			},
			{
				name:        "ValidClientSecretPost",
				authMethod:  codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost,
				expectError: false,
			},
			{
				name:        "ValidNone",
				authMethod:  codersdk.OAuth2TokenEndpointAuthMethodNone,
				expectError: false, // "none" is valid for public clients per RFC 7591
			},
			{
				name:        "InvalidPrivateKeyJWT",
				authMethod:  "private_key_jwt",
				expectError: true,
			},
			{
				name:        "InvalidClientSecretJWT",
				authMethod:  "client_secret_jwt",
				expectError: true,
			},
			{
				name:        "InvalidCustom",
				authMethod:  "custom_method",
				expectError: true,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				req := codersdk.OAuth2ClientRegistrationRequest{
					RedirectURIs:            []string{"https://example.com/callback"},
					ClientName:              fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
					TokenEndpointAuthMethod: test.authMethod,
				}

				_, err := client.PostOAuth2ClientRegistration(ctx, req)

				if test.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})
}

// TestOAuth2ClientNameValidation tests client name validation requirements
func TestOAuth2ClientNameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clientName  string
		expectError bool
	}{
		{
			name:        "ValidBasic",
			clientName:  "My App",
			expectError: false,
		},
		{
			name:        "ValidWithNumbers",
			clientName:  "My App 2.0",
			expectError: false,
		},
		{
			name:        "ValidWithSpecialChars",
			clientName:  "My-App_v1.0",
			expectError: false,
		},
		{
			name:        "ValidUnicode",
			clientName:  "My App 🚀",
			expectError: false,
		},
		{
			name:        "ValidLong",
			clientName:  strings.Repeat("A", 100),
			expectError: false,
		},
		{
			name:        "ValidEmpty",
			clientName:  "",
			expectError: false, // Empty names are allowed, defaults are applied
		},
		{
			name:        "ValidWhitespaceOnly",
			clientName:  "   ",
			expectError: false, // Whitespace-only names are allowed
		},
		{
			name:        "ValidTooLong",
			clientName:  strings.Repeat("A", 1000),
			expectError: false, // Very long names are allowed
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitLong)

			req := codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs: []string{"https://example.com/callback"},
				ClientName:   test.clientName,
			}

			_, err := client.PostOAuth2ClientRegistration(ctx, req)

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestOAuth2ClientScopeValidation tests scope parameter validation
func TestOAuth2ClientScopeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		scope       string
		expectError bool
	}{
		{
			name:        "DefaultEmpty",
			scope:       "",
			expectError: false,
		},
		{
			name:        "ValidRead",
			scope:       "read",
			expectError: false,
		},
		{
			name:        "ValidWrite",
			scope:       "write",
			expectError: false,
		},
		{
			name:        "ValidMultiple",
			scope:       "read write",
			expectError: false,
		},
		{
			name:        "ValidOpenID",
			scope:       "openid",
			expectError: false,
		},
		{
			name:        "ValidProfile",
			scope:       "profile",
			expectError: false,
		},
		{
			name:        "ValidEmail",
			scope:       "email",
			expectError: false,
		},
		{
			name:        "ValidCombined",
			scope:       "openid profile email read write",
			expectError: false,
		},
		{
			name:        "InvalidAdmin",
			scope:       "admin",
			expectError: false, // Admin scope should be allowed but validated during authorization
		},
		{
			name:        "ValidCustom",
			scope:       "custom:scope",
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitLong)

			req := codersdk.OAuth2ClientRegistrationRequest{
				RedirectURIs: []string{"https://example.com/callback"},
				ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
				Scope:        test.scope,
			}

			_, err := client.PostOAuth2ClientRegistration(ctx, req)

			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestOAuth2ClientMetadataDefaults tests that default values are properly applied
func TestOAuth2ClientMetadataDefaults(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)

	ctx := testutil.Context(t, testutil.WaitLong)

	// Register a minimal client to test defaults
	req := codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://example.com/callback"},
		ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
	}

	resp, err := client.PostOAuth2ClientRegistration(ctx, req)
	require.NoError(t, err)

	// Get the configuration to check defaults
	config, err := client.GetOAuth2ClientConfiguration(ctx, resp.ClientID, resp.RegistrationAccessToken)
	require.NoError(t, err)

	// Should default to authorization_code
	require.Contains(t, config.GrantTypes, codersdk.OAuth2ProviderGrantTypeAuthorizationCode)

	// Should default to code
	require.Contains(t, config.ResponseTypes, codersdk.OAuth2ProviderResponseTypeCode)

	// Should default to client_secret_basic or client_secret_post
	require.True(t, config.TokenEndpointAuthMethod == codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic ||
		config.TokenEndpointAuthMethod == codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost ||
		config.TokenEndpointAuthMethod == "")

	// Client secret should be generated
	require.NotEmpty(t, resp.ClientSecret)
	require.Greater(t, len(resp.ClientSecret), 20)

	// Registration access token should be generated
	require.NotEmpty(t, resp.RegistrationAccessToken)
	require.Greater(t, len(resp.RegistrationAccessToken), 20)
}

// TestOAuth2ClientMetadataEdgeCases tests edge cases and boundary conditions
func TestOAuth2ClientMetadataEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("ExtremelyLongRedirectURI", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create a very long but valid HTTPS URI
		longPath := strings.Repeat("a", 2000)
		longURI := "https://example.com/" + longPath

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{longURI},
			ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		// This might be accepted or rejected depending on URI length limits
		// The test verifies the behavior is consistent
		if err != nil {
			require.Contains(t, strings.ToLower(err.Error()), "uri")
		}
	})

	t.Run("ManyRedirectURIs", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Test with many redirect URIs
		redirectURIs := make([]string, 20)
		for i := 0; i < 20; i++ {
			redirectURIs[i] = fmt.Sprintf("https://example%d.com/callback", i)
		}

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: redirectURIs,
			ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		// Should handle multiple redirect URIs gracefully
		require.NoError(t, err)
	})

	t.Run("URIWithUnusualPort", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com:8443/callback"},
			ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)
	})

	t.Run("URIWithComplexPath", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/path/to/callback?param=value&other=123"},
			ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)
	})

	t.Run("URIWithEncodedCharacters", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Test with URL-encoded characters
		encodedURI := "https://example.com/callback?param=" + url.QueryEscape("value with spaces")

		req := codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{encodedURI},
			ClientName:   fmt.Sprintf("test-client-%d", time.Now().UnixNano()),
		}

		_, err := client.PostOAuth2ClientRegistration(ctx, req)
		require.NoError(t, err)
	})
}
