package chatd

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
)

const (
	streamSyncRetryInitialBackoff = 100 * time.Millisecond
	streamSyncRetryMaxBackoff     = time.Second
	streamSyncRetryMaxAttempts    = 5
)

func (p *Server) subscribeStreamLoop(
	ctx context.Context,
	chat database.Chat,
	requestHeader http.Header,
	afterMessageID int64,
) ([]codersdk.ChatStreamEvent, <-chan codersdk.ChatStreamEvent, func(), bool) {
	if p == nil || p.db == nil || p.pubsub == nil {
		return nil, nil, nil, false
	}
	if p.messagePartBuffer == nil {
		p.messagePartBuffer = messagepartbuffer.New(messagepartbuffer.Options{Clock: p.clock})
	}
	chatID := chat.ID
	streamCtx, streamCancel := context.WithCancel(ctx)
	events := make(chan codersdk.ChatStreamEvent, 128)
	logger := p.logger.With(slog.F("chat_id", chatID))

	updateCh := make(chan streamSyncHint, 32)
	pubsubCancel, err := p.pubsub.SubscribeWithErr(
		coderdpubsub.ChatStateUpdateChannel(chatID),
		coderdpubsub.HandleChatStateUpdate(func(_ context.Context, payload coderdpubsub.ChatStateUpdateMessage, err error) {
			if err != nil {
				logger.Warn(streamCtx, "chat stream pubsub error", slog.Error(err))
				return
			}
			select {
			case updateCh <- streamSyncHintFromUpdate(payload):
			case <-streamCtx.Done():
			}
		}),
	)
	if err != nil {
		logger.Warn(ctx, "failed to subscribe to chat state updates", slog.Error(err))
		streamCancel()
		return subscribeWithInitialError(chatID, "failed to subscribe to chat updates")
	}

	pollerCh, unregisterPoller := p.streamSyncPoller.Register(chatID)
	loop := newStreamLoop(chat, p.db, logger, afterMessageID)
	// The immediate sync builds the initial snapshot returned to the caller
	// and the relay target for the forwarder. Hints only fire on state
	// changes, so without it an idle chat would never deliver a snapshot and
	// an actively streaming chat would not relay parts until the next hint.
	//nolint:gocritic // The HTTP route authorizes the chat before subscribing; the stream loop needs chatd-scoped reads for one consistent snapshot.
	initial, target, _, err := loop.syncDB(dbauthz.AsChatd(ctx))
	if err != nil {
		logger.Error(ctx, "failed to load initial chat stream snapshot", slog.Error(err))
		unregisterPoller()
		pubsubCancel()
		streamCancel()
		return subscribeWithInitialError(chatID, "failed to load initial snapshot")
	}

	relay := newStreamRelayForwarder(
		chatID,
		requestHeader,
		p.streamPartsDialer,
		p.clock,
		logger,
	)
	relay.Configure(streamCtx, target)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(events)
		defer relay.Close()
		defer unregisterPoller()
		for {
			select {
			case <-streamCtx.Done():
				return
			case hint := <-updateCh:
				if !p.runStreamSync(streamCtx, loop, relay, events, hint) {
					return
				}
			case hint, ok := <-pollerCh:
				if !ok {
					return
				}
				if !p.runStreamSync(streamCtx, loop, relay, events, hint) {
					return
				}
			case part, ok := <-relay.Parts():
				if !ok {
					return
				}
				event, accepted, err := loop.part(part)
				if err != nil {
					logger.Error(streamCtx, "chat stream invariant violation", slog.Error(err))
					return
				}
				if accepted {
					sendStreamEvent(streamCtx, events, event)
				}
			}
		}
	}()

	cancel := func() {
		streamCancel()
		pubsubCancel()
		<-done
	}
	return initial, events, cancel, true
}

func (p *Server) runStreamSync(
	ctx context.Context,
	loop *streamLoop,
	relay *streamRelayForwarder,
	events chan<- codersdk.ChatStreamEvent,
	hint streamSyncHint,
) bool {
	syncEvents, target, changed, err := p.syncStreamWithRetry(ctx, loop, hint)
	if err != nil {
		p.logger.Error(ctx, "failed to sync chat stream after retries", slog.Error(err))
		return false
	}
	for _, event := range syncEvents {
		if !sendStreamEvent(ctx, events, event) {
			return false
		}
	}
	if changed {
		relay.Configure(ctx, target)
	}
	return true
}

func (p *Server) syncStreamWithRetry(
	ctx context.Context,
	loop *streamLoop,
	hint streamSyncHint,
) ([]codersdk.ChatStreamEvent, streamRelayTarget, bool, error) {
	var (
		syncEvents []codersdk.ChatStreamEvent
		target     streamRelayTarget
		changed    bool
		err        error
	)
	for attempt := 1; attempt <= streamSyncRetryMaxAttempts; attempt++ {
		//nolint:gocritic // The subscriber was authorized before the loop started; follow-up syncs need chatd-scoped reads for consistency.
		syncEvents, target, changed, err = loop.sync(dbauthz.AsChatd(ctx), hint)
		if err == nil || ctx.Err() != nil {
			return syncEvents, target, changed, err
		}
		p.logger.Warn(ctx, "failed to sync chat stream",
			slog.F("attempt", attempt),
			slog.Error(err),
		)
		if attempt == streamSyncRetryMaxAttempts {
			break
		}
		if !p.waitBeforeStreamSyncRetry(ctx, attempt) {
			return nil, loop.currentRelayTarget(), false, ctx.Err()
		}
	}
	return nil, loop.currentRelayTarget(), false, err
}

func (p *Server) waitBeforeStreamSyncRetry(ctx context.Context, attempt int) bool {
	delay := streamSyncRetryInitialBackoff
	for range attempt - 1 {
		delay *= 2
		if delay >= streamSyncRetryMaxBackoff {
			delay = streamSyncRetryMaxBackoff
			break
		}
	}
	timer := p.clock.NewTimer(delay, "chatd", "stream-sync-retry")
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func sendStreamEvent(ctx context.Context, ch chan<- codersdk.ChatStreamEvent, event codersdk.ChatStreamEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (p *Server) Subscribe(
	ctx context.Context,
	chatID uuid.UUID,
	requestHeader http.Header,
	afterMessageID int64,
) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
	bool,
) {
	if p == nil {
		return nil, nil, nil, false
	}

	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		if dbauthz.IsNotAuthorizedError(err) {
			return nil, nil, nil, false
		}
		p.logger.Warn(ctx, "failed to load chat for stream subscription",
			slog.F("chat_id", chatID),
			slog.Error(err),
		)
		return subscribeWithInitialError(chatID, "failed to load initial snapshot")
	}
	return p.SubscribeAuthorized(ctx, chat, requestHeader, afterMessageID)
}

// SubscribeAuthorized subscribes an already-authorized chat to stream updates.
func (p *Server) SubscribeAuthorized(
	ctx context.Context,
	chat database.Chat,
	requestHeader http.Header,
	afterMessageID int64,
) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
	bool,
) {
	return p.subscribeStreamLoop(ctx, chat, requestHeader, afterMessageID)
}
