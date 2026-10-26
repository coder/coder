package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUpdateChat(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*workerTestFixture, database.Chat, *recordingPubsub, *Server) {
		fixture := newWorkerTestFixture(t)
		chat := dbgen.Chat(t, fixture.db, database.Chat{
			OrganizationID:    fixture.org.ID,
			OwnerID:           fixture.user.ID,
			LastModelConfigID: fixture.model.ID,
		})
		recorder := newRecordingPubsub(fixture.pubsub)
		return fixture, chat, recorder, &Server{db: fixture.db, pubsub: recorder, logger: slogtest.Make(t, nil)}
	}
	requestCompaction := func(tx *chatstate.Tx, _ database.Store, _ *database.Chat) error {
		_, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
		return err
	}

	t.Run("ReloadsAndPublishesAfterStateUpdate", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		fixture, chat, recorder, server := setup(t)

		updated, err := server.updateChat(ctx, chat.ID, "test", requestCompaction)
		require.NoError(t, err)
		stored, err := fixture.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, stored, updated)
		require.Greater(t, updated.SnapshotVersion, chat.SnapshotVersion)
		require.Equal(t, database.ChatStatusRunning, updated.Status)

		events := recorder.watchEvents(t)
		require.Len(t, events, 1)
		require.Equal(t, codersdk.ChatWatchEventKindStatusChange, events[0].Kind)
		require.Equal(t, codersdk.ChatStatus(updated.Status), events[0].Chat.Status)
		// The watch event follows the state update the transaction published.
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		stateIndex, watchIndex := -1, -1
		for i, event := range recorder.events {
			switch event.channel {
			case coderdpubsub.ChatStateUpdateChannel(chat.ID):
				stateIndex = i
			case coderdpubsub.ChatWatchEventChannel(chat.OwnerID):
				watchIndex = i
			}
		}
		require.NotEqual(t, -1, stateIndex)
		require.Greater(t, watchIndex, stateIndex)
	})

	t.Run("TransitionRowSkipsReload", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, chat, recorder, server := setup(t)
		sentinel := database.Chat{ID: chat.ID, OwnerID: chat.OwnerID, Title: "from transition"}

		updated, err := server.updateChat(ctx, chat.ID, "test", func(tx *chatstate.Tx, store database.Store, updated *database.Chat) error {
			*updated = sentinel
			return requestCompaction(tx, store, nil)
		})
		require.NoError(t, err)
		require.Equal(t, sentinel, updated)
		events := recorder.watchEvents(t)
		require.Len(t, events, 1)
		require.Equal(t, sentinel.Title, events[0].Chat.Title)
	})

	t.Run("FailureRollsBackAndPublishesNothing", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		fixture, chat, recorder, server := setup(t)
		wantErr := xerrors.New("transition failed")

		_, err := server.updateChat(ctx, chat.ID, "test", func(*chatstate.Tx, database.Store, *database.Chat) error {
			return wantErr
		})
		require.ErrorIs(t, err, wantErr)
		stored, err := fixture.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.SnapshotVersion, stored.SnapshotVersion)
		require.Empty(t, recorder.watchEvents(t))
	})
}
