package oauth2provider

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestNegotiateScope(t *testing.T) {
	t.Parallel()

	const (
		inCatalog      = "coder:workspaces.access"
		alsoInCatalog  = "coder:templates.build"
		notInCatalog   = "some_removed_scope"
		neverInCatalog = "openid"
	)

	noAllowlist := sql.NullString{}
	emptyAllowlist := sql.NullString{String: "", Valid: true}

	tests := []struct {
		name        string
		requested   []string
		appScope    sql.NullString
		want        string
		wantErr     error
		wantBareErr bool
	}{
		{
			name:      "UnknownRequestedScopeRejected",
			requested: []string{"not_a_real_scope"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			name:      "UnknownRequestedScopeRejectedWithoutAllowlist",
			requested: []string{"not_a_real_scope"},
			appScope:  noAllowlist,
			wantErr:   errUnknownScope,
		},
		{
			// RBAC expands debug_info:read; only the catalog keeps it internal.
			name:      "InternalOnlyScopeRejected",
			requested: []string{"debug_info:read"},
			appScope:  noAllowlist,
			wantErr:   errUnknownScope,
		},
		{
			name:      "NoAllowlistOmittedRequestIsUnrestricted",
			requested: nil,
			appScope:  noAllowlist,
			want:      string(database.ApiKeyScopeCoderAll),
		},
		{
			name:      "EmptyAllowlistBehavesAsNoAllowlist",
			requested: nil,
			appScope:  emptyAllowlist,
			want:      string(database.ApiKeyScopeCoderAll),
		},
		{
			name:      "NoAllowlistExplicitRequestPassesThrough",
			requested: []string{inCatalog},
			appScope:  noAllowlist,
			want:      inCatalog,
		},
		{
			name:      "EmptyAllowlistExplicitRequestPassesThrough",
			requested: []string{inCatalog},
			appScope:  emptyAllowlist,
			want:      inCatalog,
		},
		{
			name:      "OmittedRequestDefaultsToAllowlist",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + alsoInCatalog, Valid: true},
			want:      inCatalog + " " + alsoInCatalog,
		},
		{
			name:      "ExactMatchAccepted",
			requested: []string{inCatalog},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			name:      "GenuineSubsetAccepted",
			requested: []string{alsoInCatalog},
			appScope:  sql.NullString{String: inCatalog + " " + alsoInCatalog, Valid: true},
			want:      alsoInCatalog,
		},
		{
			name:      "PartiallyOutOfAllowlistRejected",
			requested: []string{inCatalog, "template:update"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errScopeNotAllowed,
		},
		{
			name:      "LowLevelScopeCoveredByCompositeAllowlistAccepted",
			requested: []string{"workspace:ssh"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			want:      "workspace:ssh",
		},
		{
			name:      "PartiallyCoveredRequestRejectedWhole",
			requested: []string{"workspace:ssh", "workspace:delete"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errScopeNotAllowed,
		},
		{
			name:      "WildcardActionNotCoveredByCompositeAllowlist",
			requested: []string{"workspace:*"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errScopeNotAllowed,
		},
		{
			name:      "AllAllowlistCoversAnyScope",
			requested: []string{"user_secret:delete"},
			appScope:  sql.NullString{String: string(database.ApiKeyScopeCoderAll), Valid: true},
			want:      "user_secret:delete",
		},
		{
			name:      "CoverageSpansMultipleAllowlistEntries",
			requested: []string{"file:create", "workspace:ssh"},
			appScope:  sql.NullString{String: inCatalog + " " + alsoInCatalog, Valid: true},
			want:      "file:create workspace:ssh",
		},
		{
			name:      "StaleAllowlistEntryDroppedNotGranted",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			name:      "StaleAllowlistEntryNotRequestableExplicitly",
			requested: []string{notInCatalog},
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			name:        "AllowlistFilteringToEmptyRejected",
			requested:   nil,
			appScope:    sql.NullString{String: "openid profile email", Valid: true},
			wantErr:     errNoGrantableScope,
			wantBareErr: true,
		},
		{
			name:      "RegisteredNonCatalogScopeRejected",
			requested: []string{neverInCatalog},
			appScope:  sql.NullString{String: neverInCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			name:        "WhitespaceOnlyAllowlistRejected",
			requested:   nil,
			appScope:    sql.NullString{String: "   ", Valid: true},
			wantErr:     errNoGrantableScope,
			wantBareErr: true,
		},
		{
			name:      "LegacyAllAliasCanonicalized",
			requested: []string{"all"},
			appScope:  noAllowlist,
			want:      "coder:all",
		},
		{
			name:      "LegacyApplicationConnectAliasCanonicalized",
			requested: []string{"application_connect"},
			appScope:  noAllowlist,
			want:      "coder:application_connect",
		},
		{
			name:      "LegacyAliasInAllowlistCoversCanonicalRequest",
			requested: []string{"coder:all"},
			appScope:  sql.NullString{String: "all", Valid: true},
			want:      "coder:all",
		},
		{
			name:      "CanonicalAllowlistCoversLegacyAliasRequest",
			requested: []string{"all"},
			appScope:  sql.NullString{String: "coder:all", Valid: true},
			want:      "coder:all",
		},
		{
			name:      "DuplicateRequestedScopesDeduplicated",
			requested: []string{inCatalog, inCatalog},
			appScope:  noAllowlist,
			want:      inCatalog,
		},
		{
			name:      "DuplicateAllowlistEntriesDeduplicated",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + inCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			name:      "AliasAndCanonicalAllowlistEntriesCollapse",
			requested: nil,
			appScope:  sql.NullString{String: "all coder:all", Valid: true},
			want:      "coder:all",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			app := database.OAuth2ProviderApp{ID: uuid.New(), Scope: test.appScope}
			got, err := negotiateScope(t.Context(), slogtest.Make(t, nil), app, test.requested)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				assert.Empty(t, got, "a rejected request must not return a persistable scope")
				assert.Equal(t, 1, strings.Count(err.Error(), test.wantErr.Error()),
					"the rejection reason must appear once, not doubled by the wrap")
				if test.wantBareErr {
					assert.Equal(t, test.wantErr.Error(), err.Error(),
						"the rejection must not name the app's unvalidated registered scope")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			assert.NotEmpty(t, got, "the code scope column is NOT NULL with CHECK (scope <> '')")
			requirePersistableScope(t, got)
		})
	}
}

// requirePersistableScope asserts every name can be stored as an api_key_scope
// and expanded by RBAC. Passing the external catalog implies neither.
func requirePersistableScope(t *testing.T, scope string) {
	t.Helper()

	for _, name := range strings.Fields(scope) {
		require.Contains(t, database.AllAPIKeyScopeValues(), database.APIKeyScope(name),
			"scope %q is not an api_key_scope member, so the column cannot store it", name)

		_, err := rbac.ExpandScope(rbac.ScopeName(name))
		require.NoError(t, err, "scope %q cannot be expanded by RBAC, so it cannot be enforced", name)
	}
}

// Rejection reasons from authorize.go for the package's black-box tests, which
// cannot reach the sentinels.
var (
	ReasonUnknownScope        = errUnknownScope.Error()
	ReasonNoGrantableScope    = errNoGrantableScope.Error()
	ReasonScopeNotAllowed     = errScopeNotAllowed.Error()
	ReasonCoverageUndecidable = errCoverageUndecidable.Error()
)

// MaxErrorDescription is the description bound for the same black-box tests.
const MaxErrorDescription = maxErrorDescription

// TestGrantableScopesNotSizedByInput pins the shape of the result, not just its
// contents. app.Scope is unvalidated registration metadata read on every
// authorization and redemption, so collecting duplicates and dropping them
// afterwards would size the slice by the input rather than by the catalog.
func TestGrantableScopesNotSizedByInput(t *testing.T) {
	t.Parallel()

	repeated := grantableScopes(strings.Repeat("coder:all ", 4096))
	assert.Equal(t, []string{"coder:all"}, repeated)
	assert.Less(t, cap(repeated), 8, "a repeated name must collapse as it is read")

	assert.Empty(t, grantableScopes(strings.Repeat(" ", 4096)),
		"a value naming no scope grants nothing")
}

func TestNoScopeAllowlist(t *testing.T) {
	t.Parallel()

	assert.True(t, noScopeAllowlist(sql.NullString{}))
	assert.True(t, noScopeAllowlist(sql.NullString{String: "", Valid: true}))
	assert.False(t, noScopeAllowlist(sql.NullString{String: "coder:workspaces.access", Valid: true}))
	assert.False(t, noScopeAllowlist(sql.NullString{String: "   ", Valid: true}))
}

func TestScopeFailureResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		wantCode        codersdk.OAuth2ErrorCode
		wantDescription string
	}{
		{
			name:            "UnknownScope",
			err:             errUnknownScope,
			wantCode:        codersdk.OAuth2ErrorCodeInvalidScope,
			wantDescription: errUnknownScope.Error(),
		},
		{
			name:            "NoGrantableScope",
			err:             errNoGrantableScope,
			wantCode:        codersdk.OAuth2ErrorCodeInvalidScope,
			wantDescription: errNoGrantableScope.Error(),
		},
		{
			name:            "ScopeNotAllowed",
			err:             errScopeNotAllowed,
			wantCode:        codersdk.OAuth2ErrorCodeInvalidScope,
			wantDescription: errScopeNotAllowed.Error(),
		},
		{
			name:            "CoverageUndecidable",
			err:             errCoverageUndecidable,
			wantCode:        codersdk.OAuth2ErrorCodeServerError,
			wantDescription: "The requested scope could not be evaluated",
		},
		{
			// The sentinel still decides the response once wrapped.
			name:            "WrappedCoverageUndecidable",
			err:             xerrors.Errorf("negotiate: %w", errCoverageUndecidable),
			wantCode:        codersdk.OAuth2ErrorCodeServerError,
			wantDescription: "The requested scope could not be evaluated",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			code, description := scopeFailureResponse(test.err)
			require.Equal(t, test.wantCode, code)
			require.Equal(t, test.wantDescription, description)
		})
	}
}

func TestHashOAuth2State(t *testing.T) {
	t.Parallel()

	t.Run("EmptyState", func(t *testing.T) {
		t.Parallel()
		result := hashOAuth2State("")
		assert.False(t, result.Valid, "empty state should return invalid NullString")
		assert.Empty(t, result.String, "empty state should return empty string")
	})

	t.Run("NonEmptyState", func(t *testing.T) {
		t.Parallel()
		state := "test-state-value"
		result := hashOAuth2State(state)
		require.True(t, result.Valid, "non-empty state should return valid NullString")

		// Verify it's a proper SHA-256 hash.
		expected := sha256.Sum256([]byte(state))
		assert.Equal(t, hex.EncodeToString(expected[:]), result.String,
			"state hash should be SHA-256 hex digest")
	})

	t.Run("DifferentStatesProduceDifferentHashes", func(t *testing.T) {
		t.Parallel()
		hash1 := hashOAuth2State("state-a")
		hash2 := hashOAuth2State("state-b")
		require.True(t, hash1.Valid)
		require.True(t, hash2.Valid)
		assert.NotEqual(t, hash1.String, hash2.String,
			"different states should produce different hashes")
	})

	t.Run("SameStateProducesSameHash", func(t *testing.T) {
		t.Parallel()
		hash1 := hashOAuth2State("deterministic")
		hash2 := hashOAuth2State("deterministic")
		require.True(t, hash1.Valid)
		assert.Equal(t, hash1.String, hash2.String,
			"same state should produce identical hash")
	})
}

func TestCapErrorDescription(t *testing.T) {
	t.Parallel()

	t.Run("ShortDescriptionUnchanged", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "unknown or unsupported scope", capErrorDescription("unknown or unsupported scope"))
	})

	t.Run("BoundIsInclusive", func(t *testing.T) {
		t.Parallel()
		atBound := strings.Repeat("x", maxErrorDescription)
		assert.Equal(t, atBound, capErrorDescription(atBound))
	})

	t.Run("LongerDescriptionTruncated", func(t *testing.T) {
		t.Parallel()
		got := capErrorDescription(strings.Repeat("x", maxErrorDescription+1))
		assert.Equal(t, strings.Repeat("x", maxErrorDescription)+" (truncated)", got)
	})
}

func TestSanitizeErrorDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "PlainTextUnchanged",
			description: "Only response_type=code is supported",
			want:        "Only response_type=code is supported",
		},
		{
			// What negotiateScope's %q produces for a well-behaved scope name.
			name:        "QuotedScopeBecomesApostrophes",
			description: `"openid": unknown or unsupported scope`,
			want:        "'openid': unknown or unsupported scope",
		},
		{
			// %q escapes a quote inside the value; the backslash goes with it.
			name:        "EscapedQuoteLosesItsBackslash",
			description: `"\"><img>": unknown or unsupported scope`,
			want:        "''><img>': unknown or unsupported scope",
		},
		{
			// Both NQSCHAR exclusions, and nothing else, are rewritten.
			name:        "BoundaryCharactersSurvive",
			description: "!#[]~ ",
			want:        "!#[]~ ",
		},
		{
			name:        "ControlCharactersBecomeSpaces",
			description: "line\nbreak\ttab",
			want:        "line break tab",
		},
		{
			name:        "NonASCIIBecomesSpace",
			description: "caf\u00e9",
			want:        "caf ",
		},
		{
			name:        "Empty",
			description: "",
			want:        "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeErrorDescription(tc.description)
			assert.Equal(t, tc.want, got)
			for _, r := range got {
				assert.True(t, r == 0x20 || r == 0x21 || (r >= 0x23 && r <= 0x5B) || (r >= 0x5D && r <= 0x7E),
					"%q is outside the NQSCHAR set RFC 6749 Appendix A permits", r)
			}
		})
	}
}

func TestConsentScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		granted          string
		want             []string
		wantUnrestricted bool
	}{
		{
			name:    "NarrowGrantListed",
			granted: "workspace:ssh template:read",
			want:    []string{"workspace:ssh", "template:read"},
		},
		{
			// nil, not the name: the page says "full access" instead.
			name:             "UnrestrictedAloneCollapses",
			granted:          string(database.ApiKeyScopeCoderAll),
			want:             nil,
			wantUnrestricted: true,
		},
		{
			name:             "UnrestrictedAmongOthersCollapses",
			granted:          string(database.ApiKeyScopeCoderAll) + " coder:workspaces.access",
			want:             nil,
			wantUnrestricted: true,
		},
		{
			// Unreachable today: negotiateScope returns "" only with an error.
			name:             "EmptyGrantIsNotUnrestricted",
			granted:          "",
			want:             []string{},
			wantUnrestricted: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			names, unrestricted := consentScopes(test.granted)
			require.Equal(t, test.want, names)
			require.Equal(t, test.wantUnrestricted, unrestricted)
		})
	}
}

// TestNewAuthorizeResponse covers the two preconditions the constructor exists
// to run together, and which of them is the server's fault.
func TestNewAuthorizeResponse(t *testing.T) {
	t.Parallel()

	const registered = "https://app.example.com/callback"

	t.Run("MatchingRedirectURI", func(t *testing.T) {
		t.Parallel()

		p := httpapi.NewQueryParamParser()
		response, err := newAuthorizeResponse(p, url.Values{
			"redirect_uri": {registered},
			"state":        {"abc123"},
		}, registered)

		require.NoError(t, err)
		require.Empty(t, p.Errors)
		require.True(t, response.canRedirect())
		require.Equal(t, registered, response.callbackURL())
		require.Equal(t, "abc123", response.state)
	})

	t.Run("OmittedRedirectURIDefaultsToRegistered", func(t *testing.T) {
		t.Parallel()

		p := httpapi.NewQueryParamParser()
		response, err := newAuthorizeResponse(p, url.Values{}, registered)

		require.NoError(t, err)
		require.Empty(t, p.Errors)
		require.True(t, response.canRedirect())
		require.Equal(t, registered, response.callbackURL())
	})

	t.Run("MismatchedRedirectURIHasNoDestination", func(t *testing.T) {
		t.Parallel()

		p := httpapi.NewQueryParamParser()
		response, err := newAuthorizeResponse(p, url.Values{
			"redirect_uri": {"https://elsewhere.example/cb"},
		}, registered)

		// The client's mistake, so it joins the parser's other errors rather
		// than becoming a server fault.
		require.NoError(t, err)
		require.Len(t, p.Errors, 1)
		require.Equal(t, "redirect_uri", p.Errors[0].Field)
		require.False(t, response.canRedirect(),
			"a URI that failed the match must not become a destination")
	})

	t.Run("DangerousClientSchemeIsNotTheServersFault", func(t *testing.T) {
		t.Parallel()

		p := httpapi.NewQueryParamParser()
		response, err := newAuthorizeResponse(p, url.Values{
			"redirect_uri": {"javascript:alert(1)"},
		}, registered)

		require.NoError(t, err, "the app registered a usable callback; the client did not send one")
		require.NotEmpty(t, p.Errors)
		require.False(t, response.canRedirect())
	})

	t.Run("DangerousRegisteredSchemeIsServerState", func(t *testing.T) {
		t.Parallel()

		p := httpapi.NewQueryParamParser()
		response, err := newAuthorizeResponse(p, url.Values{}, "javascript:alert(1)")

		require.Error(t, err)
		require.Empty(t, p.Errors, "the registration is rejected before any parameter is read")
		require.False(t, response.canRedirect())
	})

	t.Run("UnparsableRegisteredCallbackIsServerState", func(t *testing.T) {
		t.Parallel()

		p := httpapi.NewQueryParamParser()
		response, err := newAuthorizeResponse(p, url.Values{}, "http://a b")

		require.Error(t, err, "a registration that does not parse is the same class as one this server rejects")
		require.Empty(t, p.Errors)
		require.False(t, response.canRedirect())
	})
}

