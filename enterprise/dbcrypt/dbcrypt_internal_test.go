package dbcrypt

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"io"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

func TestUserLinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("InsertUserLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		user := dbgen.User(t, crypt, database.User{})
		link := dbgen.UserLink(t, crypt, database.UserLink{
			UserID:            user.ID,
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		require.Equal(t, "access", link.OAuthAccessToken)
		require.Equal(t, "refresh", link.OAuthRefreshToken)
		require.Equal(t, ciphers[0].HexDigest(), link.OAuthAccessTokenKeyID.String)
		require.Equal(t, ciphers[0].HexDigest(), link.OAuthRefreshTokenKeyID.String)

		rawLink, err := db.GetUserLinkByLinkedID(ctx, link.LinkedID)
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], rawLink.OAuthAccessToken, "access")
		requireEncryptedEquals(t, ciphers[0], rawLink.OAuthRefreshToken, "refresh")
	})

	t.Run("UpdateUserLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		user := dbgen.User(t, crypt, database.User{})
		link := dbgen.UserLink(t, crypt, database.UserLink{
			UserID: user.ID,
		})

		updated, err := crypt.UpdateUserLink(ctx, database.UpdateUserLinkParams{
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
			UserID:            link.UserID,
			LoginType:         link.LoginType,
		})
		require.NoError(t, err)
		require.Equal(t, "access", updated.OAuthAccessToken)
		require.Equal(t, "refresh", updated.OAuthRefreshToken)
		require.Equal(t, ciphers[0].HexDigest(), link.OAuthAccessTokenKeyID.String)
		require.Equal(t, ciphers[0].HexDigest(), link.OAuthRefreshTokenKeyID.String)

		rawLink, err := db.GetUserLinkByLinkedID(ctx, link.LinkedID)
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], rawLink.OAuthAccessToken, "access")
		requireEncryptedEquals(t, ciphers[0], rawLink.OAuthRefreshToken, "refresh")
	})

	t.Run("GetUserLinkByLinkedID", func(t *testing.T) {
		t.Parallel()
		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, crypt, database.User{})
			link := dbgen.UserLink(t, crypt, database.UserLink{
				UserID:            user.ID,
				OAuthAccessToken:  "access",
				OAuthRefreshToken: "refresh",
			})

			link, err := crypt.GetUserLinkByLinkedID(ctx, link.LinkedID)
			require.NoError(t, err)
			require.Equal(t, "access", link.OAuthAccessToken)
			require.Equal(t, "refresh", link.OAuthRefreshToken)
			require.Equal(t, ciphers[0].HexDigest(), link.OAuthAccessTokenKeyID.String)
			require.Equal(t, ciphers[0].HexDigest(), link.OAuthRefreshTokenKeyID.String)

			rawLink, err := db.GetUserLinkByLinkedID(ctx, link.LinkedID)
			require.NoError(t, err)
			requireEncryptedEquals(t, ciphers[0], rawLink.OAuthAccessToken, "access")
			requireEncryptedEquals(t, ciphers[0], rawLink.OAuthRefreshToken, "refresh")
		})

		t.Run("DecryptErr", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, db, database.User{})
			link := dbgen.UserLink(t, db, database.UserLink{
				UserID:                 user.ID,
				OAuthAccessToken:       fakeBase64RandomData(t, 32),
				OAuthRefreshToken:      fakeBase64RandomData(t, 32),
				OAuthAccessTokenKeyID:  sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
				OAuthRefreshTokenKeyID: sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
			})

			_, err := crypt.GetUserLinkByLinkedID(ctx, link.LinkedID)
			require.Error(t, err, "expected an error")
			var derr *DecryptFailedError
			require.ErrorAs(t, err, &derr, "expected a decrypt error")
		})
	})

	t.Run("GetUserLinksByUserID", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, crypt, database.User{})
			link := dbgen.UserLink(t, crypt, database.UserLink{
				UserID:            user.ID,
				OAuthAccessToken:  "access",
				OAuthRefreshToken: "refresh",
			})
			links, err := crypt.GetUserLinksByUserID(ctx, link.UserID)
			require.NoError(t, err)
			require.Len(t, links, 1)
			require.Equal(t, "access", links[0].OAuthAccessToken)
			require.Equal(t, "refresh", links[0].OAuthRefreshToken)
			require.Equal(t, ciphers[0].HexDigest(), links[0].OAuthAccessTokenKeyID.String)
			require.Equal(t, ciphers[0].HexDigest(), links[0].OAuthRefreshTokenKeyID.String)

			rawLinks, err := db.GetUserLinksByUserID(ctx, link.UserID)
			require.NoError(t, err)
			require.Len(t, rawLinks, 1)
			requireEncryptedEquals(t, ciphers[0], rawLinks[0].OAuthAccessToken, "access")
			requireEncryptedEquals(t, ciphers[0], rawLinks[0].OAuthRefreshToken, "refresh")
		})

		t.Run("Empty", func(t *testing.T) {
			t.Parallel()
			_, crypt, _ := setup(t)
			user := dbgen.User(t, crypt, database.User{})
			links, err := crypt.GetUserLinksByUserID(ctx, user.ID)
			require.NoError(t, err)
			require.Empty(t, links)
		})

		t.Run("DecryptErr", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, db, database.User{})
			_ = dbgen.UserLink(t, db, database.UserLink{
				UserID:                 user.ID,
				OAuthAccessToken:       fakeBase64RandomData(t, 32),
				OAuthRefreshToken:      fakeBase64RandomData(t, 32),
				OAuthAccessTokenKeyID:  sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
				OAuthRefreshTokenKeyID: sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
			})
			_, err := crypt.GetUserLinksByUserID(ctx, user.ID)
			require.Error(t, err, "expected an error")
			var derr *DecryptFailedError
			require.ErrorAs(t, err, &derr, "expected a decrypt error")
		})
	})

	t.Run("GetUserLinkByUserIDLoginType", func(t *testing.T) {
		t.Parallel()
		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, crypt, database.User{})
			link := dbgen.UserLink(t, crypt, database.UserLink{
				UserID:            user.ID,
				OAuthAccessToken:  "access",
				OAuthRefreshToken: "refresh",
			})

			link, err := crypt.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    link.UserID,
				LoginType: link.LoginType,
			})
			require.NoError(t, err)
			require.Equal(t, "access", link.OAuthAccessToken)
			require.Equal(t, "refresh", link.OAuthRefreshToken)
			require.Equal(t, ciphers[0].HexDigest(), link.OAuthAccessTokenKeyID.String)
			require.Equal(t, ciphers[0].HexDigest(), link.OAuthRefreshTokenKeyID.String)

			rawLink, err := db.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    link.UserID,
				LoginType: link.LoginType,
			})
			require.NoError(t, err)
			requireEncryptedEquals(t, ciphers[0], rawLink.OAuthAccessToken, "access")
			requireEncryptedEquals(t, ciphers[0], rawLink.OAuthRefreshToken, "refresh")
		})

		t.Run("DecryptErr", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, db, database.User{})
			link := dbgen.UserLink(t, db, database.UserLink{
				UserID:                 user.ID,
				OAuthAccessToken:       fakeBase64RandomData(t, 32),
				OAuthRefreshToken:      fakeBase64RandomData(t, 32),
				OAuthAccessTokenKeyID:  sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
				OAuthRefreshTokenKeyID: sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
			})

			_, err := crypt.GetUserLinkByUserIDLoginType(ctx, database.GetUserLinkByUserIDLoginTypeParams{
				UserID:    link.UserID,
				LoginType: link.LoginType,
			})
			require.Error(t, err, "expected an error")
			var derr *DecryptFailedError
			require.ErrorAs(t, err, &derr, "expected a decrypt error")
		})
	})
}

func TestExternalAuthLinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("InsertExternalAuthLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		link := dbgen.ExternalAuthLink(t, crypt, database.ExternalAuthLink{
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		require.Equal(t, "access", link.OAuthAccessToken)
		require.Equal(t, "refresh", link.OAuthRefreshToken)

		link, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, ciphers[0], link.OAuthRefreshToken, "refresh")
	})

	t.Run("UpdateExternalAuthLink", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		link := dbgen.ExternalAuthLink(t, crypt, database.ExternalAuthLink{})
		updated, err := crypt.UpdateExternalAuthLink(ctx, database.UpdateExternalAuthLinkParams{
			ProviderID:        link.ProviderID,
			UserID:            link.UserID,
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
		})
		require.NoError(t, err)
		require.Equal(t, "access", updated.OAuthAccessToken)
		require.Equal(t, "refresh", updated.OAuthRefreshToken)

		link, err = db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], link.OAuthAccessToken, "access")
		requireEncryptedEquals(t, ciphers[0], link.OAuthRefreshToken, "refresh")
	})

	t.Run("GetExternalAuthLink", func(t *testing.T) {
		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			link := dbgen.ExternalAuthLink(t, crypt, database.ExternalAuthLink{
				OAuthAccessToken:  "access",
				OAuthRefreshToken: "refresh",
			})
			link, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
				UserID:     link.UserID,
				ProviderID: link.ProviderID,
			})
			require.NoError(t, err)
			requireEncryptedEquals(t, ciphers[0], link.OAuthAccessToken, "access")
			requireEncryptedEquals(t, ciphers[0], link.OAuthRefreshToken, "refresh")
		})
		t.Run("DecryptErr", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			link := dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
				OAuthAccessToken:       fakeBase64RandomData(t, 32),
				OAuthRefreshToken:      fakeBase64RandomData(t, 32),
				OAuthAccessTokenKeyID:  sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
				OAuthRefreshTokenKeyID: sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
			})

			_, err := crypt.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
				UserID:     link.UserID,
				ProviderID: link.ProviderID,
			})
			require.Error(t, err, "expected an error")
			var derr *DecryptFailedError
			require.ErrorAs(t, err, &derr, "expected a decrypt error")
		})
	})

	t.Run("GetExternalAuthLinksByUserID", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, crypt, database.User{})
			link := dbgen.ExternalAuthLink(t, crypt, database.ExternalAuthLink{
				UserID:            user.ID,
				OAuthAccessToken:  "access",
				OAuthRefreshToken: "refresh",
			})
			links, err := crypt.GetExternalAuthLinksByUserID(ctx, link.UserID)
			require.NoError(t, err)
			require.Len(t, links, 1)
			require.Equal(t, "access", links[0].OAuthAccessToken)
			require.Equal(t, "refresh", links[0].OAuthRefreshToken)
			require.Equal(t, ciphers[0].HexDigest(), links[0].OAuthAccessTokenKeyID.String)
			require.Equal(t, ciphers[0].HexDigest(), links[0].OAuthRefreshTokenKeyID.String)

			rawLinks, err := db.GetExternalAuthLinksByUserID(ctx, link.UserID)
			require.NoError(t, err)
			require.Len(t, rawLinks, 1)
			requireEncryptedEquals(t, ciphers[0], rawLinks[0].OAuthAccessToken, "access")
			requireEncryptedEquals(t, ciphers[0], rawLinks[0].OAuthRefreshToken, "refresh")
		})

		t.Run("DecryptErr", func(t *testing.T) {
			db, crypt, ciphers := setup(t)
			user := dbgen.User(t, db, database.User{})
			link := dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{
				UserID:                 user.ID,
				OAuthAccessToken:       fakeBase64RandomData(t, 32),
				OAuthRefreshToken:      fakeBase64RandomData(t, 32),
				OAuthAccessTokenKeyID:  sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
				OAuthRefreshTokenKeyID: sql.NullString{String: ciphers[0].HexDigest(), Valid: true},
			})
			_, err := crypt.GetExternalAuthLinksByUserID(ctx, link.UserID)
			require.Error(t, err, "expected an error")
			var derr *DecryptFailedError
			require.ErrorAs(t, err, &derr, "expected a decrypt error")
		})
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		// Given: a cipher is loaded
		cipher := initCipher(t)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		rawDB, _ := dbtestutil.NewDB(t)

		// Before: no keys should be present
		keys, err := rawDB.GetDBCryptKeys(ctx)
		require.NoError(t, err, "no error should be returned")
		require.Empty(t, keys, "no keys should be present")

		// When: we init the crypt db
		_, err = New(ctx, rawDB, cipher)
		require.NoError(t, err)

		// Then: a new key is inserted
		keys, err = rawDB.GetDBCryptKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1, "one key should be present")
		require.Equal(t, cipher.HexDigest(), keys[0].ActiveKeyDigest.String, "key digest mismatch")
		require.Empty(t, keys[0].RevokedKeyDigest.String, "key should not be revoked")
		requireEncryptedEquals(t, cipher, keys[0].Test, "coder")
	})

	t.Run("MissingKey", func(t *testing.T) {
		t.Parallel()

		// Given: there exist two valid encryption keys
		cipher1 := initCipher(t)
		cipher2 := initCipher(t)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		rawDB, _ := dbtestutil.NewDB(t)

		// Given: key 1 is already present in the database
		err := rawDB.InsertDBCryptKey(ctx, database.InsertDBCryptKeyParams{
			Number:          1,
			ActiveKeyDigest: cipher1.HexDigest(),
			Test:            fakeBase64RandomData(t, 32),
		})
		require.NoError(t, err, "no error should be returned")
		keys, err := rawDB.GetDBCryptKeys(ctx)
		require.NoError(t, err, "no error should be returned")
		require.Len(t, keys, 1, "one key should be present")

		// When: we init the crypt db with no keys
		_, err = New(ctx, rawDB)
		// Then: we error because we don't know how to decrypt the existing key
		require.Error(t, err, "expected an error")
		var derr *DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		// When: we init the crypt db with key 2
		_, err = New(ctx, rawDB, cipher2)

		// Then: we error because the key is not revoked and we don't know how to decrypt it
		require.Error(t, err, "expected an error")
		require.ErrorAs(t, err, &derr, "expected a decrypt error")

		// When: the existing key is marked as having been revoked
		err = rawDB.RevokeDBCryptKey(ctx, cipher1.HexDigest())
		require.NoError(t, err, "no error should be returned")

		// And: we init the crypt db with key 2
		_, err = New(ctx, rawDB, cipher2)

		// Then: we succeed
		require.NoError(t, err)

		// And: key 2 should now be the active key
		keys, err = rawDB.GetDBCryptKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2, "two keys should be present")
		require.EqualValues(t, keys[0].Number, 1, "key number mismatch")
		require.Empty(t, keys[0].ActiveKeyDigest.String, "key should not be active")
		require.Equal(t, cipher1.HexDigest(), keys[0].RevokedKeyDigest.String, "key should be revoked")

		require.EqualValues(t, keys[1].Number, 2, "key number mismatch")
		require.Equal(t, cipher2.HexDigest(), keys[1].ActiveKeyDigest.String, "key digest mismatch")
		require.Empty(t, keys[1].RevokedKeyDigest.String, "key should not be revoked")
		requireEncryptedEquals(t, cipher2, keys[1].Test, "coder")
	})

	t.Run("NoKeys", func(t *testing.T) {
		t.Parallel()
		// Given: no cipher is loaded
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		rawDB, _ := dbtestutil.NewDB(t)

		keys, err := rawDB.GetDBCryptKeys(ctx)
		require.NoError(t, err, "no error should be returned")
		require.Empty(t, keys, "no keys should be present")

		// When: we init the crypt db with no ciphers
		_, err = New(ctx, rawDB)

		// Then: it should succeed.
		require.NoError(t, err, "dbcrypt.New should work with no keys against an unencrypted database")

		// Assert invariant: no keys are inserted
		keys, err = rawDB.GetDBCryptKeys(ctx)
		require.NoError(t, err, "no error should be returned")
		require.Empty(t, keys, "no keys should be present")

		// Insert a key
		require.NoError(t, rawDB.InsertDBCryptKey(ctx, database.InsertDBCryptKeyParams{
			Number:          1,
			ActiveKeyDigest: "whatever",
			Test:            fakeBase64RandomData(t, 32),
		}))

		// This should fail as we do not know how to decrypt the key:
		_, err = New(ctx, rawDB)
		require.Error(t, err)
		// Until we revoke the key:
		require.NoError(t, rawDB.RevokeDBCryptKey(ctx, "whatever"))
		_, err = New(ctx, rawDB)
		require.NoError(t, err, "the above should still hold if the key is revoked")
	})

	t.Run("PrimaryRevoked", func(t *testing.T) {
		t.Parallel()
		// Given: a cipher is loaded
		cipher := initCipher(t)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		rawDB, _ := dbtestutil.NewDB(t)

		// And: the cipher is revoked before we init the crypt db
		err := rawDB.InsertDBCryptKey(ctx, database.InsertDBCryptKeyParams{
			Number:          1,
			ActiveKeyDigest: cipher.HexDigest(),
			Test:            fakeBase64RandomData(t, 32),
		})
		require.NoError(t, err, "no error should be returned")
		err = rawDB.RevokeDBCryptKey(ctx, cipher.HexDigest())
		require.NoError(t, err, "no error should be returned")

		// Then: when we init the crypt db, we error because the key is revoked
		_, err = New(ctx, rawDB, cipher)
		require.Error(t, err)
		require.ErrorContains(t, err, "has been revoked")
	})

	t.Run("Retry", func(t *testing.T) {
		t.Parallel()
		// Given: a cipher is loaded
		cipher := initCipher(t)
		ctx, cancel := context.WithCancel(context.Background())
		testVal, err := cipher.Encrypt([]byte("coder"))
		key := database.DBCryptKey{
			Number:          1,
			ActiveKeyDigest: sql.NullString{String: cipher.HexDigest(), Valid: true},
			Test:            b64encode(testVal),
		}
		require.NoError(t, err)
		t.Cleanup(cancel)

		// And: a database that returns an error once when we try to serialize a key
		ctrl := gomock.NewController(t)
		mockDB := dbmock.NewMockStore(ctrl)

		gomock.InOrder(
			// First try: we get a serialization error.
			expectInTx(mockDB),
			mockDB.EXPECT().GetDBCryptKeys(gomock.Any()).Times(1).Return([]database.DBCryptKey{}, nil),
			mockDB.EXPECT().InsertDBCryptKey(gomock.Any(), gomock.Any()).Times(1).Return(&pq.Error{Code: "40001"}),
			// Second try: we get the key we wanted to insert initially.
			expectInTx(mockDB),
			mockDB.EXPECT().GetDBCryptKeys(gomock.Any()).Times(1).Return([]database.DBCryptKey{key}, nil),
		)

		_, err = New(ctx, mockDB, cipher)
		require.NoError(t, err)
	})
}

