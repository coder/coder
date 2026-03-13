package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
)

func TestRefreshChatWorkspaceSnapshot_NoReloadWhenWorkspacePresent(t *testing.T) {
	t.Parallel()

	workspaceID := uuid.New()
	chat := database.Chat{
		ID: uuid.New(),
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			calls++
			return database.Chat{}, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, chat, refreshed)
	require.Equal(t, 0, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReloadsWhenWorkspaceMissing(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	workspaceID := uuid.New()
	chat := database.Chat{ID: chatID}
	reloaded := database.Chat{
		ID: chatID,
		WorkspaceID: uuid.NullUUID{
			UUID:  workspaceID,
			Valid: true,
		},
	}

	calls := 0
	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(_ context.Context, id uuid.UUID) (database.Chat, error) {
			calls++
			require.Equal(t, chatID, id)
			return reloaded, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, reloaded, refreshed)
	require.Equal(t, 1, calls)
}

func TestRefreshChatWorkspaceSnapshot_ReturnsReloadError(t *testing.T) {
	t.Parallel()

	chat := database.Chat{ID: uuid.New()}
	loadErr := xerrors.New("boom")

	refreshed, err := refreshChatWorkspaceSnapshot(
		context.Background(),
		chat,
		func(context.Context, uuid.UUID) (database.Chat, error) {
			return database.Chat{}, loadErr
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "reload chat workspace state")
	require.ErrorContains(t, err, loadErr.Error())
	require.Equal(t, chat, refreshed)
}

func TestPublishToStreamOverflowTriggersOnFullBuffer(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	srv := &Server{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}

	state := srv.getOrCreateStreamState(chatID)
	state.mu.Lock()
	state.buffering = true
	state.mu.Unlock()

	_, ch, overflow, cancel := srv.subscribeToStream(chatID)
	t.Cleanup(cancel)

	// Publish more events than the subscriber channel buffer (128)
	// can hold, without consuming from the channel.
	const totalPublished = 200
	for range totalPublished {
		srv.publishToStream(chatID, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeMessagePart,
			ChatID: chatID,
		})
	}

	// The overflow channel should be signaled because the
	// subscriber's buffer filled up.
	select {
	case <-overflow:
	default:
		t.Fatal("expected overflow signal when subscriber buffer is full")
	}

	// The subscriber channel should have exactly 128 buffered
	// events (its capacity) before overflow was triggered.
	var received int
drain:
	for {
		select {
		case <-ch:
			received++
		default:
			break drain
		}
	}
	require.Equal(t, 128, received)
}

func TestPublishToStreamSuccessfulDelivery(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	srv := &Server{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}

	state := srv.getOrCreateStreamState(chatID)
	state.mu.Lock()
	state.buffering = true
	state.mu.Unlock()

	_, ch, overflow, cancel := srv.subscribeToStream(chatID)
	t.Cleanup(cancel)

	// Publish events while actively consuming. The subscriber
	// should never overflow because the channel never fills.
	const totalPublished = 50
	for range totalPublished {
		srv.publishToStream(chatID, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeMessagePart,
			ChatID: chatID,
		})
		// Immediately consume the event.
		select {
		case <-ch:
		default:
			t.Fatal("expected event to be available immediately")
		}
	}

	// Overflow must not have been signaled.
	select {
	case <-overflow:
		t.Fatal("overflow signaled unexpectedly")
	default:
	}
}

// Removed: TestPublishToStreamOverflowSignalsSubscriber was a
// duplicate of TestPublishToStreamOverflowTriggersOnFullBuffer.

func TestPublishToStreamBufferFullDropsOldest(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	srv := &Server{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}

	state := srv.getOrCreateStreamState(chatID)
	state.mu.Lock()
	state.buffering = true
	state.mu.Unlock()

	// Publish more than maxStreamBufferSize events with no
	// subscribers to exercise the buffer-full oldest-drop path.
	for i := range maxStreamBufferSize + 100 {
		srv.publishToStream(chatID, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeMessagePart,
			ChatID: chatID,
		})
		_ = i
	}

	state.mu.Lock()
	bufLen := len(state.buffer)
	state.mu.Unlock()

	// The buffer should be capped at maxStreamBufferSize.
	require.Equal(t, maxStreamBufferSize, bufLen)
}

func TestPublishToStreamNotBufferingEarlyReturn(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	srv := &Server{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}

	// Do NOT enable buffering. The message_part event should hit
	// the early return path and not be buffered.
	state := srv.getOrCreateStreamState(chatID)

	srv.publishToStream(chatID, codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeMessagePart,
		ChatID: chatID,
	})

	state.mu.Lock()
	bufLen := len(state.buffer)
	state.mu.Unlock()

	require.Equal(t, 0, bufLen)

	// The stream state should have been cleaned up since there
	// are no subscribers and buffering is off.
	_, loaded := srv.chatStreams.Load(chatID)
	require.False(t, loaded, "stream state should be cleaned up when idle")
}

func TestSubscribeMergeDetectsOverflow(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	ctrl := gomock.NewController(t)
	mockDB := dbmock.NewMockStore(ctrl)

	// Subscribe calls GetChatMessagesByChatID, GetChatQueuedMessages,
	// and GetChatByID during snapshot construction.
	mockDB.EXPECT().GetChatMessagesByChatID(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	mockDB.EXPECT().GetChatQueuedMessages(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	mockDB.EXPECT().GetChatByID(gomock.Any(), gomock.Any()).
		Return(database.Chat{ID: chatID, Status: database.ChatStatusPending}, nil).AnyTimes()

	srv := &Server{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		db:     mockDB,
	}

	// Enable buffering so message_part events are accepted.
	state := srv.getOrCreateStreamState(chatID)
	state.mu.Lock()
	state.buffering = true
	state.mu.Unlock()

	// Use the full Subscribe path. pubsub is nil so the merge
	// goroutine will use local-only forwarding.
	_, mergedEvents, cancel, ok := srv.Subscribe(
		t.Context(), chatID, nil, 0,
	)
	require.True(t, ok)
	t.Cleanup(cancel)

	// Publish enough events to overflow the local subscriber
	// channel (buffer=128). The merge goroutine reads from the
	// local channel, but we publish fast enough to fill it.
	// Since mergedEvents also has a buffer of 128, we need to
	// saturate both channels. Publish 300 events to be safe.
	for range 300 {
		srv.publishToStream(chatID, codersdk.ChatStreamEvent{
			Type:   codersdk.ChatStreamEventTypeMessagePart,
			ChatID: chatID,
		})
	}

	// The merged events channel should close because the merge
	// goroutine detects the overflow signal and returns.
	for ev := range mergedEvents {
		_ = ev
	}

	// If we reach here, mergedEvents was closed. That confirms
	// the merge goroutine detected the overflow and terminated.
}
