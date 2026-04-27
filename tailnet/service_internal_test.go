package tailnet

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/coder/v2/testutil"
)

// fakeCoordinateStream is a minimal DRPCTailnet_CoordinateStream that
// records Send() calls so tests can assert how loopResp batches responses.
type fakeCoordinateStream struct {
	ctx context.Context

	mu    sync.Mutex
	sends []*proto.CoordinateResponse
	// sendErr, if non-nil, is returned from the next Send call.
	sendErr error
	// sendCh is closed to signal a send happened.
	sendCh chan struct{}

	closed bool
}

func newFakeCoordinateStream(ctx context.Context) *fakeCoordinateStream {
	return &fakeCoordinateStream{
		ctx:    ctx,
		sendCh: make(chan struct{}, 64),
	}
}

func (f *fakeCoordinateStream) Context() context.Context { return f.ctx }

func (f *fakeCoordinateStream) Send(r *proto.CoordinateResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sends = append(f.sends, r)
	select {
	case f.sendCh <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeCoordinateStream) Recv() (*proto.CoordinateRequest, error) {
	<-f.ctx.Done()
	return nil, io.EOF
}

func (f *fakeCoordinateStream) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (*fakeCoordinateStream) CloseSend() error                          { return nil }
func (*fakeCoordinateStream) MsgSend(drpc.Message, drpc.Encoding) error { return nil }
func (*fakeCoordinateStream) MsgRecv(drpc.Message, drpc.Encoding) error { return nil }

func (f *fakeCoordinateStream) snapshot() []*proto.CoordinateResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*proto.CoordinateResponse, len(f.sends))
	copy(out, f.sends)
	return out
}

// runLoopResp starts loopResp on a fresh communicator backed by the
// returned channel and stream. Returns a function to wait for it to exit.
func runLoopResp(ctx context.Context, t *testing.T) (chan *proto.CoordinateResponse, *fakeCoordinateStream, func()) {
	t.Helper()
	resps := make(chan *proto.CoordinateResponse, 64)
	stream := newFakeCoordinateStream(ctx)
	c := communicator{
		logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		stream: stream,
		reqs:   make(chan *proto.CoordinateRequest, 1),
		resps:  resps,
	}
	done := make(chan struct{})
	go func() {
		c.loopResp()
		close(done)
	}()
	wait := func() {
		select {
		case <-done:
		case <-time.After(testutil.WaitShort):
			t.Fatal("loopResp did not exit")
		}
	}
	return resps, stream, wait
}

// waitForSends blocks until the stream has recorded at least n Send calls
// or the timeout elapses.
func waitForSends(t *testing.T, stream *fakeCoordinateStream, n int) {
	t.Helper()
	deadline := time.After(testutil.WaitShort)
	for {
		if len(stream.snapshot()) >= n {
			return
		}
		select {
		case <-stream.sendCh:
		case <-deadline:
			t.Fatalf("timed out waiting for %d sends, got %d", n, len(stream.snapshot()))
		}
	}
}

func peerUpdate(reason string) *proto.CoordinateResponse_PeerUpdate {
	return &proto.CoordinateResponse_PeerUpdate{Reason: reason}
}

func TestLoopResp_SingleMessage(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	resps, stream, wait := runLoopResp(ctx, t)
	resps <- &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate("a")}}

	waitForSends(t, stream, 1)
	got := stream.snapshot()
	require.Len(t, got, 1)
	require.Len(t, got[0].PeerUpdates, 1)
	assert.Equal(t, "a", got[0].PeerUpdates[0].Reason)

	close(resps)
	wait()
}

func TestLoopResp_BatchMerge(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Use a stream that blocks the first Send until we've queued more
	// responses. This guarantees those queued responses are visible to the
	// non-blocking drain on the *next* iteration of loopResp.
	resps := make(chan *proto.CoordinateResponse, 64)
	stream := newFakeCoordinateStream(ctx)
	c := communicator{
		logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		stream: stream,
		reqs:   make(chan *proto.CoordinateRequest, 1),
		resps:  resps,
	}

	// Pre-load many responses before starting loopResp so the drain merges
	// them into one Send call.
	for i := 0; i < 5; i++ {
		resps <- &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate("u")}}
	}

	done := make(chan struct{})
	go func() {
		c.loopResp()
		close(done)
	}()

	waitForSends(t, stream, 1)
	// Give loopResp a moment to perform additional sends if it were not
	// batching. The queue is already drained, so any further send would
	// have to come from a new resp.
	time.Sleep(20 * time.Millisecond)

	got := stream.snapshot()
	require.Len(t, got, 1, "expected the burst to be merged into one Send")
	assert.Len(t, got[0].PeerUpdates, 5)

	close(resps)
	select {
	case <-done:
	case <-time.After(testutil.WaitShort):
		t.Fatal("loopResp did not exit")
	}
}

func TestLoopResp_ErrorNotBatched(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	resps, stream, wait := runLoopResp(ctx, t)

	// An error response should be flushed immediately on its own, without
	// being merged with anything else.
	resps <- &proto.CoordinateResponse{Error: "boom"}

	waitForSends(t, stream, 1)
	got := stream.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, "boom", got[0].Error)
	assert.Empty(t, got[0].PeerUpdates)

	close(resps)
	wait()
}

func TestLoopResp_PeerUpdatesThenError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	resps := make(chan *proto.CoordinateResponse, 64)
	stream := newFakeCoordinateStream(ctx)
	c := communicator{
		logger: slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		stream: stream,
		reqs:   make(chan *proto.CoordinateRequest, 1),
		resps:  resps,
	}

	// Pre-load: a couple of PeerUpdate responses followed by an error.
	// loopResp's drain should flush the accumulated batch, then send the
	// error as its own message (in order).
	resps <- &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate("a")}}
	resps <- &proto.CoordinateResponse{PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{peerUpdate("b")}}
	resps <- &proto.CoordinateResponse{Error: "boom"}

	done := make(chan struct{})
	go func() {
		c.loopResp()
		close(done)
	}()

	waitForSends(t, stream, 2)
	// Give it a beat to ensure no extra sends slip in.
	time.Sleep(20 * time.Millisecond)

	got := stream.snapshot()
	require.Len(t, got, 2, "expected one merged peer-update Send and one error Send")
	assert.Empty(t, got[0].Error)
	assert.Len(t, got[0].PeerUpdates, 2)
	assert.Equal(t, "boom", got[1].Error)
	assert.Empty(t, got[1].PeerUpdates)

	close(resps)
	select {
	case <-done:
	case <-time.After(testutil.WaitShort):
		t.Fatal("loopResp did not exit")
	}
}
