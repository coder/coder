package dbauthz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

type chatShareFlags struct {
	shareToolCalls   bool
	shareAttachments bool
}

type legacyToolResultRow struct {
	ToolCallID       string          `json:"tool_call_id"`
	ToolName         string          `json:"tool_name"`
	Result           json.RawMessage `json:"result"`
	IsError          bool            `json:"is_error,omitempty"`
	IsMedia          bool            `json:"is_media,omitempty"`
	ProviderExecuted bool            `json:"provider_executed,omitempty"`
	ProviderMetadata json.RawMessage `json:"provider_metadata,omitempty"`
}

func chatShareFlagsForUser(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	userID uuid.UUID,
) (chatShareFlags, error) {
	if len(chat.GroupACL) == 0 {
		return chatShareFlagsFromACL(chat, userID, nil), nil
	}

	groupIDs, err := userGroupIDs(ctx, store, userID)
	if err != nil {
		return chatShareFlags{}, err
	}
	return chatShareFlagsFromACL(chat, userID, groupIDs), nil
}

func chatShareFlagsFromACL(
	chat database.Chat,
	userID uuid.UUID,
	groupIDs map[string]struct{},
) (flags chatShareFlags) {
	if entry, ok := chat.UserACL[userID.String()]; ok {
		flags.shareToolCalls = entry.ShareToolCalls
		flags.shareAttachments = entry.ShareAttachments
	}
	for groupID := range groupIDs {
		entry, ok := chat.GroupACL[groupID]
		if !ok {
			continue
		}
		flags.shareToolCalls = flags.shareToolCalls || entry.ShareToolCalls
		flags.shareAttachments = flags.shareAttachments || entry.ShareAttachments
		if flags.shareToolCalls && flags.shareAttachments {
			break
		}
	}
	return flags
}

func userGroupIDs(
	ctx context.Context,
	store database.Store,
	userID uuid.UUID,
) (map[string]struct{}, error) {
	groups, err := store.GetGroups(AsSystemRestricted(ctx), database.GetGroupsParams{
		HasMemberID: userID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("list viewer groups: %w", err)
	}

	ids := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		ids[group.Group.ID.String()] = struct{}{}
	}
	return ids, nil
}

func chatShareFlagsForActor(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
) (chatShareFlags, bool, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return chatShareFlags{}, false, ErrNoActor
	}
	if actor.Type == rbac.SubjectTypeSystemRestricted || actor.Type == rbac.SubjectTypeChatd {
		return chatShareFlags{}, true, nil
	}
	if actor.Type != "" && actor.Type != rbac.SubjectTypeUser {
		return chatShareFlags{}, false, xerrors.Errorf("unsupported actor type %q", actor.Type)
	}

	userID, err := uuid.Parse(actor.ID)
	if err != nil {
		return chatShareFlags{}, false, xerrors.Errorf("parse actor id: %w", err)
	}
	if chat.OwnerID == userID {
		return chatShareFlags{}, true, nil
	}

	flags, err := chatShareFlagsForUser(ctx, store, chat, userID)
	if err != nil {
		return chatShareFlags{}, false, err
	}
	return flags, false, nil
}

func chatAttachmentsVisible(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
) (bool, error) {
	flags, bypass, err := chatShareFlagsForActor(ctx, store, chat)
	if err != nil {
		return false, err
	}
	if bypass {
		return true, nil
	}
	return flags.shareAttachments, nil
}

func redactDatabaseChat(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
) (database.Chat, error) {
	flags, bypass, err := chatShareFlagsForActor(ctx, store, chat)
	if err != nil {
		return database.Chat{}, err
	}
	if bypass {
		return chat, nil
	}
	return applyDatabaseChatRedaction(chat, flags), nil
}

func redactDatabaseChats(
	ctx context.Context,
	store database.Store,
	rows []database.GetChatsRow,
) ([]database.GetChatsRow, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return nil, ErrNoActor
	}
	if actor.Type != "" && actor.Type != rbac.SubjectTypeUser {
		return rows, nil
	}

	userID, err := uuid.Parse(actor.ID)
	if err != nil {
		return nil, xerrors.Errorf("parse actor id: %w", err)
	}

	needsGroups := false
	for _, row := range rows {
		if row.Chat.OwnerID == userID || len(row.Chat.GroupACL) == 0 {
			continue
		}
		needsGroups = true
		break
	}

	var groupIDs map[string]struct{}
	if needsGroups {
		groupIDs, err = userGroupIDs(ctx, store, userID)
		if err != nil {
			return nil, err
		}
	}

	redacted := make([]database.GetChatsRow, len(rows))
	for i, row := range rows {
		redacted[i] = row
		if row.Chat.OwnerID == userID {
			continue
		}
		redacted[i].Chat = applyDatabaseChatRedaction(
			row.Chat,
			chatShareFlagsFromACL(row.Chat, userID, groupIDs),
		)
	}
	return redacted, nil
}

