package chatd

import (
	"context"
	"fmt"
	"testing"
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

// TestStreamRelayLivePartsAfterReselectsUnderBackpressure reselects episodes
// on a same-replica relay while the stream loop reads at most one part per
// reselect and the endpoint is blocked writing parts. Once the stream loop
// resumes, a part added afterwards must still reach it.
func TestStreamRelayLivePartsAfterReselectsUnderBackpressure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// newEpisode selects a new history version on every reselect.
		newEpisode bool
		reselects  int
	}{
		{name: "SameEpisode", reselects: 1000},
		{name: "NewEpisode", newEpisode: true, reselects: 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// More parts than the relay's buffers hold, so every
			// selection replays enough parts to block the endpoint.
			const episodeParts = 1000

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
					require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageReasoning(fmt.Sprintf("r%d", i))))
				}
				return streamRelayTarget{workerID: workerID, historyVersion: historyVersion, generationAttempt: 1}
			}

			local := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{
				Buffer: buffer,
				Logger: slogtest.Make(t, nil),
			})
			sessions := make(chan *streamPartsTransportSession, 1)
			dialer := func(ctx context.Context, input StreamPartsDialInput) (StreamPartsSession, error) {
				session, err := local(ctx, input)
				if err != nil {
					return nil, err
				}
				transportSession, ok := session.(*streamPartsTransportSession)
				if !ok {
					return nil, xerrors.Errorf("unexpected session type %T", session)
				}
				sessions <- transportSession
				return transportSession, nil
			}
			relay := newStreamRelayForwarder(chatID, nil, dialer, clock, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
			defer relay.Close()

			target := createEpisode(1)
			relay.Configure(ctx, target)
			session := testutil.RequireReceive(ctx, t, sessions)
			clientTransport, ok := session.transport.(streamPartsChannelClientTransport)
			require.True(t, ok, "unexpected transport type %T", session.transport)
			pipe := clientTransport.pipe
			endpointBlocked := func(context.Context) bool {
				return len(pipe.eventsCh) == cap(pipe.eventsCh) && len(session.parts) == cap(session.parts)
			}

			for i := range tc.reselects {
				require.True(t, testutil.Eventually(ctx, t, endpointBlocked, time.Millisecond),
					"endpoint never blocked before reselect %d", i)
				if tc.newEpisode {
					target = createEpisode(target.historyVersion + 1)
				}
				relay.Configure(ctx, target)
				select {
				case <-relay.Parts():
				default:
				}
			}

			key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: target.historyVersion, GenerationAttempt: 1}
			require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
			for {
				part := testutil.RequireReceive(ctx, t, relay.Parts())
				if part.HistoryVersion == target.historyVersion && part.Seq == episodeParts+1 {
					require.Equal(t, "live", part.Part.Text)
					return
				}
			}
		})
	}
}
