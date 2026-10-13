package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
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

	setup := func(t *testing.T) (context.Context, *workerTestFixture, database.Chat, *recordingPubsub, *Server) {
		ctx := testutil.Context(t, testutil.WaitLong)
		fixture := newWorkerTestFixture(t)
		chat := dbgen.Chat(t, fixture.db, database.Chat{
			OrganizationID:    fixture.org.ID,
			OwnerID:           fixture.user.ID,
			LastModelConfigID: fixture.model.ID,
		})
		recorder := newRecordingPubsub(fixture.pubsub)
		server := &Server{db: fixture.db, pubsub: recorder, logger: slogtest.Make(t, nil)}
		return ctx, fixture, chat, recorder, server
	}

	t.Run("ReloadsAndPublishesAfterStateUpdate", func(t *testing.T) {
		t.Parallel()
		ctx, fixture, chat, recorder, server := setup(t)

		updated, err := server.updateChat(ctx, chat.ID, "test", func(tx *chatstate.Tx, _ database.Store, _ *database.Chat) error {
			_, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
			return err
		})
		require.NoError(t, err)

		stored, err := fixture.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, stored, updated)
		require.Greater(t, updated.SnapshotVersion, chat.SnapshotVersion)
		require.Equal(t, database.ChatStatusRunning, updated.Status)

		recorder.mu.Lock()
		events := append([]publishedEvent(nil), recorder.events...)
		recorder.mu.Unlock()
		stateIndex, watchIndex := -1, -1
		for i, event := range events {
			switch event.channel {
			case coderdpubsub.ChatStateUpdateChannel(chat.ID):
				stateIndex = i
			case coderdpubsub.ChatWatchEventChannel(fixture.user.ID):
				watchIndex = i
				var payload codersdk.ChatWatchEvent
				require.NoError(t, json.Unmarshal(event.payload, &payload))
				require.Equal(t, codersdk.ChatWatchEventKindStatusChange, payload.Kind)
				require.Equal(t, updated.ID, payload.Chat.ID)
				require.Equal(t, codersdk.ChatStatus(updated.Status), payload.Chat.Status)
			}
		}
		require.NotEqual(t, -1, stateIndex)
		require.Greater(t, watchIndex, stateIndex)
	})

	t.Run("TransitionRowSkipsReload", func(t *testing.T) {
		t.Parallel()
		ctx, _, chat, recorder, server := setup(t)
		sentinel := database.Chat{ID: chat.ID, OwnerID: chat.OwnerID, Title: "from transition"}

		updated, err := server.updateChat(ctx, chat.ID, "test", func(tx *chatstate.Tx, _ database.Store, updated *database.Chat) error {
			if _, err := tx.RequestCompaction(chatstate.RequestCompactionInput{}); err != nil {
				return err
			}
			*updated = sentinel
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, sentinel, updated)
		events := recorder.watchEvents(t)
		require.Len(t, events, 1)
		require.Equal(t, sentinel.Title, events[0].Chat.Title)
	})

	t.Run("FailureRollsBackAndPublishesNothing", func(t *testing.T) {
		t.Parallel()
		ctx, fixture, chat, recorder, server := setup(t)
		wantErr := xerrors.New("transition failed")

		_, err := server.updateChat(ctx, chat.ID, "test", func(*chatstate.Tx, database.Store, *database.Chat) error {
			return wantErr
		})
		require.ErrorIs(t, err, wantErr)

		stored, err := fixture.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.SnapshotVersion, stored.SnapshotVersion)
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		require.Empty(t, recorder.events)
	})
}

func TestChatMutationInputValidation(t *testing.T) {
	t.Parallel()

	server := &Server{}
	text := []codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")}
	for _, tt := range []struct {
		name string
		call func() error
		want string
	}{
		{name: "SendMissingContent", call: func() error {
			_, err := server.SendMessage(context.Background(), SendMessageOptions{ChatID: uuid.New()})
			return err
		}, want: "content is required"},
		{name: "SendInvalidBusyBehavior", call: func() error {
			_, err := server.SendMessage(context.Background(), SendMessageOptions{ChatID: uuid.New(), Content: text, BusyBehavior: "invalid"})
			return err
		}, want: `invalid busy behavior "invalid"`},
		{name: "EditMissingMessageID", call: func() error {
			_, err := server.EditMessage(context.Background(), EditMessageOptions{ChatID: uuid.New()})
			return err
		}, want: "edited_message_id is required"},
		{name: "EditMissingContent", call: func() error {
			_, err := server.EditMessage(context.Background(), EditMessageOptions{ChatID: uuid.New(), EditedMessageID: 1})
			return err
		}, want: "content is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.EqualError(t, tt.call(), tt.want)
		})
	}
}

func TestTranslateToolResultValidationError(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		cause error
		want  string
	}{
		{cause: chatstate.ErrToolResultDuplicate, want: `Duplicate tool_call_id in results.: Duplicate tool call ID "call-1".`},
		{cause: chatstate.ErrToolResultMissing, want: `Missing tool result.: Missing result for tool call "call-1".`},
		{cause: chatstate.ErrToolResultUnexpected, want: `Unexpected tool result.: No pending tool call with ID "call-1".`},
		{cause: chatstate.ErrToolResultInvalidJSON, want: `Tool result output must be valid JSON.: Output for tool call "call-1" is not valid JSON.`},
	} {
		t.Run(tt.cause.Error(), func(t *testing.T) {
			t.Parallel()
			err := translateToolResultValidationError(&chatstate.ToolResultValidationError{Cause: tt.cause, ToolCallID: "call-1"})
			var translated *ToolResultValidationError
			require.ErrorAs(t, err, &translated)
			require.EqualError(t, err, tt.want)
		})
	}
	unrelated := xerrors.New("unrelated")
	require.Same(t, unrelated, translateToolResultValidationError(unrelated))
}
