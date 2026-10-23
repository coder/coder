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

// TestStreamRelayForwarderDeliversPartsAfterStall stalls the stream loop past
// the send timeout on an open episode, then expects every part in order over
// the first connection.
func TestStreamRelayForwarderDeliversPartsAfterStall(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// The relay path holds fewer than episodeParts parts, so the buffer's
		// send blocks.
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
		require.Equal(t, cap(relay.parts), len(relay.parts), "the relay path did not fill")
		// drain waits first, so the buffer handles a fired timer before reads
		// resume.
		clock.Advance(sendTimeout).MustWait(ctx)
		drain()
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
		drain()
		require.Equal(t, int64(episodeParts+1), lastSeq, "the live part did not reach the stream loop")
		require.Equal(t, int32(1), dials.Load(), "the relay forwarder reconnected")
	})
}
