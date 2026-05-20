package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestSubscribeDeliversOutOfOrderDurableMessage covers the case where
// the durable cache receives a higher-ID message before a lower-ID one,
// then a notify arrives for the lower-ID. The merge goroutine has
// already advanced lastMessageID past the lower ID, so it must rescan
// the gap range and dedupe.
func TestSubscribeDeliversOutOfOrderDurableMessage(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusRequiresAction}
	initialUser := database.ChatMessage{ID: 3, ChatID: chatID, Role: database.ChatMessageRoleUser}
	initialAssistant := database.ChatMessage{ID: 4, ChatID: chatID, Role: database.ChatMessageRoleAssistant}

	gomock.InOrder(
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 0,
		}).Return([]database.ChatMessage{initialUser, initialAssistant}, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
	)
	// Notify-driven catch-up queries return nothing so the test only
	// exercises the cache delivery path.
	db.EXPECT().GetChatMessagesByChatID(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := newSubscribeTestServer(t, db)

	toolResult := codersdk.ChatMessage{ID: 5, ChatID: chatID, Role: codersdk.ChatMessageRoleTool}
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
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: &toolResult,
	})
	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: &resumed,
	})
	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{AfterMessageID: 4})

	first := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatStreamEventTypeMessage, first.Type)
	require.NotNil(t, first.Message)
	require.Equal(t, int64(5), first.Message.ID)
	second := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatStreamEventTypeMessage, second.Type)
	require.NotNil(t, second.Message)
	require.Equal(t, int64(7), second.Message.ID)

	// Cache id=6 after the merge goroutine has already advanced
	// lastMessageID to 7, then emit the notify for id=6.
	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: &promoted,
	})
	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{AfterMessageID: 5})

	third := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatStreamEventTypeMessage, third.Type)
	require.NotNil(t, third.Message)
	require.Equal(t, int64(6), third.Message.ID)

	requireNoStreamEvent(t, events, testutil.IntervalFast)
}

// TestSubscribeRespectsAfterMessageIDOnLateNotify guards against
// re-emitting messages the subscriber already has via the REST
// snapshot. A reconnecting client passes afterMessageID > 0 to skip
// the initial replay; lookupAfter must not drop below that boundary
// even when a late notify reports AfterMessageID below it.
func TestSubscribeRespectsAfterMessageIDOnLateNotify(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusRunning}

	gomock.InOrder(
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 100,
		}).Return(nil, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
	)
	db.EXPECT().GetChatMessagesByChatID(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := newSubscribeTestServer(t, db)

	// Seed the cache with messages the client claims to already have
	// (id<=100) plus one new message (id=101).
	for _, id := range []int64{96, 97, 98, 99, 100, 101} {
		msg := &codersdk.ChatMessage{ID: id, ChatID: chatID, Role: codersdk.ChatMessageRoleAssistant}
		server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
			Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: msg,
		})
	}

	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 100)
	require.True(t, ok)
	defer cancel()

	// A stale notify with AfterMessageID=95 would naively pull
	// id=96..101 back from the cache; only id=101 should reach the
	// live stream because the client already has 96-100.
	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{AfterMessageID: 95})

	ev := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatStreamEventTypeMessage, ev.Type)
	require.NotNil(t, ev.Message)
	require.Equal(t, int64(101), ev.Message.ID,
		"messages at or below afterMessageID must not be re-emitted")

	requireNoStreamEvent(t, events, testutil.IntervalFast)
}

// TestSubscribeRunsDBFallbackWhenCacheDeliversUnrelatedMessage
// guards against the deliveredAny gate that historically skipped the
// DB fallback whenever the cache contributed anything. The DB is
// authoritative for cross-replica messages the local cache cannot
// hold, so both passes must always run on every notify.
func TestSubscribeRunsDBFallbackWhenCacheDeliversUnrelatedMessage(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	chatID := uuid.New()
	chat := database.Chat{ID: chatID, Status: database.ChatStatusRunning}
	crossReplica := database.ChatMessage{ID: 6, ChatID: chatID, Role: database.ChatMessageRoleUser}

	gomock.InOrder(
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		db.EXPECT().GetChatByID(gomock.Any(), chatID).Return(chat, nil),
		// Snapshot: nothing above the client's afterMessageID=5 yet.
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 5,
		}).Return(nil, nil),
		db.EXPECT().GetChatQueuedMessages(gomock.Any(), chatID).Return(nil, nil),
		// Notify catchup: the cross-replica message lives only in the
		// DB on this replica.
		db.EXPECT().GetChatMessagesByChatID(gomock.Any(), database.GetChatMessagesByChatIDParams{
			ChatID:  chatID,
			AfterID: 5,
		}).Return([]database.ChatMessage{crossReplica}, nil),
	)

	server := newSubscribeTestServer(t, db)

	// Cache a locally-published higher-ID message so the cache pass
	// has something to deliver without covering id=6.
	localOnly := codersdk.ChatMessage{ID: 8, ChatID: chatID, Role: codersdk.ChatMessageRoleAssistant}
	server.cacheDurableMessage(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage, ChatID: chatID, Message: &localOnly,
	})

	_, events, cancel, ok := server.Subscribe(ctx, chatID, nil, 5)
	require.True(t, ok)
	defer cancel()

	server.publishChatStreamNotify(chatID, coderdpubsub.ChatStreamNotifyMessage{AfterMessageID: 5})

	// The cache pass delivers id=8; the DB pass must still run and
	// deliver id=6. Order between them is set by cache iteration vs
	// DB query, so accept either ordering.
	first := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatStreamEventTypeMessage, first.Type)
	require.NotNil(t, first.Message)
	second := testutil.RequireReceive(ctx, t, events)
	require.Equal(t, codersdk.ChatStreamEventTypeMessage, second.Type)
	require.NotNil(t, second.Message)

	got := map[int64]bool{first.Message.ID: true, second.Message.ID: true}
	require.True(t, got[6], "cross-replica DB message id=6 must be delivered")
	require.True(t, got[8], "locally-cached message id=8 must be delivered")

	requireNoStreamEvent(t, events, testutil.IntervalFast)
}
