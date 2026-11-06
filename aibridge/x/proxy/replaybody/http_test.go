package replaybody_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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

// TestNewRequestProxy forwards real server requests through
// httputil.ReverseProxy to an echoing upstream. Uploads must reach upstream
// through the forwarding body, bodiless requests must complete capture
// without one, and every request must keep the inbound connection reusable.
func TestNewRequestProxy(t *testing.T) {
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
			upstream, _ := startServer(t, false, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(w, r.Body)
			})
			proxy := &httputil.ReverseProxy{
				Rewrite: func(r *httputil.ProxyRequest) {
					r.Out.URL.Scheme = "http"
					r.Out.URL.Host = strings.TrimPrefix(upstream.URL, "http://")
				},
			}
			captures := make(chan result, 2)
			gateway, client := startServer(t, tc.http2, func(w http.ResponseWriter, r *http.Request) {
				capture := replaybody.NewRequest(r)
				proxy.ServeHTTP(w, r)
				_ = capture.Close()
				data, err := capture.Wait(context.Background())
				captures <- result{data, err}
			})
			for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodPost} {
				var body io.Reader
				want := ""
				if method == http.MethodPost {
					body = strings.NewReader(payload)
					want = payload
				}
				req, err := http.NewRequestWithContext(ctx, method, gateway.URL, body)
				require.NoError(t, err)
				resp, err := client.Do(req)
				require.NoError(t, err, "completed requests must allow a response")
				got, err := io.ReadAll(resp.Body)
				require.NoError(t, resp.Body.Close())
				require.NoError(t, err, "completed requests must allow the response body to finish")
				require.Equal(t, http.StatusOK, resp.StatusCode, "the response status must be preserved")
				require.Equal(t, want, string(got), "upstream must receive exactly the forwarded upload")
				require.Equal(t, tc.http2, resp.ProtoMajor == 2, "request wrapping must preserve the client protocol")
				require.False(t, resp.Close, "completed requests must keep the connection open")
				capture := testutil.RequireReceive(ctx, t, captures)
				require.NoError(t, capture.err, "completed requests must seal without an error")
				require.Equal(t, want, string(capture.data), "completed capture must retain exactly the upload")
			}
		})
	}
}

// TestNewRequestReplay checks the wrapped request without a server: the
// declared length is preserved, and Body and GetBody each replay the full
// payload.
func TestNewRequestReplay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		prefix int
	}{
		{name: "FullAttempt"},
		{name: "PartialAttempt", prefix: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("payload"))
			capture := replaybody.NewRequest(request)
			defer capture.Close()
			require.EqualValues(t, len("payload"), request.ContentLength, "wrapping must preserve the declared length")
			require.NotNil(t, request.GetBody, "wrapped requests must expose GetBody")
			var initial io.Reader = request.Body
			wantInitial := "payload"
			if tc.prefix > 0 {
				initial = io.LimitReader(initial, int64(tc.prefix))
				wantInitial = wantInitial[:tc.prefix]
			}
			payload, err := io.ReadAll(initial)
			require.NoError(t, err, "the initial attempt must stream its requested bytes")
			require.Equal(t, wantInitial, string(payload), "the initial attempt must preserve its body prefix")
			require.NoError(t, request.Body.Close(), "closing an attempt must leave the source available for retries")
			replay, err := request.GetBody()
			require.NoError(t, err, "GetBody must create an independent body")
			defer replay.Close()
			payload, err = io.ReadAll(replay)
			require.NoError(t, err, "GetBody must replay the prefix and continue the source")
			require.Equal(t, "payload", string(payload), "the retry must receive the entire body")
			captured, err := capture.Wait(t.Context())
			require.NoError(t, err, "the final attempt must complete capture at EOF")
			require.Equal(t, payload, captured, "retries must not duplicate captured bytes")
		})
	}
}

// TestNewResponseStreamsAndPreservesMetadata forwards a transport response
// through the forwarding body. The body must stream before upstream EOF,
// capture must hold exactly the forwarded bytes, and trailers populated by the
// transport at EOF must stay visible on the response.
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

			b := replaybody.NewResponse(resp)
			t.Cleanup(func() { require.NoError(t, b.Close()) })
			require.False(t, original == resp.Body, "the response must read through the forwarding body")

			prefix := make([]byte, len("prefix"))
			_, err = io.ReadFull(resp.Body, prefix)
			require.NoError(t, err, "prefix must stream before upstream EOF")
			require.Equal(t, "prefix", string(prefix), "streamed bytes must match upstream data")
			close(release)
			suffix, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "forwarding must consume the suffix before capture completes")
			require.Equal(t, "suffix", string(suffix), "forwarding must preserve the remaining bytes")
			require.NoError(t, resp.Body.Close(), "forwarding bodies must be independently closable")

			payload, err := b.Wait(ctx)
			require.NoError(t, err, "forwarding to EOF must complete capture")
			require.Equal(t, "prefixsuffix", string(payload), "capture must retain exactly the forwarded response")
			require.Equal(t, http.StatusAccepted, resp.StatusCode, "response status must be preserved")
			require.Equal(t, wantMajor, resp.ProtoMajor, "response protocol must be preserved")
			require.Equal(t, "value", resp.Header.Get("X-Upstream"), "response headers must be preserved")
			require.Equal(t, wantLength, resp.ContentLength, "response content length must be preserved")
			if !tc.contentLength {
				require.Equal(t, "done", resp.Trailer.Get("X-Trailer"), "transport trailers must stay visible after EOF")
			}
		})
	}
}

