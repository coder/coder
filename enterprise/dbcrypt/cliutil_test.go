package dbcrypt_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/testutil"
)

// rotateFixture provisions an isolated Postgres database plus two
// independent ciphers ("A" and "B") used to exercise a single rotation.
// Fixtures are seeded through cryptDBA so seeded rows start out genuinely
// encrypted under cipher A, mirroring a deployment that already has
// encryption enabled and is rotating to a new key.
type rotateFixture struct {
	ctx      context.Context
	rawDB    database.Store
	sqlDB    *sql.DB
	cipherA  dbcrypt.Cipher
	cipherB  dbcrypt.Cipher
	cryptDBA database.Store
}

func newRotateFixture(t *testing.T) *rotateFixture {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	ciphersA, err := dbcrypt.NewCiphers([]byte(testutil.MustRandString(t, 32)))
	require.NoError(t, err)
	ciphersB, err := dbcrypt.NewCiphers([]byte(testutil.MustRandString(t, 32)))
	require.NoError(t, err)

	cryptDBA, err := dbcrypt.New(ctx, rawDB, ciphersA...)
	require.NoError(t, err)

	return &rotateFixture{
		ctx:      ctx,
		rawDB:    rawDB,
		sqlDB:    sqlDB,
		cipherA:  ciphersA[0],
		cipherB:  ciphersB[0],
		cryptDBA: cryptDBA,
	}
}

// rotateErr moves every dbcrypt-managed row from cipherA to cipherB and
// returns whatever error Rotate produces, including from the trailing
// RevokeDBCryptKey(cipherA) call that Rotate issues internally. That revoke
// is where the #25381 bug surfaces: it fails with a foreign key violation
// if any row anywhere still references cipherA's digest.
func (f *rotateFixture) rotateErr(t *testing.T) error {
	t.Helper()
	return dbcrypt.Rotate(f.ctx, testutil.Logger(t), f.sqlDB, []dbcrypt.Cipher{f.cipherB, f.cipherA})
}

// rotate is rotateErr for tests that expect the rotation to succeed.
func (f *rotateFixture) rotate(t *testing.T) {
	t.Helper()
	err := f.rotateErr(t)
	require.NoError(t, err, "rotate should succeed and cleanly revoke the old key")
}

// newCipher returns a fresh, independent cipher unrelated to a fixture's
// cipherA/cipherB, for tests that need to seed data under a key Rotate is
// never told about.
func newCipher(t *testing.T) dbcrypt.Cipher {
	t.Helper()
	ciphers, err := dbcrypt.NewCiphers([]byte(testutil.MustRandString(t, 32)))
	require.NoError(t, err)
	return ciphers[0]
}

// registerCipher builds a dbcrypt-wrapped store with c as its primary
// cipher, seeded rows are encrypted under c, but also loads cipherA so
// construction can decrypt cipherA's existing dbcrypt_keys canary (every
// dbcrypt-managed column has a foreign key to dbcrypt_keys, so a row can
// only reference c's digest once c itself has a dbcrypt_keys row; and
// building any dbcrypt-wrapped store requires decrypting the canary of
// every currently active key, not just the one it intends to write with).
// Used to register and seed data under a cipher that Rotate is
// deliberately never told about.
func (f *rotateFixture) registerCipher(t *testing.T, c dbcrypt.Cipher) database.Store {
	t.Helper()
	wrapped, err := dbcrypt.New(f.ctx, f.rawDB, c, f.cipherA)
	require.NoError(t, err)
	return wrapped
}

// upsertUserAIProviderKey inserts a user_ai_provider_keys row through the
// given store. There is no dbgen helper for this table, so this wraps the
// raw UpsertUserAIProviderKey call for reuse across tests.
func upsertUserAIProviderKey(ctx context.Context, t *testing.T, store database.Store, userID, providerID uuid.UUID, apiKey string) database.UserAIProviderKey {
	t.Helper()
	now := time.Now()
	key, err := store.UpsertUserAIProviderKey(ctx, database.UpsertUserAIProviderKeyParams{
		ID:           uuid.New(),
		UserID:       userID,
		AIProviderID: providerID,
		APIKey:       apiKey,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	return key
}

// decryptRawString decodes and decrypts a raw (base64) ciphertext value read
// directly from the database, for comparison against the original plaintext.
func decryptRawString(t *testing.T, c dbcrypt.Cipher, raw string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(raw)
	require.NoError(t, err, "raw value must be valid base64 ciphertext")
	plain, err := c.Decrypt(data)
	require.NoError(t, err, "must decrypt with the expected cipher")
	return string(plain)
}

// TestRotateUserLinks covers the user_links table (OAuth login access and
// refresh tokens). Simulates an operator routinely rotating from an
// existing key to a new one on a deployment that already has encryption
// enabled:
//
//	coder server dbcrypt rotate \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --new-key <base64 key B> \
//	  --old-keys <base64 key A>
func TestRotateUserLinks(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		// user_links cannot exist for a deleted user, so only a live row is
		// exercised here.
		user := dbgen.User(t, f.rawDB, database.User{})
		const wantAccess = "access-token"
		const wantRefresh = "refresh-token"
		seeded := dbgen.UserLink(t, f.cryptDBA, database.UserLink{
			UserID:            user.ID,
			LoginType:         user.LoginType,
			OAuthAccessToken:  wantAccess,
			OAuthRefreshToken: wantRefresh,
		})
		require.Equal(t, f.cipherA.HexDigest(), seeded.OAuthAccessTokenKeyID.String, "sanity check: seed must be encrypted under cipher A")

		f.rotate(t)

		links, err := f.rawDB.GetUserLinksByUserID(f.ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, links, 1)
		link := links[0]
		require.Equal(t, f.cipherB.HexDigest(), link.OAuthAccessTokenKeyID.String)
		require.Equal(t, f.cipherB.HexDigest(), link.OAuthRefreshTokenKeyID.String)
		require.Equal(t, wantAccess, decryptRawString(t, f.cipherB, link.OAuthAccessToken))
		require.Equal(t, wantRefresh, decryptRawString(t, f.cipherB, link.OAuthRefreshToken))
	})

	// DecryptErr simulates an operator omitting an old key from --old-keys.
	// A user_links row is encrypted under cipher C, which is registered in
	// dbcrypt_keys (required for the row to exist at all, thanks to its
	// foreign key) but never passed to the rotation below. Building any
	// dbcrypt-wrapped store, including the one Rotate builds internally,
	// requires decrypting the dbcrypt_keys canary of every currently active
	// key, not just the ones the caller intends to use. So this fails
	// immediately with a DecryptFailedError before Rotate reads a single
	// row, and leaves every table, including this one, untouched.
	//
	//	# cipher C's key is missing from --old-keys here on purpose:
	//	coder server dbcrypt rotate \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --new-key <base64 key B> \
	//	  --old-keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.UserLink(t, cryptDBC, database.UserLink{
			UserID:            user.ID,
			LoginType:         user.LoginType,
			OAuthAccessToken:  "access-token",
			OAuthRefreshToken: "refresh-token",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.OAuthAccessTokenKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.rotateErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Rotate")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		links, getErr := f.rawDB.GetUserLinksByUserID(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Len(t, links, 1)
		require.Equal(t, cipherC.HexDigest(), links[0].OAuthAccessTokenKeyID.String, "row must remain encrypted under cipher C after a failed rotation")
	})
}

