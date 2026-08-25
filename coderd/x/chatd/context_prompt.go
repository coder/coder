package chatd

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

// mcpToolNameSeparator joins a server name and a tool name into the
// flattened "<server>__<tool>" form. The agent reports MCP tool names
// unprefixed alongside the server name; the workspace agent's MCP proxy
// expects this flattened form to route a call back to the owning server
// (see agent/x/agentmcp ToolNameSep).
const mcpToolNameSeparator = "__"

// decodeMCPServerBody decodes an mcp_server resource body and returns the
// server name it advertises together with its tool list. source is the row's
// locator and is used as the server name when the body omits one. ok is false
// when the body cannot be decoded. Shared by the tool-execution path and the
// API resource listing so both read a pushed server the same way.
func decodeMCPServerBody(source string, body json.RawMessage) (server string, tools []*agentproto.MCPTool, ok bool) {
	var decoded agentproto.MCPServerBody
	if err := contextBodyUnmarshalOptions.Unmarshal(body, &decoded); err != nil {
		return "", nil, false
	}
	server = decoded.GetServerName()
	if server == "" {
		server = source
	}
	return server, decoded.GetTools(), true
}

// mcpToolsFromServerBody decodes an mcp_server resource body and returns its
// tool list for the chat response. The agent prefixes each tool name with
// "<server>__"; that prefix is stripped so the name reads as the server
// exposes it. Returns nil when the body has no tools or cannot be decoded.
func mcpToolsFromServerBody(source string, body json.RawMessage) []codersdk.ChatContextTool {
	server, tools, ok := decodeMCPServerBody(source, body)
	if !ok || len(tools) == 0 {
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

// workspaceMCPToolInfosFromResources decodes an agent's live mcp_server
// resources (workspace_agent_context_resources) into execution-ready tool
// infos. MCP servers are runtime capabilities of the bound agent rather than
// pinned prompt content, so the catalog is read from the agent's latest push
// and a workspace advertising no servers contributes no tools.
//
// Only OK mcp_server rows contribute. The agent reports tool names unprefixed
// alongside the server name, so each tool is re-prefixed to
// "<server>__<tool>", the model-facing and proxy-routable form. The pushed
// input schema is a full JSON Schema object; its "properties" and "required"
// are split out to match the shape the workspace agent's live tool list
// produces (see agent/x/agentmcp). Tools with an empty name are skipped.
func workspaceMCPToolInfosFromResources(resources []database.WorkspaceAgentContextResource) []workspacesdk.MCPToolInfo {
	var out []workspacesdk.MCPToolInfo
	for _, r := range resources {
		if r.BodyKind != database.WorkspaceAgentContextBodyKindMcpServer ||
			r.Status != database.WorkspaceAgentContextResourceStatusOk {
			continue
		}
		server, tools, ok := decodeMCPServerBody(r.Source, r.Body)
		if !ok {
			continue
		}
		for _, t := range tools {
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
	decodedBody, ok := decodeInstructionFileBody(body)
	if !ok {
		return "", false
	}
	return codersdk.SanitizePromptText(string(decodedBody.GetContent())), true
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
			decodedBody, ok := decodeSkillMetaBody(r.Body)
			if !ok {
				malformed++
				continue
			}
			if decodedBody.GetName() == "" {
				continue
			}
			// source is the skill directory. MetaFile is left empty so
			// chattool falls back to DefaultSkillMetaFile ("SKILL.md").
			// SkillMetaBody carries no meta file name, so a non-default
			// CODER_AGENT_EXP_SKILL_META_FILE is not preserved on this
			// path, unlike the per-turn discovery path. Meta carries the
			// verbatim SKILL.md so read_skill serves the body from the
			// pin instead of dialing the workspace.
			skills = append(skills, chattool.SkillMeta{
				Name:        decodedBody.GetName(),
				Description: decodedBody.GetDescription(),
				Dir:         r.Source,
				Meta:        decodedBody.GetMeta(),
			})
		}
	}

	if len(contextFileParts) == 0 {
		return "", skills, malformed
	}
	return formatSystemInstructions(operatingSystem, directory, contextFileParts), skills, malformed
}

// ContextResources returns the chat's context resource inventory (metadata
// only). It is read-only and intended for the single-chat GET handler; list
// and watch payloads omit this detail to stay lightweight.
//
// The list has two lifetimes. Instruction files and skills are prompt content
// and come from the chat's pin (chat_context_resources), so they match what
// the next turn will send even after the workspace drifts. MCP configs and
// servers are runtime capabilities that only work against a reachable agent,
// so they are read live from the chat's bound agent
// (workspace_agent_context_resources): a chat with no bound agent reports
// none, and a server that appears after asynchronous MCP startup shows up
// without a refresh. Legacy pinned MCP rows are ignored.
//
// Every resource is stamped with its per-resource status so the UI can explain
// why a resource was dropped instead of silently omitting it.
func (server *Server) ContextResources(
	ctx context.Context,
	chat database.Chat,
) ([]codersdk.ChatContextResource, error) {
	pinned, err := server.db.ListChatContextResourcesByChatID(ctx, chat.ID)
	if err != nil {
		return nil, xerrors.Errorf("list chat context resources: %w", err)
	}
	resources := pinnedContextResources(pinned)

	var liveMCP []codersdk.ChatContextResource
	if chat.AgentID.Valid {
		//nolint:gocritic // The chat was authorized before reading its bound agent's runtime capabilities.
		agentResources, err := server.db.ListWorkspaceAgentContextResources(dbauthz.AsChatd(ctx), chat.AgentID.UUID)
		if err != nil {
			// Pinned prompt context remains useful if the live agent inventory is
			// temporarily unavailable. Report it without MCP rather than dropping
			// the entire resource list.
			server.logger.Warn(ctx, "failed to list live workspace MCP context resources",
				slog.F("chat_id", chat.ID),
				slog.F("agent_id", chat.AgentID.UUID),
				slog.Error(err),
			)
		} else {
			liveMCP = liveMCPContextResources(agentResources)
			resources = append(resources, liveMCP...)
		}
	}
	slices.SortFunc(resources, func(a, b codersdk.ChatContextResource) int {
		return cmp.Compare(a.Source, b.Source)
	})

	server.logger.Debug(ctx, "computed chat context resources",
		slog.F("chat_id", chat.ID),
		slog.F("resource_count", len(resources)),
		slog.F("live_mcp_count", len(liveMCP)),
	)
	return resources, nil
}

// liveMCPContextResources converts the bound agent's current MCP rows into the
// metadata-only resource list reported on a chat. It is the live counterpart
// to pinnedContextResources: MCP configs and servers describe capabilities the
// agent can execute right now, so they are reported from the agent's latest
// push rather than from the chat's prompt pin.
//
// Only the MCP kinds contribute; instruction files and skills on the agent are
// reported from the pin instead. OK mcp_server rows carry their tool list.
// Non-OK rows are surfaced with Status and Error, with empty body-specific
// fields, so a server that failed to start is explained rather than omitted.
// Input order (source ASC from the query) is preserved.
func liveMCPContextResources(resources []database.WorkspaceAgentContextResource) []codersdk.ChatContextResource {
	var out []codersdk.ChatContextResource
	for _, r := range resources {
		kind, ok := mcpContextResourceKind(r.BodyKind)
		if !ok {
			continue
		}
		resource := codersdk.ChatContextResource{
			Source:    r.Source,
			Kind:      kind,
			SizeBytes: r.SizeBytes,
			Status:    codersdk.ChatContextResourceStatus(r.Status),
			Error:     r.Error,
		}
		if r.Status == database.WorkspaceAgentContextResourceStatusOk {
			resource.Status = codersdk.ChatContextResourceStatusOK
			if r.BodyKind == database.WorkspaceAgentContextBodyKindMcpServer {
				resource.Tools = mcpToolsFromServerBody(r.Source, r.Body)
			}
		}
		out = append(out, resource)
	}
	return out
}

// pinnedContextResources converts a chat's pinned context rows into the
// metadata-only resource list reported on the chat. It is the reporting
// counterpart to contextResourcesToPrompt: both walk the same rows and share
// the body decoders, but where the prompt builder keeps only OK instruction
// files and skills, this surfaces the full prompt inventory the user can act
// on, each stamped with its Status:
//
//   - OK instruction files with non-empty (sanitized) content, and OK skills
//     with a name.
//   - Non-OK rows (invalid, unreadable, oversize, excluded) of a prompt kind,
//     carrying Status and Error so the UI can explain why the resource was
//     dropped from the prompt instead of silently omitting it. Their
//     body-specific fields are empty.
//
// OK-but-empty instruction files, OK skills with no name, and non-prompt kinds
// are skipped. MCP rows are non-prompt kinds: they are reported live from the
// bound agent, so legacy MCP rows left in a chat's pin are ignored here rather
// than reported as capabilities the agent may no longer have. Input order
// (source ASC from the query) is preserved.
func pinnedContextResources(resources []database.ChatContextResource) []codersdk.ChatContextResource {
	var out []codersdk.ChatContextResource
	for _, r := range resources {
		kind, ok := promptContextResourceKind(r.BodyKind)
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
		}
	}
	return out
}

// promptContextResourceKind maps a database body kind to the codersdk kind
// reported for pinned prompt content. ok is false for every non-prompt kind:
// the MCP kinds, which are reported live from the bound agent, and the kinds
// chatd does not track yet (the reserved plugin/hook/subagent/command kinds).
func promptContextResourceKind(kind database.WorkspaceAgentContextBodyKind) (codersdk.ChatContextResourceKind, bool) {
	switch kind {
	case database.WorkspaceAgentContextBodyKindInstructionFile:
		return codersdk.ChatContextResourceKindInstructionFile, true
	case database.WorkspaceAgentContextBodyKindSkill:
		return codersdk.ChatContextResourceKindSkill, true
	default:
		return "", false
	}
}

// mcpContextResourceKind maps a database body kind to the codersdk kind
// reported for live MCP capabilities. ok is false for every non-MCP kind.
func mcpContextResourceKind(kind database.WorkspaceAgentContextBodyKind) (codersdk.ChatContextResourceKind, bool) {
	switch kind {
	case database.WorkspaceAgentContextBodyKindMcpConfig:
		return codersdk.ChatContextResourceKindMCPConfig, true
	case database.WorkspaceAgentContextBodyKindMcpServer:
		return codersdk.ChatContextResourceKindMCPServer, true
	default:
		return "", false
	}
}
