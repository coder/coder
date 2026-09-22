package chatstate

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ReplaceInlineMCPServers makes the chat's inline MCP servers equal
// to servers. Rows with a matching slug keep their id. An empty slice
// removes every server. store must be the transaction handle inside InTx.
func ReplaceInlineMCPServers(ctx context.Context, store database.Store, chatID uuid.UUID, servers []codersdk.InlineMCPServerRequest) error {
	// A nil slice binds as SQL NULL and the delete would remove nothing.
	slugs := make([]string, 0, len(servers))
	for _, server := range servers {
		headers, err := marshalChatMCPHeaders(server.Headers)
		if err != nil {
			return xerrors.Errorf("marshal headers for %q: %w", server.Slug, err)
		}
		if _, err := store.UpsertChatMCPServer(ctx, database.UpsertChatMCPServerParams{
			ID:                  uuid.New(),
			ChatID:              chatID,
			Slug:                server.Slug,
			Url:                 server.URL,
			Headers:             headers,
			HeadersKeyID:        sql.NullString{},
			ToolAllowList:       nonNilStrings(server.ToolAllowList),
			ToolDenyList:        nonNilStrings(server.ToolDenyList),
			AllowInPlanMode:     server.AllowInPlanMode,
			AllowInSubagents:    server.AllowInSubagents,
			ForwardCoderHeaders: server.ForwardCoderHeaders,
		}); err != nil {
			return xerrors.Errorf("upsert chat MCP server %q: %w", server.Slug, err)
		}
		slugs = append(slugs, server.Slug)
	}
	if err := store.DeleteChatMCPServersByChatIDExcludingSlugs(ctx, database.DeleteChatMCPServersByChatIDExcludingSlugsParams{
		ChatID: chatID,
		Slugs:  slugs,
	}); err != nil {
		return xerrors.Errorf("delete removed chat MCP servers: %w", err)
	}
	return nil
}

func marshalChatMCPHeaders(headers map[string]string) (string, error) {
	if len(headers) == 0 {
		return "{}", nil
	}
	raw, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
