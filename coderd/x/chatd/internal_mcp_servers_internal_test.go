package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

func TestInternalMCPServersForChat(t *testing.T) {
	t.Parallel()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "attached"}, nil)
	serverFor := func(s *mcp.Server) func(context.Context, database.Chat) (*mcp.Server, error) {
		return func(context.Context, database.Chat) (*mcp.Server, error) { return s, nil }
	}
	attached := InternalMCPServer{Slug: "attached", ServerForChat: serverFor(mcpServer)}
	absent := InternalMCPServer{Slug: "absent", ServerForChat: serverFor(nil)}
	server := &Server{internalMCPServers: []InternalMCPServer{attached, absent}}
	ctx := testutil.Context(t, testutil.WaitShort)

	servers, err := server.internalMCPServersForChat(ctx, database.Chat{ID: uuid.New()})
	require.NoError(t, err)
	require.Equal(t, []mcpclient.InProcessServer{{
		ID:     attached.id(),
		Slug:   "attached",
		Server: mcpServer,
	}}, servers)
	require.NotEqual(t, attached.id(), absent.id())
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
