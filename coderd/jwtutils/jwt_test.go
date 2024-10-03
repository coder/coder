package jwtutils_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestClaims(t *testing.T) {
	t.Parallel()

	type tokenType struct {
		Name    string
		KeySize int
		Sign    bool
	}

	types := []tokenType{
		{
			Name:    "JWE",
			Sign:    false,
			KeySize: 32,
		},
		{
			Name:    "JWS",
			Sign:    true,
			KeySize: 64,
		},
	}

	type testcase struct {
		name           string
		claims         jwt.Claims
		expectedClaims jwt.Expected
		expectedErr    error
	}

	cases := []testcase{
		{
			name: "OK",
			claims: jwt.Claims{
				Issuer:    "coder",
				Subject:   "user@coder.com",
				Audience:  jwt.Audience{"coder"},
				Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
			},
		},
		{
			name: "WrongIssuer",
			claims: jwt.Claims{
				Issuer:    "coder",
				Subject:   "user@coder.com",
				Audience:  jwt.Audience{"coder"},
				Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
			},
			expectedClaims: jwt.Expected{
				Issuer: "coder2",
			},
			expectedErr: jwt.ErrInvalidIssuer,
		},
		{
			name: "WrongSubject",
			claims: jwt.Claims{
				Issuer:    "coder",
				Subject:   "user@coder.com",
				Audience:  jwt.Audience{"coder"},
				Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
			},
			expectedClaims: jwt.Expected{
				Subject: "user2@coder.com",
			},
			expectedErr: jwt.ErrInvalidSubject,
		},
		{
			name: "WrongAudience",
			claims: jwt.Claims{
				Issuer:    "coder",
				Subject:   "user@coder.com",
				Audience:  jwt.Audience{"coder"},
				Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
			},
		},
		{
			name: "Expired",
			claims: jwt.Claims{
				Issuer:    "coder",
				Subject:   "user@coder.com",
				Audience:  jwt.Audience{"coder"},
				Expiry:    jwt.NewNumericDate(time.Now().Add(time.Minute)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
			},
			expectedClaims: jwt.Expected{
				Time: time.Now().Add(time.Minute * 3),
			},
			expectedErr: jwt.ErrExpired,
		},
		{
			name: "IssuedInFuture",
			claims: jwt.Claims{
				Issuer:   "coder",
				Subject:  "user@coder.com",
				Audience: jwt.Audience{"coder"},
				Expiry:   jwt.NewNumericDate(time.Now().Add(time.Minute)),
				IssuedAt: jwt.NewNumericDate(time.Now()),
			},
			expectedClaims: jwt.Expected{
				Time: time.Now().Add(-time.Minute * 3),
			},
			expectedErr: jwt.ErrIssuedInTheFuture,
		},
		{
			name: "IsBefore",
			claims: jwt.Claims{
				Issuer:    "coder",
				Subject:   "user@coder.com",
				Audience:  jwt.Audience{"coder"},
				Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
			},
			expectedClaims: jwt.Expected{
				Time: time.Now().Add(time.Minute * 3),
			},
			expectedErr: jwt.ErrNotValidYet,
		},
	}

	for _, tt := range types {
		tt := tt

		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			for _, c := range cases {
				c := c
				t.Run(c.name, func(t *testing.T) {
					t.Parallel()

					var (
						ctx   = testutil.Context(t, testutil.WaitShort)
						key   = newKey(t, tt.KeySize)
						token string
						err   error
					)

					if tt.Sign {
						token, err = jwtutils.Sign(ctx, key, c.claims)
					} else {
						token, err = jwtutils.Encrypt(ctx, key, c.claims)
					}
					require.NoError(t, err)

					var actual testClaims
					if tt.Sign {
						err = jwtutils.Verify(ctx, key, token, &actual)
					} else {
						err = jwtutils.Decrypt(ctx, key, token, &actual)
					}
					require.NoError(t, err)
					require.Equal(t, c.claims, actual)
				})
			}
		})
	}
}

func TestJWS(t *testing.T) {
	t.Run("WrongSignatureAlgorithm", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)
		)

		key := newKey(t, 64)

		token, err := jwtutils.Sign(ctx, key, jwt.Claims{})
		require.NoError(t, err)

		var actual testClaims
		err = jwtutils.Verify(ctx, key, token, &actual, withSignatureAlgorithm(jose.HS256))
		require.Error(t, err)

	})
}

func TestJWE(t *testing.T) {
	t.Run("WrongKeyAlgorithm", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)
			key = newKey(t, 32)
		)

		token, err := jwtutils.Encrypt(ctx, key, jwt.Claims{})
		require.NoError(t, err)

		var actual testClaims
		err = jwtutils.Decrypt(ctx, key, token, &actual, withKeyAlgorithm(jose.A128GCMKW))
		require.Error(t, err)
	})

	t.Run("WrongContentyEncryption", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)
			key = newKey(t, 32)
		)

		token, err := jwtutils.Encrypt(ctx, key, jwt.Claims{})
		require.NoError(t, err)

		var actual testClaims
		err = jwtutils.Decrypt(ctx, key, token, &actual, withContentEncryptionAlgorithm(jose.A128GCM))
		require.Error(t, err)
	})
}

func generateCryptoKey(t *testing.T, seq int32, now time.Time, keySize int) codersdk.CryptoKey {
	t.Helper()

	secret := generateSecret(t, keySize)

	return codersdk.CryptoKey{
		Feature:  codersdk.CryptoKeyFeatureTailnetResume,
		Secret:   hex.EncodeToString(secret),
		Sequence: seq,
		StartsAt: now,
	}
}

func generateSecret(t *testing.T, keySize int) []byte {
	t.Helper()

	b := make([]byte, keySize)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

type testClaims struct {
	MyClaim string `json:"my_claim"`
	jwt.Claims
}

func withExpected(e jwt.Expected) func(*jwtutils.VerifyOptions) {
	return func(opts *jwtutils.VerifyOptions) {
		opts.RegisteredClaims = e
	}
}

func withSignatureAlgorithm(alg jose.SignatureAlgorithm) func(*jwtutils.VerifyOptions) {
	return func(opts *jwtutils.VerifyOptions) {
		opts.SignatureAlgorithm = alg
	}
}

func withKeyAlgorithm(alg jose.KeyAlgorithm) func(*jwtutils.DecryptOptions) {
	return func(opts *jwtutils.DecryptOptions) {
		opts.KeyAlgorithm = alg
	}
}

func withContentEncryptionAlgorithm(alg jose.ContentEncryption) func(*jwtutils.DecryptOptions) {
	return func(opts *jwtutils.DecryptOptions) {
		opts.ContentEncryptionAlgorithm = alg
	}
}

type key struct {
	t      testing.TB
	id     string
	secret []byte
}

func newKey(t *testing.T, size int) *key {
	t.Helper()

	id := uuid.New().String()
	secret := generateSecret(t, size)

	return &key{
		t:      t,
		id:     id,
		secret: secret,
	}
}

func (k *key) SigningKey(ctx context.Context) (id string, key interface{}, err error) {
	return k.id, k.secret, nil
}

func (k *key) VerifyingKey(ctx context.Context, id string) (key interface{}, err error) {
	k.t.Helper()

	require.Equal(k.t, k.id, id)
	return k.secret, nil
}

func (k *key) EncryptingKey(ctx context.Context) (id string, key interface{}, err error) {
	return k.id, k.secret, nil
}

func (k *key) DecryptingKey(ctx context.Context, id string) (key interface{}, err error) {
	k.t.Helper()

	require.Equal(k.t, k.id, id)
	return k.secret, nil
}
