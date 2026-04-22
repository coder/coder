package tailnet

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet/proto"
)

// fakeCoordinateStream implements proto.DRPCTailnet_CoordinateStream for
// testing loopResp. Only Send, Context, and Close are exercised.
type fakeCoordinateStream struct {
	drpc.Stream
	ctx  context.Context
	mu   sync.Mutex
	sent []*proto.CoordinateResponse
}

func (f *fakeCoordinateStream) Context() context.Context { return f.ctx }

func (f *fakeCoordinateStream) Send(resp *proto.CoordinateResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Deep-copy PeerUpdates slice so later mutations don't affect recorded
	// values.
	clone := &proto.CoordinateResponse{Error: resp.Error}
	clone.PeerUpdates = make([]*proto.CoordinateResponse_PeerUpdate, len(resp.PeerUpdates))
	copy(clone.PeerUpdates, resp.PeerUpdates)
	f.sent = append(f.sent, clone)
	return nil
}

func (f *fakeCoordinateStream) Close() error { return nil }

func (f *fakeCoordinateStream) Recv() (*proto.CoordinateRequest, error) {
	<-f.ctx.Done()
	return nil, f.ctx.Err()
}

func (f *fakeCoordinateStream) CloseSend() error { return nil }

func (f *fakeCoordinateStream) MsgSend(drpc.Message, drpc.Encoding) error { return nil }
func (f *fakeCoordinateStream) MsgRecv(drpc.Message, drpc.Encoding) error { return nil }

func peerUpdate(id byte) *proto.CoordinateResponse_PeerUpdate {
	return &proto.CoordinateResponse_PeerUpdate{
		Id: []byte{id},
	}
}

func TestLoopRespBatchDrain(t *testing.T) {
	t.Parallel()

	t.Run("SingleMessage", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan *proto.CoordinateResponse, 8)
		stream := &fakeCoordinateStream{ctx: ctx}
		c := communicator{
			logger: slogtest.Make(t, nil),
			stream: stream,
			reqs:   make(chan<- *proto.CoordinateRequest),
			resps:  ch,
		}

		ch <- &proto.CoordinateResponse{
			PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate(1)},
		}
		// Close channel so loopResp exits after processing.
		close(ch)
		c.loopResp()
		cancel()

		require.Len(t, stream.sent, 1)
		assert.Len(t, stream.sent[0].PeerUpdates, 1)
	})

	t.Run("BatchMerge", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan *proto.CoordinateResponse, 8)
		stream := &fakeCoordinateStream{ctx: ctx}
		c := communicator{
			logger: slogtest.Make(t, nil),
			stream: stream,
			reqs:   make(chan<- *proto.CoordinateRequest),
			resps:  ch,
		}

		// Pre-fill 3 messages so they're all available for drain.
		ch <- &proto.CoordinateResponse{
			PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate(1)},
		}
		ch <- &proto.CoordinateResponse{
			PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate(2)},
		}
		ch <- &proto.CoordinateResponse{
			PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate(3)},
		}
		close(ch)
		c.loopResp()
		cancel()

		// All 3 should be merged into a single Send.
		require.Len(t, stream.sent, 1)
		assert.Len(t, stream.sent[0].PeerUpdates, 3)
	})

	t.Run("ErrorNotBatched", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan *proto.CoordinateResponse, 8)
		stream := &fakeCoordinateStream{ctx: ctx}
		c := communicator{
			logger: slogtest.Make(t, nil),
			stream: stream,
			reqs:   make(chan<- *proto.CoordinateRequest),
			resps:  ch,
		}

		ch <- &proto.CoordinateResponse{Error: "disconnect"}
		close(ch)
		c.loopResp()
		cancel()

		require.Len(t, stream.sent, 1)
		assert.Equal(t, "disconnect", stream.sent[0].Error)
		assert.Empty(t, stream.sent[0].PeerUpdates)
	})

	t.Run("PeerUpdatesThenError", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan *proto.CoordinateResponse, 8)
		stream := &fakeCoordinateStream{ctx: ctx}
		c := communicator{
			logger: slogtest.Make(t, nil),
			stream: stream,
			reqs:   make(chan<- *proto.CoordinateRequest),
			resps:  ch,
		}

		// First message is PeerUpdate, second is error — both buffered.
		ch <- &proto.CoordinateResponse{
			PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate(1), peerUpdate(2)},
		}
		ch <- &proto.CoordinateResponse{Error: "bye"}
		close(ch)
		c.loopResp()
		cancel()

		// Should produce 2 Sends: batched PeerUpdates, then error.
		require.Len(t, stream.sent, 2)
		assert.Len(t, stream.sent[0].PeerUpdates, 2)
		assert.Empty(t, stream.sent[0].Error)
		assert.Equal(t, "bye", stream.sent[1].Error)
	})
}
