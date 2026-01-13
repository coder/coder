package oauth2provider

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// parseScopes parses a space-delimited scope string into a slice of scopes
// per RFC 6749.
func parseScopes(scope string) []string {
	return strings.Fields(strings.TrimSpace(scope))
}

// TestExtractTokenParams_Scopes tests OAuth2 scope parameter parsing
// to ensure RFC 6749 compliance where scopes are space-delimited
func TestExtractTokenParams_Scopes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		scopeParam     string   // Raw query param value (before URL encoding)
		expectedScopes []string // Expected parsed scope slice
		description    string   // Test case description
	}{
		{
			name:           "SpaceSeparatedTwoScopes",
			scopeParam:     "coder:workspace.create coder:workspace.operate",
			expectedScopes: []string{"coder:workspace.create", "coder:workspace.operate"},
			description:    "RFC 6749 compliant: space-separated scopes",
		},
		{
			name:           "SpaceSeparatedThreeScopes",
			scopeParam:     "scope1 scope2 scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
			description:    "Multiple space-separated scopes",
		},
		{
			name:           "SingleScope",
			scopeParam:     "coder:workspace.create",
			expectedScopes: []string{"coder:workspace.create"},
			description:    "Single scope without spaces",
		},
		{
			name:           "EmptyScope",
			scopeParam:     "",
			expectedScopes: []string{},
			description:    "Empty scope parameter",
		},
		{
			name:           "MultipleSpaces",
			scopeParam:     "scope1  scope2   scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
			description:    "Multiple consecutive spaces should be handled gracefully",
		},
		{
			name:           "LeadingAndTrailingSpaces",
			scopeParam:     " scope1 scope2 ",
			expectedScopes: []string{"scope1", "scope2"},
			description:    "Leading and trailing spaces should be trimmed",
		},
		{
			name:           "ColonInScope",
			scopeParam:     "coder:workspace:read coder:workspace:write",
			expectedScopes: []string{"coder:workspace:read", "coder:workspace:write"},
			description:    "Scopes with colons (common pattern)",
		},
		{
			name:           "DotInScope",
			scopeParam:     "workspace.create workspace.delete",
			expectedScopes: []string{"workspace.create", "workspace.delete"},
			description:    "Scopes with dots (common pattern)",
		},
		{
			name:           "HyphenInScope",
			scopeParam:     "workspace-read workspace-write",
			expectedScopes: []string{"workspace-read", "workspace-write"},
			description:    "Scopes with hyphens",
		},
		{
			name:           "UnderscoreInScope",
			scopeParam:     "workspace_create workspace_delete",
			expectedScopes: []string{"workspace_create", "workspace_delete"},
			description:    "Scopes with underscores",
		},
		{
			name:           "OpenIDScopes",
			scopeParam:     "openid profile email",
			expectedScopes: []string{"openid", "profile", "email"},
			description:    "Common OpenID Connect scopes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a mock request with the scope parameter
			callbackURL, err := url.Parse("http://localhost:3000/callback")
			require.NoError(t, err)

			// Build form values (simulating POST request body)
			form := url.Values{}
			form.Set("grant_type", "authorization_code")
			form.Set("client_id", "test-client")
			form.Set("client_secret", "test-secret")
			form.Set("code", "test-code")
			if tc.scopeParam != "" {
				form.Set("scope", tc.scopeParam)
			}

			// Create request with form data already parsed
			// Set PostForm and Form directly to bypass the need for a request body
			req := &http.Request{
				Method:   http.MethodPost,
				PostForm: form,
				Form:     form, // Form is the combination of PostForm and URL query
			}

			// Extract token request
			tokenReq, validationErrs, err := extractTokenRequest(req, callbackURL)

			// Verify no errors occurred
			require.NoError(t, err, "extractTokenRequest should not return error for: %s", tc.description)
			require.Empty(t, validationErrs, "should have no validation errors for: %s", tc.description)

			// Verify scopes match expected
			require.Equal(t, tc.expectedScopes, parseScopes(tokenReq.Scope), "scope parsing failed for: %s", tc.description)
		})
	}
}

// TestExtractTokenParams_ScopesURLEncoded tests that URL-encoded space-separated
// scopes are correctly decoded and parsed
func TestExtractTokenParams_ScopesURLEncoded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		rawQuery       string   // Raw query string with URL encoding
		expectedScopes []string // Expected parsed scope slice
	}{
		{
			name:           "PlusEncodedSpaces",
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&scope=scope1+scope2+scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
		},
		{
			name:           "PercentEncodedSpaces",
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&scope=scope1%20scope2%20scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
		},
		{
			name:           "MixedEncoding",
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&scope=scope1+scope2%20scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
		},
		{
			name:           "ColonEncodedInScope",
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&scope=coder%3Aworkspace.create+coder%3Aworkspace.operate",
			expectedScopes: []string{"coder:workspace.create", "coder:workspace.operate"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			callbackURL, err := url.Parse("http://localhost:3000/callback")
			require.NoError(t, err)

			// Parse the raw query string
			values, err := url.ParseQuery(tc.rawQuery)
			require.NoError(t, err)

			// Create request with form data already parsed
			req := &http.Request{
				Method:   http.MethodPost,
				PostForm: values,
				Form:     values,
			}

			// Extract token request
			tokenReq, validationErrs, err := extractTokenRequest(req, callbackURL)

			// Verify no errors
			require.NoError(t, err)
			require.Empty(t, validationErrs)

			// Verify scopes
			require.Equal(t, tc.expectedScopes, parseScopes(tokenReq.Scope))
		})
	}
}