// TestRotateExternalAuthLinks covers the external_auth_links table (external
// Git provider OAuth tokens). Simulates rotating keys on a deployment that
// includes users who have since been soft-deleted but whose tokens are
// still sitting in the table, exactly the case that must be swept before
// the old key can be revoked:
//
//	coder server dbcrypt rotate \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --new-key <base64 key B> \
//	  --old-keys <base64 key A>
func TestRotateExternalAuthLinks(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		// Unlike user_links, external_auth_links survive user soft-deletion,
		// so both a live and a deleted user's row are exercised.
		live := dbgen.User(t, f.rawDB, database.User{})
		deletedUser := dbgen.User(t, f.rawDB, database.User{Deleted: true})

		seedLink := func(u database.User) {
			dbgen.ExternalAuthLink(t, f.cryptDBA, database.ExternalAuthLink{
				UserID:            u.ID,
				ProviderID:        "fake",
				OAuthAccessToken:  "access-" + u.ID.String(),
				OAuthRefreshToken: "refresh-" + u.ID.String(),
			})
		}
		seedLink(live)
		seedLink(deletedUser)

		f.rotate(t)

		for _, u := range []database.User{live, deletedUser} {
			links, err := f.rawDB.GetExternalAuthLinksByUserID(f.ctx, u.ID)
			require.NoError(t, err, "user %s", u.ID)
			require.Len(t, links, 1, "user %s", u.ID)
			link := links[0]
			require.Equal(t, f.cipherB.HexDigest(), link.OAuthAccessTokenKeyID.String)
			require.Equal(t, f.cipherB.HexDigest(), link.OAuthRefreshTokenKeyID.String)
			require.Equal(t, "access-"+u.ID.String(), decryptRawString(t, f.cipherB, link.OAuthAccessToken))
			require.Equal(t, "refresh-"+u.ID.String(), decryptRawString(t, f.cipherB, link.OAuthRefreshToken))
		}
	})

	// DecryptErr simulates an operator omitting an old key from --old-keys.
	// An external_auth_links row is encrypted under cipher C, registered in
	// dbcrypt_keys but never passed to the rotation below, so Rotate fails
	// immediately (see TestRotateUserLinks/DecryptErr for why) and leaves
	// the row untouched.
	//
	//	# cipher C's key is missing from --old-keys here on purpose:
	//	coder server dbcrypt rotate \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --new-key <base64 key B> \
	//	  --old-keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.ExternalAuthLink(t, cryptDBC, database.ExternalAuthLink{
			UserID:            user.ID,
			ProviderID:        "fake",
			OAuthAccessToken:  "access-token",
			OAuthRefreshToken: "refresh-token",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.OAuthAccessTokenKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.rotateErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Rotate")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		links, getErr := f.rawDB.GetExternalAuthLinksByUserID(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Len(t, links, 1)
		require.Equal(t, cipherC.HexDigest(), links[0].OAuthAccessTokenKeyID.String, "row must remain encrypted under cipher C after a failed rotation")
	})
}

// TestRotateUserSecrets covers the user_secrets table (arbitrary
// user-defined secret values). Simulates a routine key rotation on a
// deployment with existing encrypted user secrets:
//
//	coder server dbcrypt rotate \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --new-key <base64 key B> \
//	  --old-keys <base64 key A>
func TestRotateUserSecrets(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		// user_secrets cannot exist for a deleted user, so only a live row
		// is exercised here.
		user := dbgen.User(t, f.rawDB, database.User{})
		const wantValue = "super-secret-value"
		dbgen.UserSecret(t, f.cryptDBA, database.UserSecret{
			UserID: user.ID,
			Name:   "my-secret",
			Value:  wantValue,
		})

		f.rotate(t)

		secrets, err := f.rawDB.ListUserSecretsWithValues(f.ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 1)
		require.Equal(t, f.cipherB.HexDigest(), secrets[0].ValueKeyID.String)
		require.Equal(t, wantValue, decryptRawString(t, f.cipherB, secrets[0].Value))
	})

	// DecryptErr simulates an operator omitting an old key from --old-keys.
	// A user_secrets row is encrypted under cipher C, registered in
	// dbcrypt_keys but never passed to the rotation below, so Rotate fails
	// immediately (see TestRotateUserLinks/DecryptErr for why) and leaves
	// the row untouched.
	//
	//	# cipher C's key is missing from --old-keys here on purpose:
	//	coder server dbcrypt rotate \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --new-key <base64 key B> \
	//	  --old-keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.UserSecret(t, cryptDBC, database.UserSecret{
			UserID: user.ID,
			Name:   "my-secret",
			Value:  "super-secret-value",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.ValueKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.rotateErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Rotate")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		secrets, getErr := f.rawDB.ListUserSecretsWithValues(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Len(t, secrets, 1)
		require.Equal(t, cipherC.HexDigest(), secrets[0].ValueKeyID.String, "row must remain encrypted under cipher C after a failed rotation")
	})
}

// TestRotateGitSSHKey covers the gitsshkeys table (per-user Git SSH private
// keys). Simulates rotating keys on a deployment where a soft-deleted
// user's SSH key row is still present, since gitsshkeys rows are preserved
// (not removed) by the user soft-delete trigger so the row can be
// regenerated if the user is restored:
//
//	coder server dbcrypt rotate \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --new-key <base64 key B> \
//	  --old-keys <base64 key A>
func TestRotateGitSSHKey(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		// gitsshkeys are preserved by the user soft-delete trigger, so both
		// a live and a deleted user's row are exercised.
		live := dbgen.User(t, f.rawDB, database.User{})
		deletedUser := dbgen.User(t, f.rawDB, database.User{Deleted: true})

		seedKey := func(u database.User) {
			dbgen.GitSSHKey(t, f.cryptDBA, database.GitSSHKey{
				UserID:     u.ID,
				PrivateKey: "private-" + u.ID.String(),
				PublicKey:  "public-" + u.ID.String(),
			})
		}
		seedKey(live)
		seedKey(deletedUser)

		f.rotate(t)

		for _, u := range []database.User{live, deletedUser} {
			key, err := f.rawDB.GetGitSSHKey(f.ctx, u.ID)
			require.NoError(t, err, "user %s", u.ID)
			require.Equal(t, f.cipherB.HexDigest(), key.PrivateKeyKeyID.String)
			require.Equal(t, "private-"+u.ID.String(), decryptRawString(t, f.cipherB, key.PrivateKey))
			// The public key is never encrypted.
			require.Equal(t, "public-"+u.ID.String(), key.PublicKey)
		}
	})

	// DecryptErr simulates an operator omitting an old key from --old-keys.
	// A gitsshkeys row is encrypted under cipher C, registered in
	// dbcrypt_keys but never passed to the rotation below, so Rotate fails
	// immediately (see TestRotateUserLinks/DecryptErr for why) and leaves
	// the row untouched.
	//
	//	# cipher C's key is missing from --old-keys here on purpose:
	//	coder server dbcrypt rotate \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --new-key <base64 key B> \
	//	  --old-keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.GitSSHKey(t, cryptDBC, database.GitSSHKey{
			UserID:     user.ID,
			PrivateKey: "private-key",
			PublicKey:  "public-key",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.PrivateKeyKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.rotateErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Rotate")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		key, getErr := f.rawDB.GetGitSSHKey(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Equal(t, cipherC.HexDigest(), key.PrivateKeyKeyID.String, "row must remain encrypted under cipher C after a failed rotation")
	})
}

// TestRotateAIProviders covers the ai_providers table (provider-level
// settings, e.g. AWS Bedrock region/model config). Simulates rotating keys
// on a deployment that has decommissioned (soft-deleted) an AI provider but
// kept the row for audit/FK history, exactly the kind of row that's easy to
// forget when a new dbcrypt-managed table is added:
//
//	coder server dbcrypt rotate \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --new-key <base64 key B> \
//	  --old-keys <base64 key A>
func TestRotateAIProviders(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		// ai_providers carries its own soft-delete flag (independent of any
		// user), so both a live and a soft-deleted provider are exercised.
		live := dbgen.AIProvider(t, f.cryptDBA, database.AIProvider{
			Settings: sql.NullString{String: "settings-live", Valid: true},
		})
		deleted := dbgen.AIProvider(t, f.cryptDBA, database.AIProvider{
			Settings: sql.NullString{String: "settings-deleted", Valid: true},
		})
		require.NoError(t, f.rawDB.DeleteAIProviderByID(f.ctx, deleted.ID))

		f.rotate(t)

		providers, err := f.rawDB.GetAIProviders(f.ctx, database.GetAIProvidersParams{
			IncludeDeleted:  true,
			IncludeDisabled: true,
		})
		require.NoError(t, err)
		byID := make(map[uuid.UUID]database.AIProvider, len(providers))
		for _, p := range providers {
			byID[p.ID] = p
		}

		gotLive, ok := byID[live.ID]
		require.True(t, ok)
		require.Equal(t, f.cipherB.HexDigest(), gotLive.SettingsKeyID.String)
		require.Equal(t, "settings-live", decryptRawString(t, f.cipherB, gotLive.Settings.String))

		gotDeleted, ok := byID[deleted.ID]
		require.True(t, ok)
		require.True(t, gotDeleted.Deleted, "provider should remain marked deleted after rotation")
		require.Equal(t, f.cipherB.HexDigest(), gotDeleted.SettingsKeyID.String)
		require.Equal(t, "settings-deleted", decryptRawString(t, f.cipherB, gotDeleted.Settings.String))
	})

	// DecryptErr simulates an operator omitting an old key from --old-keys.
	// An ai_providers row's settings are encrypted under cipher C,
	// registered in dbcrypt_keys but never passed to the rotation below, so
	// Rotate fails immediately (see TestRotateUserLinks/DecryptErr for why)
	// and leaves the row untouched.
	//
	//	# cipher C's key is missing from --old-keys here on purpose:
	//	coder server dbcrypt rotate \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --new-key <base64 key B> \
	//	  --old-keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		seeded := dbgen.AIProvider(t, cryptDBC, database.AIProvider{
			Settings: sql.NullString{String: "settings-value", Valid: true},
		})
		require.Equal(t, cipherC.HexDigest(), seeded.SettingsKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.rotateErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Rotate")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		got, getErr := f.rawDB.GetAIProviderByID(f.ctx, seeded.ID)
		require.NoError(t, getErr)
		require.Equal(t, cipherC.HexDigest(), got.SettingsKeyID.String, "row must remain encrypted under cipher C after a failed rotation")
	})
}

