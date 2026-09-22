package keypool_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/quartz"
)

type trackingBody struct {
	*bytes.Reader
	closed bool
}

func newTrackingBody(body string) *trackingBody {
	return &trackingBody{Reader: bytes.NewReader([]byte(body))}
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingTrackingBody struct {
	*trackingBody
	err error
}

func (b *failingTrackingBody) Read(p []byte) (int, error) {
	if b.Len() == 0 {
		return 0, b.err
	}
	return b.Reader.Read(p)
}

func TestKeyFailoverTransportBodyReadErrorDoesNotSpendKey(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	m := metrics.NewMetrics(registry)
	pool, err := keypool.New("test", []string{"first"}, quartz.NewMock(t), m)
	require.NoError(t, err)
	readErr := xerrors.New("request read failed")
	original := &failingTrackingBody{trackingBody: newTrackingBody("payload"), err: readErr}
	called := false
	rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, xerrors.New("inner transport unexpectedly called")
	}), failoverConfig(pool))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", original)
	require.NoError(t, err)
	req.GetBody = nil

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.Nil(t, resp)
	require.ErrorIs(t, err, readErr)
	require.False(t, called)
	require.True(t, original.closed)
	require.Zero(t, promtest.CollectAndCount(m.KeyPoolFailoverAttempts))
}

func TestKeyFailoverTransportUsesGetBody(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New("test", []string{"first", "second", "third"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	original := newTrackingBody("payload")
	var attempts []string
	rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, req.Body.Close())
		attempts = append(attempts, req.Header.Get("Authorization")+":"+string(body))
		status := http.StatusOK
		if len(attempts) == 1 {
			status = http.StatusUnauthorized
		} else if len(attempts) == 2 {
			status = http.StatusTooManyRequests
		}
		return response(req, status), nil
	}), failoverConfig(pool))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", original)
	require.NoError(t, err)
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("payload"))), nil
	}

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []string{"first:payload", "second:payload", "third:payload"}, attempts)
	require.Equal(t, len("payload"), original.Len())
	require.True(t, original.closed)
}

func TestKeyFailoverTransportLegacyBodyReplay(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New("test", []string{"first", "second"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	original := newTrackingBody("payload")
	var attempts []string
	rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		attempts = append(attempts, string(body))
		status := http.StatusOK
		if len(attempts) == 1 {
			status = http.StatusUnauthorized
		}
		return response(req, status), nil
	}), failoverConfig(pool))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", original)
	require.NoError(t, err)
	req.GetBody = nil

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []string{"payload", "payload"}, attempts)
	require.Zero(t, original.Len())
	require.True(t, original.closed)
}

func TestKeyFailoverTransportPreservesNilBody(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New("test", []string{"first"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Nil(t, req.Body)
		return response(req, http.StatusOK), nil
	}), failoverConfig(pool))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", nil)
	require.NoError(t, err)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestKeyFailoverTransportUsesGetBodyWhenBodyIsNil(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New("test", []string{"first"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	getBodyCalled := false
	rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.NoError(t, req.Body.Close())
		require.Equal(t, "payload", string(body))
		return response(req, http.StatusOK), nil
	}), failoverConfig(pool))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", nil)
	require.NoError(t, err)
	req.GetBody = func() (io.ReadCloser, error) {
		getBodyCalled = true
		return io.NopCloser(bytes.NewReader([]byte("payload"))), nil
	}

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.True(t, getBodyCalled)
}

