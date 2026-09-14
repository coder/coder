package tailnet

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