// TestRotateAIProviderKeys covers the ai_provider_keys table (provider API
// keys, e.g. OpenAI/Anthropic credentials). Simulates rotating keys where
// one key belongs to a live provider and another belongs to a provider that
// has since been soft-deleted; the key row has no deleted flag of its own,
// it inherits "deleted" from its parent provider via a JOIN:
//
//	coder server dbcrypt rotate \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --new-key <base64 key B> \
//	  --old-keys <base64 key A>
func TestRotateAIProviderKeys(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		// GetAIProviderKeys(includeDeleted=true), which Rotate calls, joins
		// against ai_providers.deleted, so a key belonging to a soft-deleted
		// provider is exercised alongside a key on a live provider.
		liveProvider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		deletedProvider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		require.NoError(t, f.rawDB.DeleteAIProviderByID(f.ctx, deletedProvider.ID))

		liveKey := dbgen.AIProviderKey(t, f.cryptDBA, database.AIProviderKey{
			ProviderID: liveProvider.ID,
			APIKey:     "api-key-live",
		})
		deletedKey := dbgen.AIProviderKey(t, f.cryptDBA, database.AIProviderKey{
			ProviderID: deletedProvider.ID,
			APIKey:     "api-key-deleted-provider",
		})

		f.rotate(t)

		gotLive, err := f.rawDB.GetAIProviderKeyByID(f.ctx, liveKey.ID)
		require.NoError(t, err)
		require.Equal(t, f.cipherB.HexDigest(), gotLive.ApiKeyKeyID.String)
		require.Equal(t, "api-key-live", decryptRawString(t, f.cipherB, gotLive.APIKey))

		gotDeleted, err := f.rawDB.GetAIProviderKeyByID(f.ctx, deletedKey.ID)
		require.NoError(t, err)
		require.Equal(t, f.cipherB.HexDigest(), gotDeleted.ApiKeyKeyID.String)
		require.Equal(t, "api-key-deleted-provider", decryptRawString(t, f.cipherB, gotDeleted.APIKey))
	})

	// DecryptErr simulates an operator omitting an old key from --old-keys.
	// An ai_provider_keys row is encrypted under cipher C, registered in
	// dbcrypt_keys but never passed to the rotation below, so Rotate fails
	// immediately (see TestRotateUserLinks/DecryptErr for why) and leaves
	// the row untouched.
	//
	//	# cipher C's key is missing from --old-keys here on purpose:
	//	coder server dbcrypt rotate \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --new-key <base64 key B> \
	//	  --old-keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		seeded := dbgen.AIProviderKey(t, cryptDBC, database.AIProviderKey{
			ProviderID: provider.ID,
			APIKey:     "api-key-value",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.ApiKeyKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.rotateErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Rotate")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		got, getErr := f.rawDB.GetAIProviderKeyByID(f.ctx, seeded.ID)
		require.NoError(t, getErr)
		require.Equal(t, cipherC.HexDigest(), got.ApiKeyKeyID.String, "row must remain encrypted under cipher C after a failed rotation")
	})
}

// TestRotateUserAIProviderKeys covers the user_ai_provider_keys table
// (per-user, user-owned AI provider API keys). Simulates rotating keys
// where one row belongs to a live user and another to a soft-deleted user;
// unlike the other tables, GetUserAIProviderKeys has no deleted/live filter
// at all, so this test exists to confirm Rotate doesn't silently need one:
//
//	coder server dbcrypt rotate \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --new-key <base64 key B> \
//	  --old-keys <base64 key A>
func TestRotateUserAIProviderKeys(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		// GetUserAIProviderKeys, which Rotate calls, has no deleted/live
		// filter at all, so a row belonging to a deleted user is exercised
		// alongside a live user's row purely to confirm Rotate doesn't need
		// one.
		liveUser := dbgen.User(t, f.rawDB, database.User{})
		deletedUser := dbgen.User(t, f.rawDB, database.User{Deleted: true})
		provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})

		liveKey := upsertUserAIProviderKey(f.ctx, t, f.cryptDBA, liveUser.ID, provider.ID, "user-key-live")
		deletedKey := upsertUserAIProviderKey(f.ctx, t, f.cryptDBA, deletedUser.ID, provider.ID, "user-key-deleted-user")

		f.rotate(t)

		keys, err := f.rawDB.GetUserAIProviderKeys(f.ctx)
		require.NoError(t, err)
		byID := make(map[uuid.UUID]database.UserAIProviderKey, len(keys))
		for _, k := range keys {
			byID[k.ID] = k
		}

		for _, tc := range []struct {
			id   uuid.UUID
			want string
		}{
			{liveKey.ID, "user-key-live"},
			{deletedKey.ID, "user-key-deleted-user"},
		} {
			got, ok := byID[tc.id]
			require.True(t, ok, "key %s", tc.id)
			require.Equal(t, f.cipherB.HexDigest(), got.ApiKeyKeyID.String)
			require.Equal(t, tc.want, decryptRawString(t, f.cipherB, got.APIKey))
		}
	})

	// DecryptErr simulates an operator omitting an old key from --old-keys.
	// A user_ai_provider_keys row is encrypted under cipher C, registered
	// in dbcrypt_keys but never passed to the rotation below, so Rotate
	// fails immediately (see TestRotateUserLinks/DecryptErr for why) and
	// leaves the row untouched.
	//
	//	# cipher C's key is missing from --old-keys here on purpose:
	//	coder server dbcrypt rotate \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --new-key <base64 key B> \
	//	  --old-keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newRotateFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		seeded := upsertUserAIProviderKey(f.ctx, t, cryptDBC, user.ID, provider.ID, "user-key-value")
		require.Equal(t, cipherC.HexDigest(), seeded.ApiKeyKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.rotateErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Rotate")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		keys, getErr := f.rawDB.GetUserAIProviderKeys(f.ctx)
		require.NoError(t, getErr)
		var got *database.UserAIProviderKey
		for i := range keys {
			if keys[i].ID == seeded.ID {
				got = &keys[i]
			}
		}
		require.NotNil(t, got)
		require.Equal(t, cipherC.HexDigest(), got.ApiKeyKeyID.String, "row must remain encrypted under cipher C after a failed rotation")
	})
}

// decryptFixture provisions an isolated Postgres database plus a single
// cipher ("A") used to exercise a single Decrypt operation. Unlike Rotate,
// Decrypt has no destination cipher, it writes plaintext back and clears
// every key ID column, then revokes every cipher it was given.
type decryptFixture struct {
	ctx      context.Context
	rawDB    database.Store
	sqlDB    *sql.DB
	cipherA  dbcrypt.Cipher
	cryptDBA database.Store
}

