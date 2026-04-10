package chatd

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// AgentChatContextSentinelPath marks the synthetic empty context-file
// part used to preserve skill-only workspace-agent additions across
// turns without treating them as persisted instruction files.
const AgentChatContextSentinelPath = ".coder/agent-chat-context-sentinel"

// FilterContextParts keeps only context-file and skill parts from parts.
// When keepEmptyContextFiles is false, context-file parts with empty
// content are dropped. When keepEmptyContextFiles is true, empty
// context-file parts are preserved.
// revive:disable-next-line:flag-parameter // Required by shared helper callers.
func FilterContextParts(
	parts []codersdk.ChatMessagePart,
	keepEmptyContextFiles bool,
) []codersdk.ChatMessagePart {
	var filtered []codersdk.ChatMessagePart
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeContextFile:
			if !keepEmptyContextFiles && part.ContextFileContent == "" {
				continue
			}
		case codersdk.ChatMessagePartTypeSkill:
		default:
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

// CollectContextPartsFromMessages unmarshals chat message content and
// collects the context-file and skill parts it contains. When
// keepEmptyContextFiles is false, empty context-file parts are skipped.
// When it is true, empty context-file parts are included in the result.
func CollectContextPartsFromMessages(
	ctx context.Context,
	logger slog.Logger,
	messages []database.ChatMessage,
	keepEmptyContextFiles bool,
) ([]codersdk.ChatMessagePart, error) {
	var collected []codersdk.ChatMessagePart
	for _, msg := range messages {
		if !msg.Content.Valid {
			continue
		}

		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			logger.Warn(ctx, "skipping malformed chat context message",
				slog.F("chat_message_id", msg.ID),
				slog.Error(err),
			)
			continue
		}

		collected = append(
			collected,
			FilterContextParts(parts, keepEmptyContextFiles)...,
		)
	}

	return collected, nil
}

func latestContextAgentIDFromParts(parts []codersdk.ChatMessagePart) (uuid.UUID, bool) {
	var lastID uuid.UUID
	found := false
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeContextFile ||
			!part.ContextFileAgentID.Valid {
			continue
		}
		lastID = part.ContextFileAgentID.UUID
		found = true
	}
	return lastID, found
}

// FilterContextPartsToLatestAgent keeps parts stamped with the latest
// workspace-agent ID seen in the slice, plus legacy unstamped parts.
// When no stamped context-file parts exist, it returns the original
// slice unchanged.
func FilterContextPartsToLatestAgent(parts []codersdk.ChatMessagePart) []codersdk.ChatMessagePart {
	latestAgentID, ok := latestContextAgentIDFromParts(parts)
	if !ok {
		return parts
	}

	filtered := make([]codersdk.ChatMessagePart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeContextFile,
			codersdk.ChatMessagePartTypeSkill:
			if part.ContextFileAgentID.Valid &&
				part.ContextFileAgentID.UUID != latestAgentID {
				continue
			}
		default:
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

// BuildLastInjectedContext filters parts down to non-empty context-file
// and skill parts, strips their internal fields, and marshals the
// result for LastInjectedContext. A nil or fully filtered input returns
// an invalid NullRawMessage.
func BuildLastInjectedContext(
	parts []codersdk.ChatMessagePart,
) (pqtype.NullRawMessage, error) {
	if parts == nil {
		return pqtype.NullRawMessage{Valid: false}, nil
	}

	filtered := FilterContextParts(parts, false)
	if len(filtered) == 0 {
		return pqtype.NullRawMessage{Valid: false}, nil
	}

	stripped := make([]codersdk.ChatMessagePart, 0, len(filtered))
	for _, part := range filtered {
		cp := part
		cp.StripInternal()
		stripped = append(stripped, cp)
	}

	raw, err := json.Marshal(stripped)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf(
			"marshal injected context: %w",
			err,
		)
	}

	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}, nil
}
