package chatstate_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestLinkFilesUnavailable(t *testing.T) {
	t.Parallel()

	for name, dbErr := range map[string]*pq.Error{
		"missing": {
			Code:       pq.ErrorCode("23503"),
			Constraint: string(database.ForeignKeyChatFileLinksFileID),
		},
		"linked to another chat": {
			Code:       pq.ErrorCode("23505"),
			Constraint: string(database.UniqueChatFileLinksFileIDKey),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			store := dbmock.NewMockStore(ctrl)
			chatID := uuid.New()
			fileID := uuid.New()
			const maxLinks = codersdk.DefaultChatMaxAttachmentsPerChat + 7
			store.EXPECT().LinkChatFiles(gomock.Any(), database.LinkChatFilesParams{
				ChatID:       chatID,
				MaxFileLinks: maxLinks,
				FileIds:      []uuid.UUID{fileID},
			}).Return(int32(0), dbErr)

			err := chatstate.LinkFiles(testutil.Context(t, testutil.WaitShort), store, chatID, []uuid.UUID{fileID}, maxLinks)
			require.ErrorIs(t, err, chatstate.ErrChatFileUnavailable)
			require.ErrorIs(t, err, dbErr)
		})
	}
}

func TestLinkFilesRejectsNonPositiveCap(t *testing.T) {
	t.Parallel()

	for _, maxLinks := range []int{-1, 0} {
		t.Run(fmt.Sprintf("Max%d", maxLinks), func(t *testing.T) {
			t.Parallel()
			store := dbmock.NewMockStore(gomock.NewController(t))
			err := chatstate.LinkFiles(testutil.Context(t, testutil.WaitShort), store, uuid.New(), []uuid.UUID{uuid.New()}, maxLinks)
			require.ErrorContains(t, err, "max file links must be positive")
		})
	}
}
