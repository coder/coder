package oauth2provider

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestValidateRequestedScope(t *testing.T) {
	t.Parallel()

	// Every scope name below is either in rbac.IsExternalScope's curated
	// catalog or deliberately outside it; the test's meaning depends on which,
	// so they are named rather than inlined.
	const (
		inCatalog      = "coder:workspaces.access"
		alsoInCatalog  = "coder:templates.build"
		notInCatalog   = "some_removed_scope"
		neverInCatalog = "openid"
	)

	noAllowlist := sql.NullString{}
	emptyAllowlist := sql.NullString{String: "", Valid: true}

	// wantErr names the branch a rejection must come from. The three reasons
	// are separately reachable and separately meaningful, so asserting only
	// that some error occurred would let a refactor route one branch through
	// another unnoticed.
	tests := []struct {
		name      string
		requested []string
		appScope  sql.NullString
		want      string
		wantErr   error
	}{
		{
			name:      "UnknownRequestedScopeRejected",
			requested: []string{"not_a_real_scope"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			// The catalog check does not depend on the allowlist, so an
			// unknown scope is rejected even where there is nothing to
			// check it against.
			name:      "UnknownRequestedScopeRejectedWithoutAllowlist",
			requested: []string{"not_a_real_scope"},
			appScope:  noAllowlist,
			wantErr:   errUnknownScope,
		},
		{
			// A different rejection from the case above, and the one that
			// matters more: debug_info:read is a real scope RBAC can expand
			// and the api_key_scope enum can store. Only the catalog's
			// curation keeps a client from negotiating an internal-only
			// permission for itself.
			name:      "InternalOnlyScopeRejected",
			requested: []string{"debug_info:read"},
			appScope:  noAllowlist,
			wantErr:   errUnknownScope,
		},
		{
			// The literal return value matters. "" is exactly what
			// the column's CHECK rejects, so asserting only "no error" would
			// let a DB-level 500 through.
			name:      "NoAllowlistOmittedRequestIsUnrestricted",
			requested: nil,
			appScope:  noAllowlist,
			want:      string(database.ApiKeyScopeCoderAll),
		},
		{
			// '' is the DCR-registered encoding of the same "no allowlist
			// configured" state NULL expresses for admin-created apps. Both must reach the same branch.
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
			// RFC 6749 §3.3: an omitted scope defaults to the app's allowlist.
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
			requested: []string{inCatalog, "template:read"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   errScopeNotAllowed,
		},
		{
			// Catalog drift. The stale entry is dropped by the filter, and
			// the surviving entry is still granted.
			name:      "StaleAllowlistEntryDroppedNotGranted",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			// A dropped entry cannot be reached by requesting it explicitly
			// either. The catalog check on the request rejects it before the
			// allowlist is consulted at all, which is why the reason here is
			// errUnknownScope and not errScopeNotAllowed.
			name:      "StaleAllowlistEntryNotRequestableExplicitly",
			requested: []string{notInCatalog},
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			// The all-entries-dropped counterpart to the case above. Falling back to the unrestricted sentinel here would
			// grant strictly more than this allowlist ever permitted.
			name:      "AllowlistFilteringToEmptyRejected",
			requested: nil,
			appScope:  sql.NullString{String: "openid profile email", Valid: true},
			wantErr:   errNoGrantableScope,
		},
		{
			// The accepted compatibility break in its most direct form: a
			// DCR client requesting exactly what it registered.
			name:      "NonCatalogScopeRequestedAsRegistered",
			requested: []string{neverInCatalog},
			appScope:  sql.NullString{String: neverInCatalog, Valid: true},
			wantErr:   errUnknownScope,
		},
		{
			// A whitespace-only allowlist is a configured value that grants
			// nothing, not an unset one, so it rejects rather than falling
			// back to unrestricted.
			name:      "WhitespaceOnlyAllowlistRejected",
			requested: nil,
			appScope:  sql.NullString{String: "   ", Valid: true},
			wantErr:   errNoGrantableScope,
		},
		{
			// rbac.IsExternalScope accepts `all` as a backward-compatible
			// alias, but the api_key_scope enum has no such member, so
			// persisting the requested spelling verbatim would store a value
			// outside the column's vocabulary.
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
			// spellings of one scope match across the subset check rather
			// than reading as different scopes.
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
			// A space-separated scope denotes a set, so a repeated request
			// stores one entry rather than two.
			name:      "DuplicateRequestedScopesDeduplicated",
			requested: []string{inCatalog, inCatalog},
			appScope:  noAllowlist,
			want:      inCatalog,
		},
		{
			// The same holds for the RFC 6749 §3.3 default, which is built
			// from the allowlist rather than from the request.
			name:      "DuplicateAllowlistEntriesDeduplicated",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + inCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			// Two spellings of one scope in the allowlist collapse to one
			// entry, so the default does not name the same grant twice.
			name:      "AliasAndCanonicalAllowlistEntriesCollapse",
			requested: nil,
			appScope:  sql.NullString{String: "all coder:all", Valid: true},
			want:      "coder:all",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateRequestedScope(test.requested, test.appScope)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				assert.Empty(t, got, "a rejected request must not return a persistable scope")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			// The return value goes straight to a NOT NULL column carrying
			// CHECK (scope <> ''), so an empty success is never legal.
			assert.NotEmpty(t, got, "a successful negotiation must never return an empty scope")
			requirePersistableScope(t, got)
		})
	}
}

// requirePersistableScope asserts that every name in a negotiated scope can
// survive the trip the value is about to take: stored as api_key_scope on the
// authorization code, carried to the token, and expanded by RBAC when the key
// minted from it is authorized. A name that passes the external scope catalog
// is not automatically one that clears all three, which is why this is
// asserted on the result rather than assumed from the input.
func requirePersistableScope(t *testing.T, scope string) {
	t.Helper()

	for _, name := range strings.Fields(scope) {
		require.Contains(t, database.AllAPIKeyScopeValues(), database.APIKeyScope(name),
			"scope %q is not an api_key_scope member, so the column cannot store it", name)

		_, err := rbac.ExpandScope(rbac.ScopeName(name))
		require.NoError(t, err, "scope %q cannot be expanded by RBAC, so it cannot be enforced", name)
	}
}

func TestNoScopeAllowlist(t *testing.T) {
	t.Parallel()

	// NULL and '' are one state. Both are produced in the tree today:
	// sql.NullString{} by admin-created apps, Valid-with-empty-string by DCR
	// registration that sent no scope.
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
