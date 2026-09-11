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

func TestChatMutator(t *testing.T) {
	t.Parallel()

	t.Run("update", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name string
			fail bool
		}{{name: "success"}, {name: "failure", fail: true}} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				fixture := newWorkerTestFixture(t)
				chat := dbgen.Chat(t, fixture.db, database.Chat{
					OrganizationID:    fixture.org.ID,
					OwnerID:           fixture.user.ID,
					LastModelConfigID: fixture.model.ID,
				})
				recorder := newRecordingPubsub(fixture.pubsub)
				mutator := chatMutator{server: &Server{db: fixture.db, pubsub: recorder, logger: slogtest.Make(t, nil)}}
				wantErr := xerrors.New("transition failed")

				updated, err := mutator.update(ctx, chat.ID, "test", func(tx *chatstate.Tx, _ database.Store, _ *database.Chat) error {
					if tt.fail {
						return wantErr
					}
					_, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
					return err
				})
				stored, loadErr := fixture.db.GetChatByID(ctx, chat.ID)
				require.NoError(t, loadErr)
				recorder.mu.Lock()
				events := append([]publishedEvent(nil), recorder.events...)
				recorder.mu.Unlock()
				if tt.fail {
					require.ErrorIs(t, err, wantErr)
					require.Equal(t, chat.SnapshotVersion, stored.SnapshotVersion)
					require.Empty(t, events)
					return
				}

				require.NoError(t, err)
				require.Equal(t, stored, updated)
				require.Greater(t, updated.SnapshotVersion, chat.SnapshotVersion)
				require.Equal(t, database.ChatStatusRunning, updated.Status)
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
		}
	})

	t.Run("validation", func(t *testing.T) {
		t.Parallel()
		mutator := chatMutator{server: &Server{}}
		child := database.Chat{ID: uuid.New(), ParentChatID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
		for _, tt := range []struct {
			name string
			call func() error
			want string
		}{
			{name: "send content", call: func() error {
				_, err := mutator.SendMessage(context.Background(), SendMessageOptions{ChatID: uuid.New()})
				return err
			}, want: "content is required"},
			{name: "send busy behavior", call: func() error {
				_, err := mutator.SendMessage(context.Background(), SendMessageOptions{ChatID: uuid.New(), Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")}, BusyBehavior: "invalid"})
				return err
			}, want: "invalid busy behavior \"invalid\""},
			{name: "edit message ID", call: func() error {
				_, err := mutator.EditMessage(context.Background(), EditMessageOptions{ChatID: uuid.New()})
				return err
			}, want: "edited_message_id is required"},
			{name: "edit content", call: func() error {
				_, err := mutator.EditMessage(context.Background(), EditMessageOptions{ChatID: uuid.New(), EditedMessageID: 1})
				return err
			}, want: "content is required"},
			{name: "archive child", call: func() error { return mutator.ArchiveChat(context.Background(), child) }, want: ErrArchiveRequiresRootChat.Error()},
			{name: "unarchive child", call: func() error { return mutator.UnarchiveChat(context.Background(), child) }, want: ErrArchiveRequiresRootChat.Error()},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				require.EqualError(t, tt.call(), tt.want)
			})
		}
	})

	t.Run("tool result error translation", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			cause error
			want  string
		}{
			{cause: chatstate.ErrToolResultDuplicate, want: "Duplicate tool_call_id in results.: Duplicate tool call ID \"call-1\"."},
			{cause: chatstate.ErrToolResultMissing, want: "Missing tool result.: Missing result for tool call \"call-1\"."},
			{cause: chatstate.ErrToolResultUnexpected, want: "Unexpected tool result.: No pending tool call with ID \"call-1\"."},
			{cause: chatstate.ErrToolResultInvalidJSON, want: "Tool result output must be valid JSON.: Output for tool call \"call-1\" is not valid JSON."},
		} {
			t.Run(tt.cause.Error(), func(t *testing.T) {
				t.Parallel()
				err := translateToolResultValidationError(&chatstate.ToolResultValidationError{Cause: tt.cause, ToolCallID: "call-1"})
				require.EqualError(t, err, tt.want)
			})
		}
	})
}
