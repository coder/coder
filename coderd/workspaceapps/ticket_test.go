package workspaceapps_test

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/square/go-jose.v2"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/workspaceapps"
)

func Test_TicketMatchesRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		req    workspaceapps.Request
		ticket workspaceapps.Ticket
		want   bool
	}{
		{
			name: "OK",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			ticket: workspaceapps.Ticket{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			want: true,
		},
		{
			name: "DifferentAccessMethod",
			req: workspaceapps.Request{
				AccessMethod: workspaceapps.AccessMethodPath,
			},
			ticket: workspaceapps.Ticket{
				AccessMethod: workspaceapps.AccessMethodSubdomain,
			},
			want: false,
		},
		{
			name: "DifferentUsernameOrID",
			req: workspaceapps.Request{
				AccessMethod: workspaceapps.AccessMethodPath,
				UsernameOrID: "foo",
			},
			ticket: workspaceapps.Ticket{
				AccessMethod: workspaceapps.AccessMethodPath,
				UsernameOrID: "bar",
			},
			want: false,
		},
		{
			name: "DifferentWorkspaceNameOrID",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
			},
			ticket: workspaceapps.Ticket{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "baz",
			},
			want: false,
		},
		{
			name: "DifferentAgentNameOrID",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
			},
			ticket: workspaceapps.Ticket{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "qux",
			},
			want: false,
		},
		{
			name: "DifferentAppSlugOrPort",
			req: workspaceapps.Request{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",
			},
			ticket: workspaceapps.Ticket{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "quux",
			},
			want: false,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, c.want, c.ticket.MatchesRequest(c.req))
		})
	}
}

func Test_GenerateTicket(t *testing.T) {
	t.Parallel()

	appSigningKeyBlock, _ := pem.Decode([]byte(coderdtest.TestAppSigningKey))
	require.NotNil(t, appSigningKeyBlock)
	appSigningKeyInterface, err := x509.ParsePKCS8PrivateKey(appSigningKeyBlock.Bytes)
	require.NoError(t, err)
	appSigningKey, ok := appSigningKeyInterface.(*rsa.PrivateKey)
	require.True(t, ok)

	provider := workspaceapps.New(slogtest.Make(t, nil), nil, nil, nil, nil, nil, appSigningKey)

	t.Run("SetExpiry", func(t *testing.T) {
		t.Parallel()

		ticketStr, err := provider.GenerateTicket(workspaceapps.Ticket{
			AccessMethod:      workspaceapps.AccessMethodPath,
			UsernameOrID:      "foo",
			WorkspaceNameOrID: "bar",
			AgentNameOrID:     "baz",
			AppSlugOrPort:     "qux",

			Expiry:      0,
			UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
			WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
			AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
			AppURL:      "http://127.0.0.1:8080",
		})
		require.NoError(t, err)

		ticket, err := provider.ParseTicket(ticketStr)
		require.NoError(t, err)

		require.InDelta(t, time.Now().Unix(), ticket.Expiry, time.Minute.Seconds())
	})

	future := time.Now().Add(time.Hour).Unix()
	cases := []struct {
		name             string
		ticket           workspaceapps.Ticket
		parseErrContains string
	}{
		{
			name: "OK1",
			ticket: workspaceapps.Ticket{
				AccessMethod:      workspaceapps.AccessMethodPath,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",

				Expiry:      future,
				UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
				WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
				AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
				AppURL:      "http://127.0.0.1:8080",
			},
		},
		{
			name: "OK2",
			ticket: workspaceapps.Ticket{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				UsernameOrID:      "oof",
				WorkspaceNameOrID: "rab",
				AgentNameOrID:     "zab",
				AppSlugOrPort:     "xuq",

				Expiry:      future,
				UserID:      uuid.MustParse("6fa684a3-11aa-49fd-8512-ab527bd9b900"),
				WorkspaceID: uuid.MustParse("b2d816cc-505c-441d-afdf-dae01781bc0b"),
				AgentID:     uuid.MustParse("6c4396e1-af88-4a8a-91a3-13ea54fc29fb"),
				AppURL:      "http://localhost:9090",
			},
		},
		{
			name: "Expired",
			ticket: workspaceapps.Ticket{
				AccessMethod:      workspaceapps.AccessMethodSubdomain,
				UsernameOrID:      "foo",
				WorkspaceNameOrID: "bar",
				AgentNameOrID:     "baz",
				AppSlugOrPort:     "qux",

				Expiry:      time.Now().Add(-time.Hour).Unix(),
				UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
				WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
				AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
				AppURL:      "http://127.0.0.1:8080",
			},
			parseErrContains: "ticket expired",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			str, err := provider.GenerateTicket(c.ticket)
			require.NoError(t, err)

			// Tickets aren't deterministic as they have a random nonce, so we
			// can't compare them directly.

			ticket, err := provider.ParseTicket(str)
			if c.parseErrContains != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, c.parseErrContains)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.ticket, ticket)
			}
		})
	}
}

