package oauth2provider

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// parseScopes parses a space-delimited scope string into a slice of scopes
// per RFC 6749.
func parseScopes(scope string) []string {
	return strings.Fields(strings.TrimSpace(scope))
}

func TestScopeStringToAPIKeyScopes(t *testing.T) {
	t.Parallel()

	t.Run("EveryNameKept", func(t *testing.T) {
		t.Parallel()

		scopes, err := scopeStringToAPIKeyScopes("workspace:ssh template:read")
		require.NoError(t, err)
		require.Equal(t, database.APIKeyScopes{
			database.ApiKeyScopeWorkspaceSsh,
			database.ApiKeyScopeTemplateRead,
		}, scopes)
	})

	// The catalog and the api_key_scope enum are maintained separately. A name
	// negotiable at authorization but unmintable at exchange leaves the client
	// holding a code it can never redeem.
	t.Run("EveryCatalogNameMintable", func(t *testing.T) {
		t.Parallel()

		// ExternalScopeNames omits the aliases IsExternalScope accepts, and the
		// catalog cannot enumerate them, so a new alias has to be added here.
		names := append(rbac.ExternalScopeNames(), "all", "application_connect")
		require.NotEmpty(t, names)
		for _, name := range names {
			require.Truef(t, rbac.IsExternalScope(rbac.ScopeName(name)),
				"scope %q is not negotiable, so this loop is not driving the catalog", name)

			canonical := string(rbac.CanonicalScopeName(rbac.ScopeName(name)))
			scopes, err := scopeStringToAPIKeyScopes(canonical)
			require.NoErrorf(t, err, "scope %q can be negotiated but not minted", name)
			require.Equal(t, database.APIKeyScopes{database.APIKeyScope(canonical)}, scopes)
		}
	})

	t.Run("UnknownNameRejectsTheWholeList", func(t *testing.T) {
		t.Parallel()

		_, err := scopeStringToAPIKeyScopes("workspace:ssh not_a_real_scope")
		require.ErrorIs(t, err, errUnmintableScope)
		require.Contains(t, err.Error(), "not_a_real_scope")
	})

	// Unreachable through the NOT NULL column, but pinned: apikey.Generate reads
	// an empty list as unrestricted, so anything but an error widens the grant.
	t.Run("EmptyRejected", func(t *testing.T) {
		t.Parallel()

		for _, scope := range []string{"", "   "} {
			_, err := scopeStringToAPIKeyScopes(scope)
			require.ErrorIs(t, err, errUnmintableScope, "scope %q", scope)
		}
	})
}

func TestScopeStillCoveredByAllowlist(t *testing.T) {
	t.Parallel()

	const (
		inCatalog     = "coder:workspaces.access"
		alsoInCatalog = "coder:templates.build"
	)

	tests := []struct {
		name     string
		granted  string
		appScope sql.NullString
		wantErr  error
	}{
		{
			name:     "NoAllowlistConstrainsNothing",
			granted:  string(database.ApiKeyScopeCoderAll),
			appScope: sql.NullString{},
		},
		{
			name:     "EmptyAllowlistConstrainsNothing",
			granted:  "workspace:ssh",
			appScope: sql.NullString{String: "", Valid: true},
		},
		{
			name:     "UnchangedAllowlistStillCovers",
			granted:  inCatalog,
			appScope: sql.NullString{String: inCatalog, Valid: true},
		},
		{
			name:     "CompositeStillCoversItsParts",
			granted:  "workspace:ssh",
			appScope: sql.NullString{String: inCatalog, Valid: true},
		},
		{
			name:     "WidenedAllowlistStillCovers",
			granted:  "workspace:ssh",
			appScope: sql.NullString{String: inCatalog + " " + alsoInCatalog, Valid: true},
		},
		{
			name:     "AllowlistNarrowedAwayRejected",
			granted:  "workspace:ssh",
			appScope: sql.NullString{String: alsoInCatalog, Valid: true},
			wantErr:  errStaleScope,
		},
		{
			name:     "PartiallyCoveredRejectedWhole",
			granted:  "workspace:ssh file:create",
			appScope: sql.NullString{String: inCatalog, Valid: true},
			wantErr:  errStaleScope,
		},
		{
			name:     "UnrestrictedGrantNarrowedRejected",
			granted:  string(database.ApiKeyScopeCoderAll),
			appScope: sql.NullString{String: inCatalog, Valid: true},
			wantErr:  errStaleScope,
		},
		{
			name:     "AllowlistFilteredToEmptyRejected",
			granted:  "workspace:ssh",
			appScope: sql.NullString{String: "openid profile", Valid: true},
			wantErr:  errNoGrantableScope,
		},
		{
			name:     "WhitespaceOnlyAllowlistRejected",
			granted:  "workspace:ssh",
			appScope: sql.NullString{String: "   ", Valid: true},
			wantErr:  errNoGrantableScope,
		},
		{
			name:     "LegacyAliasAllowlistCoversCanonicalGrant",
			granted:  "coder:all",
			appScope: sql.NullString{String: "all", Valid: true},
		},
		{
			name:     "GrantOutsideTheCatalogUndecidable",
			granted:  "some_removed_scope",
			appScope: sql.NullString{String: inCatalog, Valid: true},
			wantErr:  errCoverageUndecidable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			app := database.OAuth2ProviderApp{ID: uuid.New(), Scope: test.appScope}
			err := scopeStillCoveredByAllowlist(t.Context(), slogtest.Make(t, nil), app, test.granted)
			if test.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, test.wantErr)
			assert.Equal(t, 1, strings.Count(err.Error(), test.wantErr.Error()),
				"the rejection reason must appear once, not doubled by the wrap")
		})
	}
}

