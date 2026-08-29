package chatd

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type mutationRecorder struct {
	pubsub.Pubsub
	mu     sync.Mutex
	events []mutationEvent
}

type mutationEvent struct {
	channel string
	payload []byte
}

func (r *mutationRecorder) Publish(channel string, payload []byte) error {
	r.mu.Lock()
	r.events = append(r.events, mutationEvent{channel: channel, payload: append([]byte(nil), payload...)})
	r.mu.Unlock()
	return r.Pubsub.Publish(channel, payload)
}

func (r *mutationRecorder) snapshot() []mutationEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]mutationEvent(nil), r.events...)
}

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
				db, ps := dbtestutil.NewDB(t)
				owner := dbgen.User(t, db, database.User{})
				org := dbgen.Organization(t, db, database.Organization{})
				dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "openai", BaseUrl: "http://example.invalid"})
				model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{OrganizationID: org.ID, IsDefault: true})
				chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: owner.ID, LastModelConfigID: model.ID})
				recorder := &mutationRecorder{Pubsub: ps}
				mutator := chatMutator{server: &Server{db: db, pubsub: recorder, logger: slogtest.Make(t, nil)}}
				wantErr := xerrors.New("transition failed")

				updated, err := mutator.update(ctx, chat.ID, "test", func(tx *chatstate.Tx, _ database.Store, _ *chatMutation) error {
					if tt.fail {
						return wantErr
					}
					_, err := tx.RequestCompaction(chatstate.RequestCompactionInput{})
					return err
				})
				stored, loadErr := db.GetChatByID(ctx, chat.ID)
				require.NoError(t, loadErr)
				events := recorder.snapshot()
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
					case coderdpubsub.ChatWatchEventChannel(owner.ID):
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
			{name: "send chat ID", call: func() error { _, err := mutator.SendMessage(context.Background(), SendMessageOptions{}); return err }, want: "chat_id is required"},
			{name: "send content", call: func() error {
				_, err := mutator.SendMessage(context.Background(), SendMessageOptions{ChatID: uuid.New()})
				return err
			}, want: "content is required"},
			{name: "send busy behavior", call: func() error {
				_, err := mutator.SendMessage(context.Background(), SendMessageOptions{ChatID: uuid.New(), Content: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hi")}, BusyBehavior: "invalid"})
				return err
			}, want: "invalid busy behavior \"invalid\""},
			{name: "edit chat ID", call: func() error { _, err := mutator.EditMessage(context.Background(), EditMessageOptions{}); return err }, want: "chat_id is required"},
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
			{name: "delete queued chat ID", call: func() error { return mutator.DeleteQueued(context.Background(), uuid.Nil, 1) }, want: "chat_id is required"},
			{name: "promote queued chat ID", call: func() error {
				_, err := mutator.PromoteQueued(context.Background(), PromoteQueuedOptions{})
				return err
			}, want: "chat_id is required"},
			{name: "interrupt chat ID", call: func() error { _, err := mutator.InterruptChat(context.Background(), database.Chat{}); return err }, want: "chat_id is required"},
			{name: "compact chat ID", call: func() error { _, err := mutator.CompactChat(context.Background(), database.Chat{}); return err }, want: "chat_id is required"},
			{name: "clear chat ID", call: func() error { _, err := mutator.ClearChat(context.Background(), database.Chat{}); return err }, want: "chat_id is required"},
			{name: "reconcile chat ID", call: func() error {
				_, err := mutator.ReconcileInvalidStateChat(context.Background(), database.Chat{})
				return err
			}, want: "chat_id is required"},
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
