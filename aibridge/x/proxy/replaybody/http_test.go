package replaybody_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/x/proxy/replaybody"
	"github.com/coder/coder/v2/testutil"
)

// startServer serves handler over plain HTTP/1 or, with http2, TLS with HTTP/2.
//
//nolint:revive // http2 is a control flag selecting the test server protocol.
func startServer(t *testing.T, http2 bool, handler http.HandlerFunc) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	if http2 {
		srv.EnableHTTP2 = true
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)
	return srv, srv.Client()
}

// captureServer wraps each request with NewRequest, runs the test handler on
// the wrapped writer and request, then closes the capture and reports what it
// retained. The handler plays the body consumer, for example a proxy.
type captureServer struct {
	client   *http.Client
	url      string
	captures chan captureResult
}

type captureResult struct {
	remote string // Client address, to detect inbound connection reuse.
	data   []byte
	err    error
}

// captureHandler receives the wrapped writer and request. cancel stops
// capture from the handler side with the given cause.
type captureHandler func(w http.ResponseWriter, r *http.Request, cancel context.CancelCauseFunc)

func startCaptureServer(t *testing.T, http2 bool, handle captureHandler) *captureServer {
	t.Helper()
	s := &captureServer{captures: make(chan captureResult, 2)}
	srv, client := startServer(t, http2, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancelCause(r.Context())
		defer cancel(nil)
		rw, wrapped, capture := replaybody.NewRequest(w, r.WithContext(ctx))
		handle(rw, wrapped, cancel)
		_ = capture.Close()
		data, err := capture.Wait(context.Background())
		s.captures <- captureResult{remote: r.RemoteAddr, data: data, err: err}
	})
	s.client = client
	s.url = srv.URL
	return s
}

// post builds a POST request to the server carrying body.
func (s *captureServer) post(ctx context.Context, t *testing.T, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, body)
	require.NoError(t, err)
	return req
}

// stalledUpload returns a request body that delivers prefix and then blocks
// until the returned writer is closed, which cleanup does at the latest.
func stalledUpload(t *testing.T, prefix string) (io.Reader, *io.PipeWriter) {
	t.Helper()
	body, upload := io.Pipe()
	t.Cleanup(func() { _ = upload.Close() })
	go func() { _, _ = io.WriteString(upload, prefix) }()
	return body, upload
}

// TestNewRequestEarlyReply replies while the client upload is still stalled.
// The reply must reach the client without waiting for the upload, closing the
// capture must not block on the unfinished body, HTTP/1 must announce a
// connection close, and capture must retain the prefix and seal as interrupted.
func TestNewRequestEarlyReply(t *testing.T) {
	t.Parallel()
	const prefix = "prefix"
	for _, tc := range []struct {
		name  string
		http2 bool
	}{
		{name: "HTTP1"},
		{name: "HTTP2", http2: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			s := startCaptureServer(t, tc.http2, func(w http.ResponseWriter, r *http.Request, _ context.CancelCauseFunc) {
				got := make([]byte, len(prefix))
				_, err := io.ReadFull(r.Body, got)
				assert.NoError(t, err, "the handler must stream the prefix before replying")
				assert.Equal(t, prefix, string(got), "the streamed prefix must match the upload")
				w.WriteHeader(http.StatusTooManyRequests)
				assert.NoError(t, http.NewResponseController(w).Flush(), "the early reply must flush")
			})
			body, upload := stalledUpload(t, prefix)
			defer upload.Close()
			resp, err := s.client.Do(s.post(ctx, t, body))
			require.NoError(t, err, "early reply must arrive while the upload is stalled")
			defer resp.Body.Close()
			require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "the early status must reach the client")
			require.Equal(t, tc.http2, resp.ProtoMajor == 2, "request wrapping must preserve the client protocol")
			require.Equal(t, !tc.http2, resp.Close, "HTTP/1 must not reuse a connection with an unfinished upload")
			got := testutil.RequireReceive(ctx, t, s.captures)
			require.Equal(t, prefix, string(got.data), "early replies must retain the uploaded prefix")
			require.ErrorIs(t, got.err, io.ErrClosedPipe, "unfinished uploads must seal as partial capture")
		})
	}
}

