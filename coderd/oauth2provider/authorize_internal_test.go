package oauth2provider

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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

	tests := []struct {
		name      string
		requested []string
		appScope  sql.NullString
		want      string
		wantErr   bool
	}{
		{
			name:      "UnknownRequestedScopeRejected",
			requested: []string{"not_a_real_scope"},
			appScope:  sql.NullString{String: inCatalog, Valid: true},
			wantErr:   true,
		},
		{
			// The catalog check does not depend on the allowlist, so an
			// unknown scope is rejected even where there is nothing to
			// check it against.
			name:      "UnknownRequestedScopeRejectedWithoutAllowlist",
			requested: []string{"not_a_real_scope"},
			appScope:  noAllowlist,
			wantErr:   true,
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
			wantErr:   true,
		},
		{
			// AC3/AC16: the literal return value matters. "" is exactly what
			// the column's CHECK rejects, so asserting only "no error" would
			// let a DB-level 500 through.
			name:      "NoAllowlistOmittedRequestIsUnrestricted",
			requested: nil,
			appScope:  noAllowlist,
			want:      database.OAuth2ScopeUnrestricted,
		},
		{
			// AC16/Edge Case 22: '' is the DCR-registered encoding of the
			// same "no allowlist configured" state NULL expresses for
			// admin-created apps. Both must reach the same branch.
			name:      "EmptyAllowlistBehavesAsNoAllowlist",
			requested: nil,
			appScope:  emptyAllowlist,
			want:      database.OAuth2ScopeUnrestricted,
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
			wantErr:   true,
		},
		{
			// Edge Case 20: catalog drift. The stale entry is dropped by the
			// filter, and the surviving entry is still granted.
			name:      "StaleAllowlistEntryDroppedNotGranted",
			requested: nil,
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			want:      inCatalog,
		},
		{
			// The filter applies to the subset check too, so a dropped entry
			// cannot be reached by requesting it explicitly either.
			name:      "StaleAllowlistEntryNotRequestableExplicitly",
			requested: []string{notInCatalog},
			appScope:  sql.NullString{String: inCatalog + " " + notInCatalog, Valid: true},
			wantErr:   true,
		},
		{
			// AC15/Edge Case 19: the all-entries-dropped counterpart to the
			// case above. Falling back to the unrestricted sentinel here would
			// grant strictly more than this allowlist ever permitted.
			name:      "AllowlistFilteringToEmptyRejected",
			requested: nil,
			appScope:  sql.NullString{String: "openid profile email", Valid: true},
			wantErr:   true,
		},
		{
			// §4.2.2's compatibility break, in its most direct form: a DCR
			// client requesting exactly what it registered.
			name:      "NonCatalogScopeRequestedAsRegistered",
			requested: []string{neverInCatalog},
			appScope:  sql.NullString{String: neverInCatalog, Valid: true},
			wantErr:   true,
		},
		{
			// A whitespace-only allowlist is a configured value that grants
			// nothing, not an unset one, so it rejects rather than falling
			// back to unrestricted.
			name:      "WhitespaceOnlyAllowlistRejected",
			requested: nil,
			appScope:  sql.NullString{String: "   ", Valid: true},
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := validateRequestedScope(test.requested, test.appScope)
			if test.wantErr {
				require.Error(t, err)
				assert.Empty(t, got, "a rejected request must not return a persistable scope")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			// The return value goes straight to a NOT NULL column carrying
			// CHECK (scope <> ''), so an empty success is never legal.
			assert.NotEmpty(t, got, "a successful negotiation must never return an empty scope")
		})
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
