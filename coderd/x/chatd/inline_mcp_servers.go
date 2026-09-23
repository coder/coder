package chatd

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// callerSuppliedMCPServersEnabled reports whether this deployment loads
// and connects inline MCP servers that an API caller supplied. Both the
// experiment and the caller-supplied tools kill switch must allow it.
// Internal servers come from coderd code, so this gate does not apply
// to them.
func (server *Server) callerSuppliedMCPServersEnabled() bool {
	return !server.disableCallerSuppliedTools &&
		server.experiments.Enabled(codersdk.ExperimentChatInlineMCPServers)
}

// loadInlineMCPServers returns the inline MCP servers that apply to
// this turn. Child chats read the root chat's rows and keep only those
// allowed in subagents. A row with invalid stored headers is a connect
// outcome, so it is returned as a failed ConnectSummary row: the chat
// debug panel shows it beside connection failures.
func (server *Server) loadInlineMCPServers(ctx context.Context, chat database.Chat) ([]mcpclient.Server, []mcpclient.ConnectSummary, error) {
	rootChatID := chat.ID
	if chat.RootChatID.Valid {
		rootChatID = chat.RootChatID.UUID
	}
	rows, err := server.db.GetChatMCPServersByChatID(ctx, rootChatID)
	if err != nil {
		return nil, nil, xerrors.Errorf("get inline MCP servers: %w", err)
	}

	servers := make([]mcpclient.Server, 0, len(rows))
	var failures []mcpclient.ConnectSummary
	callerSuppliedEnabled := server.callerSuppliedMCPServersEnabled()
	for _, row := range rows {
		if !callerSuppliedEnabled && !mcpclient.IsInternalURL(row.Url) {
			continue
		}
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
		servers = append(servers, mcpclient.Server{
			ID:                  row.ID,
			Slug:                row.Slug,
			URL:                 row.Url,
			Transport:           mcpclient.TransportStreamableHTTP,
			Headers:             headers,
			ForwardCoderHeaders: row.ForwardCoderHeaders,
			ToolAllowList:       row.ToolAllowList,
			ToolDenyList:        row.ToolDenyList,
		})
	}
	return servers, failures, nil
}
