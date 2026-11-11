package replaybody_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

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

// newBuffer wraps source through NewResponse and returns the forwarding body.
func newBuffer(source io.ReadCloser) (io.ReadCloser, *replaybody.TeeBuffer) {
	response := &http.Response{Body: source}
	b := replaybody.NewResponse(response)
	return response.Body, b
}

// newRequestBuffer wraps source reader through NewRequest.
func newRequestBuffer(source io.ReadCloser) (*http.Request, *replaybody.TeeBuffer) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Body = source
	request.ContentLength = -1 // source has unknown length
	return request, replaybody.NewRequest(request)
}

type result struct {
	data []byte
	err  error
}

func TestBufferLazy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		input   string
		consume bool
		readErr error
		want    string
		wantErr error
	}{
		{name: "UnreadEmpty", wantErr: io.ErrClosedPipe},
		{name: "EmptyEOF", consume: true, readErr: io.EOF},
		{name: "Partial", input: "payload", consume: true, want: "p", wantErr: io.ErrClosedPipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				var reads atomic.Int32
				input := strings.NewReader(tc.input)
				s := &source{closed: make(chan struct{})}
				s.read = func(p []byte) (int, error) {
					reads.Add(1)
					return input.Read(p)
				}
				body, b := newBuffer(s)
				defer body.Close()
				defer b.Close()
				waited := make(chan result, 1)
				go func() { p, err := b.Wait(context.Background()); waited <- result{p, err} }()
				synctest.Wait()
				require.Zero(t, reads.Load(), "construction and Wait must not read the source")

				_, err := body.Read(nil)
				require.NoError(t, err, "zero-length forwarding reads must succeed")
				require.Zero(t, reads.Load(), "zero-length forwarding reads must not consume input")
				if tc.consume {
					_, err = body.Read(make([]byte, 1))
					require.ErrorIs(t, err, tc.readErr)
					require.EqualValues(t, 1, reads.Load(), "capture must not read ahead")
				}
				require.NoError(t, body.Close())
				require.NoError(t, b.Close())
				synctest.Wait()
				got := <-waited
				require.Equal(t, tc.want, string(got.data), "retain exactly the bytes consumed by forwarding")
				require.ErrorIs(t, got.err, tc.wantErr)
			})
		})
	}
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
			ctx := testutil.Context(t, testutil.WaitLong)
			var reads atomic.Int32
			s := &source{closed: make(chan struct{})}
			s.read = func(p []byte) (int, error) {
				if reads.Add(1) == 1 {
					return copy(p, "prefix"), nil
				}
				return copy(p, "suffix"), tc.terminal
			}
			request, b := newRequestBuffer(s)
			body := request.Body
			defer body.Close()
			t.Cleanup(func() { require.NoError(t, b.Close()) })

			prefix := make([]byte, len("prefix"))
			_, err := io.ReadFull(body, prefix)
			require.NoError(t, err)
			require.Equal(t, "prefix", string(prefix))

			_, err = io.Copy(io.Discard, body)
			require.ErrorIs(t, err, tc.want, "reading source must consume EOF and preserve other source errors")
			payload, err := b.Wait(ctx)
			require.ErrorIs(t, err, tc.want, "wait must normalize bare EOF and preserve read errors")
			require.NotErrorIs(t, err, io.ErrClosedPipe, "source errors must not become capture interruptions")
			require.Equal(t, "prefixsuffix", string(payload), "capture must retain bytes returned with a terminal error")
			require.Equal(t, len(payload), cap(payload), "shared payload capacity must not expose spare bytes")

			shared, _ := b.Wait(ctx)
			require.Same(t, &payload[0], &shared[0], "wait must return the same shared bytes")
			retry, err := request.GetBody()
			require.NoError(t, err)
			defer retry.Close()
			replay, replayErr := io.ReadAll(retry)
			require.Equal(t, payload, replay, "retries must replay all retained bytes")
			require.ErrorIs(t, replayErr, tc.want, "retries must replay the source's terminal state")
			require.EqualValues(t, 2, reads.Load(), "retries must not read the source after its terminal state")
			require.NoError(t, b.Close(), "closing finished capture must succeed")
			_, err = b.Wait(ctx)
			require.ErrorIs(t, err, tc.want, "close must preserve the source's terminal state")
		})
	}
}

// TestBodyCloseUnblocksRead closes a forwarding body while its reads wait on
// a blocked source. Every waiting read must fail, the source must stay open,
// and a retry must continue from the read that was in flight when the body
// closed.
func TestBodyCloseUnblocksRead(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	reading := make(chan struct{})
	release := make(chan struct{})
	input := strings.NewReader("request")
	var started sync.Once
	s := &source{closed: make(chan struct{})}
	s.read = func(p []byte) (int, error) {
		started.Do(func() { close(reading) })
		select {
		case <-release:
			return input.Read(p)
		case <-s.closed:
			return 0, io.ErrClosedPipe
		}
	}
	request, b := newRequestBuffer(s)
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := request.Body.Read(make([]byte, 1)); results <- err }()
	}
	testutil.TryReceive(ctx, t, reading)
	closed := make(chan error, 2)
	for range 2 {
		go func() { closed <- request.Body.Close() }()
	}
	for range 2 {
		require.NoError(t, testutil.RequireReceive(ctx, t, closed), "concurrent closes must all succeed")
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, results), io.ErrClosedPipe, "Close must unblock every read on this body")
	}
	require.Zero(t, s.closes.Load(), "closing a body must preserve the source")
	retry, err := request.GetBody()
	require.NoError(t, err)
	defer retry.Close()
	bodyResult := make(chan result, 1)
	go func() { p, err := io.ReadAll(retry); bodyResult <- result{p, err} }()
	close(release)
	got := testutil.RequireReceive(ctx, t, bodyResult)
	require.NoError(t, got.err, "a retry must continue the in-flight source read")
	require.Equal(t, "request", string(got.data), "a retry must receive every byte, including the in-flight read")
}

