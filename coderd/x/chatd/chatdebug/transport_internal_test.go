package chatdebug

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func newTestSinkContext(t *testing.T) (context.Context, *attemptSink) {
	t.Helper()

	sink := &attemptSink{}
	return withAttemptSink(context.Background(), sink), sink
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type scriptedReadCloser struct {
	chunks [][]byte
	index  int
	offset int // byte offset within current chunk
}

func (r *scriptedReadCloser) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.index]
	remaining := chunk[r.offset:]
	n := copy(p, remaining)
	r.offset += n
	if r.offset >= len(chunk) {
		r.index++
		r.offset = 0
	}
	return n, nil
}

func (*scriptedReadCloser) Close() error {
	return nil
}

type closeTrackingReadCloser struct {
	*bytes.Reader
	closed   bool
	closeErr error
}

func (c *closeTrackingReadCloser) Close() error {
	c.closed = true
	return c.closeErr
}

func TestRecordingTransport_NoSink(t *testing.T) {
	t.Parallel()

	gotMethod := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		gotMethod <- req.Method
		_, _ = rw.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "ok", string(body))
	require.Equal(t, http.MethodGet, <-gotMethod)
}

func TestRecordingTransport_CaptureRequest(t *testing.T) {
	t.Parallel()

	const requestBody = `{"message":"hello","api_key":"super-secret"}`

	type receivedRequest struct {
		authorization string
		body          []byte
	}
	gotRequest := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		gotRequest <- receivedRequest{
			authorization: req.Header.Get("Authorization"),
			body:          body,
		}
		_, _ = rw.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		server.URL,
		strings.NewReader(requestBody),
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer top-secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Equal(t, 1, attempts[0].Number)
	require.Equal(t, RedactedValue, attempts[0].RequestHeaders["Authorization"])
	require.Equal(t, "application/json", attempts[0].RequestHeaders["Content-Type"])
	require.JSONEq(t, `{"message":"hello","api_key":"[REDACTED]"}`, string(attempts[0].RequestBody))

	received := <-gotRequest
	require.JSONEq(t, requestBody, string(received.body))
	require.Equal(t, "Bearer top-secret", received.authorization)
}

func TestRecordingTransport_CaptureRequestRestoresSharedGetBody(t *testing.T) {
	t.Parallel()

	const requestBody = `{"message":"hello","api_key":"super-secret"}`

	gotRequest := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		gotRequest <- body
		_, _ = rw.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	reader := bytes.NewReader([]byte(requestBody))
	originalBody := &closeTrackingReadCloser{Reader: reader}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		server.URL,
		originalBody,
	)
	require.NoError(t, err)
	req.ContentLength = int64(len(requestBody))
	req.GetBody = func() (io.ReadCloser, error) {
		_, err := reader.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(reader), nil
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.JSONEq(t, requestBody, string(<-gotRequest))
	require.True(t, originalBody.closed)
	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.JSONEq(t, `{"message":"hello","api_key":"[REDACTED]"}`, string(attempts[0].RequestBody))
}

func TestRecordingTransport_CaptureRequestResetFailureFailsRequest(t *testing.T) {
	t.Parallel()

	const requestBody = `{"message":"hello"}`

	gotRequest := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		gotRequest <- struct{}{}
		_, _ = rw.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	reader := bytes.NewReader([]byte(requestBody))
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		server.URL,
		io.NopCloser(reader),
	)
	require.NoError(t, err)
	req.ContentLength = int64(len(requestBody))
	getBodyCalls := 0
	req.GetBody = func() (io.ReadCloser, error) {
		getBodyCalls++
		if getBodyCalls == 2 {
			return nil, xerrors.New("reset failed")
		}
		_, err := reader.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(reader), nil
	}

	resp, err := client.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.ErrorContains(t, err, "chatdebug: reset request body: reset failed")
	require.Nil(t, resp)
	require.Empty(t, sink.snapshot())
	select {
	case <-gotRequest:
		t.Fatal("request should not be sent with a drained body")
	default:
	}
}

