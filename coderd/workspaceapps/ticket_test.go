package workspaceapps_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/square/go-jose.v2"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/workspaceapps"
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

		tokenStr, err := workspaceapps.GenerateToken(coderdtest.AppSigningKey, workspaceapps.SignedToken{
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

		token, err := workspaceapps.ParseToken(coderdtest.AppSigningKey, tokenStr)
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

			str, err := workspaceapps.GenerateToken(coderdtest.AppSigningKey, c.token)
			require.NoError(t, err)

			// Tokens aren't deterministic as they have a random nonce, so we
			// can't compare them directly.

			token, err := workspaceapps.ParseToken(coderdtest.AppSigningKey, str)
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

// The ParseToken fn is tested quite thoroughly in the GenerateToken test as
// well.
func Test_ParseToken(t *testing.T) {
	t.Parallel()

	t.Run("InvalidJWS", func(t *testing.T) {
		t.Parallel()

		token, err := workspaceapps.ParseToken(coderdtest.AppSigningKey, "invalid")
		require.Error(t, err)
		require.ErrorContains(t, err, "parse JWS")
		require.Equal(t, workspaceapps.SignedToken{}, token)
	})

	t.Run("VerifySignature", func(t *testing.T) {
		t.Parallel()

		// Create a valid token using a different key.
		otherKey, err := hex.DecodeString("62656566646561646265656664656164626565666465616462656566646561646265656664656164626565666465616462656566646561646265656664656164")
		require.NoError(t, err)
		require.NotEqual(t, coderdtest.AppSigningKey, otherKey)
		require.Len(t, otherKey, 64)

		tokenStr, err := workspaceapps.GenerateToken(otherKey, workspaceapps.SignedToken{
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
		token, err := workspaceapps.ParseToken(coderdtest.AppSigningKey, tokenStr)
		require.Error(t, err)
		require.ErrorContains(t, err, "verify JWS")
		require.Equal(t, workspaceapps.SignedToken{}, token)
	})

	t.Run("InvalidBody", func(t *testing.T) {
		t.Parallel()

		// Create a signature for an invalid body.
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: coderdtest.AppSigningKey}, nil)
		require.NoError(t, err)
		signedObject, err := signer.Sign([]byte("hi"))
		require.NoError(t, err)
		serialized, err := signedObject.CompactSerialize()
		require.NoError(t, err)

		token, err := workspaceapps.ParseToken(coderdtest.AppSigningKey, serialized)
		require.Error(t, err)
		require.ErrorContains(t, err, "unmarshal payload")
		require.Equal(t, workspaceapps.SignedToken{}, token)
	})
}
