package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

// The worker cancels a mid-turn generation once the delete has removed
// the chat's heartbeat row.
func TestDeleteChatProjectStopsRunningChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	starter := newBlockingTaskStarter(false)
	chat := f.createRunningChat(t)
	project := dbgen.ChatProject(t, f.db, database.ChatProject{OrganizationID: f.org.ID, OwnerID: f.user.ID})
	_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET project_id = $2 WHERE id = $1", chat.ID, project.ID)
	require.NoError(t, err)
	worker := startWorker(t, testOptions(t, f, starter))
	generation := starter.waitCall(t, taskKindGeneration, chat.ID)

	deleted, err := DeleteChatProjectWithoutEvents(ctx, testutil.Logger(t), f.db, project.ID)
	require.NoError(t, err)
	require.Len(t, deleted, 1)

	worker.mu.Lock()
	manager := worker.manager
	worker.mu.Unlock()
	require.NoError(t, manager.heartbeatOnce(ctx))
	select {
	case <-generation.ctx.Done():
	case <-ctx.Done():
		t.Fatal("the runner of a deleted chat must stop generating")
	}
}

func TestDeleteChatProjectWithoutEventsDeadlockRetry(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	rootID, subID := uuid.New(), uuid.New()
	deadlock := &pq.Error{Code: "40P01", Message: "deadlock detected"}
	// InChatProjectDeleteTx requires messages to be deleted before the chats.
	expectAttempt := func(db *dbmock.MockStore, familiesErr error) {
		db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
			return fn(db)
		})
		db.EXPECT().GetChatProjectByIDForUpdate(gomock.Any(), projectID).Return(database.ChatProject{ID: projectID}, nil)
		db.EXPECT().LockChatProjectRootChatsForDelete(gomock.Any(), projectID).Return([]uuid.UUID{rootID}, nil)
		db.EXPECT().LockSubChatsByRootIDsForDelete(gomock.Any(), []uuid.UUID{rootID}).Return([]uuid.UUID{subID}, nil)
		db.EXPECT().GetChatsByIDs(gomock.Any(), []uuid.UUID{rootID, subID}).Return([]database.Chat{{ID: rootID}, {ID: subID}}, nil)
		gomock.InOrder(
			db.EXPECT().DeleteChatMessagesByChatIDs(gomock.Any(), []uuid.UUID{rootID, subID}).Return(nil),
			db.EXPECT().DeleteChatFamiliesByRootIDs(gomock.Any(), []uuid.UUID{rootID}).Return(familiesErr),
		)
		if familiesErr == nil {
			db.EXPECT().DeleteChatProjectByID(gomock.Any(), projectID).Return(nil)
		}
	}

	t.Run("RetriesDeadlock", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		expectAttempt(db, deadlock)
		expectAttempt(db, nil)

		deleted, err := DeleteChatProjectWithoutEvents(t.Context(), testutil.Logger(t), db, projectID)
		require.NoError(t, err)
		require.Len(t, deleted, 2)
	})

	t.Run("GivesUpAfterAttempts", func(t *testing.T) {
		t.Parallel()
		db := dbmock.NewMockStore(gomock.NewController(t))
		for range chatProjectDeleteAttempts {
			expectAttempt(db, deadlock)
		}

		_, err := DeleteChatProjectWithoutEvents(t.Context(), testutil.Logger(t), db, projectID)
		require.ErrorIs(t, err, ErrChatProjectDeleteConflict)
		require.True(t, database.IsDeadlockError(err))
		require.ErrorContains(t, err, fmt.Sprintf("failed after %d attempts", chatProjectDeleteAttempts))
	})
}