func TestRecordingTransport_CaptureRequestBodyCloseFailureFailsRequest(t *testing.T) {
	t.Parallel()

	const requestBody = `{"message":"hello"}`

	gotRequest := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		gotRequest <- struct{}{}
		_, _ = rw.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	reader := bytes.NewReader([]byte(requestBody))
	originalBody := &closeTrackingReadCloser{
		Reader:   reader,
		closeErr: xerrors.New("close failed"),
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		server.URL,
		originalBody,
	)
	require.NoError(t, err)
	req.ContentLength = int64(len(requestBody))
	req.GetBody = func() (io.ReadCloser, error) {
		_, err := reader.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(reader), nil
	}

	resp, err := client.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.ErrorContains(t, err, "chatdebug: reset request body: close failed")
	require.Nil(t, resp)
	require.True(t, originalBody.closed)
	require.Empty(t, sink.snapshot())
	select {
	case <-gotRequest:
		t.Fatal("request should not be sent when the captured body cannot be closed")
	default:
	}
}

func TestRecordingTransport_RedactsSensitiveQueryParameters(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(`ok`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+`?api_key=secret&safe=ok`, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Contains(t, attempts[0].URL, "api_key=%5BREDACTED%5D")
	require.Contains(t, attempts[0].URL, "safe=ok")
}

func TestRecordingTransport_TruncatesLargeRequestBodies(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		_, _ = rw.Write([]byte(`ok`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}

	large := strings.Repeat("x", codersdk.DefaultChatDebugMaxBodyBytes+1024)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(large))
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Equal(t, []byte("[TRUNCATED]"), attempts[0].RequestBody)
}

func TestRecordingTransport_StripsURLUserinfo(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(`ok`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.Replace(server.URL, "http://", "http://user:secret@", 1)+`?api_key=secret`, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.NotContains(t, attempts[0].URL, "user:secret")
	require.Contains(t, attempts[0].URL, "api_key=%5BREDACTED%5D")
}

func TestRecordingTransport_SkipsNonReplayableRequestBodyCapture(t *testing.T) {
	t.Parallel()

	const requestBody = `{"message":"hello"}`
	gotRequest := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		gotRequest <- body
		_, _ = rw.Write([]byte(`ok`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, io.NopCloser(strings.NewReader(requestBody)))
	require.NoError(t, err)
	req.GetBody = nil

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.JSONEq(t, requestBody, string(<-gotRequest))
	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Nil(t, attempts[0].RequestBody)
}

func TestRecordingTransport_CaptureResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("X-API-Key", "response-secret")
		rw.Header().Set("X-Trace-ID", "trace-123")
		rw.WriteHeader(http.StatusCreated)
		_, _ = rw.Write([]byte(`{"token":"response-secret","safe":"ok"}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.JSONEq(t, `{"token":"response-secret","safe":"ok"}`, string(body))

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Equal(t, http.StatusCreated, attempts[0].ResponseStatus)
	require.Equal(t, "application/json", attempts[0].ResponseHeaders["Content-Type"])
	require.Equal(t, RedactedValue, attempts[0].ResponseHeaders["X-Api-Key"])
	require.Equal(t, "trace-123", attempts[0].ResponseHeaders["X-Trace-Id"])
	require.JSONEq(t, `{"token":"[REDACTED]","safe":"ok"}`, string(attempts[0].ResponseBody))
}

// TestRecordingTransport_CaptureResponseRecordsOnClose verifies that
// EOF recording is deferred to Close() rather than firing in Read().
// This ensures Close()'s validation logic (JSON integrity, content-
// length checks) always runs.
func TestRecordingTransport_CaptureResponseRecordsOnClose(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("X-API-Key", "response-secret")
		rw.WriteHeader(http.StatusAccepted)
		_, _ = rw.Write([]byte(`{"token":"response-secret","safe":"ok"}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"token":"response-secret","safe":"ok"}`, string(body))

	// Before Close(), the attempt should not yet be recorded
	// because EOF recording is deferred to Close().
	require.Empty(t, sink.snapshot(), "attempt should not be recorded before Close()")

	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, http.StatusAccepted, attempts[0].ResponseStatus)
	require.Equal(t, "application/json", attempts[0].ResponseHeaders["Content-Type"])
	require.Equal(t, RedactedValue, attempts[0].ResponseHeaders["X-Api-Key"])
	require.JSONEq(t, `{"token":"[REDACTED]","safe":"ok"}`, string(attempts[0].ResponseBody))
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
}

func TestRecordingTransport_StreamingBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		flusher, ok := rw.(http.Flusher)
		require.True(t, ok)

		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"safe":"stream",`))
		flusher.Flush()
		_, _ = rw.Write([]byte(`"token":"chunk-secret"}`))
		flusher.Flush()
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{Base: server.Client().Transport},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	buf := make([]byte, 5)
	var body strings.Builder
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := body.Write(buf[:n])
			require.NoError(t, writeErr)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
	}
	require.NoError(t, resp.Body.Close())
	require.JSONEq(t, `{"safe":"stream","token":"chunk-secret"}`, body.String())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.JSONEq(t, `{"safe":"stream","token":"[REDACTED]"}`, string(attempts[0].ResponseBody))
}

