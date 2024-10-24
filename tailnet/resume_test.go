package tailnet_test

import (
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestResumeTokenKeyProvider(t *testing.T) {
	t.Parallel()

	key, err := tailnet.GenerateResumeTokenSigningKey()
	require.NoError(t, err)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		id := uuid.New()
		clock := quartz.NewMock(t)
		provider := tailnet.NewResumeTokenKeyProvider(newKeySigner(key), clock, tailnet.DefaultResumeTokenExpiry)
		token, err := provider.GenerateResumeToken(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, token)
		require.NotEmpty(t, token.Token)
		require.Equal(t, tailnet.DefaultResumeTokenExpiry/2, token.RefreshIn.AsDuration())
		require.WithinDuration(t, clock.Now().Add(tailnet.DefaultResumeTokenExpiry), token.ExpiresAt.AsTime(), time.Second)

		gotID, err := provider.VerifyResumeToken(ctx, token.Token)
		require.NoError(t, err)
		require.Equal(t, id, gotID)
	})

	t.Run("Expired", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		id := uuid.New()
		clock := quartz.NewMock(t)
		provider := tailnet.NewResumeTokenKeyProvider(newKeySigner(key), clock, tailnet.DefaultResumeTokenExpiry)
		token, err := provider.GenerateResumeToken(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, token)
		require.NotEmpty(t, token.Token)
		require.Equal(t, tailnet.DefaultResumeTokenExpiry/2, token.RefreshIn.AsDuration())
		require.WithinDuration(t, clock.Now().Add(tailnet.DefaultResumeTokenExpiry), token.ExpiresAt.AsTime(), time.Second)

		// Advance time past expiry. Account for leeway.
		_ = clock.Advance(tailnet.DefaultResumeTokenExpiry + time.Second*61)

		_, err = provider.VerifyResumeToken(ctx, token.Token)
		require.Error(t, err)
		require.ErrorIs(t, err, jwt.ErrExpired)
	})

	t.Run("InvalidToken", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		provider := tailnet.NewResumeTokenKeyProvider(newKeySigner(key), quartz.NewMock(t), tailnet.DefaultResumeTokenExpiry)
		_, err := provider.VerifyResumeToken(ctx, "invalid")
		require.ErrorContains(t, err, "parse JWS")
	})

	t.Run("VerifyError", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		// Generate a resume token with a different key
		otherKey, err := tailnet.GenerateResumeTokenSigningKey()
		require.NoError(t, err)
		otherSigner := newKeySigner(otherKey)
		otherProvider := tailnet.NewResumeTokenKeyProvider(otherSigner, quartz.NewMock(t), tailnet.DefaultResumeTokenExpiry)
		token, err := otherProvider.GenerateResumeToken(ctx, uuid.New())
		require.NoError(t, err)

		signer := newKeySigner(key)
		signer.ID = otherSigner.ID
		provider := tailnet.NewResumeTokenKeyProvider(signer, quartz.NewMock(t), tailnet.DefaultResumeTokenExpiry)
		_, err = provider.VerifyResumeToken(ctx, token.Token)
		require.ErrorIs(t, err, jose.ErrCryptoFailure)
	})
}

func newKeySigner(key tailnet.ResumeTokenSigningKey) jwtutils.StaticKey {
	return jwtutils.StaticKey{
		ID:  "123",
		Key: key[:],
	}
}
