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
	)
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
	scheduleRetry := func() {
		if !target.needsRelay() || f.dialer == nil || retryTimer != nil {
			return
		}
		retryTimer = f.clock.NewTimer(retryBackoff, "chatd", "stream-relay-retry")
		retryC = retryTimer.C
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
			if err := session.SelectEpisode(ctx, target.historyVersion, target.generationAttempt); err != nil {
				f.logger.Warn(ctx, "failed to select stream parts episode",
					slog.F("chat_id", f.chatID),
					slog.F("history_version", target.historyVersion),
					slog.F("generation_attempt", target.generationAttempt),
					slog.Error(err),
				)
				closeSession()
				scheduleRetry()
				return
			}
			connected = target
			retryBackoff = streamRelayRetryInitialBackoff
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
			// succeed on retry with the same inputs, so wait for the next
			// configuration instead of scheduling a retry.
			if !streamPartsDialUnrecoverable(err) {
				scheduleRetry()
			}
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
			scheduleRetry()
			return
		}
		connected = target
		retryBackoff = streamRelayRetryInitialBackoff
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
				scheduleRetry()
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