func redactDatabaseChildChats(
	ctx context.Context,
	store database.Store,
	rows []database.GetChildChatsByParentIDsRow,
) ([]database.GetChildChatsByParentIDsRow, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return nil, ErrNoActor
	}
	if actor.Type != "" && actor.Type != rbac.SubjectTypeUser {
		return rows, nil
	}

	userID, err := uuid.Parse(actor.ID)
	if err != nil {
		return nil, xerrors.Errorf("parse actor id: %w", err)
	}

	needsGroups := false
	for _, row := range rows {
		if row.Chat.OwnerID == userID || len(row.Chat.GroupACL) == 0 {
			continue
		}
		needsGroups = true
		break
	}

	var groupIDs map[string]struct{}
	if needsGroups {
		groupIDs, err = userGroupIDs(ctx, store, userID)
		if err != nil {
			return nil, err
		}
	}

	redacted := make([]database.GetChildChatsByParentIDsRow, len(rows))
	for i, row := range rows {
		redacted[i] = row
		if row.Chat.OwnerID == userID {
			continue
		}
		redacted[i].Chat = applyDatabaseChatRedaction(
			row.Chat,
			chatShareFlagsFromACL(row.Chat, userID, groupIDs),
		)
	}
	return redacted, nil
}

func applyDatabaseChatRedaction(chat database.Chat, flags chatShareFlags) database.Chat {
	chat.LastInjectedContext = stripChatPartsRawMessage(chat.LastInjectedContext, flags)
	return chat
}

func redactDatabaseMessages(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	messages []database.ChatMessage,
) ([]database.ChatMessage, error) {
	flags, bypass, err := chatShareFlagsForActor(ctx, store, chat)
	if err != nil {
		return nil, err
	}
	if bypass {
		return messages, nil
	}

	redacted := make([]database.ChatMessage, 0, len(messages))
	for _, message := range messages {
		message, ok := redactDatabaseMessage(message, flags)
		if !ok {
			continue
		}
		redacted = append(redacted, message)
	}
	return redacted, nil
}

func redactDatabaseQueuedMessages(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	messages []database.ChatQueuedMessage,
) ([]database.ChatQueuedMessage, error) {
	flags, bypass, err := chatShareFlagsForActor(ctx, store, chat)
	if err != nil {
		return nil, err
	}
	if bypass {
		return messages, nil
	}

	redacted := make([]database.ChatQueuedMessage, 0, len(messages))
	for _, message := range messages {
		message, ok := redactDatabaseQueuedMessage(message, flags)
		if !ok {
			continue
		}
		redacted = append(redacted, message)
	}
	return redacted, nil
}

func redactDatabaseMessage(
	message database.ChatMessage,
	flags chatShareFlags,
) (database.ChatMessage, bool) {
	parts, ok := parseMessagePartsForRedaction(message)
	if !ok {
		return database.ChatMessage{}, false
	}
	message.Content = marshalRedactedParts(redactChatMessageParts(parts, flags))
	if !message.Content.Valid {
		return database.ChatMessage{}, false
	}
	message.ContentVersion = 1
	return message, true
}

func redactDatabaseQueuedMessage(
	message database.ChatQueuedMessage,
	flags chatShareFlags,
) (database.ChatQueuedMessage, bool) {
	parts, ok := parseQueuedMessagePartsForRedaction(message)
	if !ok {
		return database.ChatQueuedMessage{}, false
	}
	content := marshalRedactedParts(redactChatMessageParts(parts, flags))
	if !content.Valid {
		return database.ChatQueuedMessage{}, false
	}
	message.Content = content.RawMessage
	return message, true
}

