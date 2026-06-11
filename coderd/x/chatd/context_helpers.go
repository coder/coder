package chatd

import (
	"bytes"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// agentChatContextSentinelPath marks the synthetic empty context-file
// part used to record an attempted workspace-context fetch when no
// AGENTS.md content is available. It mirrors the constant of the same
// value in the chatd package so the worker can recognize sentinel
// parts without importing chatd (which would be a cycle).
const agentChatContextSentinelPath = ".coder/agent-chat-context-sentinel"

// contextFileAgentIDFromMessages returns the most recent workspace
// agent ID stamped on a persisted context-file part, ignoring the
// skill-only sentinel. Returns uuid.Nil, false when no stamped
// non-sentinel context-file parts exist.
//
// This mirrors chatd.contextFileAgentID. It is duplicated here as a
// small pure helper so chatworker can decide whether workspace
// context is current without importing chatd.
func contextFileAgentIDFromMessages(messages []database.ChatMessage) (uuid.UUID, bool) {
	var lastID uuid.UUID
	found := false
	for _, msg := range messages {
		if !msg.Content.Valid || !bytes.Contains(msg.Content.RawMessage, []byte(`"context-file"`)) {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, p := range parts {
			if p.Type != codersdk.ChatMessagePartTypeContextFile ||
				!p.ContextFileAgentID.Valid ||
				p.ContextFilePath == agentChatContextSentinelPath {
				continue
			}
			lastID = p.ContextFileAgentID.UUID
			found = true
			break
		}
	}
	return lastID, found
}

// hasPersistedContextFileForAgent reports whether messages include
// any persisted context-file marker for the given agent, including
// the skill-only sentinel. This is true once the
// persist_workspace_context action has committed at least one
// context-file row for the agent (with or without content), so a
// subsequent decision pass will not loop on the same agent.
func hasPersistedContextFileForAgent(messages []database.ChatMessage, agentID uuid.UUID) bool {
	if agentID == uuid.Nil {
		return false
	}
	for _, msg := range messages {
		if !msg.Content.Valid || !bytes.Contains(msg.Content.RawMessage, []byte(`"context-file"`)) {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, p := range parts {
			if p.Type != codersdk.ChatMessagePartTypeContextFile ||
				!p.ContextFileAgentID.Valid {
				continue
			}
			if p.ContextFileAgentID.UUID == agentID {
				return true
			}
		}
	}
	return false
}
