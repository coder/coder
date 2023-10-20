package workspaceapps_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/coder/v2/codersdk"

	"github.com/go-jose/go-jose/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/cryptorand"
)

func Test_TokenMatchesRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		req   workspaceapps.Request
		token workspaceapps.SignedToken
		want  bool
	}{
		{
			name: "OK",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "qux",
				},
			},
			want: true,
		},
		{
			name: "NormalizePath",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod: workspaceapps.AccessMethodPath,
					// With trailing slash
					BasePath:          "/app/",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "qux",
				},
			},
			want: true,
		},
		{
			name: "DifferentAccessMethod",
			req: workspaceapps.Request{
				AccessMethod: workspaceapps.AccessMethodPath,
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod: workspaceapps.AccessMethodSubdomain,
				},
			},
			want: false,
		},
		{
			name: "DifferentBasePath",
			req: workspaceapps.Request{
				AccessMethod: workspaceapps.AccessMethodPath,
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod: workspaceapps.AccessMethodPath,
					BasePath:     "/app",
				},
			},
			want: false,
		},
		{
			name: "DifferentUsernameOrID",
			req: workspaceapps.Request{
				AccessMethod: workspaceapps.AccessMethodPath,
				BasePath:     "/app",
				UsernameOrID: "foo",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod: workspaceapps.AccessMethodPath,
					BasePath:     "/app",
					UsernameOrID: "bar",
				},
			},
			want: false,
		},
		{
			name: "DifferentWorkspaceNameOrID",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "baz",
				},
			},
			want: false,
		},
		{
			name: "DifferentAgentNameOrID",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "qux",
				},
			},
			want: false,
		},
		{
			name: "DifferentAppSlugOrPort",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "quux",
				},
			},
			want: false,
		},
		{
			name: "SamePrefix",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				Prefix:            "dean-was--here---",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					Prefix:            "dean--was--here---",
					BasePath:          "/",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "quux",
				},
			},
			want: false,
		},
		{
			name: "DifferentPrefix",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				Prefix:            "yolo--",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					Prefix:            "swag--",
					BasePath:          "/",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "quux",
				},
			},
			want: false,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, c.want, c.token.MatchesRequest(c.req))
		})
	}
}

func Test_GenerateToken(t *testing.T) {
	t.Parallel()

	t.Run("SetExpiry", func(t *testing.T) {
		t.Parallel()

		tokenStr, err := coderdtest.AppSecurityKey.SignToken(workspaceapps.SignedToken{
			Request: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},

			Expiry:      time.Time{},
			UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
			WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
			AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
			AppURL:      "http://127.0.0.1:8080",
		})
		require.NoError(t, err)

		token, err := coderdtest.AppSecurityKey.VerifySignedToken(tokenStr)
		require.NoError(t, err)

		require.WithinDuration(t, time.Now().Add(time.Minute), token.Expiry, 15*time.Second)
	})

	future := time.Now().Add(time.Hour)
	cases := []struct {
		name             string
		token            workspaceapps.SignedToken
		parseErrContains string
	}{
		{
			name: "OK1",
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "qux",
				},

				Expiry:      future,
				UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
				WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
				AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
				AppURL:      "http://127.0.0.1:8080",
			},
		},
		{
			name: "OK2",
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					BasePath:          "/",
					UsernameOrID:      "oof",
					WorkspaceNameOrID: "rab",
					AgentNameOrID:     "zab",
					AppSlugOrPort:     "xuq",
				},

				Expiry:      future,
				UserID:      uuid.MustParse("6fa684a3-11aa-49fd-8512-ab527bd9b900"),
				WorkspaceID: uuid.MustParse("b2d816cc-505c-441d-afdf-dae01781bc0b"),
				AgentID:     uuid.MustParse("6c4396e1-af88-4a8a-91a3-13ea54fc29fb"),
				AppURL:      "http://localhost:9090",
			},
		},
		{
			name: "Expired",
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					BasePath:          "/",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "qux",
				},

				Expiry:      time.Now().Add(-time.Hour),
				UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
				WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
				AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
				AppURL:      "http://127.0.0.1:8080",
			},
			parseErrContains: "token expired",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			str, err := coderdtest.AppSecurityKey.SignToken(c.token)
			require.NoError(t, err)

			// Tokens aren't deterministic as they have a random nonce, so we
			// can't compare them directly.

			token, err := coderdtest.AppSecurityKey.VerifySignedToken(str)
			if c.parseErrContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, c.parseErrContains)
			} else {
				require.NoError(t, err)
				// normalize the expiry
				require.WithinDuration(t, c.token.Expiry, token.Expiry, 10*time.Second)
				c.token.Expiry = token.Expiry
				require.Equal(t, c.token, token)
			}
		})
	}
}