// TestNewRequestCancellation cancels a request while the handler is blocked
// reading its stalled upload, from the client or from the handler context.
// The blocked read must fail, capture must retain the prefix and seal as
// interrupted, and the client request must finish.
func TestNewRequestCancellation(t *testing.T) {
	t.Parallel()
	const prefix = "prefix"
	for _, tc := range []struct {
		name          string
		http2         bool
		handlerCancel bool
	}{
		{name: "HTTP1ClientCancel"},
		{name: "HTTP1HandlerCancel", handlerCancel: true},
		{name: "HTTP2ClientCancel", http2: true},
		{name: "HTTP2HandlerCancel", http2: true, handlerCancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			reading := make(chan struct{}, 1)
			s := startCaptureServer(t, tc.http2, func(_ http.ResponseWriter, r *http.Request, cancel context.CancelCauseFunc) {
				got := make([]byte, len(prefix))
				_, err := io.ReadFull(r.Body, got)
				assert.NoError(t, err, "the handler must stream the prefix before cancellation")
				assert.Equal(t, prefix, string(got), "the streamed prefix must match the upload")
				if tc.handlerCancel {
					cancel(context.Canceled)
				}
				reading <- struct{}{}
				_, err = io.Copy(io.Discard, r.Body)
				assert.ErrorIs(t, err, io.ErrClosedPipe, "cancellation must interrupt the blocked body read")
			})
			body, upload := stalledUpload(t, prefix)
			defer upload.Close()
			reqCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			req := s.post(reqCtx, t, body)
			done := make(chan struct{})
			go func() {
				defer close(done)
				resp, err := s.client.Do(req)
				if err == nil {
					_ = resp.Body.Close()
				}
			}()
			testutil.RequireReceive(ctx, t, reading)
			if !tc.handlerCancel {
				cancel()
			}
			got := testutil.RequireReceive(ctx, t, s.captures)
			require.Equal(t, prefix, string(got.data), "cancellation must retain the uploaded prefix")
			require.ErrorIs(t, got.err, io.ErrClosedPipe, "cancellation must seal capture as interrupted")
			if tc.handlerCancel {
				require.ErrorIs(t, got.err, context.Canceled, "handler cancellation cause must reach capture")
			}
			// Release the stalled body so the client request can finish.
			require.NoError(t, upload.Close())
			testutil.TryReceive(ctx, t, done)
		})
	}
}

// TestNewRequestCompletedReuse reads the upload to EOF before replying. The
// wrapped writer must drop its Connection: close default so HTTP/1 clients
// reuse the inbound connection, and capture must seal with the full upload.
func TestNewRequestCompletedReuse(t *testing.T) {
	t.Parallel()
	const payload = "payload"
	for _, tc := range []struct {
		name  string
		http2 bool
	}{
		{name: "HTTP1"},
		{name: "HTTP2", http2: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			s := startCaptureServer(t, tc.http2, func(w http.ResponseWriter, r *http.Request, _ context.CancelCauseFunc) {
				got, err := io.ReadAll(r.Body)
				assert.NoError(t, err, "completed uploads must reach EOF through the reader")
				assert.Equal(t, payload, string(got), "the reader must deliver the full upload")
				_, _ = io.WriteString(w, "ok")
			})
			send := func() string {
				t.Helper()
				resp, err := s.client.Do(s.post(ctx, t, strings.NewReader(payload)))
				require.NoError(t, err, "completed uploads must allow a response")
				defer resp.Body.Close()
				got, err := io.ReadAll(resp.Body)
				require.NoError(t, err, "completed uploads must allow the response body to finish")
				require.Equal(t, http.StatusOK, resp.StatusCode, "the response status must be preserved")
				require.Equal(t, "ok", string(got), "the response body must be preserved")
				require.False(t, resp.Close, "completed uploads must keep the connection open")
				capture := testutil.RequireReceive(ctx, t, s.captures)
				require.Equal(t, payload, string(capture.data), "completed capture must retain the full upload")
				require.NoError(t, capture.err, "completed uploads must seal without an error")
				return capture.remote
			}
			first := send()
			require.Equal(t, first, send(), "completed uploads must reuse the inbound connection")
		})
	}
}

