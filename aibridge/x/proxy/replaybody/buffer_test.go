package replaybody_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
	"github.com/coder/coder/v2/testutil"
)

type source struct {
	read     func([]byte) (int, error)
	closed   chan struct{}
	closes   atomic.Int32
	closeErr error
	once     sync.Once
}

func (s *source) Read(p []byte) (int, error) { return s.read(p) }
func (s *source) Close() error {
	s.closes.Add(1)
	s.once.Do(func() { close(s.closed) })
	return s.closeErr
}

func newBuffer(source io.ReadCloser) *replaybody.Buffer {
	//nolint:bodyclose // Buffer.Close owns the source transferred to NewResponse.
	response, b := replaybody.NewResponse(context.Background(), &http.Response{Body: source})
	_ = response.Body.Close()
	return b
}

func newReader(t *testing.T, b *replaybody.Buffer) io.ReadCloser {
	t.Helper()
	r, err := b.NewReader()
	require.NoError(t, err, "creating an independent reader must succeed")
	t.Cleanup(func() { require.NoError(t, r.Close()) })
	return r
}

func TestBufferSharedCapture(t *testing.T) {
	t.Parallel()
	wrappedEOF := xerrors.Errorf("read: %w", io.EOF)
	for _, tc := range []struct {
		name     string
		terminal error
		want     error
	}{
		{name: "EOF", terminal: io.EOF},
		{name: "ReadFailure", terminal: io.ErrUnexpectedEOF, want: io.ErrUnexpectedEOF},
		{name: "WrappedEOF", terminal: wrappedEOF, want: wrappedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reading := make(chan struct{})
			var reads atomic.Int32
			s := &source{closed: make(chan struct{})}
			s.read = func(p []byte) (int, error) {
				if reads.Add(1) == 1 {
					return copy(p, "prefix"), nil
				}
				close(reading)
				<-s.closed
				return copy(p, "suffix"), tc.terminal
			}
			b := newBuffer(s)
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			ctx := testutil.Context(t, testutil.WaitLong)
			testutil.TryReceive(ctx, t, reading)
			require.EqualValues(t, 2, reads.Load(), "capture must start without a reader")

			first := newReader(t, b)
			require.NoError(t, first.Close(), "readers may close before reading")
			require.Zero(t, s.closes.Load(), "reader close must not close the source")
			_, err := first.Read(make([]byte, 1))
			require.ErrorIs(t, err, io.ErrClosedPipe, "closed readers must reject reads")

			second := newReader(t, b)
			prefix := make([]byte, len("prefix"))
			_, err = io.ReadFull(second, prefix)
			require.NoError(t, err, "readers must stream before source EOF")
			require.Equal(t, "prefix", string(prefix), "each reader must start at the captured prefix")
			require.NoError(t, second.Close(), "partially read cursors must close independently")

			third := newReader(t, b)
			_, err = io.ReadFull(third, prefix)
			require.NoError(t, err, "new readers must replay while capture is active")

			require.NoError(t, s.Close())
			payload, err := b.Wait(ctx)
			require.ErrorIs(t, err, tc.want, "wait must normalize bare EOF and preserve read errors")
			require.NotErrorIs(t, err, io.ErrClosedPipe, "source errors must not become capture interruptions")
			require.Equal(t, "prefixsuffix", string(payload), "capture must retain bytes returned with a terminal error")
			require.Equal(t, len(payload), cap(payload), "shared payload capacity must not expose spare bytes")

			shared, _ := b.Wait(ctx)
			require.Same(t, &payload[0], &shared[0], "wait must return the same shared bytes")
			replay, replayErr := io.ReadAll(newReader(t, b))
			require.Equal(t, payload, replay, "new readers must replay all retained bytes")
			require.ErrorIs(t, replayErr, tc.want, "reading replay must succeed at EOF and preserve source errors")
			require.NoError(t, b.Stop(context.Canceled), "stopping finished capture must succeed")
			_, err = b.Wait(ctx)
			require.ErrorIs(t, err, tc.want, "stop must preserve the source's terminal state")
		})
	}
}

func TestReaderCloseUnblocksRead(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	input, output := io.Pipe()
	b := newBuffer(input)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	first := newReader(t, b)
	second := newReader(t, b)
	result := make(chan error, 2)
	for range 2 {
		go func() { _, err := first.Read(make([]byte, 1)); result <- err }()
	}
	closed := make(chan error, 2)
	for range 2 {
		go func() { closed <- first.Close() }()
	}
	for range 2 {
		require.NoError(t, testutil.RequireReceive(ctx, t, closed), "concurrent closes must all succeed")
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, result), io.ErrClosedPipe, "every read on the closed cursor must report closure")
	}
	go func() { _, _ = io.WriteString(output, "request"); _ = output.Close() }()
	payload, err := io.ReadAll(second)
	require.NoError(t, err, "closing one cursor must not stop capture")
	require.Equal(t, "request", string(payload), "closing one cursor must leave other cursors usable")
}

