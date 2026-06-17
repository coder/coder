package workspacetraffic

import (
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

// stubConn simulates the server side of a reconnecting PTY connection.
type stubConn struct {
	// closeOnWrite closes the connection on the first write, simulating a
	// server that closes gracefully in response to Ctrl+C.
	closeOnWrite bool
	// failWrites makes every write return an error, simulating a connection
	// that can no longer send data.
	failWrites bool
	// readIgnoresClose prevents reads from unblocking when the connection is
	// closed, simulating a misbehaving connection.
	readIgnoresClose bool

	closeOnce sync.Once
	closedCh  chan struct{}
	// releaseCh unblocks reads when readIgnoresClose is set, allowing the
	// test to clean up the read goroutine.
	releaseCh chan struct{}
}

func newStubConn() *stubConn {
	return &stubConn{
		closedCh:  make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (s *stubConn) Read(_ []byte) (int, error) {
	if s.readIgnoresClose {
		<-s.releaseCh
		return 0, io.EOF
	}
	<-s.closedCh
	return 0, io.EOF
}

func (s *stubConn) Write(p []byte) (int, error) {
	if s.failWrites {
		return 0, xerrors.New("write failed")
	}
	if s.closeOnWrite {
		_ = s.Close()
	}
	return len(p), nil
}

func (s *stubConn) Close() error {
	s.closeOnce.Do(func() {
		close(s.closedCh)
	})
	return nil
}

// startDrain reads from rc until it errors, mirroring the drain goroutine in
// Runner.Run. It returns a channel that is closed when the read finishes.
func startDrain(t *testing.T, rc *rptyConn) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(io.Discard, rc)
	}()
	return done
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	_ = testutil.TryReceive(ctx, t, done)
}

func TestRPTYConn_Close(t *testing.T) {
	t.Parallel()

	t.Run("Graceful", func(t *testing.T) {
		t.Parallel()

		// The server closes the connection in response to Ctrl+C, the read
		// unblocks with io.EOF and Close reports success.
		stub := newStubConn()
		stub.closeOnWrite = true
		rc := newPTYConn(stub)
		done := startDrain(t, rc)

		err := rc.Close()
		require.NoError(t, err)
		waitDone(t, done)
	})

	t.Run("ForceClose", func(t *testing.T) {
		t.Parallel()

		// The server ignores Ctrl+C and never closes the connection. Close
		// force closes the connection to unblock the read and reports a
		// non-fatal graceful close timeout.
		stub := newStubConn()
		rc := newPTYConn(stub)
		rc.closeTimeout = testutil.IntervalFast
		done := startDrain(t, rc)

		err := rc.Close()
		require.ErrorIs(t, err, errRPTYGracefulCloseTimeout)
		waitDone(t, done)
	})

	t.Run("WriteFails", func(t *testing.T) {
		t.Parallel()

		// The Ctrl+C write fails. Close force closes the connection to
		// unblock the read and reports a hard error, not the non-fatal
		// graceful close timeout.
		stub := newStubConn()
		stub.failWrites = true
		rc := newPTYConn(stub)
		done := startDrain(t, rc)

		err := rc.Close()
		require.Error(t, err)
		require.NotErrorIs(t, err, errRPTYGracefulCloseTimeout)
		require.ErrorContains(t, err, "write ctrl+c")
		waitDone(t, done)
	})

	t.Run("ReadStuckAfterClose", func(t *testing.T) {
		t.Parallel()

		// The read doesn't unblock even after the connection is force
		// closed. Close reports an error instead of blocking forever.
		stub := newStubConn()
		stub.readIgnoresClose = true
		rc := newPTYConn(stub)
		rc.closeTimeout = testutil.IntervalFast
		rc.forceCloseReadTimeout = testutil.IntervalFast
		done := startDrain(t, rc)
		// Unblock the read goroutine at the end of the test.
		t.Cleanup(func() {
			close(stub.releaseCh)
			waitDone(t, done)
		})

		err := rc.Close()
		require.Error(t, err)
		require.NotErrorIs(t, err, errRPTYGracefulCloseTimeout)
		require.ErrorContains(t, err, "timeout waiting for read to finish after close")
	})

	t.Run("CloseTwice", func(t *testing.T) {
		t.Parallel()

		// A second Close is a no-op and returns nil.
		stub := newStubConn()
		stub.closeOnWrite = true
		rc := newPTYConn(stub)
		done := startDrain(t, rc)

		require.NoError(t, rc.Close())
		require.NoError(t, rc.Close())
		waitDone(t, done)
	})
}
