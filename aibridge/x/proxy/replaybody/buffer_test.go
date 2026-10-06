package replaybody_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	onClose  func()
}

func (s *source) Read(p []byte) (int, error) { return s.read(p) }
func (s *source) Close() error {
	s.closes.Add(1)
	s.once.Do(func() {
		close(s.closed)
		if s.onClose != nil {
			s.onClose()
		}
	})
	return s.closeErr
}

func newReader(t *testing.T, b *replaybody.Buffer) io.ReadCloser {
	t.Helper()
	r, err := b.NewReader()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close()) })
	return r
}

func TestBufferSharedCapture(t *testing.T) {
	t.Parallel()
	for _, terminal := range []error{io.EOF, io.ErrUnexpectedEOF} {
		t.Run(terminal.Error(), func(t *testing.T) {
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
				return copy(p, "suffix"), terminal
			}
			b := replaybody.New(s)
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			ctx := testutil.Context(t, testutil.WaitLong)
			testutil.TryReceive(ctx, t, reading)
			require.EqualValues(t, 2, reads.Load(), "capture must start without a reader")
			first := newReader(t, b)
			require.NoError(t, first.Close())
			require.Zero(t, s.closes.Load(), "reader close must not close the source")
			_, err := first.Read(make([]byte, 1))
			require.ErrorIs(t, err, io.ErrClosedPipe)

			second := newReader(t, b)
			prefix := make([]byte, len("prefix"))
			_, err = io.ReadFull(second, prefix)
			require.NoError(t, err)
			require.Equal(t, "prefix", string(prefix))
			require.NoError(t, second.Close())
			third := newReader(t, b)
			_, err = io.ReadFull(third, prefix)
			require.NoError(t, err)
			require.NoError(t, s.Close())
			payload, err := b.Wait(ctx)
			if errors.Is(terminal, io.EOF) {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, terminal)
			}
			require.Equal(t, "prefixsuffix", string(payload))
			require.Equal(t, len(payload), cap(payload))
			shared, _ := b.Wait(ctx)
			require.Same(t, &payload[0], &shared[0], "processing stages must share the payload")
			for range 2 {
				replay, replayErr := io.ReadAll(newReader(t, b))
				require.Equal(t, payload, replay)
				if errors.Is(terminal, io.EOF) {
					require.NoError(t, replayErr)
				} else {
					require.ErrorIs(t, replayErr, terminal)
				}
			}
			require.NoError(t, b.Stop(context.Canceled))
			_, err = b.Wait(ctx)
			if errors.Is(terminal, io.EOF) {
				require.NoError(t, err, "stop must not replace completed EOF")
			} else {
				require.ErrorIs(t, err, terminal, "stop must not replace a source error")
			}
		})
	}
}

func TestReaderCloseUnblocksRead(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	input, output := io.Pipe()
	reading := make(chan struct{})
	s := &source{closed: make(chan struct{}), onClose: func() { _ = input.Close() }}
	s.read = func(p []byte) (int, error) {
		select {
		case <-reading:
		default:
			close(reading)
		}
		return input.Read(p)
	}
	b := replaybody.New(s)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	first, second := newReader(t, b), newReader(t, b)
	result := make(chan error, 8)
	for range 8 {
		go func() { _, err := first.Read(make([]byte, 1)); result <- err }()
	}
	testutil.TryReceive(ctx, t, reading)
	closed := make(chan error, 8)
	for range 8 {
		go func() { closed <- first.Close() }()
	}
	for range 8 {
		require.NoError(t, testutil.RequireReceive(ctx, t, closed))
		require.ErrorIs(t, testutil.RequireReceive(ctx, t, result), io.ErrClosedPipe)
	}
	go func() { _, _ = io.WriteString(output, "request"); _ = output.Close() }()
	payload, err := io.ReadAll(second)
	require.NoError(t, err)
	require.Equal(t, "request", string(payload))
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
	b := replaybody.New(s)
	result := make(chan error, 1)
	r := newReader(t, b)
	go func() { _, err := io.ReadAll(r); result <- err }()
	testutil.TryReceive(ctx, t, reading)
	done := make(chan struct{}, 8)
	for range 8 {
		go func() { _ = b.Stop(context.Canceled); done <- struct{}{} }()
	}
	for range 8 {
		testutil.TryReceive(ctx, t, done)
	}
	require.EqualValues(t, 1, s.closes.Load())
	payload, err := b.Wait(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "captured", string(payload), "retain bytes returned during shutdown")
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, result), context.Canceled)
	_, replayErr := io.ReadAll(newReader(t, b))
	require.ErrorIs(t, replayErr, context.Canceled)
}

func TestBufferImmediateClose(t *testing.T) {
	t.Parallel()
	for range 50 {
		s := &source{closed: make(chan struct{})}
		s.read = func([]byte) (int, error) { <-s.closed; return 0, io.ErrClosedPipe }
		b := replaybody.New(s)
		require.NoError(t, b.Close(), "close must join capture even before it is scheduled")
		require.EqualValues(t, 1, s.closes.Load())
		_, err := b.Wait(testutil.Context(t, testutil.WaitLong))
		require.ErrorIs(t, err, io.ErrClosedPipe)
	}
}

