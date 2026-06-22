package chatd

import (
	"context"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ContextResources returns the chat's pinned context resource list (metadata
// only). It is read-only and intended for the single-chat GET handler; list
// and watch payloads omit this detail to stay lightweight.
//
// The returned list is the chat's full pinned inventory (instruction files,
// skills, and MCP configs/servers), each stamped with its per-resource status
// so the UI can explain why a resource was dropped from the prompt instead of
// silently omitting it.
func (server *Server) ContextResources(
	ctx context.Context,
	chat database.Chat,
) ([]codersdk.ChatContextResource, error) {
	pinned, err := server.db.ListChatContextResourcesByChatID(ctx, chat.ID)
	if err != nil {
		return nil, xerrors.Errorf("list chat context resources: %w", err)
	}
	resources := pinnedContextResources(pinned)
	server.logger.Debug(ctx, "computed chat context resources",
		slog.F("chat_id", chat.ID),
		slog.F("resource_count", len(resources)),
	)
	return resources, nil
}

// pinnedContextResources converts a chat's pinned context rows into the
// metadata-only resource list reported on the chat. It surfaces the full
// pinned inventory the user can act on, each stamped with its Status:
//
//   - OK instruction files with non-empty (sanitized) content, OK skills with
//     a name, and OK MCP configs/servers (mcp_server carries its tools).
//   - Non-OK rows (invalid, unreadable, oversize, excluded) of a tracked kind,
//     carrying Status and Error so the UI can explain why the resource was
//     dropped from the prompt instead of silently omitting it. Their
//     body-specific fields are empty.
//
// OK-but-empty instruction files, OK skills with no name, and untracked kinds
// (reserved plugin/hook/subagent/command) are skipped. Input order (source ASC
// from the query) is preserved.
func pinnedContextResources(resources []database.ChatContextResource) []codersdk.ChatContextResource {
	var out []codersdk.ChatContextResource
	for _, r := range resources {
		kind, ok := contextResourceKind(r.BodyKind)
		if !ok {
			continue
		}
		if r.Status != database.WorkspaceAgentContextResourceStatusOk {
			// Surface the failure (with its reason) rather than dropping it
			// silently; the body is empty for non-OK rows.
			out = append(out, codersdk.ChatContextResource{
				Source:    r.Source,
				Kind:      kind,
				SizeBytes: r.SizeBytes,
				Status:    codersdk.ChatContextResourceStatus(r.Status),
				Error:     r.Error,
			})
			continue
		}
		switch r.BodyKind {
		case database.WorkspaceAgentContextBodyKindInstructionFile:
			body, decoded := decodeInstructionFileBody(r.Body)
			if !decoded || SanitizePromptText(string(body.GetContent())) == "" {
				continue
			}
			out = append(out, codersdk.ChatContextResource{
				Source:    r.Source,
				Kind:      kind,
				SizeBytes: r.SizeBytes,
				Status:    codersdk.ChatContextResourceStatusOK,
			})
		case database.WorkspaceAgentContextBodyKindSkill:
			body, decoded := decodeSkillMetaBody(r.Body)
			if !decoded || body.GetName() == "" {
				continue
			}
			out = append(out, codersdk.ChatContextResource{
				Source:           r.Source,
				Kind:             kind,
				SizeBytes:        r.SizeBytes,
				Status:           codersdk.ChatContextResourceStatusOK,
				SkillName:        body.GetName(),
				SkillDescription: body.GetDescription(),
			})
		case database.WorkspaceAgentContextBodyKindMcpConfig:
			out = append(out, codersdk.ChatContextResource{
				Source:    r.Source,
				Kind:      kind,
				SizeBytes: r.SizeBytes,
				Status:    codersdk.ChatContextResourceStatusOK,
			})
		case database.WorkspaceAgentContextBodyKindMcpServer:
			out = append(out, codersdk.ChatContextResource{
				Source:    r.Source,
				Kind:      kind,
				SizeBytes: r.SizeBytes,
				Status:    codersdk.ChatContextResourceStatusOK,
				McpTools:  mcpToolsFromServerBody(r.Source, r.Body),
			})
		}
	}
	return out
}

// contextResourceKind maps a database body kind to the codersdk kind reported
// on the chat. ok is false only for kinds chatd does not track yet (the
// reserved plugin/hook/subagent/command kinds), which are omitted from the
// resource list.
func contextResourceKind(kind database.WorkspaceAgentContextBodyKind) (codersdk.ChatContextResourceKind, bool) {
	switch kind {
	case database.WorkspaceAgentContextBodyKindInstructionFile:
		return codersdk.ChatContextResourceKindInstructionFile, true
	case database.WorkspaceAgentContextBodyKindSkill:
		return codersdk.ChatContextResourceKindSkill, true
	case database.WorkspaceAgentContextBodyKindMcpConfig:
		return codersdk.ChatContextResourceKindMCPConfig, true
	case database.WorkspaceAgentContextBodyKindMcpServer:
		return codersdk.ChatContextResourceKindMCPServer, true
	default:
		return "", false
	}
}
