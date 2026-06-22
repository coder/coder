package chatd

import (
	"context"
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// contextBodyUnmarshalOptions reads the protojson resource bodies written by
// the agent context push (coderd/agentapi/context.go). DiscardUnknown keeps
// the reader forward compatible as new body fields are added to the proto.
var contextBodyUnmarshalOptions = protojson.UnmarshalOptions{DiscardUnknown: true}

// decodeInstructionFileBody decodes a protojson instruction-file resource
// body. ok is false when the body cannot be decoded, letting callers count it
// as malformed rather than silently treating it as empty.
func decodeInstructionFileBody(body json.RawMessage) (*agentproto.InstructionFileBody, bool) {
	var decoded agentproto.InstructionFileBody
	if err := contextBodyUnmarshalOptions.Unmarshal(body, &decoded); err != nil {
		return nil, false
	}
	return &decoded, true
}

// decodeSkillMetaBody decodes a protojson skill resource body. ok is false
// when the body cannot be decoded.
func decodeSkillMetaBody(body json.RawMessage) (*agentproto.SkillMetaBody, bool) {
	var decoded agentproto.SkillMetaBody
	if err := contextBodyUnmarshalOptions.Unmarshal(body, &decoded); err != nil {
		return nil, false
	}
	return &decoded, true
}

// mcpToolsFromServerBody decodes a stored mcp_server resource body and returns
// its tool list for the chat response. The agent prefixes each tool name with
// "<server>__"; that prefix is stripped so the name reads as the server
// exposes it. Returns nil when the body has no tools or cannot be decoded.
func mcpToolsFromServerBody(server string, body json.RawMessage) []codersdk.ChatContextMCPTool {
	var decoded agentproto.MCPServerBody
	if err := contextBodyUnmarshalOptions.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	tools := decoded.GetTools()
	if len(tools) == 0 {
		return nil
	}
	prefix := server + "__"
	out := make([]codersdk.ChatContextMCPTool, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimPrefix(t.GetName(), prefix)
		if name == "" {
			continue
		}
		out = append(out, codersdk.ChatContextMCPTool{
			Name:        name,
			Description: t.GetDescription(),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// decodeInstructionContent decodes an instruction-file resource body and
// returns its sanitized content. decoded is false when the body cannot be
// decoded, letting the prompt path count it as malformed; content is empty
// when the file sanitizes to nothing, in which case callers skip it. Shared by
// the prompt builder and the API resource listing so both interpret an
// instruction file the same way.
func decodeInstructionContent(body json.RawMessage) (content string, decoded bool) {
	decodedBody, ok := decodeInstructionFileBody(body)
	if !ok {
		return "", false
	}
	return SanitizePromptText(string(decodedBody.GetContent())), true
}

// decodeSkillIdentity decodes a skill resource body and returns its name and
// description. decoded is false when the body cannot be decoded, letting the
// prompt path count it as malformed; callers skip a skill with an empty name.
// Shared by the prompt builder and the API resource listing.
func decodeSkillIdentity(body json.RawMessage) (name, description string, decoded bool) {
	decodedBody, ok := decodeSkillMetaBody(body)
	if !ok {
		return "", "", false
	}
	return decodedBody.GetName(), decodedBody.GetDescription(), true
}

// pinnedWorkspaceContext builds the system-prompt instruction block and
// workspace skills from the chat's pinned context resources
// (chat_context_resources), populated at hydrate and refresh time.
//
// ok reports whether the caller should use these values instead of the
// per-turn, history-derived path. It is false when the chat has no pinned
// rows (an older agent that never reported context, or a chat not yet
// hydrated), so the caller falls back to the legacy path. When rows exist ok
// is true even if they all filter to empty content, because the pin is then
// the source of truth. A read error is returned rather than swallowed,
// matching the other prompt-input reads in prepareGeneration.
//
// agent only decorates the instruction header with its OS and directory; an
// unresolved (zero-value) agent does not force a fallback, so the pin keeps
// working when the workspace is unreachable.
func (server *Server) pinnedWorkspaceContext(
	ctx context.Context,
	chat database.Chat,
	agent database.WorkspaceAgent,
) (instruction string, skills []chattool.SkillMeta, ok bool, err error) {
	resources, err := server.db.ListChatContextResourcesByChatID(ctx, chat.ID)
	if err != nil {
		return "", nil, false, xerrors.Errorf("list chat context resources: %w", err)
	}
	if len(resources) == 0 {
		return "", nil, false, nil
	}

	directory := agent.ExpandedDirectory
	if directory == "" {
		directory = agent.Directory
	}
	instruction, skills, malformed := contextResourcesToPrompt(resources, agent.OperatingSystem, directory)
	if malformed > 0 {
		// A status-OK resource whose body cannot be decoded means the pin
		// hydrated content that is now unreadable; surface it so a proto
		// or encoding regression does not silently drop context.
		server.logger.Warn(ctx, "skipped malformed pinned chat context resources",
			slog.F("chat_id", chat.ID),
			slog.F("malformed_count", malformed),
			slog.F("resource_count", len(resources)),
		)
	}
	server.logger.Debug(ctx, "built prompt context from pinned chat resources",
		slog.F("chat_id", chat.ID),
		slog.F("resource_count", len(resources)),
		slog.F("skill_count", len(skills)),
		slog.F("has_instruction", instruction != ""),
	)
	return instruction, skills, true, nil
}

// resolveTurnWorkspaceContext selects the instruction block and workspace
// skills for a turn. It prefers the chat's pinned context copy when the
// workspace agent has reported context, and falls back to the per-turn,
// history-derived context-file and skill parts for older agents that have
// not. The two paths are mutually exclusive. agent is the chat's resolved
// workspace agent, used only to decorate the pinned instruction header. A
// non-workspace chat yields no context.
func (server *Server) resolveTurnWorkspaceContext(
	ctx context.Context,
	chat database.Chat,
	agent database.WorkspaceAgent,
	promptRows []database.ChatMessage,
) (instruction string, skills []chattool.SkillMeta, err error) {
	if !chat.WorkspaceID.Valid {
		return "", nil, nil
	}

	pinnedInstruction, pinnedSkills, ok, err := server.pinnedWorkspaceContext(ctx, chat, agent)
	if err != nil {
		return "", nil, err
	}
	if ok {
		return pinnedInstruction, pinnedSkills, nil
	}

	// History fallback: re-derive the instruction and skills from the
	// context-file and skill parts the per-turn pull persisted. Skills are
	// included only when context files are present; the pinned path resolves
	// them independently.
	if _, found := contextFileAgentID(promptRows); found {
		return instructionFromContextFiles(promptRows), skillsFromParts(promptRows), nil
	}
	return "", nil, nil
}

// contextResourcesToPrompt converts a chat's pinned context resources into
// the formatted instruction block and workspace skill metadata, the inverse
// of the protojson bodies written by the agent context push.
//
// operatingSystem and directory annotate the instruction header and are
// omitted when empty. Only OK resources of a prompt body kind contribute;
// other statuses, body kinds, and malformed bodies are skipped. malformed
// counts OK resources whose body failed to decode, so the caller can surface
// an otherwise silent drop. The header is emitted only when at least one
// instruction file has content, so a skill-only pin produces no instruction
// block, matching the per-turn path.
func contextResourcesToPrompt(
	resources []database.ChatContextResource,
	operatingSystem, directory string,
) (instruction string, skills []chattool.SkillMeta, malformed int) {
	var contextFileParts []codersdk.ChatMessagePart
	for _, r := range resources {
		if r.Status != database.WorkspaceAgentContextResourceStatusOk {
			continue
		}
		switch r.BodyKind {
		case database.WorkspaceAgentContextBodyKindInstructionFile:
			content, decoded := decodeInstructionContent(r.Body)
			if !decoded {
				malformed++
				continue
			}
			if content == "" {
				continue
			}
			contextFileParts = append(contextFileParts, codersdk.ChatMessagePart{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    r.Source,
				ContextFileContent: content,
			})
		case database.WorkspaceAgentContextBodyKindSkill:
			name, description, decoded := decodeSkillIdentity(r.Body)
			if !decoded {
				malformed++
				continue
			}
			if name == "" {
				continue
			}
			// source is the skill directory. MetaFile is left empty so
			// chattool falls back to DefaultSkillMetaFile ("SKILL.md").
			// SkillMetaBody carries no meta file name, so a non-default
			// CODER_AGENT_EXP_SKILL_META_FILE is not preserved on this
			// path, unlike the per-turn discovery path.
			skills = append(skills, chattool.SkillMeta{
				Name:        name,
				Description: description,
				Dir:         r.Source,
			})
		}
	}

	if len(contextFileParts) == 0 {
		return "", skills, malformed
	}
	return formatSystemInstructions(operatingSystem, directory, contextFileParts), skills, malformed
}

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
// metadata-only resource list reported on the chat. It is the reporting
// counterpart to contextResourcesToPrompt: both walk the same rows and share
// the body decoders, but where the prompt builder keeps only OK instruction
// files and skills (and ignores everything else), this surfaces the full
// inventory the user can act on, each stamped with its Status:
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
			content, decoded := decodeInstructionContent(r.Body)
			if !decoded || content == "" {
				continue
			}
			out = append(out, codersdk.ChatContextResource{
				Source:    r.Source,
				Kind:      kind,
				SizeBytes: r.SizeBytes,
				Status:    codersdk.ChatContextResourceStatusOK,
			})
		case database.WorkspaceAgentContextBodyKindSkill:
			name, description, decoded := decodeSkillIdentity(r.Body)
			if !decoded || name == "" {
				continue
			}
			out = append(out, codersdk.ChatContextResource{
				Source:           r.Source,
				Kind:             kind,
				SizeBytes:        r.SizeBytes,
				Status:           codersdk.ChatContextResourceStatusOK,
				SkillName:        name,
				SkillDescription: description,
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
