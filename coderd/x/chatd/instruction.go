package chatd

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// formatSystemInstructions builds the <workspace-context> block from
// agent metadata and zero or more context-file parts. Non-context-file
// parts in the slice are silently skipped.
func formatSystemInstructions(
	operatingSystem, directory string,
	parts []codersdk.ChatMessagePart,
) string {
	hasContent := false
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeContextFile && part.ContextFileContent != "" {
			hasContent = true
			break
		}
	}
	if !hasContent && operatingSystem == "" && directory == "" {
		return ""
	}

	var b strings.Builder
	_, _ = b.WriteString("<workspace-context>\n")
	if operatingSystem != "" {
		_, _ = b.WriteString("Operating System: ")
		_, _ = b.WriteString(operatingSystem)
		_, _ = b.WriteString("\n")
	}
	if directory != "" {
		_, _ = b.WriteString("Working Directory: ")
		_, _ = b.WriteString(directory)
		_, _ = b.WriteString("\n")
	}
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeContextFile || part.ContextFileContent == "" {
			continue
		}
		_, _ = b.WriteString("\nSource: ")
		_, _ = b.WriteString(part.ContextFilePath)
		if part.ContextFileTruncated {
			_, _ = b.WriteString(" (truncated to 64KiB)")
		}
		_, _ = b.WriteString("\n")
		_, _ = b.WriteString(part.ContextFileContent)
		_, _ = b.WriteString("\n")
	}
	_, _ = b.WriteString("</workspace-context>")
	return b.String()
}

// latestContextAgentID returns the most recent workspace-agent ID seen
// on any persisted context-file part, including the skill-only sentinel.
// Returns uuid.Nil, false when no stamped context-file parts exist.
func latestContextAgentID(messages []database.ChatMessage) (uuid.UUID, bool) {
	var lastID uuid.UUID
	found := false
	for _, msg := range messages {
		if !msg.Content.Valid ||
			!bytes.Contains(msg.Content.RawMessage, []byte(`"context-file"`)) {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeContextFile ||
				!part.ContextFileAgentID.Valid {
				continue
			}
			lastID = part.ContextFileAgentID.UUID
			found = true
			break
		}
	}
	return lastID, found
}

// instructionFromContextFiles reconstructs the formatted instruction
// string from persisted context-file parts. This is used on non-first
// turns so the instruction can be re-injected after compaction
// without re-dialing the workspace agent.
func instructionFromContextFiles(
	messages []database.ChatMessage,
) string {
	filterAgentID, filterByAgent := latestContextAgentID(messages)
	var contextParts []codersdk.ChatMessagePart
	var os, dir string
	for _, msg := range messages {
		if !msg.Content.Valid ||
			!bytes.Contains(msg.Content.RawMessage, []byte(`"context-file"`)) {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeContextFile {
				continue
			}
			if filterByAgent && part.ContextFileAgentID.Valid &&
				part.ContextFileAgentID.UUID != filterAgentID {
				continue
			}
			if part.ContextFileOS != "" {
				os = part.ContextFileOS
			}
			if part.ContextFileDirectory != "" {
				dir = part.ContextFileDirectory
			}
			if part.ContextFileContent != "" {
				contextParts = append(contextParts, part)
			}
		}
	}
	return formatSystemInstructions(os, dir, contextParts)
}

// hasPersistedInstructionFiles reports whether messages include a
// persisted context-file part that should suppress another baseline
// instruction-file lookup. The workspace-agent skill-only sentinel is
// ignored so default instructions still load on fresh chats.
func hasPersistedInstructionFiles(
	messages []database.ChatMessage,
) bool {
	for _, msg := range messages {
		if !msg.Content.Valid ||
			!bytes.Contains(msg.Content.RawMessage, []byte(`"context-file"`)) {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeContextFile ||
				!part.ContextFileAgentID.Valid ||
				part.ContextFilePath == AgentChatContextSentinelPath {
				continue
			}
			return true
		}
	}
	return false
}

func mergeSkillMetas(
	persisted []chattool.SkillMeta,
	discovered []chattool.SkillMeta,
) []chattool.SkillMeta {
	if len(persisted) == 0 {
		return discovered
	}
	if len(discovered) == 0 {
		return persisted
	}

	seen := make(map[string]struct{}, len(persisted)+len(discovered))
	merged := make([]chattool.SkillMeta, 0, len(persisted)+len(discovered))
	appendUnique := func(skill chattool.SkillMeta) {
		if _, ok := seen[skill.Name]; ok {
			return
		}
		seen[skill.Name] = struct{}{}
		merged = append(merged, skill)
	}
	for _, skill := range discovered {
		appendUnique(skill)
	}
	for _, skill := range persisted {
		appendUnique(skill)
	}
	return merged
}

// selectSkillMetasForInstructionRefresh chooses which skill metadata
// should be injected on a turn that refreshes instruction files.
func selectSkillMetasForInstructionRefresh(
	persisted []chattool.SkillMeta,
	discovered []chattool.SkillMeta,
	currentAgentID uuid.NullUUID,
	latestInjectedAgentID uuid.NullUUID,
) []chattool.SkillMeta {
	if currentAgentID.Valid && latestInjectedAgentID.Valid && latestInjectedAgentID.UUID == currentAgentID.UUID {
		return mergeSkillMetas(persisted, discovered)
	}
	if !currentAgentID.Valid && len(discovered) == 0 {
		return persisted
	}
	return discovered
}

// skillsFromParts reconstructs skill metadata from persisted
// skill parts. This is analogous to instructionFromContextFiles
// so the skill index can be re-injected after compaction without
// re-dialing the workspace agent.
func skillsFromParts(
	messages []database.ChatMessage,
) []chattool.SkillMeta {
	filterAgentID, filterByAgent := latestContextAgentID(messages)
	var skills []chattool.SkillMeta
	for _, msg := range messages {
		if !msg.Content.Valid ||
			!bytes.Contains(msg.Content.RawMessage, []byte(`"skill"`)) {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeSkill {
				continue
			}
			if filterByAgent && part.ContextFileAgentID.Valid &&
				part.ContextFileAgentID.UUID != filterAgentID {
				continue
			}
			skills = append(skills, chattool.SkillMeta{
				Name:        part.SkillName,
				Description: part.SkillDescription,
				Dir:         part.SkillDir,
				MetaFile:    part.ContextFileSkillMetaFile,
			})
		}
	}
	return skills
}

// filterSkillParts returns stripped copies of skill-type parts from
// the given slice. Internal fields are removed so the result is safe
// for the cache column. Returns nil when no skill parts exist.
func filterSkillParts(parts []codersdk.ChatMessagePart) []codersdk.ChatMessagePart {
	var out []codersdk.ChatMessagePart
	for _, p := range parts {
		if p.Type != codersdk.ChatMessagePartTypeSkill {
			continue
		}
		cp := p
		cp.StripInternal()
		out = append(out, cp)
	}
	return out
}
