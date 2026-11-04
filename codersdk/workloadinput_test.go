package codersdk_test

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestDialWorkloadInput(t *testing.T) {
	t.Parallel()
	org, id := uuid.New(), uuid.New()
	ctx := testutil.Context(t, testutil.WaitLong)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, fmt.Sprintf("/api/v2/organizations/%s/workloads/%s/input", org, id), r.URL.Path)
		assert.Empty(t, r.URL.RawQuery)
		assert.Equal(t, "callback-token", r.Header.Get("X-Callback-Token"))
		assert.Equal(t, "header-command", r.Header.Get("X-Custom-Header"))
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		conn.SetReadLimit(32 << 10)
		for range 3 {
			var input codersdk.WorkloadInput
			if !assert.NoError(t, wsjson.Read(ctx, conn, &input)) {
				return
			}
			assert.NoError(t, wsjson.Write(ctx, conn, input))
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	var plainLogs, structuredLogs bytes.Buffer
	client := codersdk.New(u, codersdk.WithHTTPClient(&http.Client{
		Transport: workloadStreamRoundTripper(func(req *http.Request) (*http.Response, error) {
			req.Header.Set("X-Custom-Header", "header-command")
			return http.DefaultTransport.RoundTrip(req)
		}),
	}), codersdk.WithLogBodies(), codersdk.WithPlainLogger(&plainLogs),
		codersdk.WithLogger(slog.Make(sloghuman.Sink(&structuredLogs)).Leveled(slog.LevelDebug)))
	provider := &workloadInputTokenProvider{}
	client.SessionTokenProvider = provider
	conn, err := client.DialWorkloadInput(ctx, org, id)
	require.NoError(t, err)
	defer conn.CloseNow()
	for _, input := range []codersdk.WorkloadInput{
		{Type: "data", Data: bytes.Repeat([]byte("s"), 16<<10)},
		{Type: "resize", Width: 120, Height: 40},
		{Type: "eof"},
	} {
		require.NoError(t, wsjson.Write(ctx, conn, input))
		var got codersdk.WorkloadInput
		require.NoError(t, wsjson.Read(ctx, conn, &got))
		require.Equal(t, input, got)
	}
	_, _, err = conn.Read(ctx)
	require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
	require.EqualValues(t, 1, provider.calls.Load())
	require.Empty(t, plainLogs.String(), "websocket data must never enter HTTP body logging")
	require.Empty(t, structuredLogs.String())
}

func TestDialWorkloadInputCancellation(t *testing.T) {
	t.Parallel()
	for _, closeConn := range []bool{false, true} {
		t.Run(fmt.Sprintf("Close=%t", closeConn), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			inputCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(closed)
				conn, err := websocket.Accept(w, r, nil)
				if !assert.NoError(t, err) {
					return
				}
				defer conn.CloseNow()
				_, _, _ = conn.Read(ctx)
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			var requestCtx context.Context
			client := codersdk.New(u, codersdk.WithHTTPClient(&http.Client{
				Transport: workloadStreamRoundTripper(func(req *http.Request) (*http.Response, error) {
					requestCtx = req.Context()
					return http.DefaultTransport.RoundTrip(req)
				}),
			}))
			conn, err := client.DialWorkloadInput(inputCtx, uuid.New(), uuid.New())
			require.NoError(t, err)
			defer conn.CloseNow()
			require.NoError(t, requestCtx.Err(), "dial must not cancel its read context on return")
			if closeConn {
				require.NoError(t, conn.CloseNow())
				require.NoError(t, inputCtx.Err())
			} else {
				cancel()
			}
			testutil.TryReceive(ctx, t, closed)
			require.ErrorIs(t, requestCtx.Err(), context.Canceled)
		})
	}
}

func TestDialWorkloadInputErrors(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusConflict, http.StatusTooManyRequests, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "private-input-error"})
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			var plainLogs, structuredLogs bytes.Buffer
			client := codersdk.New(u, codersdk.WithLogBodies(), codersdk.WithPlainLogger(&plainLogs),
				codersdk.WithLogger(slog.Make(sloghuman.Sink(&structuredLogs)).Leveled(slog.LevelDebug)))
			conn, err := client.DialWorkloadInput(t.Context(), uuid.New(), uuid.New())
			require.Nil(t, conn)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, status, sdkErr.StatusCode())
			require.Equal(t, "private-input-error", sdkErr.Message)
			var retry interface{ RetryAfter() time.Duration }
			if status == http.StatusTooManyRequests {
				require.ErrorAs(t, err, &retry)
				require.Equal(t, 7*time.Second, retry.RetryAfter())
			} else {
				require.False(t, errors.As(err, &retry))
			}
			require.EqualValues(t, 1, calls.Load())
			require.Empty(t, plainLogs.String())
			require.Empty(t, structuredLogs.String())
		})
	}
}

func TestDialWorkloadInputHandshakeTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u, err := url.Parse("http://workload.test")
		require.NoError(t, err)
		client := codersdk.New(u, codersdk.WithHTTPClient(&http.Client{
			Transport: workloadStreamRoundTripper(func(req *http.Request) (*http.Response, error) {
				<-req.Context().Done()
				return nil, req.Context().Err()
			}),
		}))
		started := time.Now()
		conn, err := client.DialWorkloadInput(t.Context(), uuid.New(), uuid.New())
		require.Nil(t, conn)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, 10*time.Second, time.Since(started))
	})
}

func TestDialWorkloadInputOutlivesHandshakeTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u, err := url.Parse("http://workload.test")
		require.NoError(t, err)
		body, peer := net.Pipe()
		defer peer.Close()
		var requestCtx context.Context
		httpClient := &http.Client{
			Timeout: time.Second,
			Transport: workloadStreamRoundTripper(func(req *http.Request) (*http.Response, error) {
				requestCtx = req.Context()
				// RFC 6455 mandates SHA-1 for the handshake accept key.
				key := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				return &http.Response{
					StatusCode: http.StatusSwitchingProtocols,
					Header: http.Header{
						"Connection": {"Upgrade"}, "Upgrade": {"websocket"},
						"Sec-Websocket-Accept": {base64.StdEncoding.EncodeToString(key[:])},
					},
					Body: body, Request: req,
				}, nil
			}),
		}
		client := codersdk.New(u, codersdk.WithHTTPClient(httpClient))
		conn, err := client.DialWorkloadInput(t.Context(), uuid.New(), uuid.New())
		require.NoError(t, err)
		defer conn.CloseNow()
		<-time.After(11 * time.Second)
		require.NoError(t, requestCtx.Err())
		require.Equal(t, time.Second, httpClient.Timeout)
		go func() { _, _ = peer.Write([]byte{0x81, 2, 'o', 'k'}) }()
		typ, data, err := conn.Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, websocket.MessageText, typ)
		require.Equal(t, "ok", string(data))
		require.NoError(t, conn.CloseNow())
		require.ErrorIs(t, requestCtx.Err(), context.Canceled)
	})
}

type workloadInputTokenProvider struct {
	codersdk.FixedSessionTokenProvider
	calls atomic.Int32
}

func (p *workloadInputTokenProvider) SetDialOption(opts *websocket.DialOptions) {
	p.calls.Add(1)
	opts.HTTPHeader = http.Header{"X-Callback-Token": {"callback-token"}}
}
