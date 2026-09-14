package tailnet

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

type coordinateeAuthFunc func(context.Context, *proto.CoordinateRequest) error

func (f coordinateeAuthFunc) Authorize(ctx context.Context, req *proto.CoordinateRequest) error {
	return f(ctx, req)
}

func TestConnIOHandleRequestRejectsBeforeMutation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	bindings := make(chan binding, 1)
	c := &connIO{
		id:       uuid.New(),
		coordCtx: ctx,
		peerCtx:  ctx,
		logger:   slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		bindings: bindings,
		auth:     agpl.SingleTailnetCoordinateeAuth{},
	}

	err := c.handleRequest(&proto.CoordinateRequest{
		UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: &proto.Node{
			PreferredDerp: 31002,
		}},
		ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{nil},
	})
	require.EqualError(t, err, "ready_for_handshake entry is required")
	select {
	case binding := <-bindings:
		t.Fatalf("unexpected binding: %+v", binding)
	default:
	}
}

func TestConnIORecvLoopPanicAfterAuthorization(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	bindings := make(chan binding, 1)
	requests := make(chan *proto.CoordinateRequest, 1)
	responses := make(chan *proto.CoordinateResponse, 1)
	rfhs := make(chan readyForHandshake)
	close(rfhs)
	authorized := make(chan struct{}, 1)
	peerID := uuid.New()
	destinationID := uuid.New()
	c := newConnIO(ctx, ctx, logger, bindings, make(chan tunnel, 1), rfhs,
		requests, responses, peerID, t.Name(), coordinateeAuthFunc(func(context.Context, *proto.CoordinateRequest) error {
			authorized <- struct{}{}
			return nil
		}))
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	c.setLatestMapping([]mapping{{peer: destinationID}})

	testutil.RequireSend(ctx, t, requests, &proto.CoordinateRequest{
		ReadyForHandshake: []*proto.CoordinateRequest_ReadyForHandshake{{Id: agpl.UUIDToByteSlice(destinationID)}},
	})
	testutil.RequireReceive(ctx, t, authorized)
	response := testutil.RequireReceive(ctx, t, responses)
	require.Equal(t, agpl.CloseErrInternal, response.Error)
	require.NotContains(t, response.Error, "send on closed channel")
	select {
	case _, ok := <-responses:
		require.False(t, ok)
	case <-ctx.Done():
		t.Fatal("response channel did not close")
	}
	select {
	case <-c.Done():
	case <-ctx.Done():
		t.Fatal("connection did not close")
	}
	withdrawn := testutil.RequireReceive(ctx, t, bindings)
	require.Equal(t, bKey(peerID), withdrawn.bKey)
	require.Equal(t, proto.CoordinateResponse_PeerUpdate_LOST, withdrawn.kind)
}
