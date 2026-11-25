package chatd

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type fakeDialError struct {
	unrecoverable bool
}

func (fakeDialError) Error() string           { return "fake dial error" }
func (e fakeDialError) IsUnrecoverable() bool { return e.unrecoverable }

func TestStreamPartsDialUnrecoverable(t *testing.T) {
	t.Parallel()

	require.False(t, streamPartsDialUnrecoverable(nil))
	require.False(t, streamPartsDialUnrecoverable(xerrors.New("plain error")))
	require.False(t, streamPartsDialUnrecoverable(fakeDialError{unrecoverable: false}))
	require.True(t, streamPartsDialUnrecoverable(fakeDialError{unrecoverable: true}))
	require.True(t, streamPartsDialUnrecoverable(xerrors.Errorf("wrapped: %w", fakeDialError{unrecoverable: true})))
}

// TestStreamRelayForwarderDeliversPartsAfterStall stops reading relay.Parts()
// for longer than the send timeout on an open episode, then expects every part
// in order over the first connection.
func TestStreamRelayForwarderDeliversPartsAfterStall(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// The pipe, session and relay part channels hold 128 parts each, fewer
		// than episodeParts, so the buffer's send blocks.
		const (
			episodeParts = 1000
			sendTimeout  = 10 * time.Second
		)
		ctx := testutil.Context(t, testutil.WaitLong)
		clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
		buffer := messagepartbuffer.New(messagepartbuffer.Options{
			Clock:                 clock,
			SubscriberSendTimeout: sendTimeout,
		})
		defer buffer.Close()
		chatID := uuid.New()
		key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 1, GenerationAttempt: 1}
		require.NoError(t, buffer.CreateEpisode(key))
		for i := range episodeParts {
			require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(fmt.Sprintf("part %d", i+1))))
		}

		logger := slogtest.Make(t, nil)
		local := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{Buffer: buffer, Logger: logger})
		var dials atomic.Int32
		dialer := func(ctx context.Context, input StreamPartsDialInput) (StreamPartsSession, error) {
			dials.Add(1)
			return local(ctx, input)
		}
		relay := newStreamRelayForwarder(chatID, nil, dialer, clock, logger)
		defer relay.Close()
		relay.Configure(ctx, streamRelayTarget{
			workerID:          uuid.NullUUID{UUID: uuid.New(), Valid: true},
			historyVersion:    key.HistoryVersion,
			generationAttempt: key.GenerationAttempt,
		})

		// drain reads until every goroutine blocks. A replay from seq 1 fails
		// its order check.
		var lastSeq int64
		drain := func() {
			for {
				synctest.Wait()
				select {
				case part := <-relay.Parts():
					require.Equal(t, lastSeq+1, part.Seq, "message part out of order")
					lastSeq = part.Seq
				default:
					return
				}
			}
		}

		synctest.Wait()
		require.Equal(t, cap(relay.parts), len(relay.parts), "relay.parts did not fill")
		clock.Advance(sendTimeout).MustWait(ctx)
		drain()
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
		drain()
		require.Equal(t, int64(episodeParts+1), lastSeq, "the live part did not reach relay.Parts()")
		require.Equal(t, int32(1), dials.Load(), "the relay forwarder reconnected")
	})
}

type selectCountingSession struct {
	StreamPartsSession
	selects *atomic.Int32
}

func (s selectCountingSession) SelectEpisode(ctx context.Context, historyVersion, generationAttempt int64) error {
	s.selects.Add(1)
	return s.StreamPartsSession.SelectEpisode(ctx, historyVersion, generationAttempt)
}

// TestStreamRelayForwarderSelectsOnlyChangedTargets configures the same
// target repeatedly and expects one selection and one copy of the episode,
// then one more selection when the target changes.
func TestStreamRelayForwarderSelectsOnlyChangedTargets(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			episodeParts = 200
			reconfigures = 10
		)
		ctx := testutil.Context(t, testutil.WaitLong)
		clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
		buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
		defer buffer.Close()
		chatID := uuid.New()
		workerID := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		createEpisode := func(historyVersion int64) streamRelayTarget {
			key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: historyVersion, GenerationAttempt: 1}
			require.NoError(t, buffer.CreateEpisode(key))
			for i := range episodeParts {
				require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(fmt.Sprintf("part %d", i+1))))
			}
			return streamRelayTarget{workerID: workerID, historyVersion: historyVersion, generationAttempt: 1}
		}

		logger := slogtest.Make(t, nil)
		local := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{Buffer: buffer, Logger: logger})
		var selects atomic.Int32
		dialer := func(ctx context.Context, input StreamPartsDialInput) (StreamPartsSession, error) {
			session, err := local(ctx, input)
			if err != nil {
				return nil, err
			}
			return selectCountingSession{StreamPartsSession: session, selects: &selects}, nil
		}
		relay := newStreamRelayForwarder(chatID, nil, dialer, clock, logger)
		defer relay.Close()

		// received reads relay.Parts() until every goroutine blocks.
		received := func() int {
			n := 0
			for {
				synctest.Wait()
				select {
				case <-relay.Parts():
					n++
				default:
					return n
				}
			}
		}

		target := createEpisode(1)
		parts := 0
		for range reconfigures {
			relay.Configure(ctx, target)
			parts += received()
		}
		require.Equal(t, int32(1), selects.Load())
		require.Equal(t, episodeParts, parts)

		relay.Configure(ctx, createEpisode(2))
		require.Equal(t, episodeParts, received())
		require.Equal(t, int32(2), selects.Load())
	})
}

