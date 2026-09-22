package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolveMemoryScope(t *testing.T) {
	t.Parallel()

	t.Run("Project", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{Name: "platform"}, nil)
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, scope, status := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}})
		require.Equal(t, memoryScopeAvailable, status)
		require.Equal(t, "platform", scope.Label)
	})

	t.Run("OutsideProject", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, status := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New()})
		require.Equal(t, memoryScopeUnavailable, status)
	})

	t.Run("Subagent", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, status := server.resolveMemoryScope(t.Context(), database.Chat{
			ID:           uuid.New(),
			ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			ProjectID:    uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
		require.Equal(t, memoryScopeUnavailable, status)
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		server := &Server{db: db, logger: slogtest.Make(t, nil)}
		_, _, status := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: uuid.New(), Valid: true}})
		require.Equal(t, memoryScopeUnavailable, status)
	})

	t.Run("ProjectLookupFailure", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		projectID := uuid.New()
		db.EXPECT().GetChatProjectByID(gomock.Any(), projectID).Return(database.ChatProject{}, xerrors.New("connection reset"))
		server := &Server{db: db, logger: slogtest.Make(t, nil), experiments: codersdk.ExperimentsKnown}
		_, _, status := server.resolveMemoryScope(t.Context(), database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}})
		require.Equal(t, memoryScopeUnavailable, status)
	})
}

func TestPlanModeKeepsMemoryTools(t *testing.T) {
	t.Parallel()

	for _, name := range []string{chattool.ReadMemoryToolName, chattool.SaveMemoryToolName, chattool.DeleteMemoryToolName} {
		require.True(t, builtinPlanToolAllowed(name, true), name)
		require.False(t, builtinPlanToolAllowed(name, false), "%s is a root-chat tool", name)
	}
}
