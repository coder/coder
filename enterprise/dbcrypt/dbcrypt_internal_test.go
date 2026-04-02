package dbcrypt

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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

		expectedClaims := database.UserLinkClaims{
			IDTokenClaims: map[string]interface{}{
				"sub": "123",
				"groups": []interface{}{
					"foo", "bar",
				},
			},
			UserInfoClaims: map[string]interface{}{
				"number": float64(2),
				"struct": map[string]interface{}{
					"number": float64(2),
				},
			},
			MergedClaims: map[string]interface{}{
				"sub": "123",
				"groups": []interface{}{
					"foo", "bar",
				},
				"number": float64(2),
				"struct": map[string]interface{}{
					"number": float64(2),
				},
			},
		}

		updated, err := crypt.UpdateUserLink(ctx, database.UpdateUserLinkParams{
			OAuthAccessToken:  "access",
			OAuthRefreshToken: "refresh",
			UserID:            link.UserID,
			LoginType:         link.LoginType,
			Claims:            expectedClaims,
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
		require.EqualValues(t, expectedClaims, rawLink.Claims)
	})

	t.Run("UpdateExternalAuthLinkRefreshToken", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		user := dbgen.User(t, crypt, database.User{})
		link := dbgen.ExternalAuthLink(t, crypt, database.ExternalAuthLink{
			UserID: user.ID,
		})

		err := crypt.UpdateExternalAuthLinkRefreshToken(ctx, database.UpdateExternalAuthLinkRefreshTokenParams{
			OAuthRefreshToken:      "",
			OAuthRefreshTokenKeyID: link.OAuthRefreshTokenKeyID.String,
			OldOauthRefreshToken:   link.OAuthRefreshToken,
			UpdatedAt:              dbtime.Now(),
			ProviderID:             link.ProviderID,
			UserID:                 link.UserID,
		})
		require.NoError(t, err)

		rawLink, err := db.GetExternalAuthLink(ctx, database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], rawLink.OAuthRefreshToken, "")
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

func TestCryptoKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("InsertCryptoKey", func(t *testing.T) {
		t.Parallel()

		db, crypt, ciphers := setup(t)
		key := dbgen.CryptoKey(t, crypt, database.CryptoKey{
			Secret: sql.NullString{String: "test", Valid: true},
		})
		require.Equal(t, "test", key.Secret.String)

		key, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  key.Feature,
			Sequence: key.Sequence,
		})
		require.NoError(t, err)
		require.Equal(t, ciphers[0].HexDigest(), key.SecretKeyID.String)
		requireEncryptedEquals(t, ciphers[0], key.Secret.String, "test")
	})

	t.Run("GetCryptoKeys", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		_ = dbgen.CryptoKey(t, crypt, database.CryptoKey{
			Secret: sql.NullString{String: "test", Valid: true},
		})
		keys, err := crypt.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, "test", keys[0].Secret.String)
		require.Equal(t, ciphers[0].HexDigest(), keys[0].SecretKeyID.String)

		keys, err = db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireEncryptedEquals(t, ciphers[0], keys[0].Secret.String, "test")
		require.Equal(t, ciphers[0].HexDigest(), keys[0].SecretKeyID.String)
	})

	t.Run("GetLatestCryptoKeyByFeature", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		_ = dbgen.CryptoKey(t, crypt, database.CryptoKey{
			Secret: sql.NullString{String: "test", Valid: true},
		})
		key, err := crypt.GetLatestCryptoKeyByFeature(ctx, database.CryptoKeyFeatureWorkspaceAppsAPIKey)
		require.NoError(t, err)
		require.Equal(t, "test", key.Secret.String)
		require.Equal(t, ciphers[0].HexDigest(), key.SecretKeyID.String)

		key, err = db.GetLatestCryptoKeyByFeature(ctx, database.CryptoKeyFeatureWorkspaceAppsAPIKey)
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], key.Secret.String, "test")
		require.Equal(t, ciphers[0].HexDigest(), key.SecretKeyID.String)
	})

	t.Run("GetCryptoKeyByFeatureAndSequence", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		key := dbgen.CryptoKey(t, crypt, database.CryptoKey{
			Secret: sql.NullString{String: "test", Valid: true},
		})
		key, err := crypt.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: key.Sequence,
		})
		require.NoError(t, err)
		require.Equal(t, "test", key.Secret.String)
		require.Equal(t, ciphers[0].HexDigest(), key.SecretKeyID.String)

		key, err = db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: key.Sequence,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], key.Secret.String, "test")
		require.Equal(t, ciphers[0].HexDigest(), key.SecretKeyID.String)
	})

	t.Run("UpdateCryptoKeyDeletesAt", func(t *testing.T) {
		t.Parallel()
		_, crypt, ciphers := setup(t)
		key := dbgen.CryptoKey(t, crypt, database.CryptoKey{
			Secret: sql.NullString{String: "test", Valid: true},
		})
		key, err := crypt.UpdateCryptoKeyDeletesAt(ctx, database.UpdateCryptoKeyDeletesAtParams{
			Feature:  key.Feature,
			Sequence: key.Sequence,
			DeletesAt: sql.NullTime{
				Time:  time.Now().Add(time.Hour),
				Valid: true,
			},
		})
		require.NoError(t, err)
		require.Equal(t, "test", key.Secret.String)
		require.Equal(t, ciphers[0].HexDigest(), key.SecretKeyID.String)
	})

	t.Run("GetCryptoKeysByFeature", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		expected := dbgen.CryptoKey(t, crypt, database.CryptoKey{
			Sequence: 2,
			Feature:  database.CryptoKeyFeatureTailnetResume,
			Secret:   sql.NullString{String: "test", Valid: true},
		})
		_ = dbgen.CryptoKey(t, crypt, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: 43,
		})
		keys, err := crypt.GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureTailnetResume)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, "test", keys[0].Secret.String)
		require.Equal(t, ciphers[0].HexDigest(), keys[0].SecretKeyID.String)
		require.Equal(t, expected.Sequence, keys[0].Sequence)
		require.Equal(t, expected.Feature, keys[0].Feature)

		keys, err = db.GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureTailnetResume)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireEncryptedEquals(t, ciphers[0], keys[0].Secret.String, "test")
		require.Equal(t, ciphers[0].HexDigest(), keys[0].SecretKeyID.String)
		require.Equal(t, expected.Sequence, keys[0].Sequence)
		require.Equal(t, expected.Feature, keys[0].Feature)
	})

	t.Run("DecryptErr", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		key := dbgen.CryptoKey(t, db, database.CryptoKey{
			Secret: sql.NullString{
				String: fakeBase64RandomData(t, 32),
				Valid:  true,
			},
			SecretKeyID: sql.NullString{
				String: ciphers[0].HexDigest(),
				Valid:  true,
			},
		})
		_, err := crypt.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  key.Feature,
			Sequence: key.Sequence,
		})
		require.Error(t, err, "expected an error")
		var derr *DecryptFailedError
		require.ErrorAs(t, err, &derr, "expected a decrypt error")
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
		func(f func(store database.Store) error, _ *database.TxOptions) error {
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

// requireMCPServerConfigDecrypted verifies all encrypted fields on an
// MCPServerConfig match the expected plaintext values and carry the
// correct key-ID.
func requireMCPServerConfigDecrypted(
	t *testing.T,
	cfg database.MCPServerConfig,
	ciphers []Cipher,
	wantSecret, wantAPIKey, wantHeaders string,
) {
	t.Helper()
	require.Equal(t, wantSecret, cfg.OAuth2ClientSecret)
	require.Equal(t, wantAPIKey, cfg.APIKeyValue)
	require.Equal(t, wantHeaders, cfg.CustomHeaders)
	require.Equal(t, ciphers[0].HexDigest(), cfg.OAuth2ClientSecretKeyID.String)
	require.Equal(t, ciphers[0].HexDigest(), cfg.APIKeyValueKeyID.String)
	require.Equal(t, ciphers[0].HexDigest(), cfg.CustomHeadersKeyID.String)
}

// requireMCPServerConfigRawEncrypted reads the config from the raw
// (unwrapped) store and asserts every secret field is encrypted.
func requireMCPServerConfigRawEncrypted(
	ctx context.Context,
	t *testing.T,
	rawDB database.Store,
	cfgID uuid.UUID,
	ciphers []Cipher,
	wantSecret, wantAPIKey, wantHeaders string,
) {
	t.Helper()
	raw, err := rawDB.GetMCPServerConfigByID(ctx, cfgID)
	require.NoError(t, err)
	requireEncryptedEquals(t, ciphers[0], raw.OAuth2ClientSecret, wantSecret)
	requireEncryptedEquals(t, ciphers[0], raw.APIKeyValue, wantAPIKey)
	requireEncryptedEquals(t, ciphers[0], raw.CustomHeaders, wantHeaders)
}

func TestMCPServerConfigs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const (
		//nolint:gosec // test credentials
		oauthSecret   = "my-oauth-secret"
		apiKeyValue   = "my-api-key"
		customHeaders = `{"X-Custom":"header-value"}`
	)
	// insertConfig is a small helper that creates a user and an MCP
	// server config through the encrypted store, returning both.
	insertConfig := func(t *testing.T, crypt *dbCrypt, ciphers []Cipher) database.MCPServerConfig {
		t.Helper()
		user := dbgen.User(t, crypt, database.User{})
		cfg, err := crypt.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
			DisplayName:        "Test MCP Server",
			Slug:               "test-mcp-" + uuid.New().String()[:8],
			Description:        "test description",
			Url:                "https://mcp.example.com",
			Transport:          "streamable_http",
			AuthType:           "oauth2",
			OAuth2ClientID:     "client-id",
			OAuth2ClientSecret: oauthSecret,
			APIKeyValue:        apiKeyValue,
			CustomHeaders:      customHeaders,
			ToolAllowList:      []string{},
			ToolDenyList:       []string{},
			Availability:       "force_on",
			Enabled:            true,
			CreatedBy:          user.ID,
			UpdatedBy:          user.ID,
		})
		require.NoError(t, err)
		requireMCPServerConfigDecrypted(t, cfg, ciphers, oauthSecret, apiKeyValue, customHeaders)
		return cfg
	}

	t.Run("InsertMCPServerConfig", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, oauthSecret, apiKeyValue, customHeaders)
	})

	t.Run("GetMCPServerConfigByID", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)

		got, err := crypt.GetMCPServerConfigByID(ctx, cfg.ID)
		require.NoError(t, err)
		requireMCPServerConfigDecrypted(t, got, ciphers, oauthSecret, apiKeyValue, customHeaders)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, oauthSecret, apiKeyValue, customHeaders)
	})

	t.Run("GetMCPServerConfigBySlug", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)

		got, err := crypt.GetMCPServerConfigBySlug(ctx, cfg.Slug)
		require.NoError(t, err)
		requireMCPServerConfigDecrypted(t, got, ciphers, oauthSecret, apiKeyValue, customHeaders)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, oauthSecret, apiKeyValue, customHeaders)
	})

	t.Run("GetMCPServerConfigs", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)

		cfgs, err := crypt.GetMCPServerConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, cfgs, 1)
		requireMCPServerConfigDecrypted(t, cfgs[0], ciphers, oauthSecret, apiKeyValue, customHeaders)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, oauthSecret, apiKeyValue, customHeaders)
	})

	t.Run("GetMCPServerConfigsByIDs", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)

		cfgs, err := crypt.GetMCPServerConfigsByIDs(ctx, []uuid.UUID{cfg.ID})
		require.NoError(t, err)
		require.Len(t, cfgs, 1)
		requireMCPServerConfigDecrypted(t, cfgs[0], ciphers, oauthSecret, apiKeyValue, customHeaders)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, oauthSecret, apiKeyValue, customHeaders)
	})

	t.Run("GetEnabledMCPServerConfigs", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)

		cfgs, err := crypt.GetEnabledMCPServerConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, cfgs, 1)
		requireMCPServerConfigDecrypted(t, cfgs[0], ciphers, oauthSecret, apiKeyValue, customHeaders)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, oauthSecret, apiKeyValue, customHeaders)
	})

	t.Run("GetForcedMCPServerConfigs", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)

		cfgs, err := crypt.GetForcedMCPServerConfigs(ctx)
		require.NoError(t, err)
		require.Len(t, cfgs, 1)
		requireMCPServerConfigDecrypted(t, cfgs[0], ciphers, oauthSecret, apiKeyValue, customHeaders)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, oauthSecret, apiKeyValue, customHeaders)
	})

	t.Run("UpdateMCPServerConfig", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg := insertConfig(t, crypt, ciphers)

		const (
			//nolint:gosec // test credential
			newSecret  = "updated-oauth-secret"
			newAPIKey  = "updated-api-key"
			newHeaders = `{"X-New":"new-value"}`
		)
		updated, err := crypt.UpdateMCPServerConfig(ctx, database.UpdateMCPServerConfigParams{
			ID:                 cfg.ID,
			DisplayName:        cfg.DisplayName,
			Slug:               cfg.Slug,
			Description:        cfg.Description,
			Url:                cfg.Url,
			Transport:          cfg.Transport,
			AuthType:           cfg.AuthType,
			OAuth2ClientID:     cfg.OAuth2ClientID,
			OAuth2ClientSecret: newSecret,
			APIKeyValue:        newAPIKey,
			CustomHeaders:      newHeaders,
			ToolAllowList:      cfg.ToolAllowList,
			ToolDenyList:       cfg.ToolDenyList,
			Availability:       cfg.Availability,
			Enabled:            cfg.Enabled,
			UpdatedBy:          cfg.CreatedBy.UUID,
		})
		require.NoError(t, err)
		requireMCPServerConfigDecrypted(t, updated, ciphers, newSecret, newAPIKey, newHeaders)
		requireMCPServerConfigRawEncrypted(ctx, t, db, cfg.ID, ciphers, newSecret, newAPIKey, newHeaders)
	})
}

func TestMCPServerUserTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const (
		accessToken  = "access-token-value"
		refreshToken = "refresh-token-value"
	)

	// insertConfigAndToken creates a user, an MCP server config, and a
	// user token through the encrypted store.
	insertConfigAndToken := func(
		t *testing.T,
		crypt *dbCrypt,
		ciphers []Cipher,
	) (database.MCPServerConfig, database.MCPServerUserToken) {
		t.Helper()
		user := dbgen.User(t, crypt, database.User{})
		cfg, err := crypt.InsertMCPServerConfig(ctx, database.InsertMCPServerConfigParams{
			DisplayName:   "Token Test MCP",
			Slug:          "tok-mcp-" + uuid.New().String()[:8],
			Url:           "https://mcp.example.com",
			Transport:     "streamable_http",
			AuthType:      "oauth2",
			ToolAllowList: []string{},
			ToolDenyList:  []string{},
			Availability:  "default_off",
			Enabled:       true,
			CreatedBy:     user.ID,
			UpdatedBy:     user.ID,
		})
		require.NoError(t, err)

		tok, err := crypt.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID,
			UserID:            user.ID,
			AccessToken:       accessToken,
			RefreshToken:      refreshToken,
			TokenType:         "Bearer",
		})
		require.NoError(t, err)
		require.Equal(t, accessToken, tok.AccessToken)
		require.Equal(t, refreshToken, tok.RefreshToken)
		require.Equal(t, ciphers[0].HexDigest(), tok.AccessTokenKeyID.String)
		require.Equal(t, ciphers[0].HexDigest(), tok.RefreshTokenKeyID.String)
		return cfg, tok
	}

	t.Run("UpsertMCPServerUserToken", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg, tok := insertConfigAndToken(t, crypt, ciphers)

		// Verify the raw DB values are encrypted.
		rawTok, err := db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID,
			UserID:            tok.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], rawTok.AccessToken, accessToken)
		requireEncryptedEquals(t, ciphers[0], rawTok.RefreshToken, refreshToken)
	})

	t.Run("GetMCPServerUserToken", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg, tok := insertConfigAndToken(t, crypt, ciphers)

		got, err := crypt.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID,
			UserID:            tok.UserID,
		})
		require.NoError(t, err)
		require.Equal(t, accessToken, got.AccessToken)
		require.Equal(t, refreshToken, got.RefreshToken)
		require.Equal(t, ciphers[0].HexDigest(), got.AccessTokenKeyID.String)
		require.Equal(t, ciphers[0].HexDigest(), got.RefreshTokenKeyID.String)

		// Raw values must be encrypted.
		rawTok, err := db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID,
			UserID:            tok.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], rawTok.AccessToken, accessToken)
		requireEncryptedEquals(t, ciphers[0], rawTok.RefreshToken, refreshToken)
	})

	t.Run("GetMCPServerUserTokensByUserID", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		cfg, tok := insertConfigAndToken(t, crypt, ciphers)

		toks, err := crypt.GetMCPServerUserTokensByUserID(ctx, tok.UserID)
		require.NoError(t, err)
		require.Len(t, toks, 1)
		require.Equal(t, accessToken, toks[0].AccessToken)
		require.Equal(t, refreshToken, toks[0].RefreshToken)
		require.Equal(t, ciphers[0].HexDigest(), toks[0].AccessTokenKeyID.String)
		require.Equal(t, ciphers[0].HexDigest(), toks[0].RefreshTokenKeyID.String)

		// Raw values must be encrypted.
		rawTok, err := db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID,
			UserID:            tok.UserID,
		})
		require.NoError(t, err)
		requireEncryptedEquals(t, ciphers[0], rawTok.AccessToken, accessToken)
		requireEncryptedEquals(t, ciphers[0], rawTok.RefreshToken, refreshToken)
	})
}

func TestUserChatProviderKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const (
		//nolint:gosec // test credentials
		initialAPIKey = "sk-initial-api-key-value"
		//nolint:gosec // test credentials
		updatedAPIKey = "sk-updated-api-key-value"
	)

	insertProviderAndKey := func(
		t *testing.T,
		crypt *dbCrypt,
		ciphers []Cipher,
	) (database.ChatProvider, database.UserChatProviderKey) {
		t.Helper()
		user := dbgen.User(t, crypt, database.User{})
		provider, err := crypt.InsertChatProvider(ctx, database.InsertChatProviderParams{
			Provider:        "openai",
			DisplayName:     "OpenAI",
			APIKey:          "",
			Enabled:         true,
			AllowUserApiKey: true,
		})
		require.NoError(t, err)

		key, err := crypt.UpsertUserChatProviderKey(ctx, database.UpsertUserChatProviderKeyParams{
			UserID:         user.ID,
			ChatProviderID: provider.ID,
			APIKey:         initialAPIKey,
		})
		require.NoError(t, err)
		require.Equal(t, initialAPIKey, key.APIKey)
		require.Equal(t, ciphers[0].HexDigest(), key.ApiKeyKeyID.String)
		return provider, key
	}

	getUserChatProviderKey := func(t *testing.T, store interface {
		GetUserChatProviderKeys(context.Context, uuid.UUID) ([]database.UserChatProviderKey, error)
	}, userID uuid.UUID, providerID uuid.UUID,
	) database.UserChatProviderKey {
		t.Helper()
		keys, err := store.GetUserChatProviderKeys(ctx, userID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, providerID, keys[0].ChatProviderID)
		return keys[0]
	}

	t.Run("UpsertUserChatProviderKeyCreatesValue", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		provider, key := insertProviderAndKey(t, crypt, ciphers)

		got := getUserChatProviderKey(t, crypt, key.UserID, provider.ID)
		require.Equal(t, key.ID, got.ID)
		require.Equal(t, initialAPIKey, got.APIKey)
		require.Equal(t, ciphers[0].HexDigest(), got.ApiKeyKeyID.String)

		rawKey := getUserChatProviderKey(t, db, key.UserID, provider.ID)
		require.NotEqual(t, initialAPIKey, rawKey.APIKey)
		requireEncryptedEquals(t, ciphers[0], rawKey.APIKey, initialAPIKey)
	})

	t.Run("GetUserChatProviderKeys", func(t *testing.T) {
		t.Parallel()
		_, crypt, ciphers := setup(t)
		_, key := insertProviderAndKey(t, crypt, ciphers)

		keys, err := crypt.GetUserChatProviderKeys(ctx, key.UserID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, key.ID, keys[0].ID)
		require.Equal(t, initialAPIKey, keys[0].APIKey)
		require.Equal(t, ciphers[0].HexDigest(), keys[0].ApiKeyKeyID.String)
	})

	t.Run("UpsertUserChatProviderKeyUpdatesValue", func(t *testing.T) {
		t.Parallel()
		db, crypt, ciphers := setup(t)
		provider, key := insertProviderAndKey(t, crypt, ciphers)

		updated, err := crypt.UpsertUserChatProviderKey(ctx, database.UpsertUserChatProviderKeyParams{
			UserID:         key.UserID,
			ChatProviderID: provider.ID,
			APIKey:         updatedAPIKey,
		})
		require.NoError(t, err)
		require.Equal(t, key.ID, updated.ID)
		require.Equal(t, key.CreatedAt, updated.CreatedAt)
		require.False(t, updated.UpdatedAt.Before(key.UpdatedAt))
		require.Equal(t, updatedAPIKey, updated.APIKey)
		require.Equal(t, ciphers[0].HexDigest(), updated.ApiKeyKeyID.String)

		got := getUserChatProviderKey(t, crypt, key.UserID, provider.ID)
		require.Equal(t, updatedAPIKey, got.APIKey)
		require.Equal(t, ciphers[0].HexDigest(), got.ApiKeyKeyID.String)

		keys, err := crypt.GetUserChatProviderKeys(ctx, key.UserID)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, updatedAPIKey, keys[0].APIKey)

		rawKey := getUserChatProviderKey(t, db, key.UserID, provider.ID)
		require.NotEqual(t, updatedAPIKey, rawKey.APIKey)
		requireEncryptedEquals(t, ciphers[0], rawKey.APIKey, updatedAPIKey)
	})
}