// TestStreamRelayForwarderDeliversLivePartsAfterSameEpisodeReconfigures
// stalls the stream loop until the buffers between the parts endpoint and
// relay.Parts() are full, configures the same episode many times while it
// reads at most one part each time, then resumes reading and expects a new
// part of the episode to arrive.
func TestStreamRelayForwarderDeliversLivePartsAfterSameEpisodeReconfigures(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			// The pipe, session and relay part channels hold 128 parts
			// each, fewer than episodeParts.
			episodeParts = 1000
			reconfigures = 1000
		)
		ctx := testutil.Context(t, testutil.WaitLong)
		clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
		buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
		defer buffer.Close()
		chatID := uuid.New()
		key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 1, GenerationAttempt: 1}
		require.NoError(t, buffer.CreateEpisode(key))
		for i := range episodeParts {
			require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText(fmt.Sprintf("part %d", i+1))))
		}

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		dialer := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{Buffer: buffer, Logger: logger})
		relay := newStreamRelayForwarder(chatID, nil, dialer, clock, logger)
		defer relay.Close()
		target := streamRelayTarget{
			workerID:          uuid.NullUUID{UUID: uuid.New(), Valid: true},
			historyVersion:    key.HistoryVersion,
			generationAttempt: key.GenerationAttempt,
		}
		relay.Configure(ctx, target)
		synctest.Wait()
		require.Equal(t, cap(relay.parts), len(relay.parts), "relay.parts did not fill")

		// readOne lets the forwarder run once, so it can take the pending
		// configuration.
		readOne := func() {
			select {
			case <-relay.Parts():
			default:
			}
			synctest.Wait()
		}
		for range reconfigures {
			relay.Configure(ctx, target)
			readOne()
		}

		// Read until every goroutine blocks, then add a live part and read
		// again.
		var lastSeq int64
		drain := func() {
			for {
				synctest.Wait()
				select {
				case part := <-relay.Parts():
					lastSeq = part.Seq
				default:
					return
				}
			}
		}
		drain()
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
		drain()
		require.Equal(t, int64(episodeParts+1), lastSeq, "the live part did not reach relay.Parts()")
	})
}

// closingSession replaces a session's parts channel with one the test closes.
type closingSession struct {
	StreamPartsSession
	parts chan StreamPart
}

func (s closingSession) Parts() <-chan StreamPart { return s.parts }

