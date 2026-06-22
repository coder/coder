package chatd

import (
	"context"

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
			var body agentproto.InstructionFileBody
			if err := contextBodyUnmarshalOptions.Unmarshal(r.Body, &body); err != nil {
				malformed++
				continue
			}
			content := SanitizePromptText(string(body.GetContent()))
			if content == "" {
				continue
			}
			contextFileParts = append(contextFileParts, codersdk.ChatMessagePart{
				Type:               codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:    r.Source,
				ContextFileContent: content,
			})
		case database.WorkspaceAgentContextBodyKindSkill:
			var body agentproto.SkillMetaBody
			if err := contextBodyUnmarshalOptions.Unmarshal(r.Body, &body); err != nil {
				malformed++
				continue
			}
			if body.GetName() == "" {
				continue
			}
			// source is the skill directory. MetaFile is left empty so
			// chattool falls back to DefaultSkillMetaFile ("SKILL.md").
			// SkillMetaBody carries no meta file name, so a non-default
			// CODER_AGENT_EXP_SKILL_META_FILE is not preserved on this
			// path, unlike the per-turn discovery path.
			skills = append(skills, chattool.SkillMeta{
				Name:        body.GetName(),
				Description: body.GetDescription(),
				Dir:         r.Source,
			})
		}
	}

	if len(contextFileParts) == 0 {
		return "", skills, malformed
	}
	return formatSystemInstructions(operatingSystem, directory, contextFileParts), skills, malformed
}
