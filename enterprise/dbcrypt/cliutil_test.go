package dbcrypt_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
)

// findAIProviderByID fetches a provider row by ID without filtering
// out soft-deleted rows. The plain GetAIProviderByID filters them out,
// which is the wrong shape for tests that verify rotation behavior
// across the live/deleted boundary.
func findAIProviderByID(ctx context.Context, t *testing.T, db database.Store, id uuid.UUID) database.AIProvider {
	t.Helper()
	all, err := db.GetAIProviders(ctx, database.GetAIProvidersParams{})
	require.NoError(t, err)
	idx := slices.IndexFunc(all, func(p database.AIProvider) bool { return p.ID == id })
	require.GreaterOrEqual(t, idx, 0, "provider %s not found", id)
	return all[idx]
}

// TestRotateAIProviders verifies that dbcrypt.Rotate re-encrypts the
// settings column of every ai_providers row, including soft-deleted
// rows that still hold a foreign-key reference to dbcrypt_keys, so
// the old keys can be revoked without violating the FK constraint.
//
// This is a regression test for an incident where rotating ciphers
// failed with:
//
//	error: rotate ciphers: revoke key: pq: update or delete on
//	  table "dbcrypt_keys" violates foreign key constraint
//	  "ai_providers_settings_key_id_fkey" on table "ai_providers"
func TestRotateAIProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	oldCipher := newCipher(t)
	newCipher := newCipher(t)

	// Encrypt rows under the old cipher.
	cdb, err := dbcrypt.New(ctx, rawDB, oldCipher)
	require.NoError(t, err)

	const (
		settingsAlive  = `{"r":"alive"}` //nolint:gosec // test fixture
		settingsDelete = `{"r":"dead"}`  //nolint:gosec // test fixture
	)

	alive := dbgen.AIProvider(t, cdb, database.AIProvider{
		Name:     "alive",
		Settings: sql.NullString{String: settingsAlive, Valid: true},
	})
	deleted := dbgen.AIProvider(t, cdb, database.AIProvider{
		Name:     "soft-deleted",
		Settings: sql.NullString{String: settingsDelete, Valid: true},
	})
	_, err = rawDB.DeleteAIProviderByID(ctx, deleted.ID)
	require.NoError(t, err)

	// Both rows should have key IDs pointing at the old cipher's
	// digest before rotation.
	beforeAlive, err := rawDB.GetAIProviderByID(ctx, alive.ID)
	require.NoError(t, err)
	require.Equal(t, oldCipher.HexDigest(), beforeAlive.SettingsKeyID.String)

	// Soft-deleted rows are filtered by GetAIProviderByID, so look
	// the row up via the unfiltered listing.
	beforeDeleted := findAIProviderByID(ctx, t, rawDB, deleted.ID)
	require.Equal(t, oldCipher.HexDigest(), beforeDeleted.SettingsKeyID.String)

	// Rotate to the new primary cipher, with the old cipher still
	// available for decryption of pre-rotation rows.
	require.NoError(t, dbcrypt.Rotate(
		ctx,
		slogtest.Make(t, nil),
		sqlDB,
		[]dbcrypt.Cipher{newCipher, oldCipher},
	))

	// All rows must now reference the new digest.
	afterAlive, err := rawDB.GetAIProviderByID(ctx, alive.ID)
	require.NoError(t, err)
	require.Equal(t, newCipher.HexDigest(), afterAlive.SettingsKeyID.String)

	afterDeleted := findAIProviderByID(ctx, t, rawDB, deleted.ID)
	require.Equal(t, newCipher.HexDigest(), afterDeleted.SettingsKeyID.String)

	// Decrypted plaintext is still accessible and unchanged.
	postRotateCDB, err := dbcrypt.New(ctx, rawDB, newCipher)
	require.NoError(t, err)
	gotAlive, err := postRotateCDB.GetAIProviderByID(ctx, alive.ID)
	require.NoError(t, err)
	require.Equal(t, settingsAlive, gotAlive.Settings.String)

	gotDeleted := findAIProviderByID(ctx, t, postRotateCDB, deleted.ID)
	require.True(t, gotDeleted.Deleted)
	require.Equal(t, settingsDelete, gotDeleted.Settings.String)

	// The old cipher must have been successfully revoked.
	keys, err := rawDB.GetDBCryptKeys(ctx)
	require.NoError(t, err)
	for _, k := range keys {
		if k.ActiveKeyDigest.String == oldCipher.HexDigest() {
			t.Fatalf("expected old key to be revoked, but it is still active: %#v", k)
		}
	}

	// Re-running Rotate is a no-op (no FK violation, no errors).
	require.NoError(t, dbcrypt.Rotate(
		ctx,
		slogtest.Make(t, nil),
		sqlDB,
		[]dbcrypt.Cipher{newCipher},
	))
}

