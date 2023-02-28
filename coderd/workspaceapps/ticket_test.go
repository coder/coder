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
UounQAJjlgsx9MOFyUPINW92R3KZ
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
