package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

func TestInternalMCPServersForChat(t *testing.T) {
	t.Parallel()

	chat := database.Chat{ID: uuid.New()}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "attached"}, nil)
	var gotChatIDs []uuid.UUID
	serverFor := func(s *mcp.Server, err error) func(context.Context, database.Chat) (*mcp.Server, error) {
		return func(_ context.Context, c database.Chat) (*mcp.Server, error) {
			gotChatIDs = append(gotChatIDs, c.ID)
			return s, err
		}
	}
	attached := InternalMCPServer{Slug: "attached", ServerForChat: serverFor(mcpServer, nil)}
	server := &Server{
		logger: slogtest.Make(t, nil),
		internalMCPServers: []InternalMCPServer{
			attached,
			{Slug: "absent", ServerForChat: serverFor(nil, nil)},
			{Slug: "broken", ServerForChat: serverFor(nil, xerrors.New("secret detail"))},
		},
	}

	servers, failures := server.internalMCPServersForChat(testutil.Context(t, testutil.WaitShort), chat)

	require.Equal(t, []uuid.UUID{chat.ID, chat.ID, chat.ID}, gotChatIDs)
	require.Equal(t, []mcpclient.Server{{
		ID:        attached.id(),
		Slug:      "attached",
		InProcess: mcpServer,
	}}, servers)
	require.Equal(t, []mcpclient.ConnectSummary{{
		ConfigID: InternalMCPServer{Slug: "broken"}.id(),
		Slug:     "broken",
		Outcome:  mcpclient.ConnectOutcomeError,
		Error:    "failed to attach internal MCP server",
	}}, failures)
	require.NotEqual(t, uuid.Nil, attached.id())
	require.NotEqual(t, attached.id(), failures[0].ConfigID)
}

func TestValidateInternalMCPServers(t *testing.T) {
	t.Parallel()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "valid"}, nil)
	serverForChat := func(context.Context, database.Chat) (*mcp.Server, error) { return mcpServer, nil }
	for _, tc := range []struct {
		name    string
		servers []InternalMCPServer
		wantErr string
	}{
		{
			name: "Valid",
			servers: []InternalMCPServer{
				{Slug: "a", ServerForChat: serverForChat},
				{Slug: "b", ServerForChat: serverForChat},
			},
		},
		{
			name:    "EmptySlug",
			servers: []InternalMCPServer{{ServerForChat: serverForChat}},
			wantErr: "slug is empty",
		},
		{
			name:    "NoServerForChat",
			servers: []InternalMCPServer{{Slug: "a"}},
			wantErr: `"a" has no ServerForChat`,
		},
		{
			name: "DuplicateSlug",
			servers: []InternalMCPServer{
				{Slug: "a", ServerForChat: serverForChat},
				{Slug: "a", ServerForChat: serverForChat},
			},
			wantErr: `duplicate internal MCP server slug "a"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateInternalMCPServers(tc.servers)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
