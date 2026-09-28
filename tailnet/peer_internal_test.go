package tailnet

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestPeerReqLoopPanic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	requests := make(chan *proto.CoordinateRequest, 1)
	responses := make(chan *proto.CoordinateResponse, 1)
	p := &peer{
		logger: logger,
		id:     uuid.New(),
		resps:  responses,
		reqs:   requests,
		sent:   make(map[uuid.UUID]*proto.Node),
	}
	core := newCore(logger)
	require.NoError(t, core.initPeer(p))
	t.Cleanup(func() { require.NoError(t, core.close()) })

	testutil.RequireSend(ctx, t, requests, &proto.CoordinateRequest{})
	err := p.reqLoop(ctx, logger, func(context.Context, *peer, *proto.CoordinateRequest) error {
		panic("private handler panic")
	})
	require.EqualError(t, err, CloseErrInternal)
	require.NoError(t, core.lostPeer(p, err.Error()))

	response := testutil.RequireReceive(ctx, t, responses)
	require.Equal(t, CloseErrInternal, response.Error)
	require.NotContains(t, response.Error, "private handler panic")
	_, ok := <-responses
	require.False(t, ok)
	core.mutex.RLock()
	_, ok = core.peers[p.id]
	core.mutex.RUnlock()
	require.False(t, ok)
}

func TestPeerCleanupPanic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	id := uuid.New()
	sink := &peerCleanupPanicSink{
		FakeSink: testutil.NewFakeSink(t),
		peerID:   id,
	}
	logger := slog.Make(sink).Leveled(slog.LevelDebug)
	c := NewCoordinator(logger).(*coordinator)
	t.Cleanup(func() { require.NoError(t, c.Close()) })

	requests, responses := c.Coordinate(ctx, id, t.Name(), SingleTailnetCoordinateeAuth{})
	close(requests)
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	testutil.TryReceive(ctx, t, done)

	entries := sink.Entries(func(entry slog.SinkEntry) bool {
		return entry.Level == slog.LevelError
	})
	require.Len(t, entries, 1)
	require.Equal(t, "panic coordinating peer (recovered)", entries[0].Message)
	fields := make(map[string]any)
	for _, field := range entries[0].Fields {
		fields[field.Name] = field.Value
	}
	require.Equal(t, id, fields["peer_id"])
	require.Equal(t, "private cleanup panic", fields["panic"])
	require.Contains(t, fields["stack"], "(*core).lostPeer")

	// Recovery must leave the core unlocked and the coordinator usable.
	healthyID := uuid.New()
	healthyRequests, healthyResponses := c.Coordinate(ctx, healthyID, t.Name(), SingleTailnetCoordinateeAuth{})
	close(healthyRequests)
	require.Nil(t, testutil.TryReceive(ctx, t, healthyResponses))

	// Shutdown must still remove the peer whose cleanup was interrupted.
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	require.NoError(t, testutil.RequireReceive(ctx, t, closed))
	response := testutil.RequireReceive(ctx, t, responses)
	require.Equal(t, CloseErrCoordinatorClose, response.Error)
	require.NotContains(t, response.Error, "private cleanup panic")
	require.Nil(t, testutil.TryReceive(ctx, t, responses))
	c.core.mutex.RLock()
	_, present := c.core.peers[id]
	c.core.mutex.RUnlock()
	require.False(t, present)
}

type peerCleanupPanicSink struct {
	*testutil.FakeSink
	peerID uuid.UUID
}

func (s *peerCleanupPanicSink) LogEntry(ctx context.Context, entry slog.SinkEntry) {
	if entry.Message == "lostPeer" {
		for _, field := range entry.Fields {
			if field.Name == "peer_id" && field.Value == s.peerID {
				panic("private cleanup panic")
			}
		}
	}
	s.FakeSink.LogEntry(ctx, entry)
}
