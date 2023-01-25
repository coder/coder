package coderd

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/testutil"
)

func TestAPIKeyEncryption(t *testing.T) {
	t.Parallel()

	generateAPIKey := func(t *testing.T, db database.Store) (keyID, keySecret string, hashedSecret []byte, data encryptedAPIKeyPayload) {
		keyID, keySecret, err := GenerateAPIKeyIDSecret()
		require.NoError(t, err)

		hashedSecretArray := sha256.Sum256([]byte(keySecret))
		data = encryptedAPIKeyPayload{
			APIKey:    keyID + "-" + keySecret,
			ExpiresAt: database.Now().Add(24 * time.Hour),
		}

		return keyID, keySecret, hashedSecretArray[:], data
	}
	insertAPIKey := func(t *testing.T, db database.Store, keyID string, hashedSecret []byte) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := db.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			ID:           keyID,
			HashedSecret: hashedSecret,
			LoginType:    database.LoginTypePassword,
			Scope:        database.APIKeyScopeAll,
		})
		require.NoError(t, err)
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		db := databasefake.New()
		keyID, _, hashedSecret, data := generateAPIKey(t, db)
		insertAPIKey(t, db, keyID, hashedSecret)

		encrypted, err := encryptAPIKey(data)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		key, token, err := decryptAPIKey(ctx, db, encrypted)
		require.NoError(t, err)
		require.Equal(t, keyID, key.ID)
		require.Equal(t, hashedSecret[:], key.HashedSecret)
		require.Equal(t, data.APIKey, token)
	})

	t.Run("Verifies", func(t *testing.T) {
		t.Parallel()

		t.Run("Expiry", func(t *testing.T) {
			t.Parallel()
			db := databasefake.New()
			keyID, _, hashedSecret, data := generateAPIKey(t, db)
			insertAPIKey(t, db, keyID, hashedSecret)

			data.ExpiresAt = database.Now().Add(-1 * time.Hour)
			encrypted, err := encryptAPIKey(data)
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			_, _, err = decryptAPIKey(ctx, db, encrypted)
			require.Error(t, err)
			require.ErrorContains(t, err, "expired")
		})

		t.Run("KeyMatches", func(t *testing.T) {
			t.Parallel()
			db := databasefake.New()
			keyID, _, _, data := generateAPIKey(t, db)
			hashedSecret := sha256.Sum256([]byte("wrong"))
			insertAPIKey(t, db, keyID, hashedSecret[:])

			encrypted, err := encryptAPIKey(data)
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			_, _, err = decryptAPIKey(ctx, db, encrypted)
			require.Error(t, err)
			require.ErrorContains(t, err, "error in crypto")
		})
	})
}
