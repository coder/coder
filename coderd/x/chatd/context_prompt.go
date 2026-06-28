package chatd

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
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

// AgentChatContextSentinelPath is the canonical path of the synthetic empty
// context-file part that legacy chats used to mark skill-only workspace-agent
// context. New turns no longer emit it; it is retained as the canonical value
// so historical-message handling and the chatopenai chain-mode tests stay in
// sync.
const AgentChatContextSentinelPath = ".coder/agent-chat-context-sentinel"

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

// mcpToolsFromServerBody decodes a stored mcp_server resource body and returns
// its tool list for the chat response. The agent prefixes each tool name with
// "<server>__"; that prefix is stripped so the name reads as the server
// exposes it. Returns nil when the body has no tools or cannot be decoded.
func mcpToolsFromServerBody(server string, body json.RawMessage) []codersdk.ChatContextTool {
	var decoded agentproto.MCPServerBody
	if err := contextBodyUnmarshalOptions.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	tools := decoded.GetTools()
	if len(tools) == 0 {
		return nil
	}
	prefix := server + mcpToolNameSeparator
	out := make([]codersdk.ChatContextTool, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		name := strings.TrimPrefix(t.GetName(), prefix)
		if name == "" {
			continue
		}
		// A server that lists the same tool twice would otherwise report it
		// twice; keep the first occurrence so the reported tool set matches
		// the deduplicated set the turn assembles.
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
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
// contribute; the per-resource decode and "<server>__<tool>" re-prefixing
// live in appendMCPToolInfos, shared with the live agent reader.
func workspaceMCPToolInfosFromResources(resources []database.ChatContextResource) []workspacesdk.MCPToolInfo {
	var out []workspacesdk.MCPToolInfo
	for _, r := range resources {
		out = appendMCPToolInfos(out, r.BodyKind, r.Status, r.Source, r.Body)
	}
	return out
}

// agentWorkspaceMCPToolInfos decodes an agent's latest pushed mcp_server
// resources (workspace_agent_context_resources) into execution-ready tool
// infos. It is the live counterpart to workspaceMCPToolInfosFromResources,
// which reads the chat's frozen pinned copy; both share appendMCPToolInfos so
// a stored server body is interpreted identically on either path.
func agentWorkspaceMCPToolInfos(resources []database.WorkspaceAgentContextResource) []workspacesdk.MCPToolInfo {
	var out []workspacesdk.MCPToolInfo
	for _, r := range resources {
		out = appendMCPToolInfos(out, r.BodyKind, r.Status, r.Source, r.Body)
	}
	return out
}

// appendMCPToolInfos decodes one mcp_server resource body and appends its
// execution-ready tool infos to out. Non-mcp_server kinds, non-OK statuses,
// undecodable bodies, and tools with an empty name are skipped. The agent
// reports tool names unprefixed alongside the server name, so each tool is
// re-prefixed to "<server>__<tool>", the model-facing and proxy-routable form
// the live discovery path also produces. The pushed input schema is a full
// JSON Schema object; its "properties" and "required" are split out to match
// the shape the workspace agent's live tool list produces (see
// agent/x/agentmcp).
func appendMCPToolInfos(
	out []workspacesdk.MCPToolInfo,
	bodyKind database.WorkspaceAgentContextBodyKind,
	status database.WorkspaceAgentContextResourceStatus,
	source string,
	body json.RawMessage,
) []workspacesdk.MCPToolInfo {
	if bodyKind != database.WorkspaceAgentContextBodyKindMcpServer ||
		status != database.WorkspaceAgentContextResourceStatusOk {
		return out
	}
	var decoded agentproto.MCPServerBody
	if err := contextBodyUnmarshalOptions.Unmarshal(body, &decoded); err != nil {
		return out
	}
	server := decoded.GetServerName()
	if server == "" {
		server = source
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

// resolveWorkspaceMCPToolsConverged returns the workspace MCP tools for a turn.
// It starts from the chat's pinned baseline (resolveWorkspaceMCPTools) and
// overlays the agent's latest pushed MCP set. MCP config and server resources
// are excluded from the drift hash, so a server the agent connects after a
// chat was pinned never flips the chat dirty and would otherwise never reach
// it. Overlaying the live set lets an already-pinned parent and a later child
// expose the same MCP servers without a manual refresh. Instruction and skill
// content stay pinned; only the MCP tool set is sourced live.
func (server *Server) resolveWorkspaceMCPToolsConverged(
	ctx context.Context,
	logger slog.Logger,
	chat database.Chat,
	workspaceCtx *turnWorkspaceContext,
) []fantasy.AgentTool {
	pinned := server.resolveWorkspaceMCPTools(ctx, logger, chat, workspaceCtx)

	agent, err := workspaceCtx.getWorkspaceAgent(ctx)
	if err != nil || agent.ID == uuid.Nil {
		// Agent unbound or unreachable: keep the pin.
		return pinned
	}
	return server.overlayLiveWorkspaceMCPTools(ctx, logger, chat.ID, agent.ID, pinned, workspaceCtx.getWorkspaceConn)
}

// overlayLiveWorkspaceMCPTools reads agentID's latest pushed MCP set and merges
// it over the pinned baseline. Live wins per server; a server present only in
// the pin is retained. A read failure or an empty live MCP set keeps the pin
// intact, so a transiently empty or disconnected snapshot never strips MCP
// tools mid-turn: the agent re-pushes and the next turn reconverges.
func (server *Server) overlayLiveWorkspaceMCPTools(
	ctx context.Context,
	logger slog.Logger,
	chatID uuid.UUID,
	agentID uuid.UUID,
	pinned []fantasy.AgentTool,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.AgentTool {
	//nolint:gocritic // Chatd reads the agent's live context as the daemon subject.
	resources, err := server.db.ListWorkspaceAgentContextResources(dbauthz.AsChatd(ctx), agentID)
	if err != nil {
		logger.Warn(ctx, "failed to read live workspace MCP context for convergence",
			slog.F("chat_id", chatID), slog.F("agent_id", agentID), slog.Error(err))
		return pinned
	}
	liveInfos := agentWorkspaceMCPToolInfos(resources)
	if len(liveInfos) == 0 {
		return pinned
	}
	return mergeWorkspaceMCPToolsByServer(ctx, logger, pinned, liveInfos, getConn)
}

// mergeWorkspaceMCPToolsByServer overlays the live MCP tool infos over the
// pinned baseline tools, replacing per server: a pinned tool whose server the
// live set also advertises is dropped in favor of the live definition, while a
// pinned tool for a server absent from the live set is retained. The live
// tools are then appended. This converges an already-pinned chat onto the
// agent's current MCP set while never dropping a server the live snapshot
// happens to omit. The prefix test uses the server separator so a server name
// is matched on a whole segment, not a partial name.
func mergeWorkspaceMCPToolsByServer(
	ctx context.Context,
	logger slog.Logger,
	pinned []fantasy.AgentTool,
	liveInfos []workspacesdk.MCPToolInfo,
	getConn func(context.Context) (workspacesdk.AgentConn, error),
) []fantasy.AgentTool {
	livePrefixes := make([]string, 0, len(liveInfos))
	seenServer := make(map[string]struct{}, len(liveInfos))
	for _, info := range liveInfos {
		if _, ok := seenServer[info.ServerName]; ok {
			continue
		}
		seenServer[info.ServerName] = struct{}{}
		livePrefixes = append(livePrefixes, info.ServerName+mcpToolNameSeparator)
	}

	merged := make([]fantasy.AgentTool, 0, len(pinned)+len(liveInfos))
	for _, tool := range pinned {
		name := tool.Info().Name
		superseded := false
		for _, prefix := range livePrefixes {
			if strings.HasPrefix(name, prefix) {
				superseded = true
				break
			}
		}
		if superseded {
			logger.Debug(ctx, "replacing pinned workspace MCP tool with live definition",
				slog.F("tool_name", name))
			continue
		}
		merged = append(merged, tool)
	}
	for _, info := range liveInfos {
		merged = append(merged, chattool.NewWorkspaceMCPTool(info, getConn, nil))
	}
	return merged
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
	// Non-OK resources (oversize, unreadable, invalid, excluded) are dropped
	// from the prompt without an error. Emit one aggregated debug line with
	// the per-status counts so a "missing context" report is diagnosable
	// without dumping every pinned row.
	if statusFields := nonOKResourceStatusFields(resources); len(statusFields) > 0 {
		server.logger.Debug(ctx, "skipped non-ok pinned chat context resources",
			slog.F("chat_id", chat.ID),
			slog.F("resource_count", len(resources)),
			slog.F("status_counts", slog.M(statusFields...)),
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

// nonOKResourceStatusFields tallies pinned resources whose status is not OK
// and returns one log field per non-OK status present, in a fixed order so the
// emitted log is deterministic. These statuses (oversize, unreadable, invalid,
// excluded) are exactly the bodies contextResourcesToPrompt drops from the
// prompt, so the counts explain a "missing context" report without dumping
// every row. Returns nil when every resource is OK.
func nonOKResourceStatusFields(resources []database.ChatContextResource) []slog.Field {
	counts := map[database.WorkspaceAgentContextResourceStatus]int{}
	for _, r := range resources {
		if r.Status != database.WorkspaceAgentContextResourceStatusOk {
			counts[r.Status]++
		}
	}
	ordered := []database.WorkspaceAgentContextResourceStatus{
		database.WorkspaceAgentContextResourceStatusOversize,
		database.WorkspaceAgentContextResourceStatusUnreadable,
		database.WorkspaceAgentContextResourceStatusInvalid,
		database.WorkspaceAgentContextResourceStatusExcluded,
	}
	var fields []slog.Field
	for _, status := range ordered {
		if n := counts[status]; n > 0 {
			fields = append(fields, slog.F(string(status), n))
		}
	}
	return fields
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
				Tools:     mcpToolsFromServerBody(r.Source, r.Body),
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