// The ParseTicket fn is tested quite thoroughly in the GenerateTicket test.
func Test_ParseTicket(t *testing.T) {
	t.Parallel()

	appSigningKeyBlock, _ := pem.Decode([]byte(coderdtest.TestAppSigningKey))
	require.NotNil(t, appSigningKeyBlock)
	appSigningKeyInterface, err := x509.ParsePKCS8PrivateKey(appSigningKeyBlock.Bytes)
	require.NoError(t, err)
	appSigningKey, ok := appSigningKeyInterface.(*rsa.PrivateKey)
	require.True(t, ok)

	provider := workspaceapps.New(slogtest.Make(t, nil), nil, nil, nil, nil, nil, appSigningKey)

	t.Run("InvalidJWS", func(t *testing.T) {
		t.Parallel()

		ticket, err := provider.ParseTicket("invalid")
		require.Error(t, err)
		require.ErrorContains(t, err, "parse JWS")
		require.Equal(t, workspaceapps.Ticket{}, ticket)
	})

	t.Run("VerifySignature", func(t *testing.T) {
		t.Parallel()

		// Create a valid ticket using a different key.
		const otherKey = `
-----BEGIN PRIVATE KEY-----
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
p0XXoNQBpDfcuuV/0ylGtUvaoG6Q7jg=
-----END PRIVATE KEY-----`

		appSigningKeyBlock, _ := pem.Decode([]byte(otherKey))
		require.NotNil(t, appSigningKeyBlock)
		appSigningKeyInterface, err := x509.ParsePKCS8PrivateKey(appSigningKeyBlock.Bytes)
		require.NoError(t, err)
		appSigningKey, ok := appSigningKeyInterface.(*rsa.PrivateKey)
		require.True(t, ok)

		otherProvider := workspaceapps.New(slogtest.Make(t, nil), nil, nil, nil, nil, nil, appSigningKey)

		ticketStr, err := otherProvider.GenerateTicket(workspaceapps.Ticket{
			AccessMethod:      workspaceapps.AccessMethodPath,
			UsernameOrID:      "foo",
			WorkspaceNameOrID: "bar",
			AgentNameOrID:     "baz",
			AppSlugOrPort:     "qux",

			Expiry:      time.Now().Add(time.Hour).Unix(),
			UserID:      uuid.MustParse("b1530ba9-76f3-415e-b597-4ddd7cd466a4"),
			WorkspaceID: uuid.MustParse("1e6802d3-963e-45ac-9d8c-bf997016ffed"),
			AgentID:     uuid.MustParse("9ec18681-d2c9-4c9e-9186-f136efb4edbe"),
			AppURL:      "http://127.0.0.1:8080",
		})
		require.NoError(t, err)

		// Verify the ticket is invalid.
		ticket, err := provider.ParseTicket(ticketStr)
		require.Error(t, err)
		require.ErrorContains(t, err, "verify JWS")
		require.Equal(t, workspaceapps.Ticket{}, ticket)
	})

	t.Run("InvalidBody", func(t *testing.T) {
		t.Parallel()

		// Create a signature for an invalid body.
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.PS512, Key: provider.TicketSigningKey}, nil)
		require.NoError(t, err)
		signedObject, err := signer.Sign([]byte("hi"))
		require.NoError(t, err)
		serialized, err := signedObject.CompactSerialize()
		require.NoError(t, err)

		ticket, err := provider.ParseTicket(serialized)
		require.Error(t, err)
		require.ErrorContains(t, err, "unmarshal payload")
		require.Equal(t, workspaceapps.Ticket{}, ticket)
	})
}
