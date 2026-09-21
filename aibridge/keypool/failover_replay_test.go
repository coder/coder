package keypool_test

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/quartz"
)

type trackingBody struct {
	reader *bytes.Reader
	mu     sync.Mutex
	reads  int
	closed bool
}

func newTrackingBody(body string) *trackingBody {
	return &trackingBody{reader: bytes.NewReader([]byte(body))}
}

func (b *trackingBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	b.reads++
	b.mu.Unlock()
	return b.reader.Read(p)
}

func (b *trackingBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}

func (b *trackingBody) state() (reads int, closed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads, b.closed
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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
	reads, closed := original.state()
	require.Zero(t, reads)
	require.True(t, closed)
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
	reads, closed := original.state()
	require.Positive(t, reads)
	require.True(t, closed)
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

	pool, err := keypool.New("test", []string{"first"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	original := newTrackingBody("payload")
	replay := newTrackingBody("replay")
	replayErr := xerrors.New("replay failed")
	rt := keypool.NewKeyFailoverTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("inner transport called after replay failure")
		return nil, xerrors.New("inner transport unexpectedly called")
	}), failoverConfig(pool))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.test", original)
	require.NoError(t, err)
	req.GetBody = func() (io.ReadCloser, error) { return replay, replayErr }

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.ErrorIs(t, err, replayErr)
	_, originalClosed := original.state()
	_, replayClosed := replay.state()
	require.True(t, originalClosed)
	require.True(t, replayClosed)
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
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    req,
	}
}
