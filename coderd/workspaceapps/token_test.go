package workspaceapps_test

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
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
		{
			name: "PortPortocolHTTP",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				Prefix:            "yolo--",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "8080",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					Prefix:            "yolo--",
					BasePath:          "/",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "8080",
				},
			},
			want: true,
		},
		{
			name: "PortPortocolHTTPS",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				Prefix:            "yolo--",
				BasePath:          "/",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "8080s",
			},
			token: workspaceapps.SignedToken{
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					Prefix:            "yolo--",
					BasePath:          "/",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "8080s",
				},
			},
			want: true,
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

		ctx := testutil.Context(t, testutil.WaitShort)
		signer := newSigner(t)

		tokenStr, err := jwtutils.Sign(ctx, signer, workspaceapps.SignedToken{
			Request: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				BasePath:          "/app",
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},

			UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
			WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
			AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
			AppURL:      "http://127.0.0.1:8080",
		})
		require.NoError(t, err)

		var token workspaceapps.SignedToken
		err = jwtutils.Verify(ctx, signer, tokenStr, &token)
		require.NoError(t, err)

		require.WithinDuration(t, time.Now().Add(time.Minute), token.Expiry.Time(), 15*time.Second)
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
				Claims: jwt.Claims{
					Expiry: jwt.NewNumericDate(future),
				},
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodPath,
					BasePath:          "/app",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "qux",
				},

				UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
				WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
				AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
				AppURL:      "http://127.0.0.1:8080",
			},
		},
		{
			name: "OK2",
			token: workspaceapps.SignedToken{
				Claims: jwt.Claims{
					Expiry: jwt.NewNumericDate(future),
				},
				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					BasePath:          "/",
					UsernameOrID:      "oof",
					WorkspaceNameOrID: "rab",
					AgentNameOrID:     "zab",
					AppSlugOrPort:     "xuq",
				},

				UserID:      uuid.MustParse("6fa684a3-11aa-49fd-8512-ab527bd9b900"),
				WorkspaceID: uuid.MustParse("b2d816cc-505c-441d-afdf-dae01781bc0b"),
				AgentID:     uuid.MustParse("6c4396e1-af88-4a8a-91a3-13ea54fc29fb"),
				AppURL:      "http://localhost:9090",
			},
		},
		{
			name: "Expired",
			token: workspaceapps.SignedToken{
				Claims: jwt.Claims{
					Expiry: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
				},

				Request: workspaceapps.Request{
					AccessMethod:      workspaceapps.AccessMethodSubdomain,
					BasePath:          "/",
					UsernameOrID:      "foo",
					WorkspaceNameOrID: "bar",
					AgentNameOrID:     "baz",
					AppSlugOrPort:     "qux",
				},

				UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
				WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
				AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
				AppURL:      "http://127.0.0.1:8080",
			},
			parseErrContains: "token expired",
		},
	}

	signer := newSigner(t)
	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			str, err := jwtutils.Sign(ctx, signer, c.token)
			require.NoError(t, err)

			// Tokens aren't deterministic as they have a random nonce, so we
			// can't compare them directly.

			var token workspaceapps.SignedToken
			err = jwtutils.Verify(ctx, signer, str, &token)
			if c.parseErrContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, c.parseErrContains)
			} else {
				require.NoError(t, err)
				// normalize the expiry
				require.WithinDuration(t, c.token.Expiry.Time(), token.Expiry.Time(), 10*time.Second)
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

		ctx := testutil.Context(t, testutil.WaitShort)
		signer := newSigner(t)

		token := workspaceapps.SignedToken{
			Claims: jwt.Claims{
				Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Request: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				BasePath:          "/",
				UsernameOrID:      "user",
				WorkspaceAndAgent: "workspace/agent",
				WorkspaceNameOrID: "workspace",
				AgentNameOrID:     "agent",
				AppSlugOrPort:     "app",
			},
			UserID:      uuid.New(),
			WorkspaceID: uuid.New(),
			AgentID:     uuid.New(),
			AppURL:      "/",
		}

		// Add an expired cookie
		expired := token
		expired.Claims.Expiry = jwt.NewNumericDate(time.Now().Add(time.Hour * -1))
		expiredStr, err := jwtutils.Sign(ctx, signer, expired)
		require.NoError(t, err)
		r.AddCookie(&http.Cookie{
			Name:  codersdk.SignedAppTokenCookie,
			Value: expiredStr,
		})

		validStr, err := jwtutils.Sign(ctx, signer, token)
		require.NoError(t, err)

		r.AddCookie(&http.Cookie{
			Name:  codersdk.SignedAppTokenCookie,
			Value: validStr,
		})

		signed, ok := workspaceapps.FromRequest(r, signer)
		require.True(t, ok, "expected a token to be found")
		// Confirm it is the correct token.
		require.Equal(t, signed.UserID, token.UserID)
	})
}

func newSigner(t *testing.T) jwtutils.SigningKeyManager {
	t.Helper()

	return jwtutils.StaticKeyManager{
		ID:  "test",
		Key: generateSecret(t, 64),
	}
}

func generateSecret(t *testing.T, size int) []byte {
	t.Helper()

	secret := make([]byte, size)
	_, err := rand.Read(secret)
	require.NoError(t, err)
	return secret
}
