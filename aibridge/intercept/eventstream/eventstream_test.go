package eventstream_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/intercept/eventstream"
	"github.com/coder/quartz"
)

// clockAdvancingFlusher wraps httptest.ResponseRecorder and advances the mock
// clock on each Flush call, simulating a slow client without real sleeping.
type clockAdvancingFlusher struct {
	*httptest.ResponseRecorder
	clk     *quartz.Mock
	advance time.Duration
}

func (f *clockAdvancingFlusher) Flush() {
	f.clk.Advance(f.advance)
	f.ResponseRecorder.Flush()
}

// Hijack satisfies the FullResponseWriter lint rule.
func (*clockAdvancingFlusher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func TestEventStream_LogsWarning_WhenFlushIsSlow(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	logger := slogtest.Make(t, nil).AppendSinks(sloghuman.Sink(&buf)).Leveled(slog.LevelWarn)
	ctx := context.Background()
	clk := quartz.NewMock(t)

	stream := eventstream.NewEventStream(ctx, logger, nil, clk)

	w := &clockAdvancingFlusher{
		ResponseRecorder: httptest.NewRecorder(),
		clk:              clk,
		advance:          eventstream.SlowFlushThreshold + time.Millisecond,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	require.NoError(t, err)
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("User-Agent", "test-agent/1.0")

	done := make(chan struct{})
	go func() {
		defer close(done)
		stream.Start(w, req)
	}()

	stream.InitiateStream(w)
	require.NoError(t, stream.SendRaw(ctx, []byte("data: hello\n\n")))
	require.NoError(t, stream.Shutdown(ctx))
	<-done

	require.Contains(t, buf.String(), "slow client detected")
	require.Contains(t, buf.String(), "192.0.2.1")
	require.Contains(t, buf.String(), "test-agent/1.0")
	require.Contains(t, buf.String(), "payload_size=13")
}

func TestEventStream_NoWarning_WhenFlushIsFast(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	logger := slogtest.Make(t, nil).AppendSinks(sloghuman.Sink(&buf)).Leveled(slog.LevelWarn)
	ctx := context.Background()
	clk := quartz.NewMock(t)

	stream := eventstream.NewEventStream(ctx, logger, nil, clk)

	// No clock advance, flush duration stays at 0, below threshold.
	w := &clockAdvancingFlusher{
		ResponseRecorder: httptest.NewRecorder(),
		clk:              clk,
		advance:          0,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		stream.Start(w, req)
	}()

	stream.InitiateStream(w)
	require.NoError(t, stream.SendRaw(ctx, []byte("data: hello\n\n")))
	require.NoError(t, stream.Shutdown(ctx))
	<-done

	require.Empty(t, buf.String())
}
