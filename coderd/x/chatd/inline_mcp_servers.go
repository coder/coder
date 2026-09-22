package chatd

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// inlineMCPServer is one inline server as prepareGeneration sees
// it: the connect spec plus the plan-mode policy chatd enforces.
type inlineMCPServer struct {
	Server          mcpclient.Server
	AllowInPlanMode bool
}

func inlineMCPServerID(srv inlineMCPServer) uuid.UUID {
	return srv.Server.ID
}

// inlineMCPServersEnabled reports whether this deployment loads and
// connects inline MCP servers at turn time. Both the experiment
// and the caller-supplied tools kill switch must allow it.
func (server *Server) inlineMCPServersEnabled() bool {
	return !server.disableCallerSuppliedTools &&
		server.experiments.Enabled(codersdk.ExperimentChatInlineMCPServers)
}

// loadInlineMCPServers returns the inline MCP servers that apply to
// this turn. Child chats read the root chat's rows and keep only those
// allowed in subagents. Anything that stops a declared server from reaching
// the model is a connect outcome, so load failures are returned as failed
// ConnectSummary rows instead of an error: the chat debug panel shows them
// beside connection failures. The summary text is fixed so database error
// detail never reaches viewer-visible data.
func (server *Server) loadInlineMCPServers(ctx context.Context, chat database.Chat) ([]inlineMCPServer, []mcpclient.ConnectSummary) {
	if isExploreSubagentMode(chat.Mode) {
		return nil, nil
	}
	rootChatID := chat.ID
	if chat.RootChatID.Valid {
		rootChatID = chat.RootChatID.UUID
	}
	rows, err := server.db.GetChatMCPServersByChatID(ctx, rootChatID)
	if err != nil {
		server.logger.Warn(ctx, "failed to load inline MCP servers",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return nil, []mcpclient.ConnectSummary{{
			Slug:    "inline-mcp-servers",
			Outcome: mcpclient.ConnectOutcomeError,
			Error:   "failed to load inline MCP servers",
		}}
	}

	servers := make([]inlineMCPServer, 0, len(rows))
	var failures []mcpclient.ConnectSummary
	for _, row := range rows {
		if chat.ParentChatID.Valid && !row.AllowInSubagents {
			continue
		}
		var headers map[string]string
		if err := json.Unmarshal([]byte(row.Headers), &headers); err != nil {
			server.logger.Warn(ctx, "skipping inline MCP server with invalid stored headers",
				slog.F("chat_id", chat.ID), slog.F("server_slug", row.Slug), slog.Error(err))
			failures = append(failures, mcpclient.ConnectSummary{
				ConfigID: row.ID,
				Slug:     row.Slug,
				Outcome:  mcpclient.ConnectOutcomeError,
				Error:    "invalid stored headers",
			})
			continue
		}
		servers = append(servers, inlineMCPServer{
			Server: mcpclient.Server{
				ID:                  row.ID,
				Slug:                row.Slug,
				URL:                 row.Url,
				Transport:           mcpclient.TransportStreamableHTTP,
				Headers:             headers,
				ForwardCoderHeaders: row.ForwardCoderHeaders,
				ToolAllowList:       row.ToolAllowList,
				ToolDenyList:        row.ToolDenyList,
			},
			AllowInPlanMode: row.AllowInPlanMode,
		})
	}
	return servers, failures
}
