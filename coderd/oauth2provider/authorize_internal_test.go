package oauth2provider

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
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
		wantErrText string
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
			name:      "AllowlistFilteringToEmptyRejected",
			requested: nil,
			appScope:  sql.NullString{String: "openid profile email", Valid: true},
			wantErr:   errNoGrantableScope,
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
			wantErrText: `"   "`,
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
				if test.wantErrText != "" {
					assert.Contains(t, err.Error(), test.wantErrText,
						"the rejection must name the value the app owner has to change")
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

// Rejection reasons for the package's black-box tests, which cannot reach the
// sentinels.
var (
	ReasonUnknownScope     = errUnknownScope.Error()
	ReasonNoGrantableScope = errNoGrantableScope.Error()
	ReasonScopeNotAllowed  = errScopeNotAllowed.Error()
)

func TestNoScopeAllowlist(t *testing.T) {
	t.Parallel()

	assert.True(t, noScopeAllowlist(sql.NullString{}))
	assert.True(t, noScopeAllowlist(sql.NullString{String: "", Valid: true}))
	assert.False(t, noScopeAllowlist(sql.NullString{String: "coder:workspaces.access", Valid: true}))
	assert.False(t, noScopeAllowlist(sql.NullString{String: "   ", Valid: true}))
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
