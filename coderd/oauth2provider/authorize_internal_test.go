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

	// Whether a name is in rbac.IsExternalScope's curated catalog is the point
	// of most cases below, so the two groups are named rather than inlined.
	const (
		inCatalog      = "coder:workspaces.access"
		alsoInCatalog  = "coder:templates.build"
		notInCatalog   = "some_removed_scope"
		neverInCatalog = "openid"
	)

	noAllowlist := sql.NullString{}
	emptyAllowlist := sql.NullString{String: "", Valid: true}

	// wantErr names the branch a rejection must come from, so a refactor
	// cannot silently route one branch through another. wantErrText, where
	// set, additionally pins the rendered text.
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
			// The catalog check does not depend on the allowlist.
			name:      "UnknownRequestedScopeRejectedWithoutAllowlist",
			requested: []string{"not_a_real_scope"},
			appScope:  noAllowlist,
			wantErr:   errUnknownScope,
		},
		{
			// debug_info:read is a real scope RBAC can expand and the enum can
			// store. Only the catalog's curation keeps a client from
			// negotiating an internal-only permission for itself.
			name:      "InternalOnlyScopeRejected",
			requested: []string{"debug_info:read"},
			appScope:  noAllowlist,
			wantErr:   errUnknownScope,
		},
		{
			// "" is exactly what the column's CHECK rejects, so asserting only
			// "no error" would let a DB-level 500 through.
			name:      "NoAllowlistOmittedRequestIsUnrestricted",
			requested: nil,
			appScope:  noAllowlist,
			want:      string(database.ApiKeyScopeCoderAll),
		},
		{
			// '' is the DCR-registered encoding of the unset state NULL
			// expresses for admin-created apps.
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
			// coder:workspaces.access grants template:read but not
			// template:update.
			name:      "PartiallyOutOfAllowlistRejected",
			requested: []string{inCatalog, "template:update"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errScopeNotAllowed,
		},
		{
			// The allowlist bounds authority, not spelling: workspace:ssh is
			// already granted by the composite, so the client gets a token
			// narrower than the ceiling instead of having to ask for the
			// whole composite.
			name:      "LowLevelScopeCoveredByCompositeAllowlistAccepted",
			requested: []string{"workspace:ssh"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			want:      "workspace:ssh",
		},
		{
			// Coverage is per requested name, so a mixed request is refused
			// whole rather than trimmed to its covered part.
			name:      "PartiallyCoveredRequestRejectedWhole",
			requested: []string{"workspace:ssh", "workspace:delete"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errScopeNotAllowed,
		},
		{
			// The wildcard action is wider than the composite covering its
			// read half.
			name:      "WildcardActionNotCoveredByCompositeAllowlist",
			requested: []string{"workspace:*"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errScopeNotAllowed,
		},
		{
			// coder:all expands to the wildcard resource and action.
			name:      "AllAllowlistCoversAnyScope",
			requested: []string{"user_secret:delete"},
			appScope:  sql.NullString{String: string(database.ApiKeyScopeCoderAll), Valid: true},
			want:      "user_secret:delete",
		},
		{
			// The allowlist is one combined ceiling, not a set of independent
			// entries, so a request may draw on more than one at once.
			name:      "CoverageSpansMultipleAllowlistEntries",
			requested: []string{"file:create", "workspace:ssh"},
			appScope:  sql.NullString{String: inCatalog + " " + alsoInCatalog, Valid: true},
			want:      "file:create workspace:ssh",
		},
		{
			// Catalog drift: the stale entry is dropped, the surviving one is
			// still granted.
			name:      "StaleAllowlistEntryDroppedNotGranted",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			// The request's catalog check runs before the allowlist is
			// consulted, so the reason is errUnknownScope rather than
			// errScopeNotAllowed.
			name:      "StaleAllowlistEntryNotRequestableExplicitly",
			requested: []string{notInCatalog},
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			// Falling back to the unrestricted sentinel here would grant
			// strictly more than this allowlist ever permitted.
			name:      "AllowlistFilteringToEmptyRejected",
			requested: nil,
			appScope:  sql.NullString{String: "openid profile email", Valid: true},
			wantErr:   errNoGrantableScope,
		},
		{
			// The accepted compatibility break in its most direct form: a DCR
			// client requesting exactly what it registered.
			name:      "NonCatalogScopeRequestedAsRegistered",
			requested: []string{neverInCatalog},
			appScope:  sql.NullString{String: neverInCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			// A whitespace-only allowlist is configured but grants nothing, so
			// it rejects rather than falling back to unrestricted. The text is
			// pinned because this is the one allowlist whose entries all
			// vanish before the message is built, so naming the filter's input
			// would show the owner an empty string.
			name:        "WhitespaceOnlyAllowlistRejected",
			requested:   nil,
			appScope:    sql.NullString{String: "   ", Valid: true},
			wantErr:     errNoGrantableScope,
			wantErrText: `"   "`,
		},
		{
			// rbac.IsExternalScope accepts `all` as a backward-compatible
			// alias, but the api_key_scope enum has no such member.
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
			// The allowlist is canonicalized on the same terms, so the two
			// spellings match instead of reading as different scopes.
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
			// A space-separated scope denotes a set.
			name:      "DuplicateRequestedScopesDeduplicated",
			requested: []string{inCatalog, inCatalog},
			appScope:  noAllowlist,
			want:      inCatalog,
		},
		{
			// The same holds for the default, which is built from the
			// allowlist rather than from the request.
			name:      "DuplicateAllowlistEntriesDeduplicated",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + inCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			// Two spellings of one scope collapse to a single entry.
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
				// xerrors repeats the wrapped text unless %w is the final
				// verb, which errors.Is cannot catch and a person reading
				// error_description sees.
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
			// The column is NOT NULL with CHECK (scope <> '').
			assert.NotEmpty(t, got, "a successful negotiation must never return an empty scope")
			requirePersistableScope(t, got)
		})
	}
}

// requirePersistableScope asserts that every name in a negotiated scope can
// survive the trip ahead of it: stored as api_key_scope on the authorization
// code, carried to the token, and expanded by RBAC when the key minted from it
// is authorized. Passing the external scope catalog does not imply all three.
func requirePersistableScope(t *testing.T, scope string) {
	t.Helper()

	for _, name := range strings.Fields(scope) {
		require.Contains(t, database.AllAPIKeyScopeValues(), database.APIKeyScope(name),
			"scope %q is not an api_key_scope member, so the column cannot store it", name)

		_, err := rbac.ExpandScope(rbac.ScopeName(name))
		require.NoError(t, err, "scope %q cannot be expanded by RBAC, so it cannot be enforced", name)
	}
}

// Rejection reasons handed to the package's black-box tests, which cannot
// reach the sentinels themselves. Binding here rather than re-typing the
// strings over there keeps a rewording from silently unpinning them.
var (
	ReasonUnknownScope     = errUnknownScope.Error()
	ReasonNoGrantableScope = errNoGrantableScope.Error()
	ReasonScopeNotAllowed  = errScopeNotAllowed.Error()
)

func TestNoScopeAllowlist(t *testing.T) {
	t.Parallel()

	// Both encodings are produced in the tree today: sql.NullString{} by
	// admin-created apps, Valid-with-empty-string by DCR registration that
	// sent no scope.
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

// consentScopes decides the sentence a user reads before approving a grant, so
// the case that matters is the one where a listed name would understate the
// authority being handed over.
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
			// nil, not the name: the page says "full access" instead, which
			// tells a user more than coder:all does.
			name:             "UnrestrictedAloneCollapses",
			granted:          string(database.ApiKeyScopeCoderAll),
			want:             nil,
			wantUnrestricted: true,
		},
		{
			// An allowlist registered as `coder:all coder:workspaces.access`
			// defaults to both names. Listing them would show the very entry
			// this collapse exists to hide, while describing an unrestricted
			// grant as if it were bounded by the other name.
			name:             "UnrestrictedAmongOthersCollapses",
			granted:          string(database.ApiKeyScopeCoderAll) + " coder:workspaces.access",
			want:             nil,
			wantUnrestricted: true,
		},
		{
			// The empty grant reaches no caller today, since negotiateScope
			// returns "" only alongside an error. It is asserted because the
			// two results must disagree here: a grant carrying no permission
			// is the one thing that must never be reported as unrestricted.
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
