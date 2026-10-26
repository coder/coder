package chatd

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
)

func newProjectTestServer(t *testing.T, db database.Store) *Server {
	return &Server{
		db:          db,
		logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		experiments: codersdk.ExperimentsKnown,
	}
}

func TestResolveUsableChatProjectID(t *testing.T) {
	t.Parallel()

	t.Run("ProjectOwner", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		ownerID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{ID: projectID, OwnerID: ownerID}, nil)
		server := newProjectTestServer(t, db)
		got := server.resolveUsableChatProjectID(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			OwnerID:   ownerID,
			ProjectID: uuid.NullUUID{UUID: projectID, Valid: true},
		})
		require.Equal(t, uuid.NullUUID{UUID: projectID, Valid: true}, got)
	})

	t.Run("OutsideProject", func(t *testing.T) {
		t.Parallel()
		// The strict mock fails the test on any lookup.
		server := newProjectTestServer(t, dbmock.NewMockStore(gomock.NewController(t)))
		require.False(t, server.resolveUsableChatProjectID(t.Context(), server.logger, database.Chat{ID: uuid.New()}).Valid)
	})

	t.Run("SubagentInheritsRootProject", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		ownerID := uuid.New()
		rootID := uuid.New()
		db.EXPECT().GetChatByID(gomock.Any(), rootID).
			Return(database.Chat{ID: rootID, ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}}, nil)
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{ID: projectID, OwnerID: ownerID}, nil)
		server := newProjectTestServer(t, db)
		got := server.resolveUsableChatProjectID(t.Context(), server.logger, database.Chat{
			ID:           uuid.New(),
			OwnerID:      ownerID,
			ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			RootChatID:   uuid.NullUUID{UUID: rootID, Valid: true},
		})
		require.Equal(t, uuid.NullUUID{UUID: projectID, Valid: true}, got)
	})

	t.Run("RevokedFromChatOwner", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		ownerID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).
			Return(database.ChatProject{ID: projectID, OwnerID: uuid.New()}, nil)
		db.EXPECT().IsChatProjectUsableByUserID(gomock.Any(), database.IsChatProjectUsableByUserIDParams{
			ProjectID: projectID,
			UserID:    ownerID,
		}).Return(false, nil)
		server := newProjectTestServer(t, db)
		require.False(t, server.resolveUsableChatProjectID(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			OwnerID:   ownerID,
			ProjectID: uuid.NullUUID{UUID: projectID, Valid: true},
		}).Valid)
	})

	t.Run("SharedWithChatOwner", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		project := database.ChatProject{ID: uuid.New(), OwnerID: uuid.New()}
		chat := database.Chat{ID: uuid.New(), OwnerID: uuid.New(), ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true}}
		db.EXPECT().GetChatProjectByID(gomock.Any(), project.ID).Return(project, nil)
		db.EXPECT().IsChatProjectUsableByUserID(gomock.Any(), database.IsChatProjectUsableByUserIDParams{
			ProjectID: project.ID,
			UserID:    chat.OwnerID,
		}).Return(true, nil)
		server := newProjectTestServer(t, db)
		require.Equal(t, chat.ProjectID, server.resolveUsableChatProjectID(t.Context(), server.logger, chat))
	})

	t.Run("ProjectLookupFailure", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{}, xerrors.New("connection reset"))
		server := newProjectTestServer(t, db)
		require.False(t, server.resolveUsableChatProjectID(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			OwnerID:   uuid.New(),
			ProjectID: uuid.NullUUID{UUID: projectID, Valid: true},
		}).Valid)
	})

	t.Run("AccessCheckFailure", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		project := database.ChatProject{ID: uuid.New(), OwnerID: uuid.New()}
		db.EXPECT().GetChatProjectByID(gomock.Any(), project.ID).Return(project, nil)
		db.EXPECT().IsChatProjectUsableByUserID(gomock.Any(), gomock.Any()).Return(false, xerrors.New("connection reset"))
		server := newProjectTestServer(t, db)
		require.False(t, server.resolveUsableChatProjectID(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			OwnerID:   uuid.New(),
			ProjectID: uuid.NullUUID{UUID: project.ID, Valid: true},
		}).Valid)
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()
		server := newProjectTestServer(t, dbmock.NewMockStore(gomock.NewController(t)))
		server.experiments = nil
		require.False(t, server.resolveUsableChatProjectID(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			ProjectID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		}).Valid)
	})
}

func TestLoadProjectInstructions(t *testing.T) {
	t.Parallel()

	t.Run("Formatted", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectInstructionsByProjectID(gomock.Any(), projectID).
			Return(database.GetChatProjectInstructionsByProjectIDRow{
				ChatProjectInstruction: database.ChatProjectInstruction{ProjectID: projectID, Instructions: "Reply\u200b in French.\n"},
			}, nil)
		server := newProjectTestServer(t, db)
		block := server.loadProjectInstructions(t.Context(), server.logger, projectID)
		require.Contains(t, block, "<project-instructions>\nReply in French.\n</project-instructions>")
	})

	t.Run("Unset", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectInstructionsByProjectID(gomock.Any(), projectID).
			Return(database.GetChatProjectInstructionsByProjectIDRow{}, sql.ErrNoRows)
		server := newProjectTestServer(t, db)
		require.Empty(t, server.loadProjectInstructions(t.Context(), server.logger, projectID))
	})

	t.Run("LookupFailure", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectInstructionsByProjectID(gomock.Any(), projectID).
			Return(database.GetChatProjectInstructionsByProjectIDRow{}, xerrors.New("connection reset"))
		server := newProjectTestServer(t, db)
		require.Empty(t, server.loadProjectInstructions(t.Context(), server.logger, projectID))
	})
}