// TestNewRequestReplay checks the cloned request without a server: headers are
// independent of the original, an unknown length becomes -1 so ReverseProxy
// keeps the body, and Body and GetBody each replay the full payload.
func TestNewRequestReplay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		contentLength int64
		wantLength    int64
	}{
		{name: "UnknownLength", contentLength: 0, wantLength: -1},
		{name: "KnownLength", contentLength: 7, wantLength: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("payload"))
			request.ContentLength = tc.contentLength
			request.Header.Set("X-Original", "original")
			_, outbound, capture := replaybody.NewRequest(httptest.NewRecorder(), request)
			require.NotNil(t, capture, "requests with bodies must return a capture buffer")
			defer capture.Close()
			outbound.Header.Set("X-Original", "outbound")
			require.Equal(t, "original", request.Header.Get("X-Original"), "forwarding headers must not mutate the original request")
			require.Equal(t, tc.wantLength, outbound.ContentLength, "preserve known length or use -1 for ReverseProxy")
			require.NotNil(t, outbound.GetBody, "wrapped requests must expose GetBody")
			replay, err := outbound.GetBody()
			require.NoError(t, err, "GetBody must create an independent reader")
			defer replay.Close()
			for _, reader := range []io.Reader{outbound.Body, replay} {
				payload, err := io.ReadAll(reader)
				require.NoError(t, err, "Body and GetBody must replay without errors")
				require.Equal(t, "payload", string(payload), "Body and GetBody must each replay the full payload")
			}
		})
	}
}

// TestNewRequestAbortedUploadTail replies over HTTP/1 while the client still
// owes declared body bytes, through every way a handler can start a reply.
// The reply must announce a close, and the server must drop the connection
// instead of parsing the rest of the upload as the next request.
func TestNewRequestAbortedUploadTail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		reply func(http.ResponseWriter)
		want  int
	}{
		{name: "Implicit", reply: func(http.ResponseWriter) {}, want: http.StatusOK},
		{name: "Explicit", reply: func(w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) }, want: http.StatusTooManyRequests},
		{name: "Write", reply: func(w http.ResponseWriter) { _, _ = io.WriteString(w, "denied") }, want: http.StatusOK},
		{name: "Flush", reply: func(w http.ResponseWriter) { _ = http.NewResponseController(w).Flush() }, want: http.StatusOK},
		{name: "Informational", reply: func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(http.StatusOK)
		}, want: http.StatusOK},
		{name: "InformationalOnly", reply: func(w http.ResponseWriter) { w.WriteHeader(http.StatusEarlyHints) }, want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var handled atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handled.Add(1)
				rw, _, capture := replaybody.NewRequest(w, r)
				prefix, _ := capture.NewReader()
				_, err := io.ReadFull(prefix, make([]byte, len("prefix")))
				assert.NoError(t, err, "capture must publish the prefix before replying")
				tc.reply(rw)
				_ = capture.Close() // Aborting returns the read deadline error.
			}))
			t.Cleanup(srv.Close)
			conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			require.NoError(t, conn.SetDeadline(time.Now().Add(testutil.WaitShort)))
			_, err = io.WriteString(conn, "POST /upload HTTP/1.1\r\nHost: gateway\r\nContent-Length: 100\r\n\r\nprefix")
			require.NoError(t, err)
			reader := bufio.NewReader(conn)
			resp, err := http.ReadResponse(reader, nil)
			require.NoError(t, err, "early reply must arrive before upload completion")
			for resp.StatusCode < http.StatusOK {
				require.NoError(t, resp.Body.Close())
				resp, err = http.ReadResponse(reader, nil)
				require.NoError(t, err, "final reply must follow informational headers")
			}
			require.Equal(t, tc.want, resp.StatusCode, "wrapping must preserve the final reply status")
			require.True(t, resp.Close, "an early reply must announce the close")
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			// Send a well-formed request as the rest of the declared upload.
			_, _ = io.WriteString(conn, "GET /tail HTTP/1.1\r\nHost: gateway\r\n\r\n")
			_, err = http.ReadResponse(reader, nil) //nolint:bodyclose // A failed read has no body.
			require.Error(t, err, "the server must close instead of answering the tail")
			require.EqualValues(t, 1, handled.Load(), "the tail must not reach a handler")
		})
	}
}