func TestRecordingTransport_CloseAfterDecoderConsumesContentLengthSucceeds(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"token":"response-secret","safe":"ok"}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	require.Equal(t, "ok", decoded["safe"])
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Empty(t, attempts[0].Error)
}

// Each case serves an unknown-length body, consumes it the way the case
// describes, closes it, and checks how the attempt was recorded.
func TestRecordingTransport_UnknownLengthClose(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		contentType string
		body        string
		readBytes   int
		decode      bool
		drain       bool
		wantStatus  string
		wantErr     string
		// wantBody is compared as JSON when set.
		wantBody string
	}{
		{
			name:        "CloseAfterDecoderConsumesJSONSucceeds",
			contentType: "application/json",
			body:        `{"token":"response-secret","safe":"ok"}`,
			decode:      true,
			wantStatus:  attemptStatusCompleted,
			wantBody:    `{"token":"[REDACTED]","safe":"ok"}`,
		},
		{
			// Exercises buildAttemptLocked's decodedJSON!=nil-but-changed==false
			// path: the completeness check precomputes a decoded value, but
			// nothing in it needs redacting, so the original bytes must be
			// recorded unchanged.
			name:        "CloseAfterDecoderConsumesJSONWithNoSensitiveKeysSucceeds",
			contentType: "application/json",
			body:        `{"safe":"ok","count":1}`,
			decode:      true,
			wantStatus:  attemptStatusCompleted,
			wantBody:    `{"safe":"ok","count":1}`,
		},
		{
			name:        "CloseAfterDecoderConsumesJSONWithTrailingDocumentMarksFailed",
			contentType: "application/json",
			body:        `{"token":"response-secret","safe":"ok"}{"token":"second"}`,
			decode:      true,
			wantStatus:  attemptStatusFailed,
			wantErr:     io.ErrUnexpectedEOF.Error(),
			wantBody:    `{"error":"chatdebug: body contains extra JSON values, redacted for safety"}`,
		},
		{
			name:        "CloseAfterDecoderConsumesNDJSONMarksFailed",
			contentType: "application/x-ndjson",
			body:        "{\"token\":\"response-secret\",\"safe\":\"ok\"}\n{\"token\":\"second\"}\n",
			decode:      true,
			wantStatus:  attemptStatusFailed,
			wantErr:     io.ErrUnexpectedEOF.Error(),
		},
		{
			name:        "CloseAfterDecoderDrainsSucceeds",
			contentType: "application/json",
			body:        `{"token":"response-secret","safe":"ok"}`,
			decode:      true,
			drain:       true,
			wantStatus:  attemptStatusCompleted,
		},
		{
			name:        "CloseWithoutReadingMarksFailed",
			contentType: "application/json",
			body:        `{"token":"response-secret","safe":"ok"}`,
			wantStatus:  attemptStatusFailed,
			wantErr:     io.ErrUnexpectedEOF.Error(),
		},
		{
			name:        "PrematureCloseMarksFailed",
			contentType: "application/json",
			body:        `{"token":"response-secret","safe":"ok"}`,
			readBytes:   5,
			wantStatus:  attemptStatusFailed,
			wantErr:     io.ErrUnexpectedEOF.Error(),
		},
		{
			name:        "SSEClosedEarlyMarksFailed",
			contentType: "text/event-stream",
			body:        "data: {\"token\":\"secret\"}\n\ndata: [DONE]\n\n",
			readBytes:   5,
			wantStatus:  attemptStatusFailed,
			wantErr:     io.ErrUnexpectedEOF.Error(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, sink := newTestSinkContext(t)
			client := &http.Client{
				Transport: &RecordingTransport{
					Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						return &http.Response{ //nolint:exhaustruct // Test response exercises unknown-length close semantics.
							StatusCode:    http.StatusOK,
							Header:        http.Header{"Content-Type": []string{tc.contentType}},
							Body:          &scriptedReadCloser{chunks: [][]byte{[]byte(tc.body)}},
							ContentLength: -1,
						}, nil
					}),
				},
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)

			if tc.readBytes > 0 {
				_, err = resp.Body.Read(make([]byte, tc.readBytes))
				require.NoError(t, err)
			}
			if tc.decode {
				var decoded map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
				require.Equal(t, "ok", decoded["safe"])
			}
			if tc.drain {
				_, err = io.Copy(io.Discard, resp.Body)
				require.NoError(t, err)
			}
			require.NoError(t, resp.Body.Close())

			attempts := sink.snapshot()
			require.Len(t, attempts, 1)
			require.Equal(t, tc.wantStatus, attempts[0].Status)
			require.Equal(t, tc.wantErr, attempts[0].Error)
			if tc.wantBody != "" {
				require.JSONEq(t, tc.wantBody, string(attempts[0].ResponseBody))
			}
		})
	}
}