func TestKeyFailoverTransportDoesNotRotateNonKeyFailures(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		byok      bool
		status    int
		transport error
	}{
		{name: "BYOK", byok: true, status: http.StatusUnauthorized},
		{name: "Forbidden", status: http.StatusForbidden},
		{name: "ServerError", status: http.StatusInternalServerError},
		{name: "TransportError", transport: xerrors.New("network failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, err := keypool.New("test", []string{"first", "second"}, quartz.NewMock(t), nil)
			require.NoError(t, err)
			calls := 0
			cfg := failoverConfig(pool)
			cfg.IsBYOK = func(*http.Request) bool { return tc.byok }
			rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if tc.transport != nil {
					return nil, tc.transport
				}
				return response(req, tc.status), nil
			}), cfg)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", bytes.NewBufferString("payload"))
			require.NoError(t, err)

			resp, err := rt.RoundTrip(req)
			if tc.transport != nil {
				require.ErrorIs(t, err, tc.transport)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.status, resp.StatusCode)
				require.NoError(t, resp.Body.Close())
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestKeyFailoverTransportGetBodyErrorClosesReaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		getBodyFailures  int
		wantCalls        int
		wantMetricOutput string
	}{
		{
			name:            "before dispatch",
			getBodyFailures: 1,
		},
		{
			name:            "after one dispatch",
			getBodyFailures: 2,
			wantCalls:       1,
			wantMetricOutput: `
# HELP key_pool_failover_attempts The number of keys attempted before success or exhaustion, per interception for bridged requests and per request for passthrough requests.
# TYPE key_pool_failover_attempts histogram
key_pool_failover_attempts_bucket{provider="test",le="1.0"} 1
key_pool_failover_attempts_bucket{provider="test",le="2.0"} 1
key_pool_failover_attempts_bucket{provider="test",le="3.0"} 1
key_pool_failover_attempts_bucket{provider="test",le="4.0"} 1
key_pool_failover_attempts_bucket{provider="test",le="5.0"} 1
key_pool_failover_attempts_bucket{provider="test",le="10.0"} 1
key_pool_failover_attempts_bucket{provider="test",le="25.0"} 1
key_pool_failover_attempts_bucket{provider="test",le="+Inf"} 1
key_pool_failover_attempts_sum{provider="test"} 1
key_pool_failover_attempts_count{provider="test"} 1
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registry := prometheus.NewRegistry()
			m := metrics.NewMetrics(registry)
			pool, err := keypool.New("test", []string{"first", "second"}, quartz.NewMock(t), m)
			require.NoError(t, err)
			original := newTrackingBody("payload")
			var replays []*trackingBody
			replayErr := xerrors.New("replay failed")
			calls := 0
			rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				require.NoError(t, req.Body.Close())
				return response(req, http.StatusUnauthorized), nil
			}), failoverConfig(pool))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", original)
			require.NoError(t, err)
			req.GetBody = func() (io.ReadCloser, error) {
				replay := newTrackingBody("replay")
				replays = append(replays, replay)
				if len(replays) == tc.getBodyFailures {
					return replay, replayErr
				}
				return replay, nil
			}

			resp, err := rt.RoundTrip(req)
			if resp != nil {
				require.NoError(t, resp.Body.Close())
			}
			require.ErrorIs(t, err, replayErr)
			require.Equal(t, tc.wantCalls, calls)
			require.True(t, original.closed)
			for _, replay := range replays {
				require.True(t, replay.closed)
			}
			if tc.wantMetricOutput == "" {
				require.Zero(t, promtest.CollectAndCount(m.KeyPoolFailoverAttempts))
				return
			}
			require.NoError(t, promtest.CollectAndCompare(
				m.KeyPoolFailoverAttempts,
				strings.NewReader(tc.wantMetricOutput),
				"key_pool_failover_attempts",
			))
		})
	}
}

func failoverConfig(pool *keypool.Pool) keypool.KeyFailoverConfig {
	return keypool.KeyFailoverConfig{
		Pool:   pool,
		Logger: slog.Make(),
		IsBYOK: func(*http.Request) bool { return false },
		InjectAuthKey: func(h *http.Header, key string) {
			h.Set("Authorization", key)
		},
		BuildKeyPoolResponse: func(*keypool.Error) *http.Response { return nil },
	}
}

func response(req *http.Request, status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}
}