func TestEncryptDecryptField(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		_, cryptDB, ciphers := setup(t)
		field := "coder"
		digest := sql.NullString{}
		require.NoError(t, cryptDB.encryptField(&field, &digest))
		require.Equal(t, ciphers[0].HexDigest(), digest.String)
		requireEncryptedEquals(t, ciphers[0], field, "coder")
		require.NoError(t, cryptDB.decryptField(&field, digest))
		require.Equal(t, "coder", field)
	})

	t.Run("NoKeys", func(t *testing.T) {
		t.Parallel()
		// With no keys, encryption and decryption are both no-ops.
		_, cryptDB := setupNoCiphers(t)
		field := "coder"
		digest := sql.NullString{}
		require.NoError(t, cryptDB.encryptField(&field, &digest))
		require.Empty(t, digest.String)
		require.Equal(t, "coder", field)
		require.NoError(t, cryptDB.decryptField(&field, digest))
		require.Equal(t, "coder", field)
	})

	t.Run("MissingKey", func(t *testing.T) {
		t.Parallel()
		_, cryptDB, ciphers := setup(t)
		field := "coder"
		digest := sql.NullString{}
		err := cryptDB.encryptField(&field, &digest)
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], field, "coder")
		require.Equal(t, ciphers[0].HexDigest(), digest.String)
		require.True(t, digest.Valid)

		digest = sql.NullString{String: "missing", Valid: true}
		var derr *DecryptFailedError
		err = cryptDB.decryptField(&field, digest)
		require.Error(t, err)
		require.ErrorAs(t, err, &derr)
	})

	t.Run("CantEncryptOrDecryptNil", func(t *testing.T) {
		t.Parallel()
		_, cryptDB, _ := setup(t)
		require.ErrorContains(t, cryptDB.encryptField(nil, nil), "developer error")
		require.ErrorContains(t, cryptDB.decryptField(nil, sql.NullString{}), "developer error")
	})

	t.Run("EncryptEmptyString", func(t *testing.T) {
		t.Parallel()
		_, cryptDB, ciphers := setup(t)
		field := ""
		digest := sql.NullString{}
		require.NoError(t, cryptDB.encryptField(&field, &digest))
		requireEncryptedEquals(t, ciphers[0], field, "")
		require.Equal(t, ciphers[0].HexDigest(), digest.String)
		require.NoError(t, cryptDB.decryptField(&field, digest))
		require.Empty(t, field)
	})

	t.Run("DecryptEmptyString", func(t *testing.T) {
		t.Parallel()
		_, cryptDB, ciphers := setup(t)
		field := ""
		digest := sql.NullString{String: ciphers[0].HexDigest(), Valid: true}
		err := cryptDB.decryptField(&field, digest)
		// Currently this has to fail because the ciphertext must at least
		// have a nonce. This may need to be changed depending on future
		// ciphers.
		require.ErrorContains(t, err, "ciphertext too short")
	})

	t.Run("InvalidBase64", func(t *testing.T) {
		t.Parallel()
		_, cryptDB, ciphers := setup(t)
		field := "not valid base64"
		digest := sql.NullString{String: ciphers[0].HexDigest(), Valid: true}
		err := cryptDB.decryptField(&field, digest)
		require.ErrorContains(t, err, "illegal base64 data")
	})
}

