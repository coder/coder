package chatd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
	"github.com/coder/retry"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// RelaySourceHeader marks replica-relayed stream requests.
const RelaySourceHeader = "X-Coder-Relay-Source-Replica"

const (
	authorizationHeader = "Authorization"
	cookieHeader        = "Cookie"

	// relayDrainTimeout is how long an established relay is
	// kept open after the chat leaves running state, giving
	// buffered snapshot events time to be forwarded before
	// the relay is torn down.
	relayDrainTimeout = 200 * time.Millisecond

	// Retry knobs for the cross-replica relay handshake. We use the
	// defaults from github.com/coder/retry (φ-growth, no jitter) via
	// retry.New(relayRetryFloor, relayRetryCeil) but drive the delay
	// ourselves because retry.Retrier.Wait uses time.After and the
	// merge loop runs on a quartz.Clock for deterministic tests.
	//
	// relayRetryFloor preserves the previous fixed-delay responsiveness
	// for the first reconnect. relayRetryCeil caps the delay so a
	// permanently-broken replica cannot silently stall events for too
	// long. relayRetryMaxAttempts bounds how many consecutive
	// intermittent failures we tolerate before tearing down the relay
	// leg so the client can reconnect cleanly.
	relayRetryFloor       = 500 * time.Millisecond
	relayRetryCeil        = 15 * time.Second
	relayRetryMaxAttempts = 6
)

// RelayDialError is returned from dialRelay when the handshake to a
// peer replica fails. It carries the HTTP status code (when
// available) so the reconnect loop can decide whether to keep
// retrying or tear the relay leg down.
//
// HTTPStatus is 0 when the failure occurred before a response was
// received (DNS, TCP, TLS, timeout, context cancel).
type RelayDialError struct {
	HTTPStatus int
	Err        error
}

func (e *RelayDialError) Error() string { return e.Err.Error() }
func (e *RelayDialError) Unwrap() error { return e.Err }

