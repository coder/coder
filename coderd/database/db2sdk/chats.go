package db2sdk

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func Chat(c database.Chat) codersdk.Chat {
	out := codersdk.Chat{
		ID:             c.ID,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
		OrganizationID: c.OrganizationID,
		OwnerID:        c.OwnerID,
		WorkspaceID:    c.WorkspaceID,
		Provider:       c.Provider,
		Model:          c.Model,
		Metadata:       c.Metadata,
	}
	if c.Title.Valid {
		out.Title = c.Title.String
	}
	return out
}

func ChatMessage(m database.ChatMessage) codersdk.ChatMessage {
	return codersdk.ChatMessage{
		ChatID:    m.ChatID,
		ID:        m.ID,
		CreatedAt: m.CreatedAt,
		Role:      m.Role,
		Content:   m.Content,
	}
}