// TestAuthorizeResponseZeroValue pins the zero value as inert, since it is what
// a failure with no deliverable destination carries.
func TestAuthorizeResponseZeroValue(t *testing.T) {
	t.Parallel()

	require.False(t, authorizeResponse{}.canRedirect())
	require.False(t, (&authorizeFailure{}).redirect.canRedirect())
	require.NotContains(t, fmt.Sprintf("%v", authorizeResponse{}), "PANIC",
		"a String method on this type would panic through fmt on every failure path")
}

// TestFailureKind pins the precedence both handlers dispatch on, in the one
// place it is now written.
func TestFailureKind(t *testing.T) {
	t.Parallel()

	deliverable := authorizeResponse{callback: &url.URL{Scheme: "https", Host: "app.example.com"}}
	unusable := xerrors.New("registered callback is not usable")

	require.Equal(t, failureAnswerHere, authorizeFailure{}.kind())
	require.Equal(t, failureDeliverToClient, authorizeFailure{redirect: deliverable}.kind())
	require.Equal(t, failureCorruptRegistration, authorizeFailure{corruptCallback: unusable}.kind())
	require.Equal(t, failureCorruptRegistration,
		authorizeFailure{corruptCallback: unusable, redirect: deliverable}.kind(),
		"the registration is what a Location header would be trusting, so its failure outranks a usable callback")
}