// TestNewResponseProxyTrailers forwards captured responses through
// httputil.ReverseProxy. Trailers populated by the transport during the
// forwarding read must reach the downstream client without racing the proxy.
func TestNewResponseProxyTrailers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		declared bool
	}{
		{name: "Declared", declared: true},
		{name: "Undeclared"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upstream, client := startServer(t, false, func(w http.ResponseWriter, _ *http.Request) {
				name := "X-Trailer"
				if tc.declared {
					w.Header().Set("Trailer", name)
				} else {
					name = http.TrailerPrefix + name
				}
				_, _ = io.WriteString(w, "payload")
				w.(http.Flusher).Flush()
				w.Header().Set(name, "done")
			})
			captures := make(chan result, 2)
			gateway, downstream := startServer(t, false, func(w http.ResponseWriter, r *http.Request) {
				var capture *replaybody.TeeBuffer
				proxy := &httputil.ReverseProxy{
					Rewrite: func(r *httputil.ProxyRequest) {
						r.Out.URL.Scheme = "http"
						r.Out.URL.Host = strings.TrimPrefix(upstream.URL, "http://")
					},
					Transport: testutil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
						response, err := client.Transport.RoundTrip(r)
						if err != nil {
							return nil, err
						}
						capture = replaybody.NewResponse(response)
						return response, nil
					}),
				}
				proxy.ServeHTTP(w, r)
				_ = capture.Close()
				data, err := capture.Wait(context.Background())
				captures <- result{data, err}
			})
			ctx := testutil.Context(t, testutil.WaitShort)
			for range 2 {
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, gateway.URL, nil)
				require.NoError(t, err)
				response, err := downstream.Do(request)
				require.NoError(t, err)
				payload, err := io.ReadAll(response.Body)
				require.NoError(t, response.Body.Close())
				require.NoError(t, err, "the proxy must forward the response to EOF")
				require.Equal(t, "payload", string(payload), "the proxy must preserve the body")
				require.Equal(t, "done", response.Trailer.Get("X-Trailer"), "the proxy must forward transport-populated trailers")
				capture := testutil.RequireReceive(ctx, t, captures)
				require.NoError(t, capture.err, "proxying to EOF must complete capture")
				require.Equal(t, "payload", string(capture.data), "capture must retain the proxied response")
			}
		})
	}
}

// TestNewRequestZeroLength treats a zero ContentLength as no body, as
// httputil.ReverseProxy does. HTTP/2 servers deliver bodiless requests with a
// Body that reads EOF rather than http.NoBody.
func TestNewRequestZeroLength(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Body = io.NopCloser(strings.NewReader(""))
	capture := replaybody.NewRequest(request)
	t.Cleanup(func() { require.NoError(t, capture.Close()) })
	require.Equal(t, http.NoBody, request.Body, "zero-length bodies must become http.NoBody")
	require.Zero(t, request.ContentLength, "zero-length requests must keep a zero length")
	replay, err := request.GetBody()
	require.NoError(t, err)
	require.Equal(t, http.NoBody, replay, "zero-length replays must use http.NoBody")
	payload, err := capture.Wait(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, err, "zero-length capture must complete immediately")
	require.Empty(t, payload, "zero-length capture must retain no bytes")
}

func TestHTTPConstructorsEmpty(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body io.ReadCloser
	}{
		{name: "Nil"},
		{name: "NoBody", body: http.NoBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/", nil)
			request.Body = tc.body
			requestCapture := replaybody.NewRequest(request)
			require.NotNil(t, requestCapture, "empty requests must still return a buffer")
			require.Equal(t, http.NoBody, request.Body, "empty request bodies must use http.NoBody")
			require.Zero(t, request.ContentLength, "empty requests must keep a zero length")
			replay, err := request.GetBody()
			require.NoError(t, err, "GetBody must work for empty requests")
			require.Equal(t, http.NoBody, replay, "empty request replays must use http.NoBody")

			response := &http.Response{Body: tc.body}
			responseCapture := replaybody.NewResponse(response)
			require.NotNil(t, responseCapture, "empty responses must still return a buffer")
			require.Equal(t, http.NoBody, response.Body, "empty response bodies must use http.NoBody")
			require.NoError(t, response.Body.Close(), "empty response bodies must be safely closable")

			waitCtx := testutil.Context(t, testutil.WaitShort)
			for _, capture := range []*replaybody.TeeBuffer{requestCapture, responseCapture} {
				payload, err := capture.Wait(waitCtx)
				require.NoError(t, err, "empty capture must complete immediately")
				require.Empty(t, payload, "empty capture must retain no bytes")
				require.NoError(t, capture.Close(), "closing empty capture must succeed")
				payload, err = capture.Wait(waitCtx)
				require.NoError(t, err, "empty capture must stay complete after Close")
				require.Empty(t, payload, "empty capture must retain no bytes")
			}
		})
	}
}