func TestBufferStop(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	reading := make(chan struct{})
	s := &source{closed: make(chan struct{})}
	s.read = func(p []byte) (int, error) {
		close(reading)
		<-s.closed
		return copy(p, "captured"), io.ErrClosedPipe
	}
	b := newBuffer(s)
	readResult := make(chan error, 1)
	r := newReader(t, b)
	go func() { _, err := io.ReadAll(r); readResult <- err }()
	testutil.TryReceive(ctx, t, reading)
	done := make(chan struct{}, 2)
	for range 2 {
		go func() { _ = b.Stop(context.Canceled); done <- struct{}{} }()
	}
	// Every concurrent Stop call must return.
	for range 2 {
		testutil.TryReceive(ctx, t, done)
	}
	require.EqualValues(t, 1, s.closes.Load(), "concurrent stops must close the source exactly once")
	payload, err := b.Wait(ctx)
	require.ErrorIs(t, err, io.ErrClosedPipe, "stopped capture must report interruption")
	require.ErrorIs(t, err, context.Canceled, "capture must preserve the requested stop cause")
	require.Equal(t, "captured", string(payload), "retain bytes returned during shutdown")
	readErr := testutil.RequireReceive(ctx, t, readResult)
	require.ErrorIs(t, readErr, io.ErrClosedPipe, "existing readers must report interruption")
	require.ErrorIs(t, readErr, context.Canceled, "existing readers must preserve the stop cause")
	_, replayErr := io.ReadAll(newReader(t, b))
	require.ErrorIs(t, replayErr, io.ErrClosedPipe, "new readers must replay interruption")
	require.ErrorIs(t, replayErr, context.Canceled, "new readers must preserve the stop cause")
}

func TestBufferTerminalErrors(t *testing.T) {
	t.Parallel()
	laterCause := xerrors.New("later stop")
	for _, tc := range []struct {
		name  string
		cause error
		close bool
	}{
		{name: "Close", close: true},
		{name: "NilStop"},
		{name: "EOFStop", cause: io.EOF},
		{name: "WrappedEOFStop", cause: xerrors.Errorf("stop: %w", io.EOF)},
		{name: "WrappedCause", cause: xerrors.Errorf("stop: %w", context.DeadlineExceeded)},
		{name: "FirstStopWins", cause: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &source{closed: make(chan struct{}), closeErr: io.ErrUnexpectedEOF}
			s.read = func([]byte) (int, error) { <-s.closed; return 0, io.ErrClosedPipe }
			b := newBuffer(s)
			var err error
			if tc.close {
				err = b.Close()
			} else {
				err = b.Stop(tc.cause)
			}
			require.ErrorIs(t, err, s.closeErr, "Close and Stop must return the source close error")
			require.ErrorIs(t, b.Stop(laterCause), s.closeErr, "later stops must return the cached close error")
			_, err = b.Wait(testutil.Context(t, testutil.WaitLong))
			require.ErrorIs(t, err, io.ErrClosedPipe, "wait must report interruption for every stop cause")
			require.NotErrorIs(t, err, laterCause, "later stops must not replace the first cause")
			_, readErr := io.ReadAll(newReader(t, b))
			require.ErrorIs(t, readErr, io.ErrClosedPipe, "reader interruption must not look like successful EOF")
			if tc.cause != nil {
				require.ErrorIs(t, err, tc.cause, "wait must preserve the original stop cause")
				require.ErrorIs(t, readErr, tc.cause, "readers must preserve the original stop cause")
			}
			require.EqualValues(t, 1, s.closes.Load(), "later stops must not close the source again")
		})
	}
}

func TestBufferConcurrentReaders(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	input, output := io.Pipe()
	b := newBuffer(input)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	type result struct {
		payload []byte
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		r := newReader(t, b)
		go func() { p, err := io.ReadAll(r); results <- result{p, err} }()
	}
	for range 2 {
		_, err := io.WriteString(output, "chunk")
		require.NoError(t, err)
	}
	require.NoError(t, output.Close())
	for range 2 {
		got := testutil.RequireReceive(ctx, t, results)
		require.NoError(t, got.err, "every cursor must finish successfully at source EOF")
		require.Equal(t, strings.Repeat("chunk", 2), string(got.payload), "each cursor must receive every chunk")
	}
}

func TestBufferWaitCanceled(t *testing.T) {
	t.Parallel()
	input, output := io.Pipe()
	b := newBuffer(input)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	payload, err := b.Wait(ctx)
	require.Nil(t, payload, "a canceled Wait must return a nil payload")
	require.ErrorIs(t, err, context.Canceled, "wait must report its own context cancellation")
	go func() { _, _ = io.WriteString(output, "request"); _ = output.Close() }()
	payload, err = b.Wait(testutil.Context(t, testutil.WaitLong))
	require.NoError(t, err, "canceling Wait must not cancel capture")
	require.Equal(t, "request", string(payload), "capture must retain the full body after Wait is canceled")
}

// afterFuncContext tracks context.AfterFunc registrations. Its distinct Done
// channel forces the AfterFunc hook without hiding context values or causes.
type afterFuncContext struct {
	context.Context
	done       chan struct{}
	registered atomic.Int32
	active     atomic.Int32
	// holdCallback, when set, delays every cancellation callback until closed.
	holdCallback <-chan struct{}
}

