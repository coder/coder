package proxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
)

type hijackingWriter struct {
	http.ResponseWriter
	conn net.Conn
}

func (*hijackingWriter) Flush() {}

func (w *hijackingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHandlerRejectsUnexpectedUpstreamUpgrade(t *testing.T) {
	t.Parallel()

	upstreamBody := &trackingReadCloser{Reader: strings.NewReader("upgrade")}
	provider := &testutil.MockProvider{
		NameStr: "openai", URL: "https://openai.example.test", Bridged: []string{"/v1/chat"},
	}
	baseURL, err := url.Parse(provider.BaseURL())
	require.NoError(t, err)
	rec := &testutil.MockRecorder{}
	handler := &forwardingHandler{
		provider: provider,
		baseURL:  baseURL,
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusSwitchingProtocols,
				Header:     http.Header{"Connection": {"Upgrade"}, "Upgrade": {"h2c"}},
				Body:       upstreamBody,
			}, nil
		}),
		breaker:     circuitbreaker.NewProviderCircuitBreakers(provider.Name(), nil, nil, nil),
		failover:    keypool.KeyFailoverConfig{},
		recorder:    rec,
		logger:      slogtest.Make(t, nil),
		tracer:      noop.NewTracerProvider().Tracer(t.Name()),
		record:      true,
		metricRoute: "/v1/chat",
	}
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
	req = req.WithContext(aibcontext.AsActor(req.Context(), "actor", recorder.Metadata{}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)

	require.Equal(t, http.StatusBadGateway, response.Code)
	require.True(t, upstreamBody.closed)
	starts := rec.RecordedInterceptions()
	require.Len(t, starts, 1)
	end := rec.RecordedInterceptionEnd(starts[0].ID)
	require.NotNil(t, end)
	require.Contains(t, end.ErrorMessage, "upstream protocol upgrades are not supported")
	require.NotContains(t, end.ErrorMessage, "before EOF")
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestErrorCapturingResponseWriterOptionalInterfaces(t *testing.T) {
	t.Parallel()

	base := httptest.NewRecorder()
	wrapped := &errorCapturingResponseWriter{ResponseWriter: base}
	require.Same(t, base, wrapped.Unwrap())
	conn, rw, err := wrapped.Hijack()
	require.Nil(t, conn)
	require.Nil(t, rw)
	require.ErrorIs(t, err, http.ErrNotSupported)

	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	hijacker := &hijackingWriter{ResponseWriter: base, conn: server}
	wrapped = &errorCapturingResponseWriter{ResponseWriter: hijacker}
	conn, rw, err = wrapped.Hijack()
	require.NoError(t, err)
	require.Same(t, server, conn)
	require.NotNil(t, rw)
}

func TestTerminalErrorKeepsTransportFailure(t *testing.T) {
	t.Parallel()

	transportErr := xerrors.New("upstream protocol upgrades are not supported")
	aborted, err := terminalError(httptest.NewRequest(http.MethodGet, "/", nil), &forwardingState{
		body:         &observedBody{},
		transportErr: transportErr,
	}, nil)
	require.False(t, aborted)
	require.ErrorIs(t, err, transportErr)
}
