package chatd

import (
	"fmt"
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

// TestStreamRelayForwarderRecoversDroppedSubscription stalls the stream loop
// for longer than the buffer's send timeout. The buffer drops the parts
// endpoint's subscription, and a part added afterwards must still reach the
// stream loop without a new Configure call.
func TestStreamRelayForwarderRecoversDroppedSubscription(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// The episode holds more parts than the relay path to the stream
		// loop can hold, so the buffer's send blocks while the stream loop
		// does not read.
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
		dialer := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{Buffer: buffer, Logger: logger})
		relay := newStreamRelayForwarder(chatID, nil, dialer, clock, logger)
		defer relay.Close()
		relay.Configure(ctx, streamRelayTarget{
			workerID:          uuid.NullUUID{UUID: uuid.New(), Valid: true},
			historyVersion:    key.HistoryVersion,
			generationAttempt: key.GenerationAttempt,
		})

		// drain reads parts the way the stream loop does: it skips seqs it
		// has already seen and rejects gaps. It returns once every goroutine
		// is blocked and no part is waiting.
		var lastSeq int64
		drain := func() {
			for {
				synctest.Wait()
				select {
				case part := <-relay.Parts():
					if part.Seq <= lastSeq {
						continue
					}
					require.Equal(t, lastSeq+1, part.Seq, "message part sequence gap")
					lastSeq = part.Seq
				default:
					return
				}
			}
		}

		synctest.Wait()
		// The mock clock delivers the timer value from another goroutine.
		// drain starts with synctest.Wait, so the buffer acts on it before
		// the stream loop reads again.
		clock.Advance(sendTimeout).MustWait(ctx)
		drain()
		require.Less(t, lastSeq, int64(episodeParts), "the stall did not drop the subscription")

		// The relay forwarder redials after its retry backoff.
		clock.Advance(streamRelayRetryInitialBackoff).MustWait(ctx)
		drain()
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
		drain()
		require.Equal(t, int64(episodeParts+1), lastSeq, "the live part did not reach the stream loop")
	})
}