// TestBodyClose closes the first forwarding body after a partial read and
// checks that a retry from GetBody replays the prefix and continues the
// source from where the first body stopped.
func TestBodyClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	input := strings.NewReader("request")
	s := &source{closed: make(chan struct{})}
	s.read = input.Read
	request, b := newRequestBuffer(s)
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	prefix := make([]byte, len("req"))
	_, err := io.ReadFull(request.Body, prefix)
	require.NoError(t, err, "the first body must stream the prefix")
	require.NoError(t, request.Body.Close())
	_, err = request.Body.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.ErrClosedPipe, "closed bodies must reject reads")
	retry, err := request.GetBody()
	require.NoError(t, err)
	defer retry.Close()
	payload, err := io.ReadAll(retry)
	require.NoError(t, err, "a retry must replay the prefix and continue the source")
	require.Equal(t, "request", string(payload), "a retry must receive the entire body")
	retained, err := b.Wait(ctx)
	require.NoError(t, err, "the retry must complete capture at EOF")
	require.Equal(t, payload, retained, "retries must not duplicate retained bytes")
	require.Zero(t, s.closes.Load(), "the server owns the request body")
}

// TestBufferClose closes the buffer while a forwarding read is blocked in the
// source. Close must return without waiting for that read, retain only the
// bytes published before it, unblock the waiting body, and close the source
// exactly once.
func TestBufferClose(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	reading := make(chan struct{})
	var reads atomic.Int32
	s := &source{closed: make(chan struct{}), closeErr: io.ErrUnexpectedEOF}
	s.read = func(p []byte) (int, error) {
		if reads.Add(1) == 1 {
			return copy(p, "prefix"), nil
		}
		close(reading)
		<-s.closed
		return copy(p, "late"), io.ErrClosedPipe
	}
	body, b := newBuffer(s)
	defer body.Close()
	bodyResult := make(chan result, 1)
	go func() { p, err := io.ReadAll(body); bodyResult <- result{p, err} }()
	testutil.TryReceive(ctx, t, reading)
	closed := make(chan error, 2)
	for range 2 {
		go func() { closed <- b.Close() }()
	}
	for range 2 {
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, closed), s.closeErr, "every Close must return the source close error")
	}
	require.EqualValues(t, 1, s.closes.Load(), "concurrent closes must close the source exactly once")
	payload, err := b.Wait(ctx)
	require.ErrorIs(t, err, io.ErrClosedPipe, "closed capture must report interruption")
	require.Equal(t, "prefix", string(payload), "close must retain the bytes published before it")
	got := testutil.RequireReceive(ctx, t, bodyResult)
	require.ErrorIs(t, got.err, io.ErrClosedPipe, "the body must report interruption without waiting for the source")
	require.Equal(t, payload, got.data, "bytes returned after Close must not reach the body")
	require.ErrorIs(t, b.Close(), s.closeErr, "later closes must return the cached close error")
	require.EqualValues(t, 1, s.closes.Load(), "later closes must not close the source again")
}

// TestBufferConcurrentBodies drives the forwarding body and a retry body
// against a pipe at once. Each must receive every chunk exactly once,
// whichever body read it from the source.
func TestBufferConcurrentBodies(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	input, output := io.Pipe()
	request, b := newRequestBuffer(input)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	retry, err := request.GetBody()
	require.NoError(t, err)
	results := make(chan result, 2)
	for _, r := range []io.ReadCloser{request.Body, retry} {
		go func() { p, err := io.ReadAll(r); results <- result{p, err} }()
	}
	for range 2 {
		_, err := io.WriteString(output, "chunk")
		require.NoError(t, err)
	}
	require.NoError(t, output.Close())
	for range 2 {
		got := testutil.RequireReceive(ctx, t, results)
		require.NoError(t, got.err, "every body must finish successfully at source EOF")
		require.Equal(t, strings.Repeat("chunk", 2), string(got.data), "each body must receive every chunk")
	}
}

func TestBufferWaitCanceled(t *testing.T) {
	t.Parallel()
	input, output := io.Pipe()
	body, b := newBuffer(input)
	defer body.Close()
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	payload, err := b.Wait(ctx)
	require.Nil(t, payload, "a canceled Wait must return a nil payload")
	require.ErrorIs(t, err, context.Canceled, "wait must report its own context cancellation")
	go func() { _, _ = io.WriteString(output, "request"); _ = output.Close() }()
	_, err = io.Copy(io.Discard, body)
	require.NoError(t, err, "canceling Wait must not interrupt forwarding")
	payload, err = b.Wait(testutil.Context(t, testutil.WaitLong))
	require.NoError(t, err, "canceling Wait must not cancel capture")
	require.Equal(t, "request", string(payload), "capture must retain the full body after Wait is canceled")
}
