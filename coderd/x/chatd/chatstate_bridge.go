package chatd

import (
	"database/sql"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

// newChatMachine constructs a chat-scoped state machine handle bound to
// the server's database and pubsub.
func (p *Server) newChatMachine(chatID uuid.UUID) *chatstate.ChatMachine {
	return chatstate.NewChatMachine(p.db, p.pubsub, chatID)
}

// systemMessage builds a chatstate.Message representing a system
// prompt entry for the initial-history slice of CreateChat.
func systemMessage(rawContent pqtype.NullRawMessage, modelConfigID uuid.UUID) chatstate.Message {
	return chatstate.Message{
		Role:           database.ChatMessageRoleSystem,
		Content:        rawContent,
		Visibility:     database.ChatMessageVisibilityModel,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func userMessageWithAPIKeyID(rawContent pqtype.NullRawMessage, modelConfigID, createdBy uuid.UUID, apiKeyID string, reasoningEffort *string) chatstate.Message {
	var effort database.NullChatReasoningEffort
	if reasoningEffort != nil && *reasoningEffort != "" {
		effort = database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffort(*reasoningEffort), Valid: true}
	}
	return chatstate.Message{
		Role:            database.ChatMessageRoleUser,
		Content:         rawContent,
		Visibility:      database.ChatMessageVisibilityBoth,
		ModelConfigID:   uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
		ReasoningEffort: effort,
		CreatedBy:       uuid.NullUUID{UUID: createdBy, Valid: createdBy != uuid.Nil},
		ContentVersion:  chatprompt.CurrentContentVersion,
		APIKeyID:        sql.NullString{String: apiKeyID, Valid: apiKeyID != ""},
	}
}

// busyBehaviorToChatState converts the public busy-behavior enum used
// by the server API to the chatstate variant.
func busyBehaviorToChatState(b SendMessageBusyBehavior) chatstate.BusyBehavior {
	switch b {
	case SendMessageBusyBehaviorInterrupt:
		return chatstate.BusyBehaviorInterrupt
	default:
		return chatstate.BusyBehaviorQueue
	}
}
