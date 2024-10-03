package jwtutils_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestJWT(t *testing.T) {
	t.Parallel()

	type tokenType struct {
		Name    string
		KeySize int
	}

	types := []tokenType{
		{
			Name:    "JWE",
			KeySize: 32,
		},
		{
			Name:    "JWS",
			KeySize: 64,
		},
	}

	for _, tt := range types {
		tt := tt

		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			t.Run("OK", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(ctx).Return(key, nil)
				keycache.EXPECT().Verifying(ctx, key.Sequence).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now()),
					},
					MyClaim: "my_value",
				}

				var token string
				var err error

				if tt.Name == "JWE" {
					token, err = jwtutils.Encrypt(ctx, keycache, claims)
					require.NoError(t, err)
				} else {
					token, err = jwtutils.Sign(ctx, keycache, claims)
					require.NoError(t, err)
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual)
				require.NoError(t, err)
				require.Equal(t, claims, actual)
			})

			t.Run("WrongIssuer", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(ctx).Return(key, nil)
				keycache.EXPECT().Verifying(ctx, key.Sequence).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now()),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withExpected(jwt.Expected{
					Issuer: "coder2",
				}))
				require.ErrorIs(t, err, jwt.ErrInvalidIssuer)
			})

			t.Run("WrongSubject", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(ctx).Return(key, nil)
				keycache.EXPECT().Verifying(ctx, key.Sequence).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now()),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withExpected(jwt.Expected{
					Subject: "user2@coder.com",
				}))
				require.ErrorIs(t, err, jwt.ErrInvalidSubject)
			})

			t.Run("WrongAudience", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
					key      = generateCryptoKey(t, 1234567890, now, tt.KeySize)
				)

				keycache.EXPECT().Signing(ctx).Return(key, nil)
				keycache.EXPECT().Verifying(ctx, key.Sequence).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now()),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withExpected(jwt.Expected{
					AnyAudience: jwt.Audience{"coder2"},
				}))
				require.ErrorIs(t, err, jwt.ErrInvalidAudience)
			})

			t.Run("Expired", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
					key      = generateCryptoKey(t, 1234567890, now, tt.KeySize)
				)

				keycache.EXPECT().Signing(ctx).Return(key, nil)
				keycache.EXPECT().Verifying(ctx, key.Sequence).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Minute)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now()),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withExpected(jwt.Expected{
					Time: time.Now().Add(time.Minute * 3),
				}))
				require.ErrorIs(t, err, jwt.ErrExpired)
			})

			t.Run("IssuedInFuture", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(ctx).Return(key, nil)
				keycache.EXPECT().Verifying(ctx, key.Sequence).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:   "coder",
						Subject:  "user@coder.com",
						Audience: jwt.Audience{"coder"},
						Expiry:   jwt.NewNumericDate(time.Now().Add(time.Minute)),
						IssuedAt: jwt.NewNumericDate(time.Now()),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withExpected(jwt.Expected{
					Time: time.Now().Add(-time.Minute * 3),
				}))
				require.ErrorIs(t, err, jwt.ErrIssuedInTheFuture)
			})

			t.Run("IsBefore", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(ctx).Return(key, nil)
				keycache.EXPECT().Verifying(ctx, key.Sequence).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withExpected(jwt.Expected{
					Time: time.Now().Add(time.Minute * 3),
				}))
				require.ErrorIs(t, err, jwt.ErrNotValidYet)
			})

			t.Run("WrongSignatureAlgorithm", func(t *testing.T) {
				t.Parallel()

				if tt.Name == "JWE" {
					t.Skip("JWE does not support this")
				}

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(ctx).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withSignatureAlgorithm(jose.HS256))
				require.Error(t, err)
			})

			t.Run("WrongKeyAlgorithm", func(t *testing.T) {
				t.Parallel()

				if tt.Name == "JWS" {
					t.Skip("JWS does not support this")
				}

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(ctx).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withKeyAlgorithm(jose.A128GCMKW))
				require.Error(t, err)
			})

			t.Run("WrongContentyEncryption", func(t *testing.T) {
				t.Parallel()

				if tt.Name == "JWS" {
					t.Skip("JWS does not support this")
				}

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(gomock.Any()).Return(key, nil)

				claims := testClaims{
					Claims: jwt.Claims{
						Issuer:    "coder",
						Subject:   "user@coder.com",
						Audience:  jwt.Audience{"coder"},
						Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
						NotBefore: jwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
					},
					MyClaim: "my_value",
				}

				token, err := tt.SignFn(ctx, keycache, claims)
				require.NoError(t, err)

				var actual testClaims
				err = tt.VerifyFn(ctx, keycache, token, &actual, withContentEncryptionAlgorithm(jose.A128GCM))
				require.Error(t, err)
			})
		})
	}
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

func withExpected(e jwt.Expected) func(*jwtutils.ParseOptions) {
	return func(opts *jwtutils.ParseOptions) {
		opts.RegisteredClaims = e
	}
}

func withSignatureAlgorithm(alg jose.SignatureAlgorithm) func(*jwtutils.ParseOptions) {
	return func(opts *jwtutils.ParseOptions) {
		opts.SignatureAlgorithm = alg
	}
}

func withKeyAlgorithm(alg jose.KeyAlgorithm) func(*jwtutils.ParseOptions) {
	return func(opts *jwtutils.ParseOptions) {
		opts.KeyAlgorithm = alg
	}
}

func withContentEncryptionAlgorithm(alg jose.ContentEncryption) func(*jwtutils.ParseOptions) {
	return func(opts *jwtutils.ParseOptions) {
		opts.ContentEncryptionAlgorithm = alg
	}
}

type godkey interface {
	jwtutils.SignKeyer
	jwtutils.VerifyKeyer
	jwtutils.EncryptKeyer
	jwtutils.DecryptKeyer
}

type key struct {
	signFn   func(context.Context) (string, interface{}, error)
	verifyFn func(context.Context, string) (interface{}, error)
}

func (k *key) SigningKey(ctx context.Context) (string, interface{}, error) {
	return k.signFn(ctx)
}

func (k *key) VerifyingKey(ctx context.Context, id string) (interface{}, error) {
	return k.verifyFn(ctx, id)
}

func (k *key) EncryptingKey(ctx context.Context) (string, interface{}, error) {
	return k.signFn(ctx)
}

func (k *key) DecryptingKey(ctx context.Context, id string) (interface{}, error) {
	return k.verifyFn(ctx, id)
}
