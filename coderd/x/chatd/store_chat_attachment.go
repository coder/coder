package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
)

func (p *Server) newStoreChatAttachmentFunc(workspaceCtx *turnWorkspaceContext) chattool.StoreFileFunc {
	return func(
		ctx context.Context,
		name string,
		detectName string,
		data []byte,
	) (chattool.AttachmentMetadata, error) {
		workspaceCtx.chatStateMu.Lock()
		chatSnapshot := *workspaceCtx.currentChat
		workspaceCtx.chatStateMu.Unlock()

		return p.storeChatAttachment(ctx, chatSnapshot, name, detectName, data)
	}
}

func (p *Server) storeChatAttachment(
	ctx context.Context,
	chatSnapshot database.Chat,
	name string,
	detectName string,
	data []byte,
) (chattool.AttachmentMetadata, error) {
	if !chatSnapshot.WorkspaceID.Valid {
		return chattool.AttachmentMetadata{}, xerrors.New("no workspace is associated with this chat. Use the create_workspace tool to create one")
	}

	storedName, mediaType, err := chatfiles.PrepareStoredFile(name, detectName, data)
	if err != nil {
		return chattool.AttachmentMetadata{}, err
	}

	// Insert and link in one transaction so a cap rejection or linking
	// failure does not leave behind an unlinked chat file row.
	var attachment chattool.AttachmentMetadata
	err = p.db.InTx(func(tx database.Store) error {
		ws, err := tx.GetWorkspaceByID(ctx, chatSnapshot.WorkspaceID.UUID)
		if err != nil {
			return xerrors.Errorf("resolve workspace: %w", err)
		}

		attachment, err = storeLinkedChatFileTx(
			ctx,
			tx,
			chatSnapshot.ID,
			chatSnapshot.OwnerID,
			ws.OrganizationID,
			storedName,
			mediaType,
			data,
		)
		return err
	}, database.DefaultTXOptions().WithID("store_chat_attachment"))
	if err != nil {
		return chattool.AttachmentMetadata{}, err
	}
	return attachment, nil
}

func storeLinkedChatFileTx(
	ctx context.Context,
	tx database.Store,
	chatID uuid.UUID,
	ownerID uuid.UUID,
	organizationID uuid.UUID,
	name string,
	mediaType string,
	data []byte,
) (chattool.AttachmentMetadata, error) {
	row, err := tx.InsertChatFile(ctx, database.InsertChatFileParams{
		OwnerID:        ownerID,
		OrganizationID: organizationID,
		Name:           name,
		Mimetype:       mediaType,
		Data:           data,
	})
	if err != nil {
		return chattool.AttachmentMetadata{}, xerrors.Errorf("insert chat file: %w", err)
	}

	rejected, err := tx.LinkChatFiles(ctx, database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{row.ID},
	})
	if err != nil {
		return chattool.AttachmentMetadata{}, xerrors.Errorf("link chat file: %w", err)
	}
	if rejected > 0 {
		return chattool.AttachmentMetadata{}, xerrors.Errorf("chat already has the maximum of %d linked files", codersdk.MaxChatFileIDs)
	}

	return chattool.AttachmentMetadata{
		FileID:    row.ID,
		MediaType: mediaType,
		Name:      name,
	}, nil
}
