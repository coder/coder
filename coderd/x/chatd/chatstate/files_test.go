package chatstate_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

func TestLinkFilesUnavailable(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	fileID := uuid.New()
	foreignKeyErr := &pq.Error{
		Code:       pq.ErrorCode("23503"),
		Constraint: string(database.ForeignKeyChatFileLinksFileID),
	}
	store.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
		ChatID:       chatID,
		MaxFileLinks: int32(codersdk.MaxChatFileIDs),
		FileIds:      []uuid.UUID{fileID},
	}).Return(int32(0), foreignKeyErr)

	err := chatstate.LinkFiles(context.Background(), store, chatID, []uuid.UUID{fileID})
	require.ErrorIs(t, err, chatstate.ErrChatFileUnavailable)
	require.ErrorIs(t, err, foreignKeyErr)
}
