package coderd

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/testutil"
)

func TestAPIKeyEncryption(t *testing.T) {
	t.Parallel()

	generateAPIKey := func(t *testing.T, db database.Store) (keyID, keyToken string, hashedSecret []byte, data encryptedAPIKeyPayload) {
		key, token := dbgen.APIKey(t, db, database.APIKey{})

		data = encryptedAPIKeyPayload{
			APIKey:    token,
			ExpiresAt: database.Now().Add(24 * time.Hour),
		}

		return key.ID, token, key.HashedSecret[:], data
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		keyID, _, hashedSecret, data := generateAPIKey(t, db)

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
			db := dbfake.New()
			_, _, _, data := generateAPIKey(t, db)

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
			db := dbfake.New()

			hashedSecret := sha256.Sum256([]byte("wrong"))
			// Insert a token with a mismatched hashed secret.
			_, token := dbgen.APIKey(t, db, database.APIKey{
				HashedSecret: hashedSecret[:],
			})

			data := encryptedAPIKeyPayload{
				APIKey:    token,
				ExpiresAt: database.Now().Add(24 * time.Hour),
			}

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