func newDecryptFixture(t *testing.T) *decryptFixture {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	ciphersA, err := dbcrypt.NewCiphers([]byte(testutil.MustRandString(t, 32)))
	require.NoError(t, err)

	cryptDBA, err := dbcrypt.New(ctx, rawDB, ciphersA...)
	require.NoError(t, err)

	return &decryptFixture{
		ctx:      ctx,
		rawDB:    rawDB,
		sqlDB:    sqlDB,
		cipherA:  ciphersA[0],
		cryptDBA: cryptDBA,
	}
}

// decryptErr decrypts every dbcrypt-managed row and revokes cipherA,
// returning whatever error Decrypt produces.
func (f *decryptFixture) decryptErr(t *testing.T) error {
	t.Helper()
	return dbcrypt.Decrypt(f.ctx, testutil.Logger(t), f.sqlDB, []dbcrypt.Cipher{f.cipherA})
}

// decrypt is decryptErr for tests that expect the decrypt to succeed.
func (f *decryptFixture) decrypt(t *testing.T) {
	t.Helper()
	err := f.decryptErr(t)
	require.NoError(t, err, "decrypt should succeed and cleanly revoke cipher A")
}

// registerCipher is the decryptFixture counterpart of
// rotateFixture.registerCipher: it builds a dbcrypt-wrapped store with c as
// primary but also loads cipherA, so construction can decrypt cipherA's
// existing dbcrypt_keys canary. Used to seed data under a cipher that
// Decrypt is deliberately never told about.
func (f *decryptFixture) registerCipher(t *testing.T, c dbcrypt.Cipher) database.Store {
	t.Helper()
	wrapped, err := dbcrypt.New(f.ctx, f.rawDB, c, f.cipherA)
	require.NoError(t, err)
	return wrapped
}

// TestDecryptUserLinks covers the user_links table (OAuth login access and
// refresh tokens). Simulates an operator turning off database encryption
// entirely on a deployment that has it enabled:
//
//	coder server dbcrypt decrypt \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --keys <base64 key A>
func TestDecryptUserLinks(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		// user_links cannot exist for a deleted user, so only a live row is
		// exercised here.
		user := dbgen.User(t, f.rawDB, database.User{})
		const wantAccess = "access-token"
		const wantRefresh = "refresh-token"
		seeded := dbgen.UserLink(t, f.cryptDBA, database.UserLink{
			UserID:            user.ID,
			LoginType:         user.LoginType,
			OAuthAccessToken:  wantAccess,
			OAuthRefreshToken: wantRefresh,
		})
		require.Equal(t, f.cipherA.HexDigest(), seeded.OAuthAccessTokenKeyID.String, "sanity check: seed must be encrypted under cipher A")

		f.decrypt(t)

		links, err := f.rawDB.GetUserLinksByUserID(f.ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, links, 1)
		link := links[0]
		require.False(t, link.OAuthAccessTokenKeyID.Valid, "key ID should be cleared after decrypt")
		require.False(t, link.OAuthRefreshTokenKeyID.Valid, "key ID should be cleared after decrypt")
		require.Equal(t, wantAccess, link.OAuthAccessToken, "value should be stored as plaintext after decrypt")
		require.Equal(t, wantRefresh, link.OAuthRefreshToken, "value should be stored as plaintext after decrypt")
	})

	// DecryptErr simulates an operator omitting a key from --keys. A
	// user_links row is encrypted under cipher C, registered in
	// dbcrypt_keys but never passed to the decrypt below, so it fails
	// immediately (see TestRotateUserLinks/DecryptErr for why building any
	// dbcrypt-wrapped store requires every active key to be known) and
	// leaves the row untouched.
	//
	//	# cipher C's key is missing from --keys here on purpose:
	//	coder server dbcrypt decrypt \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.UserLink(t, cryptDBC, database.UserLink{
			UserID:            user.ID,
			LoginType:         user.LoginType,
			OAuthAccessToken:  "access-token",
			OAuthRefreshToken: "refresh-token",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.OAuthAccessTokenKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.decryptErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Decrypt")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		links, getErr := f.rawDB.GetUserLinksByUserID(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Len(t, links, 1)
		require.Equal(t, cipherC.HexDigest(), links[0].OAuthAccessTokenKeyID.String, "row must remain encrypted under cipher C after a failed decrypt")
	})
}

// TestDecryptExternalAuthLinks covers the external_auth_links table
// (external Git provider OAuth tokens). Simulates disabling encryption on a
// deployment that includes users who have since been soft-deleted but whose
// tokens are still sitting in the table:
//
//	coder server dbcrypt decrypt \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --keys <base64 key A>
func TestDecryptExternalAuthLinks(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		// Unlike user_links, external_auth_links survive user soft-deletion,
		// so both a live and a deleted user's row are exercised.
		live := dbgen.User(t, f.rawDB, database.User{})
		deletedUser := dbgen.User(t, f.rawDB, database.User{Deleted: true})

		seedLink := func(u database.User) {
			dbgen.ExternalAuthLink(t, f.cryptDBA, database.ExternalAuthLink{
				UserID:            u.ID,
				ProviderID:        "fake",
				OAuthAccessToken:  "access-" + u.ID.String(),
				OAuthRefreshToken: "refresh-" + u.ID.String(),
			})
		}
		seedLink(live)
		seedLink(deletedUser)

		f.decrypt(t)

		for _, u := range []database.User{live, deletedUser} {
			links, err := f.rawDB.GetExternalAuthLinksByUserID(f.ctx, u.ID)
			require.NoError(t, err, "user %s", u.ID)
			require.Len(t, links, 1, "user %s", u.ID)
			link := links[0]
			require.False(t, link.OAuthAccessTokenKeyID.Valid)
			require.False(t, link.OAuthRefreshTokenKeyID.Valid)
			require.Equal(t, "access-"+u.ID.String(), link.OAuthAccessToken)
			require.Equal(t, "refresh-"+u.ID.String(), link.OAuthRefreshToken)
		}
	})

	// DecryptErr simulates an operator omitting a key from --keys. See
	// TestDecryptUserLinks/DecryptErr for the underlying mechanism.
	//
	//	# cipher C's key is missing from --keys here on purpose:
	//	coder server dbcrypt decrypt \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.ExternalAuthLink(t, cryptDBC, database.ExternalAuthLink{
			UserID:            user.ID,
			ProviderID:        "fake",
			OAuthAccessToken:  "access-token",
			OAuthRefreshToken: "refresh-token",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.OAuthAccessTokenKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.decryptErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Decrypt")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		links, getErr := f.rawDB.GetExternalAuthLinksByUserID(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Len(t, links, 1)
		require.Equal(t, cipherC.HexDigest(), links[0].OAuthAccessTokenKeyID.String, "row must remain encrypted under cipher C after a failed decrypt")
	})
}

// TestDecryptUserSecrets covers the user_secrets table (arbitrary
// user-defined secret values). Simulates disabling encryption on a
// deployment with existing encrypted user secrets:
//
//	coder server dbcrypt decrypt \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --keys <base64 key A>
func TestDecryptUserSecrets(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		// user_secrets cannot exist for a deleted user, so only a live row
		// is exercised here.
		user := dbgen.User(t, f.rawDB, database.User{})
		const wantValue = "super-secret-value"
		dbgen.UserSecret(t, f.cryptDBA, database.UserSecret{
			UserID: user.ID,
			Name:   "my-secret",
			Value:  wantValue,
		})

		f.decrypt(t)

		secrets, err := f.rawDB.ListUserSecretsWithValues(f.ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, secrets, 1)
		require.False(t, secrets[0].ValueKeyID.Valid)
		require.Equal(t, wantValue, secrets[0].Value)
	})

	// DecryptErr simulates an operator omitting a key from --keys. See
	// TestDecryptUserLinks/DecryptErr for the underlying mechanism.
	//
	//	# cipher C's key is missing from --keys here on purpose:
	//	coder server dbcrypt decrypt \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.UserSecret(t, cryptDBC, database.UserSecret{
			UserID: user.ID,
			Name:   "my-secret",
			Value:  "super-secret-value",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.ValueKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.decryptErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Decrypt")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		secrets, getErr := f.rawDB.ListUserSecretsWithValues(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Len(t, secrets, 1)
		require.Equal(t, cipherC.HexDigest(), secrets[0].ValueKeyID.String, "row must remain encrypted under cipher C after a failed decrypt")
	})
}

