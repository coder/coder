package chatd

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	osschatd "github.com/coder/coder/v2/coderd/chatd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// RelaySourceHeader marks replica-relayed stream requests.
const RelaySourceHeader = "X-Coder-Relay-Source-Replica"

const (
	authorizationHeader = "Authorization"
	cookieHeader        = "Cookie"
)

// MultiReplicaSubscribeConfig holds the dependencies for multi-replica chat
// subscription. ReplicaIDFn is called lazily because the
// replica ID may not be known at construction time.
//
// DialerFn, when set, overrides the default WebSocket relay
// dialer. This is used in tests to inject mock relay behavior
// without requiring real HTTP servers.
type MultiReplicaSubscribeConfig struct {
	ResolveReplicaAddress func(context.Context, uuid.UUID) (string, bool)
	ReplicaHTTPClient     *http.Client
	ReplicaIDFn           func() uuid.UUID
	DialerFn              func(
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
	// Clock is used for creating timers. In production use
	// quartz.NewReal(); in tests use quartz.NewMock(t) to
	// control reconnect timing deterministically.
	Clock quartz.Clock
}

// dial returns the dialer function to use for relay connections.
// If DialerFn is set (e.g. in tests), it takes precedence.
// Otherwise, dialRelay is used with the real MultiReplicaSubscribeConfig dependencies.
// Returns nil when no relay capability is configured.
func (c MultiReplicaSubscribeConfig) dial() func(
	ctx context.Context,
	chatID uuid.UUID,
	workerID uuid.UUID,
	requestHeader http.Header,
) (
	[]codersdk.ChatStreamEvent,
	<-chan codersdk.ChatStreamEvent,
	func(),
	error,
) {
	if c.DialerFn != nil {
		return c.DialerFn
	}
	if c.ResolveReplicaAddress == nil {
		return nil
	}
	return func(
		ctx context.Context,
		chatID uuid.UUID,
		workerID uuid.UUID,
		requestHeader http.Header,
	) (
		[]codersdk.ChatStreamEvent,
		<-chan codersdk.ChatStreamEvent,
		func(),
		error,
	) {
		return dialRelay(ctx, chatID, workerID, requestHeader, c, c.clock())
	}
}

// clock returns the quartz.Clock to use. Defaults to a real clock
// when not set.
func (c MultiReplicaSubscribeConfig) clock() quartz.Clock {
	if c.Clock != nil {
		return c.Clock
	}
	return quartz.NewReal()
}

// NewMultiReplicaSubscribeFn returns a SubscribeFn that manages
// relay connections to remote replicas and returns relay
// message_part events only. OSS handles pubsub subscription,
// message catch-up, queue updates, status forwarding, and local
// parts merging.
//
//nolint:gocognit // Complexity is inherent to the multi-source merge loop.
func NewMultiReplicaSubscribeFn(
	cfg MultiReplicaSubscribeConfig,
) osschatd.SubscribeFn {
	return func(ctx context.Context, params osschatd.SubscribeFnParams) (<-chan codersdk.ChatStreamEvent, func()) {
		chatID := params.ChatID
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
			cfg.dial() != nil {
			snapshot, parts, cancel, err := cfg.dial()(ctx, chatID, params.Chat.WorkerID.UUID, requestHeader)
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
			} else {
				logger.Warn(ctx, "failed to open initial relay for chat stream",
					slog.F("chat_id", chatID),
					slog.Error(err),
				)
			}
		}

		// Merge all event sources.
		mergedEvents := make(chan codersdk.ChatStreamEvent, 128)
		var allCancels []func()
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

		// Per-dial context so in-flight dials can be canceled when
		// a new dial is initiated or the relay is closed.
		var dialCancel context.CancelFunc

		// expectedWorkerID tracks which replica we expect the next
		// relay result to target. Stale results are discarded.
		var expectedWorkerID uuid.UUID

		// Reconnect timer state.
		var reconnectTimer *quartz.Timer
		var reconnectCh <-chan time.Time

		// Helper to close relay and stop any pending reconnect
		// timer.
		closeRelay := func() {
			// Cancel any in-flight dial goroutine first.
			if dialCancel != nil {
				dialCancel()
				dialCancel = nil
			}
			// Drain any buffered relay result from a canceled
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
			if cfg.dial() == nil {
				return
			}
			closeRelay()
			// Create a per-dial context so this goroutine is
			// canceled if closeRelay() or openRelayAsync() is
			// called again before the dial completes.
			var dialCtx context.Context
			dialCtx, dialCancel = context.WithCancel(ctx)
			expectedWorkerID = workerID
			go func() {
				snapshot, parts, cancel, err := cfg.dial()(dialCtx, chatID, workerID, requestHeader)
				if err != nil {
					// Don't log context-canceled errors
					// since they are expected when a dial is
					// superseded by a newer one.
					if dialCtx.Err() == nil {
						logger.Warn(ctx, "failed to open relay for message parts",
							slog.F("chat_id", chatID),
							slog.F("worker_id", workerID),
							slog.Error(err),
						)
					}
					// Send an empty result so the merge loop
					// can schedule a reconnect attempt.
					select {
					case relayReadyCh <- relayResult{workerID: workerID}:
					case <-dialCtx.Done():
					}
					return
				} // If the dial context was canceled while the
				// dial was in progress, discard the result to
				// avoid starting a wrappedParts goroutine for
				// a stale connection.
				if dialCtx.Err() != nil {
					cancel()
					return
				}
				// Wrap the relay channel so snapshot parts
				// are delivered through the same channel as
				// live parts. This goroutine only forwards
				// events — it does not own the relay
				// lifecycle. When dialCtx is canceled it
				// simply returns, closing wrappedParts via
				// its defer. The cancel() is called by
				// whoever canceled dialCtx (closeRelay or
				// the send-fallback select below).
				wrappedParts := make(chan codersdk.ChatStreamEvent, 128)
				go func() {
					defer close(wrappedParts)
					for _, event := range snapshot {
						if event.Type == codersdk.ChatStreamEventTypeMessagePart {
							select {
							case wrappedParts <- event:
							case <-dialCtx.Done():
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
								return
							}
						case <-dialCtx.Done():
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
			if cfg.dial() == nil {
				return
			}
			if reconnectTimer != nil {
				reconnectTimer.Stop()
			}
			reconnectTimer = cfg.clock().NewTimer(500*time.Millisecond, "reconnect")
			reconnectCh = reconnectTimer.C
		}

		statusNotifications := params.StatusNotifications
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
				case result := <-relayReadyCh:
					// Discard stale relay results from a
					// previous dial that was superseded.
					if result.workerID != expectedWorkerID {
						if result.cancel != nil {
							result.cancel()
						}
						continue
					}
					// A nil parts channel signals the dial
					// failed — schedule a retry.
					if result.parts == nil {
						scheduleRelayReconnect()
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
					if chatErr != nil {
						logger.Warn(ctx, "failed to get chat for relay reconnect",
							slog.F("chat_id", chatID),
							slog.Error(chatErr),
						)
						// Retry on transient DB errors to
						// avoid permanently stalling the
						// stream.
						scheduleRelayReconnect()
						continue
					}
					if currentChat.Status == database.ChatStatusRunning &&
						currentChat.WorkerID.Valid && currentChat.WorkerID.UUID != params.WorkerID {
						openRelayAsync(currentChat.WorkerID.UUID)
					}
				case sn, ok := <-statusNotifications:
					if !ok {
						statusNotifications = nil
						continue
					}
					if sn.Status == database.ChatStatusRunning && sn.WorkerID != uuid.Nil && sn.WorkerID != params.WorkerID {
						openRelayAsync(sn.WorkerID)
					} else {
						closeRelay()
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
					// relay.
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

		// The cancel function tears down the relay state
		// indirectly: the merge goroutine owns all relay state
		// (reconnectTimer, relayCancel, dialCancel, etc.) and
		// cleans it up via its defer closeRelay() when ctx is
		// canceled.
		cancel := func() {
			for _, cancelFn := range allCancels {
				if cancelFn != nil {
					cancelFn()
				}
			}
		}
		return mergedEvents, cancel
	}
}

// dialRelay opens a WebSocket relay connection to the replica
// identified by workerID and returns a snapshot of buffered
// message_part events plus a live channel of subsequent events.
// It passes afterID=MaxInt64 so the remote replica skips the
// full message history snapshot, since the relay only needs
// live message_part events.
func dialRelay(
	ctx context.Context,
	chatID uuid.UUID,
	workerID uuid.UUID,
	requestHeader http.Header,
	cfg MultiReplicaSubscribeConfig,
	clk quartz.Clock,
) (
	snapshot []codersdk.ChatStreamEvent,
	parts <-chan codersdk.ChatStreamEvent,
	cancel func(),
	err error,
) {
	address, ok := cfg.ResolveReplicaAddress(ctx, workerID)
	if !ok {
		return nil, nil, nil, xerrors.New("worker replica not found")
	}

	baseURL, err := url.Parse(address)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("parse relay address %q: %w", address, err)
	}
	replicaID := cfg.ReplicaIDFn()
	relayCtx, relayCancel := context.WithCancel(ctx)
	sdkClient := codersdk.New(baseURL)
	sdkClient.HTTPClient = cfg.ReplicaHTTPClient
	sdkClient.SessionTokenProvider = relayHeaderTokenProvider{
		header: relayHeaders(requestHeader, replicaID),
	}
	sourceEvents, sourceStream, err := sdkClient.StreamChat(relayCtx, chatID, &codersdk.StreamChatOptions{
		AfterID: ptr.Ref(int64(math.MaxInt64)),
	})
	if err != nil {
		relayCancel()
		return nil, nil, nil, xerrors.Errorf("dial relay stream: %w", err)
	}

	snapshot = make([]codersdk.ChatStreamEvent, 0, 100)

	// Wait briefly for the first event to handle the common
	// case where the remote side has buffered parts but hasn't
	// flushed them to the WebSocket yet.
	const drainTimeout = time.Second
	drainTimer := clk.NewTimer(drainTimeout, "drain")
	defer drainTimer.Stop()

drainInitial:
	for len(snapshot) < cap(snapshot) {
		select {
		case <-relayCtx.Done():
			_ = sourceStream.Close()
			relayCancel()
			return nil, nil, nil, xerrors.Errorf("dial relay stream: %w", relayCtx.Err())
		case event, ok := <-sourceEvents:
			if !ok {
				break drainInitial
			}
			if event.Type != codersdk.ChatStreamEventTypeMessagePart {
				continue
			}
			snapshot = append(snapshot, event)
			// After getting the first event, switch to
			// non-blocking drain for remaining buffered events.
			drainTimer.Stop()
			drainTimer.Reset(0)
		case <-drainTimer.C:
			break drainInitial
		}
	}

	events := make(chan codersdk.ChatStreamEvent, 128)

	go func() {
		defer close(events)
		defer relayCancel()
		defer func() {
			_ = sourceStream.Close()
		}()

		// No need to re-send snapshot events — they're
		// returned to the caller directly.
		for {
			select {
			case <-relayCtx.Done():
				return
			case event, ok := <-sourceEvents:
				if !ok {
					return
				}
				if event.Type != codersdk.ChatStreamEventTypeMessagePart {
					continue
				}
				select {
				case events <- event:
				case <-relayCtx.Done():
					return
				}
			}
		}
	}()

	cancelFn := func() {
		relayCancel()
		_ = sourceStream.Close()
	}
	return snapshot, events, cancelFn, nil
}

type relayHeaderTokenProvider struct {
	header http.Header
}

func (p relayHeaderTokenProvider) AsRequestOption() codersdk.RequestOption {
	return func(req *http.Request) {
		for key, values := range p.header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
}

func (p relayHeaderTokenProvider) SetDialOption(opts *websocket.DialOptions) {
	if opts.HTTPHeader == nil {
		opts.HTTPHeader = make(http.Header)
	}
	for key, values := range p.header {
		for _, value := range values {
			opts.HTTPHeader.Add(key, value)
		}
	}
}

func (p relayHeaderTokenProvider) GetSessionToken() string {
	return p.header.Get(codersdk.SessionTokenHeader)
}

func relayHeaders(source http.Header, replicaID uuid.UUID) http.Header {
	header := make(http.Header)
	if source != nil {
		for _, key := range []string{codersdk.SessionTokenHeader, authorizationHeader, cookieHeader} {
			for _, value := range source.Values(key) {
				header.Add(key, value)
			}
		}
	}
	header.Set(RelaySourceHeader, replicaID.String())
	return header
}
