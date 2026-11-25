package chatd

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

const (
	streamRelayRetryInitialBackoff = 100 * time.Millisecond
	streamRelayRetryMaxBackoff     = 5 * time.Second
)

type streamRelayForwarder struct {
	chatID        uuid.UUID
	requestHeader http.Header
	dialer        StreamPartsDialer
	clock         quartz.Clock
	logger        slog.Logger

	parts chan StreamPart

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	configure chan streamRelayTarget
	closeOnce sync.Once
}

func newStreamRelayForwarder(
	chatID uuid.UUID,
	requestHeader http.Header,
	dialer StreamPartsDialer,
	clock quartz.Clock,
	logger slog.Logger,
) *streamRelayForwarder {
	if clock == nil {
		clock = quartz.NewReal()
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &streamRelayForwarder{
		chatID:        chatID,
		requestHeader: cloneHeader(requestHeader),
		dialer:        dialer,
		clock:         clock,
		logger:        logger,
		parts:         make(chan StreamPart, 128),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
		configure:     make(chan streamRelayTarget, 1),
	}
	go f.loop()
	return f
}

func (f *streamRelayForwarder) Parts() <-chan StreamPart {
	return f.parts
}

func (f *streamRelayForwarder) Configure(ctx context.Context, target streamRelayTarget) {
	if f == nil {
		return
	}
	// Drop any pending target so the buffered channel always holds the most
	// recent configuration.
	select {
	case <-f.configure:
	default:
	}
	select {
	case f.configure <- target:
	case <-f.ctx.Done():
	case <-ctx.Done():
	}
}

func (f *streamRelayForwarder) Close() {
	if f == nil {
		return
	}
	f.closeOnce.Do(func() {
		f.cancel()
		<-f.done
	})
}

func (f *streamRelayForwarder) loop() {
	defer close(f.done)
	defer close(f.parts)
	var (
		target       streamRelayTarget
		connected    streamRelayTarget
		session      StreamPartsSession
		sessionParts <-chan StreamPart
		retryTimer   *quartz.Timer
		retryC       <-chan time.Time
		retryBackoff = streamRelayRetryInitialBackoff
		// After a failed connection, backoffWorker is not dialed again before
		// backoffUntil. Configurations arrive at the chat's snapshot rate, so
		// one before backoffUntil only re-arms the retry timer for the rest of
		// the wait.
		backoffWorker uuid.NullUUID
		backoffUntil  time.Time
	)
	startRetry := func(d time.Duration) {
		retryTimer = f.clock.NewTimer(d, "chatd", "stream-relay-retry")
		retryC = retryTimer.C
	}
	resetBackoff := func() {
		retryBackoff = streamRelayRetryInitialBackoff
		backoffWorker = uuid.NullUUID{}
	}
	stopRetry := func() {
		if retryTimer != nil {
			retryTimer.Stop()
			retryTimer = nil
			retryC = nil
		}
	}
	defer stopRetry()
	closeSession := func() {
		if session != nil {
			_ = session.Close()
		}
		session = nil
		sessionParts = nil
		connected = streamRelayTarget{}
	}
	defer closeSession()
	scheduleRetry := func(timed bool) {
		if !target.needsRelay() || f.dialer == nil || retryTimer != nil {
			return
		}
		backoffWorker = target.workerID
		backoffUntil = f.clock.Now().Add(retryBackoff)
		if timed {
			startRetry(retryBackoff)
		}
		if retryBackoff < streamRelayRetryMaxBackoff {
			retryBackoff *= 2
			if retryBackoff > streamRelayRetryMaxBackoff {
				retryBackoff = streamRelayRetryMaxBackoff
			}
		}
	}
	connect := func(ctx context.Context) {
		stopRetry()
		if !target.needsRelay() {
			closeSession()
			return
		}
		if f.dialer == nil {
			return
		}
		if session != nil && connected.workerID.Valid && sameNullUUID(connected.workerID, target.workerID) {
			// runStreamSync configures after every newer snapshot; a selection resends
			// from seq 1. Skipping relies on messagepartbuffer keeping open-episode subscribers.
			if connected.sameEpisode(target) {
				return
			}
			if err := session.SelectEpisode(ctx, target.historyVersion, target.generationAttempt); err != nil {
				f.logger.Warn(ctx, "failed to select stream parts episode",
					slog.F("chat_id", f.chatID),
					slog.F("history_version", target.historyVersion),
					slog.F("generation_attempt", target.generationAttempt),
					slog.Error(err),
				)
				closeSession()
				scheduleRetry(true)
				return
			}
			connected = target
			resetBackoff()
			return
		}
		if !sameNullUUID(backoffWorker, target.workerID) {
			resetBackoff()
		} else if wait := f.clock.Until(backoffUntil); wait > 0 {
			startRetry(wait)
			return
		}
		closeSession()
		newSession, err := f.dialer(ctx, StreamPartsDialInput{
			ChatID:        f.chatID,
			WorkerID:      target.workerID.UUID,
			RequestHeader: cloneHeader(f.requestHeader),
		})
		if err != nil {
			f.logger.Warn(ctx, "failed to dial stream parts relay",
				slog.F("chat_id", f.chatID),
				slog.F("worker_id", target.workerID.UUID),
				slog.Error(err),
			)
			// Unrecoverable dial errors (e.g. auth failures) will not
			// succeed on retry with the same inputs, so arm no timer. A
			// later configuration for the same worker arms one for the rest
			// of the backoff.
			scheduleRetry(!streamPartsDialUnrecoverable(err))
			return
		}
		session = newSession
		sessionParts = newSession.Parts()
		connected = streamRelayTarget{workerID: target.workerID}
		if err := session.SelectEpisode(ctx, target.historyVersion, target.generationAttempt); err != nil {
			f.logger.Warn(ctx, "failed to select stream parts episode",
				slog.F("chat_id", f.chatID),
				slog.F("history_version", target.historyVersion),
				slog.F("generation_attempt", target.generationAttempt),
				slog.Error(err),
			)
			closeSession()
			scheduleRetry(true)
			return
		}
		connected = target
		resetBackoff()
	}

	for {
		select {
		case <-f.ctx.Done():
			return
		case nextTarget := <-f.configure:
			target = nextTarget
			connect(f.ctx)
		case <-retryC:
			retryTimer = nil
			retryC = nil
			connect(f.ctx)
		case part, ok := <-sessionParts:
			if !ok {
				closeSession()
				scheduleRetry(true)
				continue
			}
			if !connected.sameEpisode(target) ||
				part.HistoryVersion != target.historyVersion ||
				part.GenerationAttempt != target.generationAttempt {
				continue
			}
			select {
			case f.parts <- part:
			case <-f.ctx.Done():
				return
			}
		}
	}
}

func (t streamRelayTarget) needsRelay() bool {
	return t.workerID.Valid && t.generationAttempt > 0
}

// streamPartsDialUnrecoverable reports whether a dial error signals that
// retrying with the same inputs is futile, such as an auth failure. Dialers
// opt in by returning errors that implement IsUnrecoverable.
func streamPartsDialUnrecoverable(err error) bool {
	var unrecoverable interface{ IsUnrecoverable() bool }
	return errors.As(err, &unrecoverable) && unrecoverable.IsUnrecoverable()
}

func (t streamRelayTarget) sameEpisode(other streamRelayTarget) bool {
	return sameNullUUID(t.workerID, other.workerID) &&
		t.historyVersion == other.historyVersion &&
		t.generationAttempt == other.generationAttempt
}
