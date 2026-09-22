package proxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type hijackingWriter struct {
	http.ResponseWriter
	conn net.Conn
}

func (*hijackingWriter) Flush() {}

func (w *hijackingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

type failingResponseWriter struct {
	header http.Header
	err    error
}

func (w *failingResponseWriter) Header() http.Header       { return w.header }
func (*failingResponseWriter) WriteHeader(int)             {}
func (w *failingResponseWriter) Write([]byte) (int, error) { return 0, w.err }
func (*failingResponseWriter) Flush()                      {}
func (*failingResponseWriter) Unwrap() http.ResponseWriter { return nil }
func (*failingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}

func TestErrorCapturingResponseWriter(t *testing.T) {
	t.Parallel()

	writeErr := xerrors.New("client disconnected")
	wrapped := &errorCapturingResponseWriter{ResponseWriter: &failingResponseWriter{header: http.Header{}, err: writeErr}}
	_, err := wrapped.Write([]byte("response"))
	require.ErrorIs(t, err, writeErr)
	require.ErrorIs(t, wrapped.err, writeErr)

	base := httptest.NewRecorder()
	wrapped = &errorCapturingResponseWriter{ResponseWriter: base}
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
	wrapped = &errorCapturingResponseWriter{ResponseWriter: &hijackingWriter{ResponseWriter: base, conn: server}}
	conn, rw, err = wrapped.Hijack()
	require.NoError(t, err)
	require.Same(t, server, conn)
	require.NotNil(t, rw)
}

func TestTerminalError(t *testing.T) {
	t.Parallel()

	transportErr := xerrors.New("transport failed")
	readErr := xerrors.New("upstream read failed")
	writeErr := xerrors.New("client write failed")
	for _, tc := range []struct {
		name        string
		state       forwardingState
		panicValue  any
		wantAborted bool
		wantErr     error
	}{
		{name: "TransportFailure", state: forwardingState{transportErr: transportErr}, wantErr: transportErr},
		{name: "TransportFailureWithIncompleteBody", state: forwardingState{body: &observedBody{}, transportErr: transportErr}, wantErr: transportErr},
		{name: "UpstreamReadFailure", state: forwardingState{body: &observedBody{readErr: readErr}}, wantAborted: true, wantErr: readErr},
		{name: "ClientWriteFailure", state: forwardingState{writeErr: writeErr}, wantAborted: true, wantErr: writeErr},
		{name: "UnexpectedIncompleteBody", state: forwardingState{body: &observedBody{}}, wantAborted: true},
		{name: "Panic", panicValue: "copy panic", wantAborted: true},
		{name: "Complete", state: forwardingState{body: &observedBody{eof: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			aborted, err := terminalError(httptest.NewRequest(http.MethodGet, "/", nil), &tc.state, tc.panicValue)
			require.Equal(t, tc.wantAborted, aborted)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantAborted:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
		})
	}
}

type readFailureBody struct {
	read bool
}

func (b *readFailureBody) Read(p []byte) (int, error) {
	if !b.read {
		b.read = true
		return copy(p, "chunk"), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func (*readFailureBody) Close() error { return nil }

func TestObservedReadFailureAbortsHandler(t *testing.T) {
	t.Parallel()

	provider := &testutil.MockProvider{NameStr: "test", URL: "http://upstream.example.test"}
	handler := &forwardingHandler{
		provider: provider,
		baseURL:  mustURL(t, provider.BaseURL()),
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: &readFailureBody{}}, nil
		}),
		logger:      slogtest.Make(t, nil),
		tracer:      noop.NewTracerProvider().Tracer(t.Name()),
		metricRoute: "/v1/models",
	}
	response := httptest.NewRecorder()
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/test/v1/models", nil))
	})
	require.Equal(t, "chunk", response.Body.String())
}

func TestRewriteRequestPreservesRawQueryAndEscapedPath(t *testing.T) {
	t.Parallel()

	baseURL := mustURL(t, "https://upstream.example.test/base?configured=1")
	request := httptest.NewRequest(http.MethodGet, "http://gateway.test/test/models/a%2Fb?raw=a;b", nil)
	pr := &httputil.ProxyRequest{In: request, Out: request.Clone(request.Context())}
	rewriteRequest(pr, "/test", baseURL)
	require.Equal(t, "/base/models/a%2Fb", pr.Out.URL.EscapedPath())
	require.Equal(t, "configured=1&raw=a;b", pr.Out.URL.RawQuery)
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}