func TestRecordingTransport_CloseWithoutReadingHeadResponseSucceeds(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test response exercises no-body close semantics.
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"application/json"}},
					Body:          &scriptedReadCloser{chunks: [][]byte{[]byte(`{"ignored":true}`)}},
					ContentLength: 13,
					Request:       req,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Empty(t, attempts[0].Error)
}

func TestRecordingTransport_PrematureCloseMarksFailed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(`{"token":"response-secret","safe":"ok"}`))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	buf := make([]byte, 5)
	_, err = resp.Body.Read(buf)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusFailed, attempts[0].Status)
	require.NotEmpty(t, attempts[0].Error, "failure-path attempt should record an Error")
}

func TestRecordingTransport_TruncatesLargeResponses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(strings.Repeat("x", codersdk.DefaultChatDebugMaxBodyBytes+1024)))
	}))
	defer server.Close()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{Transport: &RecordingTransport{Base: server.Client().Transport}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Equal(t, []byte("[TRUNCATED]"), attempts[0].ResponseBody)
}

func TestRecordingTransport_MaxBodyBytesCapsRequestAndResponse(t *testing.T) {
	t.Parallel()

	const maxBodyBytes = 64
	// Bodies must be valid JSON to survive redaction unchanged.
	jsonOfSize := func(size int) string {
		return `{"k":"` + strings.Repeat("x", size-8) + `"}`
	}

	for _, tc := range []struct {
		name     string
		payload  string
		wantBody []byte
	}{
		{name: "at cap is kept", payload: jsonOfSize(maxBodyBytes), wantBody: []byte(jsonOfSize(maxBodyBytes))},
		{name: "over cap is truncated", payload: jsonOfSize(maxBodyBytes + 1), wantBody: []byte("[TRUNCATED]")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := tc.payload
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.Header().Set("Content-Type", "application/json")
				_, _ = io.Copy(io.Discard, req.Body)
				_, _ = rw.Write([]byte(payload))
			}))
			defer server.Close()

			ctx, sink := newTestSinkContext(t)
			client := &http.Client{Transport: &RecordingTransport{
				Base:         server.Client().Transport,
				MaxBodyBytes: maxBodyBytes,
			}}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(payload))
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())

			attempts := sink.snapshot()
			require.Len(t, attempts, 1)
			require.Equal(t, attemptStatusCompleted, attempts[0].Status)
			require.Equal(t, tc.wantBody, attempts[0].RequestBody)
			require.Equal(t, tc.wantBody, attempts[0].ResponseBody)
		})
	}
}