// TestDecryptGitSSHKey covers the gitsshkeys table (per-user Git SSH
// private keys). Simulates disabling encryption on a deployment where a
// soft-deleted user's SSH key row is still present:
//
//	coder server dbcrypt decrypt \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --keys <base64 key A>
func TestDecryptGitSSHKey(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		// gitsshkeys are preserved by the user soft-delete trigger, so both
		// a live and a deleted user's row are exercised.
		live := dbgen.User(t, f.rawDB, database.User{})
		deletedUser := dbgen.User(t, f.rawDB, database.User{Deleted: true})

		seedKey := func(u database.User) {
			dbgen.GitSSHKey(t, f.cryptDBA, database.GitSSHKey{
				UserID:     u.ID,
				PrivateKey: "private-" + u.ID.String(),
				PublicKey:  "public-" + u.ID.String(),
			})
		}
		seedKey(live)
		seedKey(deletedUser)

		f.decrypt(t)

		for _, u := range []database.User{live, deletedUser} {
			key, err := f.rawDB.GetGitSSHKey(f.ctx, u.ID)
			require.NoError(t, err, "user %s", u.ID)
			require.False(t, key.PrivateKeyKeyID.Valid)
			require.Equal(t, "private-"+u.ID.String(), key.PrivateKey)
			require.Equal(t, "public-"+u.ID.String(), key.PublicKey)
		}
	})

	// DecryptErr simulates an operator omitting a key from --keys. See
	// TestDecryptUserLinks/DecryptErr for the underlying mechanism.
	//
	//	# cipher C's key is missing from --keys here on purpose:
	//	coder server dbcrypt decrypt \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		seeded := dbgen.GitSSHKey(t, cryptDBC, database.GitSSHKey{
			UserID:     user.ID,
			PrivateKey: "private-key",
			PublicKey:  "public-key",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.PrivateKeyKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.decryptErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Decrypt")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		key, getErr := f.rawDB.GetGitSSHKey(f.ctx, user.ID)
		require.NoError(t, getErr)
		require.Equal(t, cipherC.HexDigest(), key.PrivateKeyKeyID.String, "row must remain encrypted under cipher C after a failed decrypt")
	})
}

// TestDecryptAIProviders covers the ai_providers table (provider-level
// settings, e.g. AWS Bedrock region/model config). Simulates disabling
// encryption on a deployment that has decommissioned (soft-deleted) an AI
// provider but kept the row for audit/FK history:
//
//	coder server dbcrypt decrypt \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --keys <base64 key A>
func TestDecryptAIProviders(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		// ai_providers carries its own soft-delete flag (independent of any
		// user), so both a live and a soft-deleted provider are exercised.
		live := dbgen.AIProvider(t, f.cryptDBA, database.AIProvider{
			Settings: sql.NullString{String: "settings-live", Valid: true},
		})
		deleted := dbgen.AIProvider(t, f.cryptDBA, database.AIProvider{
			Settings: sql.NullString{String: "settings-deleted", Valid: true},
		})
		require.NoError(t, f.rawDB.DeleteAIProviderByID(f.ctx, deleted.ID))

		f.decrypt(t)

		providers, err := f.rawDB.GetAIProviders(f.ctx, database.GetAIProvidersParams{
			IncludeDeleted:  true,
			IncludeDisabled: true,
		})
		require.NoError(t, err)
		byID := make(map[uuid.UUID]database.AIProvider, len(providers))
		for _, p := range providers {
			byID[p.ID] = p
		}

		gotLive, ok := byID[live.ID]
		require.True(t, ok)
		require.False(t, gotLive.SettingsKeyID.Valid)
		require.Equal(t, "settings-live", gotLive.Settings.String)

		gotDeleted, ok := byID[deleted.ID]
		require.True(t, ok)
		require.True(t, gotDeleted.Deleted, "provider should remain marked deleted after decrypt")
		require.False(t, gotDeleted.SettingsKeyID.Valid)
		require.Equal(t, "settings-deleted", gotDeleted.Settings.String)
	})

	// DecryptErr simulates an operator omitting a key from --keys. See
	// TestDecryptUserLinks/DecryptErr for the underlying mechanism.
	//
	//	# cipher C's key is missing from --keys here on purpose:
	//	coder server dbcrypt decrypt \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		seeded := dbgen.AIProvider(t, cryptDBC, database.AIProvider{
			Settings: sql.NullString{String: "settings-value", Valid: true},
		})
		require.Equal(t, cipherC.HexDigest(), seeded.SettingsKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.decryptErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Decrypt")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		got, getErr := f.rawDB.GetAIProviderByID(f.ctx, seeded.ID)
		require.NoError(t, getErr)
		require.Equal(t, cipherC.HexDigest(), got.SettingsKeyID.String, "row must remain encrypted under cipher C after a failed decrypt")
	})
}

// TestDecryptAIProviderKeys covers the ai_provider_keys table (provider API
// keys, e.g. OpenAI/Anthropic credentials). Simulates disabling encryption
// where one key belongs to a live provider and another belongs to a
// provider that has since been soft-deleted:
//
//	coder server dbcrypt decrypt \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --keys <base64 key A>
func TestDecryptAIProviderKeys(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		// GetAIProviderKeys(includeDeleted=true), which Decrypt calls,
		// joins against ai_providers.deleted, so a key belonging to a
		// soft-deleted provider is exercised alongside a key on a live
		// provider.
		liveProvider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		deletedProvider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		require.NoError(t, f.rawDB.DeleteAIProviderByID(f.ctx, deletedProvider.ID))

		liveKey := dbgen.AIProviderKey(t, f.cryptDBA, database.AIProviderKey{
			ProviderID: liveProvider.ID,
			APIKey:     "api-key-live",
		})
		deletedKey := dbgen.AIProviderKey(t, f.cryptDBA, database.AIProviderKey{
			ProviderID: deletedProvider.ID,
			APIKey:     "api-key-deleted-provider",
		})

		f.decrypt(t)

		gotLive, err := f.rawDB.GetAIProviderKeyByID(f.ctx, liveKey.ID)
		require.NoError(t, err)
		require.False(t, gotLive.ApiKeyKeyID.Valid)
		require.Equal(t, "api-key-live", gotLive.APIKey)

		gotDeleted, err := f.rawDB.GetAIProviderKeyByID(f.ctx, deletedKey.ID)
		require.NoError(t, err)
		require.False(t, gotDeleted.ApiKeyKeyID.Valid)
		require.Equal(t, "api-key-deleted-provider", gotDeleted.APIKey)
	})

	// DecryptErr simulates an operator omitting a key from --keys. See
	// TestDecryptUserLinks/DecryptErr for the underlying mechanism.
	//
	//	# cipher C's key is missing from --keys here on purpose:
	//	coder server dbcrypt decrypt \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		seeded := dbgen.AIProviderKey(t, cryptDBC, database.AIProviderKey{
			ProviderID: provider.ID,
			APIKey:     "api-key-value",
		})
		require.Equal(t, cipherC.HexDigest(), seeded.ApiKeyKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.decryptErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Decrypt")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		got, getErr := f.rawDB.GetAIProviderKeyByID(f.ctx, seeded.ID)
		require.NoError(t, getErr)
		require.Equal(t, cipherC.HexDigest(), got.ApiKeyKeyID.String, "row must remain encrypted under cipher C after a failed decrypt")
	})
}