// TestExtractTokenParams_ScopesEdgeCases tests edge cases in scope parsing
func TestExtractTokenParams_ScopesEdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		setupForm      func() url.Values
		expectedScopes []string
		description    string
	}{
		{
			name: "NoScopeParameter",
			setupForm: func() url.Values {
				form := url.Values{}
				form.Set("grant_type", "authorization_code")
				form.Set("client_id", "test-client")
				form.Set("client_secret", "test-secret")
				form.Set("code", "test-code")
				return form
			},
			expectedScopes: []string{},
			description:    "Missing scope parameter should default to empty slice",
		},
		{
			name: "OnlySpaces",
			setupForm: func() url.Values {
				form := url.Values{}
				form.Set("grant_type", "authorization_code")
				form.Set("client_id", "test-client")
				form.Set("client_secret", "test-secret")
				form.Set("code", "test-code")
				form.Set("scope", "   ")
				return form
			},
			expectedScopes: []string{},
			description:    "Scope with only spaces should result in empty slice",
		},
		{
			name: "VeryLongScopeName",
			setupForm: func() url.Values {
				longScope := "coder:workspace:project:resource:action:create:read:write:delete:admin"
				form := url.Values{}
				form.Set("grant_type", "authorization_code")
				form.Set("client_id", "test-client")
				form.Set("client_secret", "test-secret")
				form.Set("code", "test-code")
				form.Set("scope", longScope)
				return form
			},
			expectedScopes: []string{"coder:workspace:project:resource:action:create:read:write:delete:admin"},
			description:    "Very long scope names should be handled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			callbackURL, err := url.Parse("http://localhost:3000/callback")
			require.NoError(t, err)

			form := tc.setupForm()
			req := &http.Request{
				Method:   http.MethodPost,
				PostForm: form,
				Form:     form,
			}

			tokenReq, validationErrs, err := extractTokenRequest(req, callbackURL)

			require.NoError(t, err, "extractTokenRequest should not error for: %s", tc.description)
			require.Empty(t, validationErrs)
			require.Equal(t, tc.expectedScopes, parseScopes(tokenReq.Scope), "scope mismatch for: %s", tc.description)
		})
	}
}

// TestExtractAuthorizeParams_Scopes tests scope parsing in the authorization endpoint
func TestExtractAuthorizeParams_Scopes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		scopeParam     string
		expectedScopes []string
	}{
		{
			name:           "SpaceSeparated",
			scopeParam:     "openid profile email",
			expectedScopes: []string{"openid", "profile", "email"},
		},
		{
			name:           "SingleScope",
			scopeParam:     "openid",
			expectedScopes: []string{"openid"},
		},
		{
			name:           "EmptyScope",
			scopeParam:     "",
			expectedScopes: []string{},
		},
		{
			name:           "CoderScopes",
			scopeParam:     "coder:workspace.create coder:workspace.read coder:workspace.delete",
			expectedScopes: []string{"coder:workspace.create", "coder:workspace.read", "coder:workspace.delete"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			callbackURL, err := url.Parse("http://localhost:3000/callback")
			require.NoError(t, err)

			// Build query parameters for GET request
			query := url.Values{}
			query.Set("response_type", "code")
			query.Set("client_id", "test-client")
			query.Set("redirect_uri", "http://localhost:3000/callback")
			if tc.scopeParam != "" {
				query.Set("scope", tc.scopeParam)
			}

			// Create request with query parameters
			reqURL, err := url.Parse("http://localhost:8080/oauth2/authorize?" + query.Encode())
			require.NoError(t, err)

			req := &http.Request{
				Method: http.MethodGet,
				URL:    reqURL,
			}

			// Extract authorize params
			params, validationErrs, err := extractAuthorizeParams(req, callbackURL)

			require.NoError(t, err)
			require.Empty(t, validationErrs)
			require.Equal(t, tc.expectedScopes, params.scope)
		})
	}
}

// TestRefreshTokenGrant_Scopes tests that scopes can be requested during refresh
func TestRefreshTokenGrant_Scopes(t *testing.T) {
	t.Parallel()

	// Test that refresh token requests can include scope parameter
	// per RFC 6749 Section 6
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", "test-refresh-token")
	form.Set("scope", "reduced:scope subset:scope")

	callbackURL, err := url.Parse("http://localhost:3000/callback")
	require.NoError(t, err)

	req := &http.Request{
		Method:   http.MethodPost,
		PostForm: form,
		Form:     form,
	}

	tokenReq, validationErrs, err := extractTokenRequest(req, callbackURL)

	require.NoError(t, err)
	require.Empty(t, validationErrs)
	require.Equal(t, codersdk.OAuth2ProviderGrantTypeRefreshToken, tokenReq.GrantType)
	require.Equal(t, []string{"reduced:scope", "subset:scope"}, parseScopes(tokenReq.Scope))
}
