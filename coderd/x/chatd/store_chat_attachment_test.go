package chatd //nolint:testpackage

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
)

func TestStoreChatAttachment_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	tx := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	chatID := uuid.New()
	ownerID := uuid.New()
	workspaceID := uuid.New()
	orgID := uuid.New()
	fileID := uuid.New()
	chatSnapshot := database.Chat{
		ID:          chatID,
		OwnerID:     ownerID,
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
	}

	expectStoreChatAttachmentTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{ID: workspaceID, OrganizationID: orgID}, nil)
	tx.EXPECT().InsertChatFile(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatFileParams{})).DoAndReturn(
		func(_ context.Context, arg database.InsertChatFileParams) (database.InsertChatFileRow, error) {
			require.Equal(t, ownerID, arg.OwnerID)
			require.Equal(t, orgID, arg.OrganizationID)
			require.Equal(t, "build.log", arg.Name)
			require.Equal(t, "text/plain", arg.Mimetype)
			require.Equal(t, []byte("build output"), arg.Data)
			return database.InsertChatFileRow{ID: fileID}, nil
		},
	)
	tx.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{fileID},
	}).Return(int32(0), nil)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "build.log", "build.log", []byte("build output"))
	require.NoError(t, err)
	require.Equal(t, chattool.AttachmentMetadata{
		FileID:    fileID,
		MediaType: "text/plain",
		Name:      "build.log",
	}, attachment)
}

func TestStoreChatAttachment_UsesDetectNameForClassification(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	tx := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	chatID := uuid.New()
	ownerID := uuid.New()
	workspaceID := uuid.New()
	orgID := uuid.New()
	fileID := uuid.New()
	chatSnapshot := database.Chat{
		ID:          chatID,
		OwnerID:     ownerID,
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
	}

	expectStoreChatAttachmentTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{ID: workspaceID, OrganizationID: orgID}, nil)
	tx.EXPECT().InsertChatFile(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatFileParams{})).DoAndReturn(
		func(_ context.Context, arg database.InsertChatFileParams) (database.InsertChatFileRow, error) {
			require.Equal(t, "payload.txt", arg.Name)
			require.Equal(t, "application/json", arg.Mimetype)
			return database.InsertChatFileRow{ID: fileID}, nil
		},
	)
	tx.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{fileID},
	}).Return(int32(0), nil)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "payload.txt", "report.json", []byte(`{"ok":true}`))
	require.NoError(t, err)
	require.Equal(t, "payload.txt", attachment.Name)
	require.Equal(t, "application/json", attachment.MediaType)
}

func TestStoreChatAttachment_RejectsUnsupportedStoredFileTypeBeforeDBWork(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	chatSnapshot := database.Chat{
		ID:          uuid.New(),
		OwnerID:     uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}

	attachment, err := server.storeChatAttachment(
		context.Background(),
		chatSnapshot,
		"evil.svg",
		"evil.svg",
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`),
	)
	require.ErrorIs(t, err, chatfiles.ErrUnsupportedStoredFileType)
	require.ErrorContains(t, err, "image/svg+xml")
	require.Equal(t, chattool.AttachmentMetadata{}, attachment)
}

func TestStoreChatAttachment_NoWorkspace(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	attachment, err := server.storeChatAttachment(context.Background(), database.Chat{}, "build.log", "build.log", []byte("build output"))
	require.ErrorContains(t, err, "no workspace is associated")
	require.Equal(t, chattool.AttachmentMetadata{}, attachment)
}

func TestStoreChatAttachment_WorkspaceLookupError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	tx := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	workspaceID := uuid.New()
	chatSnapshot := database.Chat{
		ID:          uuid.New(),
		OwnerID:     uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
	}

	expectStoreChatAttachmentTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{}, context.DeadlineExceeded)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "build.log", "build.log", []byte("build output"))
	require.ErrorContains(t, err, "resolve workspace")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, chattool.AttachmentMetadata{}, attachment)
}

func TestStoreChatAttachment_InsertError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	tx := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	workspaceID := uuid.New()
	chatSnapshot := database.Chat{
		ID:          uuid.New(),
		OwnerID:     uuid.New(),
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
	}

	expectStoreChatAttachmentTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{ID: workspaceID, OrganizationID: uuid.New()}, nil)
	tx.EXPECT().InsertChatFile(gomock.Any(), gomock.Any()).Return(database.InsertChatFileRow{}, context.DeadlineExceeded)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "build.log", "build.log", []byte("build output"))
	require.ErrorContains(t, err, "insert chat file")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, chattool.AttachmentMetadata{}, attachment)
}

func TestStoreChatAttachment_StrictCapError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	tx := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	chatID := uuid.New()
	ownerID := uuid.New()
	workspaceID := uuid.New()
	orgID := uuid.New()
	fileID := uuid.New()
	chatSnapshot := database.Chat{
		ID:          chatID,
		OwnerID:     ownerID,
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
	}

	expectStoreChatAttachmentTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{ID: workspaceID, OrganizationID: orgID}, nil)
	tx.EXPECT().InsertChatFile(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatFileParams{})).Return(database.InsertChatFileRow{ID: fileID}, nil)
	tx.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{fileID},
	}).Return(int32(1), nil)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "build.log", "build.log", []byte("build output"))
	require.ErrorContains(t, err, "chat already has the maximum of 20 linked files")
	require.Equal(t, chattool.AttachmentMetadata{}, attachment)
}

func TestStoreChatAttachment_LinkError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	tx := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	chatID := uuid.New()
	ownerID := uuid.New()
	workspaceID := uuid.New()
	orgID := uuid.New()
	fileID := uuid.New()
	chatSnapshot := database.Chat{
		ID:          chatID,
		OwnerID:     ownerID,
		WorkspaceID: uuid.NullUUID{UUID: workspaceID, Valid: true},
	}

	expectStoreChatAttachmentTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{ID: workspaceID, OrganizationID: orgID}, nil)
	tx.EXPECT().InsertChatFile(gomock.Any(), gomock.Any()).Return(database.InsertChatFileRow{ID: fileID}, nil)
	tx.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{fileID},
	}).Return(int32(0), context.DeadlineExceeded)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "build.log", "build.log", []byte("build output"))
	require.ErrorContains(t, err, "link chat file")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, chattool.AttachmentMetadata{}, attachment)
}

func expectStoreChatAttachmentTx(t *testing.T, db, tx *dbmock.MockStore) {
	t.Helper()

	db.EXPECT().InTx(gomock.Any(), gomock.AssignableToTypeOf(&database.TxOptions{})).DoAndReturn(
		func(fn func(database.Store) error, opts *database.TxOptions) error {
			require.NotNil(t, opts)
			require.Equal(t, "store_chat_attachment", opts.TxIdentifier)
			return fn(tx)
		},
	)
}