func expectInTx(mdb *dbmock.MockStore) *gomock.Call {
	return mdb.EXPECT().InTx(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(f func(store database.Store) error, _ *sql.TxOptions) error {
			return f(mdb)
		},
	)
}

func requireEncryptedEquals(t *testing.T, c Cipher, value, expected string) {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	require.NoError(t, err, "invalid base64")
	got, err := c.Decrypt(data)
	require.NoError(t, err, "failed to decrypt data")
	require.Equal(t, expected, string(got), "decrypted data does not match")
}

func initCipher(t *testing.T) *aes256 {
	t.Helper()
	key := make([]byte, 32) // AES-256 key size is 32 bytes
	_, err := io.ReadFull(rand.Reader, key)
	require.NoError(t, err)
	c, err := cipherAES256(key)
	require.NoError(t, err)
	return c
}

func setup(t *testing.T) (db database.Store, cryptDB *dbCrypt, cs []Cipher) {
	t.Helper()
	rawDB, _ := dbtestutil.NewDB(t)
	cs = append(cs, initCipher(t))
	cdb, err := New(context.Background(), rawDB, cs...)
	require.NoError(t, err)
	cryptDB, ok := cdb.(*dbCrypt)
	require.True(t, ok)
	return rawDB, cryptDB, cs
}

func setupNoCiphers(t *testing.T) (db database.Store, cryptodb *dbCrypt) {
	t.Helper()
	rawDB, _ := dbtestutil.NewDB(t)
	cdb, err := New(context.Background(), rawDB)
	require.NoError(t, err)
	cryptDB, ok := cdb.(*dbCrypt)
	require.True(t, ok)
	return rawDB, cryptDB
}

func fakeBase64RandomData(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, b)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}