// TestDecryptUserAIProviderKeys covers the user_ai_provider_keys table
// (per-user, user-owned AI provider API keys). GetUserAIProviderKeys, which
// Decrypt calls, has no deleted/live filter at all, so a row belonging to a
// deleted user is exercised alongside a live user's row:
//
//	coder server dbcrypt decrypt \
//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
//	  --keys <base64 key A>
func TestDecryptUserAIProviderKeys(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		liveUser := dbgen.User(t, f.rawDB, database.User{})
		deletedUser := dbgen.User(t, f.rawDB, database.User{Deleted: true})
		provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})

		liveKey := upsertUserAIProviderKey(f.ctx, t, f.cryptDBA, liveUser.ID, provider.ID, "user-key-live")
		deletedKey := upsertUserAIProviderKey(f.ctx, t, f.cryptDBA, deletedUser.ID, provider.ID, "user-key-deleted-user")

		f.decrypt(t)

		keys, err := f.rawDB.GetUserAIProviderKeys(f.ctx)
		require.NoError(t, err)
		byID := make(map[uuid.UUID]database.UserAIProviderKey, len(keys))
		for _, k := range keys {
			byID[k.ID] = k
		}

		for _, tc := range []struct {
			id   uuid.UUID
			want string
		}{
			{liveKey.ID, "user-key-live"},
			{deletedKey.ID, "user-key-deleted-user"},
		} {
			got, ok := byID[tc.id]
			require.True(t, ok, "key %s", tc.id)
			require.False(t, got.ApiKeyKeyID.Valid)
			require.Equal(t, tc.want, got.APIKey)
		}
	})

	// DecryptErr simulates an operator omitting a key from --keys. See
	// TestDecryptUserLinks/DecryptErr for the underlying mechanism.
	//
	//	# cipher C's key is missing from --keys here on purpose:
	//	coder server dbcrypt decrypt \
	//	  --postgres-url "$CODER_PG_CONNECTION_URL" \
	//	  --keys <base64 key A>
	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		f := newDecryptFixture(t)

		cipherC := newCipher(t)
		cryptDBC := f.registerCipher(t, cipherC)
		user := dbgen.User(t, f.rawDB, database.User{})
		provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
		seeded := upsertUserAIProviderKey(f.ctx, t, cryptDBC, user.ID, provider.ID, "user-key-value")
		require.Equal(t, cipherC.HexDigest(), seeded.ApiKeyKeyID.String, "sanity check: seed must be encrypted under cipher C")

		err := f.decryptErr(t)
		require.Error(t, err, "expected an error: cipher C is active in dbcrypt_keys but was never passed to Decrypt")
		var derr *dbcrypt.DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		keys, getErr := f.rawDB.GetUserAIProviderKeys(f.ctx)
		require.NoError(t, getErr)
		var got *database.UserAIProviderKey
		for i := range keys {
			if keys[i].ID == seeded.ID {
				got = &keys[i]
			}
		}
		require.NotNil(t, got)
		require.Equal(t, cipherC.HexDigest(), got.ApiKeyKeyID.String, "row must remain encrypted under cipher C after a failed decrypt")
	})
}

// deleteFixture provisions an isolated Postgres database plus a cipher used
// to seed encrypted rows before exercising Delete. Delete itself takes no
// cipher argument at all: it wipes rows via a fixed SQL statement and
// revokes every key it finds active in dbcrypt_keys, so there is no
// "missing cipher" failure mode analogous to Rotate/Decrypt's DecryptErr.
type deleteFixture struct {
	ctx      context.Context
	rawDB    database.Store
	sqlDB    *sql.DB
	cipherA  dbcrypt.Cipher
	cryptDBA database.Store
}

func newDeleteFixture(t *testing.T) *deleteFixture {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	ciphersA, err := dbcrypt.NewCiphers([]byte(testutil.MustRandString(t, 32)))
	require.NoError(t, err)

	cryptDBA, err := dbcrypt.New(ctx, rawDB, ciphersA...)
	require.NoError(t, err)

	return &deleteFixture{
		ctx:      ctx,
		rawDB:    rawDB,
		sqlDB:    sqlDB,
		cipherA:  ciphersA[0],
		cryptDBA: cryptDBA,
	}
}

// delete wipes every dbcrypt-managed row that's currently encrypted and
// revokes every active key, asserting the operation succeeds.
func (f *deleteFixture) delete(t *testing.T) {
	t.Helper()
	err := dbcrypt.Delete(f.ctx, testutil.Logger(t), f.sqlDB)
	require.NoError(t, err, "delete should succeed and revoke every active key")
}

// requireAllKeysRevoked asserts every row in dbcrypt_keys has been revoked,
// which Delete guarantees unconditionally regardless of which tables held
// data under that key.
func requireAllKeysRevoked(ctx context.Context, t *testing.T, rawDB database.Store) {
	t.Helper()
	keys, err := rawDB.GetDBCryptKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, keys, "expected at least one dbcrypt key to exist")
	for _, k := range keys {
		require.False(t, k.ActiveKeyDigest.Valid, "key %d should no longer be active", k.Number)
		require.True(t, k.RevokedKeyDigest.Valid, "key %d should be marked revoked", k.Number)
	}
}

// requireDBCryptKeyActive asserts cipher's dbcrypt_keys row is currently the
// active (non-revoked) key.
func requireDBCryptKeyActive(ctx context.Context, t *testing.T, rawDB database.Store, cipher dbcrypt.Cipher) {
	t.Helper()
	keys, err := rawDB.GetDBCryptKeys(ctx)
	require.NoError(t, err)
	for _, k := range keys {
		if k.ActiveKeyDigest.Valid && k.ActiveKeyDigest.String == cipher.HexDigest() {
			return
		}
	}
	t.Fatalf("no active dbcrypt_keys row found for cipher %s", cipher.HexDigest())
}

// requireDBCryptKeyRevoked asserts cipher's dbcrypt_keys row has been
// revoked, i.e. its digest moved from active_key_digest to
// revoked_key_digest.
func requireDBCryptKeyRevoked(ctx context.Context, t *testing.T, rawDB database.Store, cipher dbcrypt.Cipher) {
	t.Helper()
	keys, err := rawDB.GetDBCryptKeys(ctx)
	require.NoError(t, err)
	for _, k := range keys {
		if k.RevokedKeyDigest.Valid && k.RevokedKeyDigest.String == cipher.HexDigest() {
			require.False(t, k.ActiveKeyDigest.Valid, "revoked cipher must not also be active")
			return
		}
	}
	t.Fatalf("no revoked dbcrypt_keys row found for cipher %s", cipher.HexDigest())
}

// TestDeleteUserLinks covers the user_links table (OAuth login access and
// refresh tokens). Simulates an operator recovering from a lost encryption
// key, the last-resort case where decrypt/rotate are both impossible:
//
//	coder server dbcrypt delete \
//	  --postgres-url "$CODER_PG_CONNECTION_URL"
func TestDeleteUserLinks(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)

	// user_links cannot exist for a deleted user, so only live users are
	// exercised here: one with an encrypted row (must be deleted), one
	// with a never-encrypted row (must survive, per the WHERE ... IS NOT
	// NULL clauses in Delete's SQL).
	encUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.UserLink(t, f.cryptDBA, database.UserLink{
		UserID:            encUser.ID,
		LoginType:         encUser.LoginType,
		OAuthAccessToken:  "access-token",
		OAuthRefreshToken: "refresh-token",
	})

	plainUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.UserLink(t, f.rawDB, database.UserLink{
		UserID:            plainUser.ID,
		LoginType:         plainUser.LoginType,
		OAuthAccessToken:  "plain-access-token",
		OAuthRefreshToken: "plain-refresh-token",
	})

	f.delete(t)

	encLinks, err := f.rawDB.GetUserLinksByUserID(f.ctx, encUser.ID)
	require.NoError(t, err)
	require.Empty(t, encLinks, "encrypted user_links row should have been deleted")

	plainLinks, err := f.rawDB.GetUserLinksByUserID(f.ctx, plainUser.ID)
	require.NoError(t, err)
	require.Len(t, plainLinks, 1, "never-encrypted user_links row should survive")
	require.Equal(t, "plain-access-token", plainLinks[0].OAuthAccessToken)
	require.Equal(t, "plain-refresh-token", plainLinks[0].OAuthRefreshToken)

	requireAllKeysRevoked(f.ctx, t, f.rawDB)
}

// TestDeleteExternalAuthLinks covers the external_auth_links table
// (external Git provider OAuth tokens):
//
//	coder server dbcrypt delete \
//	  --postgres-url "$CODER_PG_CONNECTION_URL"
func TestDeleteExternalAuthLinks(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)

	encUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.ExternalAuthLink(t, f.cryptDBA, database.ExternalAuthLink{
		UserID:            encUser.ID,
		ProviderID:        "fake",
		OAuthAccessToken:  "access-token",
		OAuthRefreshToken: "refresh-token",
	})

	plainUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.ExternalAuthLink(t, f.rawDB, database.ExternalAuthLink{
		UserID:            plainUser.ID,
		ProviderID:        "fake",
		OAuthAccessToken:  "plain-access-token",
		OAuthRefreshToken: "plain-refresh-token",
	})

	f.delete(t)

	encLinks, err := f.rawDB.GetExternalAuthLinksByUserID(f.ctx, encUser.ID)
	require.NoError(t, err)
	require.Empty(t, encLinks, "encrypted external_auth_links row should have been deleted")

	plainLinks, err := f.rawDB.GetExternalAuthLinksByUserID(f.ctx, plainUser.ID)
	require.NoError(t, err)
	require.Len(t, plainLinks, 1, "never-encrypted external_auth_links row should survive")
	require.Equal(t, "plain-access-token", plainLinks[0].OAuthAccessToken)
	require.Equal(t, "plain-refresh-token", plainLinks[0].OAuthRefreshToken)

	requireAllKeysRevoked(f.ctx, t, f.rawDB)
}

