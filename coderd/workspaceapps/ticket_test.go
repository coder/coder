package workspaceapps_test

import (
	"encoding/hex"
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

	provider := workspaceapps.New(slogtest.Make(t, nil), nil, nil, nil, nil, nil, coderdtest.AppSigningKey)

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

	provider := workspaceapps.New(slogtest.Make(t, nil), nil, nil, nil, nil, nil, coderdtest.AppSigningKey)

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
		otherKey, err := hex.DecodeString("62656566646561646265656664656164626565666465616462656566646561646265656664656164626565666465616462656566646561646265656664656164")
		require.NoError(t, err)
		require.NotEqual(t, coderdtest.AppSigningKey, otherKey)
		require.Len(t, otherKey, 64)

		otherProvider := workspaceapps.New(slogtest.Make(t, nil), nil, nil, nil, nil, nil, otherKey)

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
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: provider.TicketSigningKey}, nil)
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