func TestNewResponseStreamsAndPreservesMetadata(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		http2         bool
		contentLength bool
		undeclared    bool
	}{
		{name: "HTTP1Trailer"},
		{name: "HTTP1ContentLength", contentLength: true},
		{name: "HTTP2Trailer", http2: true},
		{name: "HTTP2ContentLength", http2: true, contentLength: true},
		{name: "HTTP1UndeclaredTrailer", undeclared: true},
		{name: "HTTP2UndeclaredTrailer", http2: true, undeclared: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			release := make(chan struct{})
			upstream, client := startServer(t, tc.http2, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Upstream", "value")
				if tc.contentLength {
					w.Header().Set("Content-Length", "12")
				} else if !tc.undeclared {
					w.Header().Set("Trailer", "X-Trailer")
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, "prefix")
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				_, _ = io.WriteString(w, "suffix")
				if !tc.contentLength {
					name := "X-Trailer"
					if tc.undeclared {
						name = http.TrailerPrefix + name
					}
					w.Header().Set(name, "done")
				}
			})
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
			require.NoError(t, err)
			//nolint:bodyclose // Ownership transfers to capture, which cleanup closes.
			resp, err := client.Do(req)
			require.NoError(t, err)
			wantMajor := 1
			if tc.http2 {
				wantMajor = 2
			}
			wantLength := int64(-1)
			if tc.contentLength {
				wantLength = 12
			}
			original := resp.Body
			if tc.undeclared {
				require.Nil(t, resp.Trailer, "undeclared trailers must start without a trailer map")
			}

			copied, b := replaybody.NewResponse(ctx, resp)
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			require.NotSame(t, resp, copied, "wrapping must not reuse the original response object")
			require.True(t, original == resp.Body, "original response body must not be replaced")
			require.False(t, original == copied.Body, "copy must read from the buffer")

			prefix := make([]byte, len("prefix"))
			_, err = io.ReadFull(copied.Body, prefix)
			require.NoError(t, err, "prefix must stream before upstream EOF")
			require.Equal(t, "prefix", string(prefix), "streamed bytes must match upstream data")
			require.NoError(t, copied.Body.Close(), "response readers must be independently closable")
			close(release)

			payload, err := b.Wait(ctx)
			require.NoError(t, err, "closing a reader must not stop capture")
			require.Equal(t, "prefixsuffix", string(payload), "capture must retain the full response after reader close")
			require.Equal(t, http.StatusAccepted, copied.StatusCode, "response status must be preserved")
			require.Equal(t, wantMajor, copied.ProtoMajor, "response protocol must be preserved")
			require.Equal(t, "value", copied.Header.Get("X-Upstream"), "response headers must be preserved")
			require.Equal(t, wantLength, copied.ContentLength, "response content length must be preserved")
			if !tc.contentLength {
				require.Equal(t, "done", copied.Trailer.Get("X-Trailer"), "transport trailers must reach the copy")
			}
		})
	}
}

func TestNewResponseProtocolUpgrade(t *testing.T) {
	t.Parallel()
	upstream, peer := net.Pipe()
	defer upstream.Close()
	defer peer.Close()
	//nolint:bodyclose // The caller retains ownership of the upgraded connection.
	response, buffer := replaybody.NewResponse(t.Context(), &http.Response{StatusCode: http.StatusSwitchingProtocols, Body: upstream})
	if buffer != nil {
		t.Cleanup(func() { _ = buffer.Close() })
	}
	require.Nil(t, buffer, "upgraded streams must not start capture")
	require.Same(t, upstream, response.Body, "upgraded bodies must pass through unchanged")
	_, ok := response.Body.(io.ReadWriteCloser)
	require.True(t, ok, "upgraded bodies must stay bidirectional")
}