func Test_FromRequest(t *testing.T) {
	t.Parallel()

	t.Run("MultipleTokens", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest("GET", "/", nil)

		// Add an invalid token
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SignedAppTokenCookie,
			Value: "invalid",
		})

		token := workspaceapps.SignedToken{
			Request: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				BasePath:          "/",
				UsernameOrID:      "user",
				WorkspaceAndAgent: "workspace/agent",
				WorkspaceNameOrID: "workspace",
				AgentNameOrID:     "agent",
				AppSlugOrPort:     "app",
			},
			Expiry:      time.Now().Add(time.Hour),
			UserID:      uuid.New(),
			WorkspaceID: uuid.New(),
			AgentID:     uuid.New(),
			AppURL:      "/",
		}

		// Add an expired cookie
		expired := token
		expired.Expiry = time.Now().Add(time.Hour * -1)
		expiredStr, err := coderdtest.AppSecurityKey.SignToken(token)
		require.NoError(t, err)
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SignedAppTokenCookie,
			Value: expiredStr,
		})

		// Add a valid token
		validStr, err := coderdtest.AppSecurityKey.SignToken(token)
		require.NoError(t, err)

		r.AddCookie(&http.Cookie{
			Name:  codersdk.SignedAppTokenCookie,
			Value: validStr,
		})

		signed, ok := workspaceapps.FromRequest(r, coderdtest.AppSecurityKey)
		require.True(t, ok, "expected a token to be found")
		// Confirm it is the correct token.
		require.Equal(t, signed.UserID, token.UserID)
	})
}

// The ParseToken fn is tested quite thoroughly in the GenerateToken test as
// well.
func Test_ParseToken(t *testing.T) {
	t.Parallel()

	t.Run("InvalidJWS", func(t *testing.T) {
		t.Parallel()

		token, err := coderdtest.AppSecurityKey.VerifySignedToken("invalid")
		require.Error(t, err)
		require.ErrorContains(t, err, "parse JWS")
		require.Equal(t, workspaceapps.SignedToken{}, token)
	})

	t.Run("VerifySignature", func(t *testing.T) {
		t.Parallel()

		// Create a valid token using a different key.
		var otherKey workspaceapps.SecurityKey
		copy(otherKey[:], coderdtest.AppSecurityKey[:])
		for i := range otherKey {
			otherKey[i] ^= 0xff
		}
		require.NotEqual(t, coderdtest.AppSecurityKey, otherKey)

		tokenStr, err := otherKey.SignToken(workspaceapps.SignedToken{
			Request: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},

			Expiry:      time.Now().Add(time.Hour),
			UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
			WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
			AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
			AppURL:      "http://127.0.0.1:8080",
		})
		require.NoError(t, err)

		// Verify the token is invalid.
		token, err := coderdtest.AppSecurityKey.VerifySignedToken(tokenStr)
		require.Error(t, err)
		require.ErrorContains(t, err, "verify JWS")
		require.Equal(t, workspaceapps.SignedToken{}, token)
	})

	t.Run("InvalidBody", func(t *testing.T) {
		t.Parallel()

		// Create a signature for an invalid body.
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: coderdtest.AppSecurityKey[:64]}, nil)
		require.NoError(t, err)
		signedObject, err := signer.Sign([]byte("hi"))
		require.NoError(t, err)
		serialized, err := signedObject.CompactSerialize()
		require.NoError(t, err)

		token, err := coderdtest.AppSecurityKey.VerifySignedToken(serialized)
		require.Error(t, err)
		require.ErrorContains(t, err, "unmarshal payload")
		require.Equal(t, workspaceapps.SignedToken{}, token)
	})
}

func TestAPIKeyEncryption(t *testing.T) {
	t.Parallel()

	genAPIKey := func(t *testing.T) string {
		id, _ := cryptorand.String(10)
		secret, _ := cryptorand.String(22)

		return fmt.Sprintf("%s-%s", id, secret)
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		key := genAPIKey(t)
		encrypted, err := coderdtest.AppSecurityKey.EncryptAPIKey(workspaceapps.EncryptedAPIKeyPayload{
			APIKey: key,
		})
		require.NoError(t, err)

		decryptedKey, err := coderdtest.AppSecurityKey.DecryptAPIKey(encrypted)
		require.NoError(t, err)
		require.Equal(t, key, decryptedKey)
	})

	t.Run("Verifies", func(t *testing.T) {
		t.Parallel()

		t.Run("Expiry", func(t *testing.T) {
			t.Parallel()

			key := genAPIKey(t)
			encrypted, err := coderdtest.AppSecurityKey.EncryptAPIKey(workspaceapps.EncryptedAPIKeyPayload{
				APIKey:    key,
				ExpiresAt: dbtime.Now().Add(-1 * time.Hour),
			})
			require.NoError(t, err)

			decryptedKey, err := coderdtest.AppSecurityKey.DecryptAPIKey(encrypted)
			require.Error(t, err)
			require.ErrorContains(t, err, "expired")
			require.Empty(t, decryptedKey)
		})

		t.Run("EncryptionKey", func(t *testing.T) {
			t.Parallel()

			// Create a valid token using a different key.
			var otherKey workspaceapps.SecurityKey
			copy(otherKey[:], coderdtest.AppSecurityKey[:])
			for i := range otherKey {
				otherKey[i] ^= 0xff
			}
			require.NotEqual(t, coderdtest.AppSecurityKey, otherKey)

			// Encrypt with the other key.
			key := genAPIKey(t)
			encrypted, err := otherKey.EncryptAPIKey(workspaceapps.EncryptedAPIKeyPayload{
				APIKey: key,
			})
			require.NoError(t, err)

			// Decrypt with the original key.
			decryptedKey, err := coderdtest.AppSecurityKey.DecryptAPIKey(encrypted)
			require.Error(t, err)
			require.ErrorContains(t, err, "decrypt API key")
			require.Empty(t, decryptedKey)
		})
	})
}
