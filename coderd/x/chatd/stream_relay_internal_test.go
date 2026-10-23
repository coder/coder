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

// TestStreamRelayLivePartsAfterSameEpisodeReselects selects the current
// episode again many times on a same-replica relay while the endpoint is
// blocked sending parts and the test, in place of the stream loop, reads at
// most one part per selection. A part added afterwards must still reach the
// stream loop.
func TestStreamRelayLivePartsAfterSameEpisodeReselects(t *testing.T) {
	t.Parallel()

	const (
		// episodeParts exceeds what the relay's buffers hold, so every
		// selection replays enough parts to block the endpoint.
		episodeParts = 1000
		reselects    = 1000
	)

	ctx := testutil.Context(t, testutil.WaitLong)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	buffer := messagepartbuffer.New(messagepartbuffer.Options{Clock: clock})
	defer buffer.Close()

	chatID := uuid.New()
	key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 1, GenerationAttempt: 1}
	require.NoError(t, buffer.CreateEpisode(key))
	for i := range episodeParts {
		require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageReasoning(fmt.Sprintf("r%d", i))))
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

	target := streamRelayTarget{
		workerID:          uuid.NullUUID{UUID: uuid.New(), Valid: true},
		historyVersion:    key.HistoryVersion,
		generationAttempt: key.GenerationAttempt,
	}
	relay.Configure(ctx, target)
	session := testutil.RequireReceive(ctx, t, sessions)
	clientTransport, ok := session.transport.(streamPartsChannelClientTransport)
	require.True(t, ok, "unexpected transport type %T", session.transport)
	pipe := clientTransport.pipe
	endpointBlocked := func(context.Context) bool {
		return len(pipe.eventsCh) == cap(pipe.eventsCh) && len(session.parts) == cap(session.parts)
	}

	for i := range reselects {
		require.True(t, testutil.Eventually(ctx, t, endpointBlocked, time.Millisecond),
			"endpoint never blocked before reselect %d", i)
		relay.Configure(ctx, target)
		select {
		case <-relay.Parts():
		default:
		}
	}

	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
	for {
		part := testutil.RequireReceive(ctx, t, relay.Parts())
		if part.Seq == episodeParts+1 {
			require.Equal(t, "live", part.Part.Text)
			return
		}
	}
}
