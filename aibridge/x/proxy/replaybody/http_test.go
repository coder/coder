package replaybody_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

// proxyCaptures reports what one proxied request retained on each side.
type proxyCaptures struct {
	request  result
	response result
}

// TestNewRequestProxy is the request happy path through httputil.ReverseProxy
// with both captures installed. Uploads must reach an echoing upstream through
// replayBody, bodiless requests must complete capture without one, and every
// request must keep the inbound connection reusable.
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
			upstream, upstreamClient := startServer(t, false, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(w, r.Body)
			})
			target, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			captures := make(chan proxyCaptures, 3)
			gateway, client := startServer(t, tc.http2, func(w http.ResponseWriter, r *http.Request) {
				requestCapture := replaybody.NewRequest(r)
				var responseCapture *replaybody.TeeBuffer
				proxy := &httputil.ReverseProxy{
					Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(target) },
					Transport: testutil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
						response, err := upstreamClient.Transport.RoundTrip(r)
						if err != nil {
							return nil, err
						}
						responseCapture = replaybody.NewResponse(response)
						return response, nil
					}),
				}
				proxy.ServeHTTP(w, r)
				_ = requestCapture.Close()
				_ = responseCapture.Close()
				var c proxyCaptures
				c.request.data, c.request.err = requestCapture.Wait(context.Background())
				c.response.data, c.response.err = responseCapture.Wait(context.Background())
				captures <- c
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
				c := testutil.RequireReceive(ctx, t, captures)
				require.NoError(t, c.request.err, "completed uploads must complete capture")
				require.Equal(t, want, string(c.request.data), "request capture must retain exactly the upload")
				require.NoError(t, c.response.err, "proxied responses must complete capture")
				require.Equal(t, want, string(c.response.data), "response capture must retain exactly the echo")
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

// TestNewResponseProxy is the response happy path through
// httputil.ReverseProxy with both captures installed. The response must stream
// to the client before upstream EOF, both captures must hold exactly the bytes
// proxied, and trailers the transport sets at EOF must reach the client
// without racing the proxy.
func TestNewResponseProxy(t *testing.T) {
	t.Parallel()
	const upload = "upload"
	for _, tc := range []struct {
		name       string
		http2      bool
		undeclared bool
	}{
		{name: "HTTP1DeclaredTrailer"},
		{name: "HTTP1UndeclaredTrailer", undeclared: true},
		{name: "HTTP2DeclaredTrailer", http2: true},
		{name: "HTTP2UndeclaredTrailer", http2: true, undeclared: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			// Each request hands the upstream a channel that releases its suffix.
			releases := make(chan chan struct{}, 2)
			upstream, upstreamClient := startServer(t, tc.http2, func(w http.ResponseWriter, r *http.Request) {
				got, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, upload, string(got), "upstream must receive the upload")
				name := "X-Trailer"
				if tc.undeclared {
					name = http.TrailerPrefix + name
				} else {
					w.Header().Set("Trailer", name)
				}
				_, _ = io.WriteString(w, "prefix")
				w.(http.Flusher).Flush()
				select {
				case <-<-releases:
				case <-r.Context().Done():
					return
				}
				_, _ = io.WriteString(w, "suffix")
				w.Header().Set(name, "done")
			})
			target, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			captures := make(chan proxyCaptures, 2)
			gateway, client := startServer(t, false, func(w http.ResponseWriter, r *http.Request) {
				requestCapture := replaybody.NewRequest(r)
				var responseCapture *replaybody.TeeBuffer
				proxy := &httputil.ReverseProxy{
					Rewrite:       func(r *httputil.ProxyRequest) { r.SetURL(target) },
					FlushInterval: -1,
					Transport: testutil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
						response, err := upstreamClient.Transport.RoundTrip(r)
						if err != nil {
							return nil, err
						}
						if tc.undeclared {
							assert.Nil(t, response.Trailer, "undeclared trailers must start without a trailer map")
						} else {
							assert.Contains(t, response.Trailer, "X-Trailer", "declared trailers must start with the key announced")
							assert.Empty(t, response.Trailer.Get("X-Trailer"), "declared trailers must have no value before EOF")
						}
						original := response.Body
						responseCapture = replaybody.NewResponse(response)
						assert.False(t, original == response.Body, "the response must read through replayBody")
						return response, nil
					}),
				}
				proxy.ServeHTTP(w, r)
				_ = requestCapture.Close()
				_ = responseCapture.Close()
				var c proxyCaptures
				c.request.data, c.request.err = requestCapture.Wait(context.Background())
				c.response.data, c.response.err = responseCapture.Wait(context.Background())
				captures <- c
			})
			for range 2 {
				release := make(chan struct{})
				releases <- release
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL, strings.NewReader(upload))
				require.NoError(t, err)
				response, err := client.Do(request)
				require.NoError(t, err)
				prefix := make([]byte, len("prefix"))
				_, err = io.ReadFull(response.Body, prefix)
				require.NoError(t, err, "the proxy must stream before upstream EOF")
				require.Equal(t, "prefix", string(prefix))
				close(release)
				suffix, err := io.ReadAll(response.Body)
				require.NoError(t, response.Body.Close())
				require.NoError(t, err, "the proxy must forward the response to EOF")
				require.Equal(t, "suffix", string(suffix))
				require.Equal(t, "done", response.Trailer.Get("X-Trailer"), "the proxy must forward trailers the transport sets at EOF")
				c := testutil.RequireReceive(ctx, t, captures)
				require.NoError(t, c.request.err, "proxying the upload must complete request capture")
				require.Equal(t, upload, string(c.request.data), "request capture must retain exactly the upload")
				require.NoError(t, c.response.err, "proxying to EOF must complete response capture")
				require.Equal(t, "prefixsuffix", string(c.response.data), "response capture must retain exactly the proxied response")
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
