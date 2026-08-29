package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

type chatMutator struct {
	server *Server
}

func (m *chatMutator) DeleteQueued(ctx context.Context, chatID uuid.UUID, queuedMessageID int64) error {
	if chatID == uuid.Nil {
		return xerrors.New("chat_id is required")
	}

	machine := m.server.newChatMachine(chatID)
	return machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
			QueuedMessageID: queuedMessageID,
		})
		return err
	})
}
