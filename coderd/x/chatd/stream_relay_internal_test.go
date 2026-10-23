package chatd

import (
	"context"
	"fmt"
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

// TestStreamRelayForwarderDeliversPartsAfterStall stalls the stream loop for
// longer than the buffer's send timeout while the episode is open. The relay
// forwarder must then deliver every part, including one added after the
// stall, in order, over its first connection, and without a new Configure
// call.
func TestStreamRelayForwarderDeliversPartsAfterStall(t *testing.T) {
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

		// drain reads parts until every goroutine is blocked and no part is
		// waiting. It requires each seq to follow the previous one, so a
		// replay from seq 1 fails it.
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
		require.Equal(t, cap(relay.parts), len(relay.parts), "the relay path did not fill")
		// drain starts with synctest.Wait, so the buffer acts on any timer
		// the advance fires before the stream loop reads again.
		clock.Advance(sendTimeout).MustWait(ctx)
		drain()
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
		drain()
		require.Equal(t, int64(episodeParts+1), lastSeq, "the live part did not reach the stream loop")
		require.Equal(t, int32(1), dials.Load(), "the relay forwarder reconnected")
	})
}
