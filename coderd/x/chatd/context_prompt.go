package chatd

import (
	"context"
	"encoding/json"
	"strings"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// contextBodyUnmarshalOptions reads the protojson resource bodies written by
// the agent context push (coderd/agentapi/context.go). DiscardUnknown keeps
// the reader forward compatible as new body fields are added to the proto.
var contextBodyUnmarshalOptions = protojson.UnmarshalOptions{DiscardUnknown: true}

// decodeContextBody decodes a protojson resource body into the proto type T.
// ok is false when the body cannot be decoded, letting callers count it as
// malformed rather than silently treating it as empty.
func decodeContextBody[T any, P interface {
	*T
	proto.Message
}](body json.RawMessage) (P, bool) {
	decoded := P(new(T))
	if err := contextBodyUnmarshalOptions.Unmarshal(body, decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

// pinnedSkillMeta converts a decoded skill body into prompt skill metadata.
// ok is false when the name is not a valid kebab-case skill name; the
// agent API does not validate skill names on ingest.
func pinnedSkillMeta(dir string, body *agentproto.SkillMetaBody) (meta chattool.SkillMeta, ok bool) {
	name := body.GetName()
	if !workspacesdk.SkillNamePattern.MatchString(name) {
		return chattool.SkillMeta{}, false
	}
	// Meta carries the verbatim SKILL.md so read_skill serves the pinned
	// body without dialing the workspace.
	return chattool.SkillMeta{
		Name:        name,
		Description: body.GetDescription(),
		PluginName:  body.GetPluginName(),
		Dir:         dir,
		Meta:        body.GetMeta(),
	}, true
}

// mcpToolNameSeparator joins a server name and a tool name into the
// flattened "<server>__<tool>" form. The agent reports MCP tool names
// unprefixed alongside the server name; the workspace agent's MCP proxy
// expects this flattened form to route a call back to the owning server
// (see agent/x/agentmcp ToolNameSep).
const mcpToolNameSeparator = "__"

// mcpToolsFromServerBody returns the tool list of a decoded mcp_server body
// for the chat response. Tool names are reported unprefixed; a leading
// "<server>__" is trimmed if present. Returns nil when the body has no tools.
func mcpToolsFromServerBody(server string, decoded *agentproto.MCPServerBody) []codersdk.ChatContextTool {
	tools := decoded.GetTools()
	if len(tools) == 0 {
		return nil
	}
	prefix := server + mcpToolNameSeparator
	out := make([]codersdk.ChatContextTool, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimPrefix(t.GetName(), prefix)
		if name == "" {
			continue
		}
		out = append(out, codersdk.ChatContextTool{
			Name:        name,
			Description: t.GetDescription(),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// workspaceMCPToolInfosFromResources decodes a chat's pinned mcp_server
// resources into execution-ready tool infos. Only OK mcp_server rows
// contribute. The agent reports tool names unprefixed alongside the server
// name, so each tool is re-prefixed to "<server>__<tool>", the model-facing
// and proxy-routable form the live discovery path also produces. The pushed
// input schema is a full JSON Schema object; its "properties" and "required"
// are split out to match the shape the workspace agent's live tool list
// produces (see agent/x/agentmcp). Tools with an empty name are skipped.
func workspaceMCPToolInfosFromResources(resources []database.ChatContextResource) []workspacesdk.MCPToolInfo {
	var out []workspacesdk.MCPToolInfo
	for _, r := range resources {
		if r.BodyKind != database.WorkspaceAgentContextBodyKindMcpServer ||
			r.Status != database.WorkspaceAgentContextResourceStatusOk {
			continue
		}
		decoded, ok := decodeContextBody[agentproto.MCPServerBody](r.Body)
		if !ok {
			continue
		}
		server := decoded.GetServerName()
		if server == "" {
			server = r.Source
		}
		for _, t := range decoded.GetTools() {
			name := t.GetName()
			if name == "" {
				continue
			}
			properties, required := splitMCPInputSchema(t.GetInputSchema())
			out = append(out, workspacesdk.MCPToolInfo{
				ServerName:  server,
				Name:        server + mcpToolNameSeparator + name,
				Description: t.GetDescription(),
				Schema:      properties,
				Required:    required,
			})
		}
	}
	return out
}

// splitMCPInputSchema splits a pushed JSON Schema object into the properties
// map and required list the workspace MCP tool wrapper expects. A nil schema,
// or one missing these keys, yields nil for the absent part.
func splitMCPInputSchema(schema *structpb.Struct) (properties map[string]any, required []string) {
	if schema == nil {
		return nil, nil
	}
	m := schema.AsMap()
	if props, ok := m["properties"].(map[string]any); ok {
		properties = props
	}
	if raw, ok := m["required"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				required = append(required, s)
			}
		}
	}
	return properties, required
}

// decodeInstructionContent decodes an instruction-file resource body and
// returns its sanitized content. decoded is false when the body cannot be
// decoded, letting the prompt path count it as malformed; content is empty
// when the file sanitizes to nothing, in which case callers skip it. Shared by
// the prompt builder and the API resource listing so both interpret an
// instruction file the same way.
func decodeInstructionContent(body json.RawMessage) (content string, decoded bool) {
	decodedBody, ok := decodeContextBody[agentproto.InstructionFileBody](body)
	if !ok {
		return "", false
	}
	return codersdk.SanitizePromptText(string(decodedBody.GetContent())), true
}

// pinnedWorkspaceContext builds the system-prompt instruction block and
// workspace skills from the chat's pinned context resources
// (chat_context_resources), populated at hydrate and refresh time. A chat
// with no pinned rows yields no context. A read error is returned rather than
// swallowed, matching the other prompt-input reads in prepareGeneration.
//
// agent only decorates the instruction header with its OS and directory; an
// unresolved (zero-value) agent does not blank the context, so the pin keeps
// working when the workspace is unreachable.
func (server *Server) pinnedWorkspaceContext(
	ctx context.Context,
	chat database.Chat,
	agent database.WorkspaceAgent,
) (instruction string, skills []chattool.SkillMeta, err error) {
	resources, err := server.db.ListChatContextResourcesByChatID(ctx, chat.ID)
	if err != nil {
		return "", nil, xerrors.Errorf("list chat context resources: %w", err)
	}
	if len(resources) == 0 {
		return "", nil, nil
	}

	directory := agent.ExpandedDirectory
	if directory == "" {
		directory = agent.Directory
	}
	instruction, skills, malformed, invalidNameSources := contextResourcesToPrompt(resources, agent.OperatingSystem, directory)
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
	if len(invalidNameSources) > 0 {
		server.logger.Warn(ctx, "skipped pinned skills with invalid names",
			slog.F("chat_id", chat.ID),
			slog.F("sources", invalidNameSources),
			slog.F("resource_count", len(resources)),
		)
	}
	server.logger.Debug(ctx, "built prompt context from pinned chat resources",
		slog.F("chat_id", chat.ID),
		slog.F("resource_count", len(resources)),
		slog.F("skill_count", len(skills)),
		slog.F("has_instruction", instruction != ""),
	)
	return instruction, skills, nil
}

// resolveTurnWorkspaceContext selects the instruction block and workspace
// skills for a turn from the chat's pinned context snapshot
// (chat_context_resources). agent is the chat's resolved workspace agent,
// used only to decorate the pinned instruction header. A non-workspace chat
// yields no context.
func (server *Server) resolveTurnWorkspaceContext(
	ctx context.Context,
	chat database.Chat,
	agent database.WorkspaceAgent,
) (instruction string, skills []chattool.SkillMeta, err error) {
	if !chat.WorkspaceID.Valid {
		return "", nil, nil
	}
	return server.pinnedWorkspaceContext(ctx, chat, agent)
}

// contextResourcesToPrompt converts a chat's pinned context resources into
// the formatted instruction block and workspace skill metadata, the inverse
// of the protojson bodies written by the agent context push.
//
// operatingSystem and directory annotate the instruction header and are
// omitted when empty. Only OK resources of a prompt body kind contribute;
// other statuses, body kinds, and malformed bodies are skipped. malformed
// counts OK resources whose body failed to decode. invalidNameSources lists the
// sources of OK skills dropped for an invalid skill name. The header is emitted
// only when at least one instruction file has content, so a skill-only pin
// produces no instruction block, matching the per-turn path.
func contextResourcesToPrompt(
	resources []database.ChatContextResource,
	operatingSystem, directory string,
) (instruction string, skills []chattool.SkillMeta, malformed int, invalidNameSources []string) {
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
			decodedBody, ok := decodeContextBody[agentproto.SkillMetaBody](r.Body)
			if !ok {
				malformed++
				continue
			}
			meta, ok := pinnedSkillMeta(r.Source, decodedBody)
			if !ok {
				invalidNameSources = append(invalidNameSources, r.Source)
				continue
			}
			skills = append(skills, meta)
		}
	}

	if len(contextFileParts) == 0 {
		return "", skills, malformed, invalidNameSources
	}
	return formatSystemInstructions(operatingSystem, directory, contextFileParts), skills, malformed, invalidNameSources
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
//     a valid name, OK MCP configs/servers (mcp_server carries its tools and
//     plugin name), and OK plugin manifests.
//   - Non-OK rows (invalid, unreadable, oversize, excluded) of a tracked kind,
//     carrying Status and Error so the UI can explain why the resource was
//     dropped from the prompt instead of silently omitting it. Their
//     body-specific fields are empty, except PluginName on skill and
//     mcp_server rows.
//
// OK-but-empty instruction files, OK skills with an invalid skill name, and
// untracked kinds (reserved hook/subagent/command) are skipped.
// Error is copied for every listed row, so an OK row keeps the non-fatal
// warnings the agent recorded for it. Input order (source ASC from the
// query) is preserved.
func pinnedContextResources(resources []database.ChatContextResource) []codersdk.ChatContextResource {
	var out []codersdk.ChatContextResource
	for _, r := range resources {
		kind, ok := contextResourceKind(r.BodyKind)
		if !ok {
			continue
		}
		if r.Status != database.WorkspaceAgentContextResourceStatusOk {
			// Surface the failure (with its reason) rather than dropping it
			// silently. The agent sets the body on every status, so plugin
			// attribution is still available for skill and mcp_server rows.
			out = append(out, codersdk.ChatContextResource{
				Source:     r.Source,
				Kind:       kind,
				SizeBytes:  r.SizeBytes,
				Status:     codersdk.ChatContextResourceStatus(r.Status),
				Error:      r.Error,
				PluginName: attributedPluginName(r),
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
				Error:     r.Error,
			})
		case database.WorkspaceAgentContextBodyKindSkill:
			decodedBody, decoded := decodeContextBody[agentproto.SkillMetaBody](r.Body)
			if !decoded {
				continue
			}
			meta, ok := pinnedSkillMeta(r.Source, decodedBody)
			if !ok {
				continue
			}
			out = append(out, codersdk.ChatContextResource{
				Source:           r.Source,
				Kind:             kind,
				SizeBytes:        r.SizeBytes,
				Status:           codersdk.ChatContextResourceStatusOK,
				Error:            r.Error,
				SkillName:        meta.Name,
				SkillDescription: meta.Description,
				PluginName:       meta.PluginName,
			})
		case database.WorkspaceAgentContextBodyKindMcpConfig:
			out = append(out, codersdk.ChatContextResource{
				Source:    r.Source,
				Kind:      kind,
				SizeBytes: r.SizeBytes,
				Status:    codersdk.ChatContextResourceStatusOK,
				Error:     r.Error,
			})
		case database.WorkspaceAgentContextBodyKindMcpServer:
			resource := codersdk.ChatContextResource{
				Source:    r.Source,
				Kind:      kind,
				SizeBytes: r.SizeBytes,
				Status:    codersdk.ChatContextResourceStatusOK,
				Error:     r.Error,
			}
			if decodedBody, decoded := decodeContextBody[agentproto.MCPServerBody](r.Body); decoded {
				resource.Tools = mcpToolsFromServerBody(r.Source, decodedBody)
				resource.PluginName = decodedBody.GetPluginName()
			}
			out = append(out, resource)
		case database.WorkspaceAgentContextBodyKindPlugin:
			decodedBody, decoded := decodeContextBody[agentproto.PluginBody](r.Body)
			if !decoded {
				continue
			}
			out = append(out, codersdk.ChatContextResource{
				Source:     r.Source,
				Kind:       kind,
				SizeBytes:  r.SizeBytes,
				Status:     codersdk.ChatContextResourceStatusOK,
				Error:      r.Error,
				PluginName: decodedBody.GetName(),
			})
		}
	}
	return out
}

// contextResourceKind maps a database body kind to the codersdk kind reported
// on the chat. ok is false only for kinds chatd does not track yet (the
// reserved hook/subagent/command kinds), which are omitted from the resource
// list.
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
	case database.WorkspaceAgentContextBodyKindPlugin:
		return codersdk.ChatContextResourceKindPlugin, true
	default:
		return "", false
	}
}

// attributedPluginName runs on non-OK rows, so it skips plugin manifest
// rows: the agent names a manifest only when it is OK.
func attributedPluginName(r database.ChatContextResource) string {
	switch r.BodyKind {
	case database.WorkspaceAgentContextBodyKindSkill:
		decoded, ok := decodeContextBody[agentproto.SkillMetaBody](r.Body)
		if !ok {
			return ""
		}
		return decoded.GetPluginName()
	case database.WorkspaceAgentContextBodyKindMcpServer:
		decoded, ok := decodeContextBody[agentproto.MCPServerBody](r.Body)
		if !ok {
			return ""
		}
		return decoded.GetPluginName()
	default:
		return ""
	}
}