func TestRecordingTransport_TransportError(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, xerrors.New("transport exploded")
			}),
		},
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://example.invalid",
		strings.NewReader(`{"password":"secret","safe":"ok"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer top-secret")

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	require.Nil(t, resp)
	require.EqualError(t, err, "Post \"http://example.invalid\": transport exploded")

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusFailed, attempts[0].Status)
	require.Equal(t, 1, attempts[0].Number)
	require.Equal(t, RedactedValue, attempts[0].RequestHeaders["Authorization"])
	require.JSONEq(t, `{"password":"[REDACTED]","safe":"ok"}`, string(attempts[0].RequestBody))
	require.Zero(t, attempts[0].ResponseStatus)
	require.Equal(t, "transport exploded", attempts[0].Error)
	require.GreaterOrEqual(t, attempts[0].DurationMs, int64(0))
}

func TestRecordingTransport_TransportErrorSanitizesURLCredentials(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, xerrors.New("connection to http://admin:s3cret@api.example.com/v1?api_key=sk-1234 refused")
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	require.Error(t, err)

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusFailed, attempts[0].Status)
	require.NotContains(t, attempts[0].Error, "s3cret")
	require.NotContains(t, attempts[0].Error, "sk-1234")
	require.Contains(t, attempts[0].Error, "api_key=%5BREDACTED%5D")
}

func TestRecordingTransport_NilBase(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{Transport: &RecordingTransport{}}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
}

// TestRecordingTransport_SSEEmptyBodyRecordsOnEOF verifies that an SSE
// response with zero bytes (immediate EOF on the first Read) still
// records a completed attempt. This covers the n == 0 && err == io.EOF
// branch in accumulateReadLocked where the buffer path is skipped but
// sawEOF must still fire the Read-path recording.
func TestRecordingTransport_SSEEmptyBodyRecordsOnEOF(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test SSE content type.
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"text/event-stream"}},
					Body:          io.NopCloser(strings.NewReader("")),
					ContentLength: -1,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req) //nolint:bodyclose // Intentionally skip Close() to verify EOF-only recording.
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Empty(t, body)

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Empty(t, attempts[0].Error)
	require.Empty(t, attempts[0].ResponseBody)
}

// TestRecordingTransport_SSEReadToEOFWithCloseErrorUpgrades verifies
// that when an SSE consumer reads to EOF (which eagerly records the
// attempt as completed) and then Close() fails because inner.Close()
// returns an error, the recorded attempt is upgraded to failed with
// the close error rather than silently remaining completed.
func TestRecordingTransport_SSEReadToEOFWithCloseErrorUpgrades(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	ssePayload := "data: {\"token\":\"secret\"}\n\ndata: [DONE]\n\n"
	closeErr := xerrors.New("boom: connection reset")
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test SSE content type.
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
					Body: &failingCloseReader{
						inner:    strings.NewReader(ssePayload),
						closeErr: closeErr,
					},
					ContentLength: -1,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, ssePayload, string(body))

	// Close must surface the inner close error to the caller...
	gotCloseErr := resp.Body.Close()
	require.ErrorIs(t, gotCloseErr, closeErr)

	// ...and the recorded attempt must reflect that failure instead of
	// the provisional completed entry written on EOF.
	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusFailed, attempts[0].Status)
	require.Contains(t, attempts[0].Error, "boom: connection reset")
	require.Equal(t, ssePayload, string(attempts[0].ResponseBody))
}

// TestRecordingBody_SSEConcurrentReadCloseNoDeadlock exercises the
// lock-ordering contract between record() and recordProvisional()
// under concurrent Read/Close on an SSE body. An earlier revision
// where record() entered recordOnce.Do before acquiring r.mu (while
// recordProvisional() acquired r.mu first) deadlocked when one
// goroutine won the Once but then blocked on r.mu while the other
// held r.mu and blocked on the Once.
func TestRecordingBody_SSEConcurrentReadCloseNoDeadlock(t *testing.T) {
	t.Parallel()

	const iterations = 200
	ssePayload := []byte("data: ping\n\n")

	for i := range iterations {
		sink := &attemptSink{}
		body := &recordingBody{
			inner:         io.NopCloser(strings.NewReader(string(ssePayload))),
			contentLength: -1,
			contentType:   "text/event-stream",
			maxBodyBytes:  codersdk.DefaultChatDebugMaxBodyBytes,
			sink:          sink,
			startedAt:     time.Now(),
			base:          Attempt{Number: sink.nextAttemptNumber()},
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)
			for {
				if _, err := body.Read(buf); err != nil {
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			_ = body.Close()
		}()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(testutil.WaitShort):
			t.Fatalf("deadlock detected on iteration %d", i)
		}
	}
}

// Each case reads a body to EOF and checks what the transport records for
// its content type: SSE and plain text are preserved as-is rather than
// replaced with a redaction diagnostic, JSON-like bodies are redacted, and
// bodies that claim JSON but are not fail closed to a diagnostic.
func TestRecordingTransport_RecordedBodyByContentType(t *testing.T) {
	t.Parallel()

	ssePayload := "data: {\"token\":\"secret\"}\n\ndata: [DONE]\n\n"
	cases := []struct {
		name          string
		statusCode    int
		header        http.Header
		body          string
		contentLength int64
		// skipClose abandons the response after EOF without Close().
		skipClose bool
		// Exactly one of wantRaw or wantJSONLines describes the recorded body.
		wantRaw       string
		wantJSONLines []string
	}{
		{
			name:          "SSEReadToEOFMarksCompleted",
			header:        http.Header{"Content-Type": []string{"text/event-stream"}},
			body:          ssePayload,
			contentLength: -1,
			wantRaw:       ssePayload,
		},
		{
			// SSE consumers that reach EOF and abandon the response without
			// calling Close() (the pattern fantasy's Anthropic SSE adapter
			// follows) must still populate the attempt sink. Close()-only
			// recording would leave the chat_turn step's attempts field
			// permanently empty.
			name:          "SSEReadToEOFWithoutCloseStillRecords",
			header:        http.Header{"Content-Type": []string{"text/event-stream"}},
			body:          ssePayload,
			contentLength: -1,
			skipClose:     true,
			wantRaw:       ssePayload,
		},
		{
			name:          "TextPlainPreservedNotRedacted",
			header:        http.Header{"Content-Type": []string{"text/plain"}},
			body:          "This is plain text, not JSON.",
			contentLength: int64(len("This is plain text, not JSON.")),
			wantRaw:       "This is plain text, not JSON.",
		},
		{
			// NDJSON bodies are redacted per line rather than treated as
			// non-JSON and preserved raw.
			name:          "NDJSONRedacted",
			header:        http.Header{"Content-Type": []string{"application/x-ndjson"}},
			body:          "{\"api_key\":\"sk-123\",\"safe\":\"ok\"}\n{\"token\":\"tok-456\",\"data\":\"value\"}\n",
			contentLength: int64(len("{\"api_key\":\"sk-123\",\"safe\":\"ok\"}\n{\"token\":\"tok-456\",\"data\":\"value\"}\n")),
			wantJSONLines: []string{
				`{"api_key":"[REDACTED]","safe":"ok"}`,
				`{"token":"[REDACTED]","data":"value"}`,
			},
		},
		{
			// Content types with a +json suffix (e.g. application/vnd.api+json)
			// are treated as JSON-like.
			name:          "PlusJSONSuffixRedacted",
			header:        http.Header{"Content-Type": []string{"application/vnd.api+json"}},
			body:          `{"token":"secret","safe":"ok"}`,
			contentLength: int64(len(`{"token":"secret","safe":"ok"}`)),
			wantJSONLines: []string{`{"token":"[REDACTED]","safe":"ok"}`},
		},
		{
			// A non-canonical lowercase header key is not found by
			// http.Header.Get and must default to JSON redaction rather than
			// the raw-body preservation path.
			name:          "UnrecognizedContentTypeDefaultsToJSONRedaction",
			header:        http.Header{"content-type": []string{"application/json"}},
			body:          `{"token":"secret","safe":"ok"}`,
			contentLength: int64(len(`{"token":"secret","safe":"ok"}`)),
			wantJSONLines: []string{`{"token":"[REDACTED]","safe":"ok"}`},
		},
		{
			// An empty Content-Type takes the JSON-or-unknown branch in
			// record(), and RedactJSONSecrets fails closed on a body that is
			// not valid JSON so raw HTML that could carry credentials is
			// never recorded.
			name:          "NonJSONBodyFailClosedRedaction",
			statusCode:    http.StatusBadGateway,
			header:        http.Header{},
			body:          `<html><body>502 Bad Gateway</body></html>`,
			contentLength: int64(len(`<html><body>502 Bad Gateway</body></html>`)),
			wantJSONLines: []string{`{"error":"chatdebug: body is not valid JSON, redacted for safety"}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			statusCode := tc.statusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			ctx, sink := newTestSinkContext(t)
			client := &http.Client{
				Transport: &RecordingTransport{
					Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						return &http.Response{ //nolint:exhaustruct // Test response exercises content type handling.
							StatusCode:    statusCode,
							Header:        tc.header,
							Body:          io.NopCloser(strings.NewReader(tc.body)),
							ContentLength: tc.contentLength,
						}, nil
					}),
				},
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
			require.NoError(t, err)

			resp, err := client.Do(req) //nolint:bodyclose // Closed below unless the case verifies EOF-only recording.
			require.NoError(t, err)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			if !tc.skipClose {
				require.NoError(t, resp.Body.Close())
			}
			// The caller always sees the original, unredacted payload.
			require.Equal(t, tc.body, string(body))

			attempts := sink.snapshot()
			require.Len(t, attempts, 1)
			require.Equal(t, attemptStatusCompleted, attempts[0].Status)
			require.Empty(t, attempts[0].Error)
			if tc.wantJSONLines == nil {
				require.Equal(t, tc.wantRaw, string(attempts[0].ResponseBody))
				return
			}
			lines := strings.Split(strings.TrimSuffix(string(attempts[0].ResponseBody), "\n"), "\n")
			require.Len(t, lines, len(tc.wantJSONLines))
			for i, want := range tc.wantJSONLines {
				require.JSONEq(t, want, lines[i])
			}
		})
	}
}

