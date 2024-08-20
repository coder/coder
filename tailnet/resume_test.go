package tailnet_test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestResumeTokenSigningKeyFromDatabase(t *testing.T) {
	t.Parallel()

	assertRandomKey := func(t *testing.T, key tailnet.ResumeTokenSigningKey) {
		t.Helper()
		assert.NotEqual(t, tailnet.ResumeTokenSigningKey{}, key, "key should not be empty")
		assert.NotEqualValues(t, [64]byte{1}, key, "key should not be all 1s")
	}

	t.Run("GenerateRetrieve", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		key1, err := tailnet.ResumeTokenSigningKeyFromDatabase(ctx, db)
		require.NoError(t, err)
		assertRandomKey(t, key1)

		key2, err := tailnet.ResumeTokenSigningKeyFromDatabase(ctx, db)
		require.NoError(t, err)
		require.Equal(t, key1, key2, "keys should not be different")
	})

	t.Run("GetError", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetCoordinatorResumeTokenSigningKey(gomock.Any()).Return("", assert.AnError)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := tailnet.ResumeTokenSigningKeyFromDatabase(ctx, db)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("UpsertError", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetCoordinatorResumeTokenSigningKey(gomock.Any()).Return("", nil)
		db.EXPECT().UpsertCoordinatorResumeTokenSigningKey(gomock.Any(), gomock.Any()).Return(assert.AnError)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := tailnet.ResumeTokenSigningKeyFromDatabase(ctx, db)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("DecodeErrorShouldRegenerate", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetCoordinatorResumeTokenSigningKey(gomock.Any()).Return("invalid", nil)

		var storedKey tailnet.ResumeTokenSigningKey
		db.EXPECT().UpsertCoordinatorResumeTokenSigningKey(gomock.Any(), gomock.Any()).Do(func(_ context.Context, value string) error {
			keyBytes, err := hex.DecodeString(value)
			require.NoError(t, err)
			require.Len(t, keyBytes, len(storedKey))
			copy(storedKey[:], keyBytes)
			return nil
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		key, err := tailnet.ResumeTokenSigningKeyFromDatabase(ctx, db)
		require.NoError(t, err)
		assertRandomKey(t, key)
		require.Equal(t, storedKey, key, "key should match stored value")
	})

	t.Run("LengthErrorShouldRegenerate", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetCoordinatorResumeTokenSigningKey(gomock.Any()).Return("deadbeef", nil)
		db.EXPECT().UpsertCoordinatorResumeTokenSigningKey(gomock.Any(), gomock.Any()).Return(nil)

		ctx := testutil.Context(t, testutil.WaitShort)
		key, err := tailnet.ResumeTokenSigningKeyFromDatabase(ctx, db)
		require.NoError(t, err)
		assertRandomKey(t, key)
	})

	t.Run("EmptyError", func(t *testing.T) {
		t.Parallel()

		db := dbmock.NewMockStore(gomock.NewController(t))
		emptyKey := hex.EncodeToString(make([]byte, 64))
		db.EXPECT().GetCoordinatorResumeTokenSigningKey(gomock.Any()).Return(emptyKey, nil)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := tailnet.ResumeTokenSigningKeyFromDatabase(ctx, db)
		require.ErrorContains(t, err, "is empty")
	})
}

func TestResumeTokenKeyProvider(t *testing.T) {
	t.Parallel()

	key, err := tailnet.GenerateResumeTokenSigningKey()
	require.NoError(t, err)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()
		clock := quartz.NewMock(t)
		provider := tailnet.NewResumeTokenKeyProvider(key, clock, tailnet.DefaultResumeTokenExpiry)
		token, err := provider.GenerateResumeToken(id)
		require.NoError(t, err)
		require.NotNil(t, token)
		require.NotEmpty(t, token.Token)
		require.Equal(t, tailnet.DefaultResumeTokenExpiry/2, token.RefreshIn.AsDuration())
		require.WithinDuration(t, clock.Now().Add(tailnet.DefaultResumeTokenExpiry), token.ExpiresAt.AsTime(), time.Second)

		gotID, err := provider.VerifyResumeToken(token.Token)
		require.NoError(t, err)
		require.Equal(t, id, gotID)
	})

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()

		id := uuid.New()
		clock := quartz.NewMock(t)
		provider := tailnet.NewResumeTokenKeyProvider(key, clock, tailnet.DefaultResumeTokenExpiry)
		token, err := provider.GenerateResumeToken(id)
		require.NoError(t, err)
		require.NotNil(t, token)
		require.NotEmpty(t, token.Token)
		require.Equal(t, tailnet.DefaultResumeTokenExpiry/2, token.RefreshIn.AsDuration())
		require.WithinDuration(t, clock.Now().Add(tailnet.DefaultResumeTokenExpiry), token.ExpiresAt.AsTime(), time.Second)

		// Advance time past expiry
		_ = clock.Advance(tailnet.DefaultResumeTokenExpiry + time.Second)

		_, err = provider.VerifyResumeToken(token.Token)
		require.ErrorContains(t, err, "expired")
	})

	t.Run("InvalidToken", func(t *testing.T) {
		t.Parallel()

		provider := tailnet.NewResumeTokenKeyProvider(key, quartz.NewMock(t), tailnet.DefaultResumeTokenExpiry)
		_, err := provider.VerifyResumeToken("invalid")
		require.ErrorContains(t, err, "parse JWS")
	})

	t.Run("VerifyError", func(t *testing.T) {
		t.Parallel()

		// Generate a resume token with a different key
		otherKey, err := tailnet.GenerateResumeTokenSigningKey()
		require.NoError(t, err)
		otherProvider := tailnet.NewResumeTokenKeyProvider(otherKey, quartz.NewMock(t), tailnet.DefaultResumeTokenExpiry)
		token, err := otherProvider.GenerateResumeToken(uuid.New())
		require.NoError(t, err)

		provider := tailnet.NewResumeTokenKeyProvider(key, quartz.NewMock(t), tailnet.DefaultResumeTokenExpiry)
		_, err = provider.VerifyResumeToken(token.Token)
		require.ErrorContains(t, err, "verify JWS")
	})
}