// TestStreamRelayForwarderDialBackoff configures the forwarder on a mock
// clock with failing dials and lost sessions and expects a worker to be
// redialed on the retry backoff, however often it is configured.
func TestStreamRelayForwarderDialBackoff(t *testing.T) {
	t.Parallel()

	const (
		step     = 10 * time.Millisecond
		duration = 20 * time.Second
	)
	type dial struct {
		at     time.Duration
		worker int
	}
	dialsAt := func(worker int, ms ...int) []dial {
		dials := make([]dial, 0, len(ms))
		for _, m := range ms {
			dials = append(dials, dial{at: time.Duration(m) * time.Millisecond, worker: worker})
		}
		return dials
	}
	// configure follows the initial configuration of worker 0 at time 0. Each
	// one advances the history version.
	type configure struct {
		at                time.Duration
		worker            int
		generationAttempt int64
	}
	sameWorkerEvery := func(interval time.Duration, generationAttempt func(tick int) int64) []configure {
		var configures []configure
		for tick := 1; time.Duration(tick)*interval <= duration; tick++ {
			configures = append(configures, configure{at: time.Duration(tick) * interval, generationAttempt: generationAttempt(tick)})
		}
		return configures
	}
	attemptOne := func(int) int64 { return 1 }
	backoff := dialsAt(0, 0, 100, 300, 700, 1500, 3100, 6300, 11300, 16300)

	cases := []struct {
		name       string
		configures []configure
		// succeedDial is the only dial that succeeds, counting from 1. Zero
		// fails all.
		succeedDial   int
		unrecoverable bool
		// closedBuffer accepts every dial over a closed message part buffer,
		// so each session closes after its selection.
		closedBuffer bool
		// closeSessionAt is the time since start at which the test closes the
		// connected session's parts channel. Zero never closes; a nonzero
		// value needs succeedDial to have connected by then.
		closeSessionAt time.Duration
		wantDials      []dial
		// connects expects the last target's parts on relay.Parts().
		connects bool
	}{
		{
			name:      "NoUpdates",
			wantDials: backoff,
		},
		{
			name:       "SameWorkerEverySecond",
			configures: sameWorkerEvery(time.Second, attemptOne),
			wantDials:  backoff,
		},
		{
			// Every other update has generation attempt 0, as between model
			// steps, which must not reset the backoff.
			name: "SameWorkerEvery100ms",
			configures: sameWorkerEvery(100*time.Millisecond, func(tick int) int64 {
				return int64(tick % 2)
			}),
			wantDials: backoff,
		},
		{
			name:       "WorkerChange",
			configures: []configure{{at: 2 * time.Second, worker: 1, generationAttempt: 1}},
			wantDials: append(
				dialsAt(0, 0, 100, 300, 700, 1500),
				dialsAt(1, 2000, 2100, 2300, 2700, 3500, 5100, 8300, 13300, 18300)...,
			),
		},
		{
			name:          "UnrecoverableNoUpdates",
			unrecoverable: true,
			wantDials:     dialsAt(0, 0),
		},
		{
			// The configuration during the backoff is the last one, and is
			// dialed when the backoff ends.
			name:          "UnrecoverableRecovery",
			configures:    []configure{{at: 50 * time.Millisecond, generationAttempt: 1}},
			succeedDial:   2,
			unrecoverable: true,
			wantDials:     dialsAt(0, 0, 100),
			connects:      true,
		},
		{
			name:         "AcceptThenClose",
			closedBuffer: true,
			wantDials:    backoff,
		},
		{
			// The session delivers a part before it closes, which resets the
			// backoff, so the first redial waits 100 ms. Configurations
			// every 100 ms from 1.1 s must not redial sooner.
			name:           "SessionClose",
			configures:     sameWorkerEvery(100*time.Millisecond, attemptOne)[10:],
			succeedDial:    2,
			closeSessionAt: time.Second,
			wantDials:      dialsAt(0, 0, 100, 1100, 1300, 1700, 2500, 4100, 7300, 12300, 17300),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				ctx := testutil.Context(t, testutil.WaitLong)
				clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
				start := clock.Now()
				buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
				defer buffer.Close()
				if tc.closedBuffer {
					buffer.Close()
				}
				chatID := uuid.New()
				workers := []uuid.UUID{uuid.New(), uuid.New()}

				logger := slogtest.Make(t, nil)
				local := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{Buffer: buffer, Logger: logger})
				var (
					dials   []dial
					session closingSession
				)
				dialer := func(ctx context.Context, input StreamPartsDialInput) (StreamPartsSession, error) {
					dials = append(dials, dial{at: clock.Since(start), worker: slices.Index(workers, input.WorkerID)})
					if !tc.closedBuffer && len(dials) != tc.succeedDial {
						return nil, fakeDialError{unrecoverable: tc.unrecoverable}
					}
					connected, err := local(ctx, input)
					if err != nil || tc.closeSessionAt == 0 {
						return connected, err
					}
					session = closingSession{StreamPartsSession: connected, parts: make(chan StreamPart)}
					return session, nil
				}
				relay := newStreamRelayForwarder(chatID, nil, dialer, clock, logger)
				defer relay.Close()

				target := streamRelayTarget{
					workerID:          uuid.NullUUID{UUID: workers[0], Valid: true},
					historyVersion:    1,
					generationAttempt: 1,
				}
				relay.Configure(ctx, target)
				configures := tc.configures
				for elapsed := step; elapsed <= duration; elapsed += step {
					// The forwarder arms its retry timer before the clock moves.
					synctest.Wait()
					clock.Advance(step).MustWait(ctx)
					if elapsed == tc.closeSessionAt {
						session.parts <- StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 1}
						close(session.parts)
					}
					if len(configures) > 0 && configures[0].at == elapsed {
						target = streamRelayTarget{
							workerID:          uuid.NullUUID{UUID: workers[configures[0].worker], Valid: true},
							historyVersion:    target.historyVersion + 1,
							generationAttempt: configures[0].generationAttempt,
						}
						relay.Configure(ctx, target)
						configures = configures[1:]
					}
				}
				synctest.Wait()
				require.Empty(t, configures, "configuration times must be multiples of the step")
				require.Equal(t, tc.wantDials, dials)

				if !tc.connects {
					return
				}
				key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: target.historyVersion, GenerationAttempt: target.generationAttempt}
				require.NoError(t, buffer.CreateEpisode(key))
				require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("part")))
				synctest.Wait()
				select {
				case part := <-relay.Parts():
					require.Equal(t, int64(1), part.Seq)
				default:
					t.Fatal("the relay forwarder did not deliver the last target's part")
				}
			})
		})
	}
}
