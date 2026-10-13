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

func TestResolveProjectInstructions(t *testing.T) {
	t.Parallel()

	instructionsRow := func(projectID uuid.UUID, text string) database.GetChatProjectInstructionsByProjectIDRow {
		return database.GetChatProjectInstructionsByProjectIDRow{
			ChatProjectInstruction: database.ChatProjectInstruction{ProjectID: projectID, Instructions: text},
		}
	}
	newServer := func(t *testing.T, db database.Store) *Server {
		return &Server{
			db:          db,
			logger:      slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			experiments: codersdk.ExperimentsKnown,
		}
	}

	t.Run("Project", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectInstructionsByProjectID(gomock.Any(), projectID).
			Return(instructionsRow(projectID, "Reply\u200b in French.\n"), nil)
		server := newServer(t, db)
		block := server.resolveProjectInstructions(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			ProjectID: uuid.NullUUID{UUID: projectID, Valid: true},
		})
		require.Contains(t, block, "<project-instructions>\nReply in French.\n</project-instructions>")
	})

	t.Run("OutsideProject", func(t *testing.T) {
		t.Parallel()
		// The strict mock fails the test on any lookup.
		server := newServer(t, dbmock.NewMockStore(gomock.NewController(t)))
		require.Empty(t, server.resolveProjectInstructions(t.Context(), server.logger, database.Chat{ID: uuid.New()}))
	})

	t.Run("Unset", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectInstructionsByProjectID(gomock.Any(), projectID).
			Return(database.GetChatProjectInstructionsByProjectIDRow{}, sql.ErrNoRows)
		server := newServer(t, db)
		require.Empty(t, server.resolveProjectInstructions(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			ProjectID: uuid.NullUUID{UUID: projectID, Valid: true},
		}))
	})

	t.Run("SubagentInheritsRootProject", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		rootID := uuid.New()
		db.EXPECT().GetChatByID(gomock.Any(), rootID).
			Return(database.Chat{ID: rootID, ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}}, nil)
		db.EXPECT().GetChatProjectInstructionsByProjectID(gomock.Any(), projectID).
			Return(instructionsRow(projectID, "Reply in French."), nil)
		server := newServer(t, db)
		block := server.resolveProjectInstructions(t.Context(), server.logger, database.Chat{
			ID:           uuid.New(),
			ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			RootChatID:   uuid.NullUUID{UUID: rootID, Valid: true},
		})
		require.Contains(t, block, "Reply in French.")
	})

	t.Run("LookupFailureFailsOpen", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectInstructionsByProjectID(gomock.Any(), projectID).
			Return(database.GetChatProjectInstructionsByProjectIDRow{}, xerrors.New("connection reset"))
		server := newServer(t, db)
		require.Empty(t, server.resolveProjectInstructions(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			ProjectID: uuid.NullUUID{UUID: projectID, Valid: true},
		}))
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()
		server := newServer(t, dbmock.NewMockStore(gomock.NewController(t)))
		server.experiments = nil
		require.Empty(t, server.resolveProjectInstructions(t.Context(), server.logger, database.Chat{
			ID:        uuid.New(),
			ProjectID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		}))
	})
}
