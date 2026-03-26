package automations

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	chatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// ChatdAdapter implements ChatCreator by delegating to chatd.Server.
type ChatdAdapter struct {
	Server *chatd.Server
}

// CreateChat creates a new chat through the chatd server and returns
// the chat ID.
func (a *ChatdAdapter) CreateChat(ctx context.Context, opts CreateChatOptions) (uuid.UUID, error) {
	var modelConfigID uuid.UUID
	if opts.ModelConfigID.Valid {
		modelConfigID = opts.ModelConfigID.UUID
	}
	labels := database.StringMap{}
	for k, v := range opts.Labels {
		labels[k] = v
	}

	chat, err := a.Server.CreateChat(ctx, chatd.CreateOptions{
		OwnerID:       opts.OwnerID,
		Title:         opts.Title,
		ModelConfigID: modelConfigID,
		MCPServerIDs:  opts.MCPServerIDs,
		Labels:        labels,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(opts.Instructions),
		},
	})
	if err != nil {
		return uuid.Nil, err
	}
	return chat.ID, nil
}

// SendMessage appends a user message to an existing chat.
func (a *ChatdAdapter) SendMessage(ctx context.Context, chatID uuid.UUID, ownerID uuid.UUID, content string) error {
	_, err := a.Server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chatID,
		CreatedBy: ownerID,
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(content),
		},
	})
	return err
}