func parseMessagePartsForRedaction(message database.ChatMessage) ([]codersdk.ChatMessagePart, bool) {
	if !message.Content.Valid || len(message.Content.RawMessage) == 0 {
		return nil, false
	}

	var parts []codersdk.ChatMessagePart
	if err := json.Unmarshal(message.Content.RawMessage, &parts); err == nil && hasNonEmptyType(parts) {
		return parts, true
	}

	var text string
	if err := json.Unmarshal(message.Content.RawMessage, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		return []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}, true
	}

	var rows []legacyToolResultRow
	if err := json.Unmarshal(message.Content.RawMessage, &rows); err == nil && len(rows) > 0 {
		toolParts := make([]codersdk.ChatMessagePart, 0, len(rows))
		for _, row := range rows {
			part := codersdk.ChatMessageToolResult(
				row.ToolCallID,
				row.ToolName,
				row.Result,
				row.IsError,
				row.IsMedia,
			)
			part.ProviderExecuted = row.ProviderExecuted
			part.ProviderMetadata = row.ProviderMetadata
			toolParts = append(toolParts, part)
		}
		return toolParts, true
	}

	return nil, false
}

func parseQueuedMessagePartsForRedaction(
	message database.ChatQueuedMessage,
) ([]codersdk.ChatMessagePart, bool) {
	if len(message.Content) == 0 {
		return nil, false
	}

	var parts []codersdk.ChatMessagePart
	if err := json.Unmarshal(message.Content, &parts); err == nil && hasNonEmptyType(parts) {
		return parts, true
	}

	var text string
	if err := json.Unmarshal(message.Content, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		return []codersdk.ChatMessagePart{codersdk.ChatMessageText(text)}, true
	}

	return nil, false
}

func hasNonEmptyType(parts []codersdk.ChatMessagePart) bool {
	for _, part := range parts {
		if string(part.Type) != "" {
			return true
		}
	}
	return false
}

func FilterChatStreamEvents(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	events []codersdk.ChatStreamEvent,
) ([]codersdk.ChatStreamEvent, error) {
	flags, bypass, err := chatShareFlagsForActor(ctx, store, chat)
	if err != nil {
		return nil, err
	}
	if bypass {
		return events, nil
	}

	redacted := make([]codersdk.ChatStreamEvent, 0, len(events))
	for _, event := range events {
		if event.ActionRequired != nil && !flags.shareToolCalls {
			continue
		}
		if event.Message != nil {
			message := redactMessage(*event.Message, flags)
			if len(message.Content) == 0 {
				continue
			}
			event.Message = &message
		}
		if event.MessagePart != nil {
			parts := redactChatMessageParts(
				[]codersdk.ChatMessagePart{event.MessagePart.Part},
				flags,
			)
			if len(parts) == 0 {
				continue
			}
			messagePart := *event.MessagePart
			messagePart.Part = parts[0]
			event.MessagePart = &messagePart
		}
		if len(event.QueuedMessages) > 0 {
			event.QueuedMessages = redactQueuedMessages(event.QueuedMessages, flags)
		}
		redacted = append(redacted, event)
	}
	return redacted, nil
}

func redactQueuedMessages(
	messages []codersdk.ChatQueuedMessage,
	flags chatShareFlags,
) []codersdk.ChatQueuedMessage {
	out := make([]codersdk.ChatQueuedMessage, len(messages))
	n := 0
	for _, message := range messages {
		message.Content = redactChatMessageParts(message.Content, flags)
		if len(message.Content) == 0 {
			continue
		}
		out[n] = message
		n++
	}
	return out[:n]
}

func redactMessage(
	message codersdk.ChatMessage,
	flags chatShareFlags,
) codersdk.ChatMessage {
	message.Content = redactChatMessageParts(message.Content, flags)
	return message
}

func redactChatMessageParts(
	parts []codersdk.ChatMessagePart,
	flags chatShareFlags,
) []codersdk.ChatMessagePart {
	redacted := make([]codersdk.ChatMessagePart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeToolCall,
			codersdk.ChatMessagePartTypeToolResult:
			if !flags.shareToolCalls {
				continue
			}
		case codersdk.ChatMessagePartTypeFile,
			codersdk.ChatMessagePartTypeFileReference,
			codersdk.ChatMessagePartTypeContextFile:
			if !flags.shareAttachments {
				continue
			}
		}
		redacted = append(redacted, part)
	}
	return redacted
}

func stripChatPartsRawMessage(
	raw pqtype.NullRawMessage,
	flags chatShareFlags,
) pqtype.NullRawMessage {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return raw
	}

	var parts []codersdk.ChatMessagePart
	if err := json.Unmarshal(raw.RawMessage, &parts); err != nil || !hasNonEmptyType(parts) {
		return pqtype.NullRawMessage{}
	}
	return marshalRedactedParts(redactChatMessageParts(parts, flags))
}

func marshalRedactedParts(parts []codersdk.ChatMessagePart) pqtype.NullRawMessage {
	if len(parts) == 0 {
		return pqtype.NullRawMessage{}
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: encoded, Valid: true}
}
