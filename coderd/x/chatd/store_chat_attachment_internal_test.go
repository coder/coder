package chatd

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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

	expectStoreChatAttachmentInTx(t, db, tx)
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

	expectStoreChatAttachmentInTx(t, db, tx)
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

func TestStoreChatAttachment_AllowsUnsupportedPromptInputType(t *testing.T) {
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
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)

	expectStoreChatAttachmentInTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{ID: workspaceID, OrganizationID: orgID}, nil)
	tx.EXPECT().InsertChatFile(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatFileParams{})).DoAndReturn(
		func(_ context.Context, arg database.InsertChatFileParams) (database.InsertChatFileRow, error) {
			require.Equal(t, ownerID, arg.OwnerID)
			require.Equal(t, orgID, arg.OrganizationID)
			require.Equal(t, "evil.svg", arg.Name)
			require.Equal(t, "image/svg+xml", arg.Mimetype)
			require.Equal(t, data, arg.Data)
			return database.InsertChatFileRow{ID: fileID}, nil
		},
	)
	tx.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{fileID},
	}).Return(int32(0), nil)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "evil.svg", "evil.svg", data)
	require.NoError(t, err)
	require.Equal(t, chattool.AttachmentMetadata{
		FileID:    fileID,
		MediaType: "image/svg+xml",
		Name:      "evil.svg",
	}, attachment)
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

	expectStoreChatAttachmentInTx(t, db, tx)
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

	expectStoreChatAttachmentInTx(t, db, tx)
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

	expectStoreChatAttachmentInTx(t, db, tx)
	tx.EXPECT().GetWorkspaceByID(gomock.Any(), workspaceID).Return(database.Workspace{ID: workspaceID, OrganizationID: orgID}, nil)
	tx.EXPECT().InsertChatFile(gomock.Any(), gomock.AssignableToTypeOf(database.InsertChatFileParams{})).Return(database.InsertChatFileRow{ID: fileID}, nil)
	tx.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{fileID},
	}).Return(int32(1), nil)

	attachment, err := server.storeChatAttachment(context.Background(), chatSnapshot, "build.log", "build.log", []byte("build output"))
	require.ErrorContains(t, err, fmt.Sprintf("chat already has the maximum of %d linked files", codersdk.MaxChatFileIDs))
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

	expectStoreChatAttachmentInTx(t, db, tx)
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

func TestStoreChatAttachment_SerializesCapCheck(t *testing.T) {
	t.Parallel()

	ctx := chatdTestContext(t)
	db, _, rawDB := dbtestutil.NewDBWithSQLDB(t)
	user, _, model := seedInternalChatDeps(t, db)
	workspace, _, _ := seedWorkspaceBinding(t, db, user.ID)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    workspace.OrganizationID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: workspace.ID, Valid: true},
		LastModelConfigID: model.ID,
	})

	for i := range codersdk.MaxChatFileIDs - 1 {
		insertLinkedChatFile(
			ctx,
			t,
			db,
			chat.ID,
			user.ID,
			workspace.OrganizationID,
			fmt.Sprintf("existing-%02d.txt", i),
			"text/plain",
			[]byte("existing"),
		)
	}

	lockKey := int64(uuid.New().ID())
	_, err := rawDB.ExecContext(ctx, fmt.Sprintf(`
CREATE FUNCTION test_block_chat_file_link() RETURNS trigger AS $$
BEGIN
	PERFORM pg_advisory_xact_lock(%d);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER test_block_chat_file_link
BEFORE INSERT ON chat_file_links
FOR EACH ROW EXECUTE FUNCTION test_block_chat_file_link();
`, lockKey))
	require.NoError(t, err)

	barrierConn, err := rawDB.Conn(ctx)
	require.NoError(t, err)
	barrierReleased := false
	t.Cleanup(func() {
		if !barrierReleased {
			_, _ = barrierConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
		}
		_ = barrierConn.Close()
	})
	_, err = barrierConn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey)
	require.NoError(t, err)

	server := &Server{db: db}
	attachmentResults := make(chan error, 2)
	for i := range 2 {
		go func() {
			_, err := server.storeChatAttachment(
				ctx,
				chat,
				fmt.Sprintf("concurrent-%d.txt", i),
				"attachment.txt",
				[]byte("attachment"),
			)
			attachmentResults <- err
		}()
	}

	var linkWaits, chatLockWaits int
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		err := rawDB.QueryRowContext(ctx, `
SELECT
	COUNT(*) FILTER (
		WHERE query LIKE '%-- name: LinkChatFilesAfterLock%'
		AND wait_event = 'advisory'
	),
	COUNT(*) FILTER (
		WHERE query LIKE '%-- name: LockChatByID%'
		AND wait_event_type = 'Lock'
	)
FROM pg_stat_activity
WHERE datname = current_database()
	AND pid <> pg_backend_pid()
`).Scan(&linkWaits, &chatLockWaits)
		return err == nil && linkWaits >= 1 && linkWaits+chatLockWaits == 2
	}, testutil.IntervalFast, "wait for both attachment transactions")
	require.NoError(t, ctx.Err(), "waiting for attachment transactions")

	_, err = barrierConn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey)
	barrierReleased = true
	require.NoError(t, err)

	var successes, capRejections int
	for range 2 {
		select {
		case err := <-attachmentResults:
			if err == nil {
				successes++
				continue
			}
			require.ErrorContains(t, err, fmt.Sprintf("chat already has the maximum of %d linked files", codersdk.MaxChatFileIDs))
			capRejections++
		case <-ctx.Done():
			require.Failf(t, "attachment store did not finish", "context ended: %v", ctx.Err())
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, capRejections)

	files, err := db.GetChatFileMetadataByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, files, codersdk.MaxChatFileIDs)

	var fileCount int
	require.NoError(t, rawDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM chat_files").Scan(&fileCount))
	require.Equal(t, codersdk.MaxChatFileIDs, fileCount)
}

func expectStoreChatAttachmentInTx(t *testing.T, db, tx *dbmock.MockStore) {
	t.Helper()

	db.EXPECT().InTx(gomock.Any(), gomock.AssignableToTypeOf(&database.TxOptions{})).DoAndReturn(
		func(fn func(database.Store) error, opts *database.TxOptions) error {
			require.NotNil(t, opts)
			require.Equal(t, "store_chat_attachment", opts.TxIdentifier)
			return fn(tx)
		},
	)
}