// IsUnrecoverable reports whether the dial error is expected to keep
// failing with the same captured session token. 401 and 403 are the
// canonical cases — the user's session is dead or the peer won't
// authorize them. Everything else (5xx, 429, network, context) is
// treated as intermittent and retried with backoff.
func (e *RelayDialError) IsUnrecoverable() bool {
	return e.HTTPStatus == http.StatusUnauthorized ||
		e.HTTPStatus == http.StatusForbidden
}

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
	return func(ctx context.Context, params osschatd.SubscribeFnParams) <-chan codersdk.ChatStreamEvent {
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
		// Channel for async relay establishment.
		type relayResult struct {
			parts    <-chan codersdk.ChatStreamEvent
			cancel   func()
			workerID uuid.UUID // the worker this dial targeted
			// err is non-nil when the dial failed. The retry loop
			// uses it to classify the failure (via *RelayDialError)
			// and decide whether to back off or tear down.
			err error
		}
		relayReadyCh := make(chan relayResult, 4)

		// retryState drives the backoff + give-up policy for
		// the reconnect timer. Reset on successful dial or when
		// closeRelay is called (e.g. chat migrated to a new
		// worker) so the first retry after a fresh target
		// starts at the floor delay.
		retryState := newRelayRetryState()
		// Per-dial context so in-flight dials can be canceled when
		// a new dial is initiated or the relay is closed.
		var dialCancel context.CancelFunc

		// expectedWorkerID tracks which replica we expect the next
		// relay result to target. Stale results are discarded.
		var expectedWorkerID uuid.UUID

		// Reconnect timer state.
		var reconnectTimer *quartz.Timer
		var reconnectCh <-chan time.Time

		// drainAndClose is set when the chat transitions away
		// from running while a relay dial is still in progress.
		// Instead of canceling the dial immediately, we let it
		// complete so the snapshot of buffered message_parts
		// can be forwarded to the subscriber.
		var drainAndClose bool

		// Drain timer state. When the relay connects in
		// drain-and-close mode, a short timer is started.
		// During this window the normal relayPartsCh case
		// forwards buffered snapshot events. When the timer
		// fires the relay is torn down.
		var drainTimer *quartz.Timer
		var drainTimerCh <-chan time.Time

		// Helper to close relay and stop any pending reconnect
		// timer.
		closeRelay := func() {
			// Cancel any in-flight dial goroutine first.
			if dialCancel != nil {
				dialCancel()
				dialCancel = nil
			}
			// Drain all buffered relay results from canceled dials.
			for {
				select {
				case result := <-relayReadyCh:
					if result.cancel != nil {
						result.cancel()
					}
				default:
					goto drained
				}
			}
		drained:
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
			if drainTimer != nil {
				drainTimer.Stop()
				drainTimer = nil
				drainTimerCh = nil
			}
			drainAndClose = false
		}

		// openRelayAsync dials the remote replica in a background
		// goroutine and delivers the result on relayReadyCh so the
		// main select loop is never blocked by network I/O.
		openRelayAsync := func(workerID uuid.UUID) {
			if cfg.dial() == nil {
				return
			}
			// A new target means the retry history from the old
			// target doesn't apply; start the backoff counter
			// over. We scope the reset here rather than inside
			// closeRelay so back-to-back dials against the same
			// worker keep the attempt counter and correctly trip
			// the cap.
			if workerID != expectedWorkerID {
				retryState.reset()
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
					// can schedule a reconnect attempt, or tear the
					// relay down if the error is unrecoverable or
					// the retry cap has been hit.
					select {
					case relayReadyCh <- relayResult{workerID: workerID, err: err}:
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

		// scheduleRelayReconnect arms a timer so the select loop
		// can re-check chat status and reopen the relay without
		// spinning in a tight loop. The delay is supplied by the
		// caller so the failed-dial branch can apply backoff while
		// transient error branches (DB lookup failures, drop-on-
		// close) stay at the floor delay.
		scheduleRelayReconnect := func(delay time.Duration) {
			if cfg.dial() == nil {
				return
			}
			if reconnectTimer != nil {
				reconnectTimer.Stop()
			}
			reconnectTimer = cfg.clock().NewTimer(delay, "reconnect")
			reconnectCh = reconnectTimer.C
		}

		// sendRelayTerminalError enqueues a single error event on
		// mergedEvents describing an unrecoverable relay failure.
		// Callers return from the merge goroutine afterwards so the
		// defer close(mergedEvents) fires. The OSS merge loop nils
		// relayEvents when mergedEvents closes, so pubsub / local
		// sources continue to feed the outer stream — only the
		// relay leg is torn down.
		sendRelayTerminalError := func(msg string) {
			select {
			case mergedEvents <- codersdk.ChatStreamEvent{
				Type:   codersdk.ChatStreamEventTypeError,
				ChatID: chatID,
				Error:  &codersdk.ChatStreamError{Message: msg},
			}:
			case <-ctx.Done():
			}
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
					// failed — classify the error to decide
					// whether to schedule a backoff retry, emit a
					// terminal error and tear the relay leg down
					// (unrecoverable / cap reached), or simply
					// drop the stale drain.
					if result.parts == nil {
						if drainAndClose {
							// Dial failed and we were only
							// waiting to drain — nothing to do.
							drainAndClose = false
							continue
						}
						var dialErr *RelayDialError
						if xerrors.As(result.err, &dialErr) && dialErr.IsUnrecoverable() {
							logger.Warn(ctx, "relay dial unrecoverable; tearing down relay leg",
								slog.F("chat_id", chatID),
								slog.F("worker_id", result.workerID),
								slog.F("http_status", dialErr.HTTPStatus),
							)
							sendRelayTerminalError(fmt.Sprintf(
								"relay authentication failed (status %d)",
								dialErr.HTTPStatus,
							))
							return
						}
						delay, giveUp := retryState.next()
						if giveUp {
							logger.Warn(ctx, "relay dial retry cap reached; tearing down relay leg",
								slog.F("chat_id", chatID),
								slog.F("worker_id", result.workerID),
								slog.F("attempts", relayRetryMaxAttempts),
							)
							sendRelayTerminalError(fmt.Sprintf(
								"relay connection failed after %d attempts",
								relayRetryMaxAttempts,
							))
							return
						}
						scheduleRelayReconnect(delay)
						continue
					}
					// An async relay dial completed; reset the
					// retry state and swap in the new relay
					// channel.
					retryState.reset()
					if relayCancel != nil {
						relayCancel()
						relayCancel = nil
					}
					relayParts = result.parts
					relayCancel = result.cancel
					if drainAndClose {
						// The chat is no longer running on
						// the remote worker, but the dial
						// completed. Verify no new worker
						// has claimed the chat before we
						// drain stale parts.
						currentChat, dbErr := params.DB.GetChatByID(ctx, chatID)
						if dbErr != nil {
							logger.Warn(ctx, "failed to check chat status for relay drain",
								slog.F("chat_id", chatID),
								slog.Error(dbErr),
							)
						}
						if dbErr == nil && currentChat.Status == database.ChatStatusRunning &&
							currentChat.WorkerID.Valid &&
							currentChat.WorkerID.UUID != params.WorkerID {
							// A new worker picked up the chat;
							// discard the stale relay and let
							// openRelayAsync handle the new one.
							closeRelay()
						} else {
							// Chat is still idle — drain the
							// buffered snapshot before closing.
							if drainTimer != nil {
								drainTimer.Stop()
							}
							drainTimer = cfg.clock().NewTimer(relayDrainTimeout, "drain")
							drainTimerCh = drainTimer.C
							drainAndClose = false
						}
					}
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
						// stream. The same retry state
						// bounds the DB-error loop too so a
						// persistently broken DB eventually
						// tears the relay down instead of
						// spinning forever.
						delay, giveUp := retryState.next()
						if giveUp {
							sendRelayTerminalError(fmt.Sprintf(
								"relay reconnect failed after %d attempts",
								relayRetryMaxAttempts,
							))
							return
						}
						scheduleRelayReconnect(delay)
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
						switch {
						case dialCancel != nil && relayParts == nil:
							// In-progress dial: let it complete
							// so its snapshot can be forwarded.
							drainAndClose = true
						case relayParts != nil:
							// Active relay: give it a short
							// window to deliver any remaining
							// buffered parts before closing.
							if drainTimer != nil {
								drainTimer.Stop()
							}
							drainTimer = cfg.clock().NewTimer(relayDrainTimeout, "drain")
							drainTimerCh = drainTimer.C
						default:
							closeRelay()
						}
					}
				case <-drainTimerCh:
					drainTimerCh = nil
					drainTimer = nil
					closeRelay()
					drainTimerCh = nil
					drainTimer = nil
					closeRelay()
				case event, ok := <-relayPartsCh:
					if !ok {
						if relayCancel != nil {
							relayCancel()
							relayCancel = nil
						}
						relayParts = nil
						// Schedule reconnection with backoff
						// instead of giving up. The same retry
						// state applies so a relay that
						// repeatedly drops is torn down.
						delay, giveUp := retryState.next()
						if giveUp {
							sendRelayTerminalError(fmt.Sprintf(
								"relay connection failed after %d attempts",
								relayRetryMaxAttempts,
							))
							return
						}
						scheduleRelayReconnect(delay)
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

		// Cleanup is driven by ctx cancellation: the merge
		// goroutine owns all relay state (reconnectTimer,
		// relayCancel, dialCancel, etc.) and tears it down
		// via defer closeRelay() when ctx is done.
		return mergedEvents
	}
}

// relayRetryState drives the backoff + give-up policy for the
// cross-replica relay reconnect loop. It wraps github.com/coder/retry
// to reuse its φ-growth defaults but computes the next delay
// without blocking so the merge loop can schedule a quartz.Clock
// timer itself (and tests can advance that clock deterministically).
type relayRetryState struct {
	retrier  *retry.Retrier
	attempts int
}

func newRelayRetryState() *relayRetryState {
	return &relayRetryState{
		retrier: retry.New(relayRetryFloor, relayRetryCeil),
	}
}

// next advances the backoff and returns the delay for the next
// reconnect attempt. giveUp is true once attempts exceed
// relayRetryMaxAttempts — callers should tear the relay leg down
// instead of scheduling more reconnects.
//
// This mirrors the math in retry.Retrier.Wait (see
// github.com/coder/retry/retrier.go) but never blocks.
func (s *relayRetryState) next() (delay time.Duration, giveUp bool) {
	s.attempts++
	if s.attempts > relayRetryMaxAttempts {
		return 0, true
	}
	r := s.retrier
	d := time.Duration(float64(r.Delay) * r.Rate)
	if d > r.Ceil {
		d = r.Ceil
	}
	if d < r.Floor {
		d = r.Floor
	}
	r.Delay = d
	return d, false
}

// reset restores the retry state to the floor delay and zero
// attempts. Called after a successful dial or when the relay target
// changes (e.g. the chat migrates to a new worker).
func (s *relayRetryState) reset() {
	s.retrier.Reset()
	s.attempts = 0
}

// afterIDMaxInt64 is passed as the after_id query parameter when
// dialing a peer replica. It tells the peer to skip the full
// message-history snapshot — relays only need live message_part
// events.
const afterIDMaxInt64 = "9223372036854775807"

// dialRelay opens a WebSocket relay connection to the replica
// identified by workerID and returns a snapshot of buffered
// message_part events plus a live channel of subsequent events.
//
// On handshake failure it returns an error that unwraps to
// *RelayDialError carrying the peer's HTTP status code (or 0 for
// pre-response failures). Callers use RelayDialError.IsUnrecoverable
// to decide whether the retry loop should give up.
//
// We call websocket.Dial directly rather than routing through
// codersdk.ExperimentalClient.StreamChat because that wrapper hides
// the *http.Response we need for status classification, and the
// relay is an internal replica-to-replica channel where the SDK's
// browser-shaped ergonomics add no value.
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
		return nil, nil, nil, &RelayDialError{
			Err: xerrors.New("dial relay stream: worker replica not found"),
		}
	}

	wsURL, err := buildRelayURL(address, chatID)
	if err != nil {
		return nil, nil, nil, &RelayDialError{
			Err: xerrors.Errorf("dial relay stream: %w", err),
		}
	}

	replicaID := cfg.ReplicaIDFn()
	headers := make(http.Header, 2)
	headers.Set(codersdk.SessionTokenHeader, extractSessionToken(requestHeader))
	headers.Set(RelaySourceHeader, replicaID.String())

	relayCtx, relayCancel := context.WithCancel(ctx)
	conn, resp, dialErr := websocket.Dial(relayCtx, wsURL, &websocket.DialOptions{
		HTTPClient:      cfg.ReplicaHTTPClient,
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionDisabled,
	})
	status := 0
	if resp != nil {
		status = resp.StatusCode
		// The websocket library closes resp.Body on success; on
		// failure we close it ourselves so we don't leak the TCP
		// connection.
		if dialErr != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	if dialErr != nil {
		relayCancel()
		return nil, nil, nil, &RelayDialError{
			HTTPStatus: status,
			Err:        xerrors.Errorf("dial relay stream: %w", dialErr),
		}
	}
	// Match the server's 4 MiB read limit in codersdk.StreamChat so
	// large message_part batches don't trip the default 32 KiB cap.
	conn.SetReadLimit(1 << 22)

	snapshot = make([]codersdk.ChatStreamEvent, 0, 100)

	// sourceEvents is the flattened batch→event channel. A small
	// goroutine reads batches off the websocket and fans them out;
	// callers see a single event stream identical to the shape the
	// old SDK call produced.
	sourceEvents := make(chan codersdk.ChatStreamEvent, 128)
	go func() {
		defer close(sourceEvents)
		for {
			var batch []codersdk.ChatStreamEvent
			if readErr := wsjson.Read(relayCtx, conn, &batch); readErr != nil {
				return
			}
			for _, event := range batch {
				select {
				case sourceEvents <- event:
				case <-relayCtx.Done():
					return
				}
			}
		}
	}()

	closeSource := func() {
		relayCancel()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}

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
			closeSource()
			return nil, nil, nil, &RelayDialError{
				Err: xerrors.Errorf("dial relay stream: %w", relayCtx.Err()),
			}
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
		defer closeSource()

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

	return snapshot, events, closeSource, nil
}

// buildRelayURL builds the websocket URL for the chat stream
// endpoint on a peer replica. It maps http(s) schemes to ws(s).
func buildRelayURL(address string, chatID uuid.UUID) (string, error) {
	u, err := url.Parse(address)
	if err != nil {
		return "", xerrors.Errorf("parse relay address %q: %w", address, err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already a websocket URL, leave as-is.
	default:
		return "", xerrors.Errorf("unsupported relay address scheme %q", u.Scheme)
	}
	u.Path = fmt.Sprintf("/api/experimental/chats/%s/stream", chatID)
	q := u.Query()
	q.Set("after_id", afterIDMaxInt64)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// extractSessionToken returns the session token carried by the
// given request headers. It mirrors the priority order used by
// apiKeyMiddleware: cookie, then Coder-Session-Token header, then
// Authorization: Bearer header.
func extractSessionToken(header http.Header) string {
	if header == nil {
		return ""
	}
	// Cookie (browser WebSocket upgrade — most common relay case).
	if raw := header.Get(cookieHeader); raw != "" {
		r := &http.Request{Header: http.Header{cookieHeader: {raw}}}
		if c, err := r.Cookie(codersdk.SessionTokenCookie); err == nil && c.Value != "" {
			return c.Value
		}
	}
	// Coder-Session-Token header (SDK / CLI callers).
	if v := header.Get(codersdk.SessionTokenHeader); v != "" {
		return v
	}
	// Authorization: Bearer <token>.
	if v := header.Get(authorizationHeader); len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}
