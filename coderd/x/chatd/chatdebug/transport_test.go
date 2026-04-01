package chatdebug //nolint:testpackage // Uses unexported recorder helpers.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
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
	require.Equal(t, 1, attempts[0].Number)
	require.Equal(t, RedactedValue, attempts[0].RequestHeaders["Authorization"])
	require.Equal(t, "application/json", attempts[0].RequestHeaders["Content-Type"])
	require.JSONEq(t, `{"message":"hello","api_key":"[REDACTED]"}`, string(attempts[0].RequestBody))

	received := <-gotRequest
	require.JSONEq(t, requestBody, string(received.body))
	require.Equal(t, "Bearer top-secret", received.authorization)
}

func TestRecordingTransport_CaptureResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
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
	require.Equal(t, http.StatusCreated, attempts[0].ResponseStatus)
	require.Equal(t, RedactedValue, attempts[0].ResponseHeaders["X-Api-Key"])
	require.Equal(t, "trace-123", attempts[0].ResponseHeaders["X-Trace-Id"])
	require.JSONEq(t, `{"token":"[REDACTED]","safe":"ok"}`, string(attempts[0].ResponseBody))
}

func TestRecordingTransport_CaptureResponseOnEOFWithoutClose(t *testing.T) {
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

	attempts := sink.snapshot()
	require.Len(t, attempts, 1)
	require.Equal(t, http.StatusAccepted, attempts[0].ResponseStatus)
	require.Equal(t, "application/json", attempts[0].ResponseHeaders["Content-Type"])
	require.Equal(t, RedactedValue, attempts[0].ResponseHeaders["X-Api-Key"])
	require.JSONEq(t, `{"token":"[REDACTED]","safe":"ok"}`, string(attempts[0].ResponseBody))
	require.NoError(t, resp.Body.Close())
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
	require.JSONEq(t, `{"safe":"stream","token":"[REDACTED]"}`, string(attempts[0].ResponseBody))
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
	require.Equal(t, 1, attempts[0].Number)
	require.Equal(t, RedactedValue, attempts[0].RequestHeaders["Authorization"])
	require.JSONEq(t, `{"password":"[REDACTED]","safe":"ok"}`, string(attempts[0].RequestBody))
	require.Zero(t, attempts[0].ResponseStatus)
	require.Equal(t, "transport exploded", attempts[0].Error)
	require.GreaterOrEqual(t, attempts[0].DurationMs, int64(0))
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