func TestNewResponseContextStopsCapture(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		http2 bool
	}{
		{name: "HTTP1"},
		{name: "HTTP2", http2: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			upstreamClosed := make(chan struct{})
			upstream, client := startServer(t, tc.http2, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "prefix")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(upstreamClosed)
			})
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
			require.NoError(t, err)
			//nolint:bodyclose // Ownership transfers to capture, which cleanup closes.
			resp, err := client.Do(req)
			require.NoError(t, err)

			captureCtx, cancel := context.WithCancelCause(context.Background())
			t.Cleanup(func() { cancel(nil) })
			copied, b := replaybody.NewResponse(captureCtx, resp)
			t.Cleanup(func() { _ = copied.Body.Close() })
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			prefix := make([]byte, len("prefix"))
			_, err = io.ReadFull(copied.Body, prefix)
			require.NoError(t, err, "response readers must stream before cancellation")
			require.Equal(t, "prefix", string(prefix), "streamed prefix must match upstream data")

			cause := xerrors.New("capture canceled")
			cancel(cause)
			payload, err := b.Wait(ctx)
			require.ErrorIs(t, err, io.ErrClosedPipe, "canceling capture must report interruption")
			require.ErrorIs(t, err, cause, "canceling the constructor context must preserve its cause")
			require.Equal(t, "prefix", string(payload), "cancellation must retain the received prefix")
			require.NoError(t, ctx.Err(), "client request context must stay live")
			// Canceling capture must also close the upstream stream.
			testutil.TryReceive(ctx, t, upstreamClosed)
		})
	}
}

func TestHTTPConstructorsEmpty(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		body     io.ReadCloser
		canceled bool
	}{
		{name: "Nil"},
		{name: "NoBody", body: http.NoBody},
		{name: "CanceledNoBody", body: http.NoBody, canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			request := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
			request.Body = tc.body
			recorder := httptest.NewRecorder()
			rw, outbound, requestCapture := replaybody.NewRequest(recorder, request)
			require.NotNil(t, requestCapture, "empty requests must still return a buffer")
			require.Same(t, recorder, rw, "empty bodies must not wrap the writer")
			t.Cleanup(func() { require.NoError(t, requestCapture.Close()) })
			require.NotSame(t, request, outbound, "empty requests must still be cloned")
			require.Equal(t, http.NoBody, outbound.Body, "empty request bodies must use http.NoBody")
			replay, err := outbound.GetBody()
			require.NoError(t, err, "GetBody must work for empty requests")
			require.Equal(t, http.NoBody, replay, "empty request replays must use http.NoBody")
			response := &http.Response{Body: tc.body, ContentLength: 0}

			forwarded, responseCapture := replaybody.NewResponse(ctx, response)
			require.NotNil(t, responseCapture, "empty responses must still return a buffer")
			t.Cleanup(func() { require.NoError(t, responseCapture.Close()) })
			require.NotSame(t, response, forwarded, "empty responses must still be copied")
			require.Equal(t, http.NoBody, forwarded.Body, "empty response bodies must use http.NoBody")
			require.NoError(t, forwarded.Body.Close(), "empty response bodies must be safely closable")
			require.Zero(t, forwarded.ContentLength, "empty responses must preserve zero length")
			waitCtx := testutil.Context(t, testutil.WaitShort)
			for _, capture := range []*replaybody.Buffer{requestCapture, responseCapture} {
				require.NoError(t, capture.Close(), "closing empty capture must succeed")
				require.NoError(t, capture.Stop(context.Canceled), "stopping empty capture must be harmless")
				payload, err := capture.Wait(waitCtx)
				require.NoError(t, err, "empty capture must stay complete after cancellation or Close")
				require.Empty(t, payload, "empty capture must retain no bytes")
				payload, err = io.ReadAll(newReader(t, capture))
				require.NoError(t, err, "empty readers must reach EOF immediately")
				require.Empty(t, payload, "empty readers must yield no bytes")
			}
		})
	}
}