// TestCarveOutDelivery pins which failures reach the client's callback and which
// stay here, the two RFC 6749 §4.1.2.1 carve-outs: an unsettled client identity
// and a redirect URI at fault.
func TestCarveOutDelivery(t *testing.T) {
	t.Parallel()

	app := database.OAuth2ProviderApp{
		ID:          uuid.MustParse("6f1a9c30-0d6b-4f8e-9a71-2c4b83f0ab12"),
		CallbackURL: "https://app.example.com/callback",
	}

	valid := func() url.Values {
		return url.Values{
			"client_id":             {app.ID.String()},
			"response_type":         {"code"},
			"redirect_uri":          {"https://app.example.com/callback"},
			"code_challenge":        {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"},
			"code_challenge_method": {"S256"},
			"state":                 {"xyz"},
		}
	}

	cases := []struct {
		name    string
		mutate  func(url.Values)
		deliver bool
	}{
		{
			// Repeated: the callback was matched against one of two candidates.
			name:    "RepeatedClientID",
			mutate:  func(v url.Values) { v["client_id"] = []string{app.ID.String(), app.ID.String()} },
			deliver: false,
		},
		{
			name: "ClientIDIsNotTheResolvedApp",
			mutate: func(v url.Values) {
				v.Set("client_id", uuid.NewString())
				v.Set("code_challenge", "short")
			},
			deliver: false,
		},
		{
			// httpmw resolved the app from the POST form body, which this
			// parser never reads. The identity is not in doubt.
			name:    "ClientIDAbsentFromTheQuery",
			mutate:  func(v url.Values) { v.Del("client_id") },
			deliver: true,
		},
		{
			// RFC 6749 §3.1: sent without a value is the absent case above, so
			// httpmw resolved the same way and the failure is as deliverable.
			name:    "ClientIDValuelessInTheQuery",
			mutate:  func(v url.Values) { v.Set("client_id", "") },
			deliver: true,
		},
		{
			// Every value valueless, so httpmw had one candidate, not several.
			name:    "ClientIDRepeatedAndValueless",
			mutate:  func(v url.Values) { v["client_id"] = []string{"", ""} },
			deliver: true,
		},
		{
			// uuid.Parse accepts this and httpmw resolved through it.
			name: "ClientIDInANonCanonicalSpelling",
			mutate: func(v url.Values) {
				v.Set("client_id", "{"+strings.ToUpper(app.ID.String())+"}")
				v.Set("code_challenge", "short")
			},
			deliver: true,
		},
		{
			name:    "FailureOutsideTheIdentity",
			mutate:  func(v url.Values) { v.Set("code_challenge", "short") },
			deliver: true,
		},
		{
			// The state read shares a function with the redirect_uri match, so
			// this used to be charged to the redirect_uri carve-out.
			name:    "RepeatedState",
			mutate:  func(v url.Values) { v["state"] = []string{"xyz", "xyz"} },
			deliver: true,
		},
		{
			name:    "RedirectURIDoesNotMatchTheRegistration",
			mutate:  func(v url.Values) { v.Set("redirect_uri", "https://attacker.example.com/callback") },
			deliver: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vals := valid()
			tc.mutate(vals)
			r := httptest.NewRequest(http.MethodGet, "/oauth2/authorize?"+vals.Encode(), nil)

			_, failure := extractAuthorizeParams(r, slogtest.Make(t, nil), app)
			require.NotNil(t, failure)
			require.Equal(t, tc.deliver, failure.redirect.canRedirect())
		})
	}
}
