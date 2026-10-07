package chatd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)

// A batch that hits a running chat fails without undoing earlier batches,
// and the chats those batches deleted are still returned.
func TestDeleteChatProjectWithoutEventsReturnsCommittedBatches(t *testing.T) {
	t.Parallel()

	db := dbmock.NewMockStore(gomock.NewController(t))
	projectID := uuid.New()
	idle := database.Chat{ID: uuid.New(), ProjectID: uuid.NullUUID{UUID: projectID, Valid: true}}
	running := database.LockChatProjectRootChatsForDeleteRow{
		ID:       uuid.New(),
		WorkerID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		RunnerID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
	}

	db.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(func(fn func(database.Store) error, _ *database.TxOptions) error {
		return fn(db)
	}).Times(2)
	db.EXPECT().GetChatProjectByIDForUpdate(gomock.Any(), projectID).Return(database.ChatProject{ID: projectID}, nil).Times(2)
	gomock.InOrder(
		db.EXPECT().LockChatProjectRootChatsForDelete(gomock.Any(), gomock.Any()).Return([]database.LockChatProjectRootChatsForDeleteRow{{ID: idle.ID}}, nil),
		db.EXPECT().LockChatProjectRootChatsForDelete(gomock.Any(), gomock.Any()).Return([]database.LockChatProjectRootChatsForDeleteRow{running}, nil),
	)
	db.EXPECT().LockSubChatsByRootIDsForDelete(gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
	db.EXPECT().GetChatsByIDs(gomock.Any(), []uuid.UUID{idle.ID}).Return([]database.Chat{idle}, nil)
	db.EXPECT().DeleteChatFamiliesByRootIDs(gomock.Any(), []uuid.UUID{idle.ID}).Return(nil)
	db.EXPECT().IsChatHeartbeatStale(gomock.Any(), gomock.Any()).Return(false, nil)

	deleted, err := DeleteChatProjectWithoutEvents(t.Context(), db, projectID, time.Minute)
	require.ErrorIs(t, err, ErrChatProjectHasRunningChats)
	require.Equal(t, []database.Chat{idle}, deleted)
}