func TestBufferForwardsBeforeEOF(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	received := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := make([]byte, len("prefix"))
		if _, err := io.ReadFull(r.Body, prefix); err != nil {
			t.Error(err)
			return
		}
		received <- struct{}{}
		_, _ = io.Copy(w, io.MultiReader(strings.NewReader(string(prefix)), r.Body))
	}))
	t.Cleanup(upstream.Close)
	input, output := io.Pipe()
	b := replaybody.New(input)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream.URL, newReader(t, b))
	require.NoError(t, err)
	req.GetBody = b.NewReader
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := upstream.Client().Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || string(body) != "prefixsuffix" {
			t.Errorf("forwarded body = %q, error = %v", body, err)
		}
	}()
	_, err = io.WriteString(output, "prefix")
	require.NoError(t, err)
	testutil.TryReceive(ctx, t, received)
	_, err = io.WriteString(output, "suffix")
	require.NoError(t, err)
	require.NoError(t, output.Close())
	testutil.TryReceive(ctx, t, done)
	payload, err := b.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, "prefixsuffix", string(payload))
}

func TestBufferTerminalErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cause error
		want  error
	}{
		{name: "NilStop", want: io.ErrClosedPipe},
		{name: "EOFStop", cause: io.EOF, want: io.ErrClosedPipe},
		{name: "WrappedEOFStop", cause: xerrors.Errorf("stop: %w", io.EOF), want: io.ErrClosedPipe},
		{name: "FirstStopWins", cause: context.Canceled, want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &source{closed: make(chan struct{}), closeErr: io.ErrUnexpectedEOF}
			s.read = func([]byte) (int, error) { <-s.closed; return 0, io.ErrClosedPipe }
			b := replaybody.New(s)
			require.ErrorIs(t, b.Stop(tc.cause), s.closeErr)
			require.ErrorIs(t, b.Stop(context.DeadlineExceeded), s.closeErr)
			_, err := b.Wait(testutil.Context(t, testutil.WaitLong))
			require.ErrorIs(t, err, tc.want)
			require.EqualValues(t, 1, s.closes.Load())
		})
	}

	t.Run("WrappedSourceEOF", func(t *testing.T) {
		t.Parallel()
		terminal := xerrors.Errorf("read: %w", io.EOF)
		s := &source{closed: make(chan struct{})}
		s.read = func(p []byte) (int, error) { return copy(p, "partial"), terminal }
		b := replaybody.New(s)
		t.Cleanup(func() { require.NoError(t, b.Close()) })
		payload, err := io.ReadAll(newReader(t, b))
		require.ErrorIs(t, err, terminal)
		require.Equal(t, "partial", string(payload))
		_, err = b.Wait(testutil.Context(t, testutil.WaitLong))
		require.ErrorIs(t, err, terminal, "only a bare EOF marks capture complete")
	})
}

type writerToSource struct{ io.ReadCloser }

func (writerToSource) WriteTo(io.Writer) (int64, error) {
	return 0, xerrors.New("capture must read the source instead of using WriteTo")
}

func TestBufferUsesStreamingReads(t *testing.T) {
	t.Parallel()
	b := replaybody.New(writerToSource{io.NopCloser(strings.NewReader("request"))})
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	payload, err := io.ReadAll(newReader(t, b))
	require.NoError(t, err)
	require.Equal(t, "request", string(payload))
}

func TestBufferConcurrentReaders(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	input, output := io.Pipe()
	b := replaybody.New(input)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	type result struct {
		payload []byte
		err     error
	}
	results := make(chan result, 8)
	for range 8 {
		r := newReader(t, b)
		go func() { p, err := io.ReadAll(r); results <- result{p, err} }()
	}
	for range 8 {
		_, err := io.WriteString(output, "chunk")
		require.NoError(t, err)
	}
	require.NoError(t, output.Close())
	for range 8 {
		got := testutil.RequireReceive(ctx, t, results)
		require.NoError(t, got.err)
		require.Equal(t, strings.Repeat("chunk", 8), string(got.payload))
	}
}

func TestBufferWaitCanceled(t *testing.T) {
	t.Parallel()
	input, output := io.Pipe()
	b := replaybody.New(input)
	t.Cleanup(func() { require.NoError(t, b.Close()); _ = output.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	payload, err := b.Wait(ctx)
	require.Nil(t, payload)
	require.ErrorIs(t, err, context.Canceled)
	go func() { _, _ = io.WriteString(output, "request"); _ = output.Close() }()
	payload, err = b.Wait(testutil.Context(t, testutil.WaitLong))
	require.NoError(t, err, "canceling Wait must not cancel capture")
	require.Equal(t, "request", string(payload))
}

func BenchmarkSealedPayload(b *testing.B) {
	buffer := replaybody.New(io.NopCloser(strings.NewReader(strings.Repeat("x", 1<<20))))
	defer buffer.Close()
	reader, _ := buffer.NewReader()
	_, _ = io.Copy(io.Discard, reader)
	_ = reader.Close()
	_, _ = buffer.Wait(context.Background())
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for range 8 {
			payload, err := buffer.Wait(context.Background())
			if err != nil || len(payload) != 1<<20 {
				b.Fatal("invalid sealed payload")
			}
		}
	}
}

func TestBufferEmpty(t *testing.T) {
	t.Parallel()
	b := replaybody.New(io.NopCloser(strings.NewReader("")))
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	payload, err := io.ReadAll(newReader(t, b))
	require.NoError(t, err)
	require.Empty(t, payload)
	payload, err = b.Wait(testutil.Context(t, testutil.WaitLong))
	require.NoError(t, err)
	require.Empty(t, payload)
}