func TestNarrowGrantedScope(t *testing.T) {
	t.Parallel()

	const (
		inCatalog     = "coder:workspaces.access"
		alsoInCatalog = "coder:templates.build"
	)

	tests := []struct {
		name      string
		granted   string
		requested []string
		want      string
		wantErr   error
	}{
		{
			name:      "OmittedRequestKeepsTheGrant",
			granted:   inCatalog + " " + alsoInCatalog,
			requested: nil,
			want:      inCatalog + " " + alsoInCatalog,
		},
		{
			name:      "GenuineSubsetAccepted",
			granted:   inCatalog + " " + alsoInCatalog,
			requested: []string{inCatalog},
			want:      inCatalog,
		},
		{
			name:      "ConstituentOfCompositeAccepted",
			granted:   inCatalog,
			requested: []string{"workspace:ssh"},
			want:      "workspace:ssh",
		},
		{
			// coder:all is a member of no other set, so membership would leave
			// an unrestricted grant unnarrowable.
			name:      "UnrestrictedGrantNarrowed",
			granted:   string(database.ApiKeyScopeCoderAll),
			requested: []string{"workspace:read"},
			want:      "workspace:read",
		},
		{
			name:      "ExpansionRejected",
			granted:   inCatalog,
			requested: []string{alsoInCatalog},
			wantErr:   errScopeNotGranted,
		},
		{
			name:      "PartiallyCoveredRequestRejectedWhole",
			granted:   inCatalog,
			requested: []string{"workspace:ssh", alsoInCatalog},
			wantErr:   errScopeNotGranted,
		},
		{
			name:      "UnknownRequestedScopeRejectedAsUnknown",
			granted:   string(database.ApiKeyScopeCoderAll),
			requested: []string{"not_a_real_scope"},
			wantErr:   errUnknownScope,
		},
		{
			// RBAC expands debug_info:read; only the catalog keeps it internal.
			name:      "InternalOnlyScopeRejected",
			granted:   string(database.ApiKeyScopeCoderAll),
			requested: []string{"debug_info:read"},
			wantErr:   errUnknownScope,
		},
		{
			name:      "LegacyAliasCanonicalized",
			granted:   string(database.ApiKeyScopeCoderAll),
			requested: []string{"all"},
			want:      "coder:all",
		},
		{
			name:      "DuplicateRequestedScopesDeduplicated",
			granted:   inCatalog,
			requested: []string{"workspace:ssh", "workspace:ssh"},
			want:      "workspace:ssh",
		},
		{
			name:      "GrantOutsideTheCatalogUndecidable",
			granted:   "some_removed_scope",
			requested: []string{"workspace:ssh"},
			wantErr:   errCoverageUndecidable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			app := database.OAuth2ProviderApp{ID: uuid.New()}
			got, err := narrowGrantedScope(t.Context(), slogtest.Make(t, nil), app, test.granted, test.requested)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				assert.Empty(t, got, "a rejected refresh must not return a persistable scope")
				assert.Equal(t, 1, strings.Count(err.Error(), test.wantErr.Error()),
					"the rejection reason must appear once, not doubled by the wrap")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			requirePersistableScope(t, got)
		})
	}
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
			// This test only exercises scope parsing, but code_verifier is
			// validated unconditionally for this grant type, so use a value
			// that satisfies the RFC 7636 §4.1 length floor.
			form.Set("code_verifier", strings.Repeat("a", 43))
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
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&code_verifier=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&scope=scope1+scope2+scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
		},
		{
			name:           "PercentEncodedSpaces",
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&code_verifier=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&scope=scope1%20scope2%20scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
		},
		{
			name:           "MixedEncoding",
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&code_verifier=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&scope=scope1+scope2%20scope3",
			expectedScopes: []string{"scope1", "scope2", "scope3"},
		},
		{
			name:           "ColonEncodedInScope",
			rawQuery:       "grant_type=authorization_code&client_id=test&client_secret=secret&code=code&code_verifier=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&scope=coder%3Aworkspace.create+coder%3Aworkspace.operate",
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
				form.Set("code_verifier", strings.Repeat("a", 43))
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
				form.Set("code_verifier", strings.Repeat("a", 43))
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
				form.Set("code_verifier", strings.Repeat("a", 43))
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
			// This test only exercises scope parsing, but code_challenge is
			// still required for response_type=code and must satisfy the
			// RFC 7636 §4.1 length floor, so use a valid-length value.
			query.Set("code_challenge", strings.Repeat("a", 43))
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

// TestExtractAuthorizeParams_CodeChallengeFormat ensures a code_challenge is
// rejected at the authorization request (RFC 7636 §4.4.1) when it does not
// meet the same length and character bounds as a code_verifier, rather than
// being stored and failing later at token exchange.
func TestExtractAuthorizeParams_CodeChallengeFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		codeChallenge string
		expectValid   bool
	}{
		{
			name:          "ValidLength",
			codeChallenge: strings.Repeat("a", 43),
			expectValid:   true,
		},
		{
			name:          "TooShort",
			codeChallenge: strings.Repeat("a", 42),
			expectValid:   false,
		},
		{
			name:          "TooLong",
			codeChallenge: strings.Repeat("a", 129),
			expectValid:   false,
		},
		{
			name:          "DisallowedCharacter",
			codeChallenge: strings.Repeat("a", 42) + "+",
			expectValid:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			callbackURL, err := url.Parse("http://localhost:3000/callback")
			require.NoError(t, err)

			query := url.Values{}
			query.Set("response_type", "code")
			query.Set("client_id", "test-client")
			query.Set("redirect_uri", "http://localhost:3000/callback")
			query.Set("code_challenge", tc.codeChallenge)

			reqURL, err := url.Parse("http://localhost:8080/oauth2/authorize?" + query.Encode())
			require.NoError(t, err)

			req := &http.Request{
				Method: http.MethodGet,
				URL:    reqURL,
			}

			_, validationErrs, err := extractAuthorizeParams(req, callbackURL)
			if tc.expectValid {
				require.NoError(t, err)
				require.Empty(t, validationErrs)
			} else {
				require.Error(t, err)
				require.Len(t, validationErrs, 1)
				require.Equal(t, "code_challenge", validationErrs[0].Field)
			}
		})
	}
}

// TestExtractAuthorizeParams_TokenResponseTypeDoesNotRequirePKCE ensures
// response_type=token is parsed without requiring PKCE fields so callers can
// return unsupported_response_type instead of invalid_request.
func TestExtractAuthorizeParams_TokenResponseTypeDoesNotRequirePKCE(t *testing.T) {
	t.Parallel()

	callbackURL, err := url.Parse("http://localhost:3000/callback")
	require.NoError(t, err)

	query := url.Values{}
	query.Set("response_type", string(codersdk.OAuth2ProviderResponseTypeToken))
	query.Set("client_id", "test-client")
	query.Set("redirect_uri", "http://localhost:3000/callback")

	reqURL, err := url.Parse("http://localhost:8080/oauth2/authorize?" + query.Encode())
	require.NoError(t, err)

	req := &http.Request{
		Method: http.MethodGet,
		URL:    reqURL,
	}

	params, validationErrs, err := extractAuthorizeParams(req, callbackURL)
	require.NoError(t, err)
	require.Empty(t, validationErrs)
	require.Equal(t, codersdk.OAuth2ProviderResponseTypeToken, params.responseType)
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