// TestRotateAIProviderKeys verifies that dbcrypt.Rotate re-encrypts
// every ai_provider_keys row, including those belonging to a
// soft-deleted parent provider, so the old dbcrypt keys can be
// revoked without violating the FK constraint.
func TestRotateAIProviderKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	oldCipher := newCipher(t)
	newCipher := newCipher(t)

	cdb, err := dbcrypt.New(ctx, rawDB, oldCipher)
	require.NoError(t, err)

	const (
		apiKeyAlive   = "sk-alive"   //nolint:gosec // test fixture
		apiKeyDeleted = "sk-deleted" //nolint:gosec // test fixture
	)

	aliveProvider := dbgen.AIProvider(t, cdb, database.AIProvider{Name: "alive-keys"})
	aliveKey := dbgen.AIProviderKey(t, cdb, database.AIProviderKey{
		ProviderID: aliveProvider.ID,
		APIKey:     apiKeyAlive,
	})

	deletedProvider := dbgen.AIProvider(t, cdb, database.AIProvider{Name: "deleted-keys"})
	deletedKey := dbgen.AIProviderKey(t, cdb, database.AIProviderKey{
		ProviderID: deletedProvider.ID,
		APIKey:     apiKeyDeleted,
	})
	_, err = rawDB.DeleteAIProviderByID(ctx, deletedProvider.ID)
	require.NoError(t, err)

	beforeAlive, err := rawDB.GetAIProviderKeyByID(ctx, aliveKey.ID)
	require.NoError(t, err)
	require.Equal(t, oldCipher.HexDigest(), beforeAlive.ApiKeyKeyID.String)

	beforeDeleted, err := rawDB.GetAIProviderKeyByID(ctx, deletedKey.ID)
	require.NoError(t, err)
	require.Equal(t, oldCipher.HexDigest(), beforeDeleted.ApiKeyKeyID.String)

	require.NoError(t, dbcrypt.Rotate(
		ctx,
		slogtest.Make(t, nil),
		sqlDB,
		[]dbcrypt.Cipher{newCipher, oldCipher},
	))

	afterAlive, err := rawDB.GetAIProviderKeyByID(ctx, aliveKey.ID)
	require.NoError(t, err)
	require.Equal(t, newCipher.HexDigest(), afterAlive.ApiKeyKeyID.String)

	afterDeleted, err := rawDB.GetAIProviderKeyByID(ctx, deletedKey.ID)
	require.NoError(t, err)
	require.Equal(t, newCipher.HexDigest(), afterDeleted.ApiKeyKeyID.String)

	postRotateCDB, err := dbcrypt.New(ctx, rawDB, newCipher)
	require.NoError(t, err)
	gotAlive, err := postRotateCDB.GetAIProviderKeyByID(ctx, aliveKey.ID)
	require.NoError(t, err)
	require.Equal(t, apiKeyAlive, gotAlive.APIKey)

	gotDeleted, err := postRotateCDB.GetAIProviderKeyByID(ctx, deletedKey.ID)
	require.NoError(t, err)
	require.Equal(t, apiKeyDeleted, gotDeleted.APIKey)

	keys, err := rawDB.GetDBCryptKeys(ctx)
	require.NoError(t, err)
	for _, k := range keys {
		if k.ActiveKeyDigest.String == oldCipher.HexDigest() {
			t.Fatalf("expected old key to be revoked, but it is still active: %#v", k)
		}
	}
}