// TestRecordingTransport_TruncatedUnknownLengthMarksCompleted verifies
// that an unknown-length (chunked) response that exceeds the recording
// buffer is marked as completed, not failed. The caller consumed the
// body successfully; we just couldn't buffer all of it.
func TestRecordingTransport_TruncatedUnknownLengthMarksCompleted(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	largeBody := strings.Repeat("x", codersdk.DefaultChatDebugMaxBodyBytes+1024)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test unknown-length body.
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"application/octet-stream"}},
					Body:          io.NopCloser(strings.NewReader(largeBody)),
					ContentLength: -1,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Len(t, body, codersdk.DefaultChatDebugMaxBodyBytes+1024)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Empty(t, attempts[0].Error)
	require.Equal(t, []byte("[TRUNCATED]"), attempts[0].ResponseBody)
}

// errorAfterReadCloser returns data for the first N reads, then an error.
type errorAfterReadCloser struct {
	data   []byte
	offset int
	errAt  int // byte offset at which to return the error
	err    error
}

func (r *errorAfterReadCloser) Read(p []byte) (int, error) {
	if r.offset >= r.errAt {
		return 0, r.err
	}
	remaining := r.data[r.offset:]
	if len(remaining) > len(p) {
		remaining = remaining[:len(p)]
	}
	if r.offset+len(remaining) > r.errAt {
		remaining = remaining[:r.errAt-r.offset]
	}
	n := copy(p, remaining)
	r.offset += n
	if r.offset >= r.errAt {
		return n, r.err
	}
	return n, nil
}

