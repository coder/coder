package chatd

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

var internalMCPServerNamespace = uuid.MustParse("cbb03019-96c4-4988-a4ae-0fa5343f2b93")

// InternalMCPServer supplies a coderd-owned MCP server without storing
// its configuration on the chat.
type InternalMCPServer struct {
	// Slug prefixes the server's tool names, as for inline servers.
	Slug string
	// ServerForChat must return a server scoped to chat, or nil to exclude it.
	// chat can be a subagent. This callback must be fast because chatd calls
	// it at each step that permits MCP servers.
	ServerForChat func(ctx context.Context, chat database.Chat) (*mcp.Server, error)
}

func (s InternalMCPServer) id() uuid.UUID {
	return uuid.NewSHA1(internalMCPServerNamespace, []byte(s.Slug))
}

func validateInternalMCPServers(servers []InternalMCPServer) error {
	slugs := make(map[string]struct{}, len(servers))
	for _, s := range servers {
		if s.Slug == "" {
			return xerrors.New("internal MCP server slug is empty")
		}
		if s.ServerForChat == nil {
			return xerrors.Errorf("internal MCP server %q has no ServerForChat", s.Slug)
		}
		if _, ok := slugs[s.Slug]; ok {
			return xerrors.Errorf("duplicate internal MCP server slug %q", s.Slug)
		}
		slugs[s.Slug] = struct{}{}
	}
	return nil
}

// A fixed classification keeps callback error details out of the
// user-facing chat error.
func (server *Server) internalMCPServersForChat(ctx context.Context, chat database.Chat) ([]mcpclient.InProcessServer, error) {
	var servers []mcpclient.InProcessServer
	for _, internal := range server.internalMCPServers {
		mcpServer, err := internal.ServerForChat(ctx, chat)
		if err != nil {
			return nil, chaterror.WithClassification(
				xerrors.Errorf("get internal MCP server %q: %w", internal.Slug, err),
				chaterror.ClassifiedError{
					Message:   "An internal MCP server failed.",
					Kind:      codersdk.ChatErrorKindGeneric,
					Retryable: false,
				},
			)
		}
		if mcpServer == nil {
			continue
		}
		servers = append(servers, mcpclient.InProcessServer{
			ID:     internal.id(),
			Slug:   internal.Slug,
			Server: mcpServer,
		})
	}
	return servers, nil
}