func newAfterFuncContext(ctx context.Context) *afterFuncContext {
	c := &afterFuncContext{Context: ctx, done: make(chan struct{})}
	context.AfterFunc(ctx, func() { close(c.done) })
	return c
}

func (c *afterFuncContext) Done() <-chan struct{} { return c.done }
func (c *afterFuncContext) Err() error {
	select {
	case <-c.done:
		return c.Context.Err()
	default:
		return nil
	}
}

func (c *afterFuncContext) AfterFunc(f func()) func() bool {
	c.registered.Add(1)
	c.active.Add(1)
	stop := context.AfterFunc(c.Context, func() {
		<-c.done
		if c.holdCallback != nil {
			<-c.holdCallback
		}
		c.active.Add(-1)
		f()
	})
	return func() bool {
		if !stop() {
			return false
		}
		c.active.Add(-1)
		return true
	}
}

func TestBufferReleasesCancellationSubscription(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tracked := newAfterFuncContext(ctx)
	response, buffer := replaybody.NewResponse(tracked, &http.Response{Body: io.NopCloser(strings.NewReader("payload"))})
	defer response.Body.Close()
	defer buffer.Close()
	payload, err := buffer.Wait(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err)
	require.Equal(t, "payload", string(payload), "capture must retain all source bytes")
	require.EqualValues(t, 1, tracked.registered.Load(), "capture must register exactly one cancellation hook")
	require.Zero(t, tracked.active.Load(), "completed capture must release its context subscription")
}

// TestBufferEOFCompletion stops capture while the final read is in flight.
// In-flight read returns the last bytes with EOF after Stop has closed the source.
// Capture must complete successfully even though a stop cause is pending.
func TestBufferEOFCompletion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		stop func(t *testing.T, cancel context.CancelFunc, b *replaybody.Buffer)
	}{
		{
			name: "Stop",
			stop: func(t *testing.T, _ context.CancelFunc, b *replaybody.Buffer) {
				require.NoError(t, b.Stop(context.Canceled))
			},
		},
		{
			name: "ContextCancellation",
			stop: func(_ *testing.T, cancel context.CancelFunc, _ *replaybody.Buffer) { cancel() },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reading := make(chan struct{})
			input := &source{closed: make(chan struct{})}
			input.read = func(p []byte) (int, error) {
				close(reading)
				<-input.closed
				return copy(p, "payload"), io.EOF
			}
			//nolint:bodyclose // Buffer.Close owns the source transferred to NewResponse.
			response, b := replaybody.NewResponse(ctx, &http.Response{Body: input})
			defer response.Body.Close()
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			waitCtx := testutil.Context(t, testutil.WaitShort)
			testutil.TryReceive(waitCtx, t, reading)
			tc.stop(t, cancel, b)

			payload, err := b.Wait(waitCtx)
			require.NoError(t, err, "source EOF must complete capture even while stopping")
			require.Equal(t, "payload", string(payload), "capture must retain the bytes returned with EOF")

			replay, readErr := io.ReadAll(newReader(t, b))
			require.NoError(t, readErr, "replay must succeed at EOF even if capture was stopped")
			require.Equal(t, payload, replay, "replay must retain every captured byte")
		})
	}
}

// TestBufferContextStopsCapture cancels the context during a read while holding
// back the Stop that cancellation schedules, so only the read can report the cause.
func TestBufferContextStopsCapture(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{
		context.Canceled,
		xerrors.Errorf("cancel: %w", context.DeadlineExceeded),
		io.EOF,
		xerrors.Errorf("cancel: %w", io.EOF),
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			release := make(chan struct{})
			defer close(release)
			tracked := newAfterFuncContext(ctx)
			tracked.holdCallback = release
			input := &source{closed: make(chan struct{})}
			var reads atomic.Int32
			input.read = func(p []byte) (int, error) {
				if reads.Add(1) > 1 {
					return 0, io.ErrUnexpectedEOF
				}
				cancel(cause)
				<-tracked.Done()
				return copy(p, "partial"), nil
			}
			//nolint:bodyclose // Buffer.Close owns the source transferred to NewResponse.
			response, b := replaybody.NewResponse(tracked, &http.Response{Body: input})
			defer response.Body.Close()
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			payload, err := b.Wait(testutil.Context(t, testutil.WaitShort))
			require.ErrorIs(t, err, io.ErrClosedPipe, "the read must report interruption without waiting for Stop")
			require.ErrorIs(t, err, cause, "capture must preserve the original cancellation cause")
			require.Equal(t, "partial", string(payload), "cancellation must retain bytes returned by the same read")

			replay, readErr := io.ReadAll(newReader(t, b))
			require.ErrorIs(t, readErr, io.ErrClosedPipe, "replaying canceled capture must not swallow EOF causes")
			require.ErrorIs(t, readErr, cause, "replay must preserve the original cancellation cause")
			require.Equal(t, payload, replay, "replay must retain every captured byte")
		})
	}
}