func (*errorAfterReadCloser) Close() error {
	return nil
}

// TestRecordingTransport_MidStreamReadError verifies that a non-EOF
// read error during body consumption is recorded immediately with
// "failed" status and the correct error message.
func TestRecordingTransport_MidStreamReadError(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test mid-stream error.
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"application/json"}},
					Body:          &errorAfterReadCloser{data: []byte(`{"key":"value"}`), errAt: 10, err: io.ErrUnexpectedEOF},
					ContentLength: -1,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusFailed, attempts[0].Status)
	require.Equal(t, io.ErrUnexpectedEOF.Error(), attempts[0].Error)
}

// trackingReadCloser wraps a reader and counts total bytes delivered
// via Read. Close always succeeds.
type trackingReadCloser struct {
	inner     io.Reader
	bytesRead int64
	closed    bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

// failingCloseReader reads normally but returns an error on Close.
type failingCloseReader struct {
	inner    io.Reader
	closeErr error
}

func (r *failingCloseReader) Read(p []byte) (int, error) {
	return r.inner.Read(p)
}

func (r *failingCloseReader) Close() error {
	return r.closeErr
}

// TestRecordingTransport_MaxDrainBytesRespected verifies that
// drainToEOF stops after maxDrainBytes, preventing unbounded reads.
// The test uses a tracking reader to assert the byte cap.
func TestRecordingTransport_MaxDrainBytesRespected(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)

	// Build a body where json.Decoder consumes the first JSON document
	// but leaves trailing whitespace larger than maxDrainBytes. The
	// drain path should stop after maxDrainBytes, not read everything.
	jsonDoc := `{"safe":"ok"}`
	// Trailing whitespace much larger than maxDrainBytes. The drain
	// should consume at most maxDrainBytes of it.
	trailing := strings.Repeat(" ", maxDrainBytes*2)
	fullBody := jsonDoc + trailing

	tracker := &trackingReadCloser{inner: strings.NewReader(fullBody)}
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test maxDrainBytes.
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"application/json"}},
					Body:          tracker,
					ContentLength: -1,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	require.Equal(t, "ok", decoded["safe"])
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)

	// The key assertion: total bytes read through the tracker should
	// be bounded. The json.Decoder reads the JSON doc (~13 bytes),
	// then drainToEOF reads at most maxDrainBytes more. Without the
	// cap, the full body (maxDrainBytes*2 + 13) would be consumed.
	maxExpected := int64(len(jsonDoc)) + int64(maxDrainBytes) + 4096 // small buffer overhead
	require.Less(t, tracker.bytesRead, int64(len(fullBody)),
		"drain should NOT have consumed the entire body")
	require.LessOrEqual(t, tracker.bytesRead, maxExpected,
		"total bytes read should be bounded by maxDrainBytes")
	require.True(t, tracker.closed, "inner body should be closed")
}

