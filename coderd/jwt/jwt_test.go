package jwt_test

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	jjwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/jwt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestJWT(t *testing.T) {
	t.Parallel()

	type tokenType struct {
		Name     string
		SecureFn func(jwt.Claims, jwt.SecuringKeyFn) (string, error)
		ParseFn  func(string, jwt.Claims, jwt.ParseKeyFunc, ...func(*jwt.ParseOptions)) error
		KeySize  int
	}

	types := []tokenType{
		{
			Name:     "JWE",
			SecureFn: jwt.Encrypt,
			ParseFn:  jwt.Decrypt,
			KeySize:  32,
		},
		{
			Name:     "JWS",
			SecureFn: jwt.Sign,
			ParseFn:  jwt.Verify,
			KeySize:  64,
		},
	}

	for _, tt := range types {
		tt := tt

		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			t.Run("Basic", func(t *testing.T) {
				t.Parallel()

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now()),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key))
				require.NoError(t, err)
			})

			t.Run("Keycache", func(t *testing.T) {
				t.Parallel()

				var (
					ctx      = testutil.Context(t, testutil.WaitShort)
					ctrl     = gomock.NewController(t)
					keycache = cryptokeys.NewMockKeycache(ctrl)
					now      = time.Now()
				)

				key := generateCryptoKey(t, 1234567890, now, tt.KeySize)

				keycache.EXPECT().Signing(gomock.Any()).Return(key, nil)
				keycache.EXPECT().Verifying(gomock.Any(), key.Sequence).Return(key, nil)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now()),
				}

				token, err := tt.SecureFn(claims, jwt.SecuringKeyFromCache(ctx, keycache))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, jwt.ParseKeyFromCache(ctx, keycache))
				require.NoError(t, err)
			})

			t.Run("WrongIssuer", func(t *testing.T) {
				t.Parallel()

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now()),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withExpected(jjwt.Expected{
					Issuer: "coder2",
				}))
				require.ErrorIs(t, err, jjwt.ErrInvalidIssuer)
			})

			t.Run("WrongSubject", func(t *testing.T) {
				t.Parallel()

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now()),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withExpected(jjwt.Expected{
					Subject: "user2@coder.com",
				}))
				require.ErrorIs(t, err, jjwt.ErrInvalidSubject)
			})

			t.Run("WrongAudience", func(t *testing.T) {
				t.Parallel()

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now()),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withExpected(jjwt.Expected{
					AnyAudience: jjwt.Audience{"coder2"},
				}))
				require.ErrorIs(t, err, jjwt.ErrInvalidAudience)
			})

			t.Run("Expired", func(t *testing.T) {
				t.Parallel()

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Minute)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now()),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withExpected(jjwt.Expected{
					Time: time.Now().Add(time.Minute * 3),
				}))
				require.ErrorIs(t, err, jjwt.ErrExpired)
			})

			t.Run("IssuedInFuture", func(t *testing.T) {
				t.Parallel()

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:   "coder",
					Subject:  "user@coder.com",
					Audience: jjwt.Audience{"coder"},
					Expiry:   jjwt.NewNumericDate(time.Now().Add(time.Minute)),
					IssuedAt: jjwt.NewNumericDate(time.Now()),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withExpected(jjwt.Expected{
					Time: time.Now().Add(-time.Minute * 3),
				}))
				require.ErrorIs(t, err, jjwt.ErrIssuedInTheFuture)
			})

			t.Run("IsBefore", func(t *testing.T) {
				t.Parallel()

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withExpected(jjwt.Expected{
					Time: time.Now().Add(time.Minute * 3),
				}))
				require.ErrorIs(t, err, jjwt.ErrNotValidYet)
			})

			t.Run("WrongSignatureAlgorithm", func(t *testing.T) {
				t.Parallel()

				if tt.Name == "JWE" {
					t.Skip("JWE does not support this")
				}

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withSignatureAlgorithm(jose.HS256))
				require.Error(t, err)
			})

			t.Run("WrongKeyAlgorithm", func(t *testing.T) {
				t.Parallel()

				if tt.Name == "JWS" {
					t.Skip("JWS does not support this")
				}

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withKeyAlgorithm(jose.A128GCMKW))
				require.Error(t, err)
			})

			t.Run("WrongContentyEncryption", func(t *testing.T) {
				t.Parallel()

				if tt.Name == "JWS" {
					t.Skip("JWS does not support this")
				}

				id := uuid.New().String()
				key := generateSecret(t, tt.KeySize)

				claims := jjwt.Claims{
					Issuer:    "coder",
					Subject:   "user@coder.com",
					Audience:  jjwt.Audience{"coder"},
					Expiry:    jjwt.NewNumericDate(time.Now().Add(time.Hour)),
					IssuedAt:  jjwt.NewNumericDate(time.Now()),
					NotBefore: jjwt.NewNumericDate(time.Now().Add(time.Minute * 5)),
				}

				token, err := tt.SecureFn(claims, securingKeyFn(id, key))
				require.NoError(t, err)

				var actual testClaims
				err = tt.ParseFn(token, &actual, verifyingKeyFn(id, key), withContentEncryptionAlgorithm(jose.A128GCM))
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
	jjwt.Claims
}

func securingKeyFn(id string, key []byte) jwt.SecuringKeyFn {
	return func() (string, interface{}, error) {
		return id, key, nil
	}
}

func verifyingKeyFn(id string, key []byte) jwt.ParseKeyFunc {
	return func(header jose.Header) (interface{}, error) {
		if header.KeyID != id {
			return nil, xerrors.Errorf("expected key ID %q, got %q", id, header.KeyID)
		}
		return key, nil
	}
}

func withExpected(e jjwt.Expected) func(*jwt.ParseOptions) {
	return func(opts *jwt.ParseOptions) {
		opts.RegisteredClaims = e
	}
}

func withSignatureAlgorithm(alg jose.SignatureAlgorithm) func(*jwt.ParseOptions) {
	return func(opts *jwt.ParseOptions) {
		opts.SignatureAlgorithm = alg
	}
}

func withKeyAlgorithm(alg jose.KeyAlgorithm) func(*jwt.ParseOptions) {
	return func(opts *jwt.ParseOptions) {
		opts.KeyAlgorithm = alg
	}
}

func withContentEncryptionAlgorithm(alg jose.ContentEncryption) func(*jwt.ParseOptions) {
	return func(opts *jwt.ParseOptions) {
		opts.ContentEncryptionAlgorithm = alg
	}
}
