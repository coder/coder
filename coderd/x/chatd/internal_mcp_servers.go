package chatd

import (
	"context"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// internalMCPServerNamespace derives the stable ID of each internal MCP
// server from its slug.
var internalMCPServerNamespace = uuid.MustParse("cbb03019-96c4-4988-a4ae-0fa5343f2b93")

// InternalMCPServer is an MCP server that coderd code serves to chats in
// process. Nothing is stored on the chat: chatd calls ServerForChat at every
// step.
type InternalMCPServer struct {
	// Slug prefixes the server's tool names, as for inline servers.
	Slug string
	// ServerForChat returns the MCP server for chat, or nil when chat does
	// not get this server. chat can be a subagent. chatd calls it at every
	// step, so it must be fast. The server serves only chat, so it knows the
	// caller.
	ServerForChat func(ctx context.Context, chat database.Chat) (*mcp.Server, error)
}

// id is stable across steps and replicas and unique for each slug.
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

// internalMCPServersForChat returns the internal MCP servers that serve
// chat in this step. A server whose ServerForChat fails is skipped and
// returned as a failed ConnectSummary. The summary text is fixed so error
// detail never reaches viewer-visible data.
func (server *Server) internalMCPServersForChat(ctx context.Context, chat database.Chat) ([]mcpclient.Server, []mcpclient.ConnectSummary) {
	var (
		servers  []mcpclient.Server
		failures []mcpclient.ConnectSummary
	)
	for _, internal := range server.internalMCPServers {
		mcpServer, err := internal.ServerForChat(ctx, chat)
		if err != nil {
			server.logger.Warn(ctx, "failed to attach internal MCP server",
				slog.F("chat_id", chat.ID), slog.F("server_slug", internal.Slug), slog.Error(err))
			failures = append(failures, mcpclient.ConnectSummary{
				ConfigID: internal.id(),
				Slug:     internal.Slug,
				Outcome:  mcpclient.ConnectOutcomeError,
				Error:    "failed to attach internal MCP server",
			})
			continue
		}
		if mcpServer == nil {
			continue
		}
		servers = append(servers, mcpclient.Server{
			ID:        internal.id(),
			Slug:      internal.Slug,
			InProcess: mcpServer,
		})
	}
	return servers, failures
}