// TestRecordingTransport_InnerCloseError verifies that an error from
// the inner body's Close() is recorded as a failed attempt and
// returned to the caller.
func TestRecordingTransport_InnerCloseError(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	closeErr := xerrors.New("connection reset by peer")
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test close error.
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{"application/json"}},
					Body:          &failingCloseReader{inner: strings.NewReader(`{"ok":true}`), closeErr: closeErr},
					ContentLength: -1,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = resp.Body.Close()
	require.Error(t, err)
	require.Contains(t, err.Error(), "connection reset by peer")

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusFailed, attempts[0].Status)
	require.Contains(t, attempts[0].Error, "connection reset by peer")
}

// TestRecordingTransport_204NoContentSucceeds verifies that a 204 No
// Content response is marked completed when closed without reading.
func TestRecordingTransport_204NoContentSucceeds(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test 204 no-body.
					StatusCode:    http.StatusNoContent,
					Header:        http.Header{},
					Body:          io.NopCloser(strings.NewReader("")),
					ContentLength: 0,
					Request:       req,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://example.invalid/resource", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Empty(t, attempts[0].Error)
}

// TestRecordingTransport_304NotModifiedSucceeds verifies that a 304
// Not Modified response is marked completed when closed without
// reading, even when Content-Length is non-zero.
func TestRecordingTransport_304NotModifiedSucceeds(t *testing.T) {
	t.Parallel()

	ctx, sink := newTestSinkContext(t)
	client := &http.Client{
		Transport: &RecordingTransport{
			Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{ //nolint:exhaustruct // Test 304 no-body.
					StatusCode:    http.StatusNotModified,
					Header:        http.Header{"Content-Type": []string{"application/json"}},
					Body:          io.NopCloser(strings.NewReader("")),
					ContentLength: 42,
					Request:       req,
				}, nil
			}),
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/resource", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, attemptStatusCompleted, attempts[0].Status)
	require.Empty(t, attempts[0].Error)
}