// TestDeleteUserSecrets covers the user_secrets table (arbitrary
// user-defined secret values):
//
//	coder server dbcrypt delete \
//	  --postgres-url "$CODER_PG_CONNECTION_URL"
func TestDeleteUserSecrets(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)

	encUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.UserSecret(t, f.cryptDBA, database.UserSecret{
		UserID: encUser.ID,
		Name:   "my-secret",
		Value:  "super-secret-value",
	})

	plainUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.UserSecret(t, f.rawDB, database.UserSecret{
		UserID: plainUser.ID,
		Name:   "my-secret",
		Value:  "plain-secret-value",
	})

	f.delete(t)

	encSecrets, err := f.rawDB.ListUserSecretsWithValues(f.ctx, encUser.ID)
	require.NoError(t, err)
	require.Empty(t, encSecrets, "encrypted user_secrets row should have been deleted")

	plainSecrets, err := f.rawDB.ListUserSecretsWithValues(f.ctx, plainUser.ID)
	require.NoError(t, err)
	require.Len(t, plainSecrets, 1, "never-encrypted user_secrets row should survive")
	require.Equal(t, "plain-secret-value", plainSecrets[0].Value)

	requireAllKeysRevoked(f.ctx, t, f.rawDB)
}

// TestDeleteGitSSHKey covers the gitsshkeys table (per-user Git SSH private
// keys). Unlike the tables above, Delete's SQL clears this row in place
// with an UPDATE rather than deleting it, so the user can regenerate a key
// via the UI afterward:
//
//	coder server dbcrypt delete \
//	  --postgres-url "$CODER_PG_CONNECTION_URL"
func TestDeleteGitSSHKey(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)

	encUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.GitSSHKey(t, f.cryptDBA, database.GitSSHKey{
		UserID:     encUser.ID,
		PrivateKey: "private-key",
		PublicKey:  "public-key",
	})

	plainUser := dbgen.User(t, f.rawDB, database.User{})
	dbgen.GitSSHKey(t, f.rawDB, database.GitSSHKey{
		UserID:     plainUser.ID,
		PrivateKey: "plain-private-key",
		PublicKey:  "plain-public-key",
	})

	f.delete(t)

	encKey, err := f.rawDB.GetGitSSHKey(f.ctx, encUser.ID)
	require.NoError(t, err, "row should still exist, only cleared")
	require.Empty(t, encKey.PrivateKey, "encrypted private_key should be wiped")
	require.False(t, encKey.PrivateKeyKeyID.Valid, "private_key_key_id should be cleared")
	require.Equal(t, "public-key", encKey.PublicKey, "public key is never touched")

	plainKey, err := f.rawDB.GetGitSSHKey(f.ctx, plainUser.ID)
	require.NoError(t, err)
	require.Equal(t, "plain-private-key", plainKey.PrivateKey, "never-encrypted private key should survive untouched")
	require.Equal(t, "plain-public-key", plainKey.PublicKey)

	requireAllKeysRevoked(f.ctx, t, f.rawDB)
}

// TestDeleteAIProviders covers the ai_providers table (provider-level
// settings, e.g. AWS Bedrock region/model config). Like gitsshkeys, this
// row is cleared in place with an UPDATE, not deleted:
//
//	coder server dbcrypt delete \
//	  --postgres-url "$CODER_PG_CONNECTION_URL"
func TestDeleteAIProviders(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)

	encProvider := dbgen.AIProvider(t, f.cryptDBA, database.AIProvider{
		Settings: sql.NullString{String: "settings-value", Valid: true},
	})
	plainProvider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{
		Settings: sql.NullString{String: "plain-settings-value", Valid: true},
	})

	f.delete(t)

	got, err := f.rawDB.GetAIProviderByID(f.ctx, encProvider.ID)
	require.NoError(t, err, "row should still exist, only cleared")
	require.False(t, got.Settings.Valid, "encrypted settings should be wiped")
	require.False(t, got.SettingsKeyID.Valid, "settings_key_id should be cleared")

	gotPlain, err := f.rawDB.GetAIProviderByID(f.ctx, plainProvider.ID)
	require.NoError(t, err)
	require.True(t, gotPlain.Settings.Valid)
	require.Equal(t, "plain-settings-value", gotPlain.Settings.String, "never-encrypted settings should survive untouched")

	requireAllKeysRevoked(f.ctx, t, f.rawDB)
}

// TestDeleteAIProviderKeys covers the ai_provider_keys table (provider API
// keys, e.g. OpenAI/Anthropic credentials):
//
//	coder server dbcrypt delete \
//	  --postgres-url "$CODER_PG_CONNECTION_URL"
func TestDeleteAIProviderKeys(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)

	provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
	encKey := dbgen.AIProviderKey(t, f.cryptDBA, database.AIProviderKey{
		ProviderID: provider.ID,
		APIKey:     "api-key-value",
	})
	plainKey := dbgen.AIProviderKey(t, f.rawDB, database.AIProviderKey{
		ProviderID: provider.ID,
		APIKey:     "plain-api-key-value",
	})

	f.delete(t)

	_, err := f.rawDB.GetAIProviderKeyByID(f.ctx, encKey.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "encrypted ai_provider_keys row should have been deleted")

	gotPlain, err := f.rawDB.GetAIProviderKeyByID(f.ctx, plainKey.ID)
	require.NoError(t, err, "never-encrypted ai_provider_keys row should survive")
	require.Equal(t, "plain-api-key-value", gotPlain.APIKey)

	requireAllKeysRevoked(f.ctx, t, f.rawDB)
}

// TestDeleteUserAIProviderKeys covers the user_ai_provider_keys table
// (per-user, user-owned AI provider API keys):
//
//	coder server dbcrypt delete \
//	  --postgres-url "$CODER_PG_CONNECTION_URL"
func TestDeleteUserAIProviderKeys(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)

	// Upsert is keyed by (user_id, ai_provider_id), so encKey and plainKey
	// must belong to different users to land as two independent rows.
	encUser := dbgen.User(t, f.rawDB, database.User{})
	plainUser := dbgen.User(t, f.rawDB, database.User{})
	provider := dbgen.AIProvider(t, f.rawDB, database.AIProvider{})
	encKey := upsertUserAIProviderKey(f.ctx, t, f.cryptDBA, encUser.ID, provider.ID, "user-key-value")
	plainKey := upsertUserAIProviderKey(f.ctx, t, f.rawDB, plainUser.ID, provider.ID, "plain-user-key-value")

	f.delete(t)

	keys, err := f.rawDB.GetUserAIProviderKeys(f.ctx)
	require.NoError(t, err)
	byID := make(map[uuid.UUID]database.UserAIProviderKey, len(keys))
	for _, k := range keys {
		byID[k.ID] = k
	}

	_, stillExists := byID[encKey.ID]
	require.False(t, stillExists, "encrypted user_ai_provider_keys row should have been deleted")

	gotPlain, ok := byID[plainKey.ID]
	require.True(t, ok, "never-encrypted user_ai_provider_keys row should survive")
	require.Equal(t, "plain-user-key-value", gotPlain.APIKey)

	requireAllKeysRevoked(f.ctx, t, f.rawDB)
}

