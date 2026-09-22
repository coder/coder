package tailnet

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestMapperPanic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	bindings := make(chan binding, 1)
	responses := make(chan *proto.CoordinateResponse, 2)
	c := newConnIO(ctx, ctx, logger, bindings, make(chan tunnel, 1), nil,
		make(chan *proto.CoordinateRequest), responses, uuid.New(), t.Name(), agpl.SingleTailnetCoordinateeAuth{})
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	coordinatorID := uuid.New()
	m := &mapper{
		ctx:        c.peerCtx,
		logger:     logger,
		c:          c,
		mappings:   make(chan []mapping),
		heartbeats: &heartbeats{self: coordinatorID},
		// A nil sent map forces a panic after the mapping has been selected.
		sent: nil,
	}
	go m.run()
	require.NoError(t, agpl.SendCtx(ctx, m.mappings, []mapping{{
		peer: uuid.New(), coordinator: coordinatorID,
		node: &proto.Node{}, kind: proto.CoordinateResponse_PeerUpdate_NODE,
	}}))
	response := testutil.RequireReceive(ctx, t, responses)
	require.Equal(t, agpl.CloseErrInternal, response.Error)
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
	require.Equal(t, bKey(c.id), withdrawn.bKey)
	require.Equal(t, proto.CoordinateResponse_PeerUpdate_LOST, withdrawn.kind)
}
