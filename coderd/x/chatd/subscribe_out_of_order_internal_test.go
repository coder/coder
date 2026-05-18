package chatd

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

// TestSubscribeDeliversOutOfOrderDurableMessage covers the case where
// the durable cache receives a higher-ID message before a lower-ID one,
// then a notify arrives for the lower-ID. The merge goroutine has
// already advanced lastMessageID past the lower ID, so it must rescan
// the gap range and dedupe.
func TestSubscribeDeliversOutOfOrderDurableMessage(t *testing.T) {
	t.Parallel()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusRequiresAction}
	initialUser := database.ChatMessage{ID: 3, ChatID: chatID, Role: database.ChatMessageRoleUser}
	initialAssistant := database.ChatMessage{ID: 4, ChatID: chatID, Role: database.ChatMessageRoleAssistant}

	db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil).AnyTimes()
	db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: 0,
	}).Return([]database.ChatMessage{initialUser, initialAssistant}, nil).AnyTimes()
	db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil).AnyTimes()
	// Catch-up queries return nothing so the test only exercises the
	// cache delivery path.
	db.EXPECT().GetChatMessagesByChatID(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := newSubscribeTestServer(t, db)

	synth := codersdk.ChatMessage{ID: 5, ChatID: chatID, Role: codersdk.ChatMessageRoleTool}
	resumed := codersdk.ChatMessage{ID: 7, ChatID: chatID, Role: codersdk.ChatMessageRoleAssistant}
	promoted := codersdk.ChatMessage{ID: 6, ChatID: chatID, Role: codersdk.ChatMessageRoleUser}

	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID,
		Message: &codersdk.ChatMessage{ID: 4, ChatID: chatID, Role: codersdk.ChatMessageRoleAssistant},
	})

	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 0)
	require.True(t, ok)
	defer cancel()

	// Cache id=5 and id=7, but not id=6, then emit the notify for
	// id=5. The merge goroutine drains [5, 7] from the cache.
	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: &synth,
	})
	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: &resumed,
	})
	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{AfterMessageID: 4})

	first := requireStreamMessageEvent(t, events)
	require.Equal(t, int64(5), first.Message.ID)
	second := requireStreamMessageEvent(t, events)
	require.Equal(t, int64(7), second.Message.ID)

	// Cache id=6 after the merge goroutine has already advanced
	// lastMessageID to 7, then emit the notify for id=6.
	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: &promoted,
	})
	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{AfterMessageID: 5})

	select {
	case ev := <-events:
		require.Equal(t, codersdk.ChatStreamEventTypeMessage, ev.Type)
		require.NotNil(t, ev.Message)
		require.Equal(t, int64(6), ev.Message.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for out-of-order durable message")
	}
}
