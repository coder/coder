//go:build !slim

package chatprovider

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// CoderHeaders builds the set of Coder identity headers to attach
// to outgoing LLM API requests for the given chat.
func CoderHeaders(chat database.Chat) map[string]string {
	chatID := chat.ID
	if chat.ParentChatID.Valid {
		chatID = chat.ParentChatID.UUID
	}
	h := map[string]string{
		HeaderCoderOwnerID: chat.OwnerID.String(),
		HeaderCoderChatID:  chatID.String(),
	}
	if chat.ParentChatID.Valid {
		h[HeaderCoderSubchatID] = chat.ID.String()
	}
	if chat.WorkspaceID.Valid {
		h[HeaderCoderWorkspaceID] = chat.WorkspaceID.UUID.String()
	}
	return h
}

// CoderHeadersFromIDs is a convenience form of CoderHeaders for call
// sites that do not have a full database.Chat in scope.
func CoderHeadersFromIDs(
	ownerID uuid.UUID,
	chatID uuid.UUID,
	parentChatID uuid.NullUUID,
	workspaceID uuid.NullUUID,
) map[string]string {
	return CoderHeaders(database.Chat{
		ID:           chatID,
		OwnerID:      ownerID,
		ParentChatID: parentChatID,
		WorkspaceID:  workspaceID,
	})
}