// TestFullLifecycleAllHandledTables seeds one row in every table
// Rotate/Decrypt/Delete actually loop over, then drives the operator
// lifecycle end-to-end in a single database: seed data encrypted under
// cipher A (Encrypt), rotate to cipher B, decrypt back to plaintext, then
// delete a freshly re-encrypted row. One database with every handled table
// populated together, closer to how an operator actually runs these
// commands back to back, instead of one table at a time.
//
// This intentionally excludes crypto_keys, mcp_server_configs, and
// mcp_server_user_tokens. Rotate/Decrypt don't clear those tables at all
// (see issue #25381), so seeding them here would fail this test for a
// reason unrelated to what it's meant to cover: whether the 7 tables
// cliutil.go already handles stay consistent with each other across a full
// rotate-decrypt-delete sequence.
func TestFullLifecycleAllHandledTables(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	log := testutil.Logger(t)

	ciphersA, err := dbcrypt.NewCiphers([]byte(testutil.MustRandString(t, 32)))
	require.NoError(t, err)
	ciphersB, err := dbcrypt.NewCiphers([]byte(testutil.MustRandString(t, 32)))
	require.NoError(t, err)
	cipherA, cipherB := ciphersA[0], ciphersB[0]

	cryptDBA, err := dbcrypt.New(ctx, rawDB, cipherA)
	require.NoError(t, err)

	// --- Encrypt: seed one row per handled table under cipher A ---
	liveUser := dbgen.User(t, rawDB, database.User{})
	provider := dbgen.AIProvider(t, cryptDBA, database.AIProvider{
		Settings: sql.NullString{String: "settings-value", Valid: true},
	})

	seededLink := dbgen.UserLink(t, cryptDBA, database.UserLink{
		UserID:            liveUser.ID,
		LoginType:         liveUser.LoginType,
		OAuthAccessToken:  "access-token",
		OAuthRefreshToken: "refresh-token",
	})
	seededExtLink := dbgen.ExternalAuthLink(t, cryptDBA, database.ExternalAuthLink{
		UserID:            liveUser.ID,
		ProviderID:        "fake",
		OAuthAccessToken:  "ext-access-token",
		OAuthRefreshToken: "ext-refresh-token",
	})
	seededSecret := dbgen.UserSecret(t, cryptDBA, database.UserSecret{
		UserID:   liveUser.ID,
		Name:     "my-secret",
		Value:    "super-secret-value",
		EnvName:  "MY_SECRET_ENV",
		FilePath: "~/my-secret-path",
	})
	seededSSHKey := dbgen.GitSSHKey(t, cryptDBA, database.GitSSHKey{
		UserID:     liveUser.ID,
		PrivateKey: "private-key",
		PublicKey:  "public-key",
	})
	seededProviderKey := dbgen.AIProviderKey(t, cryptDBA, database.AIProviderKey{
		ProviderID: provider.ID,
		APIKey:     "provider-api-key",
	})
	seededUserProviderKey := upsertUserAIProviderKey(ctx, t, cryptDBA, liveUser.ID, provider.ID, "user-api-key")

	require.Equal(t, cipherA.HexDigest(), provider.SettingsKeyID.String, "sanity check: ai_providers seeded under cipher A")
	require.Equal(t, cipherA.HexDigest(), seededLink.OAuthAccessTokenKeyID.String, "sanity check: user_links seeded under cipher A")
	require.Equal(t, cipherA.HexDigest(), seededExtLink.OAuthAccessTokenKeyID.String, "sanity check: external_auth_links seeded under cipher A")
	require.Equal(t, cipherA.HexDigest(), seededSecret.ValueKeyID.String, "sanity check: user_secrets seeded under cipher A")
	require.Equal(t, cipherA.HexDigest(), seededSSHKey.PrivateKeyKeyID.String, "sanity check: gitsshkeys seeded under cipher A")
	require.Equal(t, cipherA.HexDigest(), seededProviderKey.ApiKeyKeyID.String, "sanity check: ai_provider_keys seeded under cipher A")
	require.Equal(t, cipherA.HexDigest(), seededUserProviderKey.ApiKeyKeyID.String, "sanity check: user_ai_provider_keys seeded under cipher A")

	// --- Rotate: cipher A -> cipher B ---
	err = dbcrypt.Rotate(ctx, log, sqlDB, []dbcrypt.Cipher{cipherB, cipherA})
	require.NoError(t, err, "rotate should succeed and revoke cipher A even with every handled table populated at once")
	requireDBCryptKeyRevoked(ctx, t, rawDB, cipherA)
	requireDBCryptKeyActive(ctx, t, rawDB, cipherB)

	links, err := rawDB.GetUserLinksByUserID(ctx, liveUser.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, cipherB.HexDigest(), links[0].OAuthAccessTokenKeyID.String)
	require.Equal(t, "access-token", decryptRawString(t, cipherB, links[0].OAuthAccessToken))

	extLinks, err := rawDB.GetExternalAuthLinksByUserID(ctx, liveUser.ID)
	require.NoError(t, err)
	require.Len(t, extLinks, 1)
	require.Equal(t, cipherB.HexDigest(), extLinks[0].OAuthAccessTokenKeyID.String)

	secrets, err := rawDB.ListUserSecretsWithValues(ctx, liveUser.ID)
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	require.Equal(t, cipherB.HexDigest(), secrets[0].ValueKeyID.String)
	require.Equal(t, "super-secret-value", decryptRawString(t, cipherB, secrets[0].Value))

	sshKey, err := rawDB.GetGitSSHKey(ctx, liveUser.ID)
	require.NoError(t, err)
	require.Equal(t, cipherB.HexDigest(), sshKey.PrivateKeyKeyID.String)

	gotProvider, err := rawDB.GetAIProviderByID(ctx, provider.ID)
	require.NoError(t, err)
	require.Equal(t, cipherB.HexDigest(), gotProvider.SettingsKeyID.String)

	gotProviderKey, err := rawDB.GetAIProviderKeyByID(ctx, seededProviderKey.ID)
	require.NoError(t, err)
	require.Equal(t, cipherB.HexDigest(), gotProviderKey.ApiKeyKeyID.String)

	userProviderKeys, err := rawDB.GetUserAIProviderKeys(ctx)
	require.NoError(t, err)
	var foundUserProviderKey bool
	for _, k := range userProviderKeys {
		if k.ID == seededUserProviderKey.ID {
			require.Equal(t, cipherB.HexDigest(), k.ApiKeyKeyID.String)
			foundUserProviderKey = true
		}
	}
	require.True(t, foundUserProviderKey, "seeded user_ai_provider_keys row must still exist after rotate")

	// --- Decrypt: cipher B -> plaintext ---
	err = dbcrypt.Decrypt(ctx, log, sqlDB, []dbcrypt.Cipher{cipherB})
	require.NoError(t, err, "decrypt should succeed and revoke cipher B even with every handled table populated at once")
	requireDBCryptKeyRevoked(ctx, t, rawDB, cipherB)

	links, err = rawDB.GetUserLinksByUserID(ctx, liveUser.ID)
	require.NoError(t, err)
	require.False(t, links[0].OAuthAccessTokenKeyID.Valid)
	require.Equal(t, "access-token", links[0].OAuthAccessToken)

	secrets, err = rawDB.ListUserSecretsWithValues(ctx, liveUser.ID)
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	require.False(t, secrets[0].ValueKeyID.Valid)
	require.Equal(t, "super-secret-value", secrets[0].Value)

	// --- Delete: seed one more row under a fresh cipher, then wipe it ---
	cipherC := newCipher(t)
	cryptDBC, err := dbcrypt.New(ctx, rawDB, cipherC)
	require.NoError(t, err)
	seededPostDecryptSecret := dbgen.UserSecret(t, cryptDBC, database.UserSecret{
		UserID:   liveUser.ID,
		Name:     "post-decrypt-secret",
		Value:    "another-secret-value",
		EnvName:  "POST_DECRYPT_ENV",
		FilePath: "~/post-decrypt-path",
	})
	require.Equal(t, cipherC.HexDigest(), seededPostDecryptSecret.ValueKeyID.String, "sanity check: post-decrypt secret seeded under cipher C")

	err = dbcrypt.Delete(ctx, log, sqlDB)
	require.NoError(t, err, "delete should succeed and revoke every remaining active key")

	secrets, err = rawDB.ListUserSecretsWithValues(ctx, liveUser.ID)
	require.NoError(t, err)
	require.Len(t, secrets, 1, "only the never-re-encrypted secret should remain")
	require.Equal(t, "my-secret", secrets[0].Name)
	require.Equal(t, "super-secret-value", secrets[0].Value)

	requireAllKeysRevoked(ctx, t, rawDB)
}