// TestDecryptAIProviders verifies that dbcrypt.Decrypt clears the
// FK references on every ai_providers and ai_provider_keys row
// (including soft-deleted parents) so all keys can be revoked.
func TestDecryptAIProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	cipher := newCipher(t)

	cdb, err := dbcrypt.New(ctx, rawDB, cipher)
	require.NoError(t, err)

	provider := dbgen.AIProvider(t, cdb, database.AIProvider{
		Name:     "to-decrypt",
		Settings: sql.NullString{String: `{"r":"x"}`, Valid: true},
	})
	key := dbgen.AIProviderKey(t, cdb, database.AIProviderKey{
		ProviderID: provider.ID,
		APIKey:     "sk-secret", //nolint:gosec // test fixture
	})

	require.NoError(t, dbcrypt.Decrypt(
		ctx,
		slogtest.Make(t, nil),
		sqlDB,
		[]dbcrypt.Cipher{cipher},
	))

	gotProvider, err := rawDB.GetAIProviderByID(ctx, provider.ID)
	require.NoError(t, err)
	require.False(t, gotProvider.SettingsKeyID.Valid)
	require.Equal(t, `{"r":"x"}`, gotProvider.Settings.String)

	gotKey, err := rawDB.GetAIProviderKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.False(t, gotKey.ApiKeyKeyID.Valid)
	require.Equal(t, "sk-secret", gotKey.APIKey)
}

// TestDeleteAIProviders verifies that dbcrypt.Delete wipes the
// encrypted columns of every ai_providers row and removes every
// ai_provider_keys row that held an encrypted api_key.
func TestDeleteAIProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	cipher := newCipher(t)

	cdb, err := dbcrypt.New(ctx, rawDB, cipher)
	require.NoError(t, err)

	provider := dbgen.AIProvider(t, cdb, database.AIProvider{
		Name:     "to-delete",
		Settings: sql.NullString{String: `{"r":"x"}`, Valid: true},
	})
	key := dbgen.AIProviderKey(t, cdb, database.AIProviderKey{
		ProviderID: provider.ID,
		APIKey:     "sk-secret", //nolint:gosec // test fixture
	})

	require.NoError(t, dbcrypt.Delete(
		ctx,
		slogtest.Make(t, nil),
		sqlDB,
	))

	gotProvider, err := rawDB.GetAIProviderByID(ctx, provider.ID)
	require.NoError(t, err)
	require.False(t, gotProvider.SettingsKeyID.Valid)
	require.False(t, gotProvider.Settings.Valid)

	// The encrypted key row should be removed entirely.
	gotKeys, err := rawDB.GetAIProviderKeysByProviderID(ctx, provider.ID)
	require.NoError(t, err)
	require.Empty(t, gotKeys)
	_, err = rawDB.GetAIProviderKeyByID(ctx, key.ID)
	require.Error(t, err, "expected the encrypted key row to be removed")
}

// newCipher returns a fresh AES-256 dbcrypt.Cipher seeded with random
// data. Each call yields a distinct digest so it can be used to
// represent old vs new keys in rotation tests.
func newCipher(t *testing.T) dbcrypt.Cipher {
	t.Helper()
	key, err := cryptorand.String(32)
	require.NoError(t, err)
	cs, err := dbcrypt.NewCiphers([]byte(key))
	require.NoError(t, err)
	require.Len(t, cs, 1)
	return cs[0]
}
