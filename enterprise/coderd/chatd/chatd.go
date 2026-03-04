package chatd

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	osschatd "github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

// RemotePartsProvider returns a snapshot and live stream of
// message_part events from the replica that is running the chat.
// Called when the chat is actively running on a different replica.
type RemotePartsProvider func(
	ctx context.Context,
	chatID uuid.UUID,
	workerID uuid.UUID,
	requestHeader http.Header,
) (
	snapshot []codersdk.ChatStreamEvent,
	parts <-chan codersdk.ChatStreamEvent,
	cancel func(),
	err error,
)

// NewMultiReplicaSubscribeFn returns a SubscribeFn that merges local
// message_part events, relay (remote-replica) message_part events,
// and pubsub-driven durable events into a single output channel.
// This captures all relay/pubsub merge logic that enterprise
// multi-replica deployments require.
func NewMultiReplicaSubscribeFn(
	provider RemotePartsProvider,
) osschatd.SubscribeFn {
	return func(ctx context.Context, params osschatd.SubscribeMultiReplicaParams) (<-chan codersdk.ChatStreamEvent, func()) {
		chatID := params.ChatID
		localParts := params.LocalParts
		localCancel := params.LocalCancel
		lastMessageID := params.LastMessageID
		requestHeader := params.RequestHeader
		logger := params.Logger

		var relayCancel func()
		var relayParts <-chan codersdk.ChatStreamEvent

		// If the chat is currently running on a different worker
		// and we have a remote parts provider, open an initial
		// relay synchronously so the caller gets in-flight
		// message_part events right away.
		var initialRelaySnapshot []codersdk.ChatStreamEvent
		if params.Chat.Status == database.ChatStatusRunning &&
			params.Chat.WorkerID.Valid &&
			params.Chat.WorkerID.UUID != params.WorkerID &&
			provider != nil {
			snapshot, parts, cancel, err := provider(ctx, chatID, params.Chat.WorkerID.UUID, requestHeader)
			if err == nil {
				relayCancel = cancel
				relayParts = parts
				// Collect relay message_parts to forward at the
				// start of the merge goroutine.
				for _, event := range snapshot {
					if event.Type == codersdk.ChatStreamEventTypeMessagePart {
						initialRelaySnapshot = append(initialRelaySnapshot, event)
					}
				}
			}
		}

		// Merge all event sources.
		mergedEvents := make(chan codersdk.ChatStreamEvent, 128)
		var allCancels []func()
		allCancels = append(allCancels, localCancel)
		if relayCancel != nil {
			allCancels = append(allCancels, relayCancel)
		}

		// Channel for async relay establishment.
		type relayResult struct {
			parts    <-chan codersdk.ChatStreamEvent
			cancel   func()
			workerID uuid.UUID // the worker this dial targeted
		}
		relayReadyCh := make(chan relayResult, 1)

		// Per-dial context so in-flight dials can be cancelled when
		// a new dial is initiated or the relay is closed.
		var dialCancel context.CancelFunc

		// expectedWorkerID tracks which replica we expect the next
		// relay result to target. Stale results are discarded.
		var expectedWorkerID uuid.UUID

		// Reconnect timer state.
		var reconnectTimer *time.Timer
		var reconnectCh <-chan time.Time

		// Helper to close relay and stop any pending reconnect
		// timer.
		closeRelay := func() {
			// Cancel any in-flight dial goroutine first.
			if dialCancel != nil {
				dialCancel()
				dialCancel = nil
			}
			// Drain any buffered relay result from a cancelled
			// dial.
			select {
			case result := <-relayReadyCh:
				if result.cancel != nil {
					result.cancel()
				}
			default:
			}
			expectedWorkerID = uuid.Nil
			if relayCancel != nil {
				relayCancel()
				relayCancel = nil
			}
			relayParts = nil
			if reconnectTimer != nil {
				reconnectTimer.Stop()
				reconnectTimer = nil
				reconnectCh = nil
			}
		}

		// openRelayAsync dials the remote replica in a background
		// goroutine and delivers the result on relayReadyCh so the
		// main select loop is never blocked by network I/O.
		openRelayAsync := func(workerID uuid.UUID) {
			if provider == nil {
				return
			}
			closeRelay()
			// Create a per-dial context so this goroutine is
			// cancelled if closeRelay() or openRelayAsync() is
			// called again before the dial completes.
			var dialCtx context.Context
			dialCtx, dialCancel = context.WithCancel(ctx)
			expectedWorkerID = workerID
			go func() {
				snapshot, parts, cancel, err := provider(dialCtx, chatID, workerID, requestHeader)
				if err != nil {
					logger.Warn(ctx, "failed to open relay for message parts",
						slog.F("chat_id", chatID),
						slog.F("worker_id", workerID),
						slog.Error(err),
					)
					return
				}
				// Wrap the relay channel so snapshot parts are
				// delivered through the same channel as live
				// parts.
				wrappedParts := make(chan codersdk.ChatStreamEvent, 128)
				go func() {
					defer close(wrappedParts)
					for _, event := range snapshot {
						if event.Type == codersdk.ChatStreamEventTypeMessagePart {
							select {
							case wrappedParts <- event:
							case <-dialCtx.Done():
								cancel()
								return
							}
						}
					}
					for {
						select {
						case event, ok := <-parts:
							if !ok {
								return
							}
							select {
							case wrappedParts <- event:
							case <-dialCtx.Done():
								cancel()
								return
							}
						case <-dialCtx.Done():
							cancel()
							return
						}
					}
				}()
				select {
				case relayReadyCh <- relayResult{parts: wrappedParts, cancel: cancel, workerID: workerID}:
				case <-dialCtx.Done():
					cancel()
				}
			}()
		}

		// scheduleRelayReconnect arms a short timer so the select
		// loop can re-check chat status and reopen the relay
		// without spinning in a tight loop.
		scheduleRelayReconnect := func() {
			if provider == nil {
				return
			}
			if reconnectTimer != nil {
				reconnectTimer.Stop()
			}
			reconnectTimer = time.NewTimer(500 * time.Millisecond)
			reconnectCh = reconnectTimer.C
		}

		//nolint:nestif
		if params.Pubsub != nil {
			notifications := make(chan coderdpubsub.ChatStreamNotifyMessage, 10)
			errCh := make(chan error, 1)

			listener := func(_ context.Context, message []byte, err error) {
				if err != nil {
					select {
					case <-ctx.Done():
					case errCh <- err:
					}
					return
				}
				var notify coderdpubsub.ChatStreamNotifyMessage
				if unmarshalErr := json.Unmarshal(message, &notify); unmarshalErr != nil {
					select {
					case <-ctx.Done():
					case errCh <- xerrors.Errorf("unmarshal chat stream notify: %w", unmarshalErr):
					}
					return
				}
				select {
				case <-ctx.Done():
				case notifications <- notify:
				}
			}

			// Subscribe to pubsub for durable events.
			if pubsubCancel, err := params.Pubsub.SubscribeWithErr(
				coderdpubsub.ChatStreamNotifyChannel(chatID),
				listener,
			); err == nil {
				allCancels = append(allCancels, pubsubCancel)
			} else {
				logger.Warn(ctx, "failed to subscribe to chat stream notifications",
					slog.F("chat_id", chatID),
					slog.Error(err),
				)
			}

			// Handle pubsub notifications in a goroutine.
			go func() {
				defer close(mergedEvents)
				defer closeRelay()

				// Forward any initial relay snapshot parts
				// collected synchronously above.
				for _, event := range initialRelaySnapshot {
					select {
					case <-ctx.Done():
						return
					case mergedEvents <- event:
					}
				}

				for {
					relayPartsCh := relayParts
					select {
					case <-ctx.Done():
						return
					case err := <-errCh:
						logger.Error(ctx, "chat stream pubsub error",
							slog.F("chat_id", chatID),
							slog.Error(err),
						)
						mergedEvents <- codersdk.ChatStreamEvent{
							Type:   codersdk.ChatStreamEventTypeError,
							ChatID: chatID,
							Error: &codersdk.ChatStreamError{
								Message: err.Error(),
							},
						}
						return
					case result := <-relayReadyCh:
						// Discard stale relay results from a
						// previous dial that was superseded.
						if result.workerID != expectedWorkerID {
							if result.cancel != nil {
								result.cancel()
							}
							continue
						}
						// An async relay dial completed; swap
						// in the new relay channel.
						if relayCancel != nil {
							relayCancel()
						}
						relayParts = result.parts
						relayCancel = result.cancel
					case <-reconnectCh:
						reconnectCh = nil
						// Re-check whether the chat is still
						// running on a remote worker before
						// reconnecting.
						currentChat, chatErr := params.DB.GetChatByID(ctx, chatID)
						if chatErr == nil && currentChat.Status == database.ChatStatusRunning &&
							currentChat.WorkerID.Valid && currentChat.WorkerID.UUID != params.WorkerID {
							openRelayAsync(currentChat.WorkerID.UUID)
						}
					case notify := <-notifications:
						// Handle different notification types.
						if notify.AfterMessageID > 0 {
							// Read only new messages from DB.
							messages, err := params.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
								ChatID:  chatID,
								AfterID: lastMessageID,
							})
							if err == nil {
								for _, msg := range messages {
									sdkMsg := db2sdk.ChatMessage(msg)
									select {
									case <-ctx.Done():
										return
									case mergedEvents <- codersdk.ChatStreamEvent{
										Type:    codersdk.ChatStreamEventTypeMessage,
										ChatID:  chatID,
										Message: &sdkMsg,
									}:
									}
									lastMessageID = msg.ID
								}
							}
						}
						if notify.Status != "" {
							status := database.ChatStatus(notify.Status)
							select {
							case <-ctx.Done():
								return
							case mergedEvents <- codersdk.ChatStreamEvent{
								Type:   codersdk.ChatStreamEventTypeStatus,
								ChatID: chatID,
								Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatus(status)},
							}:
							}
							// Manage relay lifecycle based on
							// status.
							if status == database.ChatStatusRunning && notify.WorkerID != "" {
								workerID, err := uuid.Parse(notify.WorkerID)
								if err == nil && workerID != params.WorkerID {
									openRelayAsync(workerID)
								} else if workerID == params.WorkerID {
									closeRelay()
								}
							} else {
								closeRelay()
							}
						}
						if notify.Error != "" {
							select {
							case <-ctx.Done():
								return
							case mergedEvents <- codersdk.ChatStreamEvent{
								Type:   codersdk.ChatStreamEventTypeError,
								ChatID: chatID,
								Error: &codersdk.ChatStreamError{
									Message: notify.Error,
								},
							}:
							}
						}
						if notify.QueueUpdate {
							queued, err := params.DB.GetChatQueuedMessages(ctx, chatID)
							if err == nil {
								select {
								case <-ctx.Done():
									return
								case mergedEvents <- codersdk.ChatStreamEvent{
									Type:           codersdk.ChatStreamEventTypeQueueUpdate,
									ChatID:         chatID,
									QueuedMessages: db2sdk.ChatQueuedMessages(queued),
								}:
								}
							}
						}
					case event, ok := <-localParts:
						if !ok {
							// Local parts channel closed, but
							// continue with pubsub.
							continue
						}
						// Only forward message_part events from
						// local (durable events come via pubsub).
						if event.Type == codersdk.ChatStreamEventTypeMessagePart {
							select {
							case <-ctx.Done():
								return
							case mergedEvents <- event:
							}
						}
					case event, ok := <-relayPartsCh:
						if !ok {
							if relayCancel != nil {
								relayCancel()
								relayCancel = nil
							}
							relayParts = nil
							// Schedule reconnection instead of
							// giving up.
							scheduleRelayReconnect()
							continue
						}
						// Only forward message_part events from
						// relay (durable events come via pubsub).
						if event.Type == codersdk.ChatStreamEventTypeMessagePart {
							select {
							case <-ctx.Done():
								return
							case mergedEvents <- event:
							}
						}
					}
				}
			}()
		} else {
			// No pubsub, just merge local parts.
			// localSnapshot was already included in initialSnapshot,
			// so only forward new events here.
			go func() {
				defer close(mergedEvents)
				for event := range localParts {
					select {
					case <-ctx.Done():
						return
					case mergedEvents <- event:
					}
				}
			}()
		}

		cancel := func() {
			for _, cancelFn := range allCancels {
				if cancelFn != nil {
					cancelFn()
				}
			}
			if reconnectTimer != nil {
				reconnectTimer.Stop()
			}
		}

		return mergedEvents, cancel
	}
}
