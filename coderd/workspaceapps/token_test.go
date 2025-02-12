package workspaceapps_test

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"

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
			RegisteredClaims: jwtutils.RegisteredClaims{
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
		expired.RegisteredClaims.Expiry = jwt.NewNumericDate(time.Now().Add(time.Hour * -1))
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

func newSigner(t *testing.T) jwtutils.StaticKey {
	t.Helper()

	return jwtutils.StaticKey{
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
