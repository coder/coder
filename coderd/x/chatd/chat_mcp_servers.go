package chatd

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// loadChatMCPServers returns the chat-attached MCP servers that apply to
// this turn plus the sensitive values to redact per server id. Child chats
// read the root chat's rows and keep only those allowed in subagents.
func (server *Server) loadChatMCPServers(ctx context.Context, chat database.Chat) ([]database.MCPServerConfig, map[uuid.UUID][]string, error) {
	if isExploreSubagentMode(chat.Mode) {
		return nil, nil, nil
	}
	rootChatID := chat.ID
	if chat.RootChatID.Valid {
		rootChatID = chat.RootChatID.UUID
	}
	rows, err := server.db.GetChatMCPServersByChatID(ctx, rootChatID)
	if err != nil {
		return nil, nil, xerrors.Errorf("get chat MCP servers: %w", err)
	}

	configs := make([]database.MCPServerConfig, 0, len(rows))
	sensitiveValues := make(map[uuid.UUID][]string, len(rows))
	for _, row := range rows {
		if chat.ParentChatID.Valid && !row.AllowInSubagents {
			continue
		}
		var headers map[string]string
		if err := json.Unmarshal([]byte(row.Headers), &headers); err != nil {
			return nil, nil, xerrors.Errorf("parse headers for chat MCP server %q: %w", row.Slug, err)
		}
		authType := "none"
		if len(headers) > 0 {
			authType = "custom_headers"
		}
		configs = append(configs, database.MCPServerConfig{
			ID:                  row.ID,
			DisplayName:         row.Slug,
			Slug:                row.Slug,
			Url:                 row.Url,
			Transport:           "streamable_http",
			AuthType:            authType,
			CustomHeaders:       row.Headers,
			ToolAllowList:       row.ToolAllowList,
			ToolDenyList:        row.ToolDenyList,
			AllowInPlanMode:     row.AllowInPlanMode,
			ForwardCoderHeaders: row.ForwardCoderHeaders,
			Enabled:             true,
			OrganizationID:      chat.OrganizationID,
		})

		values := []string{row.Url}
		for _, name := range slices.Sorted(maps.Keys(headers)) {
			values = append(values, name, headers[name])
		}
		sensitiveValues[row.ID] = values
	}
	return configs, sensitiveValues, nil
}
